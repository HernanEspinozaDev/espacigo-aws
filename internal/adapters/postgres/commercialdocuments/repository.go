package commercialdocumentspg

import (
	"context"
	"encoding/hex"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/commercialdocument"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProposalReader interface {
	Get(context.Context, string, string) (commercialproposal.Record, error)
}
type Repository struct {
	pool      *pgxpool.Pool
	proposals ProposalReader
}

func New(pool *pgxpool.Pool, proposals ProposalReader) *Repository {
	return &Repository{pool: pool, proposals: proposals}
}
func (r *Repository) Claim(ctx context.Context, at time.Time, lease time.Duration) (commercialdocument.Job, bool, error) {
	var job commercialdocument.Job
	if r == nil || r.pool == nil || ctx == nil || at.IsZero() || lease < time.Second || lease > 10*time.Minute {
		return job, false, commercialdocument.ErrInvalid
	}
	_, err := r.pool.Exec(ctx, `UPDATE public.contrato_documento_comercial SET estado='terminal',ultimo_error='attempt_limit_reached',lease_token=NULL,lease_vence_en=NULL,actualizada_en=$1 WHERE intentos>=8 AND (estado IN ('pending','retry') OR (estado='processing' AND lease_vence_en<=$1))`, at.UTC())
	if err != nil {
		return job, false, err
	}
	err = r.pool.QueryRow(ctx, `WITH due AS (SELECT id FROM public.contrato_documento_comercial WHERE intentos<8 AND ((estado IN ('pending','retry') AND disponible_en<=$1) OR (estado='processing' AND lease_vence_en<=$1)) ORDER BY disponible_en,creada_en,id FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE public.contrato_documento_comercial d SET estado='processing',intentos=d.intentos+1,lease_token=gen_random_uuid(),lease_vence_en=$2,actualizada_en=$1 FROM due WHERE d.id=due.id RETURNING d.id::text,d.operacion_id::text,d.solicitud_id::text,d.propuesta_id::text,d.object_id::text,encode(d.documento_sha256,'hex'),d.version,d.intentos,d.lease_token::text,d.lease_vence_en`, at.UTC(), at.UTC().Add(lease)).Scan(&job.ID, &job.OperationID, &job.RequestID, &job.ProposalID, &job.ObjectID, &job.DocumentSHA256, &job.Version, &job.Attempts, &job.LeaseToken, &job.LeaseUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return job, false, nil
	}
	return job, err == nil, err
}
func (r *Repository) Load(ctx context.Context, job commercialdocument.Job) (commercialdocument.Source, error) {
	var source commercialdocument.Source
	if r == nil || r.pool == nil || r.proposals == nil || ctx == nil {
		return source, commercialdocument.ErrInvalid
	}
	var actor string
	err := r.pool.QueryRow(ctx, `SELECT s.arrendatario_id::text,c.provider_payment_id,c.payment_verified_at,q.subtotal_clp,q.moneda FROM public.contrato_documento_comercial d JOIN public.checkout_preferencia_comercial c ON c.id=d.operacion_id JOIN public.reserva_arriendo_comercial r ON r.operacion_checkout_id=c.id JOIN public.contrato_solicitud_comercial s ON s.id=d.solicitud_id AND s.id=c.solicitud_comercial_id JOIN public.cotizacion_arriendo_comercial q ON q.id=c.cotizacion_id WHERE d.id=$1 AND d.estado='processing' AND d.lease_token=$2 AND c.estado='approved' AND r.estado='pagada' AND c.payment_verified_at IS NOT NULL AND c.contrato_version=d.version AND c.contrato_sha256=d.documento_sha256 AND s.propuesta_id=d.propuesta_id`, job.ID, job.LeaseToken).Scan(&actor, &source.ProviderPaymentID, &source.VerifiedAt, &source.AmountCLP, &source.Currency)
	if err != nil {
		return source, err
	}
	source.Proposal, err = r.proposals.Get(ctx, actor, job.ProposalID)
	if err != nil {
		return source, err
	}
	rows, err := r.pool.Query(ctx, `SELECT rol,actor_id::text,sesion_id::text,consentimiento_version,firmada_en FROM public.contrato_firma_comercial WHERE solicitud_id=$1 AND propuesta_version=$2 AND documento_sha256=decode($3,'hex') ORDER BY rol`, job.RequestID, job.Version, job.DocumentSHA256)
	if err != nil {
		return source, err
	}
	defer rows.Close()
	for rows.Next() {
		var s commercialdocument.Signature
		if err = rows.Scan(&s.Role, &s.ActorID, &s.SessionID, &s.ConsentVersion, &s.SignedAt); err != nil {
			return source, err
		}
		source.Signatures = append(source.Signatures, s)
	}
	return source, rows.Err()
}
func (r *Repository) Complete(ctx context.Context, job commercialdocument.Job, artifact commercialdocument.Artifact, at time.Time) error {
	digest, err := hex.DecodeString(artifact.SHA256)
	if err != nil || len(digest) != 32 || artifact.Size < 1 || artifact.Size > commercialdocument.MaxPDFBytes || artifact.Bucket == "" {
		return commercialdocument.ErrInvalid
	}
	var version any
	if artifact.Version != "" {
		version = artifact.Version
	}
	tag, err := r.pool.Exec(ctx, `UPDATE public.contrato_documento_comercial SET estado='ready',bucket_name=$3,object_version=$4,pdf_sha256=$5,pdf_bytes=$6,lista_en=$7,actualizada_en=$7,lease_token=NULL,lease_vence_en=NULL,ultimo_error=NULL WHERE id=$1 AND estado='processing' AND lease_token=$2 AND lease_vence_en>$7`, job.ID, job.LeaseToken, artifact.Bucket, version, digest, artifact.Size, at.UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return commercialdocument.ErrLostLease
	}
	return nil
}
func (r *Repository) Fail(ctx context.Context, job commercialdocument.Job, code string, terminal bool, next time.Time) error {
	allowed := map[string]bool{"evidence_unavailable": true, "render_failed": true, "unsupported_glyph": true, "storage_failed": true, "object_conflict": true, "attempt_limit_reached": true}
	if !allowed[code] {
		return commercialdocument.ErrInvalid
	}
	state := "retry"
	if terminal || job.Attempts >= 8 {
		state = "terminal"
	}
	tag, err := r.pool.Exec(ctx, `UPDATE public.contrato_documento_comercial SET estado=$3,ultimo_error=$4,disponible_en=$5,actualizada_en=now(),lease_token=NULL,lease_vence_en=NULL WHERE id=$1 AND estado='processing' AND lease_token=$2 AND lease_vence_en>now()`, job.ID, job.LeaseToken, state, code, next.UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return commercialdocument.ErrLostLease
	}
	return nil
}
