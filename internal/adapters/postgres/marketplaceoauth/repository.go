package marketplaceoauth

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/marketplaceoauth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) SaveState(ctx context.Context, hash, hostID string, verifier []byte, expires time.Time) error {
	decoded, err := decodeHash(hash)
	if err != nil {
		return err
	}
	if _, err = r.pool.Exec(ctx, `DELETE FROM public.mercadopago_oauth_state WHERE expira_en < now() OR consumida_en < now()-interval '1 day'`); err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `INSERT INTO public.mercadopago_oauth_state(state_hash,anfitrion_id,encrypted_code_verifier,expira_en) VALUES($1,$2::uuid,$3,$4)`, decoded, hostID, verifier, expires)
	return err
}
func (r *Repository) ConsumeState(ctx context.Context, hash string, now time.Time) (marketplaceoauth.State, error) {
	var state marketplaceoauth.State
	decoded, err := decodeHash(hash)
	if err != nil {
		return marketplaceoauth.State{}, marketplaceoauth.ErrNotFound
	}
	err = r.pool.QueryRow(ctx, `UPDATE public.mercadopago_oauth_state SET consumida_en=$2 WHERE state_hash=$1 AND consumida_en IS NULL AND expira_en>$2 RETURNING anfitrion_id::text,encrypted_code_verifier`, decoded, now).Scan(&state.HostID, &state.EncryptedVerifier)
	if errors.Is(err, pgx.ErrNoRows) {
		return marketplaceoauth.State{}, marketplaceoauth.ErrNotFound
	}
	return state, err
}
func (r *Repository) SaveConnection(ctx context.Context, c marketplaceoauth.Connection) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO public.mercadopago_seller_connection(anfitrion_id,seller_id,encrypted_access_token,encrypted_refresh_token,token_expires_at,scopes,live_mode,connection_state)
VALUES($1::uuid,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT(anfitrion_id) DO UPDATE SET seller_id=excluded.seller_id,encrypted_access_token=excluded.encrypted_access_token,encrypted_refresh_token=excluded.encrypted_refresh_token,token_expires_at=excluded.token_expires_at,scopes=excluded.scopes,live_mode=excluded.live_mode,connection_state=excluded.connection_state,connected_at=now(),updated_at=now()`, c.HostID, c.SellerID, c.AccessToken, c.RefreshToken, c.ExpiresAt, c.Scopes, c.LiveMode, stateFor(c))
	return err
}
func stateFor(c marketplaceoauth.Connection) string {
	if c.NeedsReauthorization {
		return "reauthorization_required"
	}
	return "active"
}
func (r *Repository) GetConnection(ctx context.Context, hostID string) (marketplaceoauth.Connection, error) {
	return scanConnection(r.pool.QueryRow(ctx, `SELECT anfitrion_id::text,seller_id,encrypted_access_token,encrypted_refresh_token,token_expires_at,scopes,live_mode,connection_state FROM public.mercadopago_seller_connection WHERE anfitrion_id=$1::uuid`, hostID))
}

func (r *Repository) RefreshConnection(ctx context.Context, hostID string, refresh func(marketplaceoauth.Connection) (marketplaceoauth.Connection, error)) (marketplaceoauth.Connection, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return marketplaceoauth.Connection{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	current, err := scanConnection(tx.QueryRow(ctx, `SELECT anfitrion_id::text,seller_id,encrypted_access_token,encrypted_refresh_token,token_expires_at,scopes,live_mode,connection_state FROM public.mercadopago_seller_connection WHERE anfitrion_id=$1::uuid FOR UPDATE`, hostID))
	if err != nil {
		return marketplaceoauth.Connection{}, err
	}
	updated, refreshErr := refresh(current)
	_, writeErr := tx.Exec(ctx, `UPDATE public.mercadopago_seller_connection SET seller_id=$2,encrypted_access_token=$3,encrypted_refresh_token=$4,token_expires_at=$5,scopes=$6,live_mode=$7,connection_state=$8,updated_at=now() WHERE anfitrion_id=$1::uuid`, hostID, updated.SellerID, updated.AccessToken, updated.RefreshToken, updated.ExpiresAt, updated.Scopes, updated.LiveMode, stateFor(updated))
	if writeErr != nil {
		return marketplaceoauth.Connection{}, writeErr
	}
	if err = tx.Commit(ctx); err != nil {
		return marketplaceoauth.Connection{}, err
	}
	return updated, refreshErr
}

type rowScanner interface{ Scan(...any) error }

func scanConnection(row rowScanner) (marketplaceoauth.Connection, error) {
	var c marketplaceoauth.Connection
	var state string
	err := row.Scan(&c.HostID, &c.SellerID, &c.AccessToken, &c.RefreshToken, &c.ExpiresAt, &c.Scopes, &c.LiveMode, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return marketplaceoauth.Connection{}, marketplaceoauth.ErrNotFound
	}
	if err != nil {
		return marketplaceoauth.Connection{}, err
	}
	c.NeedsReauthorization = state == "reauthorization_required"
	return c, nil
}
func decodeHash(v string) ([]byte, error) {
	b, err := hex.DecodeString(v)
	if err != nil || len(b) != 32 {
		return nil, marketplaceoauth.ErrNotFound
	}
	return b, nil
}

var _ marketplaceoauth.Repository = (*Repository)(nil)
