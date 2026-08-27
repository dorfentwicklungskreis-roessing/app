package admin

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

// Cookie-Namen. Alle Cookies sind HttpOnly und SameSite=Lax; „Secure“ wird
// gesetzt, sobald die öffentliche Basis-URL https ist (lokal/E2E läuft http).
const (
	cookieSession = "dorf_admin_session"
	cookieFlow    = "dorf_admin_flow"
	cookieFlash   = "dorf_admin_flash"
)

// session ist der Inhalt des Session-Cookies. Es enthält bewusst KEIN
// Access-Token: die Handler sprechen die Datenbank direkt an.
type session struct {
	Sub   string `json:"sub"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
	// Admin ist die globale Betreiber-Rolle der Plattform.
	Admin bool `json:"admin"`
	// Rollen sind die Projektrollen aus dem Token. Gebraucht werden sie nur
	// im Dev-Modus, in dem die Träger-Mitgliedschaften daraus gelesen werden
	// („<projektId>@<rolle>“); im Betrieb kommen sie aus der Rössing-ID und
	// dieses Feld bleibt ohne Wirkung.
	Rollen []string `json:"rollen,omitempty"`
	// IDToken dient nur als id_token_hint beim OIDC-Logout.
	IDToken string `json:"idt,omitempty"`
	Exp     int64  `json:"exp"`
}

// flow hält State und PKCE-Verifier zwischen /admin/login und dem Callback.
type flow struct {
	State    string `json:"state"`
	Verifier string `json:"verifier"`
	Exp      int64  `json:"exp"`
}

// signer signiert Cookie-Inhalte mit HMAC-SHA256.
type signer struct{ key []byte }

func newSigner(key []byte) *signer {
	if len(key) == 0 {
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			panic("admin: keine Zufallsquelle für den Session-Schlüssel: " + err.Error())
		}
	}
	return &signer{key: key}
}

// encode serialisiert v als "payload.signatur" (beides base64url). zweck ist
// der Cookie-Name: er geht in die Signatur ein, damit ein Cookie nicht in der
// Rolle eines anderen wiederverwendet werden kann.
func (s *signer) encode(zweck string, v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + s.sign(zweck, payload), nil
}

// decode prüft die Signatur (samt Zweck) und schreibt den Inhalt nach v.
func (s *signer) decode(zweck, value string, v any) bool {
	payload, sig, ok := strings.Cut(value, ".")
	if !ok {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(sig), []byte(s.sign(zweck, payload))) != 1 {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return false
	}
	return json.Unmarshal(raw, v) == nil
}

// sign bindet Zweck und Nutzlast zusammen. Der Trenner \x00 kann in keinem
// Cookie-Namen vorkommen, deshalb sind die Bereiche eindeutig getrennt.
func (s *signer) sign(zweck, payload string) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(zweck))
	m.Write([]byte{0})
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// randomString liefert einen zufälligen, URL-sicheren String (n Byte Entropie).
func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("admin: keine Zufallsquelle: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (a *App) setCookie(w http.ResponseWriter, name, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/admin",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *App) clearCookie(w http.ResponseWriter, name string) {
	a.setCookie(w, name, "", -1)
}

// sessionOf liest die Session aus dem Request; ok=false bei fehlender,
// manipulierter oder abgelaufener Session.
func (a *App) sessionOf(r *http.Request) (session, bool) {
	c, err := r.Cookie(cookieSession)
	if err != nil {
		return session{}, false
	}
	var s session
	if !a.signer.decode(cookieSession, c.Value, &s) {
		return session{}, false
	}
	if s.Exp < a.now().Unix() {
		return session{}, false
	}
	return s, true
}

// --- Flash-Nachrichten ------------------------------------------------------

// flash ist eine einmalige Meldung, die nach einem POST auf der Zielseite als
// DaisyUI-Alert erscheint (Post/Redirect/Get, deshalb über ein Cookie).
type flash struct {
	Kind string // "success" oder "error"
	Text string
}

func (a *App) setFlash(w http.ResponseWriter, kind, text string) {
	value, err := a.signer.encode(cookieFlash, map[string]string{"k": kind, "t": text})
	if err != nil {
		return
	}
	a.setCookie(w, cookieFlash, value, 60)
}

// takeFlash liest die Meldung und löscht sie sofort wieder.
func (a *App) takeFlash(w http.ResponseWriter, r *http.Request) *flash {
	c, err := r.Cookie(cookieFlash)
	if err != nil || c.Value == "" {
		return nil
	}
	a.clearCookie(w, cookieFlash)
	var m map[string]string
	if !a.signer.decode(cookieFlash, c.Value, &m) {
		return nil
	}
	return &flash{Kind: m["k"], Text: m["t"]}
}
