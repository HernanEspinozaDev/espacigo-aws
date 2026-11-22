package commercialdocument

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
)

var (
	ErrInvalid          = errors.New("commercial document: invalid input")
	ErrLostLease        = errors.New("commercial document: lease lost")
	ErrUnsupportedGlyph = errors.New("commercial document: unsupported glyph")
)

const MaxPDFBytes = 16 << 20

type Job struct {
	ID, OperationID, RequestID, ProposalID, ObjectID, DocumentSHA256, LeaseToken string
	Version                                                                      int64
	Attempts                                                                     int
	LeaseUntil                                                                   time.Time
}
type Signature struct {
	Role, ActorID, SessionID, ConsentVersion string
	SignedAt                                 time.Time
}
type Source struct {
	Proposal                    commercialproposal.Record
	Signatures                  []Signature
	ProviderPaymentID, Currency string
	AmountCLP                   int64
	VerifiedAt                  time.Time
}
type Artifact struct {
	Bucket, Version, SHA256 string
	Size                    int64
}
type Repository interface {
	Claim(context.Context, time.Time, time.Duration) (Job, bool, error)
	Load(context.Context, Job) (Source, error)
	Complete(context.Context, Job, Artifact, time.Time) error
	Fail(context.Context, Job, string, bool, time.Time) error
}
type PrivateStore interface {
	PutWithMetadata(context.Context, string, []byte, map[string]string) error
	GetWithMetadata(context.Context, string, int64) ([]byte, map[string]string, error)
}
type VersionedPrivateStore interface {
	GetVersionedWithMetadata(context.Context, string, int64, string) ([]byte, map[string]string, string, error)
}
type Renderer interface {
	Render(context.Context, Source, []commercialproposal.EvidenceParty) ([]byte, error)
}
type Processor struct {
	Repo     Repository
	Store    PrivateStore
	Cipher   commercialproposal.Cipher
	Renderer Renderer
	Bucket   string
	Now      func() time.Time
}

func (p *Processor) RunOnce(ctx context.Context) (bool, error) {
	if p == nil || ctx == nil || p.Repo == nil || p.Store == nil || p.Cipher == nil || p.Renderer == nil || p.Bucket == "" || p.Now == nil {
		return false, ErrInvalid
	}
	job, found, err := p.Repo.Claim(ctx, p.Now().UTC(), 2*time.Minute)
	if err != nil || !found {
		return found, err
	}
	fail := func(code string, terminal bool) (bool, error) {
		delay := time.Minute
		for n := 1; n < job.Attempts && delay < time.Hour; n++ {
			delay *= 2
		}
		if delay > time.Hour {
			delay = time.Hour
		}
		return true, p.Repo.Fail(ctx, job, code, terminal, p.Now().UTC().Add(delay))
	}
	source, err := p.Repo.Load(ctx, job)
	if err != nil {
		return fail("evidence_unavailable", false)
	}
	if source.Proposal.ID != job.ProposalID || source.Proposal.Version != job.Version || source.Proposal.SHA256 != job.DocumentSHA256 || source.Currency != "CLP" || source.AmountCLP != source.Proposal.Terms.SubtotalCLP || source.VerifiedAt.IsZero() || len(source.Signatures) != 2 {
		return fail("evidence_unavailable", true)
	}
	parties, err := commercialproposal.OpenEvidence(source.Proposal, p.Cipher)
	if err != nil {
		return fail("evidence_unavailable", true)
	}
	seen := map[string]bool{}
	for _, signature := range source.Signatures {
		matched := false
		for _, party := range parties {
			if signature.Role == party.Role && signature.ActorID == party.SubjectID {
				matched = true
			}
		}
		if !matched || seen[signature.Role] || signature.ConsentVersion != "commercial-agreement-v1" || signature.SessionID == "" || signature.SignedAt.IsZero() || signature.SignedAt.After(source.VerifiedAt) {
			return fail("evidence_unavailable", true)
		}
		seen[signature.Role] = true
	}

	pdf, err := p.Renderer.Render(ctx, source, parties)
	if errors.Is(err, ErrUnsupportedGlyph) {
		return fail("unsupported_glyph", true)
	}
	if err != nil || len(pdf) < 1 || len(pdf) > MaxPDFBytes {
		return fail("render_failed", true)
	}
	sum := sha256.Sum256(pdf)
	digest := hex.EncodeToString(sum[:])
	metadata := map[string]string{"sha256": digest, "document_sha256": job.DocumentSHA256, "proposal_version": strconv.FormatInt(job.Version, 10)}
	// A previous worker may have completed the create-only write and crashed
	// before its DB commit. Read-back verifies that exact object on every path.
	_ = p.Store.PutWithMetadata(ctx, job.ObjectID, pdf, metadata)
	var stored []byte
	var observed map[string]string
	var objectVersion string
	if versioned, ok := p.Store.(VersionedPrivateStore); ok {
		stored, observed, objectVersion, err = versioned.GetVersionedWithMetadata(ctx, job.ObjectID, MaxPDFBytes, "")
	} else {
		stored, observed, err = p.Store.GetWithMetadata(ctx, job.ObjectID, MaxPDFBytes)
	}
	if err != nil {
		return fail("storage_failed", false)
	}
	storedSum := sha256.Sum256(stored)
	if len(stored) != len(pdf) || hex.EncodeToString(storedSum[:]) != digest || observed["sha256"] != digest || observed["document_sha256"] != job.DocumentSHA256 || observed["proposal_version"] != metadata["proposal_version"] {
		return fail("object_conflict", true)
	}
	err = p.Repo.Complete(ctx, job, Artifact{Bucket: p.Bucket, Version: objectVersion, SHA256: digest, Size: int64(len(pdf))}, p.Now().UTC())
	return true, err
}
func (p *Processor) Run(ctx context.Context) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		_, err := p.RunOnce(ctx)
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
