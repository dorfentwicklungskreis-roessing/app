package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
)

// stubVerifier simuliert die Rössing-ID: "admin-jwt" → Admin Levin,
// "member-jwt" → Mitglied ohne Rollen, alles andere → ungültig.
// (Die echte JWT/JWKS-Prüfung ist in internal/auth getestet.)
type stubVerifier struct{}

func (stubVerifier) Verify(_ context.Context, raw string) (auth.User, error) {
	switch raw {
	case "admin-jwt":
		return auth.User{Sub: "levin", Name: "Levin", Roles: map[string]bool{"admin": true}}, nil
	case "member-jwt":
		return auth.User{Sub: "erna", Name: "Erna", Roles: map[string]bool{}}, nil
	}
	return auth.User{}, errors.New("ungültiges Token")
}

const issuer = "https://id.example"

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts, _ := newTestServerMitDB(t)
	return ts
}

// newTestServerMitDB liefert zusätzlich die Datenbank — für Tests, die eine
// Ausgangslage direkt setzen (z.B. eine bestehende Zusage).
func newTestServerMitDB(t *testing.T) (*httptest.Server, *db.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s := New(d, stubVerifier{}, issuer, "https://api.example", "client-123")
	s.Now = func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) }
	mux := http.NewServeMux()
	s.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, d
}

// rpc schickt einen JSON-RPC-Call mit Bearer-Token an den MCP-Endpoint.
func rpcRaw(t *testing.T, ts *httptest.Server, token, method string, params any) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, _ := http.NewRequest("POST", ts.URL+"/mcp", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func rpc(t *testing.T, ts *httptest.Server, token, method string, params any) map[string]any {
	t.Helper()
	resp := rpcRaw(t, ts, token, method, params)
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

// callTool ruft ein Tool als Admin auf und liefert den Text-Inhalt zurück.
func callTool(t *testing.T, ts *httptest.Server, name string, args any) (string, bool) {
	t.Helper()
	out := rpc(t, ts, "admin-jwt", "tools/call", map[string]any{"name": name, "arguments": args})
	result := out["result"].(map[string]any)
	content := result["content"].([]any)[0].(map[string]any)
	return content["text"].(string), result["isError"].(bool)
}

func TestOAuthGating(t *testing.T) {
	ts := newTestServer(t)

	// Ohne Token: 401 mit WWW-Authenticate → Client startet OAuth-Flow.
	resp := rpcRaw(t, ts, "", "tools/list", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ohne Token: HTTP %d, erwartet 401", resp.StatusCode)
	}
	if h := resp.Header.Get("WWW-Authenticate"); !strings.Contains(h, "oauth-protected-resource") {
		t.Fatalf("WWW-Authenticate fehlt/falsch: %q", h)
	}

	// Ungültiges Token: 401.
	if resp := rpcRaw(t, ts, "kaputt", "tools/list", nil); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ungültiges Token: HTTP %d, erwartet 401", resp.StatusCode)
	}

	// Gültig, aber keine admin-Rolle: 403.
	if resp := rpcRaw(t, ts, "member-jwt", "tools/list", nil); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("member: HTTP %d, erwartet 403", resp.StatusCode)
	}

	// Admin darf.
	out := rpc(t, ts, "admin-jwt", "tools/list", nil)
	if len(out["result"].(map[string]any)["tools"].([]any)) < 9 {
		t.Fatal("zu wenige Tools registriert")
	}
}

func TestProtectedResourceMetadata(t *testing.T) {
	ts := newTestServer(t)
	// Die Ressourcen-Kennung muss zu dem Dokument passen, unter dem sie
	// gefunden wurde (RFC 9728 §3.3). Ein Client, der die Adresse
	// „…/mcp" bekommen hat, holt das pfadspezifische Dokument und vergleicht
	// zeichengenau — stimmt es nicht, verwirft er es und kann den
	// Anmeldeweg nicht mehr selbst finden.
	cases := map[string]string{
		"/.well-known/oauth-protected-resource":     "https://api.example",
		"/.well-known/oauth-protected-resource/mcp": "https://api.example/mcp",
	}
	for path, wantResource := range cases {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		var meta struct {
			Resource             string   `json:"resource"`
			AuthorizationServers []string `json:"authorization_servers"`
			ScopesSupported      []string `json:"scopes_supported"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		resp.Body.Close()
		if meta.Resource != wantResource {
			t.Fatalf("%s: resource = %q, erwartet %q", path, meta.Resource, wantResource)
		}
		// authorization_servers zeigt auf UNS (wir spiegeln die AS-Metadata
		// und ergänzen den DCR-Endpoint, den Zitadel nicht hat).
		if len(meta.AuthorizationServers) != 1 || meta.AuthorizationServers[0] != "https://api.example" {
			t.Fatalf("%s: unerwartete Metadata: %+v", path, meta)
		}
		if !contains(meta.ScopesSupported, rolesScope) {
			t.Fatalf("%s: Rollen-Scope fehlt in scopes_supported: %v", path, meta.ScopesSupported)
		}
	}
}

func contains(list []string, value string) bool {
	for _, e := range list {
		if e == value {
			return true
		}
	}
	return false
}

// Der 401 muss auf das Dokument der Ressource zeigen, an der der Client
// gerade hängengeblieben ist — also auf die pfadspezifische Variante.
func TestUnauthorizedPointsAtTheResourceDocument(t *testing.T) {
	ts := newTestServer(t)
	resp := rpcRaw(t, ts, "", "tools/list", nil)
	defer resp.Body.Close()
	wa := resp.Header.Get("WWW-Authenticate")
	if !strings.Contains(wa, `resource_metadata="https://api.example/.well-known/oauth-protected-resource/mcp"`) {
		t.Fatalf("WWW-Authenticate zeigt woandershin: %q", wa)
	}
	// Ohne Freigabe darf ein Browser die Kopfzeile gar nicht erst lesen.
	if !strings.Contains(resp.Header.Get("Access-Control-Expose-Headers"), "WWW-Authenticate") {
		t.Fatalf("WWW-Authenticate ist für den Browser nicht sichtbar: %q",
			resp.Header.Get("Access-Control-Expose-Headers"))
	}
}

// Ein MCP-Client kann im Browser laufen. Ein Endpunkt ohne CORS ist von dort
// aus unbenutzbar: Die Vorabfrage scheitert, und der Client sieht nie, dass es
// eine Registrierung gibt — übrig bleibt die Frage nach einer Client-ID.
func TestOAuthEndpointsAreUsableFromABrowser(t *testing.T) {
	ts := newTestServer(t)
	paths := []string{
		"/mcp",
		"/oauth/register",
		"/.well-known/oauth-protected-resource",
		"/.well-known/oauth-protected-resource/mcp",
		"/.well-known/oauth-authorization-server",
	}
	for _, p := range paths {
		t.Run("preflight "+p, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodOptions, ts.URL+p, nil)
			req.Header.Set("Origin", "https://claude.ai")
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "authorization, content-type")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode >= 300 {
				t.Fatalf("Vorabfrage abgewiesen: HTTP %d", resp.StatusCode)
			}
			if resp.Header.Get("Access-Control-Allow-Origin") == "" {
				t.Fatal("Access-Control-Allow-Origin fehlt")
			}
			allowed := strings.ToLower(resp.Header.Get("Access-Control-Allow-Headers"))
			if !strings.Contains(allowed, "authorization") || !strings.Contains(allowed, "content-type") {
				t.Fatalf("Access-Control-Allow-Headers unbrauchbar: %q", allowed)
			}
		})
	}

	// Auch die Antwort der Registrierung selbst muss der Browser lesen dürfen.
	body := `{"redirect_uris":["https://claude.ai/api/mcp/auth_callback"],"client_name":"Claude"}`
	resp, err := http.Post(ts.URL+"/oauth/register", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") == "" {
		t.Fatal("Registrierungs-Antwort ohne Access-Control-Allow-Origin — im Browser unlesbar")
	}
}

// Die gespiegelte AS-Metadata ist das einzige, was claude.ai über den
// Anmeldeweg erfährt. Steht der Rollen-Scope nicht darin, fragt der Client
// ihn nicht an — und Zitadel legt die Projektrollen dann nicht ins
// Access-Token. Die Anmeldung klappt, jeder Aufruf endet trotzdem in 403.
func TestASMetadataNamesRolesScopeAndRegistration(t *testing.T) {
	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"authorization_endpoint":"https://id.example/oauth/v2/authorize",
			"token_endpoint":"https://id.example/oauth/v2/token",
			"jwks_uri":"https://id.example/oauth/v2/keys"}`))
	}))
	defer idp.Close()

	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	mux := http.NewServeMux()
	New(d, stubVerifier{}, idp.URL, "https://api.example", "client-123").Register(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/.well-known/oauth-authorization-server")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var meta struct {
		AuthorizationEndpoint string   `json:"authorization_endpoint"`
		TokenEndpoint         string   `json:"token_endpoint"`
		RegistrationEndpoint  string   `json:"registration_endpoint"`
		ScopesSupported       []string `json:"scopes_supported"`
		CodeChallengeMethods  []string `json:"code_challenge_methods_supported"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		t.Fatal(err)
	}
	if meta.AuthorizationEndpoint == "" || meta.TokenEndpoint == "" {
		t.Fatalf("Endpunkte der Rössing-ID fehlen: %+v", meta)
	}
	if meta.RegistrationEndpoint != "https://api.example/oauth/register" {
		t.Fatalf("registration_endpoint = %q", meta.RegistrationEndpoint)
	}
	if !contains(meta.ScopesSupported, rolesScope) {
		t.Fatalf("Rollen-Scope fehlt: %v", meta.ScopesSupported)
	}
	if !contains(meta.CodeChallengeMethods, "S256") {
		t.Fatalf("PKCE-Verfahren fehlt: %v", meta.CodeChallengeMethods)
	}
}

// Eine öffentliche Adresse mit Schrägstrich am Ende darf sich nicht in die
// Metadata fortpflanzen: Ressourcen-Kennungen werden zeichengenau verglichen.
func TestPublicAddressKeepsNoTrailingSlash(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	s := New(d, stubVerifier{}, issuer, "https://api.example/", "client-123")
	if s.Resource != "https://api.example" {
		t.Fatalf("Resource = %q", s.Resource)
	}
}

func TestDynamicClientRegistration(t *testing.T) {
	ts := newTestServer(t)

	register := func(uris []string) (*http.Response, map[string]any) {
		body, _ := json.Marshal(map[string]any{"redirect_uris": uris, "client_name": "Claude"})
		resp, err := http.Post(ts.URL+"/oauth/register", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return resp, out
	}

	// claude.ai-Callback → feste Client-ID zurück.
	resp, out := register([]string{"https://claude.ai/api/mcp/auth_callback"})
	if resp.StatusCode != http.StatusCreated || out["client_id"] != "client-123" {
		t.Fatalf("DCR fehlgeschlagen: HTTP %d, %v", resp.StatusCode, out)
	}
	if out["token_endpoint_auth_method"] != "none" {
		t.Fatalf("erwartet public client: %v", out)
	}

	// Fremde Redirect-URI → abgelehnt.
	resp, out = register([]string{"https://boese-seite.example/callback"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("fremde Redirect-URI nicht abgelehnt: HTTP %d, %v", resp.StatusCode, out)
	}

	// Ohne Redirect-URIs → abgelehnt.
	resp, _ = register(nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("leere Registrierung nicht abgelehnt: HTTP %d", resp.StatusCode)
	}
}

func TestInitialize(t *testing.T) {
	ts := newTestServer(t)
	out := rpc(t, ts, "admin-jwt", "initialize", map[string]any{"protocolVersion": "2025-03-26"})
	result := out["result"].(map[string]any)
	if result["serverInfo"].(map[string]any)["name"] != "dorf-app" {
		t.Fatalf("unerwartete serverInfo: %v", result)
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

	// Gießplan: 5 l pro Woche
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

	// Liste enthält Ort mit 2 Aufgaben
	text, _ = callTool(t, ts, "orte_liste", map[string]any{})
	if !strings.Contains(text, "Unter den Eichen") || !strings.Contains(text, "jaeten") {
		t.Fatalf("orte_liste unvollständig: %s", text)
	}

	// Erledigung ohne Namen → Name des eingeloggten Admins (Levin)
	text, isErr = callTool(t, ts, "erledigung_melden", map[string]any{"taskId": task.ID, "liters": 5})
	if isErr || !strings.Contains(text, "Levin") {
		t.Fatalf("erledigung_melden: isErr=%v, text=%s", isErr, text)
	}

	// Hitzefaktor + Aufgabe ändern + Ort löschen
	if text, isErr = callTool(t, ts, "hitzefaktor_setzen", map[string]any{"factor": 0.5}); isErr {
		t.Fatalf("hitzefaktor_setzen: %s", text)
	}
	if text, isErr = callTool(t, ts, "aufgabe_aendern", map[string]any{"id": task.ID, "liters": 10}); isErr || !strings.Contains(text, "10") {
		t.Fatalf("aufgabe_aendern: %s", text)
	}
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
}

// Rangliste und Rücknahme einer Erledigung — die beiden neuen Tools.
func TestLeaderboardAndWithdrawalTools(t *testing.T) {
	ts := newTestServer(t)

	text, isErr := callTool(t, ts, "ort_anlegen", map[string]any{
		"name": "Rangliste-Kasten", "lat": 52.2110, "lon": 9.8697,
	})
	if isErr {
		t.Fatalf("ort_anlegen: %s", text)
	}
	var place struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(text), &place)

	text, isErr = callTool(t, ts, "aufgabe_anlegen", map[string]any{
		"placeId": place.ID, "kind": "giessen", "liters": 10, "intervalDays": 7, "redAfterDays": 14,
	})
	if isErr {
		t.Fatalf("aufgabe_anlegen: %s", text)
	}
	var task struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(text), &task)

	// Zwei Meldungen: eine für Erna, eine irrtümliche für Kuno.
	if text, isErr = callTool(t, ts, "erledigung_melden",
		map[string]any{"taskId": task.ID, "name": "Erna", "liters": 10}); isErr {
		t.Fatalf("erledigung_melden: %s", text)
	}
	// Zweite Meldung an derselben Aufgabe: der Spielschutz sperrt sie …
	if text, isErr = callTool(t, ts, "erledigung_melden",
		map[string]any{"taskId": task.ID, "name": "Kuno"}); !isErr {
		t.Fatalf("Spielschutz greift nicht: %s", text)
	}
	// … Admins dürfen sie für telefonische Nachträge trotzdem eintragen.
	text, isErr = callTool(t, ts, "erledigung_melden",
		map[string]any{"taskId": task.ID, "name": "Kuno", "force": true})
	if isErr {
		t.Fatalf("erledigung_melden: %s", text)
	}
	var kuno struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(text), &kuno)

	// Rangliste: beide tauchen auf (MCP-Tools laufen als Admin).
	text, isErr = callTool(t, ts, "rangliste", map[string]any{"period": "saison"})
	if isErr {
		t.Fatalf("rangliste: %s", text)
	}
	if !strings.Contains(text, "Erna") || !strings.Contains(text, "Kuno") {
		t.Fatalf("rangliste unvollständig: %s", text)
	}

	// Unbekannter Zeitraum → Fehler.
	if text, isErr = callTool(t, ts, "rangliste", map[string]any{"period": "jahrzehnt"}); !isErr {
		t.Fatalf("unbekannter Zeitraum ohne Fehler: %s", text)
	}

	// Rücknahme der irrtümlichen Meldung.
	if text, isErr = callTool(t, ts, "erledigung_zuruecknehmen", map[string]any{"id": kuno.ID}); isErr {
		t.Fatalf("erledigung_zuruecknehmen: %s", text)
	}
	text, isErr = callTool(t, ts, "rangliste", map[string]any{"period": "saison"})
	if isErr {
		t.Fatalf("rangliste: %s", text)
	}
	if strings.Contains(text, "Kuno") {
		t.Fatalf("Kuno steht nach der Rücknahme noch in der Rangliste: %s", text)
	}

	// Unbekannte ID → Fehler.
	if text, isErr = callTool(t, ts, "erledigung_zuruecknehmen", map[string]any{"id": 4711}); !isErr {
		t.Fatalf("Rücknahme einer unbekannten Meldung ohne Fehler: %s", text)
	}
}
