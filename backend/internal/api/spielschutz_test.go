package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Lücken im Spielschutz, die sich nur serverseitig schließen lassen.
// Alles läuft über die echte HTTP-API gegen eine echte SQLite-Datenbank.

// TestHeatFactorDoesNotShortenWeedingCooldown: der Hitzefaktor gilt nur fürs
// Gießen. Beim Jäten darf er die Sperre nicht verkürzen — sonst ließe sich
// mit einer Wetter-Einstellung die halbe Sperrfrist aller Aufgaben abräumen.
func TestHeatFactorDoesNotShortenWeedingCooldown(t *testing.T) {
	ts, srv := newTestServer(t)
	placeID, giessen := createPlaceWithTask(t, ts)
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/places/%d/tasks", placeID), adminToken,
		map[string]any{"kind": "jaeten", "intervalDays": 7, "redAfterDays": 14})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Jätaufgabe: Status %d", resp.StatusCode)
	}
	jaeten := int64(decode[map[string]any](t, resp)["id"].(float64))

	// Hitzewelle: halbiert die Gieß-Sperre von 84 auf 42 Stunden.
	doReq(t, "PUT", ts.URL+"/api/v1/settings", adminToken, map[string]any{"wateringFactor": 0.5}).Body.Close()
	for _, id := range []int64{giessen, jaeten} {
		if resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", id), memberToken, nil); resp.StatusCode != http.StatusCreated {
			t.Fatalf("erste Meldung auf %d: Status %d", id, resp.StatusCode)
		}
	}

	// Nach 42 Stunden ist Gießen wieder frei, Jäten noch gesperrt.
	srv.Now = func() time.Time { return start.Add(42 * time.Hour) }
	for _, task := range sperrSicht(t, ts).Places[0].Tasks {
		gesperrt := task.LockedUntil != ""
		if task.Kind == "jaeten" && !gesperrt {
			t.Errorf("Jäten nach 42 h nicht mehr gesperrt (lockedUntil fehlt)")
		}
		if task.Kind == "giessen" && gesperrt {
			t.Errorf("Gießen bei Hitze nach 42 h noch gesperrt bis %s", task.LockedUntil)
		}
	}
	if resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", jaeten), memberToken, nil); resp.StatusCode != http.StatusConflict {
		t.Errorf("Jäten nach 42 h: Status %d, erwartet 409 (Hitzefaktor gilt nur fürs Gießen)", resp.StatusCode)
	}
	if resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", giessen), memberToken, nil); resp.StatusCode != http.StatusCreated {
		t.Errorf("Gießen nach 42 h bei Hitze: Status %d, erwartet 201", resp.StatusCode)
	}

	// Nach 84 Stunden ist auch das Jäten wieder dran.
	srv.Now = func() time.Time { return start.Add(84 * time.Hour) }
	if resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", jaeten), memberToken, nil); resp.StatusCode != http.StatusCreated {
		t.Errorf("Jäten nach 84 h: Status %d, erwartet 201", resp.StatusCode)
	}
}

// TestConcurrentCompletionsOnlyOneWins: zwei Meldungen im selben Augenblick
// (Doppeltipp, wackeliges Mobilfunknetz mit Wiederholung) dürfen nicht beide
// durchrutschen. Prüfen und Eintragen muss zusammen atomar sein.
func TestConcurrentCompletionsOnlyOneWins(t *testing.T) {
	ts, _ := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)
	url := ts.URL + fmt.Sprintf("/api/v1/tasks/%d/completions", taskID)

	const gleichzeitig = 8
	var (
		start          sync.WaitGroup
		fertig         sync.WaitGroup
		mu             sync.Mutex
		status         = map[int]int{}
		fehler         []error
		angelegt       int
		blockiert      int
		unerwarteteAnt []int
	)
	start.Add(1)
	for i := 0; i < gleichzeitig; i++ {
		fertig.Add(1)
		go func() {
			defer fertig.Done()
			start.Wait()
			req, err := http.NewRequest("POST", url, bytes.NewBufferString(`{"liters":10}`))
			if err == nil {
				req.Header.Set("Authorization", "Bearer "+memberToken)
				var resp *http.Response
				resp, err = http.DefaultClient.Do(req)
				if err == nil {
					resp.Body.Close()
					mu.Lock()
					status[resp.StatusCode]++
					mu.Unlock()
				}
			}
			if err != nil {
				mu.Lock()
				fehler = append(fehler, err)
				mu.Unlock()
			}
		}()
	}
	start.Done()
	fertig.Wait()
	if len(fehler) > 0 {
		t.Fatalf("Anfragen fehlgeschlagen: %v", fehler)
	}
	for code, n := range status {
		switch code {
		case http.StatusCreated:
			angelegt = n
		case http.StatusConflict:
			blockiert = n
		default:
			unerwarteteAnt = append(unerwarteteAnt, code)
		}
	}
	if len(unerwarteteAnt) > 0 {
		t.Fatalf("unerwartete Antworten: %v (alle: %v)", unerwarteteAnt, status)
	}
	if angelegt != 1 || blockiert != gleichzeitig-1 {
		t.Errorf("%d Meldungen angelegt und %d abgewiesen, erwartet 1 und %d", angelegt, blockiert, gleichzeitig-1)
	}

	hist := decode[struct {
		Completions []map[string]any `json:"completions"`
	}](t, doReq(t, "GET", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), memberToken, nil))
	if len(hist.Completions) != 1 {
		t.Errorf("Historie: %d Einträge, erwartet 1", len(hist.Completions))
	}
}

// TestCooldownMessageUsesVillageTime: die Meldung im 409 nennt einen
// Zeitpunkt — der muss in Ortszeit des Dorfes stehen. Der Server läuft in
// UTC; ohne Umrechnung stünde dort im Sommer eine um zwei Stunden falsche
// Uhrzeit („wieder ab 00:00" statt „02:00").
func TestCooldownMessageUsesVillageTime(t *testing.T) {
	// Die Zeitzone des Servers darf keine Rolle spielen.
	alteZone := time.Local
	time.Local = time.FixedZone("Testzone", -7*3600)
	t.Cleanup(func() { time.Local = alteZone })

	ts, _ := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)
	pfad := fmt.Sprintf("/api/v1/tasks/%d/completions", taskID)

	doReq(t, "POST", ts.URL+pfad, memberToken, nil).Body.Close()
	resp := doReq(t, "POST", ts.URL+pfad, memberToken, nil)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("zweite Meldung: Status %d, erwartet 409", resp.StatusCode)
	}
	body := decode[map[string]any](t, resp)
	frei, err := time.Parse(time.RFC3339, body["retryAfter"].(string))
	if err != nil {
		t.Fatalf("retryAfter unlesbar: %v", body["retryAfter"])
	}
	// Rössing liegt in Europe/Berlin: 16.08.2026, 00:00 UTC = 02:00 Ortszeit.
	want := frei.In(model.Location()).Format("02.01.2006, 15:04")
	if meldung, _ := body["error"].(string); !strings.Contains(meldung, want) {
		t.Errorf("Meldung %q nennt nicht die Ortszeit %q", meldung, want)
	}
}

// TestBackdatingWindow: telefonisch gemeldete Erledigungen müssen sich
// nachtragen lassen — bis zu 14 Tage zurück, nie in die Zukunft.
func TestBackdatingWindow(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID, _ := createPlaceWithTask(t, ts)
	start := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)

	// Je Fall eine eigene Aufgabe, damit sich die Fälle nicht gegenseitig sperren.
	neueAufgabe := func() int64 {
		t.Helper()
		resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/places/%d/tasks", placeID), adminToken,
			map[string]any{"kind": "giessen", "intervalDays": 7, "redAfterDays": 14})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Aufgabe anlegen: Status %d", resp.StatusCode)
		}
		return int64(decode[map[string]any](t, resp)["id"].(float64))
	}

	for name, f := range map[string]struct {
		zurueck time.Duration
		status  int
	}{
		"13 Tage zurück":       {13 * 24 * time.Hour, http.StatusCreated},
		"genau 14 Tage":        {14 * 24 * time.Hour, http.StatusCreated},
		"14 Tage und 1 Minute": {14*24*time.Hour + time.Minute, http.StatusBadRequest},
	} {
		wann := start.Add(-f.zurueck).Format(time.RFC3339)
		resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", neueAufgabe()), adminToken,
			map[string]any{"doneAt": wann})
		if resp.StatusCode != f.status {
			t.Errorf("%s (%s): Status %d, erwartet %d", name, wann, resp.StatusCode, f.status)
		}
	}
}

// --- Wertung in der Rangliste ------------------------------------------------

// TestLeaderboardCountsOnlyRealCompletions: gewertet wird nur, was eine
// echte Erledigung sein kann. Meldungen, die auf eine frisch erledigte
// Aufgabe fallen (Altbestand aus der Zeit vor dem Spielschutz), zählen nicht;
// bewusst erzwungene Nachträge eines Admins zählen der genannten Person.
func TestLeaderboardCountsOnlyRealCompletions(t *testing.T) {
	ts, srv := newTestServer(t)
	mai := func(tag, stunde int) time.Time {
		return time.Date(2026, time.May, tag, stunde, 0, 0, 0, time.UTC)
	}
	_, giessen := createPlaceAt(t, ts, srv, mai(1, 6)) // alle 7 Tage → 3,5 Tage Sperre

	// Erna gießt zweimal in ordentlichem Abstand.
	reportAt(t, ts, srv, giessen, ernaToken, mai(4, 9), 10)
	reportAt(t, ts, srv, giessen, ernaToken, mai(12, 9), 10)

	// Altbestand: zwei Meldungen von Karl kurz nach Ernas erster — so etwas
	// steht noch aus der Zeit vor dem Spielschutz in der Datenbank.
	for _, stunde := range []int{10, 11} {
		liter := 7.0
		if err := srv.DB.InsertCompletion(&model.Completion{
			TaskID: giessen, UserSub: "karl", UserName: "Karl",
			Liters: &liter, DoneAt: mai(4, stunde),
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Der Admin trägt für Berta telefonisch nach — trotz laufender Sperre.
	forceReport(t, ts, srv, giessen, mai(12, 11), map[string]any{
		"force": true, "name": "Berta", "liters": 5,
	})

	lb := leaderboardAt(t, ts, srv, ernaToken, "?period=gesamt", mai(20, 12))
	gezaehlt := map[string]int{}
	for _, e := range lb.Entries {
		gezaehlt[e.UserName] = e.Completions
	}
	if gezaehlt["Erna"] != 2 {
		t.Errorf("Erna hat %d Erledigungen, erwartet 2", gezaehlt["Erna"])
	}
	if gezaehlt["Berta"] != 1 {
		t.Errorf("Berta (Nachtrag des Admins) hat %d Erledigungen, erwartet 1", gezaehlt["Berta"])
	}
	if n, da := gezaehlt["Karl"]; da {
		t.Errorf("Karls Doppelmeldungen zählen mit %d, erwartet gar nicht", n)
	}
	if lb.Totals.Completions != 3 {
		t.Errorf("Gesamtsumme = %d, erwartet 3", lb.Totals.Completions)
	}
	if lb.Totals.Participants != 2 {
		t.Errorf("Beteiligte = %d, erwartet 2", lb.Totals.Participants)
	}
	if lb.Totals.Liters != 25 {
		t.Errorf("Liter gesamt = %v, erwartet 25 (Karls 14 l zählen nicht)", lb.Totals.Liters)
	}

	// Auch die Auszeichnung „Gießkanne des Monats" folgt der Wertung: Karl
	// hätte roh die meisten Meldungen, gewertet ist es Erna.
	monat := leaderboardAt(t, ts, srv, ernaToken, "?period=monat", mai(20, 12))
	for _, e := range monat.Entries {
		if e.UserName == "Erna" && !e.hasBadge(model.BadgeWateringCan) {
			t.Errorf("Erna fehlt die Gießkanne des Monats: %+v", e.Badges)
		}
		if e.UserName == "Karl" {
			t.Errorf("Karl steht trotz ungewerteter Meldungen in der Monatsliste: %+v", e)
		}
	}

	// Die Historie zeigt die Meldungen weiterhin alle — der Nachtrag ist als
	// solcher gekennzeichnet.
	hist := decode[struct {
		Completions []struct {
			UserName string `json:"userName"`
			Forced   bool   `json:"forced"`
		} `json:"completions"`
	}](t, doReq(t, "GET", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", giessen), adminToken, nil))
	if len(hist.Completions) != 5 {
		t.Fatalf("Historie: %d Einträge, erwartet 5 (die Wertung löscht nichts)", len(hist.Completions))
	}
	for _, c := range hist.Completions {
		if (c.UserName == "Berta") != c.Forced {
			t.Errorf("Kennzeichnung falsch: %s forced=%v", c.UserName, c.Forced)
		}
	}
}

// forceReport meldet als Admin mit Sonderrechten (force, name, doneAt).
func forceReport(t *testing.T, ts *httptest.Server, srv *Server, taskID int64, at time.Time, body map[string]any) {
	t.Helper()
	alt := srv.Now
	srv.Now = func() time.Time { return at }
	defer func() { srv.Now = alt }()
	url := ts.URL + fmt.Sprintf("/api/v1/tasks/%d/completions", taskID)
	roh, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", url, bytes.NewReader(roh))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Nachtrag: Status %d", resp.StatusCode)
	}
}

// sperrenAntwort ist die Sicht auf /places, die für den Spielschutz zählt.
type sperrenAntwort struct {
	Places []struct {
		Tasks []struct {
			ID          int64  `json:"id"`
			Kind        string `json:"kind"`
			LockedUntil string `json:"lockedUntil"`
		} `json:"tasks"`
	} `json:"places"`
}

func sperrSicht(t *testing.T, ts *httptest.Server) sperrenAntwort {
	t.Helper()
	return decode[sperrenAntwort](t, doReq(t, "GET", ts.URL+"/api/v1/places", memberToken, nil))
}
