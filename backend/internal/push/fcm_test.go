package push

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Tests des Push-Versands. Gegoogelt wird hier nichts: Ein lokaler Server
// spielt sowohl die Token-Ausgabe als auch FCM. Geprüft wird dabei genau
// das, was Google auch prüfen würde — die Signatur des Dienstkonto-JWT.

// --- Testdoppel ------------------------------------------------------------

// speicher ist ein Geräteverzeichnis im Arbeitsspeicher.
type speicher struct {
	mu      sync.Mutex
	geraete map[string][]model.Device
}

func neuerSpeicher(zuordnung map[string][]string) *speicher {
	s := &speicher{geraete: map[string][]model.Device{}}
	for sub, tokens := range zuordnung {
		for _, tok := range tokens {
			s.geraete[sub] = append(s.geraete[sub], model.Device{UserSub: sub, Token: tok, Platform: "android"})
		}
	}
	return s
}

func (s *speicher) DevicesForUser(sub string) ([]model.Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.geraete[sub], nil
}

func (s *speicher) DeleteDeviceToken(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for sub, liste := range s.geraete {
		behalten := liste[:0]
		for _, g := range liste {
			if g.Token != token {
				behalten = append(behalten, g)
			}
		}
		s.geraete[sub] = behalten
	}
	return nil
}

func (s *speicher) tokens(sub string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for _, g := range s.geraete[sub] {
		out = append(out, g.Token)
	}
	return out
}

// gesendet hält eine an FCM geschickte Nachricht in zerlegter Form.
type gesendet struct {
	Authorization string
	Nachricht     map[string]any
}

// googleAttrappe spielt oauth2.googleapis.com und fcm.googleapis.com.
type googleAttrappe struct {
	*httptest.Server
	t          *testing.T
	oeffentl   *rsa.PublicKey
	clientMail string

	mu        sync.Mutex
	briefe    []gesendet
	tokenRufe int
	// antwort erlaubt es, je Gerätetoken eine Fehlerantwort zu erzwingen.
	antwort map[string]struct {
		Status int
		Body   string
	}
}

func (g *googleAttrappe) briefkasten() []gesendet {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]gesendet(nil), g.briefe...)
}

func (g *googleAttrappe) tokenAufrufe() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.tokenRufe
}

// starteGoogle baut den Attrappen-Server samt frischem Dienstkonto-Schlüssel.
func starteGoogle(t *testing.T) (*googleAttrappe, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	g := &googleAttrappe{t: t, oeffentl: &key.PublicKey,
		clientMail: "dorf@roessing-test.iam.gserviceaccount.com",
		antwort: map[string]struct {
			Status int
			Body   string
		}{}}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		g.tokenRufe++
		g.mu.Unlock()
		if err := r.ParseForm(); err != nil {
			http.Error(w, "kaputtes Formular", http.StatusBadRequest)
			return
		}
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			http.Error(w, "falscher grant_type", http.StatusBadRequest)
			return
		}
		if err := g.pruefeAssertion(r.Form.Get("assertion")); err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "ya29.test-zugriffstoken", "expires_in": 3600, "token_type": "Bearer",
		})
	})
	mux.HandleFunc("POST /v1/projects/{projekt}/messages:send", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("projekt") != "roessing-test" {
			http.Error(w, "falsches Projekt", http.StatusNotFound)
			return
		}
		var koerper struct {
			Message map[string]any `json:"message"`
		}
		if err := json.NewDecoder(r.Body).Decode(&koerper); err != nil {
			http.Error(w, "kaputtes JSON", http.StatusBadRequest)
			return
		}
		g.mu.Lock()
		g.briefe = append(g.briefe, gesendet{Authorization: r.Header.Get("Authorization"), Nachricht: koerper.Message})
		fehler, gewollt := g.antwort[fmt.Sprint(koerper.Message["token"])]
		g.mu.Unlock()
		if gewollt {
			w.WriteHeader(fehler.Status)
			_, _ = w.Write([]byte(fehler.Body))
			return
		}
		_, _ = w.Write([]byte(`{"name":"projects/roessing-test/messages/1"}`))
	})
	g.Server = httptest.NewServer(mux)
	t.Cleanup(g.Close)

	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	zugang, err := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "roessing-test",
		"private_key_id": "1",
		"private_key":    string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
		"client_email":   g.clientMail,
		"token_uri":      g.URL + "/token",
	})
	if err != nil {
		t.Fatal(err)
	}
	return g, zugang
}

// pruefeAssertion prüft das selbst gebaute Dienstkonto-JWT so, wie Google es
// prüft: Signatur, Aussteller, Empfänger, Bereich und Gültigkeit.
func (g *googleAttrappe) pruefeAssertion(roh string) error {
	teile := strings.Split(roh, ".")
	if len(teile) != 3 {
		return fmt.Errorf("JWT hat %d Teile", len(teile))
	}
	var kopf struct{ Alg, Typ string }
	rohKopf, err := base64.RawURLEncoding.DecodeString(teile[0])
	if err != nil {
		return err
	}
	if err := json.Unmarshal(rohKopf, &kopf); err != nil {
		return err
	}
	if kopf.Alg != "RS256" || kopf.Typ != "JWT" {
		return fmt.Errorf("unerwarteter Kopf %+v", kopf)
	}
	signatur, err := base64.RawURLEncoding.DecodeString(teile[2])
	if err != nil {
		return err
	}
	summe := sha256.Sum256([]byte(teile[0] + "." + teile[1]))
	if err := rsa.VerifyPKCS1v15(g.oeffentl, crypto.SHA256, summe[:], signatur); err != nil {
		return fmt.Errorf("Signatur ungültig: %w", err)
	}
	var anspruch struct {
		Iss   string `json:"iss"`
		Scope string `json:"scope"`
		Aud   string `json:"aud"`
		Exp   int64  `json:"exp"`
		Iat   int64  `json:"iat"`
	}
	rohAnspruch, err := base64.RawURLEncoding.DecodeString(teile[1])
	if err != nil {
		return err
	}
	if err := json.Unmarshal(rohAnspruch, &anspruch); err != nil {
		return err
	}
	switch {
	case anspruch.Iss != g.clientMail:
		return fmt.Errorf("falscher Aussteller %q", anspruch.Iss)
	case anspruch.Scope != "https://www.googleapis.com/auth/firebase.messaging":
		return fmt.Errorf("falscher Bereich %q", anspruch.Scope)
	case anspruch.Aud != g.URL+"/token":
		return fmt.Errorf("falscher Empfänger %q", anspruch.Aud)
	case anspruch.Exp <= anspruch.Iat || anspruch.Exp-anspruch.Iat > 3600:
		return fmt.Errorf("unplausible Laufzeit %d..%d", anspruch.Iat, anspruch.Exp)
	}
	return nil
}

// --- Hilfen ----------------------------------------------------------------

func neuerZusteller(t *testing.T, g *googleAttrappe, zugang []byte, sp *speicher) *Zusteller {
	t.Helper()
	z, err := Neu(Config{Zugangsdaten: zugang, Geraete: sp, BaseURL: g.URL})
	if err != nil {
		t.Fatal(err)
	}
	return z
}

func beispielAnfrage() model.Notification {
	frist := time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)
	return model.Notification{
		ID: 7, AssignmentID: 3, TaskID: 5, PlaceID: 2, UserSub: "anna",
		Kind: model.NotifyRequest, CreatedAt: time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC),
		ExpiresAt: &frist, PlaceName: "Unter den Eichen — Kasten 1", TaskName: "Gießen",
		TaskKind: model.TaskWatering,
		Title:    "Gießen an „Unter den Eichen — Kasten 1“ ist dran",
		Text:     "Du bist als Nächste(r) an der Reihe.",
	}
}

// --- Tests -----------------------------------------------------------------

func TestZustellenAnAlleGeraeteDerPerson(t *testing.T) {
	g, zugang := starteGoogle(t)
	sp := neuerSpeicher(map[string][]string{"anna": {"handy", "tablet"}, "bernd": {"fremd"}})
	z := neuerZusteller(t, g, zugang, sp)

	if err := z.Zustellen(beispielAnfrage()); err != nil {
		t.Fatalf("Zustellen: %v", err)
	}
	briefe := g.briefkasten()
	if len(briefe) != 2 {
		t.Fatalf("erwartet 2 Nachrichten, bekommen %d", len(briefe))
	}
	ziele := map[string]bool{}
	for _, b := range briefe {
		ziele[fmt.Sprint(b.Nachricht["token"])] = true
		if b.Authorization != "Bearer ya29.test-zugriffstoken" {
			t.Errorf("falsche Autorisierung: %q", b.Authorization)
		}
	}
	if !ziele["handy"] || !ziele["tablet"] || ziele["fremd"] {
		t.Errorf("falsche Empfänger: %v", ziele)
	}
}

func TestNutzlastFuehrtZurAufgabe(t *testing.T) {
	g, zugang := starteGoogle(t)
	sp := neuerSpeicher(map[string][]string{"anna": {"handy"}})
	z := neuerZusteller(t, g, zugang, sp)

	n := beispielAnfrage()
	if err := z.Zustellen(n); err != nil {
		t.Fatal(err)
	}
	briefe := g.briefkasten()
	if len(briefe) != 1 {
		t.Fatalf("erwartet 1 Nachricht, bekommen %d", len(briefe))
	}
	nachricht := briefe[0].Nachricht

	anzeige, _ := nachricht["notification"].(map[string]any)
	if anzeige == nil || anzeige["title"] != n.Title || anzeige["body"] != n.Text {
		t.Errorf("Anzeigeteil falsch: %v", nachricht["notification"])
	}

	daten, _ := nachricht["data"].(map[string]any)
	if daten == nil {
		t.Fatalf("Datenteil fehlt: %v", nachricht)
	}
	// Alle Werte müssen Zeichenketten sein — FCM lässt nichts anderes zu.
	for k, v := range daten {
		if _, ok := v.(string); !ok {
			t.Errorf("Datenfeld %q ist keine Zeichenkette: %T", k, v)
		}
	}
	erwartet := map[string]string{
		"notificationId": "7", "assignmentId": "3", "taskId": "5", "placeId": "2",
		"kind": "anfrage", "taskKind": "giessen",
		"placeName": n.PlaceName, "taskName": n.TaskName,
		"title": n.Title, "body": n.Text,
		"expiresAt": "2026-08-14T12:00:00Z",
	}
	for k, w := range erwartet {
		if daten[k] != w {
			t.Errorf("Datenfeld %q = %v, erwartet %q", k, daten[k], w)
		}
	}

	android, _ := nachricht["android"].(map[string]any)
	if android == nil || android["priority"] != "high" {
		t.Errorf("Android-Teil ohne hohe Dringlichkeit: %v", nachricht["android"])
	}
	androidAnzeige, _ := android["notification"].(map[string]any)
	if androidAnzeige == nil || androidAnzeige["channel_id"] != "anfragen" {
		t.Errorf("falscher Kanal für eine Anfrage: %v", android["notification"])
	}
}

func TestHinweisGehtInDenHinweiskanal(t *testing.T) {
	g, zugang := starteGoogle(t)
	sp := neuerSpeicher(map[string][]string{"anna": {"handy"}})
	z := neuerZusteller(t, g, zugang, sp)

	n := beispielAnfrage()
	n.Kind = model.NotifyAssignmentDone
	if err := z.Zustellen(n); err != nil {
		t.Fatal(err)
	}
	briefe := g.briefkasten()
	android, _ := briefe[0].Nachricht["android"].(map[string]any)
	anzeige, _ := android["notification"].(map[string]any)
	if anzeige["channel_id"] != "hinweise" {
		t.Errorf("Hinweis landete im Kanal %v", anzeige["channel_id"])
	}
}

// UNREGISTERED: die App wurde deinstalliert. Das Token ist tot und muss weg,
// sonst schleppen wir es ewig mit.
func TestUnbekanntesGeraetWirdEntfernt(t *testing.T) {
	g, zugang := starteGoogle(t)
	sp := neuerSpeicher(map[string][]string{"anna": {"altes-handy", "neues-handy"}})
	g.antwort["altes-handy"] = struct {
		Status int
		Body   string
	}{http.StatusNotFound, `{"error":{"code":404,"status":"NOT_FOUND","message":"Requested entity was not found.",
		"details":[{"@type":"type.googleapis.com/google.firebase.fcm.v1.FcmError","errorCode":"UNREGISTERED"}]}}`}
	z := neuerZusteller(t, g, zugang, sp)

	if err := z.Zustellen(beispielAnfrage()); err != nil {
		t.Fatalf("ein totes Gerät darf den Versand nicht scheitern lassen: %v", err)
	}
	if got := sp.tokens("anna"); len(got) != 1 || got[0] != "neues-handy" {
		t.Errorf("Geräte nach dem Versand: %v", got)
	}
}

func TestUngueltigesArgumentEntferntDasGeraet(t *testing.T) {
	g, zugang := starteGoogle(t)
	sp := neuerSpeicher(map[string][]string{"anna": {"kaputt"}})
	g.antwort["kaputt"] = struct {
		Status int
		Body   string
	}{http.StatusBadRequest, `{"error":{"code":400,"status":"INVALID_ARGUMENT","message":"The registration token is not a valid FCM registration token"}}`}
	z := neuerZusteller(t, g, zugang, sp)

	if err := z.Zustellen(beispielAnfrage()); err != nil {
		t.Fatalf("Zustellen: %v", err)
	}
	if got := sp.tokens("anna"); len(got) != 0 {
		t.Errorf("ungültiges Gerät blieb stehen: %v", got)
	}
}

// Eine Störung bei Google ist kein Grund, das Gerät zu vergessen.
func TestStoerungLaesstDasGeraetStehen(t *testing.T) {
	g, zugang := starteGoogle(t)
	sp := neuerSpeicher(map[string][]string{"anna": {"handy"}})
	g.antwort["handy"] = struct {
		Status int
		Body   string
	}{http.StatusServiceUnavailable, `{"error":{"code":503,"status":"UNAVAILABLE"}}`}
	z := neuerZusteller(t, g, zugang, sp)

	if err := z.Zustellen(beispielAnfrage()); err == nil {
		t.Error("eine Störung sollte gemeldet werden")
	}
	if got := sp.tokens("anna"); len(got) != 1 {
		t.Errorf("Gerät wurde fälschlich entfernt: %v", got)
	}
}

// Das Zugriffstoken gilt eine Stunde — es wird nicht für jede Nachricht neu
// geholt.
func TestZugriffstokenWirdWiederverwendet(t *testing.T) {
	g, zugang := starteGoogle(t)
	sp := neuerSpeicher(map[string][]string{"anna": {"handy"}})
	z := neuerZusteller(t, g, zugang, sp)

	for i := 0; i < 3; i++ {
		if err := z.Zustellen(beispielAnfrage()); err != nil {
			t.Fatal(err)
		}
	}
	if n := g.tokenAufrufe(); n != 1 {
		t.Errorf("Zugriffstoken %d-mal geholt, erwartet 1", n)
	}
}

// Ohne Geräte gibt es nichts zu tun — und keinen Aufruf bei Google.
func TestOhneGeraeteKeinAufruf(t *testing.T) {
	g, zugang := starteGoogle(t)
	sp := neuerSpeicher(nil)
	z := neuerZusteller(t, g, zugang, sp)

	if err := z.Zustellen(beispielAnfrage()); err != nil {
		t.Fatal(err)
	}
	if n := g.tokenAufrufe(); n != 0 {
		t.Errorf("ohne Geräte wurde %d-mal ein Token geholt", n)
	}
	if len(g.briefkasten()) != 0 {
		t.Error("ohne Geräte wurde etwas verschickt")
	}
}

func TestKaputteZugangsdatenWerdenAbgelehnt(t *testing.T) {
	if _, err := Neu(Config{Zugangsdaten: []byte(`{"type":"service_account"}`), Geraete: neuerSpeicher(nil)}); err == nil {
		t.Error("Zugangsdaten ohne Schlüssel müssen abgelehnt werden")
	}
	if _, err := Neu(Config{Zugangsdaten: []byte(`kein json`), Geraete: neuerSpeicher(nil)}); err == nil {
		t.Error("kaputte Zugangsdaten müssen abgelehnt werden")
	}
}
