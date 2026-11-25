// Package commercialpolicy defines the published versions of cancellation terms.
package commercialpolicy

type Policy struct {
	Version string `json:"version"`
	Text    string `json:"text"`
}

// ByVersion keeps the terms of every published version stable. Changes to
// terms require a new version, while existing quotes retain their own text.
func ByVersion(version string) (Policy, bool) {
	var text string
	switch version {
	case "facil_v1":
		text = "Cancelación fácil: devolución del 100 % antes del inicio del arriendo."
	case "flexible_v1":
		text = "Flexible: reembolso total hasta 24 horas antes del inicio del arriendo."
	case "moderada_v1":
		text = "Moderada: reembolso del 50 % hasta 48 horas antes del inicio del arriendo."
	case "estricta_v1":
		text = "Estricta: sin reembolso después de confirmar la reserva."
	default:
		return Policy{}, false
	}
	return Policy{Version: version, Text: text}, true
}
