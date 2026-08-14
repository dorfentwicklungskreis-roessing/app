package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Technische Fehler der Verwaltung dürfen dem Browser keine Interna zeigen.
func TestFehlerseiteOhneInterna(t *testing.T) {
	a, _, _, _ := aufbau(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/admin/mithelfen/", nil)
	a.fail(w, r, http.StatusInternalServerError, errUnbekannteSeite("geheimes/detail.html"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("Status %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "geheimes/detail.html") {
		t.Fatalf("interne Details in der Antwort: %s", w.Body.String())
	}
}

// Die Cookies der Verwaltung müssen HttpOnly und SameSite=Lax sein; über
// https kommt „Secure" dazu. Ohne das wäre die Sitzung per JavaScript
// auslesbar oder würde bei fremden Seitenaufrufen mitgeschickt.
func TestSitzungsCookieEigenschaften(t *testing.T) {
	a := newApp(Config{PublicURL: "https://app.example", ClientID: "c", SessionKey: []byte("k")})
	w := httptest.NewRecorder()
	a.setCookie(w, cookieSession, "wert", 3600)
	c := w.Result().Cookies()[0]

	if !c.HttpOnly {
		t.Error("Session-Cookie ist nicht HttpOnly")
	}
	if !c.Secure {
		t.Error("Session-Cookie ist über https nicht Secure")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v", c.SameSite)
	}
	if c.MaxAge <= 0 {
		t.Error("Session-Cookie ohne Ablauf")
	}
}

// Signierte Cookies müssen an ihren Zweck gebunden sein. Sonst ließe sich ein
// Cookie, dessen Inhalt ein Angreifer beeinflussen kann (Flash-Meldung), als
// Sitzung oder als Login-Flow wiederverwenden — die Signatur passt ja.
func TestCookieSignaturIstZweckgebunden(t *testing.T) {
	a := newApp(Config{PublicURL: "https://app.example", ClientID: "c", SessionKey: []byte("geheim")})

	s := session{Sub: "u1", Name: "Erna", Admin: true, Exp: time.Now().Add(time.Hour).Unix()}
	wert, err := a.signer.encode(cookieSession, s)
	if err != nil {
		t.Fatal(err)
	}

	var zurueck session
	if !a.signer.decode(cookieSession, wert, &zurueck) {
		t.Fatal("eigener Zweck wurde nicht akzeptiert")
	}
	if zurueck.Sub != "u1" {
		t.Fatalf("Inhalt verfälscht: %+v", zurueck)
	}

	var fremd session
	if a.signer.decode(cookieFlash, wert, &fremd) {
		t.Fatal("Sitzungs-Cookie wurde als Flash-Cookie akzeptiert")
	}
	if a.signer.decode(cookieFlow, wert, &fremd) {
		t.Fatal("Sitzungs-Cookie wurde als Flow-Cookie akzeptiert")
	}
}

// Eine abgelaufene oder manipulierte Sitzung darf niemals gelten.
func TestSitzungAbgelaufenUndManipuliert(t *testing.T) {
	a := newApp(Config{PublicURL: "https://app.example", ClientID: "c", SessionKey: []byte("geheim")})

	abgelaufen, _ := a.signer.encode(cookieSession, session{Sub: "u1", Admin: true, Exp: time.Now().Add(-time.Minute).Unix()})
	r := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	r.AddCookie(&http.Cookie{Name: cookieSession, Value: abgelaufen})
	if _, ok := a.sessionOf(r); ok {
		t.Error("abgelaufene Sitzung wurde akzeptiert")
	}

	gueltig, _ := a.signer.encode(cookieSession, session{Sub: "u1", Admin: true, Exp: time.Now().Add(time.Hour).Unix()})
	nutzlast, _, _ := strings.Cut(gueltig, ".")
	r = httptest.NewRequest(http.MethodGet, "/admin/", nil)
	// Signatur einer fremden Sitzung anhängen: muss auffliegen.
	r.AddCookie(&http.Cookie{Name: cookieSession, Value: nutzlast + ".AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"})
	if _, ok := a.sessionOf(r); ok {
		t.Error("manipulierte Signatur wurde akzeptiert")
	}

	// Auch ein Cookie mit anderem Schlüssel darf nicht gelten.
	b := newApp(Config{PublicURL: "https://app.example", ClientID: "c", SessionKey: []byte("anderer")})
	fremd, _ := b.signer.encode(cookieSession, session{Sub: "boese", Admin: true, Exp: time.Now().Add(time.Hour).Unix()})
	r = httptest.NewRequest(http.MethodGet, "/admin/", nil)
	r.AddCookie(&http.Cookie{Name: cookieSession, Value: fremd})
	if _, ok := a.sessionOf(r); ok {
		t.Error("fremd signierte Sitzung wurde akzeptiert")
	}
}

// Ohne Anmeldung darf keine Verwaltungsseite Daten preisgeben.
func TestVerwaltungBrauchtAnmeldung(t *testing.T) {
	_, h, _, _ := aufbau(t)
	for _, pfad := range []string{
		"/admin/mithelfen/", "/admin/mithelfen/orte/neu", "/admin/mithelfen/rangliste",
		"/admin/mithelfen/einstellungen",
	} {
		w := hole(t, h, pfad)
		if w.Code != http.StatusSeeOther {
			t.Errorf("%s ohne Anmeldung: Status %d", pfad, w.Code)
		}
	}
	for _, pfad := range []string{"/admin/mithelfen/orte/neu", "/admin/mithelfen/einstellungen"} {
		w := sende(t, h, pfad, nil)
		if w.Code != http.StatusSeeOther {
			t.Errorf("POST %s ohne Anmeldung: Status %d", pfad, w.Code)
		}
	}
}
