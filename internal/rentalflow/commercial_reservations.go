package rentalflow

import (
	"context"
	"errors"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/checkoutpro"
)

var ErrReservationUnavailable = errors.New("rental flow: commercial reservations unavailable")

type CommercialReservation struct {
	ID                        string                  `json:"id"`
	QuoteID                   string                  `json:"quote_id"`
	SpaceID                   string                  `json:"space_id"`
	SpaceTitle                string                  `json:"space_title"`
	ParticipantRole           string                  `json:"participant_role"`
	Status                    string                  `json:"status"`
	CheckoutStatus            string                  `json:"checkout_status"`
	CheckoutMethod            string                  `json:"checkout_method,omitempty"`
	ProviderSettlement        *checkoutpro.Settlement `json:"provider_settlement,omitempty"`
	ProviderPaymentID         string                  `json:"provider_payment_id,omitempty"`
	CommissionCLP             int64                   `json:"commission_clp,omitempty"`
	VATCLP                    int64                   `json:"vat_clp,omitempty"`
	BuyerCommissionCLP        int64                   `json:"buyer_commission_clp,omitempty"`
	BuyerVATCLP               int64                   `json:"buyer_vat_clp,omitempty"`
	BuyerFeeCLP               int64                   `json:"buyer_fee_clp,omitempty"`
	TotalChargedCLP           int64                   `json:"total_charged_clp,omitempty"`
	CancellationPolicyVersion *string                 `json:"cancellation_policy_version,omitempty"`
	CancellationPolicyText    *string                 `json:"cancellation_policy_text,omitempty"`
	RefundStatus              *string                 `json:"refund_status,omitempty"`
	MarketplaceFeeCLP         int64                   `json:"marketplace_fee_clp,omitempty"`
	NetHostAmountCLP          int64                   `json:"net_host_amount_clp,omitempty"`
	RateUnit                  string                  `json:"rate_unit"`
	UnitPriceCLP              int64                   `json:"unit_price_clp"`
	Units                     int64                   `json:"units"`
	SubtotalCLP               int64                   `json:"subtotal_clp"`
	Currency                  string                  `json:"currency"`
	StartAt                   string                  `json:"start_at"`
	EndAt                     string                  `json:"end_at"`
	TimeZone                  string                  `json:"time_zone"`
	PaymentExpiresAt          string                  `json:"payment_expires_at"`
	CreatedAt                 string                  `json:"created_at"`
	UpdatedAt                 string                  `json:"updated_at"`
	Intervals                 []QuoteInterval         `json:"intervals,omitempty"`
}

type CommercialReservationPage struct {
	Items      []CommercialReservation `json:"items"`
	NextCursor string                  `json:"next_cursor,omitempty"`
}

type CommercialReservationReader interface {
	ListCommercialReservations(context.Context, string, int, string) (CommercialReservationPage, error)
	GetCommercialReservation(context.Context, string, string) (CommercialReservation, error)
}

type CommercialIntervalUsageUpdater interface {
	UpdateCommercialIntervalUsage(context.Context, string, string, string, string) (CommercialReservation, error)
}

type VerifiedPassResult struct {
	Reservation CommercialReservation `json:"reservation"`
	Interval    QuoteInterval         `json:"interval"`
}

type CommercialPassVerifier interface {
	VerifyAndApplyPass(context.Context, string, string, string, string) (VerifiedPassResult, error)
}

type CommercialReservationReconcilerFunc func(ctx context.Context, actor, quoteID string)

type CommercialReservationService struct {
	reader             CommercialReservationReader
	reconciler         CommercialReservationReconcilerFunc
	cancellations      CommercialCancellationRepository
	cancellationTokens CommercialSellerTokenProvider
	refundGateway      CommercialRefundGateway
	cancellationNow    func() time.Time
}

func NewCommercialReservationService(reader CommercialReservationReader) (*CommercialReservationService, error) {
	if reader == nil {
		return nil, ErrInvalid
	}
	return &CommercialReservationService{reader: reader}, nil
}

func (s *CommercialReservationService) SetReconciler(reconciler CommercialReservationReconcilerFunc) {
	if s != nil {
		s.reconciler = reconciler
	}
}

func needsCheckoutReconciliation(item CommercialReservation) bool {
	return item.QuoteID != "" && (item.Status == "pendiente_de_pago" || item.Status == "vencida_pago" || item.Status == "reconciliation_required") &&
		(item.CheckoutStatus == "pending" || item.CheckoutStatus == "expired" || item.CheckoutStatus == "reconciliation_required")
}

func (s *CommercialReservationService) List(ctx context.Context, participantID string, pageSize int, cursor string) (CommercialReservationPage, error) {
	if s == nil || ctx == nil || !validUUID(participantID) || pageSize < 1 || pageSize > 100 || len(cursor) > 4096 {
		return CommercialReservationPage{}, ErrInvalid
	}
	actor := canonicalUUID(participantID)
	page, err := s.reader.ListCommercialReservations(ctx, actor, pageSize, cursor)
	if err != nil {
		if errors.Is(err, ErrInvalid) {
			return CommercialReservationPage{}, ErrInvalid
		}
		if errors.Is(err, ErrNotFound) {
			return CommercialReservationPage{}, ErrNotFound
		}
		return CommercialReservationPage{}, ErrReservationUnavailable
	}
	if s.reconciler != nil && len(page.Items) > 0 {
		reconciledCount := 0
		for _, item := range page.Items {
			if reconciledCount >= 3 {
				break
			}
			if needsCheckoutReconciliation(item) {
				s.reconciler(ctx, actor, item.QuoteID)
				reconciledCount++
			}
		}
		if reconciledCount > 0 {
			if refreshed, refreshErr := s.reader.ListCommercialReservations(ctx, actor, pageSize, cursor); refreshErr == nil {
				page = refreshed
			}
		}
	}
	if page.Items == nil {
		page.Items = []CommercialReservation{}
	}
	return page, nil
}

func (s *CommercialReservationService) Get(ctx context.Context, participantID, reservationID string) (CommercialReservation, error) {
	if s == nil || ctx == nil || !validUUID(participantID) || !validUUID(reservationID) {
		return CommercialReservation{}, ErrInvalid
	}
	actor := canonicalUUID(participantID)
	resID := canonicalUUID(reservationID)
	item, err := s.reader.GetCommercialReservation(ctx, actor, resID)
	if errors.Is(err, ErrNotFound) {
		return CommercialReservation{}, ErrNotFound
	}
	if errors.Is(err, ErrInvalid) {
		return CommercialReservation{}, ErrInvalid
	}
	if err != nil {
		return CommercialReservation{}, ErrReservationUnavailable
	}
	if s.reconciler != nil && needsCheckoutReconciliation(item) {
		s.reconciler(ctx, actor, item.QuoteID)
		if refreshed, refreshErr := s.reader.GetCommercialReservation(ctx, actor, resID); refreshErr == nil {
			item = refreshed
		}
	}
	return item, nil
}

func (s *CommercialReservationService) UpdateIntervalUsage(ctx context.Context, actorID, reservationID, intervalID, status string) (CommercialReservation, error) {
	if s == nil || ctx == nil || !validUUID(actorID) || !validUUID(reservationID) || !validUUID(intervalID) {
		return CommercialReservation{}, ErrInvalid
	}
	switch status {
	case "pendiente", "en_uso", "completado", "no_asistio":
	default:
		return CommercialReservation{}, ErrInvalid
	}
	updater, ok := s.reader.(CommercialIntervalUsageUpdater)
	if !ok {
		return CommercialReservation{}, ErrReservationUnavailable
	}
	item, err := updater.UpdateCommercialIntervalUsage(ctx, canonicalUUID(actorID), canonicalUUID(reservationID), canonicalUUID(intervalID), status)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalid) {
		return CommercialReservation{}, err
	}
	if err != nil {
		return CommercialReservation{}, ErrReservationUnavailable
	}
	return item, nil
}

func (s *CommercialReservationService) VerifyPass(ctx context.Context, actorID, spaceID, codeOrQR, action string) (VerifiedPassResult, error) {
	if s == nil || ctx == nil || !validUUID(actorID) || len(codeOrQR) == 0 || len(codeOrQR) > 256 {
		return VerifiedPassResult{}, ErrInvalid
	}
	if spaceID != "" && !validUUID(spaceID) {
		return VerifiedPassResult{}, ErrInvalid
	}
	switch action {
	case "preview", "check_in", "check_out":
	default:
		return VerifiedPassResult{}, ErrInvalid
	}
	verifier, ok := s.reader.(CommercialPassVerifier)
	if !ok {
		return VerifiedPassResult{}, ErrReservationUnavailable
	}
	res, err := verifier.VerifyAndApplyPass(ctx, canonicalUUID(actorID), canonicalUUID(spaceID), codeOrQR, action)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, ErrInvalid) {
		return VerifiedPassResult{}, err
	}
	if err != nil {
		return VerifiedPassResult{}, ErrReservationUnavailable
	}
	return res, nil
}
