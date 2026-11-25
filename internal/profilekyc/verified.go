package profilekyc

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/identityverification"
	"github.com/jackc/pgx/v5"
)

type RowReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

const verifiedProfileSQL = `FROM public.account_identity_profile p
	JOIN public.account_identity_profile_revision r ON r.account_id=p.account_id AND r.id=p.current_revision
	JOIN public.usuario u ON u.id=p.account_id AND u.estado='activo'
	JOIN LATERAL (
	    SELECT state,verified_at,verified_until FROM public.account_identity_verification
	    WHERE account_id=p.account_id AND profile_revision=p.current_revision
	    ORDER BY started_at DESC,id DESC LIMIT 1
	) a ON a.state='verified' AND a.verified_at<=$2 AND a.verified_until>$2
	WHERE p.account_id=$1`

func (s *Service) CurrentForAccount(ctx context.Context, account string) (identityverification.Verification, error) {
	current := identityverification.Verification{
		SubjectID: account, Kind: identityverification.KindIdentityDocument,
		State: identityverification.StateApproved, ReviewMethod: identityverification.ReviewMethodDidit,
	}
	err := s.pool.QueryRow(ctx, "SELECT a.verified_at,a.verified_until "+verifiedProfileSQL, account, s.now().UTC()).
		Scan(&current.VerifiedAt, &current.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return identityverification.Verification{}, identityverification.ErrNotFound
	}
	return current, err
}

func ReadVerified(ctx context.Context, reader RowReader, keys Keyring, account string, asOf time.Time) (Data, error) {
	var id, key string
	var encrypted []byte
	err := reader.QueryRow(ctx, "SELECT r.id::text,r.key_version,r.encrypted_data "+verifiedProfileSQL, account, asOf.UTC()).
		Scan(&id, &key, &encrypted)
	if err != nil {
		return Data{}, err
	}
	return decryptData(keys, account, id, key, encrypted)
}
