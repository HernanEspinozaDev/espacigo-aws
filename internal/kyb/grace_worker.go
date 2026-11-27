package kyb

import (
	"context"
	"time"
)

type GraceWorker struct {
	service  *Service
	interval time.Duration
	now      func() time.Time
}

func NewGraceWorker(service *Service, interval time.Duration, now func() time.Time) (*GraceWorker, error) {
	if service == nil || interval < time.Minute || now == nil {
		return nil, ErrInvalid
	}
	if _, ok := service.repository.(ComplianceRepository); !ok {
		return nil, ErrInvalid
	}
	return &GraceWorker{service: service, interval: interval, now: now}, nil
}

// RunOnce invokes the idempotent database expiry function. The database owns
// selection, queue protection, status transitions, and audit insertion.
func (w *GraceWorker) RunOnce(ctx context.Context) (int, error) {
	if w == nil || w.service == nil || ctx == nil {
		return 0, ErrInvalid
	}
	return w.service.ExpireNoncompliant(ctx, w.now())
}

func (w *GraceWorker) Run(ctx context.Context) error {
	if w == nil || ctx == nil {
		return ErrInvalid
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	if _, err := w.RunOnce(ctx); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := w.RunOnce(ctx); err != nil {
				return err
			}
		}
	}
}
