package contracttemplate

import (
	"strings"
	"testing"
)

func TestContractTemplateCanonicalValidation(t *testing.T) {
	base := []Section{{Key: "uso", Title: "Uso permitido", Body: "Mantener limpio.\nRespetar horarios."}}
	a, hash, err := CanonicalSections(base)
	if err != nil || len(hash) != 64 {
		t.Fatalf("valid clauses: %v", err)
	}
	b, again, _ := CanonicalSections(base)
	if string(a) != string(b) || hash != again {
		t.Fatal("unstable canonicalization")
	}
	for name, sections := range map[string][]Section{
		"empty":                nil,
		"duplicate":            {base[0], base[0]},
		"html":                 {{Key: "uso", Title: "Uso", Body: "<script>alert(1)</script>"}},
		"bidi":                 {{Key: "uso", Title: "Uso", Body: "Reglas\u202e ocultas"}},
		"control":              {{Key: "uso", Title: "Uso", Body: "Reglas\x00"}},
		"reserved placeholder": {{Key: "{{rut}}", Title: "Uso", Body: "Reglas"}},
		"too long":             {{Key: "uso", Title: "Uso", Body: strings.Repeat("á", 4001)}},
		"blank":                {{Key: "uso", Title: "Uso", Body: " "}},
		"email":                {{Key: "uso", Title: "Uso", Body: "Contacta a anfitrion@example.cl"}},
		"phone":                {{Key: "uso", Title: "Uso", Body: "Llama al +56 9 1234 5678"}},
		"link":                 {{Key: "uso", Title: "Uso", Body: "Ver https://example.cl"}},
		"bare domain":          {{Key: "uso", Title: "Uso", Body: "Ver anfitrion.testingpage.store"}},
		"handle":               {{Key: "uso", Title: "Uso", Body: "Contacta @anfitrion"}},
		"full width email":     {{Key: "uso", Title: "Uso", Body: "Contacta ａｎｆｉｔｒｉｏｎ＠ｅｘａｍｐｌｅ．ｃｌ"}},
		"zero width":           {{Key: "uso", Title: "Uso", Body: "anfitrion\u200b@example.cl"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := CanonicalSections(sections); err == nil {
				t.Fatal("accepted invalid clauses")
			}
		})
	}
}

func TestContractTemplateCanonicalHashIncludesOrderAndContent(t *testing.T) {
	a := []Section{{Key: "uno", Title: "Uno", Body: "Primero"}, {Key: "dos", Title: "Dos", Body: "Segundo"}}
	_, h, _ := CanonicalSections(a)
	_, reordered, _ := CanonicalSections([]Section{a[1], a[0]})
	if h == reordered {
		t.Fatal("order omitted from digest")
	}
	a[0].Body = "Distinto"
	_, changed, _ := CanonicalSections(a)
	if h == changed {
		t.Fatal("clauses omitted from digest")
	}
}
