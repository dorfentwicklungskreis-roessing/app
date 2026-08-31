package admin

import (
	"net/http"
	"testing"
	"time"
)

// Wie lange eine Anmeldung in der Verwaltung hält.
//
// Der Anlass: Der Betreiber musste sich „nach 1h oder so" neu anmelden. Das
// lag an zwei Dingen — einem Signierschlüssel, der jeden Neustart nicht
// überlebte (siehe db.SessionKey), und einer Sitzung, die nach acht Stunden
// endete, ohne sich je zu verlängern.

// sitzungAus liest das Session-Cookie aus einer Antwort, oder nil.
func sitzungAus(w http.Header) *http.Cookie {
	for _, c := range (&http.Response{Header: w}).Cookies() {
		if c.Name == cookieSession {
			return c
		}
	}
	return nil
}

// Wer die Verwaltung benutzt, bleibt angemeldet: Bei jedem Aufruf wandert das
// Ablaufdatum nach vorn — aber höchstens einmal am Tag, damit nicht bei jedem
// Klick ein Cookie neu gesetzt wird.
func TestSitzungWirdBeimBenutzenVerlaengert(t *testing.T) {
	a, mux, _, _ := aufbau(t)

	// Eine Sitzung, die schon fast zwei Tage alt ist.
	alt := session{Sub: "u1", Name: "Testadmin", Admin: true,
		Exp: a.now().Add(sitzungsdauer - 2*sitzungsauffrisch).Unix()}
	wert, err := a.signer.encode(cookieSession, alt)
	if err != nil {
		t.Fatal(err)
	}
	w := hole(t, mux, "/admin/", &http.Cookie{Name: cookieSession, Value: wert})

	c := sitzungAus(w.Header())
	if c == nil {
		t.Fatal("die Sitzung wurde nicht verlängert")
	}
	var neu session
	if !a.signer.decode(cookieSession, c.Value, &neu) {
		t.Fatalf("verlängertes Cookie ist nicht lesbar: %q", c.Value)
	}
	if neu.Exp <= alt.Exp {
		t.Errorf("Ablauf nicht nach vorn geschoben: %d → %d", alt.Exp, neu.Exp)
	}
	if want := a.now().Add(sitzungsdauer).Unix(); neu.Exp != want {
		t.Errorf("Ablauf ist %d, erwartet %d", neu.Exp, want)
	}
	// Wer angemeldet war, bleibt es auch — mit denselben Rechten.
	if neu.Sub != "u1" || !neu.Admin {
		t.Errorf("Sitzung beim Verlängern verändert: %+v", neu)
	}
	if c.MaxAge != int(sitzungsdauer.Seconds()) {
		t.Errorf("MaxAge ist %d, erwartet %d", c.MaxAge, int(sitzungsdauer.Seconds()))
	}
}

// Eine frische Sitzung wird nicht bei jedem Klick neu gesetzt.
func TestFrischeSitzungWirdNichtStaendigNeuGesetzt(t *testing.T) {
	a, mux, _, cookie := aufbau(t)
	// testApp gibt eine Sitzung mit einer Stunde Restlaufzeit — die ist für
	// diesen Fall zu alt. Also eine ganz frische bauen.
	wert, err := a.signer.encode(cookieSession, session{Sub: "u1", Admin: true,
		Exp: a.now().Add(sitzungsdauer).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	cookie.Value = wert

	w := hole(t, mux, "/admin/", cookie)
	if c := sitzungAus(w.Header()); c != nil {
		t.Errorf("frische Sitzung wurde unnötig neu gesetzt: %+v", c)
	}
}

// Eine abgelaufene Sitzung wird nicht wiederbelebt — Verlängern gilt nur für
// Sitzungen, die noch gelten.
func TestAbgelaufeneSitzungWirdNichtVerlaengert(t *testing.T) {
	a, mux, _, _ := aufbau(t)
	wert, err := a.signer.encode(cookieSession, session{Sub: "u1", Admin: true,
		Exp: a.now().Add(-time.Minute).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	w := hole(t, mux, "/admin/mithelfen/", &http.Cookie{Name: cookieSession, Value: wert})
	if w.Code != http.StatusSeeOther {
		t.Fatalf("abgelaufene Sitzung kam durch: Status %d", w.Code)
	}
	if c := sitzungAus(w.Header()); c != nil && c.MaxAge > 0 {
		t.Errorf("abgelaufene Sitzung wurde verlängert: %+v", c)
	}
}

// Dreißig Tage sind Absicht, nicht Zufall: Die Verwaltung benutzen ein paar
// Leute im Dorf nebenbei. Wer sich dabei jedes Mal neu anmelden muss, benutzt
// sie irgendwann nicht mehr.
func TestSitzungsdauerIstLang(t *testing.T) {
	if sitzungsdauer < 7*24*time.Hour {
		t.Errorf("Sitzungsdauer ist %v — zu kurz für eine Verwaltung, "+
			"die man alle paar Tage einmal anfasst", sitzungsdauer)
	}
	if sitzungsauffrisch >= sitzungsdauer {
		t.Error("verlängert wird nie, wenn die Schwelle über der Dauer liegt")
	}
}
