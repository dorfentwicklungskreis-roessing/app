package api

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Tests der Geräteverwaltung: Die App meldet ihr Push-Token an und beim
// Abmelden wieder ab. Die Kennung selbst ist nichts, was jemand außer dem
// Server sehen müsste — sie kommt in keiner Antwort vor.

type geraetAntwort struct {
	Platform  string `json:"platform"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Token     string `json:"token"`
}

func TestGeraetAnmeldenUndAuffrischen(t *testing.T) {
	ts, srv := newTestServer(t)

	resp := doReq(t, "POST", ts.URL+"/api/v1/me/devices", annaToken,
		map[string]string{"token": "fcm-token-anna", "platform": "android"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Anmelden: Status %d", resp.StatusCode)
	}
	g := decode[geraetAntwort](t, resp)
	if g.Platform != "android" || g.UpdatedAt == "" {
		t.Errorf("unerwartete Antwort: %+v", g)
	}
	if g.Token != "" {
		t.Error("die Gerätekennung darf nicht zurückgeschickt werden")
	}

	// Auffrischen beim nächsten Start: kein zweites Gerät, kein 201.
	resp = doReq(t, "POST", ts.URL+"/api/v1/me/devices", annaToken,
		map[string]string{"token": "fcm-token-anna", "platform": "android"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Auffrischen: Status %d", resp.StatusCode)
	}
	geraete, err := srv.DB.DevicesForUser("anna-sub")
	if err != nil {
		t.Fatal(err)
	}
	if len(geraete) != 1 {
		t.Fatalf("erwartet 1 Gerät, bekommen %d", len(geraete))
	}
}

func TestGeraetAbmelden(t *testing.T) {
	ts, srv := newTestServer(t)
	doReq(t, "POST", ts.URL+"/api/v1/me/devices", annaToken,
		map[string]string{"token": "fcm-token-anna", "platform": "android"})

	// Bernd kann Annas Gerät nicht abmelden.
	resp := doReq(t, "DELETE", ts.URL+"/api/v1/me/devices", berndToken,
		map[string]string{"token": "fcm-token-anna"})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Abmelden ist immer erfolgreich: Status %d", resp.StatusCode)
	}
	if geraete, _ := srv.DB.DevicesForUser("anna-sub"); len(geraete) != 1 {
		t.Fatal("fremdes Abmelden hat Annas Gerät entfernt")
	}

	resp = doReq(t, "DELETE", ts.URL+"/api/v1/me/devices", annaToken,
		map[string]string{"token": "fcm-token-anna"})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Abmelden: Status %d", resp.StatusCode)
	}
	if geraete, _ := srv.DB.DevicesForUser("anna-sub"); len(geraete) != 0 {
		t.Fatal("Gerät wurde nicht abgemeldet")
	}
}

// Manche HTTP-Werkzeuge schicken bei DELETE keinen Rumpf mit — dann steht
// die Kennung in der Abfrage.
func TestGeraetAbmeldenPerAbfrage(t *testing.T) {
	ts, srv := newTestServer(t)
	doReq(t, "POST", ts.URL+"/api/v1/me/devices", annaToken,
		map[string]string{"token": "fcm-token-anna", "platform": "android"})

	resp := doReq(t, "DELETE", ts.URL+"/api/v1/me/devices?token=fcm-token-anna", annaToken, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Abmelden: Status %d", resp.StatusCode)
	}
	if geraete, _ := srv.DB.DevicesForUser("anna-sub"); len(geraete) != 0 {
		t.Fatal("Gerät wurde nicht abgemeldet")
	}
}

func TestGeraetOhneKennungAbgewiesen(t *testing.T) {
	ts, _ := newTestServer(t)
	for _, fall := range []struct {
		name string
		body map[string]string
	}{
		{"leer", map[string]string{"token": "", "platform": "android"}},
		{"nur Leerzeichen", map[string]string{"token": "   "}},
		{"zu lang", map[string]string{"token": strings.Repeat("x", 5000)}},
	} {
		t.Run(fall.name, func(t *testing.T) {
			resp := doReq(t, "POST", ts.URL+"/api/v1/me/devices", annaToken, fall.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("Status %d, erwartet 400", resp.StatusCode)
			}
		})
	}
}

// mitschrift merkt sich, was der Zusteller zu sehen bekommen hat.
type mitschrift struct {
	gesehen []model.Notification
}

func (m *mitschrift) Zustellen(n model.Notification) error {
	m.gesehen = append(m.gesehen, n)
	return nil
}

// Auch die Benachrichtigungen, die aus einem API-Aufruf entstehen, müssen den
// Push-Weg nehmen — sonst erführe jemand erst beim nächsten Öffnen der App,
// dass die Verwaltung seine Zusage aufgehoben hat.
func TestPushBeimAufhebenDerZusage(t *testing.T) {
	ts, srv := newTestServer(t)
	post := &mitschrift{}
	srv.Zusteller = post
	placeID, _ := createPlaceWithTask(t, ts)
	faellig(srv)

	anmelden(t, ts, annaToken, placeID, nil)
	takt(t, srv)
	ns := meineBenachrichtigungen(t, ts, annaToken)
	if len(ns.Notifications) == 0 {
		t.Fatal("keine Anfrage erzeugt")
	}
	vorgang := ns.Notifications[0].AssignmentID
	if resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/assignments/%d/claim", vorgang), annaToken, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("Zusage: Status %d", resp.StatusCode)
	}
	// Die Verwaltung hebt die Zusage auf.
	if resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/assignments/%d/release", vorgang), adminToken, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("Aufheben: Status %d", resp.StatusCode)
	}
	if len(post.gesehen) != 1 {
		t.Fatalf("erwartet 1 Push, bekommen %d", len(post.gesehen))
	}
	if post.gesehen[0].Kind != model.NotifyClaimRevoked || post.gesehen[0].UserSub != "anna-sub" {
		t.Errorf("falsche Nachricht: %+v", post.gesehen[0])
	}
	if post.gesehen[0].Title == "" || post.gesehen[0].PlaceName == "" {
		t.Error("der Zusteller braucht Titel und Ortsnamen für die Anzeige")
	}
}

// Ohne Anmeldung geht gar nichts — sonst könnte jeder fremde Kennungen
// eintragen und mitlesen.
func TestGeraetBrauchtAnmeldung(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := doReq(t, "POST", ts.URL+"/api/v1/me/devices", "",
		map[string]string{"token": "fcm-token"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Status %d, erwartet 401", resp.StatusCode)
	}
}
