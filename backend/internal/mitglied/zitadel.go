package mitglied

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Zitadel fragt die Rollenzuweisungen über die Management-API ab.
//
// Angemeldet wird sich als Dienst-Nutzer (Machine User) per JWT-Profile:
// Aus dem JSON-Schlüssel wird eine signierte Assertion gebaut und beim
// Token-Endpunkt gegen ein Access-Token getauscht. Es gibt bewusst KEIN
// statisches Token in der Konfiguration — der Schlüssel liegt als Datei
// (im Cluster ein SealedSecret) und das Token wird kurzlebig selbst erzeugt.
type Zitadel struct {
	issuer  string
	schluss dienstSchluessel
	klient  *http.Client
	cache   *speicher
	jetzt   func() time.Time

	mu       sync.Mutex
	token    string
	tokenBis time.Time
}

// dienstSchluessel ist der JSON-Schlüssel eines Zitadel-Machine-Users.
type dienstSchluessel struct {
	Type   string `json:"type"`
	KeyID  string `json:"keyId"`
	Key    string `json:"key"`
	UserID string `json:"userId"`
}

// Config beschreibt die Anbindung.
type Config struct {
	// Issuer ist die Rössing-ID, z.B. https://id.xn--rssing-wxa.de.
	Issuer string
	// SchluesselDatei ist der JSON-Schlüssel des Dienst-Nutzers.
	SchluesselDatei string
	// TTL: wie lange eine Auskunft als frisch gilt (Vorgabe DefaultTTL).
	TTL time.Duration
	// Now ist die Zeitquelle (Tests stellen die Uhr).
	Now func() time.Time
}

// FromEnv baut die Anbindung aus der Umgebung:
//
//	ZITADEL_SERVICE_USER_KEY_FILE  JSON-Schlüssel des Dienst-Nutzers
//	ZITADEL_ROLLEN_TTL             Gültigkeit einer Auskunft (Vorgabe 45s)
//
// Fehlt die Schlüsseldatei, gibt es keine Quelle (nil): Träger-Rollen sind
// dann unbekannt, der Betreiber verwaltet alles. Genau das ist der Zustand,
// solange für die Produktion noch kein Zitadel-Zugang vorliegt — die
// Einrichtung ist später reine Konfiguration.
func FromEnv(issuer string) (Quelle, error) {
	datei := strings.TrimSpace(os.Getenv("ZITADEL_SERVICE_USER_KEY_FILE"))
	if datei == "" {
		return nil, nil
	}
	cfg := Config{Issuer: issuer, SchluesselDatei: datei}
	if v := strings.TrimSpace(os.Getenv("ZITADEL_ROLLEN_TTL")); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return nil, fmt.Errorf("ZITADEL_ROLLEN_TTL ist keine gültige Dauer: %q", v)
		}
		cfg.TTL = d
	}
	return New(cfg)
}

func New(cfg Config) (*Zitadel, error) {
	roh, err := os.ReadFile(cfg.SchluesselDatei)
	if err != nil {
		return nil, fmt.Errorf("Dienst-Nutzer-Schlüssel nicht lesbar: %w", err)
	}
	var k dienstSchluessel
	if err := json.Unmarshal(roh, &k); err != nil {
		return nil, fmt.Errorf("Dienst-Nutzer-Schlüssel ist kein gültiges JSON: %w", err)
	}
	if k.KeyID == "" || k.Key == "" || k.UserID == "" {
		return nil, errors.New("Dienst-Nutzer-Schlüssel unvollständig (keyId, key, userId erwartet)")
	}
	z := &Zitadel{
		issuer:  strings.TrimSuffix(cfg.Issuer, "/"),
		schluss: k,
		klient:  &http.Client{Timeout: 10 * time.Second},
		cache:   neuerSpeicher(cfg.TTL),
		jetzt:   cfg.Now,
	}
	if z.jetzt == nil {
		z.jetzt = time.Now
	}
	return z, nil
}

// Fuer liefert die Mitgliedschaften — aus dem Zwischenspeicher, solange sie
// frisch sind, sonst frisch geholt. Scheitert das Holen, gilt der letzte
// bekannte Stand als „veraltet“ (siehe Paketdokumentation).
func (z *Zitadel) Fuer(ctx context.Context, u auth.User) Stand {
	if u.Sub == "" {
		return Stand{Rollen: model.Mitgliedschaften{}}
	}
	jetzt := z.jetzt()
	if e, ok := z.cache.frisch(u.Sub, jetzt); ok {
		return Stand{Rollen: e.rollen, Geholt: e.geholt}
	}
	rollen, err := z.holen(ctx, u.Sub)
	if err == nil {
		z.cache.merken(u.Sub, rollen, jetzt)
		return Stand{Rollen: rollen, Geholt: jetzt}
	}
	// Ausfall: mit dem letzten bekannten Stand weiterarbeiten — lesend.
	slog.Warn("Mitgliedschaften konnten nicht abgefragt werden — es gilt der letzte bekannte Stand",
		"err", err)
	if e, ok := z.cache.letzter(u.Sub); ok {
		return Stand{Rollen: e.rollen, Veraltet: true, Geholt: e.geholt}
	}
	return Stand{Rollen: model.Mitgliedschaften{}, Veraltet: true}
}

// holen fragt die Rollenzuweisungen einer Person ab.
func (z *Zitadel) holen(ctx context.Context, userSub string) (model.Mitgliedschaften, error) {
	token, err := z.dienstToken(ctx)
	if err != nil {
		return nil, err
	}
	rumpf, _ := json.Marshal(map[string]any{
		// 500 ist weit jenseits dessen, was ein Dorfbewohner je an
		// Vereinen hat — die Auskunft bleibt trotzdem in einer Seite.
		"query":   map[string]any{"limit": 500},
		"queries": []any{map[string]any{"userIdQuery": map[string]any{"userId": userSub}}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		z.issuer+"/management/v1/users/grants/_search", bytes.NewReader(rumpf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := z.klient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		// Ein abgelaufenes Token beim nächsten Versuch neu holen.
		if resp.StatusCode == http.StatusUnauthorized {
			z.tokenVerwerfen()
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("Zitadel-Management-API: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		Result []struct {
			ProjectID string   `json:"projectId"`
			RoleKeys  []string `json:"roleKeys"`
			State     string   `json:"state"`
		} `json:"result"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return nil, err
	}
	rollen := model.Mitgliedschaften{}
	for _, g := range out.Result {
		// Deaktivierte Zuweisungen zählen nicht: Wer stillgelegt ist, ist
		// draußen. Zitadel liefert sie mit, statt sie wegzulassen.
		if g.ProjectID == "" || strings.EqualFold(g.State, "USER_GRANT_STATE_INACTIVE") {
			continue
		}
		for _, r := range g.RoleKeys {
			if r == "" {
				continue
			}
			if rollen[g.ProjectID] == nil {
				rollen[g.ProjectID] = map[string]bool{}
			}
			rollen[g.ProjectID][r] = true
		}
	}
	return rollen, nil
}

func (z *Zitadel) tokenVerwerfen() {
	z.mu.Lock()
	defer z.mu.Unlock()
	z.token, z.tokenBis = "", time.Time{}
}

// dienstToken liefert ein gültiges Access-Token des Dienst-Nutzers und
// erneuert es rechtzeitig selbst.
func (z *Zitadel) dienstToken(ctx context.Context) (string, error) {
	z.mu.Lock()
	// Eine Minute Vorlauf, damit ein Token nicht mitten in einer Anfrage
	// abläuft.
	if z.token != "" && z.jetzt().Add(time.Minute).Before(z.tokenBis) {
		tok := z.token
		z.mu.Unlock()
		return tok, nil
	}
	z.mu.Unlock()

	assertion, err := z.assertion()
	if err != nil {
		return "", err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
		// Die Management-API ist Teil des Projekts „zitadel“ — ohne diesen
		// Empfänger stellt Zitadel ein Token aus, das sie nicht annimmt.
		"scope": {"openid urn:zitadel:iam:org:project:id:zitadel:aud"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, z.issuer+"/oauth/v2/token",
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := z.klient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
		Fehler      string `json:"error_description"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("kein Dienst-Token (HTTP %d): %s", resp.StatusCode, out.Fehler)
	}
	gueltig := time.Duration(out.ExpiresIn) * time.Second
	if gueltig <= 0 {
		gueltig = 10 * time.Minute
	}
	z.mu.Lock()
	z.token, z.tokenBis = out.AccessToken, z.jetzt().Add(gueltig)
	z.mu.Unlock()
	return out.AccessToken, nil
}

// assertion baut die signierte JWT-Bearer-Assertion aus dem Dienst-Schlüssel.
// Bewusst von Hand statt mit einer weiteren Bibliothek: Es sind zwei
// base64url-Blöcke und eine RSA-Signatur, und das Backend hält seine
// Abhängigkeiten klein.
func (z *Zitadel) assertion() (string, error) {
	block, _ := pem.Decode([]byte(z.schluss.Key))
	if block == nil {
		return "", errors.New("Dienst-Nutzer-Schlüssel ist kein PEM")
	}
	var priv *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		priv = k
	} else if any8, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		k8, ok := any8.(*rsa.PrivateKey)
		if !ok {
			return "", errors.New("Dienst-Nutzer-Schlüssel ist kein RSA-Schlüssel")
		}
		priv = k8
	} else {
		return "", errors.New("Dienst-Nutzer-Schlüssel ist weder PKCS#1 noch PKCS#8")
	}

	jetzt := z.jetzt()
	kopf := map[string]any{"alg": "RS256", "typ": "JWT", "kid": z.schluss.KeyID}
	inhalt := map[string]any{
		"iss": z.schluss.UserID,
		"sub": z.schluss.UserID,
		"aud": z.issuer,
		"iat": jetzt.Unix(),
		"exp": jetzt.Add(time.Hour).Unix(),
	}
	teil := func(v any) (string, error) {
		raw, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(raw), nil
	}
	kopfB64, err := teil(kopf)
	if err != nil {
		return "", err
	}
	inhaltB64, err := teil(inhalt)
	if err != nil {
		return "", err
	}
	signiert := kopfB64 + "." + inhaltB64
	summe := sha256.Sum256([]byte(signiert))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, summe[:])
	if err != nil {
		return "", err
	}
	return signiert + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
