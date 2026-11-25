package commercialcommunityhttp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/credentials"
	"github.com/HernanEspinozaDev/espaciGo/internal/conversation"
	"github.com/HernanEspinozaDev/espaciGo/internal/identity"
	"github.com/HernanEspinozaDev/espaciGo/internal/reputation"
)

const testActor = "11111111-1111-4111-8111-111111111111"
const testReservation = "22222222-2222-4222-8222-222222222222"

type authStub struct{}

func (authStub) Authorize(context.Context, identity.Secret, identity.Role, identity.ActivityKind) (identity.Principal, error) {
	return identity.Principal{AccountID: testActor}, nil
}

type conversationRepoStub struct {
	sent   conversation.Message
	cursor int64
}

func (*conversationRepoStub) List(context.Context, string, string, *int64, int) (conversation.Page, error) {
	return conversation.Page{}, nil
}
func (*conversationRepoStub) Send(context.Context, string, string, string, string, []byte, string, func() time.Time) (conversation.Message, error) {
	return conversation.Message{}, nil
}
func (*conversationRepoStub) MarkRead(context.Context, string, string, int64) (int64, error) {
	return 0, nil
}
func (r *conversationRepoStub) ListCommercial(_ context.Context, actor, reservation string, _ *int64, _ int) (conversation.Page, error) {
	if actor != testActor || reservation != testReservation {
		return conversation.Page{}, conversation.ErrNotFound
	}
	return conversation.Page{Items: []conversation.Message{{ID: "33333333-3333-4333-8333-333333333333", ReservationID: reservation, AuthorID: actor, Sequence: 3, Body: "hola", CreatedAt: time.Unix(10, 0).UTC()}}, OlderCursor: ptr(int64(2))}, nil
}
func (r *conversationRepoStub) SendCommercial(_ context.Context, actor, reservation, key, body string, _ []byte, id string, now func() time.Time) (conversation.Message, error) {
	if actor != testActor || reservation != testReservation {
		return conversation.Message{}, conversation.ErrNotFound
	}
	r.sent = conversation.Message{ID: id, ReservationID: reservation, AuthorID: actor, Sequence: 4, Body: body, CreatedAt: now()}
	return r.sent, nil
}
func (r *conversationRepoStub) MarkCommercialRead(_ context.Context, actor, reservation string, sequence int64) (int64, error) {
	if actor != testActor || reservation != testReservation {
		return 0, conversation.ErrNotFound
	}
	r.cursor = sequence
	return sequence, nil
}
func ptr(v int64) *int64 { return &v }

type reviewRepoStub struct {
	created reputation.Review
	page    reputation.CommercialReviewPage
}

func (r *reviewRepoStub) CreateReview(context.Context, string, string, string, reputation.ReviewInput, string, []byte, time.Time) (reputation.Review, error) {
	return reputation.Review{}, nil
}
func (r *reviewRepoStub) ListSpaceReviews(context.Context, string) (reputation.SpaceReviews, error) {
	return reputation.SpaceReviews{}, nil
}
func (r *reviewRepoStub) ListReservationReviews(context.Context, string, string) ([]reputation.Review, error) {
	return nil, nil
}
func (r *reviewRepoStub) Report(context.Context, string, string, string, string, string, string, []byte, time.Time) (reputation.Report, error) {
	return reputation.Report{}, nil
}
func (r *reviewRepoStub) ListReports(context.Context) ([]reputation.Report, error) { return nil, nil }
func (r *reviewRepoStub) Moderate(context.Context, string, string, string, string, string, string, string, time.Time) (reputation.Report, error) {
	return reputation.Report{}, nil
}
func (r *reviewRepoStub) MyRenterReputation(context.Context, string) (reputation.SpaceReviews, error) {
	return reputation.SpaceReviews{}, nil
}
func (r *reviewRepoStub) PurgeExpired(context.Context, time.Time, int) (reputation.RetentionCounts, error) {
	return reputation.RetentionCounts{}, nil
}
func (r *reviewRepoStub) CreateCommercialReview(_ context.Context, id, actor, reservation string, in reputation.ReviewInput, _ string, _ []byte, at time.Time) (reputation.Review, error) {
	r.created = reputation.Review{ID: id, TargetType: "espacio", Rating: in.Rating, Comment: in.Comment, State: "publicada", CreatedAt: at}
	return r.created, nil
}
func (r *reviewRepoStub) ListCommercialReservationReviews(context.Context, string, string, time.Time) (reputation.CommercialReviewPage, error) {
	return r.page, nil
}

func makeHandler(t *testing.T) (*Handler, *conversationRepoStub, *reviewRepoStub) {
	t.Helper()
	now := func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }
	cr := &conversationRepoStub{}
	cs, err := conversation.NewService(cr, credentials.Generator{}, now)
	if err != nil {
		t.Fatal(err)
	}
	rr := &reviewRepoStub{page: reputation.CommercialReviewPage{Items: []reputation.Review{}, CanReview: true, ReservationEndsAt: now().Add(-time.Hour)}}
	rs, err := reputation.New(rr, credentials.Generator{}, now)
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(authStub{}, cs, rs, []string{"https://local.test"}), cr, rr
}

func TestCommercialMessagesRoutesPreserveBookingShapeAndReadCursor(t *testing.T) {
	h, cr, _ := makeHandler(t)
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations/"+testReservation+"/messages?limit=10", nil))
	if get.Code != 200 || !strings.Contains(get.Body.String(), `"sequence":3`) || !strings.Contains(get.Body.String(), `"body":"hola"`) || !strings.Contains(get.Body.String(), `"older_cursor":2`) {
		t.Fatalf("unexpected messages response: %d %s", get.Code, get.Body.String())
	}
	post := httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+testReservation+"/messages", strings.NewReader(`{"body":"hola"}`))
	post.Header.Set("Idempotency-Key", "message-key-1")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, post)
	if response.Code != 201 || cr.sent.Body != "hola" {
		t.Fatalf("send status=%d sent=%+v body=%s", response.Code, cr.sent, response.Body.String())
	}
	read := httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+testReservation+"/messages/read", strings.NewReader(`{"through_sequence":4}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, read)
	if rr.Code != 200 || cr.cursor != 4 || !strings.Contains(rr.Body.String(), `"read_through_sequence":4`) {
		t.Fatalf("read status=%d cursor=%d body=%s", rr.Code, cr.cursor, rr.Body.String())
	}
}

func TestCommercialReviewsExposeGateAndRejectClientSelectedTarget(t *testing.T) {
	h, _, rr := makeHandler(t)
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations/"+testReservation+"/reviews", nil))
	if get.Code != 200 || !strings.Contains(get.Body.String(), `"can_review":true`) {
		t.Fatalf("review gate response: %d %s", get.Code, get.Body.String())
	}
	bad := httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+testReservation+"/reviews", strings.NewReader(`{"rating":5,"target_type":"arrendatario"}`))
	bad.Header.Set("Idempotency-Key", "review-key-1")
	badResponse := httptest.NewRecorder()
	h.ServeHTTP(badResponse, bad)
	if badResponse.Code != 422 {
		t.Fatalf("target_type should be rejected: %d %s", badResponse.Code, badResponse.Body.String())
	}
	good := httptest.NewRequest(http.MethodPost, "/api/v1/commercial-reservations/"+testReservation+"/reviews", strings.NewReader(`{"rating":5,"comment":"Muy bien"}`))
	good.Header.Set("Idempotency-Key", "review-key-1")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, good)
	if response.Code != 201 || rr.created.TargetType != "espacio" || !strings.Contains(response.Body.String(), `"target_type":"espacio"`) {
		t.Fatalf("review response: %d %+v %s", response.Code, rr.created, response.Body.String())
	}
}

func TestCommercialConversationNonParticipantIsHidden(t *testing.T) {
	h, _, _ := makeHandler(t)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/commercial-reservations/99999999-9999-4999-8999-999999999999/messages", nil))
	if response.Code != 404 {
		t.Fatalf("nonparticipant should receive 404: %d %s", response.Code, response.Body.String())
	}
}
