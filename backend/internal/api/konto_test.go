package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Tests der Kontolöschung (DELETE /api/v1/me).
//
// Der Kern ist der Zielkonflikt: Die Person muss verschwinden, die
// Gesamtsummen des Dorfes dürfen sich dabei nicht ändern. Deshalb wird hier
// beides in einem Durchgang geprüft — was weg ist und was bleibt.

// kontoErna richtet ein Konto mit Spuren in allen Tabellen ein und liefert
// die Kennung der Aufgabe zurück, an der Erna gemeldet hat.
func kontoErna(t *testing.T, ts *httptest.Server, srv *Server) (placeID, taskID int64) {
	t.Helper()
	placeID, taskID = createPlaceWithTask(t, ts)

	// Profil anlegen (entsteht beim ersten Blick auf sich selbst).
	meinProfil(t, ts, profilErna)

	// Eine Erledigung — die bleibt, anonym.
	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID),
		profilErna, map[string]any{"liters": 10})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Meldung: Status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Eine Helfer-Eintragung.
	resp = doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/places/%d/signup", placeID), profilErna, nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Anmeldung: Status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Eine Gerätekennung.
	resp = doReq(t, "POST", ts.URL+"/api/v1/me/devices", profilErna,
		map[string]any{"token": "geraet-erna", "platform": "ios"})
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		t.Fatalf("Gerät anmelden: Status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Eine Zustellung im Postfach.
	n := model.Notification{
		AssignmentID: 1, TaskID: taskID, PlaceID: placeID, UserSub: "erna-sub",
		Kind: model.NotifyRequest, CreatedAt: srv.now(),
		PlaceName: "Unter den Eichen", TaskName: "Gießen",
	}
	if err := srv.DB.InsertNotification(&n); err != nil {
		t.Fatal(err)
	}
	return placeID, taskID
}

// TestKontoLoeschenRaeumtDieEigenenDatenWeg: Nach dem Löschen ist von der
// Person nichts mehr da — kein Profil, kein Gerät, keine Anmeldung, kein
// Postfach.
func TestKontoLoeschenRaeumtDieEigenenDatenWeg(t *testing.T) {
	ts, srv := newTestServer(t)
	kontoErna(t, ts, srv)

	resp := doReq(t, "DELETE", ts.URL+"/api/v1/me", profilErna, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /me: Status %d", resp.StatusCode)
	}
	antwort := decode[map[string]any](t, resp)
	if antwort["geloescht"] != true {
		t.Errorf("Antwort meldet die Löschung nicht: %v", antwort)
	}
	// Die Antwort muss die Rössing-ID erwähnen — sonst glaubt jemand, mit
	// diesem einen Knopf sei auch die Anmeldung fürs Dorf weg.
	hinweis, _ := antwort["roessingId"].(string)
	if hinweis == "" {
		t.Error("Antwort ohne Hinweis auf die Rössing-ID")
	}

	if _, err := srv.DB.GetProfile("erna-sub"); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Profil noch da (Fehler: %v)", err)
	}
	geraete, err := srv.DB.DevicesForUser("erna-sub")
	if err != nil {
		t.Fatal(err)
	}
	if len(geraete) != 0 {
		t.Errorf("noch %d Gerätekennungen übrig", len(geraete))
	}
	anmeldungen, err := srv.DB.ListSignupsByUser("erna-sub")
	if err != nil {
		t.Fatal(err)
	}
	if len(anmeldungen) != 0 {
		t.Errorf("noch %d Helfer-Eintragungen übrig", len(anmeldungen))
	}
	offen, err := srv.DB.OpenNotifications("erna-sub")
	if err != nil {
		t.Fatal(err)
	}
	if len(offen) != 0 {
		t.Errorf("noch %d Benachrichtigungen übrig", len(offen))
	}
}

// TestKontoLoeschenLaesstErledigungenAnonymStehen: Die Zeile bleibt — sonst
// stimmten die Gesamtsummen des Dorfes und die Historie des Ortes nicht mehr
// —, aber sie trägt weder Namen noch Kennung.
func TestKontoLoeschenLaesstErledigungenAnonymStehen(t *testing.T) {
	ts, srv := newTestServer(t)
	_, taskID := kontoErna(t, ts, srv)

	vorher, err := srv.DB.ListCompletions(taskID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(vorher) != 1 {
		t.Fatalf("Vorbedingung: %d Erledigungen, erwartet 1", len(vorher))
	}

	resp := doReq(t, "DELETE", ts.URL+"/api/v1/me", profilErna, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /me: Status %d", resp.StatusCode)
	}
	resp.Body.Close()

	nachher, err := srv.DB.ListCompletions(taskID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(nachher) != 1 {
		t.Fatalf("%d Erledigungen nach dem Löschen, erwartet 1 (anonym)", len(nachher))
	}
	if nachher[0].UserSub != "" {
		t.Errorf("Erledigung trägt noch die Kennung %q", nachher[0].UserSub)
	}
	if nachher[0].UserName != LoeschErsatzname {
		t.Errorf("userName = %q, erwartet %q", nachher[0].UserName, LoeschErsatzname)
	}
	if nachher[0].Liters == nil || *nachher[0].Liters != 10 {
		t.Errorf("Liter verändert: %v — die Bilanz des Dorfes muss stehen bleiben", nachher[0].Liters)
	}

	// Und in der Rangliste taucht der alte Name nicht mehr auf.
	resp = doReq(t, "GET", ts.URL+"/api/v1/stats/leaderboard?period=gesamt", adminToken, nil)
	liste := decode[map[string]any](t, resp)
	eintraege, _ := liste["entries"].([]any)
	if len(eintraege) != 1 {
		t.Fatalf("%d Ranglisten-Zeilen, erwartet 1", len(eintraege))
	}
	zeile, _ := eintraege[0].(map[string]any)
	if zeile["userName"] != LoeschErsatzname {
		t.Errorf("Rangliste zeigt %v, erwartet %q", zeile["userName"], LoeschErsatzname)
	}
	if zeile["userSub"] != "" {
		t.Errorf("Rangliste zeigt noch die Kennung %v", zeile["userSub"])
	}
}

// TestKontoLoeschenNurDasEigene: Eine fremde Kennung im Rumpf ergibt 403 —
// auch für Verwaltende. Dieselbe Regel wie bei PUT /api/v1/me/profile.
func TestKontoLoeschenNurDasEigene(t *testing.T) {
	ts, srv := newTestServer(t)
	kontoErna(t, ts, srv)

	for name, token := range map[string]string{
		"Mitglied":    profilKarl,
		"Verwaltende": profilChefin,
	} {
		resp := doReq(t, "DELETE", ts.URL+"/api/v1/me", token,
			map[string]any{"userSub": "erna-sub"})
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s löscht fremdes Konto: Status %d, erwartet 403", name, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Ernas Konto steht unverändert da.
	if _, err := srv.DB.GetProfile("erna-sub"); err != nil {
		t.Errorf("Ernas Profil ist weg, obwohl der Versuch abgelehnt wurde: %v", err)
	}
}

// TestKontoLoeschenZweimalIstUnschaedlich: Der zweite Aufruf findet nichts
// mehr vor und meldet trotzdem Erfolg. Wichtig für ein wackeliges Netz — die
// App wiederholt den Aufruf, wenn die Antwort verloren geht.
func TestKontoLoeschenZweimalIstUnschaedlich(t *testing.T) {
	ts, srv := newTestServer(t)
	_, taskID := kontoErna(t, ts, srv)

	for durchgang := 1; durchgang <= 2; durchgang++ {
		resp := doReq(t, "DELETE", ts.URL+"/api/v1/me", profilErna, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Durchgang %d: Status %d, erwartet 200", durchgang, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Auch nach zweimaligem Löschen steht die Erledigung genau einmal da und
	// heißt weiterhin so, wie sie beim ersten Mal umbenannt wurde.
	nachher, err := srv.DB.ListCompletions(taskID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(nachher) != 1 || nachher[0].UserName != LoeschErsatzname {
		t.Errorf("Erledigungen nach zwei Durchgängen: %+v", nachher)
	}
}
