package commercialagreementspg

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	domain "github.com/HernanEspinozaDev/espaciGo/internal/commercialagreement"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const otpTTL = 5 * time.Minute

func computeOTPMAC(challengeID, actorID, proposalID, sha256Hex, role, code string) []byte {
	mac := hmac.New(sha256.New, []byte(challengeID+":"+actorID))
	mac.Write([]byte(proposalID + ":" + sha256Hex + ":" + role + ":" + code))
	return mac.Sum(nil)
}

func maskEmail(email string) string {
	parts := strings.SplitN(strings.TrimSpace(email), "@", 2)
	if len(parts) != 2 || parts[0] == "" {
		return "***"
	}
	local := parts[0]
	if len(local) <= 2 {
		return local[:1] + "***@" + parts[1]
	}
	return local[:2] + "***@" + parts[1]
}

func (r *Repository) CreateOTPChallenge(ctx context.Context, actor domain.Actor, in domain.OTPChallengeInput) (domain.OTPChallengeResult, string, string, error) {
	var ownerID, renterID, docSHA string
	var isB2B bool
	var representativeID *string
	err := r.db.QueryRow(ctx, `SELECT propietario_id::text, arrendatario_id::text, encode(documento_sha256,'hex'),es_b2b,representante_legal_id::text FROM public.contrato_propuesta_comercial WHERE id=$1`, in.ProposalID).Scan(&ownerID, &renterID, &docSHA, &isB2B, &representativeID)
	if err != nil {
		return domain.OTPChallengeResult{}, "", "", mapError(err)
	}
	ownerSigner := actor.ID == ownerID
	if isB2B && representativeID != nil && actor.ID == *representativeID {
		var active bool
		if err = r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM public.organizacion_miembro m WHERE m.organizacion_id=(SELECT organizacion_id FROM public.contrato_propuesta_comercial WHERE id=$1) AND m.usuario_id=$2 AND m.rol='representante_legal' AND m.estado='activo')`, in.ProposalID, actor.ID).Scan(&active); err != nil {
			return domain.OTPChallengeResult{}, "", "", err
		}
		ownerSigner = active
	}
	if (in.Role == "arrendatario" && actor.ID != renterID) || (in.Role == "arrendador" && !ownerSigner) {
		return domain.OTPChallengeResult{}, "", "", domain.ErrNotFound
	}
	if !hashMatches(docSHA, in.SHA256) {
		return domain.OTPChallengeResult{}, "", "", domain.ErrHash
	}
	at := r.now().UTC()
	if err = r.gate.RequireForBooking(ctx, ownerID, renterID, at); err != nil {
		return domain.OTPChallengeResult{}, "", "", mapError(err)
	}
	var email string
	if err = r.db.QueryRow(ctx, `SELECT correo_original FROM public.usuario WHERE id=$1 AND estado='activo'`, actor.ID).Scan(&email); err != nil {
		return domain.OTPChallengeResult{}, "", "", mapError(err)
	}
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return domain.OTPChallengeResult{}, "", "", err
	}
	code := fmt.Sprintf("%06d", n.Int64())
	challengeID := uuid.NewString()
	expiresAt := at.Add(otpTTL)
	digest := computeOTPMAC(challengeID, actor.ID, in.ProposalID, docSHA, in.Role, code)
	var agreementRef any
	if in.AgreementID != "" {
		agreementRef = in.AgreementID
	}
	_, err = r.db.Exec(ctx, `INSERT INTO public.contrato_firma_desafio_otp(id,propuesta_id,solicitud_id,actor_id,rol,documento_sha256,canal,codigo_hmac,intentos,expira_en,creada_en) VALUES($1,$2,$3,$4,$5,decode($6,'hex'),'email_otp',$7,0,$8,$9)`, challengeID, in.ProposalID, agreementRef, actor.ID, in.Role, docSHA, digest, expiresAt, at)
	if err != nil {
		return domain.OTPChallengeResult{}, "", "", mapError(err)
	}
	return domain.OTPChallengeResult{
		ChallengeID: challengeID,
		Channel:     "email_otp",
		ExpiresAt:   expiresAt,
		MaskedEmail: maskEmail(email),
	}, email, code, nil
}

func (r *Repository) verifyOTPInTx(ctx context.Context, tx pgx.Tx, actor domain.Actor, proposalID, sha256Hex, role, challengeID, code string, at time.Time) (string, any, error) {
	if challengeID == "" && code == "" {
		return "session_consent", nil, nil
	}
	var dbProposalID, dbActorID, dbRole, dbDocSHA string
	var storedMAC []byte
	var attempts int
	var expiresAt time.Time
	var verifiedAt *time.Time
	err := tx.QueryRow(ctx, `SELECT propuesta_id::text, actor_id::text, rol, encode(documento_sha256,'hex'), codigo_hmac, intentos, expira_en, verificado_en FROM public.contrato_firma_desafio_otp WHERE id=$1 FOR UPDATE`, challengeID).Scan(&dbProposalID, &dbActorID, &dbRole, &dbDocSHA, &storedMAC, &attempts, &expiresAt, &verifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, domain.ErrOTPInvalid
	}
	if err != nil {
		return "", nil, mapError(err)
	}
	if dbActorID != actor.ID || dbRole != role || dbProposalID != proposalID || !hashMatches(dbDocSHA, sha256Hex) || verifiedAt != nil {
		return "", nil, domain.ErrOTPInvalid
	}
	if !expiresAt.After(at) {
		return "", nil, domain.ErrOTPExpired
	}
	if attempts >= 5 {
		return "", nil, domain.ErrOTPInvalid
	}
	expectedMAC := computeOTPMAC(challengeID, actor.ID, proposalID, dbDocSHA, role, code)
	if subtle.ConstantTimeCompare(storedMAC, expectedMAC) != 1 {
		_, _ = r.db.Exec(ctx, `UPDATE public.contrato_firma_desafio_otp SET intentos = LEAST(intentos + 1, 5) WHERE id=$1 AND verificado_en IS NULL`, challengeID)
		return "", nil, domain.ErrOTPInvalid
	}
	if _, err = tx.Exec(ctx, `UPDATE public.contrato_firma_desafio_otp SET verificado_en=$2 WHERE id=$1`, challengeID, at); err != nil {
		return "", nil, mapError(err)
	}
	return "email_otp", challengeID, nil
}

func computeEvidenceSHA256(a domain.Agreement, actor domain.Actor, role, authMethod string, challengeID any, at time.Time) []byte {
	payload, _ := json.Marshal(map[string]any{
		"agreement_id":     a.ID,
		"proposal_id":      a.ProposalID,
		"proposal_version": a.ProposalVersion,
		"document_sha256":  a.SHA256,
		"actor_id":         actor.ID,
		"session_id":       actor.SessionID,
		"role":             role,
		"signature_type":   "FES",
		"auth_method":      authMethod,
		"otp_challenge_id": challengeID,
		"ip_masked":        actor.IPMasked,
		"user_agent_hash":  hex.EncodeToString(actor.UserAgentHash),
		"signed_at_utc":    at.UTC().Format(time.RFC3339Nano),
	})
	sum := sha256.Sum256(payload)
	return sum[:]
}
