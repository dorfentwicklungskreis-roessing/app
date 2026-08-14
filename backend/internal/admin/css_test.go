package admin

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// eigeneHaken sind Klassennamen, die nur als Test- und JavaScript-Anker
// dienen und bewusst kein Styling haben.
var eigeneHaken = map[string]bool{
	"erledigt-melden":    true,
	"aufgabe-bearbeiten": true,
	"aufgabe-loeschen":   true,
	// Kennzeichnet Angaben, die nur Verwaltende sehen. Die Optik macht das
	// daneben stehende badge — die Klasse ist reiner Testanker.
	"nur-verwaltung": true,
	// Anker für den Knopf, mit dem eine Zusage aufgehoben wird.
	"zusage-aufheben": true,
}

var (
	klassenAttribut = regexp.MustCompile(`class="([^"]*)"`)
	// Template-Aktionen werden vor der Prüfung durch ein Leerzeichen ersetzt.
	// Dadurch bleiben die literalen Klassennamen drumherum erhalten, ohne dass
	// Bruchstücke der Aktionen ("eq", ".Nav") als Klassen gezählt werden.
	templateAktion = regexp.MustCompile(`\{\{[\s\S]*?\}\}`)
)

// TestNurVorhandeneCSSKlassen stellt sicher, dass die Templates ausschließlich
// Klassen verwenden, die im gebauten CSS wirklich existieren. Ohne diese Probe
// bleiben Tippfehler und Klassen aus älteren DaisyUI-Versionen (z.B. das in
// DaisyUI 5 entfernte "form-control") unbemerkt: Der Browser ignoriert sie
// stillschweigend und die Seite sieht nur falsch aus.
func TestNurVorhandeneCSSKlassen(t *testing.T) {
	css, err := staticFS.ReadFile("static/app.css")
	if err != nil {
		t.Fatalf("app.css nicht lesbar: %v", err)
	}
	stil := string(css)

	dateien, err := templatesFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("Templates nicht lesbar: %v", err)
	}

	fehlend := map[string][]string{}
	for _, d := range dateien {
		roh, err := templatesFS.ReadFile("templates/" + d.Name())
		if err != nil {
			t.Fatalf("%s: %v", d.Name(), err)
		}
		ohneAktionen := templateAktion.ReplaceAllString(string(roh), " ")
		for _, treffer := range klassenAttribut.FindAllStringSubmatch(ohneAktionen, -1) {
			for _, klasse := range strings.Fields(treffer[1]) {
				if eigeneHaken[klasse] {
					continue
				}
				if !imStil(stil, klasse) {
					fehlend[klasse] = append(fehlend[klasse], d.Name())
				}
			}
		}
	}
	if len(fehlend) == 0 {
		return
	}
	namen := make([]string, 0, len(fehlend))
	for k := range fehlend {
		namen = append(namen, k)
	}
	sort.Strings(namen)
	for _, k := range namen {
		t.Errorf("Klasse %q gibt es im gebauten CSS nicht (verwendet in %s)", k, strings.Join(fehlend[k], ", "))
	}
}

// imStil prüft, ob das CSS einen Selektor für diese Klasse enthält.
// Tailwind escapt Sonderzeichen im Selektor, deshalb der gleiche Escape hier.
func imStil(stil, klasse string) bool {
	escaped := regexp.MustCompile(`([:./\[\]%])`).ReplaceAllString(klasse, `\$1`)
	return strings.Contains(stil, "."+escaped)
}
