package api

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Einmalige Aufgaben (#6) und die Verwaltung aus der App heraus (#5, #7).
//
// Zwei Dinge werden hier festgehalten:
//
//   - Anlegen, Ändern und Löschen von Orten und Aufgaben ist ausschließlich
//     Sache der Verwaltung. Geprüft wird die Rolle aus dem Token, nicht die
//     Oberfläche — die App versteckt den Knopf nur zusätzlich.
//   - Eine einmalige Aufgabe hat einen Termin statt eines Intervalls und
//     verschwindet auf Wunsch, sobald sie erledigt ist.

const jetzt = "2026-08-12T12:00:00Z"

func zeit(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// ortAnlegen legt einen Ort an und liefert seine ID.
func ortAnlegen(t *testing.T, ts string, token, name string) int64 {
	t.Helper()
	resp := doReq(t, "POST", ts+"/api/v1/places", token,
		map[string]any{"name": name, "kind": "sonstiges", "lat": 52.211, "lon": 9.87})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Ort anlegen: HTTP %d", resp.StatusCode)
	}
	return decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID
}

type aufgabeAntwort struct {
	ID             int64      `json:"id"`
	OneOff         bool       `json:"oneOff"`
	DueDate        *time.Time `json:"dueDate"`
	RemoveWhenDone bool       `json:"removeWhenDone"`
	IntervalDays   float64    `json:"intervalDays"`
	Error          string     `json:"error"`
}

// --- Rollen ------------------------------------------------------------------

// Ohne admin-Rolle darf niemand Orte oder Aufgaben anlegen, ändern oder
// löschen — auch nicht mit einer eigenen App oder curl.
func TestNurVerwaltungDarfAufgabenPflegen(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID := ortAnlegen(t, ts.URL, adminToken, "Bahnhof")
	resp := doReq(t, "POST", ts.URL+"/api/v1/places/1/tasks", adminToken,
		map[string]any{"kind": "giessen", "intervalDays": 7, "redAfterDays": 14})
	taskID := decode[aufgabeAntwort](t, resp).ID

	faellig := zeit(t, jetzt).Add(48 * time.Hour).Format(time.RFC3339)
	versuche := []struct {
		name   string
		method string
		pfad   string
		koerp  any
	}{
		{"Ort anlegen", "POST", "/api/v1/places",
			map[string]any{"name": "Heimlich", "lat": 52.2, "lon": 9.8}},
		{"Ort ändern", "PUT", "/api/v1/places/1",
			map[string]any{"name": "Umbenannt", "lat": 52.2, "lon": 9.8}},
		{"Ort löschen", "DELETE", "/api/v1/places/1", nil},
		{"Aufgabe anlegen", "POST", "/api/v1/places/1/tasks",
			map[string]any{"kind": "sonstiges", "title": "Einkauf", "oneOff": true, "dueDate": faellig}},
		{"Aufgabe ändern", "PUT", "/api/v1/tasks/1",
			map[string]any{"kind": "giessen", "intervalDays": 1, "redAfterDays": 2}},
		{"Aufgabe löschen", "DELETE", "/api/v1/tasks/1", nil},
	}
	for _, v := range versuche {
		t.Run(v.name, func(t *testing.T) {
			resp := doReq(t, v.method, ts.URL+v.pfad, memberToken, v.koerp)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("HTTP %d, erwartet 403 — ein Mitglied darf das nicht", resp.StatusCode)
			}
		})
	}
	// Und der Bestand ist unangetastet geblieben.
	liste := decode[placesResponse](t, doReq(t, "GET", ts.URL+"/api/v1/places", memberToken, nil))
	if len(liste.Places) != 1 || liste.Places[0].ID != placeID || len(liste.Places[0].Tasks) != 1 {
		t.Fatalf("Bestand verändert: %+v", liste.Places)
	}
	_ = taskID
}

// --- Anlegen einer einmaligen Aufgabe ---------------------------------------

func TestEinmaligeAufgabeAnlegen(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID := ortAnlegen(t, ts.URL, adminToken, "Bahnhof")
	termin := zeit(t, jetzt).Add(10 * 24 * time.Hour)

	resp := doReq(t, "POST", ts.URL+pfadTasks(placeID), adminToken, map[string]any{
		"kind": "sonstiges", "title": "Zum Bahnhof fahren",
		"oneOff": true, "dueDate": termin.Format(time.RFC3339), "removeWhenDone": true,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("HTTP %d: %+v", resp.StatusCode, decode[aufgabeAntwort](t, resp))
	}
	a := decode[aufgabeAntwort](t, resp)
	if !a.OneOff || !a.RemoveWhenDone {
		t.Fatalf("Schalter nicht übernommen: %+v", a)
	}
	if a.DueDate == nil || !a.DueDate.Equal(termin) {
		t.Fatalf("Termin nicht übernommen: %v", a.DueDate)
	}

	// Die Ampel richtet sich nach dem Termin: zehn Tage hin, also grün.
	liste := decode[placesResponse](t, doReq(t, "GET", ts.URL+"/api/v1/places", memberToken, nil))
	if len(liste.Places) != 1 || len(liste.Places[0].Tasks) != 1 {
		t.Fatalf("Aufgabe fehlt in der Liste: %+v", liste.Places)
	}
	if liste.Places[0].Tasks[0].Status != string(model.StatusGreen) {
		t.Fatalf("Status = %s, erwartet green", liste.Places[0].Tasks[0].Status)
	}
}

// Ein bloßes Datum („bis zum 20.“) ist die übliche Eingabe. Es zählt dann
// der ganze Tag: erst danach ist die Aufgabe überfällig.
func TestEinmaligNimmtReinesDatum(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID := ortAnlegen(t, ts.URL, adminToken, "Bahnhof")
	resp := doReq(t, "POST", ts.URL+pfadTasks(placeID), adminToken, map[string]any{
		"kind": "sonstiges", "title": "Einkauf", "oneOff": true, "dueDate": "2026-08-20",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("HTTP %d: %+v", resp.StatusCode, decode[aufgabeAntwort](t, resp))
	}
	a := decode[aufgabeAntwort](t, resp)
	if a.DueDate == nil {
		t.Fatal("kein Termin gespeichert")
	}
	// Ortszeit des Dorfes, Ende des Tages.
	ort := a.DueDate.In(model.Location())
	if ort.Year() != 2026 || ort.Month() != 8 || ort.Day() != 20 || ort.Hour() != 23 {
		t.Fatalf("Termin = %v, erwartet den 20.08.2026 zum Tagesende", ort)
	}
}

func TestEinmaligeAufgabeWirdGeprueft(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID := ortAnlegen(t, ts.URL, adminToken, "Bahnhof")
	faelle := []struct {
		name   string
		koerp  map[string]any
		erwart string
	}{
		{"einmalig ohne Termin",
			map[string]any{"kind": "sonstiges", "title": "X", "oneOff": true}, "dueDate"},
		{"einmalig mit unlesbarem Termin",
			map[string]any{"kind": "sonstiges", "oneOff": true, "dueDate": "irgendwann"}, "dueDate"},
		{"regelmäßig mit Termin",
			map[string]any{"kind": "giessen", "intervalDays": 7, "redAfterDays": 14,
				"dueDate": "2026-09-01"}, "dueDate"},
		{"regelmäßig ohne Intervall",
			map[string]any{"kind": "giessen"}, "intervalDays"},
	}
	for _, f := range faelle {
		t.Run(f.name, func(t *testing.T) {
			resp := doReq(t, "POST", ts.URL+pfadTasks(placeID), adminToken, f.koerp)
			a := decode[aufgabeAntwort](t, resp)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("HTTP %d, erwartet 400", resp.StatusCode)
			}
			if a.Error == "" {
				t.Fatal("keine Begründung im Klartext")
			}
			if !strings.Contains(a.Error, f.erwart) {
				t.Fatalf("Begründung %q nennt %q nicht", a.Error, f.erwart)
			}
		})
	}
}

// --- Erledigen ---------------------------------------------------------------

// „Nach dem Erledigen entfernen": Die Aufgabe verschwindet von Karte und
// Liste — die Erledigung bleibt und zählt weiter für die Rangliste.
func TestEinmaligeAufgabeVerschwindetNachErledigung(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID := ortAnlegen(t, ts.URL, adminToken, "Bahnhof")
	resp := doReq(t, "POST", ts.URL+pfadTasks(placeID), adminToken, map[string]any{
		"kind": "sonstiges", "title": "Zum Bahnhof fahren",
		"oneOff": true, "dueDate": "2026-08-20", "removeWhenDone": true,
	})
	taskID := decode[aufgabeAntwort](t, resp).ID

	vorher := decode[leaderboardAntwort](t,
		doReq(t, "GET", ts.URL+"/api/v1/stats/leaderboard?period=gesamt", memberToken, nil))

	// Ein gewöhnliches Mitglied erledigt sie — das darf jeder.
	resp = doReq(t, "POST", ts.URL+pfadCompletions(taskID), memberToken, map[string]any{})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Erledigung: HTTP %d", resp.StatusCode)
	}
	resp.Body.Close()

	liste := decode[placesResponse](t, doReq(t, "GET", ts.URL+"/api/v1/places", memberToken, nil))
	if len(liste.Places) != 1 {
		t.Fatalf("Ort verschwunden: %+v", liste.Places)
	}
	if len(liste.Places[0].Tasks) != 0 {
		t.Fatalf("Die erledigte einmalige Aufgabe steht noch da: %+v", liste.Places[0].Tasks)
	}

	nachher := decode[leaderboardAntwort](t,
		doReq(t, "GET", ts.URL+"/api/v1/stats/leaderboard?period=gesamt", memberToken, nil))
	if nachher.Totals.Completions != vorher.Totals.Completions+1 {
		t.Fatalf("Die Erledigung zählt nicht mehr für die Rangliste: %d → %d",
			vorher.Totals.Completions, nachher.Totals.Completions)
	}
}

// Ohne den Schalter bleibt die erledigte einmalige Aufgabe stehen — grün,
// und sie wird nicht wieder fällig.
func TestEinmaligeAufgabeBleibtOhneSchalter(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID := ortAnlegen(t, ts.URL, adminToken, "Bahnhof")
	resp := doReq(t, "POST", ts.URL+pfadTasks(placeID), adminToken, map[string]any{
		"kind": "sonstiges", "title": "Bank streichen", "oneOff": true, "dueDate": "2026-08-13",
	})
	taskID := decode[aufgabeAntwort](t, resp).ID
	resp = doReq(t, "POST", ts.URL+pfadCompletions(taskID), memberToken, map[string]any{})
	resp.Body.Close()

	liste := decode[placesResponse](t, doReq(t, "GET", ts.URL+"/api/v1/places", memberToken, nil))
	if len(liste.Places[0].Tasks) != 1 {
		t.Fatalf("Aufgabe verschwunden, obwohl der Schalter aus ist: %+v", liste.Places[0].Tasks)
	}
	if liste.Places[0].Tasks[0].Status != string(model.StatusGreen) {
		t.Fatalf("Status = %s, erwartet green", liste.Places[0].Tasks[0].Status)
	}
}

func pfadTasks(placeID int64) string {
	return "/api/v1/places/" + itoa64(placeID) + "/tasks"
}

func pfadCompletions(taskID int64) string {
	return "/api/v1/tasks/" + itoa64(taskID) + "/completions"
}

type leaderboardAntwort struct {
	Totals struct {
		Completions int `json:"completions"`
	} `json:"totals"`
}
