// Package contracttemplate manages commercial space clauses. It is independent
// of synthetic booking-trial contracts and never accepts contractual identities,
// prices, dates or locations from the editor.
package contracttemplate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
)

var (
	ErrInvalid      = errors.New("contracttemplate: invalid")
	ErrNotFound     = errors.New("contracttemplate: not found")
	ErrPrecondition = errors.New("contracttemplate: revision mismatch")
	ErrConflict     = errors.New("contracttemplate: conflict")
)

type Section struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

type Draft struct {
	SpaceID          string     `json:"space_id"`
	Revision         int64      `json:"revision"`
	Sections         []Section  `json:"sections"`
	PublishedVersion int64      `json:"published_version"`
	UpdatedAt        *time.Time `json:"updated_at"`
}

type Version struct {
	ID          string    `json:"id"`
	SpaceID     string    `json:"space_id"`
	Version     int64     `json:"version"`
	Sections    []Section `json:"sections"`
	SHA256      string    `json:"sha256"`
	PublishedAt time.Time `json:"published_at"`
}

type Repository interface {
	Get(context.Context, string, string) (Draft, error)
	Save(context.Context, string, string, int64, []Section) (Draft, error)
	Publish(context.Context, string, string, int64) (Version, int64, error)
	Versions(context.Context, string, string) ([]Version, error)
	Copy(context.Context, string, string, int64, string, int64) (Draft, error)
}

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

var contactPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`),
	regexp.MustCompile(`(?i)(?:[a-z][a-z0-9+.-]*://|www\.|mailto:|tel:|wa\.me/|t\.me/)|\b[a-z0-9][a-z0-9.-]*\.[a-z]{2,63}\b`),
	regexp.MustCompile(`@[\p{L}\p{N}_][\p{L}\p{N}_.-]*`),
	regexp.MustCompile(`(?:\+?56[ .()-]?)?\b[2-9][ .()-]?\d{4}[ .()-]?\d{4}\b|\+\d(?:[ .()-]*\d){7,14}\b`),
}

// Contact-free clauses can be shown before payment. Reject rather than silently
// rewriting a contractual rule, including common Unicode-width obfuscations.
func ContainsContactText(text string) bool {
	text = norm.NFKC.String(text)
	for _, pattern := range contactPatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

// CanonicalSections preserves the owner's section order. Only plain text is
// supported; no markup or template expressions are evaluated downstream.
func CanonicalSections(sections []Section) ([]byte, string, error) {
	if len(sections) < 1 || len(sections) > 12 {
		return nil, "", ErrInvalid
	}
	seen := map[string]bool{}
	total := 0
	for _, s := range sections {
		if !keyPattern.MatchString(s.Key) || seen[s.Key] || !validText(s.Title, 100, false) || !validText(s.Body, 4000, true) {
			return nil, "", ErrInvalid
		}
		seen[s.Key] = true
		total += utf8.RuneCountInString(s.Title) + utf8.RuneCountInString(s.Body)
	}
	if total > 20000 {
		return nil, "", ErrInvalid
	}
	blob, err := json.Marshal(sections)
	if err != nil {
		return nil, "", ErrInvalid
	}
	sum := sha256.Sum256(blob)
	return blob, hex.EncodeToString(sum[:]), nil
}

func validText(s string, max int, multiline bool) bool {
	if !utf8.ValidString(s) || s != strings.TrimSpace(s) || s == "" || utf8.RuneCountInString(s) > max {
		return false
	}
	for _, r := range s {
		if r == '<' || r == '>' || unicode.Is(unicode.Cf, r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return false
		}
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t')) {
			return false
		}
	}
	return !ContainsContactText(s)
}

func ValidIDs(ids ...string) bool {
	for _, id := range ids {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed == uuid.Nil {
			return false
		}
	}
	return true
}
