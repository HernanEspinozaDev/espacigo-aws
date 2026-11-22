package publiccatalog

import (
	"regexp"
	"strings"
	"unicode"
)

// These patterns retain the public-review privacy rules. Contact links are
// also removed here because listing descriptions may be published verbatim.
var publicEmailPattern = regexp.MustCompile(`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`)
var publicUUIDPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)
var publicRUTPattern = regexp.MustCompile(`(?i)\b\d{1,2}(?:\.\d{3}){2}-[\dk]\b|\b\d{7,8}-[\dk]\b`)
var publicPhonePattern = regexp.MustCompile(`(?:\+?56[ .-]?)?\b(?:9[ .-]?\d{4}[ .-]?\d{4}|[2-8][ .-]?\d{4}[ .-]?\d{4})\b`)
var publicLinkPattern = regexp.MustCompile(`(?i)(?:https?://|www\.|mailto:|tel:|wa\.me/|t\.me/)[^\s<>]+|\b(?:instagram|facebook|whatsapp)\s*[:=]\s*[^\s<>]+`)
var publicHandlePattern = regexp.MustCompile(`@[\p{L}\p{N}_][\p{L}\p{N}_.-]*`)

func publicText(s string) string {
	s = publicEmailPattern.ReplaceAllString(s, "[correo omitido]")
	s = publicLinkPattern.ReplaceAllString(s, "[enlace omitido]")
	s = publicUUIDPattern.ReplaceAllString(s, "[identificador omitido]")
	s = publicRUTPattern.ReplaceAllString(s, "[identificador omitido]")
	s = publicPhonePattern.ReplaceAllString(s, "[teléfono omitido]")
	s = publicHandlePattern.ReplaceAllString(s, "[contacto omitido]")
	return strings.TrimSpace(s)
}

func publicProjection(item Space) Space {
	item.Title = publicText(item.Title)
	item.Description = publicText(item.Description)
	item.UsageRules = publicText(item.UsageRules)
	if item.Location != nil {
		location := *item.Location
		location.Address = publicText(location.Address)
		item.Location = &location
	}
	if item.Host != nil {
		host := *item.Host
		host.Name = nil
		if item.Host.Name != nil {
			parts := strings.Fields(*item.Host.Name)
			if len(parts) > 0 {
				first := parts[0]
				valid := true
				for _, r := range first {
					if !unicode.IsLetter(r) && !unicode.IsMark(r) && r != '\'' && r != '-' {
						valid = false
					}
				}
				if valid {
					host.Name = &first
				}
			}
		}
		item.Host = &host
	}
	return item
}
