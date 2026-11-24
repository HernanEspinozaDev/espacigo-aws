package commercialagreement

import (
	"context"
	"fmt"
	"time"
)

// Notice delivery resolves an address only at dispatch; durable intents and
// message text contain no contractual identity, legal name, RUT or contacts.
type Notice struct {
	ID, AgreementID, Email string
	Attempt                int
}
type NoticeRepository interface {
	ClaimNotice(context.Context, time.Time) (Notice, bool, error)
	FinishNotice(context.Context, Notice, bool, time.Time) error
}
type Sender interface {
	SendLocalBookingNotice(context.Context, string, string, string) error
}
type NoticeDispatcher struct {
	repo   NoticeRepository
	sender Sender
	now    func() time.Time
}

func NewNoticeDispatcher(repo NoticeRepository, sender Sender, now func() time.Time) (*NoticeDispatcher, error) {
	if repo == nil || sender == nil {
		return nil, ErrInvalid
	}
	if now == nil {
		now = time.Now
	}
	return &NoticeDispatcher{repo: repo, sender: sender, now: now}, nil
}
func (d *NoticeDispatcher) DispatchOne(ctx context.Context) (bool, error) {
	notice, ok, err := d.repo.ClaimNotice(ctx, d.now().UTC())
	if err != nil || !ok {
		return ok, err
	}
	err = d.sender.SendLocalBookingNotice(ctx, notice.Email, "Actualización de solicitud en EspaciGo", fmt.Sprintf("Hay una actualización en tu solicitud de arriendo. Inicia sesión en EspaciGo para revisarla. Solicitud %s.", notice.AgreementID))
	if e := d.repo.FinishNotice(ctx, notice, err == nil, d.now().UTC()); e != nil {
		return true, e
	}
	return true, err
}
func (d *NoticeDispatcher) Run(ctx context.Context, interval time.Duration) {
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = d.DispatchOne(ctx)
		}
	}
}
func (s *Service) RunExpiration(ctx context.Context, interval time.Duration) {
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = s.ExpireDue(ctx, 25)
		}
	}
}
