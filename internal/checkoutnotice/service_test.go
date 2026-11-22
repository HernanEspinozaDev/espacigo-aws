package checkoutnotice

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type memoryRepo struct {
	notice Notice
	finish []any
}

func (r *memoryRepo) Claim(context.Context, time.Time) (Notice, bool, error) {
	return r.notice, true, nil
}
func (r *memoryRepo) Finish(_ context.Context, n Notice, success bool, code string, _ time.Time) error {
	r.finish = []any{n.Attempts, success, code}
	return nil
}

type captureSender struct {
	email, subject, body string
	err                  error
}

func (s *captureSender) SendLocalBookingNotice(_ context.Context, email, subject, body string) error {
	s.email, s.subject, s.body = email, subject, body
	return s.err
}

func TestDispatchRendersVerifiedPaymentNoticesWithoutDemoLanguage(t *testing.T) {
	for _, tc := range []struct {
		name, transition, role, subjectText, bodyText string
	}{
		{"renter approval", "approved", "arrendatario", "Pago recibido", "Total del arriendo: $25000 CLP"},
		{"host approval", "approved", "anfitrion", "Nueva reserva pagada", "reserva"},
		{"rejection", "rejected", "arrendatario", "No se aprobó", "nueva cotización"},
		{"expiry", "expired", "arrendatario", "Venció el pago", "ya no mantiene el horario"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &memoryRepo{notice: Notice{
				ID: "notice-1", ReservationID: "reservation-1", QuoteID: "quote-1", RecipientRole: tc.role,
				Transition: tc.transition, Email: "actor@example.invalid", SpaceTitle: "Sala sintética",
				AmountCLP: 25000, StartAt: time.Date(2030, 1, 16, 12, 0, 0, 0, time.UTC), EndAt: time.Date(2030, 1, 16, 13, 0, 0, 0, time.UTC), Attempts: 1,
			}}
			sender := &captureSender{}
			service, err := New(repo, sender, func() time.Time { return time.Date(2030, 1, 16, 12, 1, 0, 0, time.UTC) })
			if err != nil {
				t.Fatal(err)
			}
			delivered, err := service.DispatchOne(context.Background())
			if err != nil || !delivered {
				t.Fatalf("delivered=%v err=%v", delivered, err)
			}
			if sender.email != "actor@example.invalid" || !strings.Contains(sender.subject, tc.subjectText) || !strings.Contains(sender.body, tc.bodyText) {
				t.Fatalf("email=%q subject=%q body=%q", sender.email, sender.subject, sender.body)
			}
			if strings.Contains(strings.ToLower(sender.subject+sender.body), "simulad") || strings.Contains(sender.body, "SIN COBRO REAL") {
				t.Fatalf("commercial notice was labeled as simulated: %q", sender.subject+sender.body)
			}
			if len(repo.finish) != 3 || repo.finish[1] != true || repo.finish[2] != "" {
				t.Fatalf("finish=%v", repo.finish)
			}
		})
	}
}

func TestDispatchReportsSenderFailureForDurableRetry(t *testing.T) {
	repo := &memoryRepo{notice: Notice{ID: "notice-1", Transition: "approved", RecipientRole: "arrendatario", Email: "actor@example.invalid", Attempts: 2}}
	sender := &captureSender{err: errors.New("synthetic SMTP failure")}
	service, err := New(repo, sender, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	delivered, err := service.DispatchOne(context.Background())
	if err != nil || !delivered {
		t.Fatalf("delivered=%v err=%v", delivered, err)
	}
	if len(repo.finish) != 3 || repo.finish[1] != false || repo.finish[2] != "mail_delivery_failed" {
		t.Fatalf("finish=%v", repo.finish)
	}
}
