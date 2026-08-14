package admin

import (
	"net/http"
	"strings"
	"testing"
)

// Der Bereich hieß früher „Dorfpflege“ und lag unter /admin/dorfpflege/.
// Gespeicherte Lesezeichen und verschickte Links müssen weiter funktionieren,
// deshalb bleibt der alte Pfad als dauerhafte Weiterleitung bestehen.
func TestAlteDorfpflegePfadeLeitenDauerhaftWeiter(t *testing.T) {
	_, h, _, sitzung := aufbau(t)

	faelle := []struct{ alt, neu string }{
		{"/admin/dorfpflege/", "/admin/mithelfen/"},
		{"/admin/dorfpflege/orte/neu", "/admin/mithelfen/orte/neu"},
		{"/admin/dorfpflege/orte/42", "/admin/mithelfen/orte/42"},
		{"/admin/dorfpflege/aufgaben/7/erledigt", "/admin/mithelfen/aufgaben/7/erledigt"},
		{"/admin/dorfpflege/rangliste?zeitraum=woche", "/admin/mithelfen/rangliste?zeitraum=woche"},
		{"/admin/dorfpflege/einstellungen", "/admin/mithelfen/einstellungen"},
	}
	for _, f := range faelle {
		// Auch ohne Anmeldung muss die Weiterleitung greifen — sonst landet
		// ein alter Link zuerst auf der Anmeldeseite und verliert sein Ziel.
		w := hole(t, h, f.alt)
		if w.Code != http.StatusPermanentRedirect {
			t.Fatalf("%s: Code %d, erwartet %d", f.alt, w.Code, http.StatusPermanentRedirect)
		}
		if ort := w.Header().Get("Location"); ort != f.neu {
			t.Fatalf("%s: Location %q, erwartet %q", f.alt, ort, f.neu)
		}

		// Mit Anmeldung dasselbe Ziel.
		if w := hole(t, h, f.alt, sitzung); w.Header().Get("Location") != f.neu {
			t.Fatalf("%s (angemeldet): Location %q", f.alt, w.Header().Get("Location"))
		}
	}

	// Formular-Sendungen an alte Pfade dürfen die Methode nicht verlieren —
	// darum 308 und nicht 301.
	w := sende(t, h, "/admin/dorfpflege/orte/neu", nil, sitzung)
	if w.Code != http.StatusPermanentRedirect || w.Header().Get("Location") != "/admin/mithelfen/orte/neu" {
		t.Fatalf("POST auf altem Pfad: %d %s", w.Code, w.Header().Get("Location"))
	}

	// Ohne Schrägstrich am Ende ebenfalls.
	if w := hole(t, h, "/admin/dorfpflege"); w.Code != http.StatusPermanentRedirect ||
		w.Header().Get("Location") != "/admin/mithelfen/" {
		t.Fatalf("/admin/dorfpflege: %d %s", w.Code, w.Header().Get("Location"))
	}
}

// Der neue Pfad liefert die Seite — inklusive der neuen Benennung.
func TestNeuerPfadMithelfen(t *testing.T) {
	_, h, _, sitzung := aufbau(t)

	w := hole(t, h, "/admin/mithelfen/", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Übersicht: %d", w.Code)
	}
	seite := w.Body.String()
	if !strings.Contains(seite, "Mithelfen") {
		t.Fatalf("Bereichsname fehlt: %s", seite)
	}
	if strings.Contains(strings.ToLower(seite), "dorfpflege") {
		t.Fatalf("alte Benennung steht noch in der Seite: %s", seite)
	}

	// Ohne Anmeldung geht es auf die Anmeldung zurück, nicht ins Leere.
	if w := hole(t, h, "/admin/mithelfen/"); w.Code != http.StatusSeeOther ||
		w.Header().Get("Location") != "/admin/" {
		t.Fatalf("Schutz greift nicht: %d %s", w.Code, w.Header().Get("Location"))
	}
}
