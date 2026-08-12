//go:build e2e

// Kompletter End-to-End-Test gegen ein ECHTES Zitadel (docker compose)
// und das echte, kompilierte Backend-Binary — keine Mocks.
//
// Ablauf:
//  1. Zitadel-Admin-Token via JWT-Bearer (Machine-Key aus dem Compose-Init)
//  2. Bootstrap über die echte Zitadel-Management-API: Projekt, Rollen,
//     zwei Machine-User (Admin mit Rolle, Mitglied ohne)
//  3. Backend-Binary bauen und mit AUTH_ISSUER=http://localhost:8123 starten
//  4. Echte Tokens holen und REST-API + MCP durchtesten (Auth-Gating,
//     Rollen, Gieß-Flow, OAuth-Handshake)
//
// Voraussetzung: docker compose -f e2e/docker-compose.yml up -d --wait
package e2e

import (
	"bytes"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

const (
	issuer      = "http://localhost:8123"
	backendAddr = "http://localhost:8124"
)

// --- Zitadel-Hilfen ---------------------------------------------------------

type machineKey struct {
	KeyID  string `json:"keyId"`
	Key    string `json:"key"`
	UserID string `json:"userId"`
}

// signAssertion baut die JWT-Bearer-Assertion für einen Machine-Key.
func signAssertion(t *testing.T, k machineKey) string {
	t.Helper()
	pemBlock, _ := pem.Decode([]byte(k.Key))
	if pemBlock == nil {
		t.Fatal("Key ist kein PEM")
	}
	// Zitadel liefert je nach Version PKCS#1 oder PKCS#8.
	var signKey any
	if k1, err := x509.ParsePKCS1PrivateKey(pemBlock.Bytes); err == nil {
		signKey = k1
	} else if k8, err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes); err == nil {
		signKey = k8
	} else {
		t.Fatalf("Key parsen fehlgeschlagen (weder PKCS#1 noch PKCS#8)")
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: signKey},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", k.KeyID))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	raw, err := jwt.Signed(signer).Claims(map[string]any{
		"iss": k.UserID, "sub": k.UserID, "aud": issuer,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// fetchToken tauscht eine Assertion gegen ein Access-Token.
func fetchToken(t *testing.T, k machineKey, scope string) string {
	t.Helper()
	resp, err := http.PostForm(issuer+"/oauth/v2/token", map[string][]string{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {signAssertion(t, k)},
		"scope":      {scope},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error_description"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.AccessToken == "" {
		t.Fatalf("kein Token (HTTP %d): %s", resp.StatusCode, out.Error)
	}
	return out.AccessToken
}

// zapi ruft die Zitadel-Management-API auf.
func zapi(t *testing.T, token, method, path string, body any) map[string]any {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, issuer+path, &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if resp.StatusCode >= 300 {
		t.Fatalf("%s %s: HTTP %d: %v", method, path, resp.StatusCode, out)
	}
	return out
}

// --- Backend-Hilfen ---------------------------------------------------------

func request(t *testing.T, method, path, token string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, backendAddr+path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func mcpCall(t *testing.T, token, method string, params any) (*http.Response, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	req, _ := http.NewRequest("POST", backendAddr+"/mcp", bytes.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func mcpToolText(t *testing.T, token, tool string, args any) (string, bool) {
	t.Helper()
	resp, out := mcpCall(t, token, "tools/call", map[string]any{"name": tool, "arguments": args})
	if resp.StatusCode != 200 {
		t.Fatalf("tools/call %s: HTTP %d", tool, resp.StatusCode)
	}
	result := out["result"].(map[string]any)
	text := result["content"].([]any)[0].(map[string]any)["text"].(string)
	return text, result["isError"].(bool)
}

// --- Der eigentliche E2E-Test ----------------------------------------------

func TestEndToEnd(t *testing.T) {
	// 0. Admin-Machine-Key aus dem Compose-Volume lesen.
	keyRaw, err := os.ReadFile("machinekey/zitadel-admin-sa.json")
	if err != nil {
		t.Skipf("Zitadel-Machine-Key fehlt (compose nicht gestartet?): %v", err)
	}
	var adminSA machineKey
	if err := json.Unmarshal(keyRaw, &adminSA); err != nil {
		t.Fatal(err)
	}

	// 1. Admin-Token für die Zitadel-API.
	iamToken := fetchToken(t, adminSA, "openid urn:zitadel:iam:org:project:id:zitadel:aud")

	// 2. Bootstrap: Projekt, Rollen, zwei Machine-User mit Keys.
	project := zapi(t, iamToken, "POST", "/management/v1/projects",
		map[string]any{"name": fmt.Sprintf("dorf-app-e2e-%d", time.Now().UnixNano()), "projectRoleAssertion": true})
	projectID := project["id"].(string)
	for _, role := range []map[string]any{
		{"roleKey": "admin", "displayName": "Admin"},
		{"roleKey": "member", "displayName": "Mitglied"},
	} {
		zapi(t, iamToken, "POST", "/management/v1/projects/"+projectID+"/roles", role)
	}

	newMachine := func(name string, roleKeys []string) machineKey {
		u := zapi(t, iamToken, "POST", "/management/v1/users/machine", map[string]any{
			"userName": fmt.Sprintf("%s-%d", name, time.Now().UnixNano()), "name": name,
			"accessTokenType": "ACCESS_TOKEN_TYPE_JWT",
		})
		userID := u["userId"].(string)
		if len(roleKeys) > 0 {
			zapi(t, iamToken, "POST", "/management/v1/users/"+userID+"/grants",
				map[string]any{"projectId": projectID, "roleKeys": roleKeys})
		}
		k := zapi(t, iamToken, "POST", "/management/v1/users/"+userID+"/keys",
			map[string]any{"type": "KEY_TYPE_JSON"})
		decoded, err := base64Decode(k["keyDetails"].(string))
		if err != nil {
			t.Fatal(err)
		}
		var mk machineKey
		if err := json.Unmarshal(decoded, &mk); err != nil {
			t.Fatal(err)
		}
		return mk
	}
	adminUser := newMachine("Dorf-Admin", []string{"admin"})
	memberUser := newMachine("Dorf-Mitglied", nil)

	// 3. Backend bauen und starten (echtes Binary, echter OIDC-Verifier).
	bin := filepath.Join(t.TempDir(), "server")
	build := exec.Command("go", "build", "-o", bin, "../cmd/server")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		t.Fatalf("Backend-Build: %v", err)
	}
	srv := exec.Command(bin)
	srv.Env = append(os.Environ(),
		"LISTEN_ADDR=:8124",
		"DB_PATH="+filepath.Join(t.TempDir(), "e2e.sqlite"),
		"AUTH_ISSUER="+issuer,
		"PUBLIC_URL="+backendAddr,
	)
	srv.Stdout, srv.Stderr = os.Stderr, os.Stderr
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Process.Kill(); _, _ = srv.Process.Wait() })
	waitFor(t, backendAddr+"/healthz")

	// 4. Echte Tokens mit Projekt-Audience + Rollen-Assertion.
	scope := "openid urn:zitadel:iam:org:project:id:" + projectID + ":aud urn:zitadel:iam:org:projects:roles"
	adminToken := fetchToken(t, adminUser, scope)
	memberToken := fetchToken(t, memberUser, scope)

	// --- REST-API ---
	t.Run("API ohne Token → 401", func(t *testing.T) {
		resp, _ := request(t, "GET", "/api/v1/places", "", nil)
		if resp.StatusCode != 401 {
			t.Fatalf("HTTP %d", resp.StatusCode)
		}
	})
	t.Run("Rollen kommen im echten Token an", func(t *testing.T) {
		_, me := request(t, "GET", "/api/v1/me", adminToken, nil)
		if me["isAdmin"] != true {
			t.Fatalf("Admin nicht erkannt: %v", me)
		}
		_, me = request(t, "GET", "/api/v1/me", memberToken, nil)
		if me["isAdmin"] != false {
			t.Fatalf("Mitglied fälschlich Admin: %v", me)
		}
	})
	t.Run("Mitglied darf nicht verwalten", func(t *testing.T) {
		resp, _ := request(t, "POST", "/api/v1/places", memberToken,
			map[string]any{"name": "x", "lat": 52.0, "lon": 9.8})
		if resp.StatusCode != 403 {
			t.Fatalf("HTTP %d, erwartet 403", resp.StatusCode)
		}
	})

	var placeID, taskID float64
	t.Run("Admin legt Ort + Gießplan an", func(t *testing.T) {
		resp, place := request(t, "POST", "/api/v1/places", adminToken,
			map[string]any{"name": "E2E-Kasten", "kind": "blumenkasten", "lat": 52.211, "lon": 9.87})
		if resp.StatusCode != 201 {
			t.Fatalf("Ort: HTTP %d: %v", resp.StatusCode, place)
		}
		placeID = place["id"].(float64)
		resp, task := request(t, "POST", fmt.Sprintf("/api/v1/places/%.0f/tasks", placeID), adminToken,
			map[string]any{"kind": "giessen", "liters": 10, "intervalDays": 7, "redAfterDays": 14})
		if resp.StatusCode != 201 {
			t.Fatalf("Aufgabe: HTTP %d: %v", resp.StatusCode, task)
		}
		taskID = task["id"].(float64)
	})
	t.Run("Mitglied gießt, Status wird grün", func(t *testing.T) {
		resp, c := request(t, "POST", fmt.Sprintf("/api/v1/tasks/%.0f/completions", taskID), memberToken,
			map[string]any{"liters": 10})
		if resp.StatusCode != 201 {
			t.Fatalf("Erledigung: HTTP %d: %v", resp.StatusCode, c)
		}
		_, list := request(t, "GET", "/api/v1/places", memberToken, nil)
		places := list["places"].([]any)
		var found map[string]any
		for _, p := range places {
			if p.(map[string]any)["id"].(float64) == placeID {
				found = p.(map[string]any)
			}
		}
		if found == nil || found["status"] != "green" {
			t.Fatalf("Ort nicht grün: %v", found)
		}
	})

	// --- MCP ---
	t.Run("MCP ohne Token → 401 + OAuth-Metadata", func(t *testing.T) {
		resp, _ := mcpCall(t, "", "tools/list", nil)
		if resp.StatusCode != 401 {
			t.Fatalf("HTTP %d", resp.StatusCode)
		}
		if h := resp.Header.Get("WWW-Authenticate"); !strings.Contains(h, "oauth-protected-resource") {
			t.Fatalf("WWW-Authenticate: %q", h)
		}
		meta, err := http.Get(backendAddr + "/.well-known/oauth-protected-resource")
		if err != nil || meta.StatusCode != 200 {
			t.Fatalf("Metadata nicht erreichbar: %v", err)
		}
		meta.Body.Close()
	})
	t.Run("MCP: Mitglied → 403, Admin → Tools", func(t *testing.T) {
		resp, _ := mcpCall(t, memberToken, "tools/list", nil)
		if resp.StatusCode != 403 {
			t.Fatalf("Mitglied: HTTP %d, erwartet 403", resp.StatusCode)
		}
		resp, out := mcpCall(t, adminToken, "tools/list", nil)
		if resp.StatusCode != 200 {
			t.Fatalf("Admin: HTTP %d", resp.StatusCode)
		}
		if len(out["result"].(map[string]any)["tools"].([]any)) < 9 {
			t.Fatal("zu wenige MCP-Tools")
		}
	})
	t.Run("MCP-Admin-Flow: Beet + Jätplan + Erledigung", func(t *testing.T) {
		text, isErr := mcpToolText(t, adminToken, "ort_anlegen",
			map[string]any{"name": "E2E-Beet", "kind": "beet", "lat": 52.2105, "lon": 9.871})
		if isErr {
			t.Fatalf("ort_anlegen: %s", text)
		}
		var beet struct {
			ID float64 `json:"id"`
		}
		_ = json.Unmarshal([]byte(text), &beet)
		text, isErr = mcpToolText(t, adminToken, "aufgabe_anlegen",
			map[string]any{"placeId": beet.ID, "kind": "jaeten", "intervalDays": 21, "redAfterDays": 35})
		if isErr {
			t.Fatalf("aufgabe_anlegen: %s", text)
		}
		var jaet struct {
			ID float64 `json:"id"`
		}
		_ = json.Unmarshal([]byte(text), &jaet)
		text, isErr = mcpToolText(t, adminToken, "erledigung_melden", map[string]any{"taskId": jaet.ID})
		if isErr {
			t.Fatalf("erledigung_melden: %s", text)
		}
		// Die Meldung trägt die echte User-ID des eingeloggten Admins.
		// (Machine-User-Tokens haben keinen name-Claim → Anzeigename ist
		// der Fallback; entscheidend ist die korrekte Zuordnung via sub.)
		if !strings.Contains(text, adminUser.UserID) {
			t.Fatalf("Melder-Sub fehlt (erwartet %s): %s", adminUser.UserID, text)
		}
	})

	// --- Rangliste und Rücknahme ---
	t.Run("Rangliste, eigener Rang und Rücknahme", func(t *testing.T) {
		// Stand bisher: Mitglied 1 Erledigung (gegossen), Admin 1 (via MCP
		// am Beet). Das Mitglied legt zwei nach, der Admin eine.
		var memberCompletionID float64
		for i := 0; i < 2; i++ {
			resp, c := request(t, "POST", fmt.Sprintf("/api/v1/tasks/%.0f/completions", taskID), memberToken, map[string]any{})
			if resp.StatusCode != 201 {
				t.Fatalf("Erledigung %d: HTTP %d: %v", i, resp.StatusCode, c)
			}
			memberCompletionID = c["id"].(float64)
		}
		text, isErr := mcpToolText(t, adminToken, "erledigung_melden", map[string]any{"taskId": taskID})
		if isErr {
			t.Fatalf("erledigung_melden: %s", text)
		}
		var adminCompletion struct {
			ID float64 `json:"id"`
		}
		_ = json.Unmarshal([]byte(text), &adminCompletion)

		// Zeitraum „gesamt", damit der Test unabhängig vom Kalender läuft.
		leaderboard := func(token string) map[string]any {
			t.Helper()
			resp, out := request(t, "GET", "/api/v1/stats/leaderboard?period=gesamt", token, nil)
			if resp.StatusCode != 200 {
				t.Fatalf("Rangliste: HTTP %d: %v", resp.StatusCode, out)
			}
			return out
		}
		// Anzahl der Erledigungen einer Kennung in der Rangliste.
		countOf := func(lb map[string]any, sub string) float64 {
			t.Helper()
			for _, e := range lb["entries"].([]any) {
				entry := e.(map[string]any)
				if entry["userSub"] == sub {
					return entry["completions"].(float64)
				}
			}
			t.Fatalf("Kennung %s fehlt in der Rangliste: %v", sub, lb["entries"])
			return 0
		}

		lb := leaderboard(memberToken)
		if got := countOf(lb, memberUser.UserID); got != 3 {
			t.Fatalf("Mitglied hat %v Erledigungen, erwartet 3", got)
		}
		first := lb["entries"].([]any)[0].(map[string]any)
		if first["userSub"] != memberUser.UserID {
			t.Fatalf("Erster Platz = %v, erwartet das Mitglied", first["userSub"])
		}
		if me := lb["me"].(map[string]any); me["rank"].(float64) != 1 {
			t.Fatalf("eigener Rang des Mitglieds = %v, erwartet 1", me["rank"])
		}
		if totals := lb["totals"].(map[string]any); totals["completions"].(float64) != 5 {
			t.Fatalf("Gesamtsumme = %v, erwartet 5", totals["completions"])
		}

		// Der Admin liegt hinten und bekommt trotzdem seinen Rang.
		if me := leaderboard(adminToken)["me"].(map[string]any); me["rank"].(float64) != 2 {
			t.Fatalf("eigener Rang des Admins = %v, erwartet 2", me["rank"])
		}

		// Fremde Meldung darf das Mitglied nicht zurücknehmen …
		resp, _ := request(t, "DELETE", fmt.Sprintf("/api/v1/completions/%.0f", adminCompletion.ID), memberToken, nil)
		if resp.StatusCode != 403 {
			t.Fatalf("fremde Rücknahme: HTTP %d, erwartet 403", resp.StatusCode)
		}
		// … die eigene schon, und die Rangliste zählt danach eine weniger.
		resp, _ = request(t, "DELETE", fmt.Sprintf("/api/v1/completions/%.0f", memberCompletionID), memberToken, nil)
		if resp.StatusCode != 204 {
			t.Fatalf("eigene Rücknahme: HTTP %d, erwartet 204", resp.StatusCode)
		}
		if got := countOf(leaderboard(memberToken), memberUser.UserID); got != 2 {
			t.Fatalf("Mitglied nach der Rücknahme: %v Erledigungen, erwartet 2", got)
		}

		// Der Admin nimmt seine eigene Meldung über das MCP-Tool zurück.
		if text, isErr := mcpToolText(t, adminToken, "erledigung_zuruecknehmen",
			map[string]any{"id": adminCompletion.ID}); isErr {
			t.Fatalf("erledigung_zuruecknehmen: %s", text)
		}
		if got := countOf(leaderboard(adminToken), adminUser.UserID); got != 1 {
			t.Fatalf("Admin nach der Rücknahme: %v Erledigungen, erwartet 1", got)
		}

		// Und die Rangliste gibt es auch als MCP-Tool.
		text, isErr = mcpToolText(t, adminToken, "rangliste", map[string]any{"period": "gesamt"})
		if isErr || !strings.Contains(text, memberUser.UserID) {
			t.Fatalf("rangliste: isErr=%v, text=%s", isErr, text)
		}
	})
}

func waitFor(t *testing.T, url string) {
	t.Helper()
	for i := 0; i < 60; i++ {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == 200 {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s wurde nicht bereit", url)
}

func base64Decode(s string) ([]byte, error) {
	// Zitadel liefert Standard-Base64.
	return base64.StdEncoding.DecodeString(s)
}
