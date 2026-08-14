package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/vergabe"
)

// Tests der Vergabe-Endpunkte: Anmeldung, Benachrichtigungen, Zusage.
// Getaktet wird von Hand (kein Warten), gerechnet mit der gestellten Uhr des
// Servers.

const (
	annaToken  = "anna-sub:Anna:"
	berndToken = "bernd-sub:Bernd:"
)

// takt lässt die Vergabe einen Durchlauf machen — wie der Zeitgeber im Betrieb.
func takt(t *testing.T, srv *Server) {
	t.Helper()
	e := vergabe.New(srv.DB, vergabe.Config{Now: srv.now})
	if err := e.Durchlauf(); err != nil {
		t.Fatalf("Vergabe-Durchlauf: %v", err)
	}
}

// faellig stellt die Uhr acht Tage vor: Der Gießplan ist dann überfällig.
func faellig(srv *Server) time.Time {
	jetzt := time.Date(2026, 8, 20, 9, 0, 0, 0, model.Location())
	srv.Now = func() time.Time { return jetzt }
	return jetzt
}

type signupsAntwort struct {
	Signups []struct {
		ID        int64  `json:"id"`
		UserSub   string `json:"userSub"`
		UserName  string `json:"userName"`
		PlaceID   int64  `json:"placeId"`
		TaskKind  string `json:"taskKind"`
		PlaceName string `json:"placeName"`
	} `json:"signups"`
}

type notificationsAntwort struct {
	Notifications []struct {
		ID           int64  `json:"id"`
		AssignmentID int64  `json:"assignmentId"`
		Kind         string `json:"kind"`
		PlaceID      int64  `json:"placeId"`
		PlaceName    string `json:"placeName"`
		TaskName     string `json:"taskName"`
		Title        string `json:"title"`
		Text         string `json:"text"`
		ExpiresAt    string `json:"expiresAt"`
		Acknowledged string `json:"acknowledgedAt"`
	} `json:"notifications"`
}

type vergabePlaces struct {
	Places []struct {
		ID    int64 `json:"id"`
		Tasks []struct {
			ID          int64 `json:"id"`
			SignedUp    bool  `json:"signedUp"`
			SignupCount int   `json:"signupCount"`
			Assignment  *struct {
				ID            int64  `json:"id"`
				State         string `json:"state"`
				ClaimedBy     string `json:"claimedBy"`
				ClaimedByName string `json:"claimedByName"`
				ClaimedUntil  string `json:"claimedUntil"`
				AskedCount    int    `json:"askedCount"`
			} `json:"assignment"`
		} `json:"tasks"`
	} `json:"places"`
}

func anmelden(t *testing.T, ts *httptest.Server, token string, placeID int64, body any) *http.Response {
	t.Helper()
	return doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/places/%d/signup", placeID), token, body)
}

func meineBenachrichtigungen(t *testing.T, ts *httptest.Server, token string) notificationsAntwort {
	t.Helper()
	return decode[notificationsAntwort](t, doReq(t, "GET", ts.URL+"/api/v1/me/notifications", token, nil))
}

func TestAnmeldenUndAbmelden(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID, _ := createPlaceWithTask(t, ts)

	resp := anmelden(t, ts, annaToken, placeID, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Anmelden: Status %d, erwartet 201", resp.StatusCode)
	}
	resp.Body.Close()

	// Zweimal anmelden ist kein Fehler — Doppeltipp, zweites Gerät.
	resp = anmelden(t, ts, annaToken, placeID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("zweites Anmelden: Status %d, erwartet 200", resp.StatusCode)
	}
	resp.Body.Close()

	meine := decode[signupsAntwort](t, doReq(t, "GET", ts.URL+"/api/v1/me/signups", annaToken, nil))
	if len(meine.Signups) != 1 || meine.Signups[0].PlaceID != placeID {
		t.Fatalf("eigene Anmeldungen = %+v", meine.Signups)
	}
	if meine.Signups[0].PlaceName == "" {
		t.Error("Anmeldung ohne Ortsnamen — die App müsste den Ort nachschlagen")
	}

	resp = doReq(t, "DELETE", ts.URL+fmt.Sprintf("/api/v1/places/%d/signup", placeID), annaToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Abmelden: Status %d, erwartet 204", resp.StatusCode)
	}
	resp.Body.Close()
	meine = decode[signupsAntwort](t, doReq(t, "GET", ts.URL+"/api/v1/me/signups", annaToken, nil))
	if len(meine.Signups) != 0 {
		t.Fatalf("nach dem Abmelden noch angemeldet: %+v", meine.Signups)
	}
}

func TestAnmeldungNurAufEineAufgabenart(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID, _ := createPlaceWithTask(t, ts)

	resp := anmelden(t, ts, annaToken, placeID, map[string]any{"taskKind": "giessen"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Anmelden fürs Gießen: Status %d", resp.StatusCode)
	}
	resp.Body.Close()
	resp = anmelden(t, ts, annaToken, placeID, map[string]any{"taskKind": "raumschiff"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unsinnige Aufgabenart: Status %d, erwartet 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// Anmelden kann man nur sich selbst — auch als Admin.
func TestKeineFremdeAnmeldung(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID, _ := createPlaceWithTask(t, ts)

	resp := anmelden(t, ts, adminToken, placeID, map[string]any{"userSub": "anna-sub"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("fremde Anmeldung durch Admin: Status %d, erwartet 403", resp.StatusCode)
	}
	resp.Body.Close()
}

// Wer angemeldet ist, sehen nur Verwaltende — sonst niemand.
func TestAnmeldungenSehenNurAdmins(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID, _ := createPlaceWithTask(t, ts)
	anmelden(t, ts, annaToken, placeID, nil).Body.Close()

	resp := doReq(t, "GET", ts.URL+fmt.Sprintf("/api/v1/places/%d/signups", placeID), memberToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Mitglied sieht fremde Anmeldungen: Status %d, erwartet 403", resp.StatusCode)
	}
	resp.Body.Close()

	liste := decode[signupsAntwort](t, doReq(t, "GET",
		ts.URL+fmt.Sprintf("/api/v1/places/%d/signups", placeID), adminToken, nil))
	if len(liste.Signups) != 1 || liste.Signups[0].UserSub != "anna-sub" {
		t.Fatalf("Admin-Sicht = %+v", liste.Signups)
	}
}

func TestBenachrichtigungAbrufenUndBestaetigen(t *testing.T) {
	ts, srv := newTestServer(t)
	placeID, _ := createPlaceWithTask(t, ts)
	anmelden(t, ts, annaToken, placeID, nil).Body.Close()
	faellig(srv)
	takt(t, srv)

	ns := meineBenachrichtigungen(t, ts, annaToken)
	if len(ns.Notifications) != 1 {
		t.Fatalf("%d Benachrichtigungen, erwartet 1: %+v", len(ns.Notifications), ns.Notifications)
	}
	n := ns.Notifications[0]
	if n.Kind != string(model.NotifyRequest) || n.PlaceID != placeID {
		t.Fatalf("Benachrichtigung = %+v", n)
	}
	if n.PlaceName == "" || n.TaskName == "" || n.Text == "" || n.Title == "" {
		t.Fatalf("Benachrichtigung ohne Klartext: %+v", n)
	}
	if n.ExpiresAt == "" {
		t.Error("Anfrage ohne Frist")
	}

	// Fremde sehen sie nicht.
	if fremd := meineBenachrichtigungen(t, ts, berndToken); len(fremd.Notifications) != 0 {
		t.Fatalf("fremde Benachrichtigungen sichtbar: %+v", fremd.Notifications)
	}
	// Und können sie nicht bestätigen.
	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/me/notifications/%d/ack", n.ID), berndToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("fremdes Bestätigen: Status %d, erwartet 403", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/me/notifications/%d/ack", n.ID), annaToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Bestätigen: Status %d, erwartet 204", resp.StatusCode)
	}
	resp.Body.Close()

	ns = meineBenachrichtigungen(t, ts, annaToken)
	if len(ns.Notifications) != 1 || ns.Notifications[0].Acknowledged == "" {
		t.Fatalf("nach dem Bestätigen: %+v", ns.Notifications)
	}
}

func TestZusagenUndZurueckgeben(t *testing.T) {
	ts, srv := newTestServer(t)
	placeID, taskID := createPlaceWithTask(t, ts)
	anmelden(t, ts, annaToken, placeID, nil).Body.Close()
	anmelden(t, ts, berndToken, placeID, nil).Body.Close()
	jetzt := faellig(srv)
	takt(t, srv)

	ns := meineBenachrichtigungen(t, ts, annaToken)
	if len(ns.Notifications) == 0 {
		// Bernd war zuerst dran (gleiche Ausgangslage, Kennung entscheidet).
		ns = meineBenachrichtigungen(t, ts, berndToken)
	}
	if len(ns.Notifications) == 0 {
		t.Fatal("niemand wurde gefragt")
	}
	vorgangID := ns.Notifications[0].AssignmentID

	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/assignments/%d/claim", vorgangID), annaToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Zusage: Status %d", resp.StatusCode)
	}
	zusage := decode[map[string]any](t, resp)
	if zusage["state"] != string(model.AssignmentClaimed) {
		t.Fatalf("Stand nach Zusage = %v", zusage["state"])
	}
	if zusage["claimedUntil"] == nil {
		t.Fatal("Zusage ohne Frist")
	}
	if bis, err := time.Parse(time.RFC3339, zusage["claimedUntil"].(string)); err != nil ||
		!bis.Equal(jetzt.Add(24*time.Hour)) {
		t.Fatalf("Zusage läuft bis %v, erwartet %v", zusage["claimedUntil"], jetzt.Add(24*time.Hour))
	}

	// Andere sehen „übernommen von … bis …" in der Orts-Liste.
	liste := decode[vergabePlaces](t, doReq(t, "GET", ts.URL+"/api/v1/places", berndToken, nil))
	var gefunden bool
	for _, p := range liste.Places {
		for _, task := range p.Tasks {
			if task.ID != taskID {
				continue
			}
			gefunden = true
			if task.Assignment == nil || task.Assignment.ClaimedBy != "anna-sub" {
				t.Fatalf("Vergabestand = %+v", task.Assignment)
			}
			if task.Assignment.ClaimedByName != "Anna" {
				t.Errorf("Name des Zusagenden = %q, erwartet Anna", task.Assignment.ClaimedByName)
			}
			if !task.SignedUp || task.SignupCount != 2 {
				t.Errorf("Anmeldungen: signedUp=%v count=%d", task.SignedUp, task.SignupCount)
			}
		}
	}
	if !gefunden {
		t.Fatal("Aufgabe nicht in der Orts-Liste")
	}

	// Zweite Zusage prallt ab.
	resp = doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/assignments/%d/claim", vorgangID), berndToken, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("zweite Zusage: Status %d, erwartet 409", resp.StatusCode)
	}
	konflikt := decode[map[string]any](t, resp)
	if text, _ := konflikt["error"].(string); text == "" {
		t.Fatal("409 ohne Erklärung")
	}

	// Fremde können die Zusage nicht zurückgeben, die Person selbst schon.
	resp = doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/assignments/%d/release", vorgangID), berndToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("fremde Rückgabe: Status %d, erwartet 403", resp.StatusCode)
	}
	resp.Body.Close()
	resp = doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/assignments/%d/release", vorgangID), annaToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Rückgabe: Status %d", resp.StatusCode)
	}
	frei := decode[map[string]any](t, resp)
	if frei["claimedBy"] != nil && frei["claimedBy"] != "" {
		t.Fatalf("nach der Rückgabe noch vergeben: %v", frei["claimedBy"])
	}
}

// Admins dürfen eine Zusage aufheben (Verwaltung, MCP).
func TestAdminHebtZusageAuf(t *testing.T) {
	ts, srv := newTestServer(t)
	placeID, _ := createPlaceWithTask(t, ts)
	anmelden(t, ts, annaToken, placeID, nil).Body.Close()
	faellig(srv)
	takt(t, srv)

	ns := meineBenachrichtigungen(t, ts, annaToken)
	vorgangID := ns.Notifications[0].AssignmentID
	doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/assignments/%d/claim", vorgangID), annaToken, nil).Body.Close()

	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/assignments/%d/release", vorgangID), adminToken, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Admin-Rückgabe: Status %d", resp.StatusCode)
	}
	resp.Body.Close()

	ns = meineBenachrichtigungen(t, ts, annaToken)
	hinweis := false
	for _, n := range ns.Notifications {
		if n.Kind == string(model.NotifyClaimRevoked) {
			hinweis = true
		}
	}
	if !hinweis {
		t.Fatalf("kein Hinweis auf die aufgehobene Zusage: %+v", ns.Notifications)
	}
}

// Zwei gleichzeitige Zusagen über HTTP: genau eine gewinnt.
func TestNebenlaeufigeZusagenUeberHTTP(t *testing.T) {
	ts, srv := newTestServer(t)
	placeID, _ := createPlaceWithTask(t, ts)
	anmelden(t, ts, annaToken, placeID, nil).Body.Close()
	anmelden(t, ts, berndToken, placeID, nil).Body.Close()
	faellig(srv)
	takt(t, srv)

	var vorgangID int64
	for _, token := range []string{annaToken, berndToken} {
		if ns := meineBenachrichtigungen(t, ts, token); len(ns.Notifications) > 0 {
			vorgangID = ns.Notifications[0].AssignmentID
		}
	}
	if vorgangID == 0 {
		t.Fatal("kein Vorgang")
	}

	var wg sync.WaitGroup
	stati := make([]int, 2)
	for i, token := range []string{annaToken, berndToken} {
		wg.Add(1)
		go func(i int, token string) {
			defer wg.Done()
			resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/assignments/%d/claim", vorgangID), token, nil)
			defer resp.Body.Close()
			stati[i] = resp.StatusCode
		}(i, token)
	}
	wg.Wait()

	erfolge, konflikte := 0, 0
	for _, s := range stati {
		switch s {
		case http.StatusOK:
			erfolge++
		case http.StatusConflict:
			konflikte++
		}
	}
	if erfolge != 1 || konflikte != 1 {
		t.Fatalf("Status = %v, erwartet genau ein 200 und ein 409", stati)
	}
}

// Meldet irgendwer die Erledigung, ist der Vorgang sofort vorbei.
func TestErledigungBeendetVorgang(t *testing.T) {
	ts, srv := newTestServer(t)
	placeID, taskID := createPlaceWithTask(t, ts)
	anmelden(t, ts, annaToken, placeID, nil).Body.Close()
	faellig(srv)
	takt(t, srv)

	if ns := meineBenachrichtigungen(t, ts, annaToken); len(ns.Notifications) != 1 {
		t.Fatalf("keine Anfrage: %+v", ns.Notifications)
	}

	// Ein Unbeteiligter gießt.
	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), memberToken,
		map[string]any{"liters": 10})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Erledigung: Status %d", resp.StatusCode)
	}
	resp.Body.Close()

	if ns := meineBenachrichtigungen(t, ts, annaToken); len(ns.Notifications) != 0 {
		t.Fatalf("Anfrage nach der Erledigung noch offen: %+v", ns.Notifications)
	}
	liste := decode[vergabePlaces](t, doReq(t, "GET", ts.URL+"/api/v1/places", annaToken, nil))
	for _, p := range liste.Places {
		for _, task := range p.Tasks {
			if task.ID == taskID && task.Assignment != nil {
				t.Fatalf("Vorgang läuft weiter: %+v", task.Assignment)
			}
		}
	}
	// Auch der nächste Takt fragt niemanden mehr.
	takt(t, srv)
	if ns := meineBenachrichtigungen(t, ts, annaToken); len(ns.Notifications) != 0 {
		t.Fatalf("nach der Erledigung neue Anfrage: %+v", ns.Notifications)
	}
}

func TestVergabeEinstellungen(t *testing.T) {
	ts, _ := newTestServer(t)

	stand := decode[map[string]any](t, doReq(t, "GET", ts.URL+"/api/v1/settings", memberToken, nil))
	a, ok := stand["assignment"].(map[string]any)
	if !ok {
		t.Fatalf("Einstellungen ohne Vergabe-Block: %v", stand)
	}
	if a["offerMinutes"] != 60.0 || a["claimHours"] != 24.0 || a["quietFrom"] != 21.0 || a["quietTo"] != 7.0 {
		t.Fatalf("Vorgaben = %v", a)
	}

	resp := doReq(t, "PUT", ts.URL+"/api/v1/settings", adminToken, map[string]any{
		"assignment": map[string]any{"offerMinutes": 30, "claimHours": 12, "quietFrom": 22, "quietTo": 6},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Einstellungen setzen: Status %d", resp.StatusCode)
	}
	resp.Body.Close()

	stand = decode[map[string]any](t, doReq(t, "GET", ts.URL+"/api/v1/settings", memberToken, nil))
	a = stand["assignment"].(map[string]any)
	if a["offerMinutes"] != 30.0 || a["claimHours"] != 12.0 || a["quietFrom"] != 22.0 {
		t.Fatalf("gesetzte Werte = %v", a)
	}
	// Der Hitzefaktor bleibt unangetastet.
	if stand["wateringFactor"] != 1.0 {
		t.Fatalf("Hitzefaktor = %v", stand["wateringFactor"])
	}

	resp = doReq(t, "PUT", ts.URL+"/api/v1/settings", adminToken, map[string]any{
		"assignment": map[string]any{"offerMinutes": 0, "claimHours": 12, "quietFrom": 22, "quietTo": 6},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unsinniger Abstand: Status %d, erwartet 400", resp.StatusCode)
	}
	resp.Body.Close()
}

// Ohne Anmeldung keine Zusage — auch wenn man die Vorgangs-Nummer kennt.
func TestZusageOhneAnmeldungAbgewiesen(t *testing.T) {
	ts, srv := newTestServer(t)
	placeID, _ := createPlaceWithTask(t, ts)
	anmelden(t, ts, annaToken, placeID, nil).Body.Close()
	faellig(srv)
	takt(t, srv)
	vorgangID := meineBenachrichtigungen(t, ts, annaToken).Notifications[0].AssignmentID

	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/assignments/%d/claim", vorgangID), memberToken, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Zusage ohne Anmeldung: Status %d, erwartet 403", resp.StatusCode)
	}
	resp.Body.Close()

	resp = doReq(t, "POST", ts.URL+"/api/v1/assignments/999999/claim", annaToken, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unbekannter Vorgang: Status %d, erwartet 404", resp.StatusCode)
	}
	resp.Body.Close()
}
