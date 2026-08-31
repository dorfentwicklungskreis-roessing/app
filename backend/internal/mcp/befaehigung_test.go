package mcp

import (
	"encoding/json"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Einweisungen über MCP — bis dahin gab es das Feld befaehigungId an
// aufgabe_anlegen, aber keinen Weg, an eine Kennung zu kommen.

func TestBefaehigungsWerkzeugeSindAngemeldet(t *testing.T) {
	ts := newTestServer(t)
	out := rpc(t, ts, "admin-jwt", "tools/list", nil)
	namen := map[string]bool{}
	for _, roh := range out["result"].(map[string]any)["tools"].([]any) {
		namen[roh.(map[string]any)["name"].(string)] = true
	}
	for _, name := range []string{
		"befaehigungen_liste", "befaehigung_anlegen",
		"befaehigung_aendern", "befaehigung_erteilen",
	} {
		if !namen[name] {
			t.Errorf("Werkzeug %q fehlt", name)
		}
	}
}

// Der Fall, für den das gebaut wurde: das Beet vor dem
// Dorfgemeinschaftshaus, gejätet nur mit Einweisung.
func TestEinweisungAnlegenErteilenUndAnEineAufgabeHaengen(t *testing.T) {
	ts, d := serverMitDB(t)

	text, _ := callTool(t, ts, "traeger_anlegen", map[string]any{
		"name": "AK 2 Umwelt und Natur", "projektId": "388659726272954563",
	})
	var ak model.Traeger
	if err := json.Unmarshal([]byte(text), &ak); err != nil {
		t.Fatalf("Träger nicht lesbar: %v — %s", err, text)
	}

	text, fehler := callTool(t, ts, "befaehigung_anlegen", map[string]any{
		"traegerId": ak.ID, "name": "Einweisung Jäten",
		"beschreibung": "Was Beikraut ist und was stehen bleibt.",
	})
	if fehler {
		t.Fatalf("befaehigung_anlegen: %s", text)
	}
	var ein model.Befaehigung
	if err := json.Unmarshal([]byte(text), &ein); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v — %s", err, text)
	}
	if ein.TraegerID != ak.ID {
		t.Fatalf("Einweisung hängt am falschen Träger: %+v", ein)
	}

	// Ohne Einweisung darf niemand zusagen — das ist der Zweck.
	if d.HatBefaehigung("erna", ein.ID) {
		t.Fatal("Erna hat die Einweisung, ohne sie bekommen zu haben")
	}

	text, fehler = callTool(t, ts, "befaehigung_erteilen", map[string]any{
		"befaehigungId": ein.ID, "userSub": "erna",
		"notiz": "Am 12.04. von Olaf eingewiesen.",
	})
	if fehler {
		t.Fatalf("befaehigung_erteilen: %s", text)
	}
	if !d.HatBefaehigung("erna", ein.ID) {
		t.Fatal("Einweisung wurde nicht wirksam")
	}

	// Zweimal erteilen ist kein Fehler und verdoppelt nichts.
	if _, fehler := callTool(t, ts, "befaehigung_erteilen", map[string]any{
		"befaehigungId": ein.ID, "userSub": "erna",
	}); fehler {
		t.Error("erneutes Erteilen wurde abgewiesen")
	}
	if !d.HatBefaehigung("erna", ein.ID) {
		t.Fatal("erneutes Erteilen hat die Einweisung entfernt")
	}

	// Die Liste nennt den Träger beim Namen — eine Kennung allein hilft dem
	// nicht weiter, der sie vorgelesen bekommt.
	text, fehler = callTool(t, ts, "befaehigungen_liste", map[string]any{"traegerId": ak.ID})
	if fehler {
		t.Fatalf("befaehigungen_liste: %s", text)
	}
	var liste []struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		TraegerName string `json:"traegerName"`
	}
	if err := json.Unmarshal([]byte(text), &liste); err != nil {
		t.Fatalf("Liste nicht lesbar: %v — %s", err, text)
	}
	if len(liste) != 1 || liste[0].ID != ein.ID ||
		liste[0].TraegerName != "AK 2 Umwelt und Natur" {
		t.Fatalf("Liste unbrauchbar: %+v", liste)
	}
}

func TestBefaehigungWeistUnsinnAb(t *testing.T) {
	ts, _ := serverMitDB(t)

	if _, fehler := callTool(t, ts, "befaehigung_anlegen", map[string]any{
		"traegerId": 99999, "name": "Motorsense",
	}); !fehler {
		t.Error("Einweisung für einen Träger angelegt, den es nicht gibt")
	}

	text, _ := callTool(t, ts, "traeger_anlegen", map[string]any{"name": "DRK"})
	var tr model.Traeger
	if err := json.Unmarshal([]byte(text), &tr); err != nil {
		t.Fatal(err)
	}
	if _, fehler := callTool(t, ts, "befaehigung_anlegen", map[string]any{
		"traegerId": tr.ID, "name": "   ",
	}); !fehler {
		t.Error("Einweisung ohne Namen wurde angenommen")
	}

	if _, fehler := callTool(t, ts, "befaehigung_erteilen", map[string]any{
		"befaehigungId": 99999, "userSub": "erna",
	}); !fehler {
		t.Error("unbekannte Einweisung wurde erteilt")
	}

	text, _ = callTool(t, ts, "befaehigung_anlegen", map[string]any{
		"traegerId": tr.ID, "name": "Erste Hilfe",
	})
	var ein model.Befaehigung
	if err := json.Unmarshal([]byte(text), &ein); err != nil {
		t.Fatal(err)
	}
	if _, fehler := callTool(t, ts, "befaehigung_erteilen", map[string]any{
		"befaehigungId": ein.ID, "userSub": "",
	}); !fehler {
		t.Error("Einweisung ohne Person wurde erteilt")
	}
	if _, fehler := callTool(t, ts, "befaehigung_erteilen", map[string]any{
		"befaehigungId": ein.ID, "userSub": "erna", "status": "vielleicht",
	}); !fehler {
		t.Error("unsinniger Status wurde angenommen")
	}
}

// Mitglieder haben am MCP-Endpoint nichts verloren — auch nicht hier.
func TestBefaehigungsWerkzeugeNurFuerAdmins(t *testing.T) {
	ts := newTestServer(t)
	resp := rpcRaw(t, ts, "member-jwt", "tools/call",
		map[string]any{"name": "befaehigungen_liste", "arguments": map[string]any{}})
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("Mitglied bekommt Status %d, erwartet 403", resp.StatusCode)
	}
}
