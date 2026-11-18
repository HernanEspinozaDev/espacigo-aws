package migrator

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"golang.org/x/sys/unix"
)

const e1Version99Name = "V000099__early_rental_manual_contract.sql"
const e1Version99Checksum = "ff466a678929b32d413128f1a885b9bfce3177c5176be1d7ab3ea2d2eab1fa29"

var e1RDSEndpoint = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}\.[a-z0-9]{1,63}\.us-east-2\.rds\.amazonaws\.com$`)

// E1RDSTransport belongs to the pinned operation. Its values identify files,
// never inline credentials. The CA checksum pins the TLS trust roots.
type E1RDSTransport struct {
	CAFile       string
	CASHA256     string
	PasswordFile string
}

func validateE1RDSTarget(t SingleOwnerMigrationTarget) bool {
	return e1RDSEndpoint.MatchString(t.Host) && t.Database == "espacigo_prod" && t.Port == 5432 &&
		t.SessionUser == "espacigo_migrator_login" && t.EffectiveRole == "espacigo_schema_owner" &&
		t.OccupancyOwner == t.EffectiveRole && t.LedgerOwner == t.EffectiveRole
}

func validateE1Transport(op E1MigrationOperation) error {
	if op.RDS == nil {
		if !singleOwnerLocalHost(op.Migration.Target.Host) {
			return errors.New("E1 remote transport requires an explicitly reviewed RDS operation")
		}
		return nil
	}
	if !validateE1RDSTarget(op.Migration.Target) ||
		op.Migration.Version != 99 || op.Migration.Name != e1Version99Name || op.Migration.Checksum != e1Version99Checksum {
		return errors.New("E1 RDS requires the exact Ohio production target, dedicated owner, and canonical V99")
	}
	if !validHex(op.RDS.CASHA256, 64) ||
		!validE1AbsolutePath(op.RDS.CAFile) || !validE1AbsolutePath(op.RDS.PasswordFile) {
		return errors.New("E1 RDS requires pinned CA and password file paths")
	}
	return nil
}

func validE1AbsolutePath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path
}

func readE1RDSFile(path string, limit int64, private bool) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, errors.New("cannot open an E1 RDS transport file")
	}
	f := os.NewFile(uintptr(fd), path)
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit ||
		(private && info.Mode().Perm() != 0600 && info.Mode().Perm() != 0400) {
		return nil, errors.New("invalid E1 RDS transport file type, size, or permissions")
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) != info.Size() {
		return nil, errors.New("could not read an E1 RDS transport file")
	}
	return data, nil
}

func configureE1RDS(cfg *pgx.ConnConfig, rds E1RDSTransport) error {
	ca, err := readE1RDSFile(rds.CAFile, 1<<20, false)
	if err != nil {
		return err
	}
	roots := x509.NewCertPool()
	if sha256Hex(ca) != rds.CASHA256 || !roots.AppendCertsFromPEM(ca) {
		return errors.New("E1 RDS CA bundle differs from the reviewed certificate material")
	}
	password, err := readE1RDSFile(rds.PasswordFile, 4096, true)
	if err != nil {
		return err
	}
	cfg.Password = strings.TrimSuffix(strings.TrimSuffix(string(password), "\n"), "\r")
	if cfg.Password == "" || strings.ContainsAny(cfg.Password, "\r\n\x00") {
		return errors.New("invalid E1 RDS password file content")
	}
	cfg.TLSConfig = &tls.Config{RootCAs: roots, ServerName: cfg.Host, MinVersion: tls.VersionTLS12}
	return nil
}

func verifyE1RDSTLS(conn *pgx.Conn, host string) error {
	tlsConn, ok := conn.PgConn().Conn().(*tls.Conn)
	if !ok {
		return errors.New("E1 RDS requires a verified TLS connection")
	}
	state := tlsConn.ConnectionState()
	if !state.HandshakeComplete || state.Version < tls.VersionTLS12 ||
		state.ServerName != host || len(state.VerifiedChains) == 0 {
		return errors.New("E1 RDS TLS endpoint identity was not verified")
	}
	return nil
}
