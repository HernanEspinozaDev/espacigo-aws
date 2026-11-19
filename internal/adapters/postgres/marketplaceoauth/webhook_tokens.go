package marketplaceoauth

import (
	"context"

	"github.com/HernanEspinozaDev/espaciGo/internal/marketplaceoauth"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type sellerTokenService interface {
	AccessToken(context.Context, string) ([]byte, error)
}

// SellerCredentialResolver selects an active local connection by the provider
// seller ID, then delegates decryption/refresh policy to the OAuth service.
// The caller must still compare the provider-authenticated payment seller ID.
type SellerCredentialResolver struct {
	pool    *pgxpool.Pool
	service sellerTokenService
}

func NewSellerCredentialResolver(pool *pgxpool.Pool, service sellerTokenService) (*SellerCredentialResolver, error) {
	if pool == nil || service == nil {
		return nil, marketplaceoauth.ErrNotConfigured
	}
	return &SellerCredentialResolver{pool: pool, service: service}, nil
}

func (r *SellerCredentialResolver) AccessTokenForSeller(ctx context.Context, sellerID string) ([]byte, error) {
	if r == nil || r.pool == nil || r.service == nil || ctx == nil || sellerID == "" || len(sellerID) > 64 {
		return nil, marketplaceoauth.ErrNotFound
	}
	var hostID string
	err := r.pool.QueryRow(ctx, `SELECT anfitrion_id::text FROM public.mercadopago_seller_connection
WHERE seller_id=$1 AND connection_state='active'`, sellerID).Scan(&hostID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, marketplaceoauth.ErrNotFound
		}
		return nil, err
	}
	return r.service.AccessToken(ctx, hostID)
}

var _ interface {
	AccessTokenForSeller(context.Context, string) ([]byte, error)
} = (*SellerCredentialResolver)(nil)
