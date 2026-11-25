package profilekyc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/text/unicode/norm"
)

type Provider interface {
	Start(context.Context, string, Data, string) (Session, error)
	Decision(context.Context, string) (Decision, error)
}

type Session struct {
	ID  string
	URL string
}

type Decision struct {
	SessionID       string
	AttemptID       string
	State           string
	FirstName       string
	LastName        string
	RUT             string
	DateOfBirth     string
	DocumentExpires string
	ChecksPassed    bool
}

type Notice struct {
	ID         string
	SessionID  string
	AttemptID  string
	OccurredAt time.Time
}

type Verifier struct {
	*Service
	provider Provider
}

func NewVerifier(service *Service, provider Provider) *Verifier {
	return &Verifier{Service: service, provider: provider}
}

func (s *Verifier) Start(ctx context.Context, account string, returnPath ...string) (Session, error) {
	if s.provider == nil || s.keys == nil {
		return Session{}, ErrUnavailable
	}
	attempt, data, err := s.prepare(ctx, account)
	if err != nil {
		return Session{}, err
	}
	var destination string
	if len(returnPath) > 0 {
		destination = returnPath[0]
	}
	session, err := s.provider.Start(ctx, attempt, data, destination)
	if err != nil {
		return Session{}, ErrUnavailable
	}
	if _, err := uuid.Parse(session.ID); err != nil || session.URL == "" {
		return Session{}, ErrUnavailable
	}
	tag, err := s.pool.Exec(ctx, `UPDATE public.account_identity_verification a SET provider_session=$1,updated_at=$4
		FROM public.account_identity_profile p WHERE a.id=$2 AND a.account_id=$3 AND p.account_id=a.account_id
		  AND p.current_revision=a.profile_revision AND a.state='in_progress'
		  AND (a.provider_session IS NULL OR a.provider_session=$1)`, session.ID, attempt, account, s.now().UTC())
	if err != nil || tag.RowsAffected() != 1 {
		return Session{}, ErrConflict
	}
	return session, nil
}

func (s *Verifier) prepare(ctx context.Context, account string) (string, Data, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", Data{}, err
	}
	defer tx.Rollback(ctx)
	if err := lockAccount(ctx, tx, account); err != nil {
		return "", Data{}, err
	}
	var revision, key string
	var encrypted []byte
	err = tx.QueryRow(ctx, `SELECT r.id::text,r.key_version,r.encrypted_data
		FROM public.account_identity_profile p JOIN public.account_identity_profile_revision r
		ON r.account_id=p.account_id AND r.id=p.current_revision WHERE p.account_id=$1`, account).Scan(&revision, &key, &encrypted)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", Data{}, ErrNotReady
	}
	if err != nil {
		return "", Data{}, err
	}
	data, err := decryptData(s.keys, account, revision, key, encrypted)
	if err != nil {
		return "", Data{}, err
	}
	var attempt, state string
	var until *time.Time
	now := s.now().UTC()
	err = tx.QueryRow(ctx, `SELECT id::text,state,verified_until FROM public.account_identity_verification
		WHERE account_id=$1 AND profile_revision=$2 ORDER BY started_at DESC,id DESC LIMIT 1`, account, revision).Scan(&attempt, &state, &until)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", Data{}, err
	}
	switch {
	case state == "in_review", state == "verified" && until != nil && until.After(now):
		return "", Data{}, ErrConflict
	case state != "in_progress":
		attempt = uuid.NewString()
		if _, err := tx.Exec(ctx, `INSERT INTO public.account_identity_verification
			(id,account_id,profile_revision,state,started_at,updated_at) VALUES ($1,$2,$3,'in_progress',$4,$4)`,
			attempt, account, revision, now); err != nil {
			return "", Data{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", Data{}, err
	}
	return attempt, data, nil
}

func (s *Verifier) Refresh(ctx context.Context, account string) (Status, error) {
	if s.provider == nil || s.rutKeys == nil {
		return Status{}, ErrUnavailable
	}
	var attempt, session string
	now := s.now().UTC()
	err := s.pool.QueryRow(ctx, `UPDATE public.account_identity_verification a SET last_refresh_at=$2
		WHERE a.id=(SELECT v.id FROM public.account_identity_verification v
		    JOIN public.account_identity_profile p ON p.account_id=v.account_id AND p.current_revision=v.profile_revision
		    WHERE v.account_id=$1 ORDER BY v.started_at DESC,v.id DESC LIMIT 1)
		AND a.state IN ('in_progress','in_review') AND a.provider_session IS NOT NULL
		AND (a.last_refresh_at IS NULL OR a.last_refresh_at<=$2::timestamptz-interval '15 seconds')
		RETURNING id::text,provider_session::text`, account, now).Scan(&attempt, &session)
	if errors.Is(err, pgx.ErrNoRows) {
		return s.Status(ctx, account)
	}
	if err != nil {
		return Status{}, err
	}
	decision, err := s.provider.Decision(ctx, session)
	if err != nil || decision.SessionID != session || decision.AttemptID != attempt {
		return Status{}, ErrUnavailable
	}
	if err := s.apply(ctx, account, Notice{ID: uuid.NewString(), SessionID: session, AttemptID: attempt, OccurredAt: now}, decision); err != nil {
		return Status{}, err
	}
	return s.Status(ctx, account)
}

func (s *Verifier) ProcessNotice(ctx context.Context, n Notice) error {
	if s.provider == nil || s.rutKeys == nil {
		return ErrUnavailable
	}
	for _, id := range []string{n.ID, n.SessionID, n.AttemptID} {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed.String() != id {
			return ErrConflict
		}
	}
	if n.OccurredAt.IsZero() {
		return ErrConflict
	}
	var duplicate bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM public.account_identity_webhook_receipt WHERE event_id=$1)`, n.ID).Scan(&duplicate); err != nil || duplicate {
		return err
	}
	var account string
	if err := s.pool.QueryRow(ctx, `SELECT account_id::text FROM public.account_identity_verification WHERE id=$1`, n.AttemptID).Scan(&account); err != nil {
		return err
	}
	decision, err := s.provider.Decision(ctx, n.SessionID)
	if err != nil {
		return ErrUnavailable
	}
	if decision.SessionID != n.SessionID || decision.AttemptID != n.AttemptID {
		return ErrConflict
	}
	return s.apply(ctx, account, n, decision)
}

func sameName(a, b string) bool {
	return strings.EqualFold(norm.NFC.String(compactInput(a)), norm.NFC.String(compactInput(b)))
}

func decisionState(decision Decision, data Data, now time.Time) (string, *time.Time) {
	switch decision.State {
	case "in_progress", "in_review", "needs_correction", "expired", "revoked":
		return decision.State, nil
	case "verified":
		rut, err := normalizeRUT(decision.RUT)
		location, locationErr := time.LoadLocation("America/Santiago")
		if locationErr != nil {
			return "needs_correction", nil
		}
		documentDate, expiryErr := time.ParseInLocation(time.DateOnly, decision.DocumentExpires, location)
		expiry := documentDate.AddDate(0, 0, 1).UTC()
		if err != nil || expiryErr != nil || !expiry.After(now) || !decision.ChecksPassed ||
			!sameName(data.FirstName, decision.FirstName) || !sameName(data.LastName(), decision.LastName) ||
			data.RUT != rut || data.DateOfBirth != decision.DateOfBirth {
			return "needs_correction", nil
		}
		limit := now.AddDate(1, 0, 0)
		if expiry.Before(limit) {
			limit = expiry
		}
		return "verified", &limit
	default:
		return "", nil
	}
}

func (s *Verifier) apply(ctx context.Context, account string, n Notice, decision Decision) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockAccount(ctx, tx, account); err != nil {
		return err
	}
	var revision, currentRevision, currentAttempt, key, currentState string
	var session *string
	var previous *time.Time
	var started time.Time
	var ciphertext []byte
	err = tx.QueryRow(ctx, `SELECT a.profile_revision::text,p.current_revision::text,
		COALESCE((SELECT latest.id::text FROM public.account_identity_verification latest
		    WHERE latest.account_id=a.account_id AND latest.profile_revision=p.current_revision
		    ORDER BY latest.started_at DESC,latest.id DESC LIMIT 1),''),
		r.key_version,r.encrypted_data,
		a.state,a.provider_session::text,a.decision_at,a.started_at
		FROM public.account_identity_verification a JOIN public.account_identity_profile p ON p.account_id=a.account_id
		JOIN public.account_identity_profile_revision r ON r.account_id=a.account_id AND r.id=a.profile_revision
		WHERE a.id=$1 AND a.account_id=$2 FOR UPDATE OF a`, n.AttemptID, account).Scan(
		&revision, &currentRevision, &currentAttempt, &key, &ciphertext, &currentState, &session, &previous, &started)
	if err != nil {
		return err
	}
	if session != nil && *session != n.SessionID {
		return ErrConflict
	}
	now := s.now().UTC()
	tag, err := tx.Exec(ctx, `INSERT INTO public.account_identity_webhook_receipt(event_id,attempt_id,received_at)
		VALUES($1,$2,$3) ON CONFLICT(event_id) DO NOTHING`, n.ID, n.AttemptID, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 || revision != currentRevision || currentAttempt != n.AttemptID || previous != nil && n.OccurredAt.Before(*previous) ||
		n.OccurredAt.Before(started.Add(-5*time.Minute)) {
		return tx.Commit(ctx)
	}
	data, err := decryptData(s.keys, account, revision, key, ciphertext)
	if err != nil {
		return err
	}
	next, until := decisionState(decision, data, now)
	if next == "" || currentState == "revoked" || currentState == "in_review" && next == "in_progress" {
		return tx.Commit(ctx)
	}
	if currentState == "verified" {
		switch next {
		case "verified", "in_progress":
			return tx.Commit(ctx)
		case "in_review", "needs_correction":
			next = "revoked"
		}
	}
	var verifiedAt *time.Time
	if next == "verified" {
		// A duplicate identity is handled without losing the receipt or committing a partial claim.
		claimTx, err := tx.Begin(ctx)
		if err != nil {
			return err
		}
		err = claimVerifiedRUT(ctx, claimTx, s.rutKeys, account, revision, data.RUT, now)
		switch {
		case errors.Is(err, ErrConflict):
			if err := claimTx.Rollback(ctx); err != nil {
				return err
			}
			next, until = "needs_correction", nil
		case err != nil:
			_ = claimTx.Rollback(ctx)
			return err
		default:
			if err := claimTx.Commit(ctx); err != nil {
				return err
			}
			verifiedAt = &now
		}
	}
	tag, err = tx.Exec(ctx, `UPDATE public.account_identity_verification
		SET state=$1,provider_session=$2,updated_at=$3,decision_at=$4,verified_at=COALESCE($5,verified_at),verified_until=COALESCE($6,verified_until)
		WHERE id=$7`, next, n.SessionID, now, n.OccurredAt, verifiedAt, until, n.AttemptID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: missing verification attempt", ErrConflict)
	}
	return tx.Commit(ctx)
}
