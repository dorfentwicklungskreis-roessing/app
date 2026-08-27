package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/httpx"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Error reports: one tap from the app, and the report has to arrive. Tested
// against a real SQLite file, nothing mocked.

var reportNow = time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)

// errorReportServer builds a server with a real DB, optional authentication
// and a generous limit — the limit has its own test.
func errorReportServer(t *testing.T) (*httptest.Server, *db.DB) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	srv := &Server{
		DB:                 d,
		Now:                func() time.Time { return reportNow },
		OptionalAuth:       auth.Optional(auth.InsecureDevVerifier{}),
		ErrorReportLimiter: httpx.NewRateLimiter(httpx.RateLimitConfig{Burst: 10000, PerMinute: 100000}),
	}
	ts := httptest.NewServer(srv.Handler(auth.Middleware(auth.InsecureDevVerifier{}), nil))
	t.Cleanup(ts.Close)
	return ts, d
}

// guterBericht is what an app sends after one tap — nobody typed a word.
func guterBericht() map[string]any {
	return map[string]any{
		"kind":        "server",
		"message":     "Der Server antwortet gerade nicht (500).",
		"detail":      "HTTP 500 · GET /api/v1/places",
		"area":        "Mithelfen",
		"platform":    "ios",
		"appVersion":  "0.1.10 (42)",
		"osVersion":   "iOS 18.5",
		"deviceModel": "iPhone14,3",
		"occurredAt":  "2026-08-27T11:59:00Z",
	}
}

func TestErrorReportOhneAnmeldung(t *testing.T) {
	ts, d := errorReportServer(t)

	// Genau der Fall, für den der Eingang offen ist: Die Anmeldung klemmt,
	// ein Token gibt es nicht — der Bericht muss trotzdem ankommen.
	resp := doReq(t, "POST", ts.URL+"/api/v1/error-reports", "", guterBericht())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Bericht ohne Anmeldung: Status %d, erwartet 201", resp.StatusCode)
	}
	got := decode[map[string]any](t, resp)
	if got["status"] != "new" || got["kind"] != "server" {
		t.Fatalf("Antwort unerwartet: %v", got)
	}

	berichte, err := d.ListErrorReports("", "")
	if err != nil || len(berichte) != 1 {
		t.Fatalf("Bericht nicht gespeichert: %v %v", berichte, err)
	}
	b := berichte[0]
	if b.UserSub != "" || b.UserName != "" {
		t.Errorf("ohne Token darf keine Person am Bericht hängen: %q/%q", b.UserSub, b.UserName)
	}
	if b.Platform != "ios" || b.Area != "Mithelfen" || b.AppVersion != "0.1.10 (42)" {
		t.Errorf("Angaben nicht übernommen: %+v", b)
	}
	if !b.OccurredAt.Equal(time.Date(2026, 8, 27, 11, 59, 0, 0, time.UTC)) {
		t.Errorf("Zeitpunkt des Vorfalls: %v", b.OccurredAt)
	}
	if !b.CreatedAt.Equal(reportNow) {
		t.Errorf("Eingangszeitpunkt: %v", b.CreatedAt)
	}
}

func TestErrorReportMitAnmeldungHaengtAmKonto(t *testing.T) {
	ts, d := errorReportServer(t)

	// Die Person wird aus dem Token genommen, nie aus dem Rumpf: Wer etwas
	// gemeldet hat, darf eine App nicht behaupten können.
	eingabe := guterBericht()
	eingabe["userSub"] = "fremde-kennung"
	eingabe["userName"] = "Jemand anderes"
	resp := doReq(t, "POST", ts.URL+"/api/v1/error-reports", memberToken, eingabe)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Status %d, erwartet 201", resp.StatusCode)
	}
	resp.Body.Close()

	berichte, _ := d.ListErrorReports("", "")
	if len(berichte) != 1 {
		t.Fatalf("erwartet 1 Bericht, sind %d", len(berichte))
	}
	if berichte[0].UserSub != "member-sub" || berichte[0].UserName != "Erna" {
		t.Fatalf("Person kommt nicht aus dem Token: %q/%q",
			berichte[0].UserSub, berichte[0].UserName)
	}
}

func TestErrorReportMitErgaenzung(t *testing.T) {
	ts, d := errorReportServer(t)

	eingabe := guterBericht()
	eingabe["comment"] = "Ich wollte gerade das Gießen melden.\nDanach war die App weg."
	resp := doReq(t, "POST", ts.URL+"/api/v1/error-reports", "", eingabe)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Status %d, erwartet 201", resp.StatusCode)
	}
	resp.Body.Close()

	berichte, _ := d.ListErrorReports("", "")
	if !strings.Contains(berichte[0].Comment, "Danach war die App weg.") {
		t.Fatalf("Ergänzung nicht gespeichert: %q", berichte[0].Comment)
	}
}

func TestErrorReportValidierung(t *testing.T) {
	ts, _ := errorReportServer(t)

	faelle := []struct {
		name   string
		aendre func(map[string]any)
	}{
		{"unbekannte Art", func(m map[string]any) { m["kind"] = "kaputt" }},
		{"fehlende Art", func(m map[string]any) { delete(m, "kind") }},
		{"unbekannte Plattform", func(m map[string]any) { m["platform"] = "windows" }},
		{"leere Meldung", func(m map[string]any) { m["message"] = "   " }},
		{"überlange Meldung", func(m map[string]any) {
			m["message"] = strings.Repeat("a", MaxErrorMessageLen+1)
		}},
		{"überlange Angaben", func(m map[string]any) {
			m["detail"] = strings.Repeat("x", MaxErrorDetailLen+1)
		}},
		{"überlange Ergänzung", func(m map[string]any) {
			m["comment"] = strings.Repeat("x", MaxErrorCommentLen+1)
		}},
		{"Steuerzeichen in der Meldung", func(m map[string]any) { m["message"] = "kaputt\x00hier" }},
	}
	for _, f := range faelle {
		t.Run(f.name, func(t *testing.T) {
			eingabe := guterBericht()
			f.aendre(eingabe)
			resp := doReq(t, "POST", ts.URL+"/api/v1/error-reports", "", eingabe)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("Status %d, erwartet 400", resp.StatusCode)
			}
			// Die Begründung ist für einen Menschen gedacht und geht im
			// Wortlaut zurück in die App.
			got := decode[map[string]string](t, resp)
			if got["error"] == "" {
				t.Fatalf("ohne Begründung abgewiesen")
			}
		})
	}
}

func TestErrorReportZeitstempelAusDerZukunftWirdEingefangen(t *testing.T) {
	ts, d := errorReportServer(t)

	// Eine falsch gestellte Uhr darf den Bericht nicht ans Ende der Liste
	// schieben, wo ihn niemand mehr sieht.
	eingabe := guterBericht()
	eingabe["occurredAt"] = "2030-01-01T00:00:00Z"
	resp := doReq(t, "POST", ts.URL+"/api/v1/error-reports", "", eingabe)
	resp.Body.Close()

	berichte, _ := d.ListErrorReports("", "")
	if !berichte[0].OccurredAt.Equal(reportNow) {
		t.Fatalf("Zeitpunkt aus der Zukunft übernommen: %v", berichte[0].OccurredAt)
	}
}

func TestErrorReportOhneZeitstempel(t *testing.T) {
	ts, d := errorReportServer(t)

	eingabe := guterBericht()
	delete(eingabe, "occurredAt")
	resp := doReq(t, "POST", ts.URL+"/api/v1/error-reports", "", eingabe)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Status %d, erwartet 201", resp.StatusCode)
	}
	resp.Body.Close()

	berichte, _ := d.ListErrorReports("", "")
	if !berichte[0].OccurredAt.Equal(reportNow) {
		t.Fatalf("ohne Zeitpunkt: %v", berichte[0].OccurredAt)
	}
}

func TestErrorReportRateLimit(t *testing.T) {
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	an := true
	srv := &Server{
		DB:  d,
		Now: func() time.Time { return reportNow },
		// Zwei am Stück, danach ist Schluss — die Grenze wird geprüft, nicht
		// abgewartet: Ein Test, der wartet, prüft das Falsche.
		ErrorReportLimiter: httpx.NewRateLimiter(httpx.RateLimitConfig{
			Burst: 2, PerHour: 2, Enabled: &an,
		}),
	}
	ts := httptest.NewServer(srv.Handler(auth.Middleware(auth.InsecureDevVerifier{}), nil))
	t.Cleanup(ts.Close)

	for i := 0; i < 2; i++ {
		resp := doReq(t, "POST", ts.URL+"/api/v1/error-reports", "", guterBericht())
		resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Bericht %d: Status %d, erwartet 201", i+1, resp.StatusCode)
		}
	}
	resp := doReq(t, "POST", ts.URL+"/api/v1/error-reports", "", guterBericht())
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("dritter Bericht: Status %d, erwartet 429", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Errorf("429 ohne Retry-After")
	}
}

func TestErrorReportStatusUndArtLesen(t *testing.T) {
	if _, err := ErrorReportStatusFrom("quatsch"); err == nil {
		t.Errorf("unbekannter Stand wurde angenommen")
	}
	if st, err := ErrorReportStatusFrom(""); err != nil || st != "" {
		t.Errorf("leerer Stand heißt alle: %q %v", st, err)
	}
	if st, err := ErrorReportStatusFrom("FIXED"); err != nil || st != model.ErrorReportFixed {
		t.Errorf("Großschreibung: %q %v", st, err)
	}
	if _, err := ErrorReportKindFrom("quatsch"); err == nil {
		t.Errorf("unbekannte Art wurde angenommen")
	}
	if k, err := ErrorReportKindFrom("crash"); err != nil || k != model.ErrorReportCrash {
		t.Errorf("Art crash: %q %v", k, err)
	}
}

func TestErrorReportListeFiltert(t *testing.T) {
	ts, d := errorReportServer(t)

	for _, art := range []string{"crash", "server", "network"} {
		eingabe := guterBericht()
		eingabe["kind"] = art
		resp := doReq(t, "POST", ts.URL+"/api/v1/error-reports", "", eingabe)
		resp.Body.Close()
	}
	alle, _ := d.ListErrorReports("", "")
	if len(alle) != 3 {
		t.Fatalf("erwartet 3, sind %d", len(alle))
	}
	abstuerze, _ := d.ListErrorReports("", model.ErrorReportCrash)
	if len(abstuerze) != 1 || abstuerze[0].Kind != model.ErrorReportCrash {
		t.Fatalf("Filter nach Art: %+v", abstuerze)
	}
	neue, _ := d.ListErrorReports(model.ErrorReportNew, "")
	if len(neue) != 3 {
		t.Fatalf("alle sind neu, sind aber %d", len(neue))
	}
	behoben, _ := d.ListErrorReports(model.ErrorReportFixed, "")
	if len(behoben) != 0 {
		t.Fatalf("noch nichts behoben, sind aber %d", len(behoben))
	}
}

func TestErrorReportEinordnenUndLoeschen(t *testing.T) {
	ts, d := errorReportServer(t)
	resp := doReq(t, "POST", ts.URL+"/api/v1/error-reports", "", guterBericht())
	resp.Body.Close()

	berichte, _ := d.ListErrorReports("", "")
	b := berichte[0]
	b.Status = model.ErrorReportFixed
	b.Note = "Lag am Zeitüberschritt beim Kartenstil."
	if err := d.UpdateErrorReport(&b); err != nil {
		t.Fatal(err)
	}
	wieder, err := d.GetErrorReport(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if wieder.Status != model.ErrorReportFixed || wieder.Note == "" {
		t.Fatalf("Stand und Notiz nicht gespeichert: %+v", wieder)
	}
	// Am gemeldeten Sachverhalt ändert das Einordnen nichts.
	if wieder.Message != b.Message || wieder.Detail != b.Detail {
		t.Fatalf("gemeldeter Sachverhalt verändert: %+v", wieder)
	}

	anzahl, _ := d.CountErrorReports()
	if anzahl[model.ErrorReportFixed] != 1 {
		t.Fatalf("Zähler je Stand: %v", anzahl)
	}

	if err := d.DeleteErrorReport(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.GetErrorReport(b.ID); err == nil {
		t.Fatalf("gelöschter Bericht ist noch da")
	}
}
