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
		// Empfängerprüfung wie in Produktion. Die Machine-User holen ihre
		// Tokens unten mit dem Scope „…:project:id:<projectID>:aud", tragen
		// die Projekt-ID also als Empfänger — genau darauf wird hier geprüft.
		// Ohne diese Zeile verweigert der Server im OIDC-Modus den Start.
		"AUTH_AUDIENCE="+projectID,
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
		// Träger-Mitgliedschaften: Das Backend fragt sie mit einem
		// Dienst-Nutzer über die echte Management-API ab. Im E2E ist das
		// derselbe Machine-Key, mit dem auch der Bootstrap läuft.
		"ZITADEL_SERVICE_USER_KEY_FILE="+machineKeyPfad(t),
		// Kurze Frist, damit der Test nicht auf den Zwischenspeicher wartet.
		// Im Betrieb sind es 45 Sekunden.
		"ZITADEL_ROLLEN_TTL=1s",
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

	// --- Einmalige Aufgaben (#6) und die Verwaltung aus der App (#5, #7) ---
	//
	// Der ganze Weg mit echten Tokens: anlegen → erscheint mit der richtigen
	// Ampel → wird erledigt → verschwindet bzw. bleibt. Und: Ohne die Rolle
	// „admin" geht gar nichts, egal mit welchem Client.
	// Der Rollen-Scope entscheidet, ob überhaupt jemand verwalten darf.
	//
	// Das ist kein theoretischer Fall: Die Android-App forderte anfangs nur
	// „openid profile email offline_access" an. Zitadel stellte daraufhin ein
	// Token ganz ohne Rollen aus — in der ausgelieferten App war damit
	// niemand Verwaltung, auch nicht, wer die Rolle in Zitadel hatte. Alle
	// Pflege-Endpunkte antworteten mit 403.
	//
	// Der Test hält beide Richtungen fest, am echten Aussteller: ohne den
	// Scope keine Rechte, mit ihm die vollen.
	t.Run("Ohne Rollen-Scope ist niemand Verwaltung", func(t *testing.T) {
		// Einziger Unterschied zum Token oben: der Rollen-Scope fehlt. Die
		// Projekt-Audience bleibt, sonst wäre das Token schon deshalb
		// ungültig und der Test würde etwas anderes messen.
		audienz := "openid profile email urn:zitadel:iam:org:project:id:" + projectID + ":aud"
		ohneRollen := fetchToken(t, adminUser, audienz)

		_, me := request(t, "GET", "/api/v1/me", ohneRollen, nil)
		if me["isAdmin"] != false {
			t.Fatalf("ohne Rollen-Scope trotzdem Admin: %v", me)
		}
		if rollen, ok := me["roles"].([]any); ok && len(rollen) != 0 {
			t.Fatalf("ohne Rollen-Scope kamen Rollen an: %v", rollen)
		}
		// Und damit steht die Verwaltung offen wie ein Scheunentor — nach innen:
		// Sie lässt niemanden mehr durch.
		resp, out := request(t, "POST", "/api/v1/places", ohneRollen,
			map[string]any{"name": "Ohne Rollen", "lat": 52.2, "lon": 9.8})
		if resp.StatusCode != 403 {
			t.Fatalf("Ort anlegen ohne Rollen-Scope: HTTP %d, erwartet 403: %v", resp.StatusCode, out)
		}

		// Mit dem Scope, den die App jetzt anfordert, geht es.
		mitRollen := fetchToken(t, adminUser, audienz+" urn:zitadel:iam:org:projects:roles")
		_, me = request(t, "GET", "/api/v1/me", mitRollen, nil)
		if me["isAdmin"] != true {
			t.Fatalf("mit Rollen-Scope nicht als Verwaltung erkannt: %v", me)
		}
		resp, out = request(t, "POST", "/api/v1/places", mitRollen,
			map[string]any{"name": "Mit Rollen " + fmt.Sprint(time.Now().UnixNano()), "lat": 52.2, "lon": 9.8})
		if resp.StatusCode != 201 {
			t.Fatalf("Ort anlegen mit Rollen-Scope: HTTP %d, erwartet 201: %v", resp.StatusCode, out)
		}
	})

	t.Run("Mitglied darf keine Aufgaben pflegen", func(t *testing.T) {
		morgen := time.Now().AddDate(0, 0, 2).Format("2006-01-02")
		faelle := []struct {
			name   string
			method string
			pfad   string
			koerp  map[string]any
		}{
			{"Aufgabe anlegen", "POST", fmt.Sprintf("/api/v1/places/%.0f/tasks", placeID),
				map[string]any{"kind": "sonstiges", "title": "Heimlich", "oneOff": true, "dueDate": morgen}},
			{"Aufgabe ändern", "PUT", fmt.Sprintf("/api/v1/tasks/%.0f", taskID),
				map[string]any{"kind": "giessen", "intervalDays": 1, "redAfterDays": 2}},
			{"Aufgabe löschen", "DELETE", fmt.Sprintf("/api/v1/tasks/%.0f", taskID), nil},
			{"Ort ändern", "PUT", fmt.Sprintf("/api/v1/places/%.0f", placeID),
				map[string]any{"name": "Umbenannt", "lat": 52.2, "lon": 9.8}},
			{"Ort löschen", "DELETE", fmt.Sprintf("/api/v1/places/%.0f", placeID), nil},
		}
		for _, f := range faelle {
			var koerper any
			if f.koerp != nil {
				koerper = f.koerp
			}
			resp, out := request(t, f.method, f.pfad, memberToken, koerper)
			if resp.StatusCode != 403 {
				t.Fatalf("%s als Mitglied: HTTP %d, erwartet 403: %v", f.name, resp.StatusCode, out)
			}
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

	// Steht bewusst am Ende: Der Block meldet zwei Erledigungen, und die
	// Zählungen der Rangliste weiter oben rechnen mit festen Zahlen.
	t.Run("Einmalige Aufgabe: Termin macht die Ampel", func(t *testing.T) {
		// Ein eigener Ort, damit der Gießplan oben unberührt bleibt.
		_, ort := request(t, "POST", "/api/v1/places", adminToken,
			map[string]any{"name": "E2E-Bahnhof", "kind": "sonstiges", "lat": 52.2108, "lon": 9.8692})
		bahnhof := ort["id"].(float64)
		tasksPfad := fmt.Sprintf("/api/v1/places/%.0f/tasks", bahnhof)

		anlegen := func(titel, termin string, entfernen bool) float64 {
			t.Helper()
			resp, out := request(t, "POST", tasksPfad, adminToken, map[string]any{
				"kind": "sonstiges", "title": titel,
				"oneOff": true, "dueDate": termin, "removeWhenDone": entfernen,
			})
			if resp.StatusCode != 201 {
				t.Fatalf("%s: HTTP %d: %v", titel, resp.StatusCode, out)
			}
			if out["oneOff"] != true {
				t.Fatalf("%s wurde nicht als einmalig gespeichert: %v", titel, out)
			}
			return out["id"].(float64)
		}
		tag := func(abstand int) string {
			return time.Now().AddDate(0, 0, abstand).Format("2006-01-02")
		}

		fern := anlegen("Weit weg", tag(30), false)
		bald := anlegen("Übermorgen", tag(1), false)
		vorbei := anlegen("Längst fällig", tag(-2), false)
		weg := anlegen("Zum Bahnhof fahren", tag(10), true)

		status := func() map[float64]string {
			t.Helper()
			_, liste := request(t, "GET", "/api/v1/places", memberToken, nil)
			out := map[float64]string{}
			for _, p := range liste["places"].([]any) {
				ortDaten := p.(map[string]any)
				if ortDaten["id"].(float64) != bahnhof {
					continue
				}
				for _, x := range ortDaten["tasks"].([]any) {
					aufgabe := x.(map[string]any)
					out[aufgabe["id"].(float64)] = aufgabe["status"].(string)
				}
			}
			return out
		}
		stand := status()
		for id, erwartet := range map[float64]string{fern: "green", bald: "yellow", vorbei: "red", weg: "green"} {
			if stand[id] != erwartet {
				t.Errorf("Aufgabe %.0f: Status %q, erwartet %q", id, stand[id], erwartet)
			}
		}

		// Eine einmalige Aufgabe darf nicht ohne Termin entstehen.
		resp, out := request(t, "POST", tasksPfad, adminToken,
			map[string]any{"kind": "sonstiges", "title": "Ohne Termin", "oneOff": true})
		if resp.StatusCode != 400 {
			t.Fatalf("einmalig ohne Termin: HTTP %d, erwartet 400: %v", resp.StatusCode, out)
		}

		// Erledigen: „nach dem Erledigen entfernen" nimmt sie von der Karte,
		// die Rangliste behält sie.
		_, vorRang := request(t, "GET", "/api/v1/stats/leaderboard?period=gesamt", memberToken, nil)
		vorher := vorRang["totals"].(map[string]any)["completions"].(float64)

		resp, out = request(t, "POST", fmt.Sprintf("/api/v1/tasks/%.0f/completions", weg), memberToken, map[string]any{})
		if resp.StatusCode != 201 {
			t.Fatalf("Erledigung: HTTP %d: %v", resp.StatusCode, out)
		}
		resp, out = request(t, "POST", fmt.Sprintf("/api/v1/tasks/%.0f/completions", vorbei), memberToken, map[string]any{})
		if resp.StatusCode != 201 {
			t.Fatalf("Erledigung: HTTP %d: %v", resp.StatusCode, out)
		}

		stand = status()
		if _, da := stand[weg]; da {
			t.Error("Die erledigte einmalige Aufgabe steht noch auf der Karte")
		}
		if stand[vorbei] != "green" {
			t.Errorf("Erledigte einmalige Aufgabe ohne Schalter: Status %q, erwartet green", stand[vorbei])
		}

		_, nachRang := request(t, "GET", "/api/v1/stats/leaderboard?period=gesamt", memberToken, nil)
		nachher := nachRang["totals"].(map[string]any)["completions"].(float64)
		if nachher != vorher+2 {
			t.Errorf("Rangliste: %v → %v, erwartet zwei Erledigungen mehr", vorher, nachher)
		}

		// Und zweimal erledigen geht nicht: einmalig ist einmalig.
		resp, out = request(t, "POST", fmt.Sprintf("/api/v1/tasks/%.0f/completions", vorbei), memberToken, map[string]any{})
		if resp.StatusCode != 409 {
			t.Fatalf("zweite Meldung auf einmalige Aufgabe: HTTP %d, erwartet 409: %v", resp.StatusCode, out)
		}
	})

	// --- Träger, Sichtbarkeit und Befähigungen ---------------------------
	//
	// Der Kern der Umstellung, gegen ein ECHTES Zitadel: Ein zweites Projekt
	// ist ein zweiter Verein. Seine Rollen stehen in KEINEM Token — das
	// Backend fragt sie mit dem Dienst-Nutzer über die Management-API ab.
	t.Run("Träger: Rollen aus einem fremden Zitadel-Projekt greifen", func(t *testing.T) {
		// 1. Ein echtes zweites Projekt mit den zwei Rollen anlegen.
		traegerProjekt := zapi(t, iamToken, "POST", "/management/v1/projects",
			map[string]any{"name": fmt.Sprintf("dorfpflege-e2e-%d", time.Now().UnixNano())})
		traegerProjektID := traegerProjekt["id"].(string)
		for _, rolle := range []map[string]any{
			{"roleKey": "admin", "displayName": "Verwaltung"},
			{"roleKey": "mitglied", "displayName": "Mitglied"},
		} {
			zapi(t, iamToken, "POST", "/management/v1/projects/"+traegerProjektID+"/roles", rolle)
		}

		// 2. Den Träger im Backend anlegen und zulassen (nur der Betreiber).
		resp, traeger := request(t, "POST", "/api/v1/traeger", adminToken, map[string]any{
			"name": "Dorfpflege", "projektId": traegerProjektID,
			"status": "zugelassen", "sichtbarkeit": "offen",
		})
		if resp.StatusCode != 201 {
			t.Fatalf("Träger anlegen: HTTP %d: %v", resp.StatusCode, traeger)
		}
		traegerID := traeger["id"].(float64)

		// 3. Ein Vorstandsmitglied mit der admin-Rolle DIESES Projekts.
		// Sein Token trägt die Rolle nicht — es ist für die Dorf-App
		// ausgestellt, nicht für den Verein.
		vorstandUser := newMachine("Dorfpflege-Vorstand", nil)
		zapi(t, iamToken, "POST", "/management/v1/users/"+vorstandUser.UserID+"/grants",
			map[string]any{"projectId": traegerProjektID, "roleKeys": []string{"admin"}})
		vorstandToken := fetchToken(t, vorstandUser, scope)

		// 4. Der Vorstand legt einen Ort mit einer INTERNEN Aufgabe an.
		resp, ort := request(t, "POST", "/api/v1/places", vorstandToken, map[string]any{
			"name": "Gerätehaus", "kind": "sonstiges", "lat": 52.212, "lon": 9.871,
			"traegerId": traegerID})
		if resp.StatusCode != 201 {
			t.Fatalf("Der Träger-Admin darf keinen Ort anlegen: HTTP %d: %v", resp.StatusCode, ort)
		}
		ortID := ort["id"].(float64)

		resp, intern := request(t, "POST", fmt.Sprintf("/api/v1/places/%.0f/tasks", ortID),
			vorstandToken, map[string]any{"kind": "sonstiges", "title": "Interne Prüfung",
				"intervalDays": 30, "redAfterDays": 60, "sichtbarkeit": "nur_mitglieder"})
		if resp.StatusCode != 201 {
			t.Fatalf("interne Aufgabe: HTTP %d: %v", resp.StatusCode, intern)
		}
		internID := intern["id"].(float64)

		// 5. Das gewöhnliche Mitglied der Dorf-App gehört dem Verein NICHT an
		// und darf die Aufgabe auf keinem Weg sehen.
		sieht := func(token string, taskID float64) bool {
			_, liste := request(t, "GET", "/api/v1/places", token, nil)
			for _, p := range liste["places"].([]any) {
				for _, roh := range p.(map[string]any)["tasks"].([]any) {
					if roh.(map[string]any)["id"] == taskID {
						return true
					}
				}
			}
			return false
		}
		if sieht(memberToken, internID) {
			t.Fatal("die interne Aufgabe ist außerhalb des Trägers sichtbar")
		}
		resp, _ = request(t, "GET", fmt.Sprintf("/api/v1/tasks/%.0f/completions", internID),
			memberToken, nil)
		if resp.StatusCode != 404 {
			t.Errorf("Historie der internen Aufgabe: HTTP %d, erwartet 404", resp.StatusCode)
		}
		resp, _ = request(t, "POST", fmt.Sprintf("/api/v1/tasks/%.0f/completions", internID),
			memberToken, map[string]any{})
		if resp.StatusCode != 404 {
			t.Errorf("Meldung auf die interne Aufgabe: HTTP %d, erwartet 404", resp.StatusCode)
		}
		if !sieht(vorstandToken, internID) {
			t.Fatal("der Träger-Admin sieht seine eigene interne Aufgabe nicht")
		}

		// 6. DAS Versprechen des Entwurfs: Eine frisch erteilte Mitgliedschaft
		// wirkt sofort — mit DEMSELBEN Token, ohne Ab- und Anmelden.
		zapi(t, iamToken, "POST", "/management/v1/users/"+memberUser.UserID+"/grants",
			map[string]any{"projectId": traegerProjektID, "roleKeys": []string{"mitglied"}})
		sichtbarGeworden := false
		for i := 0; i < 20; i++ {
			if sieht(memberToken, internID) {
				sichtbarGeworden = true
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if !sichtbarGeworden {
			t.Fatal("die neue Mitgliedschaft wirkt nicht ohne erneute Anmeldung")
		}

		// 7. Mitglied ist nicht Verwaltung: Ändern bleibt verwehrt.
		resp, _ = request(t, "PUT", fmt.Sprintf("/api/v1/tasks/%.0f", internID), memberToken,
			map[string]any{"kind": "sonstiges", "intervalDays": 5, "redAfterDays": 10})
		if resp.StatusCode != 403 {
			t.Errorf("Mitglied ändert die Aufgabe: HTTP %d, erwartet 403", resp.StatusCode)
		}

		// 8. Befähigung: ohne Einweisung keine Zusage.
		resp, befaehigung := request(t, "POST",
			fmt.Sprintf("/api/v1/traeger/%.0f/befaehigungen", traegerID), vorstandToken,
			map[string]any{"name": "Motorsense", "beschreibung": "Einweisung am Gerät"})
		if resp.StatusCode != 201 {
			t.Fatalf("Befähigung: HTTP %d: %v", resp.StatusCode, befaehigung)
		}
		befaehigungID := befaehigung["id"].(float64)

		// Die Aufgabe ist einmalig mit einem Termin von gestern — damit ist
		// sie sofort fällig und der Zeitgeber eröffnet einen Vorgang.
		gestern := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		resp, mit := request(t, "POST", fmt.Sprintf("/api/v1/places/%.0f/tasks", ortID),
			vorstandToken, map[string]any{"kind": "sonstiges", "title": "Rasenmähen",
				"oneOff": true, "dueDate": gestern, "sichtbarkeit": "oeffentlich",
				"befaehigungId": befaehigungID})
		if resp.StatusCode != 201 {
			t.Fatalf("Aufgabe mit Einweisung: HTTP %d: %v", resp.StatusCode, mit)
		}
		mitID := mit["id"].(float64)

		// Anmelden zum Mithelfen darf jede und jeder — daran hängt die
		// Einweisung nicht.
		for _, token := range []string{memberToken, vorstandToken} {
			resp, out := request(t, "POST", fmt.Sprintf("/api/v1/places/%.0f/signup", ortID),
				token, map[string]any{})
			if resp.StatusCode >= 300 {
				t.Fatalf("Anmelden: HTTP %d: %v", resp.StatusCode, out)
			}
		}

		// Der Vorstand hat die Einweisung (er trägt sie sich selbst ein — er
		// verwaltet den Träger). Damit gibt es überhaupt jemanden, den die
		// Vergabe fragen kann, und der Vorgang entsteht.
		befaehigungErteilen(t, befaehigungID, vorstandToken, vorstandToken)
		vorgangID := warteAufVorgang(t, memberToken, mitID)

		// Das Mitglied ohne Einweisung kann NICHT zusagen — serverseitig.
		resp, out := request(t, "POST", fmt.Sprintf("/api/v1/assignments/%.0f/claim", vorgangID),
			memberToken, nil)
		if resp.StatusCode != 403 {
			t.Fatalf("Zusage ohne Einweisung: HTTP %d, erwartet 403: %v", resp.StatusCode, out)
		}
		if text, _ := out["error"].(string); text == "" {
			t.Error("403 ohne verständliche Begründung")
		}

		// Beantragen — und niemand entscheidet über sich selbst, wenn er den
		// Träger nicht verwaltet.
		resp, antrag := request(t, "POST",
			fmt.Sprintf("/api/v1/befaehigungen/%.0f/antrag", befaehigungID), memberToken,
			map[string]any{"begruendung": "War bei der Einweisung dabei"})
		if resp.StatusCode != 201 {
			t.Fatalf("Antrag: HTTP %d: %v", resp.StatusCode, antrag)
		}
		antragID := antrag["id"].(float64)

		resp, _ = request(t, "POST", fmt.Sprintf("/api/v1/antraege/%.0f", antragID), memberToken,
			map[string]any{"status": "erteilt"})
		if resp.StatusCode != 403 {
			t.Errorf("Selbstfreigabe: HTTP %d, erwartet 403", resp.StatusCode)
		}
		resp, out = request(t, "POST", fmt.Sprintf("/api/v1/antraege/%.0f", antragID), vorstandToken,
			map[string]any{"status": "erteilt", "notiz": "eingewiesen"})
		if resp.StatusCode != 200 {
			t.Fatalf("Freigabe: HTTP %d: %v", resp.StatusCode, out)
		}
		resp, out = request(t, "POST", fmt.Sprintf("/api/v1/assignments/%.0f/claim", vorgangID),
			memberToken, nil)
		if resp.StatusCode != 200 {
			t.Fatalf("Zusage mit Einweisung: HTTP %d, erwartet 200: %v", resp.StatusCode, out)
		}

		// 9. Sperrt der Betreiber den Träger, verschwindet alles davon.
		resp, _ = request(t, "PUT", fmt.Sprintf("/api/v1/traeger/%.0f", traegerID), adminToken,
			map[string]any{"name": "Dorfpflege", "projektId": traegerProjektID,
				"status": "gesperrt", "sichtbarkeit": "offen"})
		if resp.StatusCode != 200 {
			t.Fatalf("Sperren: HTTP %d", resp.StatusCode)
		}
		if sieht(memberToken, mitID) {
			t.Error("die Aufgabe eines gesperrten Trägers ist noch sichtbar")
		}
	})

	// --- Beitritt: beantragen, freigeben, wirklich Mitglied sein -----------
	//
	// Der Punkt, an dem sich entscheidet, ob das Verfahren etwas taugt: Die
	// Freigabe muss die Rollenzuweisung in Zitadel ZURÜCKSCHREIBEN. Geprüft
	// wird deshalb nicht die eigene Datenbank, sondern die Management-API —
	// und danach die Wirkung mit demselben Token, ohne neue Anmeldung.
	t.Run("Träger: Beitritt beantragen und freigeben schreibt nach Zitadel zurück", func(t *testing.T) {
		beitrittProjekt := zapi(t, iamToken, "POST", "/management/v1/projects",
			map[string]any{"name": fmt.Sprintf("ak2-e2e-%d", time.Now().UnixNano())})
		beitrittProjektID := beitrittProjekt["id"].(string)
		for _, rolle := range []map[string]any{
			{"roleKey": "admin", "displayName": "Verwaltung"},
			{"roleKey": "mitglied", "displayName": "Mitglied"},
		} {
			zapi(t, iamToken, "POST", "/management/v1/projects/"+beitrittProjektID+"/roles", rolle)
		}

		resp, traeger := request(t, "POST", "/api/v1/traeger", adminToken, map[string]any{
			"name": "AK 2 Umwelt und Natur", "projektId": beitrittProjektID,
			"status": "zugelassen", "sichtbarkeit": "offen",
		})
		if resp.StatusCode != 201 {
			t.Fatalf("Träger anlegen: HTTP %d: %v", resp.StatusCode, traeger)
		}
		traegerID := traeger["id"].(float64)

		// Ein Vorstand mit der admin-Rolle des Arbeitskreises und jemand aus
		// dem Dorf, der noch nirgends dabei ist.
		vorstandUser := newMachine("AK2-Vorstand", nil)
		zapi(t, iamToken, "POST", "/management/v1/users/"+vorstandUser.UserID+"/grants",
			map[string]any{"projectId": beitrittProjektID, "roleKeys": []string{"admin"}})
		vorstandToken := fetchToken(t, vorstandUser, scope)
		neulingUser := newMachine("Dorf-Neuling", nil)
		neulingToken := fetchToken(t, neulingUser, scope)

		// „Ich will mitjäten.“
		resp, antrag := request(t, "POST", fmt.Sprintf("/api/v1/traeger/%.0f/beitritt", traegerID),
			neulingToken, map[string]any{"begruendung": "Ich wohne neben dem Beet"})
		if resp.StatusCode != 201 {
			t.Fatalf("Beitrittsantrag: HTTP %d: %v", resp.StatusCode, antrag)
		}
		antragID := antrag["id"].(float64)

		// Niemand entscheidet über sich selbst.
		resp, _ = request(t, "POST", fmt.Sprintf("/api/v1/beitritte/%.0f", antragID),
			neulingToken, map[string]any{"status": "erteilt"})
		if resp.StatusCode != 403 {
			t.Errorf("Selbstaufnahme: HTTP %d, erwartet 403", resp.StatusCode)
		}

		resp, out := request(t, "POST", fmt.Sprintf("/api/v1/beitritte/%.0f", antragID),
			vorstandToken, map[string]any{"status": "erteilt", "notiz": "willkommen"})
		if resp.StatusCode != 200 {
			t.Fatalf("Freigabe: HTTP %d: %v", resp.StatusCode, out)
		}

		// Der eigentliche Beweis: In Zitadel steht die Rolle.
		grants := zapi(t, iamToken, "POST", "/management/v1/users/grants/_search", map[string]any{
			"queries": []any{map[string]any{"userIdQuery": map[string]any{"userId": neulingUser.UserID}}},
		})
		eingetragen := false
		for _, roh := range grants["result"].([]any) {
			g := roh.(map[string]any)
			if g["projectId"] != beitrittProjektID {
				continue
			}
			for _, rolle := range g["roleKeys"].([]any) {
				if rolle == "mitglied" {
					eingetragen = true
				}
			}
		}
		if !eingetragen {
			t.Fatalf("die Mitgliedschaft steht nicht in Zitadel: %v", grants["result"])
		}

		// Und sie wirkt sofort — derselbe Token, keine neue Anmeldung.
		sichtbar := false
		for i := 0; i < 20; i++ {
			_, liste := request(t, "GET", "/api/v1/traeger", neulingToken, nil)
			for _, roh := range liste["traeger"].([]any) {
				tr := roh.(map[string]any)
				if tr["id"] == traegerID && tr["istMitglied"] == true {
					sichtbar = true
				}
			}
			if sichtbar {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if !sichtbar {
			t.Fatal("die frisch erteilte Mitgliedschaft wirkt nicht ohne erneute Anmeldung")
		}

		// Ein zweites Mal beitreten gibt es nicht — man ist ja schon dabei.
		resp, _ = request(t, "POST", fmt.Sprintf("/api/v1/traeger/%.0f/beitritt", traegerID),
			neulingToken, map[string]any{})
		if resp.StatusCode != 409 {
			t.Errorf("zweiter Antrag: HTTP %d, erwartet 409", resp.StatusCode)
		}

		// Und der kurze Weg: aufnehmen ohne Antrag (so nimmt eine
		// geschlossene Gruppe ihre Leute auf).
		zweiterUser := newMachine("Dorf-Nachbarin", nil)
		resp, out = request(t, "POST", fmt.Sprintf("/api/v1/traeger/%.0f/mitglieder", traegerID),
			vorstandToken, map[string]any{"userSub": zweiterUser.UserID, "notiz": "auf der Versammlung gefragt"})
		if resp.StatusCode != 200 {
			t.Fatalf("Aufnehmen ohne Antrag: HTTP %d: %v", resp.StatusCode, out)
		}
		grants = zapi(t, iamToken, "POST", "/management/v1/users/grants/_search", map[string]any{
			"queries": []any{map[string]any{"userIdQuery": map[string]any{"userId": zweiterUser.UserID}}},
		})
		if len(grants["result"].([]any)) == 0 {
			t.Fatal("die Aufnahme ohne Antrag steht nicht in Zitadel")
		}
	})
}

// befaehigungErteilen stellt einen Antrag und gibt ihn gleich frei. Der
// Träger-Admin darf beides — er verwaltet den Verein.
func befaehigungErteilen(t *testing.T, befaehigungID float64, antragsToken, adminToken string) {
	t.Helper()
	resp, antrag := request(t, "POST",
		fmt.Sprintf("/api/v1/befaehigungen/%.0f/antrag", befaehigungID), antragsToken,
		map[string]any{"begruendung": "Einweisung durchgeführt"})
	if resp.StatusCode != 201 {
		t.Fatalf("Antrag: HTTP %d: %v", resp.StatusCode, antrag)
	}
	resp, out := request(t, "POST", fmt.Sprintf("/api/v1/antraege/%.0f", antrag["id"].(float64)),
		adminToken, map[string]any{"status": "erteilt"})
	if resp.StatusCode != 200 {
		t.Fatalf("Freigabe: HTTP %d: %v", resp.StatusCode, out)
	}
}

// warteAufVorgang pollt die Orts-Liste, bis der Zeitgeber einen
// Vergabe-Vorgang für die Aufgabe eröffnet hat.
func warteAufVorgang(t *testing.T, token string, taskID float64) float64 {
	t.Helper()
	for i := 0; i < 60; i++ {
		_, liste := request(t, "GET", "/api/v1/places", token, nil)
		if a := vergabeStandVon(t, liste, taskID); a != nil {
			return a["id"].(float64)
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("es wurde kein Vergabe-Vorgang eröffnet")
	return 0
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

// machineKeyPfad liefert den absoluten Pfad des Machine-Keys aus dem
// Compose-Volume — das Backend läuft mit einem anderen Arbeitsverzeichnis.
func machineKeyPfad(t *testing.T) string {
	t.Helper()
	pfad, err := filepath.Abs("machinekey/zitadel-admin-sa.json")
	if err != nil {
		t.Fatal(err)
	}
	return pfad
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
