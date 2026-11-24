package rentalflowpg

import (
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

// enqueueCommercialContractDocument shares the verified-payment commit. It
// creates no object and duplicate provider receipts retain the same object UUID.
func enqueueCommercialContractDocument(ctx context.Context, tx pgx.Tx, operationID string, at time.Time) error {
	_, err := tx.Exec(ctx, `INSERT INTO public.contrato_documento_comercial(id,operacion_id,solicitud_id,propuesta_id,version,documento_sha256,estado,disponible_en,creada_en,actualizada_en) SELECT c.id,c.id,s.id,s.propuesta_id,c.contrato_version,c.contrato_sha256,'pending',$2,$2,$2 FROM public.checkout_preferencia_comercial c JOIN public.contrato_solicitud_comercial s ON s.id=c.solicitud_comercial_id JOIN public.reserva_arriendo_comercial r ON r.operacion_checkout_id=c.id WHERE c.id=$1 AND c.estado='approved' AND c.payment_verified_at IS NOT NULL AND r.estado='pagada' AND s.propuesta_version=c.contrato_version AND s.documento_sha256=c.contrato_sha256 ON CONFLICT(operacion_id) DO NOTHING`, operationID, at.UTC())
	return err
}
