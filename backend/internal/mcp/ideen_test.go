package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Aus Claude heraus sollen sich die Wünsche aus dem Dorf ansehen und
// einordnen lassen — ohne dafür die Verwaltung im Browser zu öffnen.

func ideeAnlegen(t *testing.T, d *db.DB, wunsch string, status model.IdeeStatus) model.Idee {
	t.Helper()
	i := model.Idee{
		Name: "Erna", Email: "erna@example.org", Wunsch: wunsch,
		Quelle: model.IdeeQuelleWebsite, Status: status,
		CreatedAt: vergabeJetzt,
	}
	if err := d.InsertIdee(&i); err != nil {
		t.Fatal(err)
	}
	return i
}

func TestIdeenWerkzeugeSindAngemeldet(t *testing.T) {
	ts := newTestServer(t)
	out := rpc(t, ts, "admin-jwt", "tools/list", nil)
	namen := map[string]bool{}
	for _, roh := range out["result"].(map[string]any)["tools"].([]any) {
		namen[roh.(map[string]any)["name"].(string)] = true
	}
	for _, name := range []string{"ideen_liste", "idee_status_setzen"} {
		if !namen[name] {
			t.Errorf("Werkzeug %q fehlt", name)
		}
	}
	// Bestehende Namen dürfen sich nicht ändern.
	for _, name := range []string{"orte_liste", "ort_anlegen", "erledigung_melden", "rangliste"} {
		if !namen[name] {
			t.Errorf("bestehendes Werkzeug %q ist verschwunden", name)
		}
	}
}

func TestIdeenListeMitStatusfilter(t *testing.T) {
	ts, d := serverMitDB(t)
	ideeAnlegen(t, d, "Ein Mitfahrbrett nach Hildesheim.", model.IdeeNeu)
	ideeAnlegen(t, d, "Ein Vertretungsplan für die Grundschule.", model.IdeeUmgesetzt)

	text, fehler := callTool(t, ts, "ideen_liste", map[string]any{})
	if fehler {
		t.Fatalf("ideen_liste: %s", text)
	}
	if !strings.Contains(text, "Mitfahrbrett") || !strings.Contains(text, "Vertretungsplan") {
		t.Fatalf("Liste unvollständig: %s", text)
	}

	text, fehler = callTool(t, ts, "ideen_liste", map[string]any{"status": "neu"})
	if fehler {
		t.Fatalf("ideen_liste gefiltert: %s", text)
	}
	if strings.Contains(text, "Vertretungsplan") {
		t.Errorf("Filter wirkt nicht: %s", text)
	}
	if !strings.Contains(text, "Mitfahrbrett") {
		t.Errorf("gefilterte Liste ist leer: %s", text)
	}

	// Ein unbekannter Status ist ein Fehler, kein stilles Nichts.
	if _, fehler := callTool(t, ts, "ideen_liste", map[string]any{"status": "vielleicht"}); !fehler {
		t.Error("unbekannter Status wurde akzeptiert")
	}
}

func TestIdeeStatusSetzen(t *testing.T) {
	ts, d := serverMitDB(t)
	idee := ideeAnlegen(t, d, "Ein Mitfahrbrett nach Hildesheim.", model.IdeeNeu)

	text, fehler := callTool(t, ts, "idee_status_setzen", map[string]any{
		"id": idee.ID, "status": "umgesetzt", "notiz": "Kommt mit Version 0.3.",
	})
	if fehler {
		t.Fatalf("idee_status_setzen: %s", text)
	}
	var got model.Idee
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v — %s", err, text)
	}
	if got.Status != model.IdeeUmgesetzt || got.Notiz != "Kommt mit Version 0.3." {
		t.Fatalf("nicht übernommen: %+v", got)
	}
	nach, _ := d.GetIdee(idee.ID)
	if nach.Status != model.IdeeUmgesetzt {
		t.Fatalf("in der Datenbank steht %q", nach.Status)
	}

	if _, fehler := callTool(t, ts, "idee_status_setzen", map[string]any{"id": idee.ID, "status": "vielleicht"}); !fehler {
		t.Error("unsinniger Status wurde akzeptiert")
	}
	if _, fehler := callTool(t, ts, "idee_status_setzen", map[string]any{"id": 99999, "status": "gelesen"}); !fehler {
		t.Error("unbekannte ID wurde akzeptiert")
	}
}

// Mitglieder haben am MCP-Endpoint nichts verloren — auch nicht bei den Ideen.
func TestIdeenWerkzeugeNurFuerAdmins(t *testing.T) {
	ts := newTestServer(t)
	resp := rpcRaw(t, ts, "member-jwt", "tools/call",
		map[string]any{"name": "ideen_liste", "arguments": map[string]any{}})
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("Mitglied bekommt Status %d, erwartet 403", resp.StatusCode)
	}
}

// „Was ist reingekommen?" soll eine brauchbare Antwort geben, ohne dass
// erst die ganze Liste durchgezählt werden muss.
func TestIdeenListeLiefertUeberblick(t *testing.T) {
	ts, d := serverMitDB(t)
	ideeAnlegen(t, d, "Ein Mitfahrbrett nach Hildesheim.", model.IdeeNeu)
	ideeAnlegen(t, d, "Ein Vertretungsplan für die Grundschule.", model.IdeeNeu)
	ideeAnlegen(t, d, "Ein Dorfkalender mit allen Terminen.", model.IdeeUmgesetzt)
	ausDerApp := model.Idee{
		Wunsch: "Erinnerungen bitte auch abends.", Quelle: model.IdeeQuelleApp,
		UserSub: "u1", Status: model.IdeeGelesen, CreatedAt: vergabeJetzt,
	}
	if err := d.InsertIdee(&ausDerApp); err != nil {
		t.Fatal(err)
	}

	text, fehler := callTool(t, ts, "ideen_liste", map[string]any{})
	if fehler {
		t.Fatalf("ideen_liste: %s", text)
	}
	var out struct {
		Ideen      []model.Idee `json:"ideen"`
		Ueberblick struct {
			Gesamt  int            `json:"gesamt"`
			JeStand map[string]int `json:"jeStand"`
			JeWeg   map[string]int `json:"jeWeg"`
			Neueste string         `json:"neueste"`
		} `json:"ueberblick"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v — %s", err, text)
	}
	u := out.Ueberblick
	if u.Gesamt != 4 {
		t.Errorf("gesamt = %d, erwartet 4", u.Gesamt)
	}
	if u.JeStand["neu"] != 2 || u.JeStand["umgesetzt"] != 1 || u.JeStand["gelesen"] != 1 {
		t.Errorf("jeStand = %v", u.JeStand)
	}
	if u.JeWeg["website"] != 3 || u.JeWeg["app"] != 1 {
		t.Errorf("jeWeg = %v", u.JeWeg)
	}
	if u.Neueste == "" {
		t.Error("neueste Einreichung fehlt")
	}

	// Auch bei gefilterter Liste beschreibt der Überblick den ganzen Bestand —
	// sonst wäre „wie viel ist offen?" nicht zu beantworten.
	text, _ = callTool(t, ts, "ideen_liste", map[string]any{"status": "umgesetzt"})
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Ideen) != 1 {
		t.Fatalf("gefilterte Liste: %d Einträge", len(out.Ideen))
	}
	if out.Ueberblick.Gesamt != 4 {
		t.Errorf("Überblick folgt dem Filter: gesamt = %d", out.Ueberblick.Gesamt)
	}
}
