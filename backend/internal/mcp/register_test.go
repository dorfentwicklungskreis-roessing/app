package mcp

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// registriere schickt eine Dynamic-Client-Registration-Anfrage.
func registriere(t *testing.T, url string, koerper string) *http.Response {
	t.Helper()
	resp, err := http.Post(url+"/oauth/register", "application/json", strings.NewReader(koerper))
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// Die Allowlist der Redirect-URIs ist die einzige Stelle, an der wir die
// Client-ID herausgeben. Sie muss exakt vergleichen — jede Aufweichung
// (Präfix, Groß-/Kleinschreibung, eingebettete Zugangsdaten, offene
// Weiterleitung) würde einer fremden Seite den Login-Rückkanal öffnen.
func TestRegistrierungLehntFremdeRedirectsAb(t *testing.T) {
	ts := newTestServer(t)

	boese := []string{
		"https://claude.ai.boese.example/api/mcp/auth_callback",
		"https://boese.example/api/mcp/auth_callback",
		"https://claude.ai@boese.example/api/mcp/auth_callback",
		"https://claude.ai/api/mcp/auth_callback/../../../boese",
		"https://claude.ai/api/mcp/auth_callback?weiter=https://boese.example",
		"https://claude.ai/api/mcp/auth_callback#boese",
		"https://claude.ai/api/mcp/auth_callback/",
		"https://CLAUDE.AI/api/mcp/auth_callback",
		"http://claude.ai/api/mcp/auth_callback",
		"https://claude.ai:443/api/mcp/auth_callback",
		"//claude.ai/api/mcp/auth_callback",
		"javascript:alert(1)",
		"data:text/html,<script>alert(1)</script>",
	}
	for _, u := range boese {
		t.Run(u, func(t *testing.T) {
			roh, _ := json.Marshal(map[string]any{"redirect_uris": []string{u}, "client_name": "test"})
			resp := registriere(t, ts.URL, string(roh))
			defer resp.Body.Close()
			text, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("Status %d für %q: %s", resp.StatusCode, u, text)
			}
			if strings.Contains(string(text), "client-123") {
				t.Fatalf("Client-ID an fremde Redirect-URI herausgegeben: %s", text)
			}
		})
	}

	// Auch eine gemischte Liste (gültig + fremd) darf nicht durchgehen.
	roh, _ := json.Marshal(map[string]any{"redirect_uris": []string{
		"https://claude.ai/api/mcp/auth_callback", "https://boese.example/cb",
	}})
	resp := registriere(t, ts.URL, string(roh))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("gemischte Liste akzeptiert: %d", resp.StatusCode)
	}
}

func TestRegistrierungErlaubtClaude(t *testing.T) {
	ts := newTestServer(t)
	for _, u := range []string{
		"https://claude.ai/api/mcp/auth_callback",
		"https://claude.com/api/mcp/auth_callback",
	} {
		roh, _ := json.Marshal(map[string]any{"redirect_uris": []string{u}, "client_name": "Claude"})
		resp := registriere(t, ts.URL, string(roh))
		if resp.StatusCode != http.StatusCreated {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			t.Fatalf("%s abgelehnt: %d %s", u, resp.StatusCode, b)
		}
		var antwort map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&antwort)
		resp.Body.Close()
		if antwort["client_id"] != "client-123" {
			t.Fatalf("falsche Client-ID: %v", antwort)
		}
	}
}

// Ein riesiger Registrierungs-Körper darf den Server nicht in den Speicher
// laufen lassen. Der Endpunkt ist unauthentifiziert — hier kann jeder klopfen.
func TestRegistrierungBegrenztKoerper(t *testing.T) {
	ts := newTestServer(t)
	var b bytes.Buffer
	b.WriteString(`{"client_name":"`)
	b.WriteString(strings.Repeat("A", 4<<20)) // 4 MiB
	b.WriteString(`","redirect_uris":["https://claude.ai/api/mcp/auth_callback"]}`)

	resp := registriere(t, ts.URL, b.String())
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusCreated {
		t.Fatal("übergroßer Registrierungs-Körper wurde verarbeitet")
	}
	if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("unerwarteter Status: %d", resp.StatusCode)
	}
}

// Ohne Token gibt es 401 samt Hinweis auf die Metadata-URL (RFC 9728) —
// darauf baut der komplette OAuth-Ablauf von claude.ai auf.
func TestMCPOhneTokenLiefertHinweis(t *testing.T) {
	ts := newTestServer(t)
	resp := rpcRaw(t, ts, "", "tools/list", nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Status %d", resp.StatusCode)
	}
	wa := resp.Header.Get("WWW-Authenticate")
	if !strings.Contains(wa, "resource_metadata=") || !strings.Contains(wa, "/.well-known/oauth-protected-resource") {
		t.Fatalf("WWW-Authenticate unbrauchbar: %q", wa)
	}
}

// Ein gültig angemeldetes Mitglied ohne admin-Rolle darf nichts sehen und
// nichts ändern.
func TestMCPOhneAdminRolleVerboten(t *testing.T) {
	ts := newTestServer(t)
	for _, methode := range []string{"tools/list", "tools/call"} {
		resp := rpcRaw(t, ts, "member-jwt", methode, map[string]any{"name": "orte_liste"})
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("%s: Status %d (%s)", methode, resp.StatusCode, body)
		}
	}
}
