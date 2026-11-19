package identity

import (
	"strings"
	"unicode"
)

// MaskEmail keeps only the first and last characters of a valid address.
func MaskEmail(email string) string {
	email = strings.TrimSpace(email)
	if ValidateEmail(email) != nil {
		return "••••"
	}
	characters := []rune(email)
	return string(characters[0]) + "••••" + string(characters[len(characters)-1])
}

// MaskRUT keeps only the last four significant characters of a RUT (including DV).
// Invalid or short values return "••••".
func MaskRUT(rut string) string {
	rut = strings.TrimSpace(rut)
	if rut == "" {
		return "••••"
	}
	runes := []rune(rut)
	if strings.HasPrefix(rut, "••••") {
		if len(runes) == 8 {
			validSuffix := true
			for _, r := range runes[4:] {
				if !(r >= '0' && r <= '9') && r != 'K' && r != 'k' {
					validSuffix = false
					break
				}
			}
			if validSuffix {
				return string(runes[:4]) + strings.ToUpper(string(runes[4:]))
			}
		}
		return "••••"
	}

	significant := make([]rune, 0, len(runes))
	for _, r := range runes {
		if r == '.' || r == '-' || r == '–' || unicode.IsSpace(r) {
			continue
		}
		if (r >= '0' && r <= '9') || r == 'k' || r == 'K' {
			significant = append(significant, unicode.ToUpper(r))
		} else {
			return "••••"
		}
	}

	if len(significant) < 4 || len(significant) > 10 {
		return "••••"
	}

	for i := 0; i < len(significant)-1; i++ {
		if significant[i] == 'K' {
			return "••••"
		}
	}

	return "••••" + string(significant[len(significant)-4:])
}
