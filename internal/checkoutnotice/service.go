// Package checkoutnotice delivers durable email intents for verified
// commercial checkout transitions.
package checkoutnotice

import (
	"context"
	"errors"
	"fmt"
	"time"
)

var ErrInvalid = errors.New("checkoutnotice: invalid request")

type Notice struct {
	ID, ReservationID, QuoteID, RecipientRole, Transition string
	Email, SpaceTitle, TimeZone                           string
	AmountCLP                                             int64
	StartAt, EndAt                                        time.Time
	Attempts                                              int
}

type Repository interface {
	Claim(context.Context, time.Time) (Notice, bool, error)
	Finish(context.Context, Notice, bool, string, time.Time) error
}

type Sender interface {
	SendLocalBookingNotice(context.Context, string, string, string) error
}

type Service struct {
	repo   Repository
	sender Sender
	now    func() time.Time
}

func New(repo Repository, sender Sender, now func() time.Time) (*Service, error) {
	if repo == nil || sender == nil {
		return nil, ErrInvalid
	}
	if now == nil {
		now = time.Now
	}
	return &Service{repo: repo, sender: sender, now: now}, nil
}

func (s *Service) DispatchOne(ctx context.Context) (bool, error) {
	n, ok, err := s.repo.Claim(ctx, s.now().UTC())
	if err != nil || !ok {
		return ok, err
	}
	subject, body := render(n)
	err = s.sender.SendLocalBookingNotice(ctx, n.Email, subject, body)
	code := ""
	if err != nil {
		code = "mail_delivery_failed"
	}
	if finishErr := s.repo.Finish(ctx, n, err == nil, code, s.now().UTC()); finishErr != nil {
		return true, finishErr
	}
	return true, nil
}

func (s *Service) Run(ctx context.Context, interval time.Duration) {
	if interval < 250*time.Millisecond {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.DispatchOne(ctx)
		}
	}
}

func render(n Notice) (string, string) {
	title := n.SpaceTitle
	if title == "" {
		title = "tu espacio reservado"
	}
	amount := fmt.Sprintf("$%d CLP", n.AmountCLP)
	location, err := time.LoadLocation(n.TimeZone)
	if err != nil {
		location = time.UTC
	}
	interval := n.StartAt.In(location).Format("02-01-2006 15:04 MST") + " – " + n.EndAt.In(location).Format("02-01-2006 15:04 MST")
	switch n.Transition {
	case "approved":
		if n.RecipientRole == "anfitrion" {
			return "Nueva reserva pagada en EspaciGo", fmt.Sprintf("Se confirmó el pago de una reserva para %s. Total del arriendo: %s. Horario: %s. Inicia sesión en EspaciGo para revisar la reserva. Reserva %s.", title, amount, interval, n.ReservationID)
		}
		return "Pago recibido en EspaciGo", fmt.Sprintf("Recibimos y verificamos tu pago por %s. Total del arriendo: %s. Horario: %s. Inicia sesión en EspaciGo para consultar tu reserva. Reserva %s.", title, amount, interval, n.ReservationID)
	case "rejected":
		return "No se aprobó el pago de tu arriendo", fmt.Sprintf("Mercado Pago no aprobó el pago de %s por %s. No se confirmó la reserva. Puedes volver a consultar disponibilidad y solicitar una nueva cotización. Reserva %s.", title, amount, n.ReservationID)
	case "expired":
		return "Venció el pago de tu cotización", fmt.Sprintf("El plazo para pagar el arriendo de %s por %s venció. La reserva ya no mantiene el horario. Consulta nuevamente disponibilidad para continuar.", title, amount)
	default:
		return "Actualización de tu arriendo en EspaciGo", "Inicia sesión en EspaciGo para consultar el estado actualizado de tu arriendo."
	}
}
