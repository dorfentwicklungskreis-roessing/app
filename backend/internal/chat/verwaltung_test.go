package chat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/mitglied"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// sitzung baut die Sicht einer Person genau so, wie der Handler sie baut —
// über denselben Verifier und dieselbe Mitgliedschaftsquelle.
func (dd *dorf) sitzung(t *testing.T, token string) Sitzung {
	t.Helper()
	nutzer, err := (auth.InsecureDevVerifier{}).Verify(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	return Sitzung{
		DB:      dd.DB,
		Now:     jetzt,
		Zugriff: mitglied.Zugriff(context.Background(), mitglied.DevQuelle{}, nutzer),
		Nutzer:  nutzer,
	}
}

// rufeWerkzeug führt ein Werkzeug unmittelbar aus — ohne den Umweg über ein
// Modell, weil hier die Regel geprüft wird und nicht das Gespräch.
func rufeWerkzeug(t *testing.T, s Sitzung, name string, args map[string]any) (any, error) {
	t.Helper()
	roh, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range Werkzeuge() {
		if w.Name == name {
			return w.Handler(roh, s)
		}
	}
	t.Fatalf("Werkzeug %q gibt es nicht", name)
	return nil, nil
}

// --- Was am MCP-Endpunkt geht, geht auch hier --------------------------------

// Der Chat soll können, was der Betreiber heute über den MCP-Endpunkt macht.
// Bleibt dort ein Werkzeug übrig, das hier keine Entsprechung hat, fällt es
// hier auf und nicht erst dem Betreiber.
func TestChatKannWasDerMcpEndpunktKann(t *testing.T) {
	vorhanden := map[string]bool{}
	for _, w := range Werkzeuge() {
		vorhanden[w.Name] = true
	}
	// Namensgleich, wo die Sache dieselbe ist; die Abweichungen stehen
	// daneben, damit sie eine Entscheidung bleiben und kein Versehen.
	erwartet := map[string]string{
		"orte_liste":               "orte_liste",
		"ort_anlegen":              "ort_anlegen",
		"ort_aendern":              "ort_aendern",
		"ort_loeschen":             "ort_loeschen",
		"aufgabe_anlegen":          "aufgabe_anlegen",
		"aufgabe_aendern":          "aufgabe_aendern",
		"aufgabe_loeschen":         "aufgabe_loeschen",
		"erledigung_melden":        "erledigung_melden",
		"erledigung_zuruecknehmen": "erledigung_zuruecknehmen",
		"rangliste":                "rangliste",
		"vergabe_stand":            "vergabe_stand",
		"zusage_aufheben":          "zusage_aufheben",
		"hitzefaktor_setzen":       "hitzefaktor_setzen",
		"ideen_liste":              "ideen_liste",
		"idee_status_setzen":       "idee_status_setzen",
	}
	for mcpName, chatName := range erwartet {
		if !vorhanden[chatName] {
			t.Errorf("das MCP-Werkzeug %q hat im Chat keine Entsprechung (erwartet: %q)",
				mcpName, chatName)
		}
	}
}

// --- Ändern ------------------------------------------------------------------

// Wer nicht verwaltet, ändert auch nichts — die Absage nennt den Träger, den
// diese Person ohnehin sehen darf.
func TestOrtAendernNurMitVerwaltungsrecht(t *testing.T) {
	dd := neuesDorf(t)
	_, err := rufeWerkzeug(t, dd.sitzung(t, tokenMitglied), "ort_aendern",
		map[string]any{"id": dd.Offen.ID, "name": "Umbenannt"})
	if err == nil {
		t.Fatal("ein einfaches Mitglied darf keinen Ort umbenennen")
	}
	ort, _ := dd.DB.GetPlace(dd.Offen.ID)
	if ort.Name == "Umbenannt" {
		t.Fatal("der Ort wurde trotz fehlender Berechtigung geändert")
	}

	if _, err := rufeWerkzeug(t, dd.sitzung(t, tokenVorstand), "ort_aendern",
		map[string]any{"id": dd.Offen.ID, "name": "Umbenannt"}); err != nil {
		t.Fatalf("der Vorstand seines Trägers darf das: %v", err)
	}
	ort, _ = dd.DB.GetPlace(dd.Offen.ID)
	if ort.Name != "Umbenannt" {
		t.Fatalf("Name = %q, erwartet „Umbenannt“", ort.Name)
	}
}

// Eine Änderung ändert nur, wovon die Rede war. Sonst setzte „mach das
// Intervall auf 10 Tage“ nebenbei eine interne Aufgabe auf öffentlich.
func TestAendernLaesstUngenannteFelderInRuhe(t *testing.T) {
	dd := neuesDorf(t)
	s := dd.sitzung(t, tokenVorstand)
	if _, err := rufeWerkzeug(t, s, "aufgabe_aendern",
		map[string]any{"id": dd.Internes.ID, "intervallTage": 10.0, "rotNachTagen": 20.0}); err != nil {
		t.Fatal(err)
	}
	aufgabe, err := dd.DB.GetTask(dd.Internes.ID)
	if err != nil {
		t.Fatal(err)
	}
	if aufgabe.Sichtbarkeit != model.AufgabeNurMitglieder {
		t.Fatalf("Sichtbarkeit = %q — eine Intervalländerung darf sie nicht anfassen",
			aufgabe.Sichtbarkeit)
	}
	if aufgabe.Title != dd.Internes.Title {
		t.Fatalf("Titel = %q, erwartet %q", aufgabe.Title, dd.Internes.Title)
	}
	if aufgabe.IntervalDays != 10 {
		t.Fatalf("intervalDays = %v, erwartet 10", aufgabe.IntervalDays)
	}
}

// Auch über die bloße Nummer kommt niemand an eine interne Aufgabe heran —
// und die Absage lautet wie bei einer, die es wirklich nicht gibt.
func TestInterneAufgabeLaesstSichNichtUeberDieNummerAendern(t *testing.T) {
	dd := neuesDorf(t)
	for _, fall := range []struct {
		werkzeug string
		args     map[string]any
	}{
		{"aufgabe_aendern", map[string]any{"id": dd.Internes.ID, "titel": "Umbenannt"}},
		{"aufgabe_loeschen", map[string]any{"id": dd.Internes.ID}},
		{"vergabe_stand", map[string]any{"aufgabeId": dd.Internes.ID}},
		{"zusage_aufheben", map[string]any{"aufgabeId": dd.Internes.ID}},
		{"ort_aendern", map[string]any{"id": dd.Intern.ID, "name": "Umbenannt"}},
		{"ort_loeschen", map[string]any{"id": dd.Intern.ID}},
	} {
		t.Run(fall.werkzeug, func(t *testing.T) {
			_, err := rufeWerkzeug(t, dd.sitzung(t, tokenNachbarin), fall.werkzeug, fall.args)
			if err == nil {
				t.Fatal("das hätte nicht gehen dürfen")
			}
			if !strings.Contains(err.Error(), "gibt es nicht") {
				t.Fatalf("Absage = %q — sie muss klingen wie „gibt es nicht“, "+
					"sonst verrät sie die interne Aufgabe", err)
			}
		})
	}
	if aufgabe, err := dd.DB.GetTask(dd.Internes.ID); err != nil || aufgabe.Title != dd.Internes.Title {
		t.Fatal("die interne Aufgabe wurde trotzdem angefasst")
	}
}

// Löschen geht — aber nur mit Recht, und dann wirklich.
func TestAufgabeLoeschen(t *testing.T) {
	dd := neuesDorf(t)
	if _, err := rufeWerkzeug(t, dd.sitzung(t, tokenMitglied), "aufgabe_loeschen",
		map[string]any{"id": dd.Giessen.ID}); err == nil {
		t.Fatal("ein einfaches Mitglied darf keine Aufgabe löschen")
	}
	if _, err := dd.DB.GetTask(dd.Giessen.ID); err != nil {
		t.Fatal("die Aufgabe wurde trotz fehlender Berechtigung gelöscht")
	}
	if _, err := rufeWerkzeug(t, dd.sitzung(t, tokenVorstand), "aufgabe_loeschen",
		map[string]any{"id": dd.Giessen.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := dd.DB.GetTask(dd.Giessen.ID); err == nil {
		t.Fatal("die Aufgabe steht noch da")
	}
}

// --- Erledigungen zurücknehmen ----------------------------------------------

// Die eigene Meldung nimmt jede Person zurück, die fremde nur die Verwaltung
// — dieselbe Regel wie in der REST-API.
func TestErledigungZuruecknehmen(t *testing.T) {
	dd := neuesDorf(t)
	nachbarin := dd.sitzung(t, tokenNachbarin)
	if _, err := rufeWerkzeug(t, nachbarin, "erledigung_melden",
		map[string]any{"aufgabeId": dd.Giessen.ID}); err != nil {
		t.Fatal(err)
	}
	meldungen, err := dd.DB.ListCompletions(dd.Giessen.ID, 10)
	if err != nil || len(meldungen) != 1 {
		t.Fatalf("%d Meldungen, erwartet 1 (%v)", len(meldungen), err)
	}
	id := meldungen[0].ID

	// Eine fremde Meldung nimmt niemand einfach so zurück.
	if _, err := rufeWerkzeug(t, dd.sitzung(t, tokenMitglied), "erledigung_zuruecknehmen",
		map[string]any{"id": id}); err == nil {
		t.Fatal("eine fremde Meldung darf ein einfaches Mitglied nicht zurücknehmen")
	}
	// Die eigene schon.
	if _, err := rufeWerkzeug(t, nachbarin, "erledigung_zuruecknehmen",
		map[string]any{"id": id}); err != nil {
		t.Fatal(err)
	}
	meldungen, _ = dd.DB.ListCompletions(dd.Giessen.ID, 10)
	if len(meldungen) != 0 {
		t.Fatalf("%d Meldungen, erwartet 0", len(meldungen))
	}
}

// --- Was dem ganzen Dorf gehört ---------------------------------------------

// Hitzefaktor und Ideen gehören nicht einem Verein, sondern der Plattform.
// Ein Vereinsvorstand ist deshalb hier so weit draußen wie jeder andere.
func TestDorfweitesNurFuerDenBetreiber(t *testing.T) {
	dd := neuesDorf(t)
	faelle := []struct {
		werkzeug string
		args     map[string]any
	}{
		{"hitzefaktor_setzen", map[string]any{"faktor": 0.5}},
		{"ideen_liste", map[string]any{}},
		{"idee_status_setzen", map[string]any{"id": 1, "status": "gelesen"}},
	}
	for _, fall := range faelle {
		t.Run(fall.werkzeug+"/vorstand", func(t *testing.T) {
			if _, err := rufeWerkzeug(t, dd.sitzung(t, tokenVorstand), fall.werkzeug, fall.args); err == nil {
				t.Fatal("das ist Sache des Betreibers, nicht eines Vereinsvorstands")
			}
		})
	}
	if faktor, _ := dd.DB.WateringFactor(); faktor == 0.5 {
		t.Fatal("der Hitzefaktor wurde trotzdem gesetzt")
	}
	if _, err := rufeWerkzeug(t, dd.sitzung(t, tokenBetreiber), "hitzefaktor_setzen",
		map[string]any{"faktor": 0.5}); err != nil {
		t.Fatal(err)
	}
	faktor, err := dd.DB.WateringFactor()
	if err != nil || faktor != 0.5 {
		t.Fatalf("Hitzefaktor = %v (%v), erwartet 0.5", faktor, err)
	}
	if _, err := rufeWerkzeug(t, dd.sitzung(t, tokenBetreiber), "ideen_liste",
		map[string]any{}); err != nil {
		t.Fatalf("der Betreiber darf die Ideen sehen: %v", err)
	}
}

// --- Die Wache über den Tests -----------------------------------------------

// Kein Test dieses Pakets ruft Anthropic an. Das steht nicht nur in der
// Hausordnung, sondern wird hier geprüft: Ein Test, der versehentlich den
// echten Endpunkt oder den Zugang aus der Umgebung benutzte, würde Geld
// kosten, wäre unzuverlässig — und fiele sonst erst in der Rechnung auf.
func TestKeinTestRuftAnthropicAn(t *testing.T) {
	dateien, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(dateien) == 0 {
		t.Fatal("keine Testdateien gefunden — die Wache liefe ins Leere")
	}
	verboten := []struct {
		muster string
		grund  string
	}{
		{"api.anthropic.com", "der echte Endpunkt der Claude-API"},
		{"AnthropicBasis", "die Konstante mit dem echten Endpunkt"},
		{"AnbieterAusUmgebung", "der Zugang aus der Umgebung — mit gesetztem " +
			"ANTHROPIC_API_KEY ginge die Anfrage wirklich hinaus"},
		{"AusUmgebung(", "dasselbe über die Einstellungen"},
	}
	for _, datei := range dateien {
		if datei == "verwaltung_test.go" {
			// Die Wache selbst nennt die Muster naturgemäß.
			continue
		}
		roh, err := os.ReadFile(datei)
		if err != nil {
			t.Fatal(err)
		}
		for _, v := range verboten {
			if strings.Contains(string(roh), v.muster) {
				t.Errorf("%s benutzt %q (%s) — Tests laufen ausschließlich gegen "+
					"das lokale Modell", datei, v.muster, v.grund)
			}
		}
	}
}
