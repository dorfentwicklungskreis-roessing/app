package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Tests der Profilverwaltung: eigenes Profil, Sichtbarkeit gegenüber anderen
// Dorfbewohnern und die Wirkung auf Rangliste und Historie.
//
// Tokens des InsecureDevVerifier: "sub:name:rollen:email".
const (
	profilErna   = "erna-sub:Erna Beispiel::erna@example.org"
	profilKarl   = "karl-sub:Karl Beispiel::karl@example.org"
	profilChefin = "chef-sub:Chefin:admin:chefin@example.org"
)

func meineDaten(t *testing.T, ts *httptest.Server, token string) map[string]any {
	t.Helper()
	resp := doReq(t, "GET", ts.URL+"/api/v1/me", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /me: Status %d", resp.StatusCode)
	}
	return decode[map[string]any](t, resp)
}

func meinProfil(t *testing.T, ts *httptest.Server, token string) map[string]any {
	t.Helper()
	me := meineDaten(t, ts, token)
	p, ok := me["profile"].(map[string]any)
	if !ok {
		t.Fatalf("GET /me liefert kein Profil: %v", me)
	}
	return p
}

func sichtbarkeit(t *testing.T, profil map[string]any) map[string]any {
	t.Helper()
	v, ok := profil["visibility"].(map[string]any)
	if !ok {
		t.Fatalf("Profil ohne Sichtbarkeiten: %v", profil)
	}
	return v
}

func speichereProfil(t *testing.T, ts *httptest.Server, token string, body map[string]any) *http.Response {
	t.Helper()
	return doReq(t, "PUT", ts.URL+"/api/v1/me/profile", token, body)
}

// TestProfilWirdAusTokenVorbelegt: Wer sich zum ersten Mal meldet, findet
// Anzeigename und E-Mail aus der Rössing-ID vor — Telefon und E-Mail aber
// ausdrücklich noch nicht veröffentlicht.
func TestProfilWirdAusTokenVorbelegt(t *testing.T) {
	ts, _ := newTestServer(t)
	p := meinProfil(t, ts, profilErna)

	if p["displayName"] != "Erna Beispiel" {
		t.Errorf("displayName = %v, erwartet „Erna Beispiel“ aus dem Token", p["displayName"])
	}
	if p["email"] != "erna@example.org" {
		t.Errorf("email = %v, erwartet Vorbelegung aus dem Token", p["email"])
	}
	if p["nickname"] != "" {
		t.Errorf("nickname = %v, erwartet leer", p["nickname"])
	}

	v := sichtbarkeit(t, p)
	for feld, erwartet := range map[string]string{
		"displayName": "dorf",
		"nickname":    "dorf",
		"phone":       "verwaltung",
		"email":       "verwaltung",
		"note":        "verwaltung",
	} {
		if v[feld] != erwartet {
			t.Errorf("Vorbelegung Sichtbarkeit %s = %v, erwartet %s", feld, v[feld], erwartet)
		}
	}
}

func TestProfilAendernUndLesen(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := speichereProfil(t, ts, profilErna, map[string]any{
		"displayName": "Erna Beispiel",
		"nickname":    "Gießmeisterin",
		"phone":       "05066 / 12 34 56",
		"email":       "erna.privat@example.org",
		"note":        "erreichbar abends",
		"visibility": map[string]any{
			"displayName": "dorf", "nickname": "dorf",
			"phone": "dorf", "email": "verwaltung", "note": "dorf",
		},
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /me/profile: Status %d", resp.StatusCode)
	}
	gespeichert := decode[map[string]any](t, resp)
	if gespeichert["nickname"] != "Gießmeisterin" {
		t.Fatalf("Antwort enthält den Nickname nicht: %v", gespeichert)
	}

	p := meinProfil(t, ts, profilErna)
	if p["phone"] != "05066 / 12 34 56" || p["note"] != "erreichbar abends" {
		t.Fatalf("Profil nicht gespeichert: %v", p)
	}
	if sichtbarkeit(t, p)["phone"] != "dorf" {
		t.Fatalf("Sichtbarkeit nicht gespeichert: %v", p)
	}
	if p["updatedAt"] == "" || p["updatedAt"] == nil {
		t.Fatalf("updatedAt fehlt: %v", p)
	}
}

// TestFremdesProfilVerboten: Der Aufruf ändert ausschließlich das eigene
// Profil. Wer eine fremde Kennung mitschickt, bekommt 403 — auch als Admin.
func TestFremdesProfilVerboten(t *testing.T) {
	ts, _ := newTestServer(t)
	for name, token := range map[string]string{"Mitglied": profilErna, "Verwaltende": profilChefin} {
		resp := speichereProfil(t, ts, token, map[string]any{
			"userSub": "karl-sub", "displayName": "Fremdgeschrieben",
		})
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s ändert fremdes Profil: Status %d, erwartet 403", name, resp.StatusCode)
		}
		resp.Body.Close()
	}
	// Und wirklich nichts passiert.
	if p := meinProfil(t, ts, profilKarl); p["displayName"] == "Fremdgeschrieben" {
		t.Fatalf("fremdes Profil wurde trotzdem geändert: %v", p)
	}
}

func TestProfilValidierung(t *testing.T) {
	ts, _ := newTestServer(t)
	for name, body := range map[string]map[string]any{
		"Anzeigename zu lang":      {"displayName": strings.Repeat("a", MaxNameLen+1)},
		"Nickname zu lang":         {"nickname": strings.Repeat("b", MaxNickLen+1)},
		"Notiz zu lang":            {"note": strings.Repeat("c", MaxNoteLen+1)},
		"kaputte E-Mail":           {"email": "keine-adresse"},
		"E-Mail ohne Domain":       {"email": "erna@localhost"},
		"Telefon mit Buchstaben":   {"phone": "ruf mich an"},
		"Telefon zu kurz":          {"phone": "12"},
		"Steuerzeichen im Namen":   {"displayName": "Erna\a Beispiel"},
		"Nullbyte im Nickname":     {"nickname": "Gie\u00df\x00meisterin"},
		"Tabulator im Telefon":     {"phone": "05066\t123456"},
		"Telefon zu lang":          {"phone": strings.Repeat("1", MaxPhoneLen+1)},
		"E-Mail mit Leerzeichen":   {"email": "erna @example.org"},
		"Zeilenumbruch in Notiz":   {"note": "abends\nund nachts"},
		"unbekannte Sichtbarkeit":  {"visibility": map[string]any{"phone": "alle-welt"}},
		"Steuerzeichen im Telefon": {"phone": "05066\x0012 34"},
	} {
		resp := speichereProfil(t, ts, profilErna, body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: Status %d, erwartet 400", name, resp.StatusCode)
		}
		resp.Body.Close()
	}
}

// --- Dorfbewohner-Liste -------------------------------------------------------

type mitgliederAntwort struct {
	AdminView bool `json:"adminView"`
	Members   []struct {
		UserSub     string   `json:"userSub"`
		Name        string   `json:"name"`
		DisplayName string   `json:"displayName"`
		Nickname    string   `json:"nickname"`
		Phone       string   `json:"phone"`
		Email       string   `json:"email"`
		Note        string   `json:"note"`
		Restricted  []string `json:"restricted"`
	} `json:"members"`
}

func mitglieder(t *testing.T, ts *httptest.Server, token string) mitgliederAntwort {
	t.Helper()
	resp := doReq(t, "GET", ts.URL+"/api/v1/members", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /members: Status %d", resp.StatusCode)
	}
	return decode[mitgliederAntwort](t, resp)
}

func TestMitgliederlisteNurAngemeldet(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := doReq(t, "GET", ts.URL+"/api/v1/members", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("ohne Token: Status %d, erwartet 401", resp.StatusCode)
	}
	resp.Body.Close()
}

// TestMitgliederSehenNurFreigegebenes ist der Kern der Zusage an die
// Dorfbewohner: Was nicht freigegeben ist, verlässt den Server nicht.
func TestMitgliederSehenNurFreigegebenes(t *testing.T) {
	ts, _ := newTestServer(t)
	if resp := speichereProfil(t, ts, profilErna, map[string]any{
		"displayName": "Erna Beispiel", "nickname": "Gießmeisterin",
		"phone": "05066 123456", "email": "erna@example.org", "note": "erreichbar abends",
		"visibility": map[string]any{
			"displayName": "dorf", "nickname": "dorf",
			"phone": "verwaltung", "email": "dorf", "note": "verwaltung",
		},
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("Profil speichern: Status %d", resp.StatusCode)
	}

	// Sicht eines Dorfbewohners.
	liste := mitglieder(t, ts, profilKarl)
	if liste.AdminView {
		t.Error("adminView ist für ein Mitglied gesetzt")
	}
	var erna *struct {
		UserSub     string   `json:"userSub"`
		Name        string   `json:"name"`
		DisplayName string   `json:"displayName"`
		Nickname    string   `json:"nickname"`
		Phone       string   `json:"phone"`
		Email       string   `json:"email"`
		Note        string   `json:"note"`
		Restricted  []string `json:"restricted"`
	}
	for i := range liste.Members {
		if liste.Members[i].UserSub == "erna-sub" {
			erna = &liste.Members[i]
		}
	}
	if erna == nil {
		t.Fatalf("Erna fehlt in der Liste: %+v", liste)
	}
	if erna.Name != "Gießmeisterin" {
		t.Errorf("Name = %q, erwartet den Nickname", erna.Name)
	}
	if erna.Email != "erna@example.org" {
		t.Errorf("freigegebene E-Mail fehlt: %q", erna.Email)
	}
	if erna.Phone != "" {
		t.Errorf("nicht freigegebene Telefonnummer wurde ausgeliefert: %q", erna.Phone)
	}
	if erna.Note != "" {
		t.Errorf("nicht freigegebene Notiz wurde ausgeliefert: %q", erna.Note)
	}
	if len(erna.Restricted) != 0 {
		t.Errorf("restricted ist für Mitglieder leer, war %v", erna.Restricted)
	}

	// Sicht der Verwaltung: alles, aber gekennzeichnet.
	adminListe := mitglieder(t, ts, profilChefin)
	if !adminListe.AdminView {
		t.Error("adminView fehlt für Verwaltende")
	}
	var ernaAdmin string
	var eingeschraenkt []string
	for _, m := range adminListe.Members {
		if m.UserSub == "erna-sub" {
			ernaAdmin, eingeschraenkt = m.Phone, m.Restricted
		}
	}
	if ernaAdmin != "05066 123456" {
		t.Errorf("Verwaltende sehen die Telefonnummer nicht: %q", ernaAdmin)
	}
	if !enthaelt(eingeschraenkt, "phone") || !enthaelt(eingeschraenkt, "note") {
		t.Errorf("restricted = %v, erwartet phone und note", eingeschraenkt)
	}
	if enthaelt(eingeschraenkt, "email") {
		t.Errorf("die freigegebene E-Mail steht fälschlich in restricted: %v", eingeschraenkt)
	}
}

// TestGanzZurueckhaltendeTauchenNichtAuf: Wer weder Anzeigename noch Nickname
// freigibt, erscheint für Mitglieder gar nicht in der Liste — die Verwaltung
// sieht ihn weiterhin.
func TestGanzZurueckhaltendeTauchenNichtAuf(t *testing.T) {
	ts, _ := newTestServer(t)
	if resp := speichereProfil(t, ts, profilErna, map[string]any{
		"displayName": "Erna Beispiel",
		"visibility": map[string]any{
			"displayName": "verwaltung", "nickname": "verwaltung",
			"phone": "verwaltung", "email": "verwaltung", "note": "verwaltung",
		},
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("Profil speichern: Status %d", resp.StatusCode)
	}
	for _, m := range mitglieder(t, ts, profilKarl).Members {
		if m.UserSub == "erna-sub" {
			t.Fatalf("zurückhaltendes Profil ist für Mitglieder sichtbar: %+v", m)
		}
	}
	gefunden := false
	for _, m := range mitglieder(t, ts, profilChefin).Members {
		if m.UserSub == "erna-sub" {
			gefunden = true
		}
	}
	if !gefunden {
		t.Fatal("Verwaltende sehen das Profil nicht")
	}
}

func enthaelt(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// --- Namen in Rangliste und Historie -----------------------------------------

type ranglisteAntwort struct {
	Entries []struct {
		UserSub  string `json:"userSub"`
		UserName string `json:"userName"`
	} `json:"entries"`
	Me *struct {
		UserName string `json:"userName"`
	} `json:"me"`
}

func rangliste(t *testing.T, ts *httptest.Server, token string) ranglisteAntwort {
	t.Helper()
	resp := doReq(t, "GET", ts.URL+"/api/v1/stats/leaderboard?period=gesamt", token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /stats/leaderboard: Status %d", resp.StatusCode)
	}
	return decode[ranglisteAntwort](t, resp)
}

// TestRanglisteNutztProfilnamen: Der Nickname aus dem Profil ersetzt den bei
// der Meldung eingefrorenen Namen — rückwirkend, ohne Datenwanderung.
func TestRanglisteNutztProfilnamen(t *testing.T) {
	ts, _ := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)

	// Erna meldet noch unter ihrem Namen aus der Rössing-ID.
	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), profilErna, map[string]any{"liters": 10})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Erledigung melden: Status %d", resp.StatusCode)
	}
	resp.Body.Close()

	if got := rangliste(t, ts, profilErna).Entries[0].UserName; got != "Erna Beispiel" {
		t.Fatalf("Rangliste vor dem Profil = %q, erwartet den Namen aus dem Token", got)
	}

	// Nun gibt sie sich einen Nickname — und die Rangliste zieht nach.
	if resp := speichereProfil(t, ts, profilErna, map[string]any{
		"displayName": "Erna Beispiel", "nickname": "Gießmeisterin",
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("Profil speichern: Status %d", resp.StatusCode)
	}

	liste := rangliste(t, ts, profilErna)
	if got := liste.Entries[0].UserName; got != "Gießmeisterin" {
		t.Fatalf("Rangliste = %q, erwartet den Nickname aus dem Profil", got)
	}
	if liste.Me == nil || liste.Me.UserName != "Gießmeisterin" {
		t.Fatalf("eigener Eintrag = %+v, erwartet den Nickname", liste.Me)
	}
}

// TestRanglisteOhneNicknameNutztAnzeigenamen: Ist kein Nickname gesetzt, gilt
// der Anzeigename.
func TestRanglisteOhneNicknameNutztAnzeigenamen(t *testing.T) {
	ts, _ := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)
	doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), profilErna, nil).Body.Close()

	if resp := speichereProfil(t, ts, profilErna, map[string]any{"displayName": "Erna aus der Bachstraße"}); resp.StatusCode != http.StatusOK {
		t.Fatalf("Profil speichern: Status %d", resp.StatusCode)
	}
	if got := rangliste(t, ts, profilErna).Entries[0].UserName; got != "Erna aus der Bachstraße" {
		t.Fatalf("Rangliste = %q, erwartet den Anzeigenamen", got)
	}
}

// TestBestandsdatenOhneProfilBleiben: Meldungen von Leuten ohne Profil
// behalten den gespeicherten Namen. Ohne diese Regel wäre die alte Historie
// nach der Umstellung namenlos.
func TestBestandsdatenOhneProfilBleiben(t *testing.T) {
	ts, _ := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)
	doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), memberToken, nil).Body.Close()

	if got := rangliste(t, ts, profilChefin).Entries[0].UserName; got != "Erna" {
		t.Fatalf("Rangliste = %q, erwartet den gespeicherten Namen", got)
	}
}

// TestNachtragUnterFremdemNamenBleibt: Trägt die Verwaltung eine telefonisch
// gemeldete Erledigung für jemanden ein, gehört diese Zeile der genannten
// Person — der Profilname der eintragenden Person darf sie nicht überschreiben.
func TestNachtragUnterFremdemNamenBleibt(t *testing.T) {
	ts, _ := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)

	if resp := speichereProfil(t, ts, profilChefin, map[string]any{
		"displayName": "Chefin", "nickname": "Die Chefin",
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("Profil speichern: Status %d", resp.StatusCode)
	}
	resp := doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), profilChefin,
		map[string]any{"name": "Oma Meier", "force": true})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Nachtrag: Status %d", resp.StatusCode)
	}
	resp.Body.Close()

	namen := map[string]bool{}
	for _, e := range rangliste(t, ts, profilChefin).Entries {
		namen[e.UserName] = true
	}
	if !namen["Oma Meier"] {
		t.Fatalf("Nachtrag läuft nicht mehr unter „Oma Meier“: %v", namen)
	}
	if namen["Die Chefin"] {
		t.Fatalf("Nachtrag wurde der eintragenden Person zugeschlagen: %v", namen)
	}
}

// TestHistorieNutztProfilnamen: Auch die Erledigungs-Historie zeigt den
// Profilnamen.
func TestHistorieNutztProfilnamen(t *testing.T) {
	ts, _ := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)
	doReq(t, "POST", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), profilErna, nil).Body.Close()
	if resp := speichereProfil(t, ts, profilErna, map[string]any{
		"displayName": "Erna Beispiel", "nickname": "Gießmeisterin",
	}); resp.StatusCode != http.StatusOK {
		t.Fatalf("Profil speichern: Status %d", resp.StatusCode)
	}

	hist := decode[struct {
		Completions []struct {
			UserName string `json:"userName"`
		} `json:"completions"`
	}](t, doReq(t, "GET", ts.URL+fmt.Sprintf("/api/v1/tasks/%d/completions", taskID), profilKarl, nil))
	if len(hist.Completions) != 1 || hist.Completions[0].UserName != "Gießmeisterin" {
		t.Fatalf("Historie = %+v, erwartet den Nickname", hist.Completions)
	}

	// Und in der Ortsliste (letzte Erledigung) ebenso.
	orte := decode[struct {
		Places []struct {
			Tasks []struct {
				LastCompletion *struct {
					UserName string `json:"userName"`
				} `json:"lastCompletion"`
			} `json:"tasks"`
		} `json:"places"`
	}](t, doReq(t, "GET", ts.URL+"/api/v1/places", profilKarl, nil))
	letzte := orte.Places[0].Tasks[0].LastCompletion
	if letzte == nil || letzte.UserName != "Gießmeisterin" {
		t.Fatalf("letzte Erledigung = %+v, erwartet den Nickname", letzte)
	}
}

// TestRanglisteZeigtSpitznameStattLeerstelle baut den Zustand nach, der in
// der Produktion aufgefallen ist: ein Konto ohne jeden Namen — die Rössing-ID
// liefert weder „name“ noch „preferred_username“, also ist auch der bei der
// Meldung eingefrorene Name leer. Die Zeile stand mit Punkten, Litern und
// Auszeichnungen in der Rangliste, nur eben ohne Namen.
//
// Der Umweg über die Datenbank ist Absicht: Der Dev-Verifier kann kein Token
// ohne Namen ausstellen (er setzt notfalls die Kennung ein), der echte
// OIDC-Verifier sehr wohl.
func TestRanglisteZeigtSpitznameStattLeerstelle(t *testing.T) {
	ts, srv := newTestServer(t)
	_, taskID := createPlaceWithTask(t, ts)

	const namenlos = "namenlose-kennung"
	if err := srv.DB.UpsertProfile(&model.Profile{
		UserSub: namenlos, Visibility: model.DefaultVisibility(), UpdatedAt: srv.now(),
	}); err != nil {
		t.Fatal(err)
	}
	liter := 20.0
	if err := srv.DB.InsertCompletion(&model.Completion{
		TaskID: taskID, UserSub: namenlos, Liters: &liter, DoneAt: srv.now(),
	}); err != nil {
		t.Fatal(err)
	}

	liste := rangliste(t, ts, profilKarl)
	if len(liste.Entries) == 0 {
		t.Fatal("die Rangliste ist leer")
	}
	eintrag := liste.Entries[0]
	if eintrag.UserSub != namenlos {
		t.Fatalf("Platz 1 = %+v, erwartet die namenlose Kennung", eintrag)
	}
	if eintrag.UserName == "" {
		t.Fatal("Platz 1 steht weiterhin ohne Namen in der Rangliste")
	}
	if got, want := eintrag.UserName, model.AnonymousName(namenlos); got != want {
		t.Fatalf("Platz 1 heißt %q, erwartet den Spitznamen %q", got, want)
	}

	// Im Verzeichnis der Dorfbewohner ändert sich dagegen nichts: Wer keinen
	// Namen freigegeben hat, erscheint dort weiterhin nicht.
	for _, m := range mitglieder(t, ts, profilKarl).Members {
		if m.UserSub == namenlos {
			t.Fatalf("der Spitzname holt jemanden ins Verzeichnis: %+v", m)
		}
	}
}
