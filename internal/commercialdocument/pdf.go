package commercialdocument

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// PDFRenderer embeds a Unicode TrueType font and emits ToUnicode maps. It
// wraps every paragraph over as many A4 pages as required, never truncates
// clauses, and fails explicitly if the font cannot display an input glyph.
type PDFRenderer struct{}

func (PDFRenderer) Render(ctx context.Context, source Source, parties []commercialproposal.EvidenceParty) ([]byte, error) {
	if ctx == nil || len(parties) != 2 {
		return nil, ErrInvalid
	}
	terms := source.Proposal.Terms
	var landlord, renter commercialproposal.EvidenceParty
	for _, p := range parties {
		if p.Role == "arrendador" {
			landlord = p
		} else if p.Role == "arrendatario" {
			renter = p
		}
	}
	ordinals := []string{"QUINTO", "SEXTO", "SÉPTIMO", "OCTAVO", "NOVENO", "DÉCIMO", "DÉCIMO PRIMERO", "DÉCIMO SEGUNDO", "DÉCIMO TERCERO", "DÉCIMO CUARTO", "DÉCIMO QUINTO", "DÉCIMO SEXTO"}
	ordinal := func(i int) string {
		if i < len(ordinals) {
			return ordinals[i]
		}
		return "CLÁUSULA " + strconv.Itoa(i+5)
	}
	lines := []string{
		"ESPACIGO — CONTRATO DE ARRENDAMIENTO DE ESPACIO",
		"Propuesta: " + source.Proposal.ID + " · Versión: " + strconv.FormatInt(source.Proposal.Version, 10),
		"SHA-256 documento firmado: " + source.Proposal.SHA256,
		"",
		fmt.Sprintf("En Chile, comparecen: por una parte, %s, RUT %s, en adelante EL ARRENDADOR; y por la otra, %s, RUT %s, en adelante EL ARRENDATARIO; ambos mayores de edad, con identidad verificada en EspaciGo, quienes convienen en celebrar el siguiente Contrato de Arrendamiento Simple regido por la Ley N.º 18.101, el Código Civil de Chile y la Ley N.º 19.799 (FES):", landlord.LegalName, landlord.RUT, renter.LegalName, renter.RUT),
		"",
		fmt.Sprintf("PRIMERO. INDIVIDUALIZACIÓN DEL INMUEBLE: EL ARRENDADOR entrega en arrendamiento el espacio denominado «%s», ubicado en %s, en condiciones aptas para el uso acordado.", terms.SpaceTitle, spaces.StripPostalCodeFromAddress(terms.Location.Address)),
		"SEGUNDO. OBJETO Y USO PERMITIDO: EL ARRENDATARIO no podrá subarrendar, ceder ni destinar el espacio a un uso distinto o ilícito sin autorización previa y escrita de EL ARRENDADOR.",
		fmt.Sprintf("TERCERO. VIGENCIA, HORARIO Y RESTITUCIÓN: El arrendamiento regirá desde %s hasta %s (zona horaria %s). Al término de dicho período, EL ARRENDATARIO restituirá el inmueble de inmediato en el mismo estado de conservación y aseo.", terms.StartAt.Format("2006-01-02T15:04:05Z07:00"), terms.EndAt.Format("2006-01-02T15:04:05Z07:00"), terms.TimeZone),
		fmt.Sprintf("CUARTO. CANON DE ARRIENDO Y FORMA DE PAGO: La renta se fija en %d %s (%d %s a razón de %d %s por %s), pagadera en línea mediante EspaciGo / Mercado Pago.", terms.SubtotalCLP, terms.Currency, terms.Units, terms.RateUnit, terms.UnitPriceCLP, terms.Currency, terms.RateUnit),
		"",
	}
	for i, section := range terms.Sections {
		lines = append(lines, ordinal(i)+". "+strings.ToUpper(section.Title)+": "+section.Body, "")
	}
	lines = append(lines, ordinal(len(terms.Sections))+". POLÍTICA DE CANCELACIÓN: "+terms.CancellationPolicy.Text)
	lines = append(lines, ordinal(len(terms.Sections)+1)+". DOMICILIO Y JURISDICCIÓN: Las partes fijan domicilio en la comuna del inmueble y se someten a los Tribunales Ordinarios de Justicia de Chile, perfeccionándose mediante Firma Electrónica Simple (FES) conforme a la Ley N.º 19.799.", "", "EVIDENCIA DE FIRMAS ELECTRÓNICAS")
	signatures := append([]Signature(nil), source.Signatures...)
	sort.Slice(signatures, func(i, j int) bool { return signatures[i].Role < signatures[j].Role })
	for _, s := range signatures {
		lines = append(lines, s.Role+" — "+s.SignedAt.UTC().Format("2006-01-02T15:04:05Z"), "Consentimiento: "+s.ConsentVersion, "Referencia de sesión: "+s.SessionID)
	}
	lines = append(lines, "", "MODALIDAD DE FIRMA", "Firma Electrónica Simple (FES) — Ley N.º 19.799", "Sello criptográfico de plataforma: PKCS#7 / RSA-2048", "", "CONFIRMACIÓN DE PAGO", "Proveedor: Mercado Pago", "Referencia verificada: "+source.ProviderPaymentID, fmt.Sprintf("Monto verificado: %d %s", source.AmountCLP, source.Currency), "Confirmado por el servidor: "+source.VerifiedAt.UTC().Format("2006-01-02T15:04:05Z"), "Estado: aprobado")
	rawPDF, err := renderUnicodePDF(ctx, lines)
	if err != nil {
		return nil, err
	}
	seal, err := DefaultPlatformSeal()
	if err != nil {
		return nil, err
	}
	return SealPDF(rawPDF, seal, source.VerifiedAt)
}

type pdfGlyph struct {
	cid   uint16
	gid   sfnt.GlyphIndex
	width int
}

func renderUnicodePDF(ctx context.Context, paragraphs []string) ([]byte, error) {
	face, err := sfnt.Parse(goregular.TTF)
	if err != nil {
		return nil, err
	}
	var buf sfnt.Buffer
	glyphs := map[rune]pdfGlyph{}
	// Include every footer character before assigning deterministic CIDs.
	characters := map[rune]bool{}
	for _, text := range append(append([]string(nil), paragraphs...), "Página 0123456789 / ") {
		for _, r := range strings.ReplaceAll(text, "\t", "    ") {
			if r != '\n' && r != '\r' {
				characters[r] = true
			}
		}
	}
	runes := make([]rune, 0, len(characters))
	for r := range characters {
		runes = append(runes, r)
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
	if len(runes) > 65534 {
		return nil, ErrInvalid
	}
	for i, r := range runes {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		gid, e := face.GlyphIndex(&buf, r)
		if e != nil {
			return nil, e
		}
		if gid == 0 {
			return nil, fmt.Errorf("%w: U+%04X", ErrUnsupportedGlyph, r)
		}
		advance, e := face.GlyphAdvance(&buf, gid, fixed.I(1000), font.HintingNone)
		if e != nil {
			return nil, e
		}
		glyphs[r] = pdfGlyph{cid: uint16(i + 1), gid: gid, width: int(advance) / 64}
	}
	var lines []string
	for _, paragraph := range paragraphs {
		paragraph = strings.ReplaceAll(strings.ReplaceAll(paragraph, "\r\n", "\n"), "\r", "\n")
		paragraph = strings.ReplaceAll(paragraph, "\t", "    ")
		for _, raw := range strings.Split(paragraph, "\n") {
			if raw == "" {
				lines = append(lines, "")
				continue
			}
			var line strings.Builder
			width := 0.0
			for _, r := range raw {
				w := float64(glyphs[r].width) * 11 / 1000
				if width+w > 499 && line.Len() > 0 {
					lines = append(lines, line.String())
					line.Reset()
					width = 0
				}
				line.WriteRune(r)
				width += w
			}
			lines = append(lines, line.String())
		}
	}
	const perPage = 47
	pageCount := (len(lines) + perPage - 1) / perPage
	if pageCount == 0 {
		pageCount = 1
	}
	encode := func(text string) string {
		b := make([]byte, 0, len(text)*2)
		for _, r := range text {
			g := glyphs[r]
			b = binary.BigEndian.AppendUint16(b, g.cid)
		}
		return strings.ToUpper(hex.EncodeToString(b))
	}
	stream := func(data []byte, extra string) []byte {
		return append(append([]byte(fmt.Sprintf("<< /Length %d %s >>\nstream\n", len(data), extra)), data...), []byte("\nendstream")...)
	}
	objects := make([][]byte, 8+pageCount*2)
	objects[0] = []byte("<< /Type /Catalog /Pages 2 0 R >>")
	var kids strings.Builder
	for p := 0; p < pageCount; p++ {
		fmt.Fprintf(&kids, "%d 0 R ", 9+p*2)
	}
	objects[1] = []byte(fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", pageCount, kids.String()))
	objects[2] = []byte("<< /Type /Font /Subtype /Type0 /BaseFont /GoRegular /Encoding /Identity-H /DescendantFonts [4 0 R] /ToUnicode 7 0 R >>")
	var widths strings.Builder
	for _, r := range runes {
		g := glyphs[r]
		fmt.Fprintf(&widths, "%d [%d] ", g.cid, g.width)
	}
	objects[3] = []byte(fmt.Sprintf("<< /Type /Font /Subtype /CIDFontType2 /BaseFont /GoRegular /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> /FontDescriptor 5 0 R /DW 1000 /W [%s] /CIDToGIDMap 8 0 R >>", widths.String()))
	metrics, err := face.Metrics(&buf, fixed.I(1000), font.HintingNone)
	if err != nil {
		return nil, err
	}
	objects[4] = []byte(fmt.Sprintf("<< /Type /FontDescriptor /FontName /GoRegular /Flags 32 /FontBBox [-1000 -1000 2000 2000] /ItalicAngle 0 /Ascent %d /Descent %d /CapHeight %d /StemV 80 /FontFile2 6 0 R >>", int(metrics.Ascent)/64, -int(metrics.Descent)/64, int(metrics.CapHeight)/64))
	objects[5] = stream(goregular.TTF, fmt.Sprintf("/Length1 %d", len(goregular.TTF)))
	var cmap strings.Builder
	cmap.WriteString("/CIDInit /ProcSet findresource begin\n12 dict begin\nbegincmap\n/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def\n/CMapName /GoRegular-UCS def\n/CMapType 2 def\n1 begincodespacerange\n<0000> <FFFF>\nendcodespacerange\n")
	for start := 0; start < len(runes); start += 100 {
		end := start + 100
		if end > len(runes) {
			end = len(runes)
		}
		fmt.Fprintf(&cmap, "%d beginbfchar\n", end-start)
		for _, r := range runes[start:end] {
			utf := utf16.Encode([]rune{r})
			var units strings.Builder
			for _, u := range utf {
				fmt.Fprintf(&units, "%04X", u)
			}
			fmt.Fprintf(&cmap, "<%04X> <%s>\n", glyphs[r].cid, units.String())
		}
		cmap.WriteString("endbfchar\n")
	}
	cmap.WriteString("endcmap\nCMapName currentdict /CMap defineresource pop\nend\nend\n")
	objects[6] = stream([]byte(cmap.String()), "")
	cidMap := make([]byte, (len(runes)+1)*2)
	for _, r := range runes {
		g := glyphs[r]
		binary.BigEndian.PutUint16(cidMap[int(g.cid)*2:], uint16(g.gid))
	}
	objects[7] = stream(cidMap, "")
	for p := 0; p < pageCount; p++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		pageID := 9 + p*2
		objects[pageID-1] = []byte(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", pageID+1))
		var content strings.Builder
		content.WriteString("BT /F1 11 Tf 15 TL 48 794 Td\n")
		start := p * perPage
		end := start + perPage
		if end > len(lines) {
			end = len(lines)
		}
		for i, line := range lines[start:end] {
			if i > 0 {
				content.WriteString("T*\n")
			}
			fmt.Fprintf(&content, "<%s> Tj\n", encode(line))
		}
		content.WriteString("ET\n")
		fmt.Fprintf(&content, "BT /F1 9 Tf 48 28 Td <%s> Tj ET", encode(fmt.Sprintf("Página %d / %d", p+1, pageCount)))
		objects[pageID] = stream([]byte(content.String()), "")
	}
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.7\n%\xE2\xE3\xCF\xD3\n")
	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		offsets[i+1] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n", i+1)
		pdf.Write(object)
		pdf.WriteString("\nendobj\n")
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	if pdf.Len() > MaxPDFBytes {
		return nil, ErrInvalid
	}
	return pdf.Bytes(), nil
}
