package mcp

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// MCP ist der Verwaltungsweg (#62): Was sich in der Web-Verwaltung
// einstellen lässt, muss auch aus Claude heraus gehen — sonst ist die
// Jahreszeit einer Aufgabe (#78) wieder nur im Browser erreichbar.

// mcpOrt legt einen Ort an und liefert seine ID.
func mcpOrt(t *testing.T, ts *httptest.Server) int64 {
	t.Helper()
	text, fehler := callTool(t, ts, "ort_anlegen", map[string]any{
		"name": "Dorfgemeinschaftshaus", "kind": "beet", "lat": 52.211, "lon": 9.87,
	})
	if fehler {
		t.Fatalf("ort_anlegen: %s", text)
	}
	var ort struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(text), &ort); err != nil {
		t.Fatalf("Antwort unlesbar: %s", text)
	}
	return ort.ID
}

type mcpAufgabe struct {
	ID               int64 `json:"id"`
	SeasonStartMonth int   `json:"seasonStartMonth"`
	SeasonEndMonth   int   `json:"seasonEndMonth"`
}

func TestMcpLegtAufgabeMitJahreszeitAn(t *testing.T) {
	ts := newTestServer(t)
	ortID := mcpOrt(t, ts)

	text, fehler := callTool(t, ts, "aufgabe_anlegen", map[string]any{
		"placeId": ortID, "kind": "jaeten", "intervalDays": 56, "redAfterDays": 70,
		"seasonStartMonth": 4, "seasonEndMonth": 9,
	})
	if fehler {
		t.Fatalf("aufgabe_anlegen: %s", text)
	}
	var aufgabe mcpAufgabe
	if err := json.Unmarshal([]byte(text), &aufgabe); err != nil {
		t.Fatalf("Antwort unlesbar: %s", text)
	}
	if aufgabe.SeasonStartMonth != 4 || aufgabe.SeasonEndMonth != 9 {
		t.Fatalf("Jahreszeit = %d/%d, erwartet April bis September",
			aufgabe.SeasonStartMonth, aufgabe.SeasonEndMonth)
	}
}

func TestMcpAendertUndRaeumtDieJahreszeitAb(t *testing.T) {
	ts := newTestServer(t)
	ortID := mcpOrt(t, ts)
	text, _ := callTool(t, ts, "aufgabe_anlegen", map[string]any{
		"placeId": ortID, "kind": "jaeten", "intervalDays": 56, "redAfterDays": 70,
	})
	var aufgabe mcpAufgabe
	if err := json.Unmarshal([]byte(text), &aufgabe); err != nil {
		t.Fatalf("Antwort unlesbar: %s", text)
	}

	text, fehler := callTool(t, ts, "aufgabe_aendern", map[string]any{
		"id": aufgabe.ID, "seasonStartMonth": 11, "seasonEndMonth": 2,
	})
	if fehler {
		t.Fatalf("aufgabe_aendern: %s", text)
	}
	if err := json.Unmarshal([]byte(text), &aufgabe); err != nil {
		t.Fatalf("Antwort unlesbar: %s", text)
	}
	if aufgabe.SeasonStartMonth != 11 || aufgabe.SeasonEndMonth != 2 {
		t.Fatalf("Jahreszeit = %d/%d, erwartet November bis Februar",
			aufgabe.SeasonStartMonth, aufgabe.SeasonEndMonth)
	}

	// Eine Änderung an etwas anderem lässt sie stehen.
	text, _ = callTool(t, ts, "aufgabe_aendern", map[string]any{
		"id": aufgabe.ID, "title": "Beet jäten",
	})
	if err := json.Unmarshal([]byte(text), &aufgabe); err != nil {
		t.Fatalf("Antwort unlesbar: %s", text)
	}
	if aufgabe.SeasonStartMonth != 11 || aufgabe.SeasonEndMonth != 2 {
		t.Fatalf("Jahreszeit ging beim Umbenennen verloren: %d/%d",
			aufgabe.SeasonStartMonth, aufgabe.SeasonEndMonth)
	}

	// Und ausdrückliche 0 nimmt sie weg. Frische Struktur, weil eine
	// ganzjährige Aufgabe die Felder gar nicht mehr mitschickt.
	text, _ = callTool(t, ts, "aufgabe_aendern", map[string]any{
		"id": aufgabe.ID, "seasonStartMonth": 0, "seasonEndMonth": 0,
	})
	var ganzjaehrig mcpAufgabe
	if err := json.Unmarshal([]byte(text), &ganzjaehrig); err != nil {
		t.Fatalf("Antwort unlesbar: %s", text)
	}
	if ganzjaehrig.SeasonStartMonth != 0 || ganzjaehrig.SeasonEndMonth != 0 {
		t.Fatalf("Jahreszeit nicht abgeräumt: %d/%d",
			ganzjaehrig.SeasonStartMonth, ganzjaehrig.SeasonEndMonth)
	}
}

// Halbe Angaben und Unmögliches müssen im Klartext abgewiesen werden — damit
// Claude dem Menschen erklären kann, was fehlt.
func TestMcpWeistUnmoeglicheJahreszeitAb(t *testing.T) {
	ts := newTestServer(t)
	ortID := mcpOrt(t, ts)

	faelle := map[string]map[string]any{
		"nur Anfangsmonat": {"placeId": ortID, "kind": "jaeten",
			"intervalDays": 56, "redAfterDays": 70, "seasonStartMonth": 4},
		"Monat 13": {"placeId": ortID, "kind": "jaeten",
			"intervalDays": 56, "redAfterDays": 70,
			"seasonStartMonth": 13, "seasonEndMonth": 9},
		"einmalig mit Jahreszeit": {"placeId": ortID, "kind": "sonstiges",
			"oneOff": true, "dueDate": "2026-09-01",
			"seasonStartMonth": 4, "seasonEndMonth": 9},
	}
	for name, args := range faelle {
		t.Run(name, func(t *testing.T) {
			text, fehler := callTool(t, ts, "aufgabe_anlegen", args)
			if !fehler {
				t.Fatalf("wurde angenommen: %s", text)
			}
			if strings.TrimSpace(text) == "" {
				t.Error("ohne Begründung abgewiesen")
			}
		})
	}
}

// Die neuen Angaben müssen im Werkzeug-Schema stehen, sonst schickt Claude
// sie nie mit.
func TestMcpSchemaKenntDieJahreszeit(t *testing.T) {
	ts := newTestServer(t)
	out := rpc(t, ts, "admin-jwt", "tools/list", nil)
	roh, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, feld := range []string{"seasonStartMonth", "seasonEndMonth"} {
		if !strings.Contains(string(roh), feld) {
			t.Errorf("%q fehlt in der Werkzeugliste", feld)
		}
	}
}
