package mcp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Auch aus Claude heraus muss sich eine einmalige Aufgabe anlegen lassen —
// sonst kennen die drei Wege (App, Web-Verwaltung, MCP) verschiedene
// Aufgabenarten.

func TestMcpLegtEinmaligeAufgabeAn(t *testing.T) {
	ts := newTestServer(t)

	text, fehler := callTool(t, ts, "ort_anlegen", map[string]any{
		"name": "Bahnhof", "kind": "sonstiges", "lat": 52.211, "lon": 9.87,
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

	text, fehler = callTool(t, ts, "aufgabe_anlegen", map[string]any{
		"placeId": ort.ID, "kind": "sonstiges", "title": "Zum Bahnhof fahren",
		"oneOff": true, "dueDate": "2026-08-20", "removeWhenDone": true,
	})
	if fehler {
		t.Fatalf("aufgabe_anlegen: %s", text)
	}
	var aufgabe struct {
		ID             int64  `json:"id"`
		OneOff         bool   `json:"oneOff"`
		RemoveWhenDone bool   `json:"removeWhenDone"`
		DueDate        string `json:"dueDate"`
	}
	if err := json.Unmarshal([]byte(text), &aufgabe); err != nil {
		t.Fatalf("Antwort unlesbar: %s", text)
	}
	if !aufgabe.OneOff || !aufgabe.RemoveWhenDone {
		t.Fatalf("Schalter nicht übernommen: %s", text)
	}
	if !strings.HasPrefix(aufgabe.DueDate, "2026-08-20") {
		t.Fatalf("Termin = %q, erwartet den 20.08.2026", aufgabe.DueDate)
	}
}

// Eine einmalige Aufgabe ohne Termin ist keine — der Fehler muss im Klartext
// zurückkommen, damit Claude ihn dem Menschen erklären kann.
func TestMcpWeistEinmaligOhneTerminAb(t *testing.T) {
	ts := newTestServer(t)
	text, _ := callTool(t, ts, "ort_anlegen", map[string]any{
		"name": "Bahnhof", "kind": "sonstiges", "lat": 52.211, "lon": 9.87,
	})
	var ort struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(text), &ort)

	text, fehler := callTool(t, ts, "aufgabe_anlegen", map[string]any{
		"placeId": ort.ID, "kind": "sonstiges", "title": "X", "oneOff": true,
	})
	if !fehler {
		t.Fatalf("wurde angenommen: %s", text)
	}
	if !strings.Contains(text, "dueDate") {
		t.Fatalf("Begründung nennt dueDate nicht: %s", text)
	}
}

// Das Tool-Schema muss die neuen Felder beschreiben, sonst rät Claude.
func TestMcpSchemaKenntEinmalig(t *testing.T) {
	ts := newTestServer(t)
	out := rpc(t, ts, "admin-jwt", "tools/list", nil)
	roh, _ := json.Marshal(out)
	for _, feld := range []string{"oneOff", "dueDate", "removeWhenDone"} {
		if !strings.Contains(string(roh), feld) {
			t.Errorf("Tool-Schema kennt %q nicht", feld)
		}
	}
}

// Auch über MCP darf eine zugesagte Aufgabe nicht wortlos verschwinden.
func TestMcpLoeschenBenachrichtigtDieZusagende(t *testing.T) {
	ts, d := newTestServerMitDB(t)
	jetzt := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	p := model.Place{Name: "Bahnhof", Kind: model.PlaceOther, Lat: 52.2, Lon: 9.87,
		Active: true, CreatedAt: jetzt}
	if err := d.InsertPlace(&p); err != nil {
		t.Fatal(err)
	}
	task := model.CareTask{PlaceID: p.ID, Kind: model.TaskWatering, IntervalDays: 7,
		RedAfterDays: 14, Active: true, CreatedAt: jetzt}
	if err := d.InsertTask(&task); err != nil {
		t.Fatal(err)
	}
	vorgang := model.Assignment{TaskID: task.ID, State: model.AssignmentOpen, CreatedAt: jetzt}
	if err := d.InsertAssignment(&vorgang); err != nil {
		t.Fatal(err)
	}
	ok, err := d.ClaimAssignment(vorgang.ID, "erna", "Erna", jetzt, jetzt.Add(24*time.Hour))
	if err != nil || !ok {
		t.Fatalf("Zusage: %v %v", ok, err)
	}

	text, fehler := callTool(t, ts, "aufgabe_loeschen", map[string]any{"id": task.ID})
	if fehler {
		t.Fatalf("aufgabe_loeschen: %s", text)
	}

	offen, err := d.OpenNotifications("erna")
	if err != nil {
		t.Fatal(err)
	}
	var hinweis *model.Notification
	for i := range offen {
		if offen[i].Kind == model.NotifyAssignmentDropped {
			hinweis = &offen[i]
		}
	}
	if hinweis == nil {
		t.Fatalf("Erna erfährt nichts vom Löschen: %+v", offen)
	}
	if hinweis.PlaceName != "Bahnhof" {
		t.Errorf("Hinweis nennt den Ort nicht: %+v", *hinweis)
	}
}
