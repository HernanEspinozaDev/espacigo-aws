package earlyrental

import (
	"crypto/sha256"
	"encoding/hex"
	"unicode/utf8"
)

const MaxContractTextRunes = 20_000

// DigestContractText returns the lowercase SHA-256 of the exact UTF-8 bytes.
// It does not trim, normalize line endings, or apply Unicode normalization.
func DigestContractText(text string) (string, error) {
	if text == "" || !utf8.ValidString(text) || utf8.RuneCountInString(text) > MaxContractTextRunes {
		return "", ErrInvalid
	}
	digest := sha256.Sum256([]byte(text))
	return hex.EncodeToString(digest[:]), nil
}
