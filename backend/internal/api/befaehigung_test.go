package api

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Befähigungen („Einweisung nötig“): Wer die Motorsense nicht bedienen darf,
// kann die Aufgabe nicht zusagen — und zwar serverseitig, nicht bloß weil die
// Oberfläche den Knopf versteckt.

// befaehigungAnlegen legt eine Befähigung beim Träger an.
func befaehigungAnlegen(t *testing.T, ts string, traegerID int64, name string) int64 {
	t.Helper()
	resp := doReq(t, "POST", fmt.Sprintf("%s/api/v1/traeger/%d/befaehigungen", ts, traegerID),
		dorfpflegeAdmin, map[string]any{"name": name, "beschreibung": "Einweisung am Gerät"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Befähigung anlegen: HTTP %d", resp.StatusCode)
	}
	return decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID
}

func TestOhneBefaehigungKeineZusage(t *testing.T) {
	ts, srv := newTestServer(t)
	traegerID := traegerAnlegen(t, ts.URL, "Dorfpflege", "222")
	befaehigung := befaehigungAnlegen(t, ts.URL, traegerID, "Motorsense")

	// Ort mit einer Aufgabe, die die Einweisung verlangt.
	resp := doReq(t, "POST", ts.URL+"/api/v1/places", dorfpflegeAdmin, map[string]any{
		"name": "Streuobstwiese", "kind": "sonstiges", "lat": 52.21, "lon": 9.87,
		"traegerId": traegerID})
	ortID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID
	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/places/%d/tasks", ts.URL, ortID), dorfpflegeAdmin,
		map[string]any{"kind": "sonstiges", "title": "Rasenmähen", "intervalDays": 1, "redAfterDays": 2,
			"sichtbarkeit": "oeffentlich", "befaehigungId": befaehigung})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Aufgabe anlegen: HTTP %d", resp.StatusCode)
	}
	aufgabeID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID

	// Anmelden zum Mithelfen darf jede und jeder — daran hängt die
	// Befähigung nicht.
	if resp := doReq(t, "POST", fmt.Sprintf("%s/api/v1/places/%d/signup", ts.URL, ortID),
		dorfpflegeMitglied, map[string]any{}); resp.StatusCode >= 300 {
		t.Fatalf("Anmelden: HTTP %d", resp.StatusCode)
	}

	// Die Aufgabe fällig werden lassen, damit ein Vorgang entsteht.
	vorgangID := vorgangEroeffnen(t, srv, aufgabeID)

	zusagen := func(token string) *http.Response {
		return doReq(t, "POST", fmt.Sprintf("%s/api/v1/assignments/%d/claim", ts.URL, vorgangID),
			token, nil)
	}

	resp = zusagen(dorfpflegeMitglied)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Zusage ohne Einweisung: HTTP %d, erwartet 403", resp.StatusCode)
	}
	fehler := decode[map[string]any](t, resp)
	if text, _ := fehler["error"].(string); text == "" {
		t.Error("403 ohne verständliche Begründung")
	}

	// Beantragen, freigeben — danach geht es.
	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/befaehigungen/%d/antrag", ts.URL, befaehigung),
		dorfpflegeMitglied, map[string]any{"begruendung": "War bei der Einweisung dabei"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Antrag: HTTP %d", resp.StatusCode)
	}
	antragID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID

	// Ein einfaches Mitglied entscheidet nicht über sich selbst.
	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/antraege/%d", ts.URL, antragID),
		dorfpflegeMitglied, map[string]any{"status": "erteilt"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Selbstfreigabe: HTTP %d, erwartet 403", resp.StatusCode)
	}

	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/antraege/%d", ts.URL, antragID),
		dorfpflegeAdmin, map[string]any{"status": "erteilt", "notiz": "am 12.8. eingewiesen"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Freigabe: HTTP %d", resp.StatusCode)
	}

	if resp := zusagen(dorfpflegeMitglied); resp.StatusCode != http.StatusOK {
		t.Fatalf("Zusage mit Einweisung: HTTP %d, erwartet 200", resp.StatusCode)
	}
}

// vorgangEroeffnen lässt die Vergabe einen Vorgang für die Aufgabe anlegen,
// indem die Aufgabe künstlich altert.
func vorgangEroeffnen(t *testing.T, srv *Server, aufgabeID int64) int64 {
	t.Helper()
	task, err := srv.DB.GetTask(aufgabeID)
	if err != nil {
		t.Fatal(err)
	}
	// Die Aufgabe ist längst fällig: angelegt vor 30 Tagen.
	task.CreatedAt = srv.now().Add(-30 * 24 * time.Hour)
	if err := srv.DB.UpdateTask(task); err != nil {
		t.Fatal(err)
	}
	a := model.Assignment{TaskID: aufgabeID, State: model.AssignmentOpen, CreatedAt: srv.now()}
	if err := srv.DB.InsertAssignment(&a); err != nil {
		t.Fatal(err)
	}
	return a.ID
}

// Eine Befähigung gehört ihrem Träger. Ein fremder Verein darf sie weder
// pflegen noch über Anträge darauf entscheiden.
func TestBefaehigungGehoertIhremTraeger(t *testing.T) {
	ts, _ := newTestServer(t)
	traegerID := traegerAnlegen(t, ts.URL, "Dorfpflege", "222")
	traegerAnlegen(t, ts.URL, "Schützenverein", "333")
	befaehigung := befaehigungAnlegen(t, ts.URL, traegerID, "Motorsense")

	resp := doReq(t, "PUT", fmt.Sprintf("%s/api/v1/befaehigungen/%d", ts.URL, befaehigung),
		"fremd:Fremd:333@admin", map[string]any{"name": "Umbenannt"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("fremder Träger pflegt die Befähigung: HTTP %d, erwartet 403", resp.StatusCode)
	}
	resp = doReq(t, "DELETE", fmt.Sprintf("%s/api/v1/befaehigungen/%d", ts.URL, befaehigung),
		"fremd:Fremd:333@admin", nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("fremder Träger löscht die Befähigung: HTTP %d, erwartet 403", resp.StatusCode)
	}
}

// Eine Aufgabe darf nur eine Befähigung ihres EIGENEN Trägers verlangen —
// sonst könnte ein Verein die Mitglieder eines anderen aussperren.
func TestAufgabeVerlangtNurEigeneBefaehigung(t *testing.T) {
	ts, _ := newTestServer(t)
	eigener := traegerAnlegen(t, ts.URL, "Dorfpflege", "222")
	fremder := traegerAnlegen(t, ts.URL, "Schützenverein", "333")
	resp := doReq(t, "POST", fmt.Sprintf("%s/api/v1/traeger/%d/befaehigungen", ts.URL, fremder),
		betreiberToken, map[string]any{"name": "Schießstand"})
	fremdeBefaehigung := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID

	resp = doReq(t, "POST", ts.URL+"/api/v1/places", dorfpflegeAdmin, map[string]any{
		"name": "Wiese", "kind": "sonstiges", "lat": 52.21, "lon": 9.87, "traegerId": eigener})
	ortID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID

	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/places/%d/tasks", ts.URL, ortID), dorfpflegeAdmin,
		map[string]any{"kind": "sonstiges", "title": "Mähen", "intervalDays": 7, "redAfterDays": 14,
			"befaehigungId": fremdeBefaehigung})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("fremde Befähigung angenommen: HTTP %d, erwartet 400", resp.StatusCode)
	}
}
