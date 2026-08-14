package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/httpx"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Ideen-Sammlung: „Sag uns, was die App können soll.“ Der Eingang ist
// öffentlich (die Website ist öffentlich), alles andere darf nur die
// Verwaltung. Getestet wird gegen eine echte SQLite-Datei, nichts gemockt.

var ideenJetzt = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

// ideenServer baut einen Server mit echter DB, optionaler Anmeldung (die
// App schickt ein Token, die Website nicht) und abgeschalteter Begrenzung.
func ideenServer(t *testing.T) (*httptest.Server, *Server, *db.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	srv := &Server{
		DB:           d,
		Now:          func() time.Time { return ideenJetzt },
		OptionalAuth: auth.Optional(auth.InsecureDevVerifier{}),
		// Großzügig: die Begrenzung hat einen eigenen Test.
		IdeenLimiter: httpx.NewRateLimiter(httpx.RateLimitConfig{Burst: 10000, PerMinute: 100000}),
	}
	ts := httptest.NewServer(srv.Handler(auth.Middleware(auth.InsecureDevVerifier{}), nil))
	t.Cleanup(ts.Close)
	return ts, srv, d
}

// sendeIdee schickt ein klassisches HTML-Formular an den öffentlichen Eingang.
func sendeIdee(t *testing.T, ts *httptest.Server, token string, werte url.Values, kopf map[string]string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/ideen", strings.NewReader(werte.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range kopf {
		req.Header.Set(k, v)
	}
	// Weiterleitungen nicht selbst verfolgen — sie sind Teil der Prüfung.
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

const guterWunsch = "Ein Vertretungsplan für die Grundschule wäre großartig."

func TestIdeeEinreichenOhneAnmeldung(t *testing.T) {
	ts, _, d := ideenServer(t)

	resp := doReq(t, "POST", ts.URL+"/api/v1/ideen", "", map[string]any{
		"name": "Erna Musterfrau", "email": "erna@example.org", "wunsch": guterWunsch,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Einreichen ohne Anmeldung: Status %d, erwartet 201", resp.StatusCode)
	}
	got := decode[map[string]any](t, resp)
	if got["wunsch"] != guterWunsch || got["status"] != "neu" || got["quelle"] != "website" {
		t.Fatalf("Antwort unerwartet: %v", got)
	}

	ideen, err := d.ListIdeen("")
	if err != nil || len(ideen) != 1 {
		t.Fatalf("Idee nicht gespeichert: %v %v", ideen, err)
	}
	i := ideen[0]
	if i.Name != "Erna Musterfrau" || i.Email != "erna@example.org" || i.UserSub != "" {
		t.Fatalf("Idee falsch gespeichert: %+v", i)
	}
	if i.Status != model.IdeeNeu || i.Quelle != model.IdeeQuelleWebsite {
		t.Fatalf("Status/Quelle falsch: %+v", i)
	}
	if !i.CreatedAt.Equal(ideenJetzt) {
		t.Fatalf("createdAt = %v, erwartet %v", i.CreatedAt, ideenJetzt)
	}
}

func TestIdeeAusDerAppTraegtDasKonto(t *testing.T) {
	ts, _, d := ideenServer(t)

	resp := sendeIdee(t, ts, memberToken, url.Values{
		"wunsch": {guterWunsch}, "quelle": {"app"},
	}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Einreichen aus der App: Status %d, erwartet 201", resp.StatusCode)
	}
	ideen, _ := d.ListIdeen("")
	if len(ideen) != 1 {
		t.Fatalf("Idee nicht gespeichert: %v", ideen)
	}
	if ideen[0].UserSub != "member-sub" {
		t.Fatalf("userSub = %q, erwartet member-sub", ideen[0].UserSub)
	}
	if ideen[0].Quelle != model.IdeeQuelleApp {
		t.Fatalf("quelle = %q, erwartet app", ideen[0].Quelle)
	}
}

func TestIdeeValidierung(t *testing.T) {
	ts, _, d := ideenServer(t)

	for name, werte := range map[string]map[string]any{
		"Wunsch fehlt":          {"wunsch": ""},
		"Wunsch zu kurz":        {"wunsch": "kurz"},
		"Wunsch zu lang":        {"wunsch": strings.Repeat("a", 2001)},
		"Wunsch Steuerzeichen":  {"wunsch": "Ein Wunsch mit \x00 Nullbyte darin"},
		"Name zu lang":          {"wunsch": guterWunsch, "name": strings.Repeat("N", 101)},
		"Name Steuerzeichen":    {"wunsch": guterWunsch, "name": "Erna\nMusterfrau"},
		"E-Mail kaputt":         {"wunsch": guterWunsch, "email": "keine-mail"},
		"E-Mail ohne Punkt":     {"wunsch": guterWunsch, "email": "erna@localhost"},
		"E-Mail zu lang":        {"wunsch": guterWunsch, "email": strings.Repeat("a", 195) + "@example.org"},
		"E-Mail Steuerzeichen":  {"wunsch": guterWunsch, "email": "erna@example.org\r\nBcc: x@y.z"},
		"Wunsch nur Leerzeilen": {"wunsch": "   \n\t  "},
	} {
		resp := doReq(t, "POST", ts.URL+"/api/v1/ideen", "", werte)
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: Status %d, erwartet 400", name, resp.StatusCode)
		}
		resp.Body.Close()
	}
	if ideen, _ := d.ListIdeen(""); len(ideen) != 0 {
		t.Fatalf("trotz Fehlern gespeichert: %v", ideen)
	}

	// Zeilenumbrüche im Wunsch sind erlaubt — Menschen gliedern ihren Text.
	resp := doReq(t, "POST", ts.URL+"/api/v1/ideen", "", map[string]any{
		"wunsch": "Erstens: ein Kalender.\nZweitens: ein Mitfahrbrett.",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Wunsch mit Zeilenumbruch: Status %d, erwartet 201", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestIdeeHonigtopfWirdVerworfen(t *testing.T) {
	ts, _, d := ideenServer(t)

	// Das versteckte Feld füllt kein Mensch aus — nur ein Skript.
	resp := sendeIdee(t, ts, "", url.Values{
		"wunsch": {guterWunsch}, "webseite": {"http://spam.example"},
	}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Honigtopf: Status %d, erwartet freundliche 201", resp.StatusCode)
	}
	if ideen, _ := d.ListIdeen(""); len(ideen) != 0 {
		t.Fatalf("Honigtopf-Einreichung wurde gespeichert: %v", ideen)
	}
}

func TestIdeeMindestzeitZwischenAufrufUndAbsenden(t *testing.T) {
	ts, _, d := ideenServer(t)

	// Im selben Augenblick abgeschickt wie aufgerufen → Skript.
	zuSchnell := url.Values{"wunsch": {guterWunsch}, "gestartet": {millis(ideenJetzt)}}
	resp := sendeIdee(t, ts, "", zuSchnell, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("zu schnell: Status %d, erwartet freundliche 201", resp.StatusCode)
	}
	if ideen, _ := d.ListIdeen(""); len(ideen) != 0 {
		t.Fatalf("zu schnelle Einreichung wurde gespeichert: %v", ideen)
	}

	// Wer eine halbe Minute getippt hat, kommt durch.
	langsam := url.Values{"wunsch": {guterWunsch}, "gestartet": {millis(ideenJetzt.Add(-30 * time.Second))}}
	resp = sendeIdee(t, ts, "", langsam, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("langsam getippt: Status %d, erwartet 201", resp.StatusCode)
	}
	if ideen, _ := d.ListIdeen(""); len(ideen) != 1 {
		t.Fatalf("langsame Einreichung fehlt: %v", ideen)
	}

	// Ohne das Feld (kein JavaScript) wird nicht geprüft.
	resp = sendeIdee(t, ts, "", url.Values{"wunsch": {guterWunsch}}, nil)
	resp.Body.Close()
	if ideen, _ := d.ListIdeen(""); len(ideen) != 2 {
		t.Fatalf("Einreichung ohne Zeitstempel fehlt: %v", ideen)
	}
}

func TestIdeenRateLimit(t *testing.T) {
	ts, srv, d := ideenServer(t)
	// Drei Einreichungen am Stück, danach ist Schluss (Nachschub pro Stunde).
	srv.IdeenLimiter = httpx.NewRateLimiter(httpx.RateLimitConfig{Burst: 3, PerHour: 3})

	for i := 0; i < 3; i++ {
		resp := sendeIdee(t, ts, "", url.Values{"wunsch": {guterWunsch}}, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Einreichung %d: Status %d, erwartet 201", i+1, resp.StatusCode)
		}
	}
	resp := sendeIdee(t, ts, "", url.Values{"wunsch": {guterWunsch}}, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("vierte Einreichung: Status %d, erwartet 429", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("Retry-After fehlt")
	}
	if ideen, _ := d.ListIdeen(""); len(ideen) != 3 {
		t.Fatalf("%d Ideen gespeichert, erwartet 3", len(ideen))
	}
}

func TestIdeenWeiterleitungNurAufErlaubteZiele(t *testing.T) {
	ts, srv, _ := ideenServer(t)
	srv.IdeenRedirects = []string{"https://xn--rssing-wxa.de"}

	// Erlaubtes Ziel → 303 dorthin.
	resp := sendeIdee(t, ts, "", url.Values{
		"wunsch": {guterWunsch}, "redirect": {"https://xn--rssing-wxa.de/app/danke"},
	}, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("erlaubtes Ziel: Status %d, erwartet 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); got != "https://xn--rssing-wxa.de/app/danke" {
		t.Fatalf("Location = %q", got)
	}

	// Alles andere wird abgewiesen — offene Weiterleitungen gibt es hier nicht.
	for _, ziel := range []string{
		"https://boese.example/",
		"//boese.example/",
		"https://xn--rssing-wxa.de.boese.example/app/danke",
		"http://xn--rssing-wxa.de/app/danke",
		"/app/danke",
		"javascript:alert(1)",
		"https://xn--rssing-wxa.de@boese.example/",
	} {
		resp := sendeIdee(t, ts, "", url.Values{"wunsch": {guterWunsch}, "redirect": {ziel}}, nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Ziel %q: Status %d, erwartet 400", ziel, resp.StatusCode)
		}
	}
}

func TestIdeeAusDemBrowserLandetAufDerDankeseite(t *testing.T) {
	ts, srv, d := ideenServer(t)
	srv.IdeenRedirects = []string{"https://xn--rssing-wxa.de"}

	// Ein Browser ohne JavaScript schickt ein Formular und akzeptiert HTML.
	html := map[string]string{"Accept": "text/html,application/xhtml+xml"}
	resp := sendeIdee(t, ts, "", url.Values{"wunsch": {guterWunsch}}, html)
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("Browser-Absenden: Status %d, erwartet 303", resp.StatusCode)
	}
	if got := resp.Header.Get("Location"); !strings.HasSuffix(got, "/app/danke") {
		t.Fatalf("Location = %q, erwartet Dankeseite", got)
	}
	if ideen, _ := d.ListIdeen(""); len(ideen) != 1 {
		t.Fatalf("Idee nicht gespeichert: %v", ideen)
	}

	// Fehlerfall: verständliche Seite, und der getippte Text ist noch da.
	resp = sendeIdee(t, ts, "", url.Values{
		"wunsch": {"zu"}, "name": {"Erna"}, "email": {"erna@example.org"},
	}, html)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Fehlerfall: Status %d, erwartet 400", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, erwartet HTML", ct)
	}
	seite := lies(t, resp)
	for _, muss := range []string{"zu", "Erna", "erna@example.org", "mindestens"} {
		if !strings.Contains(seite, muss) {
			t.Errorf("Fehlerseite enthält %q nicht:\n%s", muss, seite)
		}
	}
}

func TestIdeenVerwaltungNurFuerAdmins(t *testing.T) {
	ts, _, d := ideenServer(t)
	id := legeIdeeAn(t, d)

	faelle := []struct{ methode, pfad string }{
		{"GET", "/api/v1/ideen"},
		{"PATCH", "/api/v1/ideen/" + itoa64(id)},
		{"DELETE", "/api/v1/ideen/" + itoa64(id)},
	}
	for _, f := range faelle {
		resp := doReq(t, f.methode, ts.URL+f.pfad, memberToken, map[string]any{"status": "gelesen"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s als Mitglied: Status %d, erwartet 403", f.methode, f.pfad, resp.StatusCode)
		}
		resp = doReq(t, f.methode, ts.URL+f.pfad, "", map[string]any{"status": "gelesen"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s ohne Token: Status %d, erwartet 401", f.methode, f.pfad, resp.StatusCode)
		}
	}
}

func TestIdeenListeStatuswechselUndLoeschen(t *testing.T) {
	ts, _, d := ideenServer(t)
	id := legeIdeeAn(t, d)

	liste := decode[struct {
		Ideen []model.Idee `json:"ideen"`
	}](t, doReq(t, "GET", ts.URL+"/api/v1/ideen", adminToken, nil))
	if len(liste.Ideen) != 1 || liste.Ideen[0].ID != id {
		t.Fatalf("Liste unerwartet: %+v", liste)
	}

	// Statuswechsel samt interner Notiz.
	resp := doReq(t, "PATCH", ts.URL+"/api/v1/ideen/"+itoa64(id), adminToken,
		map[string]any{"status": "umgesetzt", "notiz": "Kommt mit Version 0.3."})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PATCH: Status %d", resp.StatusCode)
	}
	nach := decode[model.Idee](t, resp)
	if nach.Status != model.IdeeUmgesetzt || nach.Notiz != "Kommt mit Version 0.3." {
		t.Fatalf("Statuswechsel nicht übernommen: %+v", nach)
	}

	// Unbekannter Status wird abgewiesen.
	resp = doReq(t, "PATCH", ts.URL+"/api/v1/ideen/"+itoa64(id), adminToken, map[string]any{"status": "vielleicht"})
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unsinniger Status: %d, erwartet 400", resp.StatusCode)
	}

	// Filter nach Status.
	leer := decode[struct {
		Ideen []model.Idee `json:"ideen"`
	}](t, doReq(t, "GET", ts.URL+"/api/v1/ideen?status=neu", adminToken, nil))
	if len(leer.Ideen) != 0 {
		t.Fatalf("Filter status=neu liefert noch Einträge: %+v", leer)
	}
	voll := decode[struct {
		Ideen []model.Idee `json:"ideen"`
	}](t, doReq(t, "GET", ts.URL+"/api/v1/ideen?status=umgesetzt", adminToken, nil))
	if len(voll.Ideen) != 1 {
		t.Fatalf("Filter status=umgesetzt liefert nichts: %+v", voll)
	}

	// Löschen.
	resp = doReq(t, "DELETE", ts.URL+"/api/v1/ideen/"+itoa64(id), adminToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE: Status %d, erwartet 204", resp.StatusCode)
	}
	resp = doReq(t, "DELETE", ts.URL+"/api/v1/ideen/"+itoa64(id), adminToken, nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("DELETE zweimal: Status %d, erwartet 404", resp.StatusCode)
	}
}

// --- Helfer ------------------------------------------------------------------

func legeIdeeAn(t *testing.T, d *db.DB) int64 {
	t.Helper()
	i := model.Idee{
		Name: "Erna", Email: "erna@example.org", Wunsch: guterWunsch,
		Quelle: model.IdeeQuelleWebsite, Status: model.IdeeNeu, CreatedAt: ideenJetzt,
	}
	if err := d.InsertIdee(&i); err != nil {
		t.Fatal(err)
	}
	return i.ID
}

func millis(t time.Time) string { return itoa64(t.UnixMilli()) }

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [24]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

func lies(t *testing.T, resp *http.Response) string {
	t.Helper()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String()
}
