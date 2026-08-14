package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/vergabe"
)

// Die Verwaltung soll auch aus Claude heraus sehen können, wie die Vergabe
// einer Aufgabe steht, und eine festhängende Zusage aufheben können.

var vergabeJetzt = time.Date(2026, 8, 12, 12, 0, 0, 0, model.Location())

// serverMitDB baut denselben MCP-Server wie newTestServer, gibt aber die
// Datenbank mit heraus — für Aufbauten, die es als Werkzeug nicht gibt
// (Anmeldungen, Takt der Vergabe).
func serverMitDB(t *testing.T) (*httptest.Server, *db.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s := New(d, stubVerifier{}, issuer, "https://api.example", "client-123")
	s.Now = func() time.Time { return vergabeJetzt }
	mux := http.NewServeMux()
	s.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, d
}

// aufbauMitAufgabe legt einen längst fälligen Gießplan an.
func aufbauMitAufgabe(t *testing.T, d *db.DB) model.CareTask {
	t.Helper()
	p := model.Place{Name: "Unter den Eichen — Kasten 1", Kind: model.PlaceFlowerbox,
		Lat: 52.211, Lon: 9.87, Active: true, CreatedAt: vergabeJetzt.AddDate(0, 0, -30)}
	if err := d.InsertPlace(&p); err != nil {
		t.Fatal(err)
	}
	task := model.CareTask{PlaceID: p.ID, Kind: model.TaskWatering, IntervalDays: 7,
		RedAfterDays: 14, Active: true, CreatedAt: vergabeJetzt.AddDate(0, 0, -30)}
	if err := d.InsertTask(&task); err != nil {
		t.Fatal(err)
	}
	return task
}

func TestWerkzeugVergabeStand(t *testing.T) {
	ts, d := serverMitDB(t)
	task := aufbauMitAufgabe(t, d)

	text, isErr := callTool(t, ts, "vergabe_stand", map[string]any{"taskId": task.ID})
	if isErr {
		t.Fatalf("vergabe_stand: %s", text)
	}
	var stand struct {
		PlaceName string `json:"placeName"`
		TaskName  string `json:"taskName"`
		Signups   []any  `json:"signups"`
		Vorgang   any    `json:"assignment"`
	}
	if err := json.Unmarshal([]byte(text), &stand); err != nil {
		t.Fatalf("Antwort ist kein JSON: %s", text)
	}
	if stand.PlaceName == "" || stand.TaskName == "" {
		t.Fatalf("Stand ohne Ort/Aufgabe: %s", text)
	}
	if stand.Vorgang != nil {
		t.Fatalf("Vorgang ohne Angemeldete: %s", text)
	}

	// Jetzt meldet sich jemand an, die Vergabe läuft und Anna sagt zu.
	anmeldung := model.Signup{UserSub: "anna", PlaceID: task.PlaceID, CreatedAt: vergabeJetzt.AddDate(0, 0, -5)}
	if _, err := d.InsertSignup(&anmeldung); err != nil {
		t.Fatal(err)
	}
	e := vergabe.New(d, vergabe.Config{Now: func() time.Time { return vergabeJetzt }})
	if err := e.Durchlauf(); err != nil {
		t.Fatal(err)
	}
	a, err := d.ActiveAssignment(task.ID)
	if err != nil || a == nil {
		t.Fatalf("kein Vorgang: %v", err)
	}
	if _, err := e.Zusagen(a.ID, "anna", "Anna"); err != nil {
		t.Fatal(err)
	}

	text, isErr = callTool(t, ts, "vergabe_stand", map[string]any{"taskId": task.ID})
	if isErr {
		t.Fatalf("vergabe_stand: %s", text)
	}
	if !strings.Contains(text, "uebernommen") || !strings.Contains(text, "Anna") {
		t.Fatalf("Stand zeigt die Zusage nicht: %s", text)
	}
	if !strings.Contains(text, "anfrage") {
		t.Fatalf("Stand zeigt nicht, wer gefragt wurde: %s", text)
	}
}

// Die bestehenden Werkzeugnamen dürfen sich nicht ändern — Claude-Connectors
// und eingespielte Abläufe hängen daran.
func TestWerkzeugnamenBleibenBestehen(t *testing.T) {
	ts := newTestServer(t)
	out := rpc(t, ts, "admin-jwt", "tools/list", nil)
	vorhanden := map[string]bool{}
	for _, roh := range out["result"].(map[string]any)["tools"].([]any) {
		vorhanden[roh.(map[string]any)["name"].(string)] = true
	}
	for _, name := range []string{
		"orte_liste", "ort_anlegen", "ort_aendern", "ort_loeschen",
		"aufgabe_anlegen", "aufgabe_aendern", "aufgabe_loeschen",
		"erledigung_melden", "erledigung_zuruecknehmen", "rangliste", "hitzefaktor_setzen",
		// neu für die Vergabe:
		"vergabe_stand", "zusage_aufheben",
	} {
		if !vorhanden[name] {
			t.Errorf("Werkzeug %q fehlt", name)
		}
	}
}

func TestWerkzeugZusageAufheben(t *testing.T) {
	ts, d := serverMitDB(t)
	task := aufbauMitAufgabe(t, d)

	// Ohne Vorgang gibt es nichts aufzuheben — mit verständlicher Ansage.
	text, isErr := callTool(t, ts, "zusage_aufheben", map[string]any{"taskId": task.ID})
	if !isErr {
		t.Fatalf("Aufheben ohne Vorgang wurde bestätigt: %s", text)
	}
	if !strings.Contains(text, "Vorgang") && !strings.Contains(text, "Zusage") {
		t.Fatalf("unverständlicher Fehlertext: %s", text)
	}

	anmeldung := model.Signup{UserSub: "anna", PlaceID: task.PlaceID, CreatedAt: vergabeJetzt.AddDate(0, 0, -5)}
	if _, err := d.InsertSignup(&anmeldung); err != nil {
		t.Fatal(err)
	}
	e := vergabe.New(d, vergabe.Config{Now: func() time.Time { return vergabeJetzt }})
	if err := e.Durchlauf(); err != nil {
		t.Fatal(err)
	}
	a, _ := d.ActiveAssignment(task.ID)
	if _, err := e.Zusagen(a.ID, "anna", "Anna"); err != nil {
		t.Fatal(err)
	}

	text, isErr = callTool(t, ts, "zusage_aufheben", map[string]any{"taskId": task.ID})
	if isErr {
		t.Fatalf("zusage_aufheben: %s", text)
	}
	danach, err := d.ActiveAssignment(task.ID)
	if err != nil || danach == nil {
		t.Fatalf("Vorgang verschwunden: %v", err)
	}
	if danach.ClaimedBy != "" {
		t.Fatalf("Zusage nicht aufgehoben: %+v", danach)
	}
	// Anna erfährt davon.
	offen, err := d.OpenNotifications("anna")
	if err != nil {
		t.Fatal(err)
	}
	hinweis := false
	for _, n := range offen {
		if n.Kind == model.NotifyClaimRevoked {
			hinweis = true
		}
	}
	if !hinweis {
		t.Fatalf("kein Hinweis an die betroffene Person: %+v", offen)
	}
}
