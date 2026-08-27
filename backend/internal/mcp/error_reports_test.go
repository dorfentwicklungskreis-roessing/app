package mcp

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Aus Claude heraus soll sich beantworten lassen: Was ist gerade kaputt? Ohne
// dafür die Verwaltung im Browser zu öffnen — genau dafür gibt es den
// MCP-Endpunkt.

func berichtAnlegen(t *testing.T, d *db.DB, meldung string,
	art model.ErrorReportKind, status model.ErrorReportStatus,
) model.ErrorReport {
	t.Helper()
	e := model.ErrorReport{
		Kind: art, Message: meldung, Detail: "HTTP 500 · GET /api/v1/places",
		Area: "Mithelfen", Platform: "ios", AppVersion: "0.1.10 (42)",
		OSVersion: "iOS 18.5", DeviceModel: "iPhone14,3",
		OccurredAt: vergabeJetzt, CreatedAt: vergabeJetzt,
		UserSub: "erna", UserName: "Erna", Status: status,
	}
	if err := d.InsertErrorReport(&e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestFehlerberichtWerkzeugeSindAngemeldet(t *testing.T) {
	ts := newTestServer(t)
	out := rpc(t, ts, "admin-jwt", "tools/list", nil)
	namen := map[string]bool{}
	for _, roh := range out["result"].(map[string]any)["tools"].([]any) {
		namen[roh.(map[string]any)["name"].(string)] = true
	}
	for _, name := range []string{"fehlerberichte_liste", "fehlerbericht_status_setzen"} {
		if !namen[name] {
			t.Errorf("Werkzeug %q fehlt", name)
		}
	}
	// Bestehende Namen dürfen sich nicht ändern.
	for _, name := range []string{"ideen_liste", "orte_liste"} {
		if !namen[name] {
			t.Errorf("bestehendes Werkzeug %q ist verschwunden", name)
		}
	}
}

func TestFehlerberichteListeMitUeberblick(t *testing.T) {
	ts, d := serverMitDB(t)
	berichtAnlegen(t, d, "Die App hat sich beendet.", model.ErrorReportCrash, model.ErrorReportNew)
	berichtAnlegen(t, d, "Der Server antwortet nicht.", model.ErrorReportServer, model.ErrorReportNew)
	berichtAnlegen(t, d, "Alter Fall.", model.ErrorReportNetwork, model.ErrorReportFixed)

	text, fehler := callTool(t, ts, "fehlerberichte_liste", map[string]any{})
	if fehler {
		t.Fatalf("fehlerberichte_liste: %s", text)
	}
	for _, muss := range []string{"beendet", "antwortet nicht", "Alter Fall"} {
		if !strings.Contains(text, muss) {
			t.Errorf("Liste enthält %q nicht", muss)
		}
	}

	var out struct {
		Ueberblick struct {
			Gesamt      int            `json:"gesamt"`
			Offen       int            `json:"offen"`
			JeArt       map[string]int `json:"jeArt"`
			JePlattform map[string]int `json:"jePlattform"`
			JeVersion   map[string]int `json:"jeVersion"`
			Neuester    string         `json:"neuester"`
		} `json:"ueberblick"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v", err)
	}
	if out.Ueberblick.Gesamt != 3 || out.Ueberblick.Offen != 2 {
		t.Fatalf("Überblick: gesamt %d, offen %d", out.Ueberblick.Gesamt, out.Ueberblick.Offen)
	}
	if out.Ueberblick.JeArt["crash"] != 1 || out.Ueberblick.JeArt["network"] != 1 {
		t.Fatalf("je Art: %v", out.Ueberblick.JeArt)
	}
	if out.Ueberblick.JePlattform["ios"] != 3 || out.Ueberblick.JePlattform["android"] != 0 {
		t.Fatalf("je Plattform: %v", out.Ueberblick.JePlattform)
	}
	if out.Ueberblick.JeVersion["0.1.10 (42)"] != 3 {
		t.Fatalf("je Version: %v", out.Ueberblick.JeVersion)
	}
	if out.Ueberblick.Neuester == "" {
		t.Error("neuester Bericht fehlt im Überblick")
	}
}

func TestFehlerberichteListeFiltertUndBeschreibtTrotzdemAlles(t *testing.T) {
	ts, d := serverMitDB(t)
	berichtAnlegen(t, d, "Die App hat sich beendet.", model.ErrorReportCrash, model.ErrorReportNew)
	berichtAnlegen(t, d, "Der Server antwortet nicht.", model.ErrorReportServer, model.ErrorReportFixed)

	text, fehler := callTool(t, ts, "fehlerberichte_liste", map[string]any{"art": "crash"})
	if fehler {
		t.Fatalf("gefilterte Liste: %s", text)
	}
	if strings.Contains(text, "antwortet nicht") {
		t.Error("Filter art=crash liefert auch Server-Fehler")
	}
	// Der Überblick beschreibt weiterhin den ganzen Bestand — sonst ließe
	// sich „wie viel ist noch offen?“ nicht beantworten.
	var out struct {
		Ueberblick struct {
			Gesamt int `json:"gesamt"`
			Offen  int `json:"offen"`
		} `json:"ueberblick"`
	}
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	if out.Ueberblick.Gesamt != 2 || out.Ueberblick.Offen != 1 {
		t.Fatalf("Überblick bei gefilterter Liste: %+v", out.Ueberblick)
	}

	// Unbekannte Werte werden mit einer verständlichen Begründung abgewiesen.
	text, fehler = callTool(t, ts, "fehlerberichte_liste", map[string]any{"status": "quatsch"})
	if !fehler || !strings.Contains(text, "status muss") {
		t.Fatalf("unbekannter Stand: %q (Fehler: %v)", text, fehler)
	}
}

func TestFehlerberichtStatusSetzen(t *testing.T) {
	ts, d := serverMitDB(t)
	bericht := berichtAnlegen(t, d, "Die App hat sich beendet.",
		model.ErrorReportCrash, model.ErrorReportNew)

	text, fehler := callTool(t, ts, "fehlerbericht_status_setzen", map[string]any{
		"id": bericht.ID, "status": "fixed", "notiz": "In 0.1.11 behoben.",
	})
	if fehler {
		t.Fatalf("Status setzen: %s", text)
	}
	wieder, err := d.GetErrorReport(bericht.ID)
	if err != nil {
		t.Fatal(err)
	}
	if wieder.Status != model.ErrorReportFixed || wieder.Note != "In 0.1.11 behoben." {
		t.Fatalf("nicht gespeichert: %+v", wieder)
	}
	// Der gemeldete Sachverhalt bleibt, wie er beobachtet wurde.
	if wieder.Message != bericht.Message || wieder.Detail != bericht.Detail {
		t.Fatalf("gemeldeter Sachverhalt verändert: %+v", wieder)
	}

	text, fehler = callTool(t, ts, "fehlerbericht_status_setzen", map[string]any{
		"id": int64(9999), "status": "fixed",
	})
	if !fehler || !strings.Contains(text, "nicht gefunden") {
		t.Fatalf("unbekannte ID: %q (Fehler: %v)", text, fehler)
	}
}

func TestFehlerberichteNurFuerAdmins(t *testing.T) {
	ts := newTestServer(t)
	// Ohne admin-Rolle kommt der Aufruf gar nicht erst bis zum Werkzeug —
	// der MCP-Endpunkt weist ihn vorher ab (siehe auth.go).
	resp := rpcRaw(t, ts, "member-jwt", "tools/call", map[string]any{
		"name": "fehlerberichte_liste", "arguments": map[string]any{},
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Mitglied bekommt HTTP %d, erwartet 403", resp.StatusCode)
	}
}
