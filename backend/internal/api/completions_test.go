package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Feature A: eine irrtümliche Meldung muss zurückgenommen werden können —
// vom Melder selbst und von Admins.

const otherMemberToken = "karl-sub:Karl:"

// reportCompletion meldet eine Erledigung und liefert deren ID.
func reportCompletion(t *testing.T, ts *httptest.Server, taskID int64, token string) int64 {
	t.Helper()
	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), token, map[string]any{"liters": 10})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Erledigung melden: Status %d", resp.StatusCode)
	}
	return int64(decode[map[string]any](t, resp)["id"].(float64))
}

func TestWithdrawCompletion(t *testing.T) {
	ts, srv := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)

	// 15 Tage nach dem Anlegen ist die Aufgabe rot.
	srv.Now = func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }
	if got := getPlaces(t, ts).Places[0].Status; got != "red" {
		t.Fatalf("Ausgangsstatus = %v, erwartet red", got)
	}

	// Erna meldet versehentlich Vollzug → grün.
	id := reportCompletion(t, ts, taskID, memberToken)
	if got := getPlaces(t, ts).Places[0].Status; got != "green" {
		t.Fatalf("Status nach Meldung = %v, erwartet green", got)
	}

	// Karl darf Ernas Meldung nicht zurücknehmen.
	if resp := doReq(t, "DELETE", ts.URL+fmt.Sprintf("/api/v1/completions/%d", id), otherMemberToken, nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("fremde Meldung zurücknehmen: Status %d, erwartet 403", resp.StatusCode)
	}
	if got := getPlaces(t, ts).Places[0].Status; got != "green" {
		t.Fatalf("Status nach abgelehnter Rücknahme = %v, erwartet green", got)
	}

	// Erna nimmt ihre eigene Meldung zurück → Ampel rechnet neu, wieder rot.
	if resp := doReq(t, "DELETE", ts.URL+fmt.Sprintf("/api/v1/completions/%d", id), memberToken, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("eigene Meldung zurücknehmen: Status %d, erwartet 204", resp.StatusCode)
	}
	if got := getPlaces(t, ts).Places[0].Status; got != "red" {
		t.Fatalf("Status nach Rücknahme = %v, erwartet red", got)
	}

	// Die Historie ist leer.
	hist := decode[struct {
		Completions []map[string]any `json:"completions"`
	}](t, doReq(t, "GET", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), memberToken, nil))
	if len(hist.Completions) != 0 {
		t.Fatalf("Historie: %d Einträge, erwartet 0", len(hist.Completions))
	}

	// Nochmal löschen → 404, unbekannte ID → 404.
	for _, path := range []string{fmt.Sprintf("/api/v1/completions/%d", id), "/api/v1/completions/999999"} {
		if resp := doReq(t, "DELETE", ts.URL+path, memberToken, nil); resp.StatusCode != http.StatusNotFound {
			t.Errorf("DELETE %s: Status %d, erwartet 404", path, resp.StatusCode)
		}
	}
	// Kaputte ID → 400.
	if resp := doReq(t, "DELETE", ts.URL+"/api/v1/completions/abc", memberToken, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("DELETE mit kaputter ID: Status %d, erwartet 400", resp.StatusCode)
	}
}

func TestAdminWithdrawsForeignCompletion(t *testing.T) {
	ts, _ := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)
	id := reportCompletion(t, ts, taskID, memberToken)

	if resp := doReq(t, "DELETE", ts.URL+fmt.Sprintf("/api/v1/completions/%d", id), adminToken, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Admin nimmt fremde Meldung zurück: Status %d, erwartet 204", resp.StatusCode)
	}
}
