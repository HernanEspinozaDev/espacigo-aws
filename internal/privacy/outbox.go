package privacy

import (
	"context"
	"errors"
	"time"
)

type NoticeSender interface {
	SendLocalBookingNotice(ctx context.Context, email, subject, body string) error
}

type PrivacyNoticeJob struct {
	ID             string `json:"id"`
	RequestID      string `json:"request_id"`
	NoticeType     string `json:"notice_type"`
	IdempotencyKey string `json:"idempotency_key"`
	RecipientEmail string `json:"recipient_email"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
	Attempt        int    `json:"attempt"`
}

// NoticeOutboxRepository claims and finalizes durable privacy email notifications.
type NoticeOutboxRepository interface {
	ClaimPrivacyNotice(ctx context.Context, now time.Time) (PrivacyNoticeJob, bool, error)
	FinishPrivacyNotice(ctx context.Context, job PrivacyNoticeJob, delivered bool, errorCode string, now time.Time) error
}

func (s *Service) DispatchPrivacyNoticeOnce(ctx context.Context, sender NoticeSender) (bool, error) {
	if s == nil || s.repo == nil || ctx == nil || sender == nil {
		return false, ErrInvalid
	}
	repo, ok := s.repo.(NoticeOutboxRepository)
	if !ok {
		return false, errors.New("privacy: notice outbox repository unavailable")
	}
	job, claimed, err := repo.ClaimPrivacyNotice(ctx, s.clockNow().UTC())
	if err != nil || !claimed {
		return claimed, err
	}
	sendErr := sender.SendLocalBookingNotice(ctx, job.RecipientEmail, job.Subject, job.Body)
	errorCode := ""
	if sendErr != nil {
		errorCode = "mail_delivery_failed"
	}
	if err := repo.FinishPrivacyNotice(ctx, job, sendErr == nil, errorCode, s.clockNow().UTC()); err != nil {
		return true, err
	}
	return true, nil
}

func (s *Service) RunPrivacyNoticeWorker(ctx context.Context, interval time.Duration, sender NoticeSender) {
	if s == nil || ctx == nil || sender == nil {
		return
	}
	if interval <= 0 {
		interval = time.Minute
	}
	drain := func() {
		for {
			if ctx.Err() != nil {
				return
			}
			processed, err := s.DispatchPrivacyNoticeOnce(ctx, sender)
			if err != nil || !processed {
				return
			}
		}
	}
	drain()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			drain()
		}
	}
}
