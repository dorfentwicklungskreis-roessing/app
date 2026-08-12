package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
)

// newTestServer baut einen Server mit frischer SQLite-DB und Dev-Auth.
// Tokens haben das Format "sub:name:rolle1,rolle2" (InsecureDevVerifier).
func newTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	srv := &Server{DB: d, Now: func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) }}
	ts := httptest.NewServer(srv.Handler(auth.Middleware(auth.InsecureDevVerifier{}), nil))
	t.Cleanup(ts.Close)
	return ts, srv
}

func doReq(t *testing.T, method, url, token string, body any) *http.Response {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func decode[T any](t *testing.T, resp *http.Response) T {
	t.Helper()
	defer resp.Body.Close()
	var v T
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

const (
	adminToken  = "admin-sub:Levin:admin"
	memberToken = "member-sub:Erna:"
)

type placesResponse struct {
	Places []struct {
		ID     int64  `json:"id"`
		Name   string `json:"name"`
		Status string `json:"status"`
		Tasks  []struct {
			ID     int64  `json:"id"`
			Kind   string `json:"kind"`
			Status string `json:"status"`
		} `json:"tasks"`
	} `json:"places"`
	WateringFactor float64 `json:"wateringFactor"`
}

// createPlaceWithTask legt „Unter den Eichen" mit Gießplan (10 l, 7/14 Tage) an.
func createPlaceWithTask(t *testing.T, ts *httptest.Server) (placeID, taskID int64) {
	t.Helper()
	resp := doReq(t, "POST", ts.URL+"/api/v1/places", adminToken, map[string]any{
		"name": "Unter den Eichen — Kasten 1", "kind": "blumenkasten", "lat": 52.2110, "lon": 9.8697,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Ort anlegen: Status %d", resp.StatusCode)
	}
	p := decode[map[string]any](t, resp)
	placeID = int64(p["id"].(float64))

	resp = doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/places/%d/tasks", placeID), adminToken, map[string]any{
		"kind": "giessen", "liters": 10, "intervalDays": 7, "redAfterDays": 14,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Aufgabe anlegen: Status %d", resp.StatusCode)
	}
	task := decode[map[string]any](t, resp)
	return placeID, int64(task["id"].(float64))
}

func getPlaces(t *testing.T, ts *httptest.Server) placesResponse {
	t.Helper()
	return decode[placesResponse](t, doReq(t, "GET", ts.URL+"/api/v1/places", memberToken, nil))
}

func TestAuthRequired(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := doReq(t, "GET", ts.URL+"/api/v1/places", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ohne Token: Status %d, erwartet 401", resp.StatusCode)
	}
}

func TestMemberCannotAdministrate(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID, taskID := createPlaceWithTask(t, ts)
	for name, req := range map[string]struct {
		method, path string
		body         any
	}{
		"Ort anlegen":        {"POST", "/api/v1/places", map[string]any{"name": "x", "lat": 1, "lon": 1}},
		"Ort ändern":         {"PUT", fmt.Sprintf("/api/v1/places/%d", placeID), map[string]any{"name": "x", "lat": 1, "lon": 1}},
		"Ort löschen":        {"DELETE", fmt.Sprintf("/api/v1/places/%d", placeID), nil},
		"Aufgabe anlegen":    {"POST", fmt.Sprintf("/api/v1/places/%d/tasks", placeID), map[string]any{"kind": "giessen", "intervalDays": 7, "redAfterDays": 14}},
		"Aufgabe löschen":    {"DELETE", fmt.Sprintf("/api/v1/tasks/%d", taskID), nil},
		"Settings schreiben": {"PUT", "/api/v1/settings", map[string]any{"wateringFactor": 0.5}},
	} {
		resp := doReq(t, req.method, ts.URL+req.path, memberToken, req.body)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s als member: Status %d, erwartet 403", name, resp.StatusCode)
		}
	}
}

func TestValidation(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID, _ := createPlaceWithTask(t, ts)
	for name, req := range map[string]struct {
		path string
		body map[string]any
	}{
		"Ort ohne Name":      {"/api/v1/places", map[string]any{"lat": 1, "lon": 1}},
		"Ort kaputte Lat":    {"/api/v1/places", map[string]any{"name": "x", "lat": 999, "lon": 1}},
		"Ort falsche Art":    {"/api/v1/places", map[string]any{"name": "x", "kind": "raumschiff", "lat": 1, "lon": 1}},
		"Aufgabe rot<gelb":   {fmt.Sprintf("/api/v1/places/%d/tasks", placeID), map[string]any{"kind": "giessen", "intervalDays": 7, "redAfterDays": 3}},
		"Aufgabe ohne Art":   {fmt.Sprintf("/api/v1/places/%d/tasks", placeID), map[string]any{"intervalDays": 7, "redAfterDays": 14}},
		"Aufgabe Liter <= 0": {fmt.Sprintf("/api/v1/places/%d/tasks", placeID), map[string]any{"kind": "giessen", "liters": 0, "intervalDays": 7, "redAfterDays": 14}},
	} {
		resp := doReq(t, "POST", ts.URL+req.path, adminToken, req.body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: Status %d, erwartet 400", name, resp.StatusCode)
		}
	}
}

func TestCompletionFlowAndStatus(t *testing.T) {
	ts, srv := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)

	// Frisch angelegt → grün.
	if got := getPlaces(t, ts).Places[0].Status; got != "green" {
		t.Fatalf("Status frisch angelegt = %v, erwartet green", got)
	}

	// 8 Tage später → gelb.
	srv.Now = func() time.Time { return time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC) }
	if got := getPlaces(t, ts).Places[0].Status; got != "yellow" {
		t.Fatalf("Status nach 8 Tagen = %v, erwartet yellow", got)
	}

	// 15 Tage → rot.
	srv.Now = func() time.Time { return time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC) }
	if got := getPlaces(t, ts).Places[0].Status; got != "red" {
		t.Fatalf("Status nach 15 Tagen = %v, erwartet red", got)
	}

	// Mitglied gießt → wieder grün, Meldung trägt Namen.
	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), memberToken,
		map[string]any{"liters": 10})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Erledigung melden: Status %d", resp.StatusCode)
	}
	c := decode[map[string]any](t, resp)
	if c["userName"] != "Erna" {
		t.Errorf("userName = %v, erwartet Erna", c["userName"])
	}
	if got := getPlaces(t, ts).Places[0].Status; got != "green" {
		t.Fatalf("Status nach Gießen = %v, erwartet green", got)
	}

	// Historie enthält die Meldung.
	hist := decode[struct {
		Completions []map[string]any `json:"completions"`
	}](t, doReq(t, "GET", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), memberToken, nil))
	if len(hist.Completions) != 1 {
		t.Fatalf("Historie: %d Einträge, erwartet 1", len(hist.Completions))
	}
}

func TestPlaceStatusIsWorstTask(t *testing.T) {
	ts, srv := newTestServer(t)
	placeID, taskID := createPlaceWithTask(t, ts)

	// Zweite Aufgabe: Jäten alle 3 Tage, rot nach 5.
	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/places/%d/tasks", placeID), adminToken,
		map[string]any{"kind": "jaeten", "intervalDays": 3, "redAfterDays": 5})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Jäten anlegen: Status %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Gießen erledigen, dann 4 Tage warten: Gießen grün, Jäten gelb → Ort gelb.
	doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), memberToken, nil).Body.Close()
	srv.Now = func() time.Time { return time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC) }
	p := getPlaces(t, ts).Places[0]
	if p.Status != "yellow" {
		t.Fatalf("Ort-Status = %v, erwartet yellow (Jäten fällig)", p.Status)
	}
}

func TestHeatFactorOnlyAffectsWatering(t *testing.T) {
	ts, srv := newTestServer(t)
	placeID, _ := createPlaceWithTask(t, ts)
	// Jäten mit gleichem Intervall wie Gießen.
	doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/places/%d/tasks", placeID), adminToken,
		map[string]any{"kind": "jaeten", "intervalDays": 7, "redAfterDays": 14}).Body.Close()

	// Hitzewelle: Faktor 0.5.
	if resp := doReq(t, "PUT", ts.URL+"/api/v1/settings", adminToken, map[string]any{"wateringFactor": 0.5}); resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /settings: Status %d", resp.StatusCode)
	}

	// Nach 4 Tagen: Gießen gelb (halbiertes Intervall), Jäten noch grün.
	srv.Now = func() time.Time { return time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC) }
	p := getPlaces(t, ts).Places[0]
	statusByKind := map[string]string{}
	for _, task := range p.Tasks {
		statusByKind[task.Kind] = task.Status
	}
	if statusByKind["giessen"] != "yellow" {
		t.Errorf("Gießen bei Hitze nach 4 Tagen = %v, erwartet yellow", statusByKind["giessen"])
	}
	if statusByKind["jaeten"] != "green" {
		t.Errorf("Jäten bei Hitze nach 4 Tagen = %v, erwartet green (Faktor gilt nicht)", statusByKind["jaeten"])
	}
}

func TestUpdateAndDelete(t *testing.T) {
	ts, _ := newTestServer(t)
	placeID, taskID := createPlaceWithTask(t, ts)

	resp := doReq(t, "PUT", ts.URL+fmt.Sprintf("/api/v1/places/%d", placeID), adminToken, map[string]any{
		"name": "Neuer Name", "kind": "beet", "lat": 52.2, "lon": 9.87,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Ort-Update: Status %d", resp.StatusCode)
	}
	p := decode[map[string]any](t, resp)
	if p["name"] != "Neuer Name" || p["kind"] != "beet" {
		t.Fatalf("Ort-Update nicht übernommen: %v", p)
	}

	resp = doReq(t, "PUT", ts.URL+fmt.Sprintf("/api/v1/tasks/%d", taskID), adminToken, map[string]any{
		"kind": "giessen", "liters": 5, "intervalDays": 3, "redAfterDays": 6,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Aufgaben-Update: Status %d", resp.StatusCode)
	}
	task := decode[map[string]any](t, resp)
	if task["liters"].(float64) != 5 {
		t.Fatalf("Aufgaben-Update nicht übernommen: %v", task)
	}

	if resp := doReq(t, "DELETE", ts.URL+fmt.Sprintf("/api/v1/places/%d", placeID), adminToken, nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("Ort löschen: Status %d, erwartet 204", resp.StatusCode)
	}
	if resp := doReq(t, "DELETE", ts.URL+fmt.Sprintf("/api/v1/places/%d", placeID), adminToken, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Ort nochmal löschen: Status %d, erwartet 404", resp.StatusCode)
	}
	// Aufgabe hing am Ort → kaskadiert gelöscht.
	if resp := doReq(t, "DELETE", ts.URL+fmt.Sprintf("/api/v1/tasks/%d", taskID), adminToken, nil); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Aufgabe nach Kaskade: Status %d, erwartet 404", resp.StatusCode)
	}
}
