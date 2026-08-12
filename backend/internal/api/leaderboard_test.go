package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Feature B: Rangliste („wer tut wie viel fürs Dorf?").
//
// Alle Tests laufen gegen eine echte SQLite-Datenbank und die echte
// HTTP-API — nichts wird gemockt. Erledigungen werden über den regulären
// Endpunkt gemeldet; der Zeitpunkt kommt aus srv.Now.

const (
	ernaToken  = "erna-sub:Erna:"
	karlToken  = "karl:Karl:"
	bertaToken = "berta:Berta:"
)

// berlinLoc ist die Ortszeit, in der die Zeiträume abgegrenzt werden.
func berlinLoc(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

type lbBadge struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

type lbEntry struct {
	Rank           int            `json:"rank"`
	UserSub        string         `json:"userSub"`
	UserName       string         `json:"userName"`
	Completions    int            `json:"completions"`
	ByKind         map[string]int `json:"byKind"`
	Liters         float64        `json:"liters"`
	LastCompletion *time.Time     `json:"lastCompletion"`
	Badges         []lbBadge      `json:"badges"`
}

func (e lbEntry) hasBadge(key string) bool {
	for _, b := range e.Badges {
		if b.Key == key {
			return true
		}
	}
	return false
}

type lbResponse struct {
	Period  string    `json:"period"`
	From    time.Time `json:"from"`
	To      time.Time `json:"to"`
	Entries []lbEntry `json:"entries"`
	Totals  struct {
		Completions  int            `json:"completions"`
		ByKind       map[string]int `json:"byKind"`
		Liters       float64        `json:"liters"`
		Participants int            `json:"participants"`
	} `json:"totals"`
	Me *lbEntry `json:"me"`
}

// reportAt meldet eine Erledigung zum Zeitpunkt at (echte HTTP-Meldung).
func reportAt(t *testing.T, ts *httptest.Server, srv *Server, taskID int64, token string, at time.Time, liters float64) int64 {
	t.Helper()
	old := srv.Now
	srv.Now = func() time.Time { return at }
	defer func() { srv.Now = old }()
	body := map[string]any{}
	if liters > 0 {
		body["liters"] = liters
	}
	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), token, body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Erledigung melden (%s, %s): Status %d", token, at.Format(time.RFC3339), resp.StatusCode)
	}
	return int64(decode[map[string]any](t, resp)["id"].(float64))
}

// leaderboardAt fragt die Rangliste zum Stichtag now ab.
func leaderboardAt(t *testing.T, ts *httptest.Server, srv *Server, token, query string, now time.Time) lbResponse {
	t.Helper()
	old := srv.Now
	srv.Now = func() time.Time { return now }
	defer func() { srv.Now = old }()
	resp := doReq(t, "GET", ts.URL+"/api/v1/stats/leaderboard"+query, token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Rangliste%s: Status %d", query, resp.StatusCode)
	}
	return decode[lbResponse](t, resp)
}

// createPlaceAt legt Ort und Gießaufgabe zu einem bestimmten Zeitpunkt an.
func createPlaceAt(t *testing.T, ts *httptest.Server, srv *Server, at time.Time) (placeID, taskID int64) {
	t.Helper()
	old := srv.Now
	srv.Now = func() time.Time { return at }
	defer func() { srv.Now = old }()
	return createPlaceWithTask(t, ts)
}

func TestLeaderboardSortingAndTie(t *testing.T) {
	loc := berlinLoc(t)
	ts, srv := newTestServer(t)
	placeID, giessen := createPlaceWithTask(t, ts)

	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/places/%d/tasks", placeID), adminToken,
		map[string]any{"kind": "jaeten", "intervalDays": 21, "redAfterDays": 35})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Jätaufgabe: Status %d", resp.StatusCode)
	}
	jaeten := int64(decode[map[string]any](t, resp)["id"].(float64))

	day := func(d int) time.Time { return time.Date(2026, time.August, d, 12, 0, 0, 0, loc) }

	// Erna: 3 Erledigungen, davon 2× Gießen mit je 10 l.
	reportAt(t, ts, srv, giessen, ernaToken, day(3), 10)
	reportAt(t, ts, srv, giessen, ernaToken, day(4), 10)
	reportAt(t, ts, srv, jaeten, ernaToken, day(5), 0)
	// Karl: ebenfalls 3 Erledigungen, aber nur 5 l → Gleichstand, weniger Liter.
	reportAt(t, ts, srv, giessen, karlToken, day(6), 5)
	reportAt(t, ts, srv, giessen, karlToken, day(7), 0)
	reportAt(t, ts, srv, giessen, karlToken, day(8), 0)
	// Berta: eine Erledigung.
	reportAt(t, ts, srv, jaeten, bertaToken, day(9), 0)

	lb := leaderboardAt(t, ts, srv, ernaToken, "", day(12))
	if lb.Period != "saison" {
		t.Errorf("Standard-Zeitraum = %q, erwartet saison", lb.Period)
	}
	if len(lb.Entries) != 3 {
		t.Fatalf("%d Einträge, erwartet 3: %+v", len(lb.Entries), lb.Entries)
	}
	want := []struct {
		name        string
		rank        int
		completions int
		liters      float64
	}{
		{"Erna", 1, 3, 20},
		{"Karl", 2, 3, 5},
		{"Berta", 3, 1, 0},
	}
	for i, w := range want {
		e := lb.Entries[i]
		if e.UserName != w.name || e.Rank != w.rank || e.Completions != w.completions || e.Liters != w.liters {
			t.Errorf("Platz %d = %+v, erwartet %v", i+1, e, w)
		}
	}
	if got := lb.Entries[0].ByKind; got["giessen"] != 2 || got["jaeten"] != 1 || got["sonstiges"] != 0 {
		t.Errorf("Ernas byKind = %v, erwartet giessen=2, jaeten=1, sonstiges=0", got)
	}
	if lb.Entries[0].LastCompletion == nil || !lb.Entries[0].LastCompletion.Equal(day(5)) {
		t.Errorf("Ernas letzte Erledigung = %v, erwartet %v", lb.Entries[0].LastCompletion, day(5))
	}
	if lb.Totals.Completions != 7 || lb.Totals.Liters != 25 || lb.Totals.Participants != 3 {
		t.Errorf("Gesamtsummen = %+v, erwartet 7 Erledigungen, 25 l, 3 Beteiligte", lb.Totals)
	}
	if lb.Totals.ByKind["giessen"] != 5 || lb.Totals.ByKind["jaeten"] != 2 {
		t.Errorf("Gesamt byKind = %v, erwartet giessen=5, jaeten=2", lb.Totals.ByKind)
	}
	if lb.Me == nil || lb.Me.UserName != "Erna" || lb.Me.Rank != 1 {
		t.Errorf("me = %+v, erwartet Erna auf Rang 1", lb.Me)
	}
}

func TestLeaderboardPeriodBoundaries(t *testing.T) {
	loc := berlinLoc(t)
	ts, srv := newTestServer(t)
	_, taskID := createPlaceAt(t, ts, srv, time.Date(2026, time.January, 1, 12, 0, 0, 0, loc))

	at := func(m time.Month, d, h, min int) time.Time { return time.Date(2026, m, d, h, min, 0, 0, loc) }
	// Vier Meldungen rund um die Saisongrenzen.
	reportAt(t, ts, srv, taskID, ernaToken, at(time.February, 28, 12, 0), 0) // vor der Saison
	reportAt(t, ts, srv, taskID, ernaToken, at(time.March, 1, 0, 30), 0)     // Saisonbeginn
	reportAt(t, ts, srv, taskID, ernaToken, at(time.October, 31, 23, 0), 0)  // Saisonende
	reportAt(t, ts, srv, taskID, ernaToken, at(time.November, 1, 0, 30), 0)  // nach der Saison

	stand := at(time.December, 15, 12, 0)
	for query, wantCount := range map[string]int{
		"?period=saison": 2,
		"?period=jahr":   4,
		"?period=gesamt": 4,
		"?period=monat":  0, // Dezember: nichts gemeldet
	} {
		lb := leaderboardAt(t, ts, srv, ernaToken, query, stand)
		got := 0
		if len(lb.Entries) > 0 {
			got = lb.Entries[0].Completions
		}
		if got != wantCount {
			t.Errorf("%s: %d Erledigungen, erwartet %d (from=%s, to=%s)",
				query, got, wantCount, lb.From.Format(time.RFC3339), lb.To.Format(time.RFC3339))
		}
		if lb.Totals.Completions != wantCount {
			t.Errorf("%s: totals = %d, erwartet %d", query, lb.Totals.Completions, wantCount)
		}
	}

	// Monatswechsel: im März zählt nur die Meldung vom 1. März.
	if lb := leaderboardAt(t, ts, srv, ernaToken, "?period=monat", at(time.March, 15, 12, 0)); lb.Totals.Completions != 1 {
		t.Errorf("März: %d Erledigungen, erwartet 1", lb.Totals.Completions)
	}
	// Die Woche ab Montag, 2. März, enthält die Sonntagsmeldung vom 1. März nicht.
	if lb := leaderboardAt(t, ts, srv, ernaToken, "?period=woche", at(time.March, 5, 12, 0)); lb.Totals.Completions != 0 {
		t.Errorf("Woche ab 2. März: %d Erledigungen, erwartet 0", lb.Totals.Completions)
	}
	// In der Woche des 1. März (Sonntag) zählt sie.
	if lb := leaderboardAt(t, ts, srv, ernaToken, "?period=woche", at(time.February, 25, 12, 0)); lb.Totals.Completions != 2 {
		t.Errorf("Woche mit 28.2. und 1.3.: %d Erledigungen, erwartet 2", lb.Totals.Completions)
	}

	if resp := doReq(t, "GET", ts.URL+"/api/v1/stats/leaderboard?period=jahrzehnt", ernaToken, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unbekannter Zeitraum: Status %d, erwartet 400", resp.StatusCode)
	}
}

func TestLeaderboardMeOutsideTopList(t *testing.T) {
	loc := berlinLoc(t)
	ts, srv := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)

	day := func(d int) time.Time { return time.Date(2026, time.August, d, 12, 0, 0, 0, loc) }
	// Fünf Melder mit absteigender Anzahl: 5, 4, 3, 2, 1.
	tokens := []string{"a:Anna:", "b:Bernd:", "c:Cora:", "d:Dieter:", "e:Emma:"}
	for i, token := range tokens {
		for n := 0; n < len(tokens)-i; n++ {
			reportAt(t, ts, srv, taskID, token, day(1+i).Add(time.Duration(n)*time.Hour), 0)
		}
	}

	lb := leaderboardAt(t, ts, srv, "e:Emma:", "?limit=3", day(20))
	if len(lb.Entries) != 3 {
		t.Fatalf("%d Einträge, erwartet 3 (limit)", len(lb.Entries))
	}
	if lb.Entries[0].UserName != "Anna" {
		t.Errorf("Erster = %q, erwartet Anna", lb.Entries[0].UserName)
	}
	if lb.Me == nil || lb.Me.Rank != 5 || lb.Me.Completions != 1 || lb.Me.UserName != "Emma" {
		t.Fatalf("me = %+v, erwartet Emma auf Rang 5 mit 1 Erledigung", lb.Me)
	}
	if lb.Totals.Participants != 5 {
		t.Errorf("Beteiligte = %d, erwartet 5", lb.Totals.Participants)
	}

	// Wer nichts gemeldet hat, bekommt einen leeren Eintrag ohne Rang.
	lb = leaderboardAt(t, ts, srv, "z:Zacharias:", "?limit=3", day(20))
	if lb.Me == nil || lb.Me.Rank != 0 || lb.Me.Completions != 0 || lb.Me.UserName != "Zacharias" {
		t.Fatalf("me ohne Meldungen = %+v, erwartet Rang 0 / 0 Erledigungen", lb.Me)
	}
}

func TestLeaderboardBadges(t *testing.T) {
	loc := berlinLoc(t)
	ts, srv := newTestServer(t)
	// Aufgabe seit dem 1. März — die erste Erledigung im Juli rettet sie.
	_, taskID := createPlaceAt(t, ts, srv, time.Date(2026, time.March, 1, 12, 0, 0, 0, loc))

	at := func(m time.Month, d, h int) time.Time { return time.Date(2026, m, d, h, 0, 0, 0, loc) }
	// Erna gießt vier Montage in Folge um 6 Uhr morgens …
	for _, d := range []int{6, 13, 20, 27} {
		reportAt(t, ts, srv, taskID, ernaToken, at(time.July, d, 6), 10)
	}
	// … Karl einmal mittags …
	reportAt(t, ts, srv, taskID, karlToken, at(time.August, 5, 12), 5)
	// … und Erna nochmal zweimal früh im August (Gießkanne des Monats).
	reportAt(t, ts, srv, taskID, ernaToken, at(time.August, 6, 6), 10)
	reportAt(t, ts, srv, taskID, ernaToken, at(time.August, 7, 6), 10)

	lb := leaderboardAt(t, ts, srv, ernaToken, "", at(time.August, 12, 12))
	byName := map[string]lbEntry{}
	for _, e := range lb.Entries {
		byName[e.UserName] = e
	}
	erna, ok := byName["Erna"]
	if !ok {
		t.Fatalf("Erna fehlt in der Rangliste: %+v", lb.Entries)
	}
	for _, key := range []string{"giesskanne", "fruehaufsteher", "retter", "ausdauer"} {
		if !erna.hasBadge(key) {
			t.Errorf("Erna fehlt die Auszeichnung %q (hat: %+v)", key, erna.Badges)
		}
	}
	for _, b := range erna.Badges {
		if b.Label == "" || b.Description == "" {
			t.Errorf("Auszeichnung ohne Text: %+v", b)
		}
	}
	if karl := byName["Karl"]; len(karl.Badges) != 0 {
		t.Errorf("Karl sollte keine Auszeichnung haben, hat: %+v", karl.Badges)
	}
}

func TestLeaderboardEmpty(t *testing.T) {
	ts, srv := newTestServer(t)
	createPlaceWithTask(t, ts)
	lb := leaderboardAt(t, ts, srv, ernaToken, "", time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC))
	if len(lb.Entries) != 0 {
		t.Errorf("Einträge = %+v, erwartet leer", lb.Entries)
	}
	if lb.Totals.Completions != 0 || lb.Totals.Liters != 0 || lb.Totals.Participants != 0 {
		t.Errorf("Gesamtsummen = %+v, erwartet alles 0", lb.Totals)
	}
	if lb.Me == nil || lb.Me.Rank != 0 {
		t.Errorf("me = %+v, erwartet leeren Eintrag", lb.Me)
	}
}

// Eine zurückgenommene Meldung darf in der Rangliste nicht mehr zählen.
func TestLeaderboardIgnoresWithdrawnCompletion(t *testing.T) {
	loc := berlinLoc(t)
	ts, srv := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)
	day := func(d int) time.Time { return time.Date(2026, time.August, d, 12, 0, 0, 0, loc) }

	reportAt(t, ts, srv, taskID, ernaToken, day(3), 10)
	id := reportAt(t, ts, srv, taskID, ernaToken, day(4), 10)

	if lb := leaderboardAt(t, ts, srv, ernaToken, "", day(12)); lb.Me.Completions != 2 || lb.Me.Liters != 20 {
		t.Fatalf("vor der Rücknahme: %+v", lb.Me)
	}
	if resp := doReq(t, "DELETE", ts.URL+fmt.Sprintf("/api/v1/completions/%d", id), ernaToken, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Rücknahme: Status %d", resp.StatusCode)
	}
	lb := leaderboardAt(t, ts, srv, ernaToken, "", day(12))
	if lb.Me.Completions != 1 || lb.Me.Liters != 10 || lb.Totals.Completions != 1 {
		t.Fatalf("nach der Rücknahme: me=%+v, totals=%+v", lb.Me, lb.Totals)
	}
}
