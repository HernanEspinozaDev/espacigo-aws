package profilekyc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/profilepin"
	"github.com/HernanEspinozaDev/espaciGo/internal/verification"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotReady      = errors.New("identity profile incomplete")
	ErrUsernameTaken = errors.New("username unavailable")
	ErrRUTLocked     = errors.New("verified identity number cannot be changed")
	ErrUnavailable   = errors.New("identity verification unavailable")
	ErrConflict      = errors.New("identity verification conflict")
	rutInputPattern  = regexp.MustCompile(`^(?:[0-9]{1,8}-?|[0-9]{1,2}(?:\.[0-9]{3}){1,2}-?)[0-9K]$`)
	usernamePattern  = regexp.MustCompile("^[A-Za-z0-9_][A-Za-z0-9_.-]{2,29}$")
	phonePattern     = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
)

type FieldError struct{ Fields map[string]string }

func (e *FieldError) Error() string { return "invalid identity profile" }

type Data struct {
	Username         string `json:"username"`
	FirstName        string `json:"first_name"`
	FirstLastName    string `json:"first_last_name"`
	SecondLastName   string `json:"second_last_name,omitempty"`
	NoSecondLastName bool   `json:"no_second_last_name"`
	RUT              string `json:"rut"`
	DateOfBirth      string `json:"date_of_birth"`
	DocumentType     string `json:"document_type"`
	Phone            string `json:"phone"`
	Consent          bool   `json:"consent"`
}

func (d Data) LastName() string  { return strings.TrimSpace(d.FirstLastName + " " + d.SecondLastName) }
func (d Data) LegalName() string { return d.FirstName + " " + d.LastName() }

type Status struct {
	Username      string     `json:"username,omitempty"`
	Complete      bool       `json:"complete"`
	State         string     `json:"state"`
	MaskedEmail   string     `json:"masked_email,omitempty"`
	MaskedRUT     string     `json:"masked_rut,omitempty"`
	MaskedPhone   string     `json:"masked_phone,omitempty"`
	VerifiedUntil *time.Time `json:"verified_until,omitempty"`
	RUTLocked     bool       `json:"rut_locked"`
	DraftSaved    bool       `json:"draft_saved"`
}

type Keyring interface {
	Seal(aad, plaintext []byte) (string, []byte, error)
	Open(version string, aad, ciphertext []byte) ([]byte, error)
}

type Service struct {
	pool    *pgxpool.Pool
	keys    Keyring
	rutKeys *verification.RutHMACKeyring
	now     func() time.Time
}

func New(pool *pgxpool.Pool, keys Keyring, rutKeys *verification.RutHMACKeyring, now func() time.Time) *Service {
	return &Service{pool: pool, keys: keys, rutKeys: rutKeys, now: now}
}

func compactInput(value string) string { return strings.Join(strings.Fields(value), " ") }

func validName(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > 100 {
		return false
	}
	hasLetter := false
	for _, char := range value {
		if unicode.IsLetter(char) {
			hasLetter = true
		} else if !unicode.IsMark(char) && !strings.ContainsRune(" -'’", char) {
			return false
		}
	}
	return hasLetter
}

func normalizeRUT(raw string) (string, error) {
	raw = strings.ToUpper(strings.ReplaceAll(strings.Join(strings.Fields(raw), ""), "–", "-"))
	if !rutInputPattern.MatchString(raw) {
		return "", verification.ErrInvalid
	}
	if !strings.Contains(raw, "-") {
		raw = raw[:len(raw)-1] + "-" + raw[len(raw)-1:]
	}
	rut, err := verification.NormalizeRut(raw)
	if err != nil || strings.HasPrefix(rut, "-") {
		return "", verification.ErrInvalid
	}
	return rut, nil
}

func normalize(data Data, now time.Time) (Data, error) {
	invalid := make(map[string]string)
	data.Username = strings.TrimSpace(data.Username)
	if !usernamePattern.MatchString(data.Username) {
		invalid["username"] = "invalid_username"
	}
	data.FirstName = compactInput(data.FirstName)
	data.FirstLastName = compactInput(data.FirstLastName)
	data.SecondLastName = compactInput(data.SecondLastName)
	if !validName(data.FirstName) {
		invalid["first_name"] = "invalid_name"
	}
	if !validName(data.FirstLastName) {
		invalid["first_last_name"] = "invalid_name"
	}
	if data.NoSecondLastName && data.SecondLastName != "" || !data.NoSecondLastName && !validName(data.SecondLastName) {
		invalid["second_last_name"] = "invalid_second_last_name"
	}
	if canonical, err := normalizeRUT(data.RUT); err == nil {
		data.RUT = canonical
	} else {
		invalid["rut"] = "invalid_rut"
	}
	if verification.ValidateAdultBirthDate(data.DateOfBirth, now) != nil {
		invalid["date_of_birth"] = "invalid_birth_date"
	}
	if data.DocumentType != "cl_id" {
		invalid["document_type"] = "invalid_document_type"
	}
	data.Phone = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(data.Phone)
	if !phonePattern.MatchString(data.Phone) {
		invalid["phone"] = "invalid_phone"
	}
	if !data.Consent {
		invalid["consent"] = "consent_required"
	}
	if len(invalid) > 0 {
		return Data{}, &FieldError{Fields: invalid}
	}
	return data, nil
}

func revisionAAD(account, revision string) []byte {
	return []byte("espacigo:account_identity_profile:v1:" + account + ":" + revision)
}

func decryptData(keys Keyring, account, revision, key string, ciphertext []byte) (Data, error) {
	if keys == nil {
		return Data{}, ErrUnavailable
	}
	raw, err := keys.Open(key, revisionAAD(account, revision), ciphertext)
	if err != nil {
		return Data{}, ErrUnavailable
	}
	var data Data
	if err := json.Unmarshal(raw, &data); err != nil {
		return Data{}, ErrUnavailable
	}
	return data, nil
}

func (s *Service) Status(ctx context.Context, account string) (Status, error) {
	var out Status
	var email string
	now := s.now().UTC()
	err := s.pool.QueryRow(ctx, `
		SELECT u.correo_original,COALESCE(p.username,''),p.account_id IS NOT NULL,
		       COALESCE(CASE WHEN a.state='verified' AND a.verified_until<=$2 THEN 'expired' ELSE a.state END,
		                CASE WHEN p.account_id IS NULL THEN 'incomplete' ELSE 'ready' END),
		       COALESCE(r.masked_rut,''),COALESCE(r.masked_phone,''),
		       CASE WHEN a.state='verified' THEN a.verified_until END,
		       EXISTS (SELECT 1 FROM public.account_identity_rut_claim c WHERE c.account_id=u.id),
		       EXISTS (SELECT 1 FROM public.account_identity_profile_draft d WHERE d.account_id=u.id)
		FROM public.usuario u
		LEFT JOIN public.account_identity_profile p ON p.account_id=u.id
		LEFT JOIN public.account_identity_profile_revision r ON r.account_id=u.id AND r.id=p.current_revision
		LEFT JOIN LATERAL (
		    SELECT v.state,v.verified_until FROM public.account_identity_verification v
		    WHERE v.account_id=u.id AND v.profile_revision=p.current_revision
		    ORDER BY v.started_at DESC,v.id DESC LIMIT 1
		) a ON true
		WHERE u.id=$1 AND u.estado='activo'`, account, now).Scan(
		&email, &out.Username, &out.Complete, &out.State, &out.MaskedRUT, &out.MaskedPhone, &out.VerifiedUntil, &out.RUTLocked, &out.DraftSaved,
	)
	if err != nil {
		return Status{}, err
	}
	out.MaskedEmail = identity.MaskEmail(email)
	return out, nil
}

func mapWriteError(err error) error {
	var dbErr *pgconn.PgError
	if errors.As(err, &dbErr) && dbErr.ConstraintName == "account_identity_profile_username_unique" {
		return ErrUsernameTaken
	}
	return err
}

func lockAccount(ctx context.Context, tx pgx.Tx, account string) error {
	var ok int
	return tx.QueryRow(ctx, `SELECT 1 FROM public.usuario WHERE id=$1 AND estado='activo' FOR UPDATE`, account).Scan(&ok)
}

func (s *Service) Save(ctx context.Context, account string, data Data) (Status, error) {
	now := s.now().UTC()
	if s.keys == nil {
		return Status{}, ErrUnavailable
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Status{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockAccount(ctx, tx, account); err != nil {
		return Status{}, err
	}

	var claimedRevision, key string
	var ciphertext []byte
	err = tx.QueryRow(ctx, `
		SELECT r.id::text,r.key_version,r.encrypted_data FROM public.account_identity_rut_claim c
		JOIN public.account_identity_profile_revision r ON r.account_id=c.account_id AND r.id=c.identity_revision
		WHERE c.account_id=$1`, account).Scan(&claimedRevision, &key, &ciphertext)
	if err == nil {
		claimed, decodeErr := decryptData(s.keys, account, claimedRevision, key, ciphertext)
		if decodeErr != nil {
			return Status{}, decodeErr
		}
		if data.RUT != "" {
			candidate, normalizeErr := normalizeRUT(data.RUT)
			if normalizeErr != nil || candidate != claimed.RUT {
				return Status{}, ErrRUTLocked
			}
		}
		data.RUT = claimed.RUT
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Status{}, err
	}
	data, err = normalize(data, now)
	if err != nil {
		return Status{}, err
	}

	var revision string
	useExisting := false
	err = tx.QueryRow(ctx, `
		SELECT r.id::text,r.key_version,r.encrypted_data FROM public.account_identity_profile p
		JOIN public.account_identity_profile_revision r ON r.account_id=p.account_id AND r.id=p.current_revision
		WHERE p.account_id=$1`, account).Scan(&revision, &key, &ciphertext)
	if err == nil {
		current, decodeErr := decryptData(s.keys, account, revision, key, ciphertext)
		if decodeErr != nil {
			return Status{}, decodeErr
		}
		current.Username = data.Username
		useExisting = current == data
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return Status{}, err
	}
	if !useExisting {
		revision = uuid.NewString()
		// The public username belongs to the current account projection.
		private := data
		private.Username = ""
		raw, err := json.Marshal(private)
		if err != nil {
			return Status{}, err
		}
		version, blob, err := s.keys.Seal(revisionAAD(account, revision), raw)
		if err != nil {
			return Status{}, ErrUnavailable
		}
		maskPhone := "••••" + data.Phone[len(data.Phone)-4:]
		if _, err := tx.Exec(ctx, `INSERT INTO public.account_identity_profile_revision
			(id,account_id,key_version,encrypted_data,masked_rut,masked_phone,saved_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			revision, account, version, blob, identity.MaskRUT(data.RUT), maskPhone, now); err != nil {
			return Status{}, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.account_identity_profile(account_id,username,current_revision) VALUES ($1,$2,$3)
		ON CONFLICT(account_id) DO UPDATE SET username=EXCLUDED.username,current_revision=EXCLUDED.current_revision`, account, data.Username, revision); err != nil {
		return Status{}, mapWriteError(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM public.account_identity_profile_draft WHERE account_id=$1`, account); err != nil {
		return Status{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Status{}, err
	}
	return s.Status(ctx, account)
}

func (s *Service) SaveDraft(ctx context.Context, account string, data Data) error {
	if s.keys == nil {
		return ErrUnavailable
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockAccount(ctx, tx, account); err != nil {
		return err
	}
	var complete bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.account_identity_profile WHERE account_id=$1)`, account).Scan(&complete); err != nil {
		return err
	}
	if complete {
		return ErrConflict
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	version, blob, err := s.keys.Seal(revisionAAD(account, "draft"), raw)
	if err != nil {
		return ErrUnavailable
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.account_identity_profile_draft(account_id,key_version,encrypted_data,updated_at)
		VALUES($1,$2,$3,$4) ON CONFLICT(account_id) DO UPDATE SET key_version=EXCLUDED.key_version,
		encrypted_data=EXCLUDED.encrypted_data,updated_at=EXCLUDED.updated_at`, account, version, blob, s.now().UTC()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Service) ReadDraft(ctx context.Context, account string) (Data, error) {
	var key string
	var encrypted []byte
	err := s.pool.QueryRow(ctx, `SELECT d.key_version,d.encrypted_data FROM public.account_identity_profile_draft d
		JOIN public.usuario u ON u.id=d.account_id AND u.estado='activo'
		WHERE d.account_id=$1 AND NOT EXISTS(SELECT 1 FROM public.account_identity_profile p WHERE p.account_id=d.account_id)`, account).Scan(&key, &encrypted)
	if errors.Is(err, pgx.ErrNoRows) {
		return Data{}, ErrNotReady
	}
	if err != nil {
		return Data{}, err
	}
	return decryptData(s.keys, account, "draft", key, encrypted)
}

func (s *Service) SaveUsername(ctx context.Context, account, username string) (Status, error) {
	username = strings.TrimSpace(username)
	if !usernamePattern.MatchString(username) {
		return Status{}, &FieldError{Fields: map[string]string{"username": "invalid_username"}}
	}
	tag, err := s.pool.Exec(ctx, `UPDATE public.account_identity_profile p SET username=$2
		FROM public.usuario u WHERE p.account_id=$1 AND u.id=p.account_id AND u.estado='activo'`, account, username)
	if err != nil {
		return Status{}, mapWriteError(err)
	}
	if tag.RowsAffected() != 1 {
		return Status{}, ErrNotReady
	}
	return s.Status(ctx, account)
}

func (s *Service) OwnSensitiveProfile(ctx context.Context, account string) (map[string]string, error) {
	var email string
	var revision, key *string
	var ciphertext []byte
	var reader RowReader = s.pool
	if transaction, ok := profilepin.SensitiveQueryerFromContext(ctx); ok {
		reader = transaction
	}
	err := reader.QueryRow(ctx, `
		SELECT u.correo_original,r.id::text,r.key_version,r.encrypted_data FROM public.usuario u
		LEFT JOIN public.account_identity_profile p ON p.account_id=u.id
		LEFT JOIN public.account_identity_profile_revision r ON r.account_id=u.id AND r.id=p.current_revision
		WHERE u.id=$1 AND u.estado='activo'`, account).Scan(&email, &revision, &key, &ciphertext)
	if err != nil {
		return nil, err
	}
	out := map[string]string{"email": email}
	if revision != nil {
		data, err := decryptData(s.keys, account, *revision, *key, ciphertext)
		if err != nil {
			return nil, err
		}
		out["run"], out["phone"], out["legal_name"], out["date_of_birth"] = data.RUT, data.Phone, data.LegalName(), data.DateOfBirth
	}
	return out, nil
}

func pgUniqueConflict(err error) bool {
	var dbErr *pgconn.PgError
	return errors.As(err, &dbErr) && dbErr.Code == "23505"
}

func claimVerifiedRUT(ctx context.Context, tx pgx.Tx, keys *verification.RutHMACKeyring, account, revision, rut string, now time.Time) error {
	configured, err := keys.Fingerprints(rut)
	if err != nil {
		return ErrUnavailable
	}
	var uncovered bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.account_identity_rut_fingerprint
		WHERE split_part(fingerprint,':',1) <> ALL($1::text[]))`, keys.Versions()).Scan(&uncovered); err != nil {
		return err
	}
	if uncovered {
		return fmt.Errorf("%w: identity key rotation is incomplete", ErrUnavailable)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO public.account_identity_rut_claim(account_id,identity_revision,claimed_at)
		VALUES($1,$2,$3) ON CONFLICT(account_id) DO NOTHING`, account, revision, now); err != nil {
		return err
	}
	for _, fingerprint := range configured.Candidates {
		_, err := tx.Exec(ctx, `INSERT INTO public.account_identity_rut_fingerprint(fingerprint,account_id)
			VALUES($1,$2) ON CONFLICT(account_id,fingerprint) DO NOTHING`, fingerprint, account)
		if pgUniqueConflict(err) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
	}
	return nil
}
