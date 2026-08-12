package api

import (
	"fmt"
	"net/http"
	"testing"
	"time"
)

// Spielschutz an der API: die Sperrfrist muss serverseitig greifen, sonst
// lässt sie sich mit einem eigenen Client umgehen.

func TestCompletionCooldown(t *testing.T) {
	ts, srv := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts) // Gießen, alle 7 Tage → 3,5 Tage Sperre
	pfad := fmt.Sprintf("/api/v1/tasks/%d/completions", taskID)
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	if resp := doReq(t, "POST", ts.URL+pfad, memberToken, map[string]any{"liters": 10}); resp.StatusCode != http.StatusCreated {
		t.Fatalf("erste Meldung: Status %d", resp.StatusCode)
	}

	// Sofort nochmal → 409 mit Zeitpunkt, ab wann es wieder geht.
	resp := doReq(t, "POST", ts.URL+pfad, memberToken, map[string]any{"liters": 10})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("zweite Meldung: Status %d, erwartet 409", resp.StatusCode)
	}
	body := decode[map[string]any](t, resp)
	if body["error"] == nil || body["retryAfter"] == nil {
		t.Fatalf("409 ohne Erklärung: %v", body)
	}
	frei, err := time.Parse(time.RFC3339, body["retryAfter"].(string))
	if err != nil {
		t.Fatalf("retryAfter unlesbar: %v", body["retryAfter"])
	}
	if want := start.Add(84 * time.Hour); !frei.Equal(want) {
		t.Errorf("retryAfter = %v, erwartet %v", frei, want)
	}

	// Auch ein anderer Nutzer darf nicht nachmelden — die Sperre hängt an der Aufgabe.
	if resp := doReq(t, "POST", ts.URL+pfad, otherMemberToken, nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("fremde Nachmeldung: Status %d, erwartet 409", resp.StatusCode)
	}

	// Kurz vor Ablauf gesperrt, danach wieder erlaubt.
	srv.Now = func() time.Time { return frei.Add(-time.Second) }
	if resp := doReq(t, "POST", ts.URL+pfad, memberToken, nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("kurz vor Ablauf: Status %d, erwartet 409", resp.StatusCode)
	}
	srv.Now = func() time.Time { return frei }
	if resp := doReq(t, "POST", ts.URL+pfad, memberToken, nil); resp.StatusCode != http.StatusCreated {
		t.Errorf("nach Ablauf: Status %d, erwartet 201", resp.StatusCode)
	}

	// Eine zurückgenommene Meldung zählt auch für die Sperre nicht mehr:
	// direkt danach darf wieder gemeldet werden.
	hist := decode[struct {
		Completions []map[string]any `json:"completions"`
	}](t, doReq(t, "GET", ts.URL+pfad, memberToken, nil))
	letzte := int64(hist.Completions[0]["id"].(float64))
	if resp := doReq(t, "DELETE", ts.URL+fmt.Sprintf("/api/v1/completions/%d", letzte), memberToken, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Rücknahme: Status %d", resp.StatusCode)
	}
	srv.Now = func() time.Time { return frei.Add(time.Minute) }
	if resp := doReq(t, "POST", ts.URL+pfad, memberToken, nil); resp.StatusCode != http.StatusCreated {
		t.Errorf("nach der Rücknahme: Status %d, erwartet 201", resp.StatusCode)
	}
	// … und die neue Meldung sperrt sofort wieder.
	if resp := doReq(t, "POST", ts.URL+pfad, memberToken, nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("direkt danach: Status %d, erwartet 409", resp.StatusCode)
	}
}

func TestCooldownFollowsHeatFactor(t *testing.T) {
	ts, srv := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)
	pfad := fmt.Sprintf("/api/v1/tasks/%d/completions", taskID)
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	// Hitzewelle: Faktor 0.5 → Sperre halbiert sich auf 42 Stunden.
	doReq(t, "PUT", ts.URL+"/api/v1/settings", adminToken, map[string]any{"wateringFactor": 0.5}).Body.Close()
	doReq(t, "POST", ts.URL+pfad, memberToken, nil).Body.Close()

	srv.Now = func() time.Time { return start.Add(41 * time.Hour) }
	if resp := doReq(t, "POST", ts.URL+pfad, memberToken, nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("nach 41 h bei Hitze: Status %d, erwartet 409", resp.StatusCode)
	}
	srv.Now = func() time.Time { return start.Add(42 * time.Hour) }
	if resp := doReq(t, "POST", ts.URL+pfad, memberToken, nil); resp.StatusCode != http.StatusCreated {
		t.Errorf("nach 42 h bei Hitze: Status %d, erwartet 201", resp.StatusCode)
	}
}

func TestAdminOverrideAndBackdating(t *testing.T) {
	ts, srv := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)
	pfad := fmt.Sprintf("/api/v1/tasks/%d/completions", taskID)
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	doReq(t, "POST", ts.URL+pfad, memberToken, nil).Body.Close()

	// Mitglieder dürfen die Sperre nicht überschreiben.
	if resp := doReq(t, "POST", ts.URL+pfad, memberToken, map[string]any{"force": true}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("Mitglied mit force: Status %d, erwartet 403", resp.StatusCode)
	}

	// Admins dürfen — telefonisch gemeldete Nachträge müssen möglich bleiben.
	resp := doReq(t, "POST", ts.URL+pfad, adminToken, map[string]any{"force": true, "name": "Erna"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Admin-Override: Status %d", resp.StatusCode)
	}
	if c := decode[map[string]any](t, resp); c["forced"] != true {
		t.Errorf("Override nicht vermerkt: %v", c)
	}

	// Rückdatierung: nur Admins, höchstens ein paar Tage zurück, nie in die Zukunft.
	gestern := start.Add(-24 * time.Hour).Format(time.RFC3339)
	if resp := doReq(t, "POST", ts.URL+pfad, memberToken, map[string]any{"doneAt": gestern}); resp.StatusCode != http.StatusForbidden {
		t.Errorf("Mitglied datiert zurück: Status %d, erwartet 403", resp.StatusCode)
	}
	resp = doReq(t, "POST", ts.URL+pfad, adminToken, map[string]any{"doneAt": gestern, "force": true})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Rückdatierung: Status %d", resp.StatusCode)
	}
	if c := decode[map[string]any](t, resp); c["doneAt"] != gestern {
		t.Errorf("doneAt = %v, erwartet %v", c["doneAt"], gestern)
	}

	for name, wann := range map[string]string{
		"Zukunft":     start.Add(time.Hour).Format(time.RFC3339),
		"zu weit weg": start.Add(-30 * 24 * time.Hour).Format(time.RFC3339),
		"kaputt":      "vorgestern",
	} {
		if resp := doReq(t, "POST", ts.URL+pfad, adminToken, map[string]any{"doneAt": wann, "force": true}); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: Status %d, erwartet 400", name, resp.StatusCode)
		}
	}
	_ = srv
}

// Die Ampel-Ansicht sagt, ab wann wieder gemeldet werden darf — damit die App
// den Knopf sperren kann, statt erst in einen 409 zu laufen.
func TestPlacesShowLockedUntil(t *testing.T) {
	ts, _ := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	type antwort struct {
		Places []struct {
			Tasks []struct {
				ID          int64  `json:"id"`
				LockedUntil string `json:"lockedUntil"`
			} `json:"tasks"`
		} `json:"places"`
	}
	vorher := decode[antwort](t, doReq(t, "GET", ts.URL+"/api/v1/places", memberToken, nil))
	if vorher.Places[0].Tasks[0].LockedUntil != "" {
		t.Errorf("frische Aufgabe ist gesperrt: %v", vorher.Places[0].Tasks[0].LockedUntil)
	}

	doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), memberToken, nil).Body.Close()
	nachher := decode[antwort](t, doReq(t, "GET", ts.URL+"/api/v1/places", memberToken, nil))
	blockiert, err := time.Parse(time.RFC3339, nachher.Places[0].Tasks[0].LockedUntil)
	if err != nil {
		t.Fatalf("lockedUntil fehlt: %v", nachher.Places[0].Tasks[0])
	}
	if want := start.Add(84 * time.Hour); !blockiert.Equal(want) {
		t.Errorf("lockedUntil = %v, erwartet %v", blockiert, want)
	}
}
