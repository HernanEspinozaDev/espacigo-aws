package hostsettlementspg

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/contracttemplate"
	"github.com/HernanEspinozaDev/espaciGo/internal/hostsettlement"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

const source = ` FROM public.reserva_arriendo_comercial r
JOIN public.checkout_preferencia_comercial c ON c.id=r.operacion_checkout_id AND c.host_id=r.anfitrion_id AND c.arrendatario_id=r.arrendatario_id AND c.espacio_id=r.espacio_id
JOIN public.cotizacion_arriendo_comercial q ON q.id=r.cotizacion_id
WHERE r.anfitrion_id=$1 AND r.estado='pagada' AND c.estado='approved' AND c.provider_payment_status='approved' AND c.payment_verified_at IS NOT NULL AND c.provider_payment_id IS NOT NULL`
const columns = `r.id::text,r.espacio_id::text,q.titulo_espacio,c.provider_payment_id,c.payment_verified_at,c.test_mode,r.subtotal_clp,c.base_clp,c.commission_clp,c.iva_clp,c.marketplace_fee_clp`

func scan(row pgx.Row) (hostsettlement.Item, error) {
	var item hostsettlement.Item
	var gross, base, commission, vat, fee int64
	err := row.Scan(&item.ReservationID, &item.SpaceID, &item.SpaceTitle, &item.ProviderReference, &item.VerifiedAt, &item.TestMode, &gross, &base, &commission, &vat, &fee)
	if err != nil {
		return item, err
	}
	item.Currency = "CLP"
	item.VerifiedAt = item.VerifiedAt.UTC()
	item.Amounts, item.FeePolicy, item.CommissionRatePercent, item.CommissionVATRatePercent, err = hostsettlement.RecordedAmounts(gross, base, commission, vat, fee)
	return item, err
}

type cursor struct {
	Version int       `json:"v"`
	Actor   string    `json:"actor"`
	Month   string    `json:"month"`
	Mode    string    `json:"mode"`
	At      time.Time `json:"at"`
	ID      string    `json:"id"`
}

func decodeCursor(encoded, actor string, q hostsettlement.Query) (*cursor, error) {
	if encoded == "" {
		return nil, nil
	}
	if len(encoded) > 2048 {
		return nil, hostsettlement.ErrInvalid
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, hostsettlement.ErrInvalid
	}
	var c cursor
	if json.Unmarshal(data, &c) != nil || c.Version != 1 || c.Actor != actor || c.Month != q.Month || c.Mode != q.Mode || !contracttemplate.ValidIDs(c.ID) || c.At.Before(q.StartAt) || !c.At.Before(q.EndAt) {
		return nil, hostsettlement.ErrInvalid
	}
	return &c, nil
}
func (r *Repository) List(ctx context.Context, actor string, q hostsettlement.Query) (hostsettlement.Page, error) {
	if !contracttemplate.ValidIDs(actor) {
		return hostsettlement.Page{}, hostsettlement.ErrInvalid
	}
	actor = strings.ToLower(actor)
	c, err := decodeCursor(q.Cursor, actor, q)
	if err != nil {
		return hostsettlement.Page{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return hostsettlement.Page{}, err
	}
	defer tx.Rollback(ctx)
	page := hostsettlement.Page{Summary: hostsettlement.Summary{Month: q.Month, TimeZone: "America/Santiago", Mode: q.Mode, Currency: "CLP"}, Items: []hostsettlement.Item{}}
	mode := q.Mode == "test"
	// numeric SUMs are returned as strings, retaining CLP precision even when a
	// monthly total exceeds the range of an individual PostgreSQL bigint amount.
	err = tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(r.subtotal_clp),0)::text,COALESCE(sum(c.base_clp),0)::text,COALESCE(sum(c.commission_clp),0)::text,COALESCE(sum(c.iva_clp),0)::text,COALESCE(sum(c.marketplace_fee_clp),0)::text,COALESCE(sum(c.base_clp-c.marketplace_fee_clp),0)::text`+source+` AND c.payment_verified_at >= $2 AND c.payment_verified_at < $3 AND c.test_mode=$4`, actor, q.StartAt, q.EndAt, mode).Scan(&page.Summary.PaymentCount, &page.Summary.GrossCLP, &page.Summary.BaseCLP, &page.Summary.CommissionCLP, &page.Summary.CommissionVATCLP, &page.Summary.MarketplaceFeeCLP, &page.Summary.AfterMarketplaceFeeCLP)
	if err != nil {
		return page, err
	}
	var afterAt, afterID any
	if c != nil {
		afterAt = c.At.UTC()
		afterID = c.ID
	}
	rows, err := tx.Query(ctx, `SELECT `+columns+source+` AND c.payment_verified_at >= $2 AND c.payment_verified_at < $3 AND c.test_mode=$4 AND ($5::timestamptz IS NULL OR (c.payment_verified_at,r.id)<($5::timestamptz,$6::uuid)) ORDER BY c.payment_verified_at DESC,r.id DESC LIMIT 51`, actor, q.StartAt, q.EndAt, mode, afterAt, afterID)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		item, e := scan(rows)
		if e != nil {
			rows.Close()
			return page, e
		}
		page.Items = append(page.Items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return page, err
	}
	if len(page.Items) > 50 {
		page.Items = page.Items[:50]
		last := page.Items[49]
		blob, _ := json.Marshal(cursor{Version: 1, Actor: actor, Month: q.Month, Mode: q.Mode, At: last.VerifiedAt, ID: last.ReservationID})
		next := base64.RawURLEncoding.EncodeToString(blob)
		page.NextCursor = &next
	}
	return page, tx.Commit(ctx)
}
func (r *Repository) Get(ctx context.Context, actor, id string) (hostsettlement.Item, error) {
	if !contracttemplate.ValidIDs(actor, id) {
		return hostsettlement.Item{}, hostsettlement.ErrNotFound
	}
	item, err := scan(r.pool.QueryRow(ctx, `SELECT `+columns+source+` AND r.id=$2`, actor, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return item, hostsettlement.ErrNotFound
	}
	return item, err
}
