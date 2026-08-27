package admin

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Scopes wie bisher: „…:projects:roles“ sorgt dafür, dass Zitadel die
// Projektrollen in den Token schreibt (Grundlage der Admin-Prüfung).
const oidcScopes = "openid profile email urn:zitadel:iam:org:projects:roles"

// discovery ist der Ausschnitt der OIDC-Discovery, den wir brauchen.
type discovery struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
}

// discoverer lädt die Discovery-Konfiguration einmalig und merkt sie sich.
// Erst beim ersten Login — so startet der Server auch, wenn die Rössing-ID
// gerade nicht erreichbar ist.
type discoverer struct {
	issuer string
	mu     sync.Mutex
	cached *discovery
}

func (d *discoverer) get(ctx context.Context) (*discovery, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.cached != nil {
		return d.cached, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimSuffix(d.issuer, "/")+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, err
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Discovery lieferte HTTP %d", res.StatusCode)
	}
	var out discovery
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.AuthorizationEndpoint == "" || out.TokenEndpoint == "" {
		return nil, fmt.Errorf("Discovery ohne authorization_endpoint/token_endpoint")
	}
	d.cached = &out
	return d.cached, nil
}

// --- Handler ----------------------------------------------------------------

// handleLogin startet den Authorization-Code-Flow mit PKCE. State und
// Code-Verifier landen signiert in einem kurzlebigen HttpOnly-Cookie.
func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	cfg, err := a.discovery.get(r.Context())
	if err != nil {
		a.setFlash(w, "error", "Die Rössing-ID ist gerade nicht erreichbar: "+err.Error())
		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
		return
	}

	verifier := randomString(48)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	f := flow{State: randomString(24), Verifier: verifier, Exp: a.now().Add(10 * time.Minute).Unix()}
	value, err := a.signer.encode(cookieFlow, f)
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setCookie(w, cookieFlow, value, 600)

	q := url.Values{
		"client_id":             {a.clientID},
		"redirect_uri":          {a.redirectURI},
		"response_type":         {"code"},
		"scope":                 {oidcScopes},
		"state":                 {f.State},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}
	http.Redirect(w, r, cfg.AuthorizationEndpoint+"?"+q.Encode(), http.StatusSeeOther)
}

// handleCallback tauscht den Code serverseitig gegen Token, prüft das JWT und
// setzt die Session. Der Callback liegt auf /admin/ selbst (siehe Register),
// damit die bereits registrierte Redirect-URI weiterverwendet werden kann.
func (a *App) handleCallback(w http.ResponseWriter, r *http.Request) {
	a.clearCookie(w, cookieFlow)
	ziel := "/admin/"

	if e := r.URL.Query().Get("error"); e != "" {
		a.setFlash(w, "error", "Anmeldung abgebrochen: "+e+" "+r.URL.Query().Get("error_description"))
		http.Redirect(w, r, ziel, http.StatusSeeOther)
		return
	}

	c, err := r.Cookie(cookieFlow)
	var f flow
	if err != nil || !a.signer.decode(cookieFlow, c.Value, &f) || f.Exp < a.now().Unix() {
		a.setFlash(w, "error", "Die Anmeldung ist abgelaufen. Bitte erneut versuchen.")
		http.Redirect(w, r, ziel, http.StatusSeeOther)
		return
	}
	if f.State == "" || f.State != r.URL.Query().Get("state") {
		a.setFlash(w, "error", "Die Anmeldung passte nicht zur Sitzung (State). Bitte erneut versuchen.")
		http.Redirect(w, r, ziel, http.StatusSeeOther)
		return
	}

	tok, err := a.exchange(r.Context(), r.URL.Query().Get("code"), f.Verifier)
	if err != nil {
		slog.Warn("admin: Token-Tausch fehlgeschlagen", "err", err)
		a.setFlash(w, "error", "Der Token-Tausch mit der Rössing-ID ist fehlgeschlagen.")
		http.Redirect(w, r, ziel, http.StatusSeeOther)
		return
	}

	user, err := a.verifier.Verify(r.Context(), tok.AccessToken)
	if err != nil {
		slog.Warn("admin: Token ungültig", "err", err)
		a.setFlash(w, "error", "Das Token der Rössing-ID konnte nicht geprüft werden.")
		http.Redirect(w, r, ziel, http.StatusSeeOther)
		return
	}
	// Herein kommt, wer etwas zu verwalten hat: der Plattform-Betreiber
	// (globale admin-Rolle) und die Verwaltenden eines Trägers. Wer nur
	// Mitglied ist, hat hier nichts zu tun — die App ist der Ort zum
	// Mitmachen.
	if !user.IsAdmin() && !a.verwaltetIrgendwas(r, user) {
		a.setFlash(w, "error",
			"Dieses Konto verwaltet weder die Dorf-App noch einen Träger.")
		http.Redirect(w, r, ziel, http.StatusSeeOther)
		return
	}

	// Zitadel legt die Profil-Claims nicht ins Access-Token (das trägt nur
	// Subject und Rollen). Den Anzeigenamen holen wir deshalb vom
	// userinfo-Endpoint; klappt das nicht, bleibt es beim Subject.
	if name, email := a.userinfo(r.Context(), tok.AccessToken); name != "" || email != "" {
		if user.Name == "" {
			user.Name = name
		}
		if user.Email == "" {
			user.Email = email
		}
	}

	s := session{
		Sub: user.Sub, Name: user.Name, Email: user.Email, Admin: user.IsAdmin(),
		Rollen:  rollenListe(user),
		IDToken: tok.IDToken,
		Exp:     a.now().Add(8 * time.Hour).Unix(),
	}
	value, err := a.signer.encode(cookieSession, s)
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setCookie(w, cookieSession, value, 8*60*60)
	a.setFlash(w, "success", "Angemeldet als "+anzeigeName(s))
	http.Redirect(w, r, ziel, http.StatusSeeOther)
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
}

// exchange führt den Code-Tausch am Token-Endpoint durch (PKCE, ohne Secret:
// die App ist als „token_endpoint_auth_method=none“ registriert).
func (a *App) exchange(ctx context.Context, code, verifier string) (*tokenResponse, error) {
	if code == "" {
		return nil, fmt.Errorf("kein Code in der Antwort")
	}
	cfg, err := a.discovery.get(ctx)
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {a.redirectURI},
		"client_id":     {a.clientID},
		"code_verifier": {verifier},
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}
	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, err
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("Antwort ohne access_token")
	}
	return &tok, nil
}

// userinfo liest Anzeigenamen und E-Mail vom userinfo-Endpoint. Fehler sind
// nicht kritisch: ohne Namen zeigt die Oberfläche das Subject.
func (a *App) userinfo(ctx context.Context, accessToken string) (name, email string) {
	cfg, err := a.discovery.get(ctx)
	if err != nil || cfg.UserinfoEndpoint == "" {
		return "", ""
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.UserinfoEndpoint, nil)
	if err != nil {
		return "", ""
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", ""
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		slog.Warn("admin: userinfo nicht abrufbar", "status", res.StatusCode)
		return "", ""
	}
	var info struct {
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
		Email             string `json:"email"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&info); err != nil {
		return "", ""
	}
	name = info.Name
	if name == "" {
		name = info.PreferredUsername
	}
	return name, info.Email
}

// handleLogout löscht die Session und beendet – wenn möglich – auch die
// Sitzung bei der Rössing-ID, damit der nächste Login wieder fragt.
func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	s, ok := a.sessionOf(r)
	a.clearCookie(w, cookieSession)

	cfg, err := a.discovery.get(r.Context())
	if err != nil || cfg.EndSessionEndpoint == "" {
		a.setFlash(w, "success", "Abgemeldet.")
		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
		return
	}
	q := url.Values{"post_logout_redirect_uri": {a.redirectURI}, "client_id": {a.clientID}}
	if ok && s.IDToken != "" {
		q.Set("id_token_hint", s.IDToken)
	}
	http.Redirect(w, r, cfg.EndSessionEndpoint+"?"+q.Encode(), http.StatusSeeOther)
}
