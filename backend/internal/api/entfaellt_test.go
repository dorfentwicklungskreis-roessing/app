package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Eine Aufgabe, die jemand gerade zugesagt hat, darf nicht still
// verschwinden (#7). Wird sie gelöscht oder pausiert, muss die Person, die
// zugesagt hat, davon erfahren — sonst zieht sie mit der Gießkanne los.

// zugesagt baut die Ausgangslage: Ort mit Gießplan, Anna angemeldet, Aufgabe
// fällig, Vorgang eröffnet, Anna hat zugesagt. Liefert Orts- und Aufgaben-ID.
func zugesagt(t *testing.T, ts *httptest.Server, srv *Server) (placeID, taskID int64) {
	t.Helper()
	placeID, taskID = createPlaceWithTask(t, ts)
	anmelden(t, ts, annaToken, placeID, nil).Body.Close()
	faellig(srv)
	takt(t, srv)

	ns := meineBenachrichtigungen(t, ts, annaToken)
	if len(ns.Notifications) != 1 {
		t.Fatalf("keine Anfrage an Anna: %+v", ns.Notifications)
	}
	resp := doReq(t, "POST",
		ts.URL+fmt.Sprintf("/api/v1/assignments/%d/claim", ns.Notifications[0].AssignmentID), annaToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Zusage: HTTP %d", resp.StatusCode)
	}
	resp.Body.Close()
	return placeID, taskID
}

// hinweis sucht in der Abrufliste den Hinweis „nicht mehr nötig".
func hinweis(t *testing.T, ts *httptest.Server, token string) (kind, titel, text string, gefunden bool) {
	t.Helper()
	for _, n := range meineBenachrichtigungen(t, ts, token).Notifications {
		if n.Kind == string(model.NotifyAssignmentDropped) {
			return n.Kind, n.Title, n.Text, true
		}
	}
	return "", "", "", false
}

func TestGeloeschteAufgabeBenachrichtigtDieZusagende(t *testing.T) {
	ts, srv := newTestServer(t)
	_, taskID := zugesagt(t, ts, srv)

	resp := doReq(t, "DELETE", ts.URL+fmt.Sprintf("/api/v1/tasks/%d", taskID), adminToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Löschen: HTTP %d", resp.StatusCode)
	}
	resp.Body.Close()

	_, titel, text, ok := hinweis(t, ts, annaToken)
	if !ok {
		t.Fatal("Anna erfährt nichts davon, dass ihre zugesagte Aufgabe gelöscht wurde")
	}
	if titel == "" || text == "" {
		t.Fatalf("Hinweis ohne Text: %q / %q", titel, text)
	}
	// Ort und Aufgabe gibt es nicht mehr — der Hinweis muss trotzdem sagen,
	// worum es ging.
	if !strings.Contains(text, "Unter den Eichen") {
		t.Errorf("Hinweis nennt den Ort nicht: %q", text)
	}
}

func TestPausierteAufgabeBenachrichtigtDieZusagende(t *testing.T) {
	ts, srv := newTestServer(t)
	_, taskID := zugesagt(t, ts, srv)

	// Pausieren heißt: auf inaktiv setzen (Urlaub, Kasten abgebaut).
	resp := doReq(t, "PUT", ts.URL+fmt.Sprintf("/api/v1/tasks/%d", taskID), adminToken, map[string]any{
		"kind": "giessen", "liters": 10, "intervalDays": 7, "redAfterDays": 14, "active": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Pausieren: HTTP %d", resp.StatusCode)
	}
	resp.Body.Close()

	if _, _, _, ok := hinweis(t, ts, annaToken); !ok {
		t.Fatal("Anna erfährt nichts davon, dass ihre zugesagte Aufgabe pausiert wurde")
	}
}

func TestGeloeschterOrtBenachrichtigtDieZusagende(t *testing.T) {
	ts, srv := newTestServer(t)
	placeID, _ := zugesagt(t, ts, srv)

	resp := doReq(t, "DELETE", ts.URL+fmt.Sprintf("/api/v1/places/%d", placeID), adminToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Ort löschen: HTTP %d", resp.StatusCode)
	}
	resp.Body.Close()

	if _, _, _, ok := hinweis(t, ts, annaToken); !ok {
		t.Fatal("Anna erfährt nichts davon, dass der Ort ihrer zugesagten Aufgabe gelöscht wurde")
	}
}

// Wer weiter fortfährt wie bisher, bekommt keinen Hinweis: Ein Speichern
// ohne Pausieren ist keine Nachricht wert.
func TestAendernOhnePauseSchweigt(t *testing.T) {
	ts, srv := newTestServer(t)
	_, taskID := zugesagt(t, ts, srv)

	resp := doReq(t, "PUT", ts.URL+fmt.Sprintf("/api/v1/tasks/%d", taskID), adminToken, map[string]any{
		"kind": "giessen", "liters": 12, "intervalDays": 7, "redAfterDays": 14, "active": true,
	})
	resp.Body.Close()

	if _, _, _, ok := hinweis(t, ts, annaToken); ok {
		t.Fatal("Ein bloßes Speichern hat die Zusage aufgehoben")
	}
}
