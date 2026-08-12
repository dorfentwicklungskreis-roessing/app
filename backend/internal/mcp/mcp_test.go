package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s := New(d, "geheim")
	s.Now = func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) }
	mux := http.NewServeMux()
	s.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// rpc schickt einen JSON-RPC-Call an den MCP-Endpoint.
func rpc(t *testing.T, ts *httptest.Server, path, token, method string, params any) map[string]any {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, _ := http.NewRequest("POST", ts.URL+path, bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: HTTP %d", method, resp.StatusCode)
	}
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

// callTool ruft ein Tool auf und liefert den Text-Inhalt zurück.
func callTool(t *testing.T, ts *httptest.Server, name string, args any) (string, bool) {
	t.Helper()
	out := rpc(t, ts, "/mcp", "geheim", "tools/call", map[string]any{"name": name, "arguments": args})
	result := out["result"].(map[string]any)
	content := result["content"].([]any)[0].(map[string]any)
	return content["text"].(string), result["isError"].(bool)
}

func TestAuthRejected(t *testing.T) {
	ts := newTestServer(t)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	// Ohne Token
	resp, _ := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(body))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ohne Token: HTTP %d, erwartet 401", resp.StatusCode)
	}
	// Falsches Token im Pfad
	resp, _ = http.Post(ts.URL+"/mcp/falsch", "application/json", strings.NewReader(body))
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("falsches Token: HTTP %d, erwartet 401", resp.StatusCode)
	}
	// Richtiges Token im Pfad funktioniert
	resp, _ = http.Post(ts.URL+"/mcp/geheim", "application/json", strings.NewReader(body))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Token im Pfad: HTTP %d, erwartet 200", resp.StatusCode)
	}
}

func TestInitializeAndToolsList(t *testing.T) {
	ts := newTestServer(t)
	out := rpc(t, ts, "/mcp", "geheim", "initialize", map[string]any{"protocolVersion": "2025-03-26"})
	result := out["result"].(map[string]any)
	if result["serverInfo"].(map[string]any)["name"] != "dorf-app" {
		t.Fatalf("unerwartete serverInfo: %v", result)
	}
	out = rpc(t, ts, "/mcp", "geheim", "tools/list", nil)
	tools := out["result"].(map[string]any)["tools"].([]any)
	if len(tools) < 9 {
		t.Fatalf("nur %d Tools registriert", len(tools))
	}
}

func TestFullAdminFlow(t *testing.T) {
	ts := newTestServer(t)

	// Ort anlegen
	text, isErr := callTool(t, ts, "ort_anlegen", map[string]any{
		"name": "Unter den Eichen — Kasten 1", "lat": 52.2110, "lon": 9.8697,
	})
	if isErr {
		t.Fatalf("ort_anlegen: %s", text)
	}
	var place struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(text), &place)

	// Gießplan anlegen: 5 l pro Woche
	text, isErr = callTool(t, ts, "aufgabe_anlegen", map[string]any{
		"placeId": place.ID, "kind": "giessen", "liters": 5, "intervalDays": 7, "redAfterDays": 14,
	})
	if isErr {
		t.Fatalf("aufgabe_anlegen: %s", text)
	}
	var task struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(text), &task)

	// Jäten dazu
	if text, isErr = callTool(t, ts, "aufgabe_anlegen", map[string]any{
		"placeId": place.ID, "kind": "jaeten", "intervalDays": 21, "redAfterDays": 35,
	}); isErr {
		t.Fatalf("jäten anlegen: %s", text)
	}

	// Liste enthält Ort mit 2 Aufgaben und Status
	text, _ = callTool(t, ts, "orte_liste", map[string]any{})
	if !strings.Contains(text, "Unter den Eichen") || !strings.Contains(text, "jaeten") {
		t.Fatalf("orte_liste unvollständig: %s", text)
	}

	// Erledigung melden
	if text, isErr = callTool(t, ts, "erledigung_melden", map[string]any{
		"taskId": task.ID, "name": "Levin", "liters": 5,
	}); isErr {
		t.Fatalf("erledigung_melden: %s", text)
	}

	// Hitzefaktor setzen
	if text, isErr = callTool(t, ts, "hitzefaktor_setzen", map[string]any{"factor": 0.5}); isErr {
		t.Fatalf("hitzefaktor_setzen: %s", text)
	}

	// Aufgabe ändern (Gießmenge auf 10 l)
	if text, isErr = callTool(t, ts, "aufgabe_aendern", map[string]any{
		"id": task.ID, "liters": 10,
	}); isErr {
		t.Fatalf("aufgabe_aendern: %s", text)
	}
	if !strings.Contains(text, "10") {
		t.Fatalf("Liter nicht geändert: %s", text)
	}

	// Ort löschen
	if text, isErr = callTool(t, ts, "ort_loeschen", map[string]any{"id": place.ID}); isErr {
		t.Fatalf("ort_loeschen: %s", text)
	}
}

func TestUnknownToolAndValidation(t *testing.T) {
	ts := newTestServer(t)
	if text, isErr := callTool(t, ts, "gibts_nicht", nil); !isErr {
		t.Fatalf("unbekanntes Tool ohne Fehler: %s", text)
	}
	if text, isErr := callTool(t, ts, "ort_anlegen", map[string]any{"name": "x", "lat": 999, "lon": 0}); !isErr {
		t.Fatalf("ungültige Koordinaten ohne Fehler: %s", text)
	}
	if text, isErr := callTool(t, ts, "hitzefaktor_setzen", map[string]any{"factor": 99}); !isErr {
		t.Fatalf("Faktor 99 ohne Fehler: %s", text)
	}
	if text, isErr := callTool(t, ts, "erledigung_melden", map[string]any{"taskId": 4711}); !isErr {
		t.Fatalf("Erledigung auf unbekannte Aufgabe ohne Fehler: %s", text)
	}
	_ = fmt.Sprint() // fmt bleibt genutzt
}
