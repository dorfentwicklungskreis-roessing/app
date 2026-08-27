// Package push stellt Benachrichtigungen der Vergabe zusätzlich als
// Push-Nachricht auf die Handys zu — über Firebase Cloud Messaging (HTTP v1).
//
// Der Weg über die Datenbank bleibt der verlässliche: Jede Benachrichtigung
// steht dort und wird von der App abgeholt. Push ist der schnelle Weg
// obendrauf, damit jemand eine Anfrage bemerkt, ohne die App zu öffnen.
// Deshalb gilt hier durchgehend: Ein misslungener Versand ist ärgerlich,
// aber nie ein Grund, die Vergabe anzuhalten.
//
// Der Zugang zu Google läuft über ein Dienstkonto. Das dafür nötige
// Zugriffstoken erzeugen wir selbst — ein signiertes JWT gegen
// oauth2.googleapis.com eintauschen ist ein Dutzend Zeilen Standardbibliothek
// (wie in internal/auth) und spart eine schwergewichtige Fremdbibliothek samt
// ihrer Abhängigkeiten.
package push

import (
	"bytes"
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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/clock"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Geraetespeicher ist der Ausschnitt der Datenbank, den der Versand braucht.
// Die kleine Schnittstelle hält den Versand ohne SQLite prüfbar.
type Geraetespeicher interface {
	DevicesForUser(userSub string) ([]model.Device, error)
	DeleteDeviceToken(token string) error
}

// Config beschreibt den Versandweg.
type Config struct {
	// Zugangsdaten ist der Inhalt der Dienstkonto-Datei (JSON).
	Zugangsdaten []byte
	Geraete      Geraetespeicher
	// BaseURL ist der FCM-Endpunkt (leer = Google). Tests setzen hier ihren
	// eigenen Server ein.
	BaseURL string
	HTTP    *http.Client
	Now     func() time.Time
}

// FCMBase ist der öffentliche Endpunkt von Firebase Cloud Messaging.
const FCMBase = "https://fcm.googleapis.com"

// messagingScope ist der einzige Bereich, den wir anfordern — mehr Rechte
// braucht der Versand nicht.
const messagingScope = "https://www.googleapis.com/auth/firebase.messaging"

// Kanäle der App. Getrennt, weil Anfragen etwas von einem wollen und Hinweise
// nur berichten: Wer die Hinweise leise stellt, soll trotzdem gefragt werden
// können.
const (
	KanalAnfragen = "anfragen"
	KanalHinweise = "hinweise"
)

// zugang sind die Felder der Dienstkonto-Datei, die wir brauchen.
type zugang struct {
	Type        string `json:"type"`
	ProjectID   string `json:"project_id"`
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
	TokenURI    string `json:"token_uri"`
}

type Zusteller struct {
	geraete  Geraetespeicher
	projekt  string
	mail     string
	tokenURL string
	sendURL  string
	key      *rsa.PrivateKey
	http     *http.Client
	now      func() time.Time

	mu       sync.Mutex
	token    string
	tokenBis time.Time
}

// Neu baut einen Zusteller aus den Zugangsdaten eines Dienstkontos.
func Neu(cfg Config) (*Zusteller, error) {
	var z zugang
	if err := json.Unmarshal(cfg.Zugangsdaten, &z); err != nil {
		return nil, fmt.Errorf("Zugangsdaten sind kein JSON: %w", err)
	}
	if z.ProjectID == "" || z.ClientEmail == "" || z.PrivateKey == "" {
		return nil, errors.New("Zugangsdaten unvollständig (project_id, client_email, private_key)")
	}
	key, err := privaterSchluessel(z.PrivateKey)
	if err != nil {
		return nil, err
	}
	if cfg.Geraete == nil {
		return nil, errors.New("kein Geräteverzeichnis übergeben")
	}
	basis := strings.TrimSuffix(cfg.BaseURL, "/")
	if basis == "" {
		basis = FCMBase
	}
	tokenURL := z.TokenURI
	if tokenURL == "" {
		tokenURL = "https://oauth2.googleapis.com/token"
	}
	klient := cfg.HTTP
	if klient == nil {
		klient = &http.Client{Timeout: 15 * time.Second}
	}
	jetzt := cfg.Now
	if jetzt == nil {
		jetzt = clock.Now
	}
	return &Zusteller{
		geraete: cfg.Geraete, projekt: z.ProjectID, mail: z.ClientEmail,
		tokenURL: tokenURL,
		sendURL:  fmt.Sprintf("%s/v1/projects/%s/messages:send", basis, z.ProjectID),
		key:      key, http: klient, now: jetzt,
	}, nil
}

// FCMFromEnv baut den Zusteller aus FCM_CREDENTIALS_FILE. Fehlt die Angabe
// (oder die Datei), wird über Google schlicht nicht gepusht — der Betrieb muss
// ohne Google vollständig funktionieren, und lokal gibt es den Schlüssel gar
// nicht.
func FCMFromEnv(geraete Geraetespeicher) (*Zusteller, error) {
	pfad := strings.TrimSpace(os.Getenv("FCM_CREDENTIALS_FILE"))
	if pfad == "" {
		return nil, nil
	}
	roh, err := os.ReadFile(pfad)
	if err != nil {
		return nil, fmt.Errorf("FCM_CREDENTIALS_FILE %q: %w", pfad, err)
	}
	return Neu(Config{Zugangsdaten: roh, Geraete: geraete})
}

func privaterSchluessel(pemText string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("private_key ist kein PEM-Block")
	}
	roh, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Ältere Schlüssel liegen in PKCS#1 vor.
		if k, err1 := x509.ParsePKCS1PrivateKey(block.Bytes); err1 == nil {
			return k, nil
		}
		return nil, fmt.Errorf("private_key nicht lesbar: %w", err)
	}
	key, ok := roh.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private_key ist kein RSA-Schlüssel")
	}
	return key, nil
}

// --- Zustellung -------------------------------------------------------------

// Zustellen schickt die Benachrichtigung an alle Geräte der Person.
//
// Erfüllt die Schnittstelle vergabe.Zusteller. Ein Gerät, das Google als
// ungültig meldet, wird dabei gleich vergessen.
//
// iOS-Geräte lässt dieser Weg aus: Sie sprechen direkt mit Apple (apns.go).
// Ohne diese Weiche ginge eine APNs-Kennung an Google, käme als
// INVALID_ARGUMENT zurück — und würde weggeworfen.
func (z *Zusteller) Zustellen(n model.Notification) error {
	alle, err := z.geraete.DevicesForUser(n.UserSub)
	if err != nil {
		return err
	}
	var geraete []model.Device
	for _, g := range alle {
		if !IstIOS(g.Platform) {
			geraete = append(geraete, g)
		}
	}
	if len(geraete) == 0 {
		return nil
	}
	token, err := z.zugriffstoken()
	if err != nil {
		return err
	}
	nutzlast := Nutzlast(n)
	var fehler []error
	for _, g := range geraete {
		if err := z.senden(token, g.Token, nutzlast); err != nil {
			var tot *totesGeraet
			if errors.As(err, &tot) {
				slog.Info("Push: Gerät vergessen", "grund", tot.grund)
				if derr := z.geraete.DeleteDeviceToken(g.Token); derr != nil {
					fehler = append(fehler, derr)
				}
				continue
			}
			fehler = append(fehler, err)
		}
	}
	return errors.Join(fehler...)
}

// Nutzlast baut den `message`-Teil des FCM-Aufrufs (ohne das Ziel).
//
// Zwei Teile: `notification` sorgt dafür, dass Android die Meldung auch
// anzeigt, wenn die App gar nicht läuft; `data` sagt der App, wohin der
// Fingertipp führen soll — Ort, Aufgabe, Vorgang und Art der Nachricht.
// FCM lässt in `data` ausschließlich Zeichenketten zu.
func Nutzlast(n model.Notification) map[string]any {
	return map[string]any{
		"notification": map[string]any{"title": n.Title, "body": n.Text},
		"data":         daten(n),
		"android": map[string]any{
			// Anfragen sind zeitgebunden (eine Stunde Vortritt) — sie dürfen
			// den Ruhezustand des Geräts unterbrechen. Die Ruhezeit des Dorfes
			// wacht ohnehin schon darüber, dass nachts nichts rausgeht.
			"priority": "high",
			"notification": map[string]any{
				"channel_id": kanal(n.Kind),
				// Alles zu einem Vorgang ersetzt sich gegenseitig, statt sich
				// zu stapeln: Wer die Anfrage übersehen hat, braucht nicht
				// zusätzlich den Hinweis, dass sie erledigt ist. Auf iOS
				// erledigt das apns-collapse-id.
				"tag": vorgangsKennung(n.AssignmentID),
			},
		},
	}
}

// daten ist der Teil, der der App sagt, wohin der Fingertipp führt: Ort,
// Aufgabe, Vorgang und Art der Nachricht.
//
// **Beide Apps lesen denselben Vertrag** — FCM trägt ihn im `data`-Feld,
// APNs neben dem `aps`-Objekt (siehe apnsNutzlast). Wer hier einen Schlüssel
// ändert, ändert ihn für Android *und* iOS: android/…/push/PushZiel.kt und
// ios/Dorf/Push/PushZiel.swift. FCM lässt in `data` ausschließlich
// Zeichenketten zu, deshalb ist alles eine.
func daten(n model.Notification) map[string]string {
	d := map[string]string{
		"notificationId": strconv.FormatInt(n.ID, 10),
		"assignmentId":   strconv.FormatInt(n.AssignmentID, 10),
		"taskId":         strconv.FormatInt(n.TaskID, 10),
		"placeId":        strconv.FormatInt(n.PlaceID, 10),
		"kind":           string(n.Kind),
		"taskKind":       string(n.TaskKind),
		"placeName":      n.PlaceName,
		"taskName":       n.TaskName,
		"title":          n.Title,
		"body":           n.Text,
	}
	if n.ExpiresAt != nil {
		d["expiresAt"] = n.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return d
}

func kanal(k model.NotificationKind) string {
	if k.IsRequest() {
		return KanalAnfragen
	}
	return KanalHinweise
}

// totesGeraet meldet, dass diese Kennung nicht mehr gilt.
type totesGeraet struct{ grund string }

func (t *totesGeraet) Error() string { return "Gerätekennung ungültig: " + t.grund }

func (z *Zusteller) senden(zugriffstoken, geraeteToken string, nutzlast map[string]any) error {
	nachricht := map[string]any{"token": geraeteToken}
	for k, v := range nutzlast {
		nachricht[k] = v
	}
	koerper, err := json.Marshal(map[string]any{"message": nachricht})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, z.sendURL, bytes.NewReader(koerper))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+zugriffstoken)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := z.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	antwort, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	if grund, tot := istTot(resp.StatusCode, antwort); tot {
		return &totesGeraet{grund: grund}
	}
	return fmt.Errorf("FCM antwortete mit %d: %s", resp.StatusCode, strings.TrimSpace(string(antwort)))
}

// istTot erkennt die Antworten, nach denen eine Kennung nie wieder
// funktioniert. Alles andere (Störung, Zeitüberschreitung) lässt das Gerät
// stehen — sonst verlieren wir bei jedem Schluckauf von Google das halbe Dorf.
func istTot(status int, antwort []byte) (string, bool) {
	var f struct {
		Error struct {
			Status  string `json:"status"`
			Message string `json:"message"`
			Details []struct {
				ErrorCode string `json:"errorCode"`
			} `json:"details"`
		} `json:"error"`
	}
	_ = json.Unmarshal(antwort, &f)
	for _, d := range f.Error.Details {
		switch d.ErrorCode {
		case "UNREGISTERED", "INVALID_ARGUMENT", "SENDER_ID_MISMATCH":
			return d.ErrorCode, true
		}
	}
	switch {
	case status == http.StatusNotFound:
		return "UNREGISTERED", true
	case status == http.StatusBadRequest && f.Error.Status == "INVALID_ARGUMENT":
		return "INVALID_ARGUMENT", true
	case status == http.StatusForbidden && f.Error.Status == "PERMISSION_DENIED" &&
		strings.Contains(f.Error.Message, "SenderId"):
		return "SENDER_ID_MISMATCH", true
	}
	return "", false
}

// --- Zugriffstoken ----------------------------------------------------------

// zugriffstoken liefert ein gültiges OAuth-Token und holt nur dann ein neues,
// wenn das alte abläuft.
func (z *Zusteller) zugriffstoken() (string, error) {
	z.mu.Lock()
	defer z.mu.Unlock()
	// Eine Minute Sicherheitsabstand: Ein Token, das während des Versands
	// abläuft, wäre so gut wie keins.
	if z.token != "" && z.now().Add(time.Minute).Before(z.tokenBis) {
		return z.token, nil
	}
	jetzt := z.now()
	assertion, err := z.assertion(jetzt)
	if err != nil {
		return "", err
	}
	formular := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	resp, err := z.http.PostForm(z.tokenURL, formular)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	roh, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Zugriffstoken abgelehnt (%d): %s", resp.StatusCode, strings.TrimSpace(string(roh)))
	}
	var antwort struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(roh, &antwort); err != nil {
		return "", err
	}
	if antwort.AccessToken == "" {
		return "", errors.New("Antwort ohne access_token")
	}
	if antwort.ExpiresIn <= 0 {
		antwort.ExpiresIn = 3600
	}
	z.token = antwort.AccessToken
	z.tokenBis = jetzt.Add(time.Duration(antwort.ExpiresIn) * time.Second)
	return z.token, nil
}

// assertion baut das signierte Dienstkonto-JWT (RS256), das Google gegen ein
// Zugriffstoken eintauscht.
func (z *Zusteller) assertion(jetzt time.Time) (string, error) {
	kopf := map[string]string{"alg": "RS256", "typ": "JWT"}
	anspruch := map[string]any{
		"iss":   z.mail,
		"scope": messagingScope,
		"aud":   z.tokenURL,
		"iat":   jetzt.Unix(),
		"exp":   jetzt.Add(time.Hour).Unix(),
	}
	kopfTeil, err := teil(kopf)
	if err != nil {
		return "", err
	}
	anspruchTeil, err := teil(anspruch)
	if err != nil {
		return "", err
	}
	zuSignieren := kopfTeil + "." + anspruchTeil
	summe := sha256.Sum256([]byte(zuSignieren))
	signatur, err := rsa.SignPKCS1v15(rand.Reader, z.key, crypto.SHA256, summe[:])
	if err != nil {
		return "", err
	}
	return zuSignieren + "." + base64.RawURLEncoding.EncodeToString(signatur), nil
}

func teil(v any) (string, error) {
	roh, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(roh), nil
}
