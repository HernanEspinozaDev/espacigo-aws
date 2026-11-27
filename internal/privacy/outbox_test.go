package privacy

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryOutboxRow struct {
	job          PrivacyNoticeJob
	state        string
	attempts     int
	availableAt  time.Time
	deliveredAt  *time.Time
	terminalAt   *time.Time
	lastErrCode  string
}

type memoryOutboxRepo struct {
	memoryRepository
	mu   sync.Mutex
	rows []*memoryOutboxRow
}

func (m *memoryOutboxRepo) addPending(id, email, subject, body string, availableAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows = append(m.rows, &memoryOutboxRow{
		job: PrivacyNoticeJob{
			ID:             id,
			RequestID:      "req-" + id,
			NoticeType:     "acuse_recepcion",
			IdempotencyKey: "acuse:req-" + id,
			RecipientEmail: email,
			Subject:        subject,
			Body:           body,
		},
		state:       "pending",
		availableAt: availableAt,
	})
}

func (m *memoryOutboxRepo) ClaimPrivacyNotice(_ context.Context, now time.Time) (PrivacyNoticeJob, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.rows {
		if row.state == "pending" && !row.availableAt.After(now) && row.attempts < 6 {
			row.state = "processing"
			row.attempts++
			job := row.job
			job.Attempt = row.attempts
			return job, true, nil
		}
	}
	return PrivacyNoticeJob{}, false, nil
}

func (m *memoryOutboxRepo) FinishPrivacyNotice(_ context.Context, job PrivacyNoticeJob, delivered bool, errorCode string, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, row := range m.rows {
		if row.job.ID == job.ID && row.state == "processing" {
			if delivered {
				row.state = "delivered"
				t := now
				row.deliveredAt = &t
				row.lastErrCode = ""
				return nil
			}
			if job.Attempt >= 6 {
				row.state = "terminal"
				t := now
				row.terminalAt = &t
				row.lastErrCode = "attempt_limit_reached"
				return nil
			}
			row.state = "pending"
			row.availableAt = now.Add(time.Duration(job.Attempt) * time.Minute)
			row.lastErrCode = errorCode
			return nil
		}
	}
	return ErrNotFound
}

type stubNoticeSender struct {
	mu       sync.Mutex
	failErr  error
	sentList []PrivacyNoticeJob
}

func (s *stubNoticeSender) SendLocalBookingNotice(_ context.Context, email, subject, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failErr != nil {
		return s.failErr
	}
	s.sentList = append(s.sentList, PrivacyNoticeJob{RecipientEmail: email, Subject: subject, Body: body})
	return nil
}

func TestDispatchPrivacyNoticeOnceSuccessRetryAndTerminal(t *testing.T) {
	now := time.Date(2026, time.September, 1, 10, 0, 0, 0, time.UTC)
	repo := &memoryOutboxRepo{}
	svc, err := NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = func() time.Time { return now }

	sender := &stubNoticeSender{}
	processed, err := svc.DispatchPrivacyNoticeOnce(context.Background(), sender)
	if err != nil || processed {
		t.Fatalf("empty queue dispatch: processed=%t err=%v", processed, err)
	}

	subject, body := FormatAcuseRecepcionNotice("PRIV-2026-ABCDEF12", "acceso", now.Add(24*time.Hour))
	repo.addPending("1", "titular@ejemplo.cl", subject, body, now)

	// First 5 failures reschedule with backoff and mail_delivery_failed.
	sender.failErr = errors.New("smtp down")
	for attempt := 1; attempt <= 5; attempt++ {
		processed, err := svc.DispatchPrivacyNoticeOnce(context.Background(), sender)
		if err != nil || !processed {
			t.Fatalf("attempt %d: processed=%t err=%v", attempt, processed, err)
		}
		row := repo.rows[0]
		if row.state != "pending" || row.attempts != attempt || row.lastErrCode != "mail_delivery_failed" {
			t.Fatalf("attempt %d unexpected row state: %+v", attempt, row)
		}
		wantNext := now.Add(time.Duration(attempt) * time.Minute)
		if !row.availableAt.Equal(wantNext) {
			t.Fatalf("attempt %d availableAt=%v want %v", attempt, row.availableAt, wantNext)
		}
		now = wantNext
	}

	// 6th failure moves job to terminal with attempt_limit_reached.
	processed, err = svc.DispatchPrivacyNoticeOnce(context.Background(), sender)
	if err != nil || !processed {
		t.Fatalf("attempt 6: processed=%t err=%v", processed, err)
	}
	if repo.rows[0].state != "terminal" || repo.rows[0].terminalAt == nil || repo.rows[0].lastErrCode != "attempt_limit_reached" {
		t.Fatalf("expected terminal row after 6 failures, got %+v", repo.rows[0])
	}

	// Second job succeeds immediately.
	sender.failErr = nil
	repo.addPending("2", "otro@ejemplo.cl", subject, body, now)
	processed, err = svc.DispatchPrivacyNoticeOnce(context.Background(), sender)
	if err != nil || !processed {
		t.Fatalf("second job: processed=%t err=%v", processed, err)
	}
	if repo.rows[1].state != "delivered" || repo.rows[1].deliveredAt == nil || len(sender.sentList) != 1 {
		t.Fatalf("expected delivered row, got %+v sent=%d", repo.rows[1], len(sender.sentList))
	}
}

func TestRunPrivacyNoticeWorkerDrainsAndStopsOnCancel(t *testing.T) {
	now := time.Date(2026, time.September, 1, 10, 0, 0, 0, time.UTC)
	repo := &memoryOutboxRepo{}
	repo.addPending("1", "a@ejemplo.cl", "Asunto 1", "Cuerpo neutro 1", now)
	repo.addPending("2", "b@ejemplo.cl", "Asunto 2", "Cuerpo neutro 2", now)

	svc := newTestServiceWithClock(repo, now)
	sender := &stubNoticeSender{}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.RunPrivacyNoticeWorker(ctx, 10*time.Millisecond, sender)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sender.mu.Lock()
		count := len(sender.sentList)
		sender.mu.Unlock()
		if count == 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	<-done

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.sentList) != 2 {
		t.Fatalf("worker sent %d notices, want 2", len(sender.sentList))
	}
}

func TestFormatAcuseRecepcionNoticeIsNeutralAndBounded(t *testing.T) {
	due := time.Date(2026, time.July, 15, 23, 59, 59, 0, SantiagoLocation()).UTC()
	subject, body := FormatAcuseRecepcionNotice("PRIV-2026-1234ABCD", "bloqueo", due)
	for _, want := range []string{"PRIV-2026-1234ABCD", "bloqueo", "America/Santiago", "/privacidad/solicitudes", "no confirma"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	if len(subject) < 3 || len(subject) > 200 || len(body) < 10 || len(body) > 4000 {
		t.Fatalf("invalid lengths subject=%d body=%d", len(subject), len(body))
	}
}
