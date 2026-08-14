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
	"net/url"
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
	// nachtragsName: unter diesem Namen trägt der Admin telefonisch
	// gemeldeten Vollzug nach (force + name).
	nachtragsName = "Telefon-Nachtrag"
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
		// Der Takt der Vergabe läuft im Betrieb jede Minute; im Test soll er
		// nicht bremsen. Die Regeln selbst bleiben unverändert.
		"VERGABE_TAKT=1s",
		// Ideen-Eingang: erlaubtes Weiterleitungsziel wie in Produktion. Die
		// Zugriffsgrenze wird hier hochgesetzt, weil der Test in Folge
		// einreicht — sie hat einen eigenen Test in internal/api.
		"IDEEN_ZIELE=https://xn--rssing-wxa.de",
		"IDEEN_BURST=100",
		"IDEEN_PRO_STUNDE=100",
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

	// --- Spielschutz ---
	// Die Sperrfrist muss am echten Server hängen, nicht in der App: sonst
	// ließe sie sich mit einem selbstgebauten Client umgehen.
	t.Run("Spielschutz: zweite Meldung wird abgewiesen", func(t *testing.T) {
		pfad := fmt.Sprintf("/api/v1/tasks/%.0f/completions", taskID)

		resp, out := request(t, "POST", pfad, memberToken, map[string]any{"liters": 10})
		if resp.StatusCode != 409 {
			t.Fatalf("zweite Meldung: HTTP %d, erwartet 409: %v", resp.StatusCode, out)
		}
		if _, ok := out["error"].(string); !ok {
			t.Fatalf("409 ohne Erklärung: %v", out)
		}
		frei, err := time.Parse(time.RFC3339, out["retryAfter"].(string))
		if err != nil {
			t.Fatalf("retryAfter ist kein RFC3339: %v", out["retryAfter"])
		}

		// Die Ampel-Ansicht sagt dasselbe, damit die App den Knopf sperrt.
		_, list := request(t, "GET", "/api/v1/places", memberToken, nil)
		gesperrt := ""
		for _, p := range list["places"].([]any) {
			for _, task := range p.(map[string]any)["tasks"].([]any) {
				if task.(map[string]any)["id"].(float64) == taskID {
					gesperrt, _ = task.(map[string]any)["lockedUntil"].(string)
				}
			}
		}
		if bis, err := time.Parse(time.RFC3339, gesperrt); err != nil || !bis.Equal(frei) {
			t.Fatalf("lockedUntil = %q, erwartet %s", gesperrt, frei.Format(time.RFC3339))
		}

		// Mitglieder dürfen die Sperre nicht übergehen, Admins schon.
		resp, _ = request(t, "POST", pfad, memberToken, map[string]any{"force": true})
		if resp.StatusCode != 403 {
			t.Fatalf("Mitglied mit force: HTTP %d, erwartet 403", resp.StatusCode)
		}
		resp, nachtrag := request(t, "POST", pfad, adminToken,
			map[string]any{"force": true, "name": nachtragsName, "liters": 8})
		if resp.StatusCode != 201 || nachtrag["forced"] != true {
			t.Fatalf("Admin-Nachtrag: HTTP %d: %v", resp.StatusCode, nachtrag)
		}
		// Und niemand darf in die Zukunft melden.
		resp, _ = request(t, "POST", pfad, adminToken, map[string]any{
			"force": true, "doneAt": time.Now().Add(2 * time.Hour).Format(time.RFC3339)})
		if resp.StatusCode != 400 {
			t.Fatalf("Meldung aus der Zukunft: HTTP %d, erwartet 400", resp.StatusCode)
		}
	})

	// --- Stilllegung ---
	// Eine deaktivierte Aufgabe (Kasten im Winter) oder ein deaktivierter Ort
	// nehmen keine Meldungen mehr an. Auch das muss am Server hängen.
	t.Run("Stillgelegtes nimmt keine Meldungen an", func(t *testing.T) {
		aufgabePfad := fmt.Sprintf("/api/v1/tasks/%.0f", taskID)
		ortPfad := fmt.Sprintf("/api/v1/places/%.0f", placeID)
		meldePfad := fmt.Sprintf("/api/v1/tasks/%.0f/completions", taskID)
		aufgabe := func(aktiv bool) {
			t.Helper()
			resp, out := request(t, "PUT", aufgabePfad, adminToken, map[string]any{
				"kind": "giessen", "liters": 10, "intervalDays": 7, "redAfterDays": 14, "active": aktiv})
			if resp.StatusCode != 200 {
				t.Fatalf("Aufgabe auf active=%v: HTTP %d: %v", aktiv, resp.StatusCode, out)
			}
		}
		ort := func(aktiv bool) {
			t.Helper()
			resp, out := request(t, "PUT", ortPfad, adminToken, map[string]any{
				"name": "E2E-Kasten", "kind": "blumenkasten", "lat": 52.211, "lon": 9.87, "active": aktiv})
			if resp.StatusCode != 200 {
				t.Fatalf("Ort auf active=%v: HTTP %d: %v", aktiv, resp.StatusCode, out)
			}
		}

		aufgabe(false)
		resp, out := request(t, "POST", meldePfad, memberToken, map[string]any{"liters": 10})
		if resp.StatusCode != 409 {
			t.Fatalf("Meldung auf inaktive Aufgabe: HTTP %d, erwartet 409: %v", resp.StatusCode, out)
		}
		if text, _ := out["error"].(string); !strings.Contains(text, "deaktiviert") {
			t.Fatalf("409 ohne verständlichen Grund: %v", out)
		}

		// Jetzt die Aufgabe wieder an, dafür der ganze Ort aus.
		aufgabe(true)
		ort(false)
		resp, out = request(t, "POST", meldePfad, memberToken, map[string]any{"liters": 10})
		if resp.StatusCode != 409 {
			t.Fatalf("Meldung an inaktivem Ort: HTTP %d, erwartet 409: %v", resp.StatusCode, out)
		}
		if text, _ := out["error"].(string); !strings.Contains(text, "deaktiviert") {
			t.Fatalf("409 ohne verständlichen Grund: %v", out)
		}
		// Aufräumen: Der Rest des Tests rechnet mit einem aktiven Ort.
		ort(true)
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
		// am Beet), dazu der Nachtrag aus dem Spielschutz-Block. Das Mitglied
		// legt zwei nach — jede auf einer eigenen Aufgabe, denn dieselbe
		// Aufgabe ist gesperrt (so stehen im Dorf ja auch mehrere Kästen).
		var memberCompletionID float64
		for i := 0; i < 2; i++ {
			resp, task := request(t, "POST", fmt.Sprintf("/api/v1/places/%.0f/tasks", placeID), adminToken,
				map[string]any{"kind": "giessen", "liters": 10, "intervalDays": 7, "redAfterDays": 14})
			if resp.StatusCode != 201 {
				t.Fatalf("Aufgabe %d: HTTP %d: %v", i, resp.StatusCode, task)
			}
			resp, c := request(t, "POST", fmt.Sprintf("/api/v1/tasks/%.0f/completions", task["id"].(float64)),
				memberToken, map[string]any{})
			if resp.StatusCode != 201 {
				t.Fatalf("Erledigung %d: HTTP %d: %v", i, resp.StatusCode, c)
			}
			memberCompletionID = c["id"].(float64)
		}
		// Der Admin trägt über MCP nach — auf der gesperrten Aufgabe geht das
		// nur bewusst mit force.
		text, isErr := mcpToolText(t, adminToken, "erledigung_melden",
			map[string]any{"taskId": taskID, "force": true})
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
		// Gewertete Erledigungen einer Kennung. Nachträge, die unter einem
		// anderen Namen laufen, bleiben außen vor — sie gehören der genannten
		// Person, nicht dem eintragenden Admin.
		countOf := func(lb map[string]any, sub string) float64 {
			t.Helper()
			var n float64
			gefunden := false
			for _, e := range lb["entries"].([]any) {
				entry := e.(map[string]any)
				if entry["userSub"] != sub || entry["userName"] == nachtragsName {
					continue
				}
				n += entry["completions"].(float64)
				gefunden = true
			}
			if !gefunden {
				t.Fatalf("Kennung %s fehlt in der Rangliste: %v", sub, lb["entries"])
			}
			return n
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
		if totals := lb["totals"].(map[string]any); totals["completions"].(float64) != 6 {
			t.Fatalf("Gesamtsumme = %v, erwartet 6", totals["completions"])
		}
		// Der erzwungene Nachtrag zählt der genannten Person — genau einmal.
		nachtrag := 0.0
		for _, e := range lb["entries"].([]any) {
			if e.(map[string]any)["userName"] == nachtragsName {
				nachtrag = e.(map[string]any)["completions"].(float64)
			}
		}
		if nachtrag != 1 {
			t.Fatalf("Nachtrag für %s zählt %v, erwartet 1", nachtragsName, nachtrag)
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

	// --- Profilverwaltung ---
	//
	// Echte Tokens der Rössing-ID, echtes Backend: Was hier durchgeht, geht
	// auch in Produktion durch.
	t.Run("Profil", func(t *testing.T) {
		// Das Profil kommt beim ersten /me aus dem Token — Kontaktdaten sind
		// dabei ausdrücklich noch nicht veröffentlicht.
		_, me := request(t, "GET", "/api/v1/me", memberToken, nil)
		profil, ok := me["profile"].(map[string]any)
		if !ok {
			t.Fatalf("/me liefert kein Profil: %v", me)
		}
		sicht := profil["visibility"].(map[string]any)
		if sicht["phone"] != "verwaltung" || sicht["email"] != "verwaltung" {
			t.Fatalf("Kontaktdaten sind in der Vorbelegung sichtbar: %v", sicht)
		}
		if sicht["displayName"] != "dorf" {
			t.Fatalf("Anzeigename ist in der Vorbelegung nicht sichtbar: %v", sicht)
		}

		// Fremdes Profil ändern ist verboten.
		resp, _ := request(t, "PUT", "/api/v1/me/profile", memberToken,
			map[string]any{"userSub": adminUser.UserID, "displayName": "Fremdgeschrieben"})
		if resp.StatusCode != 403 {
			t.Fatalf("fremdes Profil ändern: HTTP %d, erwartet 403", resp.StatusCode)
		}

		// Kaputte Eingaben werden abgewiesen.
		for name, body := range map[string]map[string]any{
			"kaputte E-Mail": {"email": "keine-adresse"},
			"Telefon-Unsinn": {"phone": "ruf mich an"},
			"Steuerzeichen":  {"nickname": "Gie\u00df\x00meister"},
			"falsche Sicht":  {"visibility": map[string]any{"phone": "alle-welt", "displayName": "dorf", "nickname": "dorf", "email": "dorf", "note": "dorf"}},
		} {
			resp, _ := request(t, "PUT", "/api/v1/me/profile", memberToken, body)
			if resp.StatusCode != 400 {
				t.Errorf("%s: HTTP %d, erwartet 400", name, resp.StatusCode)
			}
		}

		// Das Mitglied pflegt sein Profil und gibt die Telefonnummer bewusst frei.
		nickname := fmt.Sprintf("Gie\u00dfmeister-%d", time.Now().UnixNano())
		resp, gespeichert := request(t, "PUT", "/api/v1/me/profile", memberToken, map[string]any{
			"displayName": "Mitglied aus dem E2E",
			"nickname":    nickname,
			"phone":       "05066 123456",
			"email":       "mitglied@example.org",
			"note":        "erreichbar abends",
			"visibility": map[string]any{
				"displayName": "dorf", "nickname": "dorf",
				"phone": "dorf", "email": "verwaltung", "note": "verwaltung",
			},
		})
		if resp.StatusCode != 200 {
			t.Fatalf("Profil speichern: HTTP %d (%v)", resp.StatusCode, gespeichert)
		}

		// Die Verwaltung sieht in der Mitgliederliste alles — gekennzeichnet.
		_, adminSicht := request(t, "GET", "/api/v1/members", adminToken, nil)
		if adminSicht["adminView"] != true {
			t.Fatalf("adminView fehlt: %v", adminSicht)
		}
		eintrag := mitgliedMit(t, adminSicht, memberUser.UserID)
		if eintrag["phone"] != "05066 123456" || eintrag["email"] != "mitglied@example.org" {
			t.Fatalf("Verwaltung sieht nicht alles: %v", eintrag)
		}
		gesperrt := map[string]bool{}
		for _, f := range eintrag["restricted"].([]any) {
			gesperrt[f.(string)] = true
		}
		if !gesperrt["email"] || !gesperrt["note"] {
			t.Fatalf("restricted = %v, erwartet email und note", eintrag["restricted"])
		}

		// Mit einem Mitglieds-Token verlässt nur das Freigegebene den
		// Server — die Filterung hängt an der Rolle, nicht an der Person.
		_, mitgliedSicht := request(t, "GET", "/api/v1/members", memberToken, nil)
		if mitgliedSicht["adminView"] != false {
			t.Fatalf("adminView ist für ein Mitglied gesetzt: %v", mitgliedSicht)
		}
		eigen := mitgliedMit(t, mitgliedSicht, memberUser.UserID)
		if eigen["phone"] != "05066 123456" {
			t.Fatalf("freigegebene Telefonnummer fehlt: %v", eigen)
		}
		if eigen["email"] != nil || eigen["note"] != nil {
			t.Fatalf("nicht freigegebene Felder wurden ausgeliefert: %v", eigen)
		}
		if len(eigen["restricted"].([]any)) != 0 {
			t.Fatalf("restricted ist für Mitglieder leer, war %v", eigen["restricted"])
		}

		// Und die Rangliste trägt jetzt den Nickname statt des Namens, der
		// beim Melden galt.
		_, liste := request(t, "GET", "/api/v1/stats/leaderboard?period=gesamt", memberToken, nil)
		gefunden := false
		for _, roh := range liste["entries"].([]any) {
			e := roh.(map[string]any)
			if e["userSub"] == memberUser.UserID && e["userName"] == nickname {
				gefunden = true
			}
		}
		if !gefunden {
			t.Fatalf("Rangliste nutzt den Nickname nicht: %v", liste["entries"])
		}
	})

	// --- Vergabe ---
	// Der ganze Weg am echten Server: anmelden, gefragt werden, zusagen,
	// und die Erledigung durch jemand anderen beendet den Vorgang sofort.
	t.Run("Vergabe: anmelden, gefragt werden, zusagen", func(t *testing.T) {
		resp, ort := request(t, "POST", "/api/v1/places", adminToken,
			map[string]any{"name": "E2E-Vergabe-Kasten", "kind": "blumenkasten", "lat": 52.212, "lon": 9.871})
		if resp.StatusCode != 201 {
			t.Fatalf("Ort: HTTP %d: %v", resp.StatusCode, ort)
		}
		vergabeOrt := ort["id"].(float64)
		resp, aufgabe := request(t, "POST", fmt.Sprintf("/api/v1/places/%.0f/tasks", vergabeOrt), adminToken,
			map[string]any{"kind": "giessen", "liters": 10, "intervalDays": 7, "redAfterDays": 14})
		if resp.StatusCode != 201 {
			t.Fatalf("Aufgabe: HTTP %d: %v", resp.StatusCode, aufgabe)
		}
		vergabeAufgabe := aufgabe["id"].(float64)

		// Ohne Angemeldete passiert nichts — auch wenn die Aufgabe fällig
		// ist. Fällig wird sie durch einen Nachtrag von vor acht Tagen.
		resp, nachtrag := request(t, "POST", fmt.Sprintf("/api/v1/tasks/%.0f/completions", vergabeAufgabe), adminToken,
			map[string]any{"force": true, "doneAt": time.Now().Add(-8 * 24 * time.Hour).Format(time.RFC3339)})
		if resp.StatusCode != 201 {
			t.Fatalf("Nachtrag: HTTP %d: %v", resp.StatusCode, nachtrag)
		}
		time.Sleep(2 * time.Second)
		if _, offen := request(t, "GET", "/api/v1/me/notifications", memberToken, nil); len(offen["notifications"].([]any)) != 0 {
			t.Fatalf("Anfrage ohne Anmeldung: %v", offen["notifications"])
		}

		// Jetzt meldet sich das Mitglied an und wird gefragt.
		resp, angemeldet := request(t, "POST", fmt.Sprintf("/api/v1/places/%.0f/signup", vergabeOrt), memberToken, nil)
		if resp.StatusCode != 201 {
			t.Fatalf("Anmelden: HTTP %d: %v", resp.StatusCode, angemeldet)
		}
		anfrage := warteAufAnfrage(t, memberToken, vergabeAufgabe)
		for _, feld := range []string{"placeName", "taskName", "title", "text", "expiresAt"} {
			if anfrage[feld] == nil || anfrage[feld] == "" {
				t.Errorf("Anfrage ohne %s: %v", feld, anfrage)
			}
		}

		// Empfang bestätigen und zusagen.
		resp, _ = request(t, "POST", fmt.Sprintf("/api/v1/me/notifications/%.0f/ack", anfrage["id"].(float64)), memberToken, nil)
		if resp.StatusCode != 204 {
			t.Fatalf("Empfang bestätigen: HTTP %d", resp.StatusCode)
		}
		vorgang := anfrage["assignmentId"].(float64)
		resp, zusage := request(t, "POST", fmt.Sprintf("/api/v1/assignments/%.0f/claim", vorgang), memberToken, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("Zusage: HTTP %d: %v", resp.StatusCode, zusage)
		}
		if zusage["claimedUntil"] == nil {
			t.Fatalf("Zusage ohne Frist: %v", zusage)
		}
		// Ein zweiter Zugriff prallt ab — auch der des Admins.
		resp, konflikt := request(t, "POST", fmt.Sprintf("/api/v1/assignments/%.0f/claim", vorgang), adminToken, nil)
		if resp.StatusCode != 409 {
			t.Fatalf("zweite Zusage: HTTP %d, erwartet 409: %v", resp.StatusCode, konflikt)
		}

		// Die Orts-Liste zeigt „übernommen von … bis …".
		_, liste := request(t, "GET", "/api/v1/places", adminToken, nil)
		if a := vergabeStandVon(t, liste, vergabeAufgabe); a == nil || a["claimedBy"] != memberUser.UserID {
			t.Fatalf("Vergabestand = %v", a)
		}

		// Und die Verwaltung sieht, wer angemeldet ist.
		_, angemeldete := request(t, "GET", fmt.Sprintf("/api/v1/places/%.0f/signups", vergabeOrt), adminToken, nil)
		if len(angemeldete["signups"].([]any)) != 1 {
			t.Fatalf("Anmeldungen in der Verwaltung: %v", angemeldete)
		}
		resp, _ = request(t, "GET", fmt.Sprintf("/api/v1/places/%.0f/signups", vergabeOrt), memberToken, nil)
		if resp.StatusCode != 403 {
			t.Fatalf("Mitglied sieht fremde Anmeldungen: HTTP %d", resp.StatusCode)
		}

		// Jemand anderes gießt: Der Vorgang endet sofort, offene Anfragen
		// erlöschen, und niemand wird mehr gefragt.
		resp, erledigt := request(t, "POST", fmt.Sprintf("/api/v1/tasks/%.0f/completions", vergabeAufgabe), adminToken,
			map[string]any{"force": true, "name": "Nachbarschaftshilfe"})
		if resp.StatusCode != 201 {
			t.Fatalf("fremde Erledigung: HTTP %d: %v", resp.StatusCode, erledigt)
		}
		time.Sleep(2 * time.Second)
		_, liste = request(t, "GET", "/api/v1/places", adminToken, nil)
		if a := vergabeStandVon(t, liste, vergabeAufgabe); a != nil {
			t.Fatalf("Vorgang läuft nach der Erledigung weiter: %v", a)
		}
		_, offen := request(t, "GET", "/api/v1/me/notifications", memberToken, nil)
		for _, roh := range offen["notifications"].([]any) {
			n := roh.(map[string]any)
			if n["taskId"] == vergabeAufgabe && (n["kind"] == "anfrage" || n["kind"] == "rundruf") {
				t.Fatalf("Anfrage nach der Erledigung noch offen: %v", n)
			}
		}

		// Abmelden geht jederzeit.
		resp, _ = request(t, "DELETE", fmt.Sprintf("/api/v1/places/%.0f/signup", vergabeOrt), memberToken, nil)
		if resp.StatusCode != 204 {
			t.Fatalf("Abmelden: HTTP %d", resp.StatusCode)
		}
	})

	// --- Ideen-Sammlung ---
	t.Run("Ideen: öffentlicher Eingang, Missbrauchsschutz, Verwaltung", func(t *testing.T) {
		// Ohne Anmeldung einreichen — genau so kommt es von der Website.
		wunsch := fmt.Sprintf("E2E-Wunsch %d: ein Mitfahrbrett für Fahrten nach Hildesheim.", time.Now().UnixNano())
		resp := formular(t, "/api/v1/ideen", "", nil, map[string]string{
			"name": "Erna E2E", "email": "erna@example.org", "wunsch": wunsch,
		})
		resp.Body.Close()
		if resp.StatusCode != 201 {
			t.Fatalf("öffentliche Einreichung: HTTP %d", resp.StatusCode)
		}

		// Aus der angemeldeten App: die Idee hängt am Konto.
		appWunsch := fmt.Sprintf("E2E-App-Wunsch %d: Erinnerungen bitte auch abends.", time.Now().UnixNano())
		resp = formular(t, "/api/v1/ideen", memberToken, nil, map[string]string{"wunsch": appWunsch})
		resp.Body.Close()
		if resp.StatusCode != 201 {
			t.Fatalf("Einreichung aus der App: HTTP %d", resp.StatusCode)
		}

		// Honigtopf: freundliche 201, aber nichts wird gespeichert.
		resp = formular(t, "/api/v1/ideen", "", nil, map[string]string{
			"wunsch": "E2E-Honigtopf: das darf nirgends auftauchen.", "webseite": "http://spam.example",
		})
		resp.Body.Close()
		if resp.StatusCode != 201 {
			t.Fatalf("Honigtopf: HTTP %d, erwartet 201", resp.StatusCode)
		}

		// Zu kurzer Wunsch → 400, nichts gespeichert.
		resp = formular(t, "/api/v1/ideen", "", nil, map[string]string{"wunsch": "hm"})
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("zu kurzer Wunsch: HTTP %d, erwartet 400", resp.StatusCode)
		}

		// Weiterleitung nur auf erlaubte Ziele.
		resp = formular(t, "/api/v1/ideen", "", nil, map[string]string{
			"wunsch": "E2E-Umleitung: das darf nirgends auftauchen.", "redirect": "https://boese.example/",
		})
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Fatalf("fremdes Weiterleitungsziel: HTTP %d, erwartet 400", resp.StatusCode)
		}
		resp = formular(t, "/api/v1/ideen", "", nil, map[string]string{
			"wunsch": fmt.Sprintf("E2E-Danke %d: Weiterleitung prüfen.", time.Now().UnixNano()),
			// Muss zu IDEEN_ZIELE unten passen.
			"redirect": "https://xn--rssing-wxa.de/app/danke",
		})
		resp.Body.Close()
		if resp.StatusCode != 303 || resp.Header.Get("Location") != "https://xn--rssing-wxa.de/app/danke" {
			t.Fatalf("erlaubtes Ziel: HTTP %d, Location %q", resp.StatusCode, resp.Header.Get("Location"))
		}

		// Lesen und Ändern darf nur die Verwaltung.
		for _, methode := range []string{"GET"} {
			r, _ := request(t, methode, "/api/v1/ideen", memberToken, nil)
			if r.StatusCode != 403 {
				t.Fatalf("Mitglied darf Ideen lesen: HTTP %d", r.StatusCode)
			}
		}

		_, liste := request(t, "GET", "/api/v1/ideen", adminToken, nil)
		ideen := liste["ideen"].([]any)
		var meine, ausDerApp map[string]any
		for _, roh := range ideen {
			i := roh.(map[string]any)
			if i["wunsch"] == wunsch {
				meine = i
			}
			if i["wunsch"] == appWunsch {
				ausDerApp = i
			}
			if s, _ := i["wunsch"].(string); strings.Contains(s, "E2E-Honigtopf") || strings.Contains(s, "E2E-Umleitung") {
				t.Fatalf("verworfene Einreichung ist gespeichert: %v", i)
			}
		}
		if meine == nil {
			t.Fatalf("eingereichte Idee fehlt in der Liste: %v", ideen)
		}
		if meine["status"] != "neu" || meine["quelle"] != "website" || meine["userSub"] != "" {
			t.Fatalf("Idee falsch abgelegt: %v", meine)
		}
		if ausDerApp == nil || ausDerApp["quelle"] != "app" || ausDerApp["userSub"] == "" {
			t.Fatalf("Idee aus der App ist keinem Konto zugeordnet: %v", ausDerApp)
		}

		// Stand und Notiz ändern — auch das nur als Admin.
		id := meine["id"].(float64)
		pfad := fmt.Sprintf("/api/v1/ideen/%.0f", id)
		if r, _ := request(t, "PATCH", pfad, memberToken, map[string]any{"status": "gelesen"}); r.StatusCode != 403 {
			t.Fatalf("Mitglied darf Ideen ändern: HTTP %d", r.StatusCode)
		}
		resp2, geaendert := request(t, "PATCH", pfad, adminToken,
			map[string]any{"status": "umgesetzt", "notiz": "E2E-Notiz"})
		if resp2.StatusCode != 200 || geaendert["status"] != "umgesetzt" {
			t.Fatalf("Statuswechsel: HTTP %d: %v", resp2.StatusCode, geaendert)
		}

		// Über MCP sichtbar und änderbar.
		text, fehler := mcpToolText(t, adminToken, "ideen_liste", map[string]any{"status": "umgesetzt"})
		if fehler || !strings.Contains(text, wunsch) {
			t.Fatalf("ideen_liste über MCP: %s", text)
		}
		text, fehler = mcpToolText(t, adminToken, "idee_status_setzen",
			map[string]any{"id": id, "status": "abgelehnt", "notiz": "E2E über MCP"})
		if fehler || !strings.Contains(text, "abgelehnt") {
			t.Fatalf("idee_status_setzen über MCP: %s", text)
		}

		// Aufräumen: die E2E-Einträge wieder löschen.
		if r, _ := request(t, "DELETE", pfad, memberToken, nil); r.StatusCode != 403 {
			t.Fatalf("Mitglied darf Ideen löschen: HTTP %d", r.StatusCode)
		}
		for _, roh := range ideen {
			i := roh.(map[string]any)
			r, _ := request(t, "DELETE", fmt.Sprintf("/api/v1/ideen/%.0f", i["id"].(float64)), adminToken, nil)
			if r.StatusCode != 204 {
				t.Fatalf("Idee löschen: HTTP %d", r.StatusCode)
			}
		}
	})
}

// formular schickt ein klassisches HTML-Formular (so kommt es von der
// Website) und folgt Weiterleitungen bewusst nicht.
func formular(t *testing.T, pfad, token string, kopf map[string]string, werte map[string]string) *http.Response {
	t.Helper()
	form := url.Values{}
	for k, v := range werte {
		form.Set(k, v)
	}
	req, _ := http.NewRequest("POST", backendAddr+pfad, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range kopf {
		req.Header.Set(k, v)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// warteAufAnfrage pollt die Benachrichtigungen, bis eine Anfrage zu dieser
// Aufgabe da ist — der Zeitgeber im Server braucht einen Takt dafür.
func warteAufAnfrage(t *testing.T, token string, taskID float64) map[string]any {
	t.Helper()
	for i := 0; i < 60; i++ {
		_, offen := request(t, "GET", "/api/v1/me/notifications", token, nil)
		for _, roh := range offen["notifications"].([]any) {
			n := roh.(map[string]any)
			if n["taskId"] == taskID && n["kind"] == "anfrage" {
				return n
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("es kam keine Anfrage an")
	return nil
}

// vergabeStandVon sucht den Vorgang einer Aufgabe in der Orts-Liste.
func vergabeStandVon(t *testing.T, liste map[string]any, taskID float64) map[string]any {
	t.Helper()
	for _, p := range liste["places"].([]any) {
		for _, roh := range p.(map[string]any)["tasks"].([]any) {
			task := roh.(map[string]any)
			if task["id"] != taskID {
				continue
			}
			if a, ok := task["assignment"].(map[string]any); ok {
				return a
			}
			return nil
		}
	}
	t.Fatalf("Aufgabe %.0f fehlt in der Orts-Liste", taskID)
	return nil
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

// mitgliedMit sucht einen Eintrag der Dorfbewohner-Liste.
func mitgliedMit(t *testing.T, antwort map[string]any, userSub string) map[string]any {
	t.Helper()
	for _, roh := range antwort["members"].([]any) {
		m := roh.(map[string]any)
		if m["userSub"] == userSub {
			return m
		}
	}
	t.Fatalf("Kennung %s fehlt in der Mitgliederliste: %v", userSub, antwort["members"])
	return nil
}
