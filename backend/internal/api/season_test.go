package api

import (
	"fmt"
	"net/http"
	"testing"
)

// Die Jahreszeit einer Aufgabe über die REST-Schnittstelle (#78).
//
// Die Testuhr steht auf dem 12. August 2026. Eine Aufgabe „November bis
// Februar" ist an diesem Tag also außer Dienst, eine „April bis September"
// im Dienst — damit lassen sich beide Seiten prüfen, ohne an der Uhr zu
// drehen.

// aufgabeMitJahreszeit legt einen Ort samt Aufgabe an und liefert beide IDs.
func aufgabeMitJahreszeit(t *testing.T, ts string, von, bis int) (int64, int64) {
	t.Helper()
	placeID := ortAnlegen(t, ts, adminToken, "Beet am Dorfgemeinschaftshaus")
	resp := doReq(t, "POST", ts+fmt.Sprintf("/api/v1/places/%d/tasks", placeID), adminToken,
		map[string]any{"kind": "jaeten", "intervalDays": 56, "redAfterDays": 70,
			"seasonStartMonth": von, "seasonEndMonth": bis})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Aufgabe anlegen: HTTP %d", resp.StatusCode)
	}
	return placeID, decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID
}

// Der Fall aus dem Issue: Im Winter zeigt die Karte keinen roten Punkt für
// ein Beet, an dem nichts zu jäten ist — die Aufgabe ist außer Dienst, und
// der Ort mit ihr.
func TestAufgabeAusserhalbDerJahreszeitIstAusserDienst(t *testing.T) {
	ts, _ := newTestServer(t)
	aufgabeMitJahreszeit(t, ts.URL, 11, 2) // November bis Februar

	orte := getPlaces(t, ts)
	if len(orte.Places) != 1 || len(orte.Places[0].Tasks) != 1 {
		t.Fatalf("unerwartete Ortsliste: %+v", orte)
	}
	if got := orte.Places[0].Tasks[0].Status; got != "dormant" {
		t.Errorf("Aufgabenstatus im August = %q, erwartet \"dormant\"", got)
	}
	if got := orte.Places[0].Status; got != "dormant" {
		t.Errorf("Ortsstatus = %q, erwartet \"dormant\"", got)
	}
}

// Innerhalb ihrer Jahreszeit rechnet die Ampel wie eh und je.
func TestAufgabeInnerhalbDerJahreszeitZaehltNormal(t *testing.T) {
	ts, _ := newTestServer(t)
	// April bis September: Der 12. August liegt mittendrin, die Aufgabe ist
	// am Anlegetag frisch und damit grün.
	aufgabeMitJahreszeit(t, ts.URL, 4, 9)

	orte := getPlaces(t, ts)
	if got := orte.Places[0].Tasks[0].Status; got != "green" {
		t.Errorf("Aufgabenstatus am Anlegetag = %q, erwartet \"green\"", got)
	}
	if got := orte.Places[0].Status; got != "green" {
		t.Errorf("Ortsstatus = %q, erwartet \"green\"", got)
	}
}

// Ein Ort mit einer ruhenden und einer ganzjährigen Aufgabe richtet sich
// nach der, an der etwas zu tun ist.
func TestOrtMitRuhenderUndGanzjaehrigerAufgabe(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID, _ := aufgabeMitJahreszeit(t, ts.URL, 11, 2)
	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/places/%d/tasks", placeID), adminToken,
		map[string]any{"kind": "sonstiges", "title": "Müll aufsammeln",
			"intervalDays": 7, "redAfterDays": 14})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("zweite Aufgabe anlegen: HTTP %d", resp.StatusCode)
	}
	orte := getPlaces(t, ts)
	if got := orte.Places[0].Status; got != "green" {
		t.Errorf("Ortsstatus = %q, erwartet \"green\" — die ganzjährige Aufgabe entscheidet", got)
	}
}

// Wer eine Jahreszeit angibt, muss beide Monate angeben; eine einmalige
// Aufgabe hat gar keine.
func TestJahreszeitWirdGeprueft(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID := ortAnlegen(t, ts.URL, adminToken, "Beet")
	pfad := fmt.Sprintf("/api/v1/places/%d/tasks", placeID)

	faelle := map[string]map[string]any{
		"nur Anfangsmonat": {"kind": "jaeten", "intervalDays": 56, "redAfterDays": 70,
			"seasonStartMonth": 4},
		"nur Endmonat": {"kind": "jaeten", "intervalDays": 56, "redAfterDays": 70,
			"seasonEndMonth": 9},
		"Monat 13": {"kind": "jaeten", "intervalDays": 56, "redAfterDays": 70,
			"seasonStartMonth": 13, "seasonEndMonth": 9},
		"einmalig mit Jahreszeit": {"kind": "sonstiges", "oneOff": true,
			"dueDate": "2026-09-01", "seasonStartMonth": 4, "seasonEndMonth": 9},
	}
	for name, koerper := range faelle {
		t.Run(name, func(t *testing.T) {
			resp := doReq(t, "POST", ts.URL+pfad, adminToken, koerper)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("HTTP %d, erwartet 400", resp.StatusCode)
			}
			if msg := decode[struct {
				Error string `json:"error"`
			}](t, resp).Error; msg == "" {
				t.Error("ohne Begründung abgewiesen")
			}
		})
	}
}

// Eine ältere App-Version schickt die Felder nicht mit. Ein geändertes
// Intervall darf der Aufgabe dann nicht ihre Jahreszeit wegnehmen.
func TestAendernOhneJahreszeitLaesstSieStehen(t *testing.T) {
	ts, _ := newTestServer(t)
	_, taskID := aufgabeMitJahreszeit(t, ts.URL, 4, 9)

	resp := doReq(t, "PUT", ts.URL+fmt.Sprintf("/api/v1/tasks/%d", taskID), adminToken,
		map[string]any{"kind": "jaeten", "intervalDays": 42, "redAfterDays": 56})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Ändern: HTTP %d", resp.StatusCode)
	}
	geaendert := decode[struct {
		IntervalDays     float64 `json:"intervalDays"`
		SeasonStartMonth int     `json:"seasonStartMonth"`
		SeasonEndMonth   int     `json:"seasonEndMonth"`
	}](t, resp)
	if geaendert.IntervalDays != 42 {
		t.Errorf("Intervall = %v, erwartet 42", geaendert.IntervalDays)
	}
	if geaendert.SeasonStartMonth != 4 || geaendert.SeasonEndMonth != 9 {
		t.Errorf("Jahreszeit = %d/%d, erwartet 4/9",
			geaendert.SeasonStartMonth, geaendert.SeasonEndMonth)
	}
}

// Ausdrücklich 0 nimmt die Jahreszeit wieder weg.
func TestJahreszeitLaesstSichAbraeumen(t *testing.T) {
	ts, _ := newTestServer(t)
	_, taskID := aufgabeMitJahreszeit(t, ts.URL, 11, 2)

	resp := doReq(t, "PUT", ts.URL+fmt.Sprintf("/api/v1/tasks/%d", taskID), adminToken,
		map[string]any{"kind": "jaeten", "intervalDays": 56, "redAfterDays": 70,
			"seasonStartMonth": 0, "seasonEndMonth": 0})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Ändern: HTTP %d", resp.StatusCode)
	}
	orte := getPlaces(t, ts)
	if got := orte.Places[0].Tasks[0].Status; got == "dormant" {
		t.Error("die Aufgabe ruht noch, obwohl die Jahreszeit weg ist")
	}
}
