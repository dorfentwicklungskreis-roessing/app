package push

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Tests des iOS-Versands. **Apple wird hier nicht angefasst**: Ein lokaler
// Server spielt APNs und prüft dabei genau das, was Apple auch prüfen würde —
// die ES256-Signatur des Anbietertokens, die Kopfzeilen und die Nutzlast.
//
// Wer hier eine Adresse von Apple einträgt, macht aus einem Unit-Test einen
// Ferngespräch-Test; siehe CLAUDE.md, „Tests laufen ausschließlich lokal".

// --- Attrappe ---------------------------------------------------------------

// apnsBrief ist eine bei „Apple" eingegangene Nachricht in zerlegter Form.
type apnsBrief struct {
	Geraetetoken  string
	Authorization string
	Topic         string
	PushTyp       string
	Prioritaet    string
	Ablauf        string
	SammelID      string
	Nutzlast      map[string]any
}

// appleAttrappe spielt den APNs-Endpunkt — lokal, versteht sich. Die
// echten Adressen von Apple stehen in apns.go und in keinem Test.
type appleAttrappe struct {
	*httptest.Server
	t        *testing.T
	oeffentl *ecdsa.PublicKey

	mu     sync.Mutex
	briefe []apnsBrief
	// antwort erlaubt es, je Gerätekennung eine Fehlerantwort zu erzwingen.
	antwort map[string]struct {
		Status int
		Reason string
	}
}

func (a *appleAttrappe) briefkasten() []apnsBrief {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]apnsBrief(nil), a.briefe...)
}

func (a *appleAttrappe) antworteMit(token string, status int, reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.antwort[token] = struct {
		Status int
		Reason string
	}{status, reason}
}

// starteApple baut die Attrappe samt frischem .p8-Schlüssel.
func starteApple(t *testing.T) (*appleAttrappe, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	roh, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	p8 := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: roh})

	a := &appleAttrappe{t: t, oeffentl: &key.PublicKey, antwort: map[string]struct {
		Status int
		Reason string
	}{}}
	a.Server = httptest.NewServer(http.HandlerFunc(a.bedienen))
	t.Cleanup(a.Close)
	return a, p8
}

func (a *appleAttrappe) bedienen(w http.ResponseWriter, r *http.Request) {
	token, gefunden := strings.CutPrefix(r.URL.Path, "/3/device/")
	if !gefunden {
		http.Error(w, `{"reason":"BadPath"}`, http.StatusNotFound)
		return
	}
	// Genau die Prüfung, an der eine selbstgebaute Anbindung scheitert:
	// Stimmt die Signatur des Anbietertokens?
	autor := r.Header.Get("authorization")
	if !strings.HasPrefix(autor, "bearer ") {
		http.Error(w, `{"reason":"MissingProviderToken"}`, http.StatusUnauthorized)
		return
	}
	if err := a.pruefeToken(strings.TrimPrefix(autor, "bearer ")); err != "" {
		a.t.Errorf("Anbietertoken abgelehnt: %s", err)
		http.Error(w, `{"reason":"InvalidProviderToken"}`, http.StatusForbidden)
		return
	}

	var nutzlast map[string]any
	if err := json.NewDecoder(r.Body).Decode(&nutzlast); err != nil {
		http.Error(w, `{"reason":"BadMessageId"}`, http.StatusBadRequest)
		return
	}
	a.mu.Lock()
	a.briefe = append(a.briefe, apnsBrief{
		Geraetetoken:  token,
		Authorization: autor,
		Topic:         r.Header.Get("apns-topic"),
		PushTyp:       r.Header.Get("apns-push-type"),
		Prioritaet:    r.Header.Get("apns-priority"),
		Ablauf:        r.Header.Get("apns-expiration"),
		SammelID:      r.Header.Get("apns-collapse-id"),
		Nutzlast:      nutzlast,
	})
	fehler, gewollt := a.antwort[token]
	a.mu.Unlock()

	if gewollt {
		w.Header().Set("content-type", "application/json")
		w.WriteHeader(fehler.Status)
		_, _ = w.Write([]byte(`{"reason":"` + fehler.Reason + `"}`))
		return
	}
	// Apple antwortet im Erfolgsfall mit 200 und leerem Rumpf.
	w.WriteHeader(http.StatusOK)
}

// pruefeToken prüft das Anbietertoken so, wie Apple es prüft: ES256 über
// Kopf und Anspruch, `kid` im Kopf, `iss` und `iat` im Anspruch.
func (a *appleAttrappe) pruefeToken(token string) string {
	teile := strings.Split(token, ".")
	if len(teile) != 3 {
		return "kein JWT aus drei Teilen"
	}
	kopfRoh, err := base64.RawURLEncoding.DecodeString(teile[0])
	if err != nil {
		return "Kopf nicht lesbar"
	}
	var kopf struct{ Alg, Kid, Typ string }
	if err := json.Unmarshal(kopfRoh, &kopf); err != nil {
		return "Kopf ist kein JSON"
	}
	if kopf.Alg != "ES256" {
		return "alg ist " + kopf.Alg + ", nicht ES256"
	}
	if kopf.Kid != "ABCDE12345" {
		return "kid ist " + kopf.Kid
	}
	anspruchRoh, err := base64.RawURLEncoding.DecodeString(teile[1])
	if err != nil {
		return "Anspruch nicht lesbar"
	}
	var anspruch struct {
		Iss string `json:"iss"`
		Iat int64  `json:"iat"`
	}
	if err := json.Unmarshal(anspruchRoh, &anspruch); err != nil {
		return "Anspruch ist kein JSON"
	}
	if anspruch.Iss != "TEAM123456" {
		return "iss ist " + anspruch.Iss
	}
	if anspruch.Iat == 0 {
		return "iat fehlt"
	}
	signatur, err := base64.RawURLEncoding.DecodeString(teile[2])
	if err != nil {
		return "Signatur nicht lesbar"
	}
	if len(signatur) != 64 {
		return "Signatur ist nicht 64 Byte lang (ASN.1 statt JWS?)"
	}
	summe := sha256.Sum256([]byte(teile[0] + "." + teile[1]))
	r := new(big.Int).SetBytes(signatur[:32])
	s := new(big.Int).SetBytes(signatur[32:])
	if !ecdsa.Verify(a.oeffentl, summe[:], r, s) {
		return "Signatur stimmt nicht"
	}
	return ""
}

// --- Hilfen -----------------------------------------------------------------

// iosSpeicher baut ein Geräteverzeichnis, in dem jede Kennung als iOS-Gerät
// steht.
func iosSpeicher(zuordnung map[string][]string) *speicher {
	s := &speicher{geraete: map[string][]model.Device{}}
	for sub, tokens := range zuordnung {
		for _, tok := range tokens {
			s.geraete[sub] = append(s.geraete[sub],
				model.Device{UserSub: sub, Token: tok, Platform: PlattformIOS})
		}
	}
	return s
}

const (
	// Zwei Hex-Kennungen in der Form, die Apple ausgibt (32 Byte = 64
	// Zeichen). Kein echtes Gerät, nur die richtige Gestalt.
	kennungHandy  = "8a5f1c2d3e4b5a69788899aabbccddeeff00112233445566778899aabbccddee"
	kennungTablet = "1122334455667788990011223344556677889900112233445566778899001122"
)

func neuerAPNs(t *testing.T, a *appleAttrappe, p8 []byte, sp Geraetespeicher, uhr func() time.Time) *APNsZusteller {
	t.Helper()
	z, err := NeuAPNs(APNsConfig{
		Schluessel: p8, SchluesselID: "ABCDE12345", TeamID: "TEAM123456",
		Geraete: sp, BaseURL: a.URL, Now: uhr,
	})
	if err != nil {
		t.Fatalf("NeuAPNs: %v", err)
	}
	return z
}

// --- Tests ------------------------------------------------------------------

func TestAPNsSchicktAnAlleIPhonesDerPerson(t *testing.T) {
	a, p8 := starteApple(t)
	sp := iosSpeicher(map[string][]string{
		"anna":  {kennungHandy, kennungTablet},
		"bernd": {"aabbccdd"},
	})
	z := neuerAPNs(t, a, p8, sp, nil)

	if err := z.Zustellen(beispielAnfrage()); err != nil {
		t.Fatalf("Zustellen: %v", err)
	}
	briefe := a.briefkasten()
	if len(briefe) != 2 {
		t.Fatalf("erwartet 2 Nachrichten, bekommen %d", len(briefe))
	}
	ziele := map[string]bool{}
	for _, b := range briefe {
		ziele[b.Geraetetoken] = true
		if b.Topic != APNsTopicVorgabe {
			t.Errorf("apns-topic ist %q, erwartet %q", b.Topic, APNsTopicVorgabe)
		}
	}
	if !ziele[kennungHandy] || !ziele[kennungTablet] || ziele["aabbccdd"] {
		t.Errorf("falsche Empfänger: %v", ziele)
	}
}

func TestAPNsLaesstAndroidGeraeteInRuhe(t *testing.T) {
	// Beide Wege lesen dasselbe Verzeichnis. Ohne die Weiche bekäme Apple
	// eine Firebase-Kennung zu sehen — und würde sie als ungültig melden,
	// woraufhin das Backend ein völlig gesundes Android-Gerät wegwirft.
	a, p8 := starteApple(t)
	sp := &speicher{geraete: map[string][]model.Device{"anna": {
		{UserSub: "anna", Token: kennungHandy, Platform: PlattformIOS},
		{UserSub: "anna", Token: "cXyZ:APA91b-firebase-kennung", Platform: PlattformAndroid},
		{UserSub: "anna", Token: "ohne-angabe", Platform: ""},
	}}}
	z := neuerAPNs(t, a, p8, sp, nil)

	if err := z.Zustellen(beispielAnfrage()); err != nil {
		t.Fatalf("Zustellen: %v", err)
	}
	briefe := a.briefkasten()
	if len(briefe) != 1 || briefe[0].Geraetetoken != kennungHandy {
		t.Fatalf("nur das iPhone hätte etwas bekommen dürfen, bekommen: %+v", briefe)
	}
	if len(sp.tokens("anna")) != 3 {
		t.Errorf("kein Gerät hätte verschwinden dürfen: %v", sp.tokens("anna"))
	}
}

func TestFCMLaesstIOSGeraeteInRuhe(t *testing.T) {
	// Die Gegenprobe: Google darf die Apple-Kennung nie zu sehen bekommen.
	g, zugang := starteGoogle(t)
	sp := &speicher{geraete: map[string][]model.Device{"anna": {
		{UserSub: "anna", Token: kennungHandy, Platform: PlattformIOS},
		{UserSub: "anna", Token: "android-handy", Platform: PlattformAndroid},
	}}}
	z := neuerZusteller(t, g, zugang, sp)

	if err := z.Zustellen(beispielAnfrage()); err != nil {
		t.Fatalf("Zustellen: %v", err)
	}
	briefe := g.briefkasten()
	if len(briefe) != 1 {
		t.Fatalf("erwartet 1 Nachricht, bekommen %d", len(briefe))
	}
	if ziel := briefe[0].Nachricht["token"]; ziel != "android-handy" {
		t.Errorf("Google bekam %v zu sehen", ziel)
	}
}

func TestAPNsNutzlastFuehrtZurAufgabe(t *testing.T) {
	a, p8 := starteApple(t)
	sp := iosSpeicher(map[string][]string{"anna": {kennungHandy}})
	z := neuerAPNs(t, a, p8, sp, nil)

	n := beispielAnfrage()
	if err := z.Zustellen(n); err != nil {
		t.Fatal(err)
	}
	briefe := a.briefkasten()
	if len(briefe) != 1 {
		t.Fatalf("erwartet 1 Nachricht, bekommen %d", len(briefe))
	}
	nutz := briefe[0].Nutzlast

	aps, ok := nutz["aps"].(map[string]any)
	if !ok {
		t.Fatalf("kein aps-Objekt: %v", nutz)
	}
	alarm, ok := aps["alert"].(map[string]any)
	if !ok {
		t.Fatalf("kein alert-Objekt: %v", aps)
	}
	if alarm["title"] != n.Title || alarm["body"] != n.Text {
		t.Errorf("Titel/Text falsch: %v", alarm)
	}
	if aps["category"] != KanalAnfragen {
		t.Errorf("category ist %v, erwartet %q", aps["category"], KanalAnfragen)
	}
	if aps["thread-id"] != "vorgang-3" {
		t.Errorf("thread-id ist %v", aps["thread-id"])
	}
	// Der Datenteil ist derselbe wie bei FCM — beide Apps lesen ihn gleich.
	fuer := map[string]string{
		"notificationId": "7", "assignmentId": "3", "taskId": "5", "placeId": "2",
		"kind": "anfrage", "taskKind": "giessen",
		"placeName": n.PlaceName, "taskName": n.TaskName,
		"title": n.Title, "body": n.Text,
		"expiresAt": "2026-08-14T12:00:00Z",
	}
	for schluessel, erwartet := range fuer {
		if nutz[schluessel] != erwartet {
			t.Errorf("%s ist %v, erwartet %q", schluessel, nutz[schluessel], erwartet)
		}
	}
}

func TestAPNsKopfzeilenTrennenAnfrageUndHinweis(t *testing.T) {
	jetzt := time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC)

	anfrage := apnsKopfzeilen(beispielAnfrage(), jetzt)
	if anfrage.PushTyp != "alert" {
		t.Errorf("apns-push-type ist %q", anfrage.PushTyp)
	}
	if anfrage.Prioritaet != "10" {
		t.Errorf("eine Anfrage muss sofort raus, Priorität war %q", anfrage.Prioritaet)
	}
	// Die Frist der Anfrage ist zugleich der Ablauf: Was vorbei ist, muss auf
	// keinem Handy mehr auftauchen.
	if anfrage.Ablauf != time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC).Unix() {
		t.Errorf("apns-expiration ist %d", anfrage.Ablauf)
	}
	if anfrage.SammelID != "vorgang-3" {
		t.Errorf("apns-collapse-id ist %q", anfrage.SammelID)
	}

	hinweis := beispielAnfrage()
	hinweis.Kind = model.NotifyAssignmentDone
	hinweis.ExpiresAt = nil
	k := apnsKopfzeilen(hinweis, jetzt)
	if k.Prioritaet != "5" {
		t.Errorf("ein Hinweis darf warten, Priorität war %q", k.Prioritaet)
	}
	if k.Ablauf != jetzt.Add(24*time.Hour).Unix() {
		t.Errorf("ohne Frist gilt ein Tag, bekommen %d", k.Ablauf)
	}

	// Eine abgelaufene Frist ergäbe „gar nicht erst zustellen" — die Anfrage
	// bleibt aber gültig, nur der Vortritt ist vorbei.
	spaet := apnsKopfzeilen(beispielAnfrage(), time.Date(2026, 8, 14, 13, 0, 0, 0, time.UTC))
	if spaet.Ablauf <= time.Date(2026, 8, 14, 13, 0, 0, 0, time.UTC).Unix() {
		t.Errorf("abgelaufene Frist darf den Versand nicht abwürgen: %d", spaet.Ablauf)
	}
}

func TestAPNsHinweisGehtInDenHinweiskanal(t *testing.T) {
	a, p8 := starteApple(t)
	sp := iosSpeicher(map[string][]string{"anna": {kennungHandy}})
	z := neuerAPNs(t, a, p8, sp, nil)

	n := beispielAnfrage()
	n.Kind = model.NotifyClaimExpired
	if err := z.Zustellen(n); err != nil {
		t.Fatal(err)
	}
	briefe := a.briefkasten()
	aps := briefe[0].Nutzlast["aps"].(map[string]any)
	if aps["category"] != KanalHinweise {
		t.Errorf("category ist %v, erwartet %q", aps["category"], KanalHinweise)
	}
	if briefe[0].Prioritaet != "5" {
		t.Errorf("apns-priority ist %q", briefe[0].Prioritaet)
	}
}

func TestAPNsVergisstAbgemeldetesGeraet(t *testing.T) {
	for _, fall := range []struct {
		name   string
		status int
		reason string
	}{
		{"Unregistered", http.StatusGone, "Unregistered"},
		{"BadDeviceToken", http.StatusBadRequest, "BadDeviceToken"},
		{"DeviceTokenNotForTopic", http.StatusBadRequest, "DeviceTokenNotForTopic"},
	} {
		t.Run(fall.name, func(t *testing.T) {
			a, p8 := starteApple(t)
			sp := iosSpeicher(map[string][]string{"anna": {kennungHandy}})
			a.antworteMit(kennungHandy, fall.status, fall.reason)
			z := neuerAPNs(t, a, p8, sp, nil)

			if err := z.Zustellen(beispielAnfrage()); err != nil {
				t.Fatalf("ein totes Gerät ist kein Fehler des Versands: %v", err)
			}
			if len(sp.tokens("anna")) != 0 {
				t.Errorf("die Kennung hätte weg sein müssen: %v", sp.tokens("anna"))
			}
		})
	}
}

func TestAPNsStoerungLaesstDasGeraetStehen(t *testing.T) {
	// Bei Apple klemmt es — das ist kein Grund, das halbe Dorf abzumelden.
	a, p8 := starteApple(t)
	sp := iosSpeicher(map[string][]string{"anna": {kennungHandy}})
	a.antworteMit(kennungHandy, http.StatusServiceUnavailable, "ServiceUnavailable")
	z := neuerAPNs(t, a, p8, sp, nil)

	if err := z.Zustellen(beispielAnfrage()); err == nil {
		t.Fatal("eine Störung muss gemeldet werden")
	}
	if len(sp.tokens("anna")) != 1 {
		t.Errorf("das Gerät hätte stehen bleiben müssen: %v", sp.tokens("anna"))
	}
}

func TestAPNsFalschesTopicLaesstDasGeraetStehen(t *testing.T) {
	// BadTopic ist ein Fehler in unserer Konfiguration, nicht am Gerät.
	a, p8 := starteApple(t)
	sp := iosSpeicher(map[string][]string{"anna": {kennungHandy}})
	a.antworteMit(kennungHandy, http.StatusBadRequest, "BadTopic")
	z := neuerAPNs(t, a, p8, sp, nil)

	if err := z.Zustellen(beispielAnfrage()); err == nil {
		t.Fatal("ein falsches Topic muss auffallen")
	}
	if len(sp.tokens("anna")) != 1 {
		t.Errorf("das Gerät hätte stehen bleiben müssen: %v", sp.tokens("anna"))
	}
}

func TestAPNsWirftKennungWegDieKeinHexIst(t *testing.T) {
	// Der klassische iOS-Fehler: `Data.description` ergibt "<8a5f1c2d …>".
	// So eine Kennung kann bei Apple nie funktionieren — sie muss weg, sonst
	// prallt jede Vergabe erneut daran ab.
	a, p8 := starteApple(t)
	sp := iosSpeicher(map[string][]string{"anna": {"<8a5f1c2d 3e4b5a69>"}})
	z := neuerAPNs(t, a, p8, sp, nil)

	if err := z.Zustellen(beispielAnfrage()); err != nil {
		t.Fatalf("Zustellen: %v", err)
	}
	if len(a.briefkasten()) != 0 {
		t.Error("dafür hätte Apple gar nicht erst gefragt werden dürfen")
	}
	if len(sp.tokens("anna")) != 0 {
		t.Errorf("die unbrauchbare Kennung hätte weg sein müssen: %v", sp.tokens("anna"))
	}
}

func TestIstHex(t *testing.T) {
	for _, gut := range []string{"deadbeef", "DEADBEEF", kennungHandy, "00"} {
		if !istHex(gut) {
			t.Errorf("%q ist eine Hex-Kennung", gut)
		}
	}
	for _, schlecht := range []string{"", "abc", "<dead beef>", "dead beef", "cXyZ:APA91b", "dead-beef"} {
		if istHex(schlecht) {
			t.Errorf("%q ist keine Hex-Kennung", schlecht)
		}
	}
}

func TestAnbietertokenWirdWiederverwendet(t *testing.T) {
	// Apple weist ein Token ab, das älter als eine Stunde ist, und beschwert
	// sich, wenn öfter als alle 20 Minuten ein neues kommt. Also: innerhalb
	// von 45 Minuten dasselbe, danach ein neues.
	a, p8 := starteApple(t)
	sp := iosSpeicher(map[string][]string{"anna": {kennungHandy}})
	uhr := time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC)
	z := neuerAPNs(t, a, p8, sp, func() time.Time { return uhr })

	if err := z.Zustellen(beispielAnfrage()); err != nil {
		t.Fatal(err)
	}
	uhr = uhr.Add(30 * time.Minute)
	if err := z.Zustellen(beispielAnfrage()); err != nil {
		t.Fatal(err)
	}
	uhr = uhr.Add(30 * time.Minute)
	if err := z.Zustellen(beispielAnfrage()); err != nil {
		t.Fatal(err)
	}

	briefe := a.briefkasten()
	if len(briefe) != 3 {
		t.Fatalf("erwartet 3 Nachrichten, bekommen %d", len(briefe))
	}
	if briefe[0].Authorization != briefe[1].Authorization {
		t.Error("nach 30 Minuten hätte dasselbe Token genügt")
	}
	if briefe[1].Authorization == briefe[2].Authorization {
		t.Error("nach 60 Minuten hätte ein neues Token her müssen")
	}
}

func TestAbgelaufenesAnbietertokenWirdEinmalErneuert(t *testing.T) {
	a, p8 := starteApple(t)
	sp := iosSpeicher(map[string][]string{"anna": {kennungHandy}})
	uhr := time.Date(2026, 8, 14, 11, 0, 0, 0, time.UTC)
	z := neuerAPNs(t, a, p8, sp, func() time.Time { return uhr })

	if err := z.Zustellen(beispielAnfrage()); err != nil {
		t.Fatal(err)
	}
	// Eine halbe Stunde später hält Apple das Token für abgelaufen (etwa
	// weil unsere Uhr nachging). Ein zweiter Versuch mit frischem Token muss
	// gelingen.
	uhr = uhr.Add(25 * time.Minute)
	a.antworteMit(kennungHandy, http.StatusForbidden, "ExpiredProviderToken")
	geplatzt := len(a.briefkasten())
	if err := z.Zustellen(beispielAnfrage()); err == nil {
		t.Fatal("solange Apple ablehnt, bleibt es ein Fehler")
	}
	briefe := a.briefkasten()[geplatzt:]
	if len(briefe) != 2 {
		t.Fatalf("erwartet zwei Versuche, bekommen %d", len(briefe))
	}
	if briefe[0].Authorization == briefe[1].Authorization {
		t.Error("der zweite Versuch hätte ein frisches Token gebraucht")
	}
	if len(sp.tokens("anna")) != 1 {
		t.Error("an unserem Token liegt es, nicht am Gerät — es muss stehen bleiben")
	}
}

func TestAPNsUmgebungEntscheidetUeberDieAdresse(t *testing.T) {
	// TestFlight-Builds bekommen Sandbox-Kennungen. Wer sie an die
	// Produktionsadresse schickt, bekommt BadDeviceToken — und das Backend
	// würde ein gesundes Gerät wegwerfen.
	faelle := map[string]string{
		"":           APNsProduktion,
		"produktion": APNsProduktion,
		"Produktion": APNsProduktion,
		"sandbox":    APNsSandbox,
		"SANDBOX":    APNsSandbox,
	}
	for eingabe, erwartet := range faelle {
		bekommen, err := apnsAdresse(eingabe)
		if err != nil {
			t.Errorf("%q: %v", eingabe, err)
			continue
		}
		if bekommen != erwartet {
			t.Errorf("%q ergab %q, erwartet %q", eingabe, bekommen, erwartet)
		}
	}
	if _, err := apnsAdresse("testflight"); err == nil {
		t.Error("ein Tippfehler darf nicht stillschweigend in der Produktion landen")
	}
}

func TestKaputterAPNsSchluesselWirdAbgelehnt(t *testing.T) {
	sp := iosSpeicher(map[string][]string{})
	faelle := map[string]APNsConfig{
		"kein PEM":     {Schluessel: []byte("das ist keine .p8-Datei"), SchluesselID: "A", TeamID: "B", Geraete: sp},
		"ohne Key-ID":  {Schluessel: gueltigerP8(t), TeamID: "B", Geraete: sp},
		"ohne Team-ID": {Schluessel: gueltigerP8(t), SchluesselID: "A", Geraete: sp},
		"ohne Geräte":  {Schluessel: gueltigerP8(t), SchluesselID: "A", TeamID: "B"},
		"leer":         {SchluesselID: "A", TeamID: "B", Geraete: sp},
	}
	for name, cfg := range faelle {
		if _, err := NeuAPNs(cfg); err == nil {
			t.Errorf("%s: hätte abgelehnt werden müssen", name)
		}
	}
}

func gueltigerP8(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	roh, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: roh})
}

// --- Weiche -----------------------------------------------------------------

// wegAttrappe merkt sich, was ihr zugestellt wurde.
type wegAttrappe struct {
	empfangen []model.Notification
	fehler    error
}

func (w *wegAttrappe) Zustellen(n model.Notification) error {
	w.empfangen = append(w.empfangen, n)
	return w.fehler
}

func TestWeicheOhneWegIstNil(t *testing.T) {
	// Ehrliche Antwort statt eines Zustellers, der nichts tut: Der Aufrufer
	// soll „kein Push" im Log sehen.
	if w := NeueWeiche(nil, nil); w != nil {
		t.Fatal("ohne eingerichteten Weg darf es keine Weiche geben")
	}
}

func TestWeicheBedientBeideWege(t *testing.T) {
	links, rechts := &wegAttrappe{}, &wegAttrappe{}
	w := &Weiche{wege: []Weg{links, rechts}}
	if err := w.Zustellen(beispielAnfrage()); err != nil {
		t.Fatal(err)
	}
	if len(links.empfangen) != 1 || len(rechts.empfangen) != 1 {
		t.Errorf("beide Wege hätten die Nachricht bekommen müssen: %d/%d",
			len(links.empfangen), len(rechts.empfangen))
	}
}

func TestWeicheHaeltNichtBeimErstenFehlerAn(t *testing.T) {
	// Ein Ausfall bei Google darf den Versand an die iPhones nicht aufhalten.
	kaputt := &wegAttrappe{fehler: errBeispiel}
	heil := &wegAttrappe{}
	w := &Weiche{wege: []Weg{kaputt, heil}}
	if err := w.Zustellen(beispielAnfrage()); err == nil {
		t.Fatal("der Fehler muss gemeldet werden")
	}
	if len(heil.empfangen) != 1 {
		t.Error("der zweite Weg wurde übersprungen")
	}
}

var errBeispiel = &totesGeraet{grund: "Beispiel"}
