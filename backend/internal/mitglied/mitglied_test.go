package mitglied

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Diese Proben prüfen den Zwischenspeicher und das Verhalten bei einem
// Ausfall. Gegen ein ECHTES Zitadel läuft die Anbindung im E2E
// (backend/e2e) — hier geht es nur darum, was passiert, wenn es schweigt.

// dienstServer spielt die beiden Endpunkte, die wir ansprechen: den
// Token-Endpunkt und die Rollen-Suche. antwortet steuert, ob er gerade lebt.
type dienstServer struct {
	*httptest.Server
	lebt     atomic.Bool
	abfragen atomic.Int32
	rollen   atomic.Value // []map[string]any
}

func neuerDienstServer(t *testing.T) *dienstServer {
	t.Helper()
	ds := &dienstServer{}
	ds.lebt.Store(true)
	ds.rollen.Store([]map[string]any{})
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/v2/token", func(w http.ResponseWriter, r *http.Request) {
		if !ds.lebt.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "dienst-token", "expires_in": 3600})
	})
	mux.HandleFunc("/management/v1/users/grants/_search", func(w http.ResponseWriter, r *http.Request) {
		ds.abfragen.Add(1)
		if !ds.lebt.Load() {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": ds.rollen.Load()})
	})
	ds.Server = httptest.NewServer(mux)
	t.Cleanup(ds.Close)
	return ds
}

// schluesselDatei legt einen echten RSA-Dienstschlüssel im Zitadel-Format ab.
func schluesselDatei(t *testing.T) string {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	pfad := filepath.Join(t.TempDir(), "sa.json")
	inhalt, _ := json.Marshal(map[string]string{
		"type": "serviceaccount", "keyId": "1", "userId": "42", "key": string(pemBytes)})
	if err := os.WriteFile(pfad, inhalt, 0o600); err != nil {
		t.Fatal(err)
	}
	return pfad
}

func TestRollenKommenAusDerManagementAPI(t *testing.T) {
	ds := neuerDienstServer(t)
	ds.rollen.Store([]map[string]any{
		{"projectId": "222", "roleKeys": []string{"admin"}},
		{"projectId": "333", "roleKeys": []string{"mitglied"}},
		// Eine stillgelegte Zuweisung zählt nicht — wer draußen ist, ist draußen.
		{"projectId": "444", "roleKeys": []string{"mitglied"}, "state": "USER_GRANT_STATE_INACTIVE"},
	})
	z, err := New(Config{Issuer: ds.URL, SchluesselDatei: schluesselDatei(t)})
	if err != nil {
		t.Fatal(err)
	}
	stand := z.Fuer(context.Background(), auth.User{Sub: "erna"})
	if stand.Veraltet {
		t.Fatal("frische Auskunft fälschlich als veraltet gemeldet")
	}
	if !stand.Rollen.Hat("222", model.RolleAdmin) {
		t.Errorf("admin-Rolle fehlt: %+v", stand.Rollen)
	}
	if !stand.Rollen.IstMitglied("333") {
		t.Errorf("Mitgliedschaft fehlt: %+v", stand.Rollen)
	}
	if stand.Rollen.IstMitglied("444") {
		t.Errorf("stillgelegte Zuweisung zählt: %+v", stand.Rollen)
	}
}

// Der Zwischenspeicher hält die Last klein — aber nur kurz, damit eine neue
// Mitgliedschaft ohne Ab- und Anmelden wirkt.
func TestAuskunftWirdKurzGepuffertUndLaeuftAb(t *testing.T) {
	ds := neuerDienstServer(t)
	jetzt := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	z, err := New(Config{Issuer: ds.URL, SchluesselDatei: schluesselDatei(t),
		TTL: 30 * time.Second, Now: func() time.Time { return jetzt }})
	if err != nil {
		t.Fatal(err)
	}
	u := auth.User{Sub: "erna"}
	z.Fuer(context.Background(), u)
	z.Fuer(context.Background(), u)
	if n := ds.abfragen.Load(); n != 1 {
		t.Fatalf("%d Abfragen statt 1 — der Zwischenspeicher greift nicht", n)
	}

	// Nach Ablauf der Frist wird neu gefragt; eine frisch erteilte Rolle
	// wirkt also ohne Ab- und Anmelden.
	ds.rollen.Store([]map[string]any{{"projectId": "222", "roleKeys": []string{"mitglied"}}})
	jetzt = jetzt.Add(31 * time.Second)
	stand := z.Fuer(context.Background(), u)
	if ds.abfragen.Load() != 2 {
		t.Fatalf("nach Ablauf wurde nicht neu gefragt (%d Abfragen)", ds.abfragen.Load())
	}
	if !stand.Rollen.IstMitglied("222") {
		t.Errorf("die neue Mitgliedschaft wirkt nicht: %+v", stand.Rollen)
	}
}

// Der Kern des Ausfallverhaltens: Fällt Zitadel aus, gilt der letzte bekannte
// Stand — als „veraltet“ markiert, damit damit nicht geschrieben wird.
func TestBeiAusfallGiltDerLetzteBekannteStand(t *testing.T) {
	ds := neuerDienstServer(t)
	ds.rollen.Store([]map[string]any{{"projectId": "222", "roleKeys": []string{"admin"}}})
	jetzt := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	z, err := New(Config{Issuer: ds.URL, SchluesselDatei: schluesselDatei(t),
		TTL: 30 * time.Second, Now: func() time.Time { return jetzt }})
	if err != nil {
		t.Fatal(err)
	}
	u := auth.User{Sub: "erna"}
	if stand := z.Fuer(context.Background(), u); !stand.Rollen.Hat("222", model.RolleAdmin) {
		t.Fatalf("Vorbedingung: %+v", stand.Rollen)
	}

	// Zitadel fällt aus, die Frist läuft ab.
	ds.lebt.Store(false)
	jetzt = jetzt.Add(time.Hour)
	stand := z.Fuer(context.Background(), u)

	if !stand.Veraltet {
		t.Error("der Stand muss als veraltet gekennzeichnet sein")
	}
	if !stand.Rollen.Hat("222", model.RolleAdmin) {
		t.Errorf("der letzte bekannte Stand ging verloren: %+v", stand.Rollen)
	}

	// Und die Folge: lesen ja, schreiben nein.
	traeger := model.Traeger{ProjektID: "222", Status: model.TraegerZugelassen,
		Sichtbarkeit: model.TraegerOffen}
	zugriff := Zugriff(context.Background(), z, u)
	if !zugriff.SiehtAufgabe(traeger, model.AufgabeNurMitglieder) {
		t.Error("mit dem letzten bekannten Stand muss weiter gelesen werden können")
	}
	if zugriff.DarfVerwalten(traeger) {
		t.Error("mit veraltetem Stand darf nicht verwaltet werden")
	}
}

// Ist überhaupt nichts bekannt und Zitadel schweigt, gibt es keine
// Mitgliedschaften — man sieht dann genau das Öffentliche, nie mehr.
func TestOhneJedeAuskunftKeineMitgliedschaft(t *testing.T) {
	ds := neuerDienstServer(t)
	ds.lebt.Store(false)
	z, err := New(Config{Issuer: ds.URL, SchluesselDatei: schluesselDatei(t)})
	if err != nil {
		t.Fatal(err)
	}
	stand := z.Fuer(context.Background(), auth.User{Sub: "unbekannt"})
	if !stand.Veraltet {
		t.Error("ein Ausfall muss als solcher erkennbar sein")
	}
	if len(stand.Rollen) != 0 {
		t.Errorf("aus dem Nichts entstandene Mitgliedschaften: %+v", stand.Rollen)
	}
}

// Der Betreiber hängt an der Rolle im Token, nicht an Zitadels API: Er bleibt
// auch bei einem Ausfall handlungsfähig — sonst wäre niemand mehr da, der
// etwas richten könnte.
func TestBetreiberBleibtBeiAusfallHandlungsfaehig(t *testing.T) {
	ds := neuerDienstServer(t)
	ds.lebt.Store(false)
	z, err := New(Config{Issuer: ds.URL, SchluesselDatei: schluesselDatei(t)})
	if err != nil {
		t.Fatal(err)
	}
	betreiber := auth.User{Sub: "levin", Roles: map[string]bool{"admin": true}}
	zugriff := Zugriff(context.Background(), z, betreiber)
	traeger := model.Traeger{ProjektID: "222", Status: model.TraegerZugelassen}
	if !zugriff.DarfVerwalten(traeger) {
		t.Error("der Betreiber muss bei einem Zitadel-Ausfall weiter verwalten können")
	}
}

// Ohne eingerichteten Dienst-Nutzer (Produktion bis auf Weiteres) gibt es
// keine Träger-Rollen — aber auch keinen „Ausfall“: Das ist ein bewusster
// Betriebszustand, in dem der Betreiber alles verwaltet.
func TestOhneQuelleIstKeinAusfall(t *testing.T) {
	zugriff := Zugriff(context.Background(), nil, auth.User{Sub: "erna"})
	if zugriff.Veraltet {
		t.Error("fehlende Einrichtung ist kein Ausfall")
	}
	if len(zugriff.Mitglied) != 0 {
		t.Error("ohne Quelle darf es keine Mitgliedschaften geben")
	}
}

func TestDevQuelleLiestRollenAusDemToken(t *testing.T) {
	u := auth.User{Sub: "erna", Roles: map[string]bool{"222@admin": true, "admin": true}}
	stand := DevQuelle{}.Fuer(context.Background(), u)
	if !stand.Rollen.Hat("222", model.RolleAdmin) {
		t.Errorf("Träger-Rolle nicht erkannt: %+v", stand.Rollen)
	}
	// Die globale Betreiber-Rolle ist keine Träger-Mitgliedschaft.
	if len(stand.Rollen) != 1 {
		t.Errorf("die globale Rolle wurde zur Mitgliedschaft: %+v", stand.Rollen)
	}
}
