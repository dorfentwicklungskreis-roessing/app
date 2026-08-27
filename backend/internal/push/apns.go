// APNs — der Weg zu den iPhones.
//
// Android bekommt seine Nachrichten über Firebase (siehe fcm.go), iOS bekommt
// sie **direkt von uns bei Apple**. Kein Firebase-iOS-SDK, aus drei Gründen:
//
//  1. Es wäre mit Abstand die größte Bibliothek, die diese App je gesehen
//     hätte — für genau eine Funktion (siehe CLAUDE.md: „keine schweren
//     Frameworks").
//  2. Das Backend baut seine Token ohnehin selbst. Googles Dienstkonto-JWT
//     steht nebenan in fcm.go; APNs verlangt dasselbe Verfahren, nur mit
//     ES256 und einem .p8-Schlüssel von Apple. Der Aufwand ist derselbe.
//  3. Der iOS-Weg kommt damit ganz ohne Google aus. Für eine Dorf-App, die
//     Datensparsamkeit ernst nimmt, ist das ein echter Gewinn — und steht so
//     in store/ios-datenschutz.md.
//
// Es gilt dieselbe Grundregel wie bei FCM: Push ist die Abkürzung, nicht der
// Weg. Jede Benachrichtigung steht in der Datenbank und wird von der App
// abgeholt; ein misslungener Versand ist ärgerlich, aber nie ein Grund, die
// Vergabe anzuhalten.
package push

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
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
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/clock"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Die Plattformen, die ein Gerät melden kann (Feld `platform` in
// POST /api/v1/me/devices). Sie entscheiden über den Versandweg: „ios" geht
// über APNs, alles andere über Firebase.
const (
	PlattformIOS     = "ios"
	PlattformAndroid = "android"
)

// Die beiden Adressen von Apple.
//
// **Sandbox ist kein Testserver, sondern eine eigene Welt**: Ein Build aus
// Xcode oder von TestFlight bekommt eine Gerätekennung, die *nur* dort gilt.
// Wer die Produktionsadresse damit anspricht, bekommt BadDeviceToken — und
// andersherum genauso. Deshalb ist APNS_UMGEBUNG eine eigene Einstellung und
// keine Ableitung aus irgendetwas anderem.
const (
	APNsProduktion = "https://api.push.apple.com"
	APNsSandbox    = "https://api.sandbox.push.apple.com"
)

// APNsTopicVorgabe ist die Bundle-ID der App — bei APNs heißt sie „Topic".
const APNsTopicVorgabe = "de.roessing.app"

// Bezeichner der beiden Umgebungen, wie sie in APNS_UMGEBUNG stehen dürfen.
const (
	UmgebungProduktion = "produktion"
	UmgebungSandbox    = "sandbox"
)

// Anbietertoken: Apple weist ein Token ab, das älter als eine Stunde ist, und
// beschwert sich (TooManyProviderTokenUpdates), wenn eines häufiger als alle
// 20 Minuten neu erzeugt wird. 45 Minuten liegen bequem dazwischen — weit
// genug von beiden Kanten, dass weder eine ungenaue Uhr noch ein langer
// Versand daran vorbeirutscht.
const (
	apnsTokenErneuerung   = 45 * time.Minute
	apnsTokenMindestalter = 20 * time.Minute
)

// APNsConfig beschreibt den Versandweg zu Apple.
type APNsConfig struct {
	// Schluessel ist der Inhalt der .p8-Datei von Apple (PEM, PKCS#8).
	Schluessel []byte
	// SchluesselID ist die Key-ID des .p8 (10 Zeichen) — landet als `kid`
	// im Kopf des Tokens.
	SchluesselID string
	// TeamID ist die Team-ID des Entwicklerkontos (10 Zeichen) — `iss`.
	TeamID string
	// Topic ist die Bundle-ID der App (leer = APNsTopicVorgabe).
	Topic string
	// Umgebung ist "produktion" oder "sandbox" (leer = produktion).
	Umgebung string
	Geraete  Geraetespeicher
	// BaseURL übersteuert die Adresse von Apple. Tests setzen hier ihren
	// eigenen Server ein; im Betrieb bleibt das Feld leer.
	BaseURL string
	HTTP    *http.Client
	Now     func() time.Time
}

// APNsZusteller schickt Benachrichtigungen an die iOS-Geräte einer Person.
//
// Erfüllt dieselbe Schnittstelle wie der FCM-Zusteller (vergabe.Zusteller);
// die Weiche zwischen beiden steht in weiche.go.
type APNsZusteller struct {
	geraete Geraetespeicher
	basis   string
	topic   string
	keyID   string
	teamID  string
	key     *ecdsa.PrivateKey
	http    *http.Client
	now     func() time.Time

	mu      sync.Mutex
	token   string
	tokenAb time.Time
}

// NeuAPNs baut einen Zusteller aus dem .p8-Schlüssel von Apple.
func NeuAPNs(cfg APNsConfig) (*APNsZusteller, error) {
	if len(cfg.Schluessel) == 0 {
		return nil, errors.New("kein APNs-Schlüssel übergeben")
	}
	if strings.TrimSpace(cfg.SchluesselID) == "" {
		return nil, errors.New("APNS_KEY_ID fehlt")
	}
	if strings.TrimSpace(cfg.TeamID) == "" {
		return nil, errors.New("APNS_TEAM_ID fehlt")
	}
	if cfg.Geraete == nil {
		return nil, errors.New("kein Geräteverzeichnis übergeben")
	}
	key, err := apnsSchluessel(cfg.Schluessel)
	if err != nil {
		return nil, err
	}
	basis := strings.TrimSuffix(strings.TrimSpace(cfg.BaseURL), "/")
	if basis == "" {
		basis, err = apnsAdresse(cfg.Umgebung)
		if err != nil {
			return nil, err
		}
	}
	topic := strings.TrimSpace(cfg.Topic)
	if topic == "" {
		topic = APNsTopicVorgabe
	}
	klient := cfg.HTTP
	if klient == nil {
		// Ohne eigenen Transport: http.DefaultTransport spricht über TLS von
		// sich aus HTTP/2, und genau das verlangt APNs. Wer hier einen
		// eigenen Transport einsetzt, muss ForceAttemptHTTP2 mitbringen.
		klient = &http.Client{Timeout: 15 * time.Second}
	}
	jetzt := cfg.Now
	if jetzt == nil {
		jetzt = clock.Now
	}
	return &APNsZusteller{
		geraete: cfg.Geraete, basis: basis, topic: topic,
		keyID: strings.TrimSpace(cfg.SchluesselID), teamID: strings.TrimSpace(cfg.TeamID),
		key: key, http: klient, now: jetzt,
	}, nil
}

// apnsAdresse übersetzt APNS_UMGEBUNG in eine Adresse. Ein Tippfehler darf
// hier nicht stillschweigend in der Produktion landen — dort käme jede
// TestFlight-Kennung als „ungültig" zurück und das Gerät würde weggeworfen.
func apnsAdresse(umgebung string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(umgebung)) {
	case "", UmgebungProduktion:
		return APNsProduktion, nil
	case UmgebungSandbox:
		return APNsSandbox, nil
	default:
		return "", fmt.Errorf("APNS_UMGEBUNG %q ist weder %q noch %q",
			umgebung, UmgebungProduktion, UmgebungSandbox)
	}
}

// APNsFromEnv baut den Zusteller aus der Umgebung.
//
// Fehlt APNS_KEY_FILE, wird für iOS schlicht nicht gepusht (Rückgabe nil,
// nil) — genau wie ohne FCM_CREDENTIALS_FILE. Der Betrieb läuft dann
// unverändert weiter: Die App holt ihre Benachrichtigungen ohnehin ab.
func APNsFromEnv(geraete Geraetespeicher) (*APNsZusteller, error) {
	pfad := strings.TrimSpace(os.Getenv("APNS_KEY_FILE"))
	if pfad == "" {
		return nil, nil
	}
	roh, err := os.ReadFile(pfad)
	if err != nil {
		return nil, fmt.Errorf("APNS_KEY_FILE %q: %w", pfad, err)
	}
	return NeuAPNs(APNsConfig{
		Schluessel:   roh,
		SchluesselID: os.Getenv("APNS_KEY_ID"),
		TeamID:       os.Getenv("APNS_TEAM_ID"),
		Topic:        os.Getenv("APNS_TOPIC"),
		Umgebung:     os.Getenv("APNS_UMGEBUNG"),
		Geraete:      geraete,
	})
}

// apnsSchluessel liest den .p8 von Apple. Er kommt als PKCS#8-PEM mit einem
// P-256-Schlüssel; alles andere kann APNs nicht brauchen.
func apnsSchluessel(roh []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(roh)
	if block == nil {
		return nil, errors.New("APNs-Schlüssel ist kein PEM-Block (erwartet wird die .p8-Datei von Apple)")
	}
	geparst, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("APNs-Schlüssel nicht lesbar: %w", err)
	}
	key, ok := geparst.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("APNs-Schlüssel ist kein ECDSA-Schlüssel")
	}
	return key, nil
}

// --- Zustellung -------------------------------------------------------------

// Zustellen schickt die Benachrichtigung an alle **iOS**-Geräte der Person.
// Android-Geräte lässt dieser Weg unangetastet; sie gehen über FCM.
func (a *APNsZusteller) Zustellen(n model.Notification) error {
	geraete, err := a.geraete.DevicesForUser(n.UserSub)
	if err != nil {
		return err
	}
	var ziele []model.Device
	for _, g := range geraete {
		if IstIOS(g.Platform) {
			ziele = append(ziele, g)
		}
	}
	if len(ziele) == 0 {
		return nil
	}
	koerper, err := json.Marshal(apnsNutzlast(n))
	if err != nil {
		return err
	}
	kopf := apnsKopfzeilen(n, a.now())
	var fehler []error
	for _, g := range ziele {
		if err := a.senden(g.Token, kopf, koerper); err != nil {
			var tot *totesGeraet
			if errors.As(err, &tot) {
				slog.Info("Push (APNs): Gerät vergessen", "grund", tot.grund)
				if derr := a.geraete.DeleteDeviceToken(g.Token); derr != nil {
					fehler = append(fehler, derr)
				}
				continue
			}
			fehler = append(fehler, err)
		}
	}
	return errors.Join(fehler...)
}

// IstIOS beantwortet die Weichenfrage. Groß- und Kleinschreibung sind egal —
// die Kennung kommt aus einer App, nicht aus einer Datenbank.
func IstIOS(plattform string) bool {
	return strings.EqualFold(strings.TrimSpace(plattform), PlattformIOS)
}

// apnsKopf sind die Kopfzeilen, die für alle Geräte einer Benachrichtigung
// gleich sind. Als eigener Wert, damit sich die Regeln ohne HTTP prüfen
// lassen.
type apnsKopf struct {
	// PushTyp: "alert" — es geht immer um etwas Sichtbares.
	PushTyp string
	// Prioritaet: 10 sofort, 5 rücksichtsvoll (darf gebündelt werden).
	Prioritaet string
	// Ablauf ist der Unix-Zeitpunkt, ab dem Apple nicht weiter zustellen
	// soll. 0 hieße „genau einmal versuchen" — das wollen wir nie.
	Ablauf int64
	// SammelID fasst alles zu einem Vorgang zusammen: Die neue Meldung
	// ersetzt die alte, statt sich daneben zu stapeln (wie der `tag` auf
	// Android).
	SammelID string
}

// apnsKopfzeilen leitet die Kopfzeilen aus der Benachrichtigung ab.
//
// Anfragen sind zeitgebunden (eine Stunde Vortritt) und gehen deshalb sofort
// raus; Hinweise berichten nur und dürfen warten. Der Ablauf folgt der Frist
// der Anfrage: Was ohnehin abgelaufen ist, muss auf keinem Handy mehr
// auftauchen. Ohne Frist gilt ein Tag — lange genug für ein Handy, das über
// Nacht aus war, kurz genug, dass nichts Vorsintflutliches ankommt.
func apnsKopfzeilen(n model.Notification, jetzt time.Time) apnsKopf {
	k := apnsKopf{
		PushTyp:    "alert",
		Prioritaet: "5",
		Ablauf:     jetzt.Add(24 * time.Hour).Unix(),
		SammelID:   vorgangsKennung(n.AssignmentID),
	}
	if n.Kind.IsRequest() {
		k.Prioritaet = "10"
	}
	if n.ExpiresAt != nil {
		ablauf := n.ExpiresAt.Unix()
		// Eine Frist, die schon vorbei ist, ergäbe „nicht zustellen".
		// Ein Versuch ist trotzdem richtig: Der Vortritt endet, die Anfrage
		// bleibt gültig.
		if ablauf > jetzt.Unix() {
			k.Ablauf = ablauf
		}
	}
	return k
}

// vorgangsKennung ist die gemeinsame Klammer aller Meldungen zu einem
// Vorgang — dieselbe Zeichenkette wie im `tag` der Android-Nutzlast.
func vorgangsKennung(assignmentID int64) string {
	return "vorgang-" + strconv.FormatInt(assignmentID, 10)
}

// apnsNutzlast baut den JSON-Rumpf für Apple.
//
// `aps` sagt dem System, was es anzeigen soll; alles daneben ist unser Teil
// und sagt der App, wohin der Fingertipp führt. Die Schlüssel sind
// **dieselben wie im `data`-Teil von FCM** (siehe daten()) — beide Apps
// lesen denselben Vertrag, und wer ihn ändert, ändert ihn für beide.
//
// `category` ist auf iOS das, was auf Android der Kanal ist: „anfragen" und
// „hinweise" (siehe UNNotificationCategory in ios/Dorf/Push/).
func apnsNutzlast(n model.Notification) map[string]any {
	aps := map[string]any{
		"alert": map[string]any{
			"title": n.Title,
			"body":  n.Text,
		},
		"sound":     "default",
		"category":  kanal(n.Kind),
		"thread-id": vorgangsKennung(n.AssignmentID),
	}
	nutzlast := map[string]any{"aps": aps}
	for schluessel, wert := range daten(n) {
		nutzlast[schluessel] = wert
	}
	return nutzlast
}

func (a *APNsZusteller) senden(geraeteToken string, kopf apnsKopf, koerper []byte) error {
	// APNs kennt Gerätekennungen nur als Hex. Kommt etwas anderes an, ist es
	// die Beschreibung eines Data-Objekts („<a1b2 c3d4>") oder eine
	// FCM-Kennung, die als „ios" gemeldet wurde — beides wird nie
	// funktionieren. Wegwerfen statt bei jeder Vergabe erneut abprallen:
	// Die App meldet sich beim nächsten Start richtig an.
	if !istHex(geraeteToken) {
		return &totesGeraet{grund: "Kennung ist keine Hex-Zeichenkette"}
	}
	token, err := a.anbietertoken()
	if err != nil {
		return err
	}
	fehler := a.einVersuch(token, geraeteToken, kopf, koerper)
	var abgelaufen *anbietertokenAbgelaufen
	if errors.As(fehler, &abgelaufen) {
		// Apple hält unser Token für abgelaufen (z.B. weil die Uhr des
		// Servers nachging). Einmal neu erzeugen und noch einmal versuchen —
		// aber nur, wenn das alte alt genug ist, sonst dreht sich das mit
		// TooManyProviderTokenUpdates im Kreis.
		neu, err := a.tokenErneuern()
		if err != nil {
			return err
		}
		if neu == token {
			return fehler
		}
		return a.einVersuch(neu, geraeteToken, kopf, koerper)
	}
	return fehler
}

func (a *APNsZusteller) einVersuch(anbietertoken, geraeteToken string, kopf apnsKopf, koerper []byte) error {
	adresse := a.basis + "/3/device/" + geraeteToken
	req, err := http.NewRequest(http.MethodPost, adresse, bytes.NewReader(koerper))
	if err != nil {
		return err
	}
	req.Header.Set("authorization", "bearer "+anbietertoken)
	req.Header.Set("apns-topic", a.topic)
	req.Header.Set("apns-push-type", kopf.PushTyp)
	req.Header.Set("apns-priority", kopf.Prioritaet)
	req.Header.Set("apns-expiration", strconv.FormatInt(kopf.Ablauf, 10))
	req.Header.Set("apns-collapse-id", kopf.SammelID)
	req.Header.Set("content-type", "application/json; charset=utf-8")

	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	antwort, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	grund := apnsGrund(antwort)
	if apnsIstTot(resp.StatusCode, grund) {
		return &totesGeraet{grund: grund}
	}
	if grund == "ExpiredProviderToken" {
		return &anbietertokenAbgelaufen{}
	}
	return fmt.Errorf("APNs antwortete mit %d (%s)", resp.StatusCode, grund)
}

// anbietertokenAbgelaufen ist der einzige Fehler, der einen zweiten Versuch
// rechtfertigt: Am Gerät liegt es nicht, an der Nachricht auch nicht.
type anbietertokenAbgelaufen struct{}

func (*anbietertokenAbgelaufen) Error() string { return "Anbietertoken abgelaufen" }

// apnsGrund liest den `reason` aus Apples Antwort. Apple schickt bei jedem
// Fehler ein winziges JSON: {"reason":"BadDeviceToken"}.
func apnsGrund(antwort []byte) string {
	var f struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(antwort, &f); err != nil || f.Reason == "" {
		gekuerzt := strings.TrimSpace(string(antwort))
		if gekuerzt == "" {
			return "ohne Begründung"
		}
		if len(gekuerzt) > 200 {
			gekuerzt = gekuerzt[:200]
		}
		return gekuerzt
	}
	return f.Reason
}

// apnsIstTot erkennt die Antworten, nach denen diese Kennung nie wieder
// funktioniert — dieselbe Regel, die istTot() für Google hat.
//
// Alles andere (Störung bei Apple, Zeitüberschreitung, falsches Topic in der
// Konfiguration) lässt das Gerät stehen: Ein Konfigurationsfehler auf unserer
// Seite darf nicht das halbe Dorf abmelden.
func apnsIstTot(status int, grund string) bool {
	switch grund {
	case "BadDeviceToken", "Unregistered", "DeviceTokenNotForTopic":
		return true
	}
	// 410 Gone heißt bei APNs immer: Die App ist von diesem Gerät weg.
	return status == http.StatusGone
}

// istHex sagt, ob die Zeichenkette eine reine Hex-Kennung ist — genau die
// Form, in der ein APNs-Token gehört (siehe ios/Dorf/Push/Geraetekennung.swift).
func istHex(s string) bool {
	if s == "" || len(s)%2 != 0 {
		return false
	}
	for _, z := range s {
		switch {
		case z >= '0' && z <= '9', z >= 'a' && z <= 'f', z >= 'A' && z <= 'F':
		default:
			return false
		}
	}
	return true
}

// --- Anbietertoken ----------------------------------------------------------

// anbietertoken liefert das JWT, mit dem wir uns bei Apple ausweisen — und
// erzeugt nur dann ein neues, wenn das alte alt genug ist.
func (a *APNsZusteller) anbietertoken() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	jetzt := a.now()
	if a.token != "" && jetzt.Sub(a.tokenAb) < apnsTokenErneuerung {
		return a.token, nil
	}
	return a.neuesToken(jetzt)
}

// tokenErneuern erzwingt ein neues Token — aber nur, wenn Apple das erlaubt
// (mindestens 20 Minuten Abstand). Sonst bleibt es beim alten, und der
// Aufrufer meldet den ursprünglichen Fehler.
func (a *APNsZusteller) tokenErneuern() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	jetzt := a.now()
	if a.token != "" && jetzt.Sub(a.tokenAb) < apnsTokenMindestalter {
		return a.token, nil
	}
	return a.neuesToken(jetzt)
}

// neuesToken erzeugt das Anbietertoken. Aufruf nur mit gehaltenem Schloss.
func (a *APNsZusteller) neuesToken(jetzt time.Time) (string, error) {
	token, err := a.jwt(jetzt)
	if err != nil {
		return "", err
	}
	a.token = token
	a.tokenAb = jetzt
	return token, nil
}

// jwt baut das Anbietertoken nach Apples Vorgabe: ES256, `kid` im Kopf, im
// Rumpf nur `iss` (Team-ID) und `iat`. Kein `exp` — Apple rechnet die
// Gültigkeit selbst aus `iat` (eine Stunde), und ein zusätzliches Feld
// verwirrt die Prüfung nur.
func (a *APNsZusteller) jwt(jetzt time.Time) (string, error) {
	kopf := map[string]string{"alg": "ES256", "kid": a.keyID, "typ": "JWT"}
	anspruch := map[string]any{"iss": a.teamID, "iat": jetzt.Unix()}
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
	r, s, err := ecdsa.Sign(rand.Reader, a.key, summe[:])
	if err != nil {
		return "", err
	}
	// JWS verlangt r und s als feste 32-Byte-Blöcke hintereinander — nicht
	// die ASN.1-Verpackung, die ecdsa.SignASN1 liefert. Genau daran scheitert
	// die Hälfte aller selbstgebauten APNs-Anbindungen.
	signatur := make([]byte, 64)
	r.FillBytes(signatur[:32])
	s.FillBytes(signatur[32:])
	return zuSignieren + "." + base64.RawURLEncoding.EncodeToString(signatur), nil
}
