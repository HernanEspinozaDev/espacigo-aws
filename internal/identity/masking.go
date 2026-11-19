package identity

import (
	"fmt"
	"net/netip"
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

// MaskPhone keeps the international prefix (+56) and the last 4 digits (e.g. "+56912345678" -> "+56 ••••5678").
// Already masked phones "+56 ••••XXXX" return the normalized masked format.
// Invalid or short values return "••••".
func MaskPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return "••••"
	}
	if strings.HasPrefix(phone, "+56 ••••") {
		suffix := strings.TrimPrefix(phone, "+56 ••••")
		if len(suffix) == 4 {
			allDigits := true
			for _, r := range suffix {
				if r < '0' || r > '9' {
					allDigits = false
					break
				}
			}
			if allDigits {
				return phone
			}
		}
		return "••••"
	}

	normalized, err := ValidateAndNormalizeChileanPhone(phone)
	if err != nil {
		return "••••"
	}
	return "+56 ••••" + normalized[len(normalized)-4:]
}

// MaskIP masks IPv4 and IPv6 addresses to protect user privacy (A.8.11).
// IPv4 retains the first two octets (e.g. "190.162.12.45" -> "190.162.•••.•••").
// IPv6 retains the first two 16-bit blocks (e.g. "2001:db8:••••:••••").
// Empty string returns "", invalid IP returns "••••".
func MaskIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return "••••"
	}
	addr = addr.Unmap()
	if addr.Is4() {
		b := addr.As4()
		return fmt.Sprintf("%d.%d.•••.•••", b[0], b[1])
	}
	if addr.Is6() {
		b := addr.As16()
		g1 := uint16(b[0])<<8 | uint16(b[1])
		g2 := uint16(b[2])<<8 | uint16(b[3])
		return fmt.Sprintf("%x:%x:••••:••••", g1, g2)
	}
	return "••••"
}


