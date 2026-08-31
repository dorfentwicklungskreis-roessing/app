package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/mitglied"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Beitritt: Jemand sieht einen offenen Träger, sagt „ich will mitmachen“, der
// Träger-Admin gibt frei — und danach ist die Person wirklich Mitglied, ohne
// dass jemand die Zitadel-Konsole öffnet.
//
// Das „wirklich“ ist der Kern dieser Proben: Eine Freigabe, die nur in der
// eigenen Datenbank landet, wäre eine Lüge.

// roessingID spielt die Rössing-ID: lesen wie die Dev-Quelle (Rollen aus dem
// Token), schreiben wie der Dienst-Nutzer im Betrieb.
type roessingID struct {
	mu sync.Mutex
	// aufgenommen: Kennung → Projekt → Rolle.
	aufgenommen map[string]map[string]string
	// fehler lässt die Rössing-ID die Aufnahme verweigern.
	fehler error
}

func neueRoessingID() *roessingID {
	return &roessingID{aufgenommen: map[string]map[string]string{}}
}

func (q *roessingID) Fuer(ctx context.Context, u auth.User) mitglied.Stand {
	stand := mitglied.DevQuelle{}.Fuer(ctx, u)
	q.mu.Lock()
	defer q.mu.Unlock()
	for projekt, rolle := range q.aufgenommen[u.Sub] {
		if stand.Rollen[projekt] == nil {
			stand.Rollen[projekt] = map[string]bool{}
		}
		stand.Rollen[projekt][rolle] = true
	}
	return stand
}

func (q *roessingID) Aufnehmen(_ context.Context, projektID, userSub, rolle string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.fehler != nil {
		return q.fehler
	}
	if q.aufgenommen[userSub] == nil {
		q.aufgenommen[userSub] = map[string]string{}
	}
	q.aufgenommen[userSub][projektID] = rolle
	return nil
}

func (q *roessingID) istMitglied(sub, projektID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.aufgenommen[sub][projektID] == model.RolleMitglied
}

// beitrittsServer ist der Testserver mit einer Rössing-ID, in die
// zurückgeschrieben werden kann.
func beitrittsServer(t *testing.T) (string, *roessingID) {
	t.Helper()
	ts, srv := newTestServer(t)
	q := neueRoessingID()
	srv.Mitglieder = q
	return ts.URL, q
}

func beitrittStellen(t *testing.T, ts, token string, traegerID int64, grund string) *http.Response {
	t.Helper()
	return doReq(t, "POST", fmt.Sprintf("%s/api/v1/traeger/%d/beitritt", ts, traegerID),
		token, map[string]any{"begruendung": grund})
}

// Der Durchstich: beantragen, freigeben, dabei sein.
func TestBeitrittBeantragenUndFreigeben(t *testing.T) {
	ts, roessing := beitrittsServer(t)
	traegerID := traegerAnlegen(t, ts, "AK 2 Umwelt und Natur", "222")

	resp := beitrittStellen(t, ts, aussenToken, traegerID, "Ich wohne neben dem Beet")
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Antrag: HTTP %d", resp.StatusCode)
	}
	antragID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID

	// Der Träger-Admin sieht ihn — mit Namen, nicht mit nackter Kennung.
	resp = doReq(t, "GET", fmt.Sprintf("%s/api/v1/traeger/%d/beitritte?status=beantragt", ts, traegerID),
		dorfpflegeAdmin, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Liste: HTTP %d", resp.StatusCode)
	}
	liste := decode[struct {
		Beitritte []model.Beitritt `json:"beitritte"`
	}](t, resp).Beitritte
	if len(liste) != 1 || liste[0].UserName == "" {
		t.Fatalf("offener Antrag fehlt oder ist namenlos: %+v", liste)
	}

	// Niemand entscheidet über sich selbst.
	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/beitritte/%d", ts, antragID), aussenToken,
		map[string]any{"status": "erteilt"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Selbstaufnahme: HTTP %d, erwartet 403", resp.StatusCode)
	}

	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/beitritte/%d", ts, antragID), dorfpflegeAdmin,
		map[string]any{"status": "erteilt", "notiz": "willkommen"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Freigabe: HTTP %d", resp.StatusCode)
	}
	if got := decode[model.Beitritt](t, resp); got.Status != model.AntragErteilt {
		t.Fatalf("Antrag nicht erteilt: %+v", got)
	}

	// Das eigentliche Versprechen: Die Mitgliedschaft steht in der
	// Rössing-ID, nicht bloß in unserer Datenbank.
	if !roessing.istMitglied("fremd", "222") {
		t.Fatal("die Mitgliedschaft wurde nicht in die Rössing-ID zurückgeschrieben")
	}

	// Und sie wirkt sofort: derselbe Token, keine neue Anmeldung.
	resp = doReq(t, "GET", ts+"/api/v1/traeger", aussenToken, nil)
	sicht := decode[struct {
		Traeger []TraegerAnsicht `json:"traeger"`
	}](t, resp).Traeger
	if len(sicht) != 1 || !sicht[0].IstMitglied {
		t.Fatalf("die Aufnahme wirkt nicht: %+v", sicht)
	}
	if sicht[0].BeitrittMoeglich {
		t.Error("wer dabei ist, bekommt weiter „mitmachen“ angeboten")
	}
}

// Ohne schreibenden Dienst-Nutzer bewirkt eine Freigabe nichts. Dann muss sie
// scheitern und sagen, was fehlt — der Antrag bleibt offen. Eine Verwaltung,
// die „aufgenommen“ meldet, während die Tür zu bleibt, wäre schlimmer als
// gar keine.
func TestOhneSchreibendenDienstNutzerScheitertDieFreigabe(t *testing.T) {
	ts, _ := newTestServer(t) // Dev-Quelle: liest aus dem Token, schreibt nicht
	traegerID := traegerAnlegen(t, ts.URL, "AK 2 Umwelt und Natur", "222")
	resp := beitrittStellen(t, ts.URL, aussenToken, traegerID, "bitte")
	antragID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID

	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/beitritte/%d", ts.URL, antragID),
		dorfpflegeAdmin, map[string]any{"status": "erteilt"})
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("HTTP %d, erwartet 503", resp.StatusCode)
	}
	if text, _ := decode[map[string]any](t, resp)["error"].(string); !strings.Contains(text, "Dienst-Nutzer") {
		t.Errorf("die Absage sagt nicht, was einzurichten ist: %q", text)
	}

	// Der Antrag bleibt offen — nichts ist passiert, und das steht auch so da.
	resp = doReq(t, "GET", fmt.Sprintf("%s/api/v1/traeger/%d/beitritte?status=beantragt", ts.URL, traegerID),
		dorfpflegeAdmin, nil)
	if liste := decode[struct {
		Beitritte []model.Beitritt `json:"beitritte"`
	}](t, resp).Beitritte; len(liste) != 1 {
		t.Fatalf("der Antrag wurde trotz gescheiterter Aufnahme abgehakt: %+v", liste)
	}
}

// Antwortet die Rössing-ID nicht, gilt dasselbe: kein Vermerk, kein
// halbfertiger Zustand.
func TestScheiterndeAufnahmeLaesstDenAntragOffen(t *testing.T) {
	ts, roessing := beitrittsServer(t)
	roessing.fehler = fmt.Errorf("Rössing-ID antwortet nicht")
	traegerID := traegerAnlegen(t, ts, "AK 2 Umwelt und Natur", "222")
	resp := beitrittStellen(t, ts, aussenToken, traegerID, "bitte")
	antragID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID

	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/beitritte/%d", ts, antragID),
		dorfpflegeAdmin, map[string]any{"status": "erteilt"})
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("HTTP %d, erwartet 502", resp.StatusCode)
	}
	resp = doReq(t, "GET", fmt.Sprintf("%s/api/v1/traeger/%d/beitritte?status=beantragt", ts, traegerID),
		dorfpflegeAdmin, nil)
	if liste := decode[struct {
		Beitritte []model.Beitritt `json:"beitritte"`
	}](t, resp).Beitritte; len(liste) != 1 {
		t.Fatalf("der Antrag wurde abgehakt, obwohl nichts eingetragen wurde: %+v", liste)
	}
}

// Ablehnen geht immer: Dabei ist in der Rössing-ID nichts zu schreiben.
func TestAblehnenBrauchtKeineRoessingID(t *testing.T) {
	ts, _ := newTestServer(t)
	traegerID := traegerAnlegen(t, ts.URL, "AK 2 Umwelt und Natur", "222")
	resp := beitrittStellen(t, ts.URL, aussenToken, traegerID, "bitte")
	antragID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID

	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/beitritte/%d", ts.URL, antragID),
		dorfpflegeAdmin, map[string]any{"status": "abgelehnt", "notiz": "gerade voll"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Ablehnen: HTTP %d", resp.StatusCode)
	}
	if got := decode[model.Beitritt](t, resp); got.Status != model.AntragAbgelehnt {
		t.Fatalf("nicht abgelehnt: %+v", got)
	}
}

// Eine geschlossene Gruppe steht nicht im Verzeichnis: Für Außenstehende gibt
// es sie nicht, und selbst wer sie sieht, kann ihr nichts schicken. Sie nimmt
// selbst auf.
func TestGeschlosseneGruppeNimmtKeineAntraegeEntgegen(t *testing.T) {
	ts, roessing := beitrittsServer(t)
	traegerID := traegerAnlegen(t, ts, "Stiller Kreis", "222")
	resp := doReq(t, "PUT", fmt.Sprintf("%s/api/v1/traeger/%d", ts, traegerID), betreiberToken,
		map[string]any{"name": "Stiller Kreis", "projektId": "222",
			"status": "zugelassen", "sichtbarkeit": "geschlossen"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("schließen: HTTP %d", resp.StatusCode)
	}

	// Für Außenstehende gibt es sie gar nicht.
	if resp := beitrittStellen(t, ts, aussenToken, traegerID, "bitte"); resp.StatusCode != http.StatusNotFound {
		t.Errorf("Antrag von außen: HTTP %d, erwartet 404", resp.StatusCode)
	}
	// Der Betreiber sieht sie — und bekommt trotzdem kein Antragsrecht.
	resp = beitrittStellen(t, ts, betreiberToken, traegerID, "bitte")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("Antrag an eine geschlossene Gruppe: HTTP %d, erwartet 409", resp.StatusCode)
	}
	if text, _ := decode[map[string]any](t, resp)["error"].(string); !strings.Contains(text, "geschlossen") {
		t.Errorf("die Absage nennt den Grund nicht: %q", text)
	}

	// Aufnehmen kann die Gruppe trotzdem — das ist ihr Weg.
	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/traeger/%d/mitglieder", ts, traegerID),
		dorfpflegeAdmin, map[string]any{"userSub": "erna", "notiz": "auf der Versammlung gefragt"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Aufnehmen: HTTP %d", resp.StatusCode)
	}
	if !roessing.istMitglied("erna", "222") {
		t.Fatal("die Aufnahme steht nicht in der Rössing-ID")
	}
	if got := decode[model.Beitritt](t, resp); got.Status != model.AntragErteilt ||
		got.Notiz != "auf der Versammlung gefragt" {
		t.Fatalf("der Vorgang wurde nicht festgehalten: %+v", got)
	}
}

// Wer schon dabei ist, fragt nicht noch einmal.
func TestWerDabeiIstBeantragtNichtNochmal(t *testing.T) {
	ts, _ := beitrittsServer(t)
	traegerID := traegerAnlegen(t, ts, "Dorfpflege", "222")
	resp := beitrittStellen(t, ts, dorfpflegeMitglied, traegerID, "bitte")
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("HTTP %d, erwartet 409", resp.StatusCode)
	}
}

// Ein Träger ohne Zitadel-Projekt hat keine Mitglieder — dann gibt es auch
// nichts zu beantragen und nichts einzutragen.
func TestOhneProjektKeinBeitritt(t *testing.T) {
	ts, _ := beitrittsServer(t)
	resp := doReq(t, "POST", ts+"/api/v1/traeger", betreiberToken, map[string]any{
		"name": "Noch ohne Projekt", "status": "zugelassen", "sichtbarkeit": "offen"})
	traegerID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID

	if resp := beitrittStellen(t, ts, aussenToken, traegerID, "bitte"); resp.StatusCode != http.StatusConflict {
		t.Errorf("HTTP %d, erwartet 409", resp.StatusCode)
	}
	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/traeger/%d/mitglieder", ts, traegerID),
		betreiberToken, map[string]any{"userSub": "erna"})
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("Aufnehmen ohne Projekt: HTTP %d, erwartet 409", resp.StatusCode)
	}
}

// Jede und jeder sieht die eigenen Anträge — mit dem Namen des Trägers, nicht
// mit einer Kennung.
func TestMeineBeitritte(t *testing.T) {
	ts, _ := beitrittsServer(t)
	traegerID := traegerAnlegen(t, ts, "AK 2 Umwelt und Natur", "222")
	beitrittStellen(t, ts, aussenToken, traegerID, "bitte")

	resp := doReq(t, "GET", ts+"/api/v1/me/beitritte", aussenToken, nil)
	liste := decode[struct {
		Beitritte []model.Beitritt `json:"beitritte"`
	}](t, resp).Beitritte
	if len(liste) != 1 || liste[0].TraegerName != "AK 2 Umwelt und Natur" ||
		liste[0].Status != model.AntragBeantragt {
		t.Fatalf("eigene Anträge fehlen oder sind unvollständig: %+v", liste)
	}
}

// Die Liste der Träger sagt, was hier möglich ist: Ohne diese Felder müsste
// die App die Regeln nachbauen — und käme früher oder später zu einem
// anderen Ergebnis als der Server.
func TestTraegerListeSagtWasMoeglichIst(t *testing.T) {
	ts, _ := beitrittsServer(t)
	traegerID := traegerAnlegen(t, ts, "AK 2 Umwelt und Natur", "222")

	resp := doReq(t, "GET", ts+"/api/v1/traeger", aussenToken, nil)
	sicht := decode[struct {
		Traeger []TraegerAnsicht `json:"traeger"`
	}](t, resp).Traeger
	if len(sicht) != 1 || !sicht[0].BeitrittMoeglich || sicht[0].BeitrittStatus != "" {
		t.Fatalf("Beitritt wird nicht angeboten: %+v", sicht)
	}

	beitrittStellen(t, ts, aussenToken, traegerID, "bitte")
	resp = doReq(t, "GET", ts+"/api/v1/traeger", aussenToken, nil)
	sicht = decode[struct {
		Traeger []TraegerAnsicht `json:"traeger"`
	}](t, resp).Traeger
	if sicht[0].BeitrittStatus != model.AntragBeantragt {
		t.Fatalf("der eigene Antrag taucht nicht auf: %+v", sicht)
	}

	// Der Träger-Admin sieht, dass etwas offen ist.
	resp = doReq(t, "GET", ts+"/api/v1/traeger", dorfpflegeAdmin, nil)
	sicht = decode[struct {
		Traeger []TraegerAnsicht `json:"traeger"`
	}](t, resp).Traeger
	if len(sicht) != 1 || sicht[0].OffeneBeitritte != 1 || !sicht[0].DarfVerwalten {
		t.Fatalf("der Träger-Admin sieht den offenen Antrag nicht: %+v", sicht)
	}
}

// Fremde Anträge gehen niemanden etwas an — auch nicht die Verwaltung eines
// anderen Vereins.
func TestFremdeBeitritteBleibenFremd(t *testing.T) {
	ts, _ := beitrittsServer(t)
	traegerID := traegerAnlegen(t, ts, "AK 2 Umwelt und Natur", "222")
	beitrittStellen(t, ts, aussenToken, traegerID, "bitte")

	resp := doReq(t, "GET", fmt.Sprintf("%s/api/v1/traeger/%d/beitritte", ts, traegerID),
		dorfpflegeMitglied, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("ein einfaches Mitglied liest die Anträge: HTTP %d", resp.StatusCode)
	}
	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/traeger/%d/mitglieder", ts, traegerID),
		dorfpflegeMitglied, map[string]any{"userSub": "erna"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("ein einfaches Mitglied nimmt Leute auf: HTTP %d", resp.StatusCode)
	}
}
