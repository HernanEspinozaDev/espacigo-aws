package adminglobal

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type AccountResource struct {
	ID                string            `json:"id"`
	Kind              string            `json:"kind"`
	DataOrigin        string            `json:"data_origin,omitempty"`
	TestMode          *bool             `json:"test_mode,omitempty"`
	RoleInRecord      string            `json:"role_in_record,omitempty"`
	AmountCLP         *int64            `json:"amount_clp,omitempty"`
	TotalChargedCLP   *int64            `json:"total_charged_clp,omitempty"`
	Currency          string            `json:"currency,omitempty"`
	SpaceID           string            `json:"space_id,omitempty"`
	SpaceTitle        string            `json:"space_title,omitempty"`
	ReservationID     string            `json:"reservation_id,omitempty"`
	State             string            `json:"state"`
	Participant       string            `json:"participant_role,omitempty"`
	ProviderPaymentID string            `json:"provider_payment_id,omitempty"`
	ProviderStatus    string            `json:"provider_status,omitempty"`
	DocumentStatus    string            `json:"document_status,omitempty"`
	RelatedIDs        map[string]string `json:"related_ids,omitempty"`
	OccurredAt        time.Time         `json:"occurred_at"`
}

type AccountResourcePage struct {
	Items         []AccountResource `json:"items"`
	NextCursor    string            `json:"next_cursor"`
	MatchingTotal int64             `json:"matching_total"`
	GeneratedAt   time.Time         `json:"generated_at"`
}

type accountResourceCursor struct {
	Version int       `json:"v"`
	Filter  string    `json:"f"`
	At      time.Time `json:"a"`
	ID      string    `json:"i"`
}

func (s *Service) ListAccountResources(ctx context.Context, actor, accountID, correlation, kind, encodedCursor string, pageSize int) (AccountResourcePage, error) {
	if !canonicalUUID.MatchString(actor) || !canonicalUUID.MatchString(accountID) || !canonicalUUID.MatchString(correlation) || len(encodedCursor) > 2048 || pageSize < 1 || pageSize > 50 {
		return AccountResourcePage{}, ErrInvalid
	}
	query, valid := accountResourceQuery(kind)
	if !valid {
		return AccountResourcePage{}, ErrInvalid
	}
	filterBytes, _ := json.Marshal(struct{ Account, Kind string }{accountID, kind})
	filterHashBytes := sha256.Sum256(filterBytes)
	filterHash := hex.EncodeToString(filterHashBytes[:])
	var cursor accountResourceCursor
	if encodedCursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(encodedCursor)
		if err != nil || len(raw) > 1024 {
			return AccountResourcePage{}, ErrInvalid
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&cursor) != nil || cursor.Version != 1 || cursor.Filter != filterHash || cursor.At.IsZero() || !canonicalUUID.MatchString(cursor.ID) {
			return AccountResourcePage{}, ErrInvalid
		}
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return AccountResourcePage{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = requireGlobalActor(ctx, tx, actor); err != nil {
		return AccountResourcePage{}, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.usuario WHERE id=$1)`, accountID).Scan(&exists); err != nil {
		return AccountResourcePage{}, err
	}
	if !exists {
		return AccountResourcePage{}, ErrNotFound
	}
	var total int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM (`+query+`) resources`, accountID).Scan(&total); err != nil {
		return AccountResourcePage{}, err
	}
	args := []any{accountID}
	pageQuery := `SELECT id,kind,space_id,reservation_id,state,participant,occurred_at,data_origin,test_mode,role_in_record,amount_clp,total_charged_clp,currency,space_title,provider_payment_id,provider_status,document_status,related_ids FROM (` + query + `) resources WHERE ($2::timestamptz IS NULL OR (occurred_at,id)<($2,$3::text)) ORDER BY occurred_at DESC,id DESC LIMIT $4`
	var lastAt any
	var lastID any
	if cursor.Version == 1 {
		lastAt = cursor.At.UTC()
		lastID = cursor.ID
	}
	args = append(args, lastAt, lastID, pageSize+1)
	rows, err := tx.Query(ctx, pageQuery, args...)
	if err != nil {
		return AccountResourcePage{}, err
	}
	items := make([]AccountResource, 0, pageSize+1)
	for rows.Next() {
		var item AccountResource
		var relatedRaw []byte
		if err = rows.Scan(
			&item.ID,
			&item.Kind,
			&item.SpaceID,
			&item.ReservationID,
			&item.State,
			&item.Participant,
			&item.OccurredAt,
			&item.DataOrigin,
			&item.TestMode,
			&item.RoleInRecord,
			&item.AmountCLP,
			&item.TotalChargedCLP,
			&item.Currency,
			&item.SpaceTitle,
			&item.ProviderPaymentID,
			&item.ProviderStatus,
			&item.DocumentStatus,
			&relatedRaw,
		); err != nil {
			rows.Close()
			return AccountResourcePage{}, err
		}
		if len(relatedRaw) > 0 {
			var related map[string]string
			if err = json.Unmarshal(relatedRaw, &related); err != nil {
				rows.Close()
				return AccountResourcePage{}, err
			}
			if len(related) > 0 {
				item.RelatedIDs = related
			}
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return AccountResourcePage{}, err
	}
	rows.Close()
	page := AccountResourcePage{Items: items, MatchingTotal: total, GeneratedAt: s.now().UTC()}
	if len(items) > pageSize {
		page.Items = items[:pageSize]
		last := page.Items[len(page.Items)-1]
		encoded, marshalErr := json.Marshal(accountResourceCursor{Version: 1, Filter: filterHash, At: last.OccurredAt.UTC(), ID: last.ID})
		if marshalErr != nil {
			return AccountResourcePage{}, marshalErr
		}
		page.NextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	if err = s.auditWithTarget(ctx, tx, actor, accountID, "admin.global.accounts.resources.list", "exito", "consulta_recurso", correlation, page.GeneratedAt, accountID); err != nil {
		return AccountResourcePage{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return AccountResourcePage{}, err
	}
	return page, nil
}

func (s *Service) GetAccountResource(ctx context.Context, actor, accountID, resourceID, correlation, kind string) (map[string]any, error) {
	if !canonicalUUID.MatchString(actor) || !canonicalUUID.MatchString(accountID) || !canonicalUUID.MatchString(resourceID) || !canonicalUUID.MatchString(correlation) {
		return nil, ErrInvalid
	}
	query, valid := accountResourceDetailQuery(kind)
	if !valid {
		return nil, ErrInvalid
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = requireGlobalActor(ctx, tx, actor); err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if err = tx.QueryRow(ctx, query, resourceID, accountID).Scan(&raw); errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	} else if err != nil {
		return nil, err
	}
	var detail map[string]any
	if err = json.Unmarshal(raw, &detail); err != nil {
		return nil, err
	}
	if err = s.auditWithTarget(ctx, tx, actor, resourceID, "admin.global.accounts.resources.read", "exito", "consulta_recurso", correlation, s.now().UTC(), accountID); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return detail, nil
}

func accountResourceQuery(kind string) (string, bool) {
	switch kind {
	case "reservations":
		return `SELECT id::text AS id,'reservation'::text AS kind,espacio_id::text AS space_id,id::text AS reservation_id,estado AS state,CASE WHEN anfitrion_id=$1 THEN 'arrendador' ELSE 'arrendatario' END AS participant,creada_en AS occurred_at,'trial'::text AS data_origin,NULL::boolean AS test_mode,''::text AS role_in_record,NULL::bigint AS amount_clp,NULL::bigint AS total_charged_clp,''::text AS currency,''::text AS space_title,''::text AS provider_payment_id,''::text AS provider_status,''::text AS document_status,NULL::jsonb AS related_ids FROM public.reserva_ensayo_local WHERE anfitrion_id=$1 OR arrendatario_id=$1`, true
	case "incidents":
		return `SELECT c.id::text AS id,'damage_claim'::text AS kind,r.espacio_id::text AS space_id,r.id::text AS reservation_id,CASE WHEN EXISTS(SELECT 1 FROM public.reclamo_dano_resolucion_ensayo_local x WHERE x.reclamo_id=c.id) THEN 'resuelta' ELSE c.estado END AS state,CASE WHEN c.anfitrion_id=$1 THEN 'arrendador' ELSE 'arrendatario' END AS participant,c.abierto_en AS occurred_at,'trial'::text AS data_origin,NULL::boolean AS test_mode,''::text AS role_in_record,NULL::bigint AS amount_clp,NULL::bigint AS total_charged_clp,''::text AS currency,''::text AS space_title,''::text AS provider_payment_id,''::text AS provider_status,''::text AS document_status,NULL::jsonb AS related_ids FROM public.reclamo_dano_ensayo_local c JOIN public.reserva_ensayo_local r ON r.id=c.reserva_id WHERE c.anfitrion_id=$1 OR c.arrendatario_id=$1 UNION ALL SELECT d.id::text AS id,'dispute'::text AS kind,r.espacio_id::text AS space_id,r.id::text AS reservation_id,d.estado AS state,CASE WHEN d.anfitrion_id=$1 THEN 'arrendador' ELSE 'arrendatario' END AS participant,d.abierta_en AS occurred_at,'trial'::text AS data_origin,NULL::boolean AS test_mode,''::text AS role_in_record,NULL::bigint AS amount_clp,NULL::bigint AS total_charged_clp,''::text AS currency,''::text AS space_title,''::text AS provider_payment_id,''::text AS provider_status,''::text AS document_status,NULL::jsonb AS related_ids FROM public.disputa_ensayo_local d JOIN public.reserva_ensayo_local r ON r.id=d.reserva_id WHERE d.anfitrion_id=$1 OR d.arrendatario_id=$1`, true
	case "commercial_reservations":
		return `SELECT r.id::text AS id,'commercial_reservation'::text AS kind,r.espacio_id::text AS space_id,r.id::text AS reservation_id,r.estado AS state,CASE WHEN r.anfitrion_id::text=$1 THEN 'anfitrion' ELSE 'arrendatario' END AS participant,r.creada_en AS occurred_at,'commercial'::text AS data_origin,cp.test_mode AS test_mode,CASE WHEN r.anfitrion_id::text=$1 THEN 'anfitrion' ELSE 'arrendatario' END AS role_in_record,r.subtotal_clp AS amount_clp,cp.total_charged_clp AS total_charged_clp,btrim(r.moneda::text) AS currency,e.titulo AS space_title,''::text AS provider_payment_id,''::text AS provider_status,''::text AS document_status,jsonb_strip_nulls(jsonb_build_object('checkout_operation_id',r.operacion_checkout_id::text,'contract_request_id',cp.solicitud_comercial_id::text)) AS related_ids FROM public.reserva_arriendo_comercial r JOIN public.espacio e ON e.id=r.espacio_id LEFT JOIN public.checkout_preferencia_comercial cp ON cp.id=r.operacion_checkout_id WHERE r.arrendatario_id::text=$1 OR r.anfitrion_id::text=$1`, true
	case "commercial_contracts":
		return `SELECT cs.id::text AS id,'commercial_contract'::text AS kind,cs.espacio_id::text AS space_id,''::text AS reservation_id,cs.estado AS state,CASE WHEN cs.propietario_id::text=$1 THEN 'arrendador' ELSE 'arrendatario' END AS participant,cs.creada_en AS occurred_at,'commercial'::text AS data_origin,NULL::boolean AS test_mode,CASE WHEN cs.propietario_id::text=$1 THEN 'arrendador' ELSE 'arrendatario' END AS role_in_record,cp.subtotal_clp AS amount_clp,NULL::bigint AS total_charged_clp,cp.moneda AS currency,e.titulo AS space_title,''::text AS provider_payment_id,''::text AS provider_status,COALESCE(cd.estado,'') AS document_status,jsonb_strip_nulls(jsonb_build_object('proposal_id',cs.propuesta_id::text,'quote_id',cs.cotizacion_id::text,'signing_mode',cs.modalidad_firma)) AS related_ids FROM public.contrato_solicitud_comercial cs JOIN public.contrato_propuesta_comercial cp ON cp.id=cs.propuesta_id JOIN public.espacio e ON e.id=cs.espacio_id LEFT JOIN public.contrato_documento_comercial cd ON cd.solicitud_id=cs.id WHERE cs.arrendatario_id::text=$1 OR cs.propietario_id::text=$1`, true
	case "commercial_payments":
		return `SELECT cp.id::text AS id,'commercial_payment'::text AS kind,cp.espacio_id::text AS space_id,''::text AS reservation_id,cp.estado AS state,CASE WHEN cp.host_id::text=$1 THEN 'anfitrion' ELSE 'arrendatario' END AS participant,cp.creada_en AS occurred_at,'commercial'::text AS data_origin,cp.test_mode AS test_mode,CASE WHEN cp.host_id::text=$1 THEN 'anfitrion' ELSE 'arrendatario' END AS role_in_record,cp.base_clp AS amount_clp,cp.total_charged_clp AS total_charged_clp,'CLP'::text AS currency,e.titulo AS space_title,COALESCE(cp.provider_payment_id,'') AS provider_payment_id,COALESCE(cp.provider_payment_status,'') AS provider_status,''::text AS document_status,jsonb_strip_nulls(jsonb_build_object('quote_id',cp.cotizacion_id::text,'checkout_method',cp.checkout_method,'contract_request_id',cp.solicitud_comercial_id::text)) AS related_ids FROM public.checkout_preferencia_comercial cp JOIN public.espacio e ON e.id=cp.espacio_id WHERE cp.arrendatario_id::text=$1 OR cp.host_id::text=$1`, true
	default:
		return "", false
	}
}

func accountResourceDetailQuery(kind string) (string, bool) {
	switch kind {
	case "reservations":
		return `SELECT jsonb_build_object('id',id,'data_origin','trial','space_id',espacio_id,'host_id',anfitrion_id,'renter_id',arrendatario_id,'state',estado,'start_at',inicio,'end_at',termino,'subtotal_clp',subtotal_clp::text,'currency',moneda,'created_at',creada_en,'updated_at',actualizada_en) FROM public.reserva_ensayo_local WHERE id=$1 AND (anfitrion_id=$2 OR arrendatario_id=$2)`, true
	case "incidents":
		return `SELECT detail FROM (SELECT jsonb_build_object('id',c.id,'kind','damage_claim','data_origin','trial','reservation_id',c.reserva_id,'space_id',r.espacio_id,'state',CASE WHEN x.reclamo_id IS NULL THEN c.estado ELSE 'resuelta' END,'description',c.descripcion,'occurred_at',c.abierto_en,'host_id',c.anfitrion_id,'renter_id',c.arrendatario_id) AS detail FROM public.reclamo_dano_ensayo_local c JOIN public.reserva_ensayo_local r ON r.id=c.reserva_id LEFT JOIN public.reclamo_dano_resolucion_ensayo_local x ON x.reclamo_id=c.id WHERE c.id=$1 AND (c.anfitrion_id=$2 OR c.arrendatario_id=$2) UNION ALL SELECT jsonb_build_object('id',d.id,'kind','dispute','data_origin','trial','reservation_id',d.reserva_id,'space_id',r.espacio_id,'state',d.estado,'reason_code',d.motivo_codigo,'occurred_at',d.abierta_en,'host_id',d.anfitrion_id,'renter_id',d.arrendatario_id) FROM public.disputa_ensayo_local d JOIN public.reserva_ensayo_local r ON r.id=d.reserva_id WHERE d.id=$1 AND (d.anfitrion_id=$2 OR d.arrendatario_id=$2)) details(detail) LIMIT 1`, true
	case "commercial_reservations":
		return `SELECT jsonb_strip_nulls(jsonb_build_object('id',r.id,'kind','commercial_reservation','data_origin','commercial','space_id',r.espacio_id,'space_title',e.titulo,'reservation_id',r.id,'host_id',r.anfitrion_id,'renter_id',r.arrendatario_id,'role_in_record',CASE WHEN r.anfitrion_id::text=$2 THEN 'anfitrion' ELSE 'arrendatario' END,'state',r.estado,'test_mode',cp.test_mode,'amount_clp',r.subtotal_clp,'total_charged_clp',cp.total_charged_clp,'currency',btrim(r.moneda::text),'modality',r.modalidad,'unit_price_clp',r.precio_unitario_clp,'units',r.unidades,'start_at',r.inicio,'end_at',r.termino,'payment_due_at',r.pago_vence_en,'created_at',r.creada_en,'updated_at',r.actualizada_en,'related_ids',jsonb_strip_nulls(jsonb_build_object('checkout_operation_id',r.operacion_checkout_id::text,'contract_request_id',cp.solicitud_comercial_id::text,'quote_id',r.cotizacion_id::text)))) FROM public.reserva_arriendo_comercial r JOIN public.espacio e ON e.id=r.espacio_id LEFT JOIN public.checkout_preferencia_comercial cp ON cp.id=r.operacion_checkout_id WHERE r.id=$1 AND (r.arrendatario_id::text=$2 OR r.anfitrion_id::text=$2)`, true
	case "commercial_contracts":
		return `SELECT jsonb_strip_nulls(jsonb_build_object('id',cs.id,'kind','commercial_contract','data_origin','commercial','space_id',cs.espacio_id,'space_title',e.titulo,'owner_id',cs.propietario_id,'renter_id',cs.arrendatario_id,'role_in_record',CASE WHEN cs.propietario_id::text=$2 THEN 'arrendador' ELSE 'arrendatario' END,'state',cs.estado,'amount_clp',cp.subtotal_clp,'currency',cp.moneda,'document_status',cd.estado,'proposal_version',cs.propuesta_version,'signing_mode',cs.modalidad_firma,'expires_at',cs.vence_en,'payment_due_at',cs.pago_vence_en,'created_at',cs.creada_en,'updated_at',cs.actualizada_en,'related_ids',jsonb_strip_nulls(jsonb_build_object('proposal_id',cs.propuesta_id::text,'quote_id',cs.cotizacion_id::text,'signing_mode',cs.modalidad_firma)))) FROM public.contrato_solicitud_comercial cs JOIN public.contrato_propuesta_comercial cp ON cp.id=cs.propuesta_id JOIN public.espacio e ON e.id=cs.espacio_id LEFT JOIN public.contrato_documento_comercial cd ON cd.solicitud_id=cs.id WHERE cs.id=$1 AND (cs.arrendatario_id::text=$2 OR cs.propietario_id::text=$2)`, true
	case "commercial_payments":
		return `SELECT jsonb_strip_nulls(jsonb_build_object('id',cp.id,'kind','commercial_payment','data_origin','commercial','space_id',cp.espacio_id,'space_title',e.titulo,'host_id',cp.host_id,'renter_id',cp.arrendatario_id,'role_in_record',CASE WHEN cp.host_id::text=$2 THEN 'anfitrion' ELSE 'arrendatario' END,'state',cp.estado,'test_mode',cp.test_mode,'amount_clp',cp.base_clp,'total_charged_clp',cp.total_charged_clp,'commission_clp',cp.commission_clp,'iva_clp',cp.iva_clp,'buyer_fee_clp',cp.buyer_fee_clp,'marketplace_fee_clp',cp.marketplace_fee_clp,'currency','CLP','checkout_method',cp.checkout_method,'provider_payment_id',cp.provider_payment_id,'provider_status',cp.provider_payment_status,'payment_verified_at',cp.payment_verified_at,'expires_at',cp.vence_en,'created_at',cp.creada_en,'updated_at',cp.actualizada_en,'related_ids',jsonb_strip_nulls(jsonb_build_object('quote_id',cp.cotizacion_id::text,'checkout_method',cp.checkout_method,'contract_request_id',cp.solicitud_comercial_id::text)))) FROM public.checkout_preferencia_comercial cp JOIN public.espacio e ON e.id=cp.espacio_id WHERE cp.id=$1 AND (cp.arrendatario_id::text=$2 OR cp.host_id::text=$2)`, true
	default:
		return "", false
	}
}
