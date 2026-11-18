package migrator

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgproto3"
)

const e1TestRDSEndpoint = "espacigo-db-prod.c123test.us-east-2.rds.amazonaws.com"

func testE1RDSOperation(t *testing.T) (E1MigrationOperation, tls.Certificate) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), DNSNames: []string{e1TestRDSEndpoint},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		BasicConstraintsValid: true, IsCA: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, public, private)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	rds := &E1RDSTransport{
		CAFile: filepath.Join(root, "ca.pem"), CASHA256: sha256Hex(ca),
		PasswordFile: filepath.Join(root, "password"),
	}
	if err := os.WriteFile(rds.CAFile, ca, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rds.PasswordFile, []byte("test-password\n"), 0400); err != nil {
		t.Fatal(err)
	}
	return E1MigrationOperation{RDS: rds, Migration: SingleOwnerMigrationContract{
		Version: 99, Name: e1Version99Name, Checksum: e1Version99Checksum,
		Target: SingleOwnerMigrationTarget{
			Host: e1TestRDSEndpoint, Port: 5432, Database: "espacigo_prod",
			SessionUser: "espacigo_migrator_login", EffectiveRole: "espacigo_schema_owner",
			OccupancyOwner: "espacigo_schema_owner", LedgerOwner: "espacigo_schema_owner",
		},
	}}, tls.Certificate{Certificate: [][]byte{der}, PrivateKey: private}
}

func TestE1RDSRequiresPinnedTargetAndTransport(t *testing.T) {
	valid, _ := testE1RDSOperation(t)
	cfg, err := e1ConnectionConfig(valid)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLSConfig == nil || cfg.TLSConfig.InsecureSkipVerify || cfg.TLSConfig.ServerName != e1TestRDSEndpoint ||
		cfg.TLSConfig.MinVersion != tls.VersionTLS12 || cfg.Password != "test-password" || len(cfg.Fallbacks) != 0 {
		t.Fatal("expected strict TLS and explicit credentials")
	}
	for name, change := range map[string]func(*E1MigrationOperation){
		"implicit remote": func(op *E1MigrationOperation) { op.RDS = nil },
		"other region": func(op *E1MigrationOperation) {
			op.Migration.Target.Host = strings.Replace(e1TestRDSEndpoint, "us-east-2", "us-west-2", 1)
		},
		"other database": func(op *E1MigrationOperation) { op.Migration.Target.Database = "staging" },
		"admin role":     func(op *E1MigrationOperation) { op.Migration.Target.SessionUser = "espacigo_admin" },
		"other owner":    func(op *E1MigrationOperation) { op.Migration.Target.LedgerOwner = "espacigo_admin" },
		"other SQL":      func(op *E1MigrationOperation) { op.Migration.Checksum = strings.Repeat("0", 64) },
		"changed CA":     func(op *E1MigrationOperation) { op.RDS.CASHA256 = strings.Repeat("0", 64) },
		"missing secret": func(op *E1MigrationOperation) { op.RDS.PasswordFile += ".missing" },
	} {
		t.Run(name, func(t *testing.T) {
			op, rds := valid, *valid.RDS
			op.RDS = &rds
			change(&op)
			if _, err := e1ConnectionConfig(op); err == nil {
				t.Fatal("accepted mismatched RDS operation")
			}
		})
	}
	if err := os.Chmod(valid.RDS.PasswordFile, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := e1ConnectionConfig(valid); err == nil {
		t.Fatal("accepted nonprivate password file")
	}
}

// The fixture server has no network interface. This test-only relay checks the
// real pgx TLS negotiation on loopback and forwards PostgreSQL to its socket.
func relayE1TestPostgres(t *testing.T, socket string, certificate tls.Certificate, cfg *pgx.ConnConfig) {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	cfg.LookupFunc = func(context.Context, string) ([]string, error) { return []string{"127.0.0.1"}, nil }
	cfg.DialFunc = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp4", listener.Addr().String())
	}
	go func() {
		for {
			client, err := listener.Accept()
			if err != nil {
				return
			}
			go relayE1TestConnection(client, socket, certificate)
		}
	}()
}

func relayE1TestConnection(client net.Conn, socket string, certificate tls.Certificate) {
	defer client.Close()
	var request [8]byte
	if _, err := io.ReadFull(client, request[:]); err != nil {
		return
	}
	if err := (&pgproto3.SSLRequest{}).Decode(request[4:]); err != nil {
		return
	}
	if _, err := client.Write([]byte{'S'}); err != nil {
		return
	}
	tlsClient := tls.Server(client, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
	if err := tlsClient.Handshake(); err != nil {
		return
	}
	db, err := net.Dial("unix", socket)
	if err != nil {
		return
	}
	defer db.Close()
	go func() {
		_, _ = io.Copy(db, tlsClient)
		_ = db.Close()
	}()
	_, _ = io.Copy(tlsClient, db)
}
