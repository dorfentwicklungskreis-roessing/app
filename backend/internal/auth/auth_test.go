package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

// fakeIssuer startet einen OIDC-Provider (Discovery + JWKS) für Tests.
type fakeIssuer struct {
	srv *httptest.Server
	key *rsa.PrivateKey
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIssuer{key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                f.srv.URL,
			"jwks_uri":                              f.srv.URL + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{
			{Key: key.Public(), KeyID: "test", Algorithm: "RS256", Use: "sig"},
		}})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIssuer) token(t *testing.T, claims map[string]any) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: f.key},
		(&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test"))
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]any{
		"iss": f.srv.URL, "sub": "user-1", "aud": "app",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	}
	for k, v := range claims {
		base[k] = v
	}
	raw, err := jwt.Signed(signer).Claims(base).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestOIDCVerifier(t *testing.T) {
	f := newFakeIssuer(t)
	v, err := NewOIDCVerifier(context.Background(), f.srv.URL, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("gültiges Token mit Admin-Rolle", func(t *testing.T) {
		raw := f.token(t, map[string]any{
			"name":                              "Levin",
			"urn:zitadel:iam:org:project:roles": map[string]any{"admin": map[string]any{"org": "roessing"}},
		})
		u, err := v.Verify(context.Background(), raw)
		if err != nil {
			t.Fatal(err)
		}
		if u.Sub != "user-1" || u.Name != "Levin" || !u.IsAdmin() {
			t.Fatalf("unerwarteter Nutzer: %+v", u)
		}
	})

	t.Run("projekt-spezifischer Rollen-Claim (Machine-User-Token)", func(t *testing.T) {
		// Zitadel nutzt bei Tokens mit Projekt-Audience den Claim
		// urn:zitadel:iam:org:project:<projectid>:roles — in Produktion
		// verifiziert am 12.08.2026, Bug-Fix-Regression-Test.
		raw := f.token(t, map[string]any{
			"urn:zitadel:iam:org:project:385941791695700163:roles": map[string]any{
				"admin": map[string]any{"377268803274342501": "rössing.id.xn--rssing-wxa.de"},
			},
		})
		u, err := v.Verify(context.Background(), raw)
		if err != nil {
			t.Fatal(err)
		}
		if !u.IsAdmin() {
			t.Fatalf("admin-Rolle aus projekt-spezifischem Claim nicht erkannt: %+v", u)
		}
	})

	t.Run("ohne Rollen kein Admin", func(t *testing.T) {
		u, err := v.Verify(context.Background(), f.token(t, map[string]any{"name": "Erna"}))
		if err != nil {
			t.Fatal(err)
		}
		if u.IsAdmin() {
			t.Fatal("Erna darf kein Admin sein")
		}
	})

	t.Run("abgelaufenes Token wird abgelehnt", func(t *testing.T) {
		raw := f.token(t, map[string]any{"exp": time.Now().Add(-time.Hour).Unix()})
		if _, err := v.Verify(context.Background(), raw); err == nil {
			t.Fatal("abgelaufenes Token wurde akzeptiert")
		}
	})

	t.Run("Token von fremdem Issuer wird abgelehnt", func(t *testing.T) {
		other := newFakeIssuer(t)
		raw := other.token(t, nil)
		if _, err := v.Verify(context.Background(), raw); err == nil {
			t.Fatal("fremdes Token wurde akzeptiert")
		}
	})
}

// Wird eine Audience-Liste konfiguriert, muss sie auch geprüft werden.
// Vorher wurde der Parameter stillschweigend verworfen (SkipClientIDCheck):
// jedes Token derselben Rössing-ID — auch das einer ganz anderen Anwendung —
// wurde akzeptiert.
func TestOIDCVerifierPrueftAudience(t *testing.T) {
	f := newFakeIssuer(t)
	v, err := NewOIDCVerifier(context.Background(), f.srv.URL, []string{"dorf-app", "dorf-app-mcp"})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("passende Audience wird akzeptiert", func(t *testing.T) {
		raw := f.token(t, map[string]any{"aud": "dorf-app-mcp"})
		if _, err := v.Verify(context.Background(), raw); err != nil {
			t.Fatalf("gültige Audience abgelehnt: %v", err)
		}
	})

	t.Run("Audience-Liste mit einem Treffer genügt", func(t *testing.T) {
		raw := f.token(t, map[string]any{"aud": []string{"fremde-app", "dorf-app"}})
		if _, err := v.Verify(context.Background(), raw); err != nil {
			t.Fatalf("gültige Audience in Liste abgelehnt: %v", err)
		}
	})

	t.Run("fremde Audience wird abgelehnt", func(t *testing.T) {
		raw := f.token(t, map[string]any{"aud": "ganz-andere-app"})
		if _, err := v.Verify(context.Background(), raw); err == nil {
			t.Fatal("Token einer fremden Anwendung wurde akzeptiert")
		}
	})

	t.Run("ohne konfigurierte Audience wird nicht geprüft", func(t *testing.T) {
		offen, err := NewOIDCVerifier(context.Background(), f.srv.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := offen.Verify(context.Background(), f.token(t, map[string]any{"aud": "beliebig"})); err != nil {
			t.Fatalf("ohne Audience-Vorgabe darf nichts abgelehnt werden: %v", err)
		}
	})
}
