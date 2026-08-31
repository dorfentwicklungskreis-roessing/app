package admin

import (
	"net/http"
	"strings"
	"testing"
)

// Wer den Connector einrichtet, sitzt am Rechner in der Web-Verwaltung. Steht
// die Adresse dort nicht, bleibt nur: im Quelltext nachsehen oder jemanden
// fragen. Beides ist keine Bedienung — deshalb ist sie hier Gegenstand eines
// Tests und nicht bloß eines Textes.
func TestConnectorPageShowsTheAddress(t *testing.T) {
	_, h, _, sitzung := aufbau(t)

	w := hole(t, h, "/admin/connector/", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Seite antwortet mit %d", w.Code)
	}
	body := w.Body.String()

	// Die Adresse wird aus PUBLIC_URL abgeleitet (aufbau: localhost:8080).
	// Stünde sie fest im Template, zeigte die Entwicklung auf die Produktion.
	for _, erwartet := range []string{
		"http://localhost:8080/mcp",
		"http://localhost:8080/.well-known/oauth-protected-resource/mcp",
		"http://localhost:8080/oauth/register",
	} {
		if !strings.Contains(body, erwartet) {
			t.Errorf("Adresse fehlt auf der Seite: %s", erwartet)
		}
	}
	if strings.Contains(body, "xn--rssing-wxa.de") {
		t.Error("Die Seite zeigt eine fest eingetragene Adresse statt PUBLIC_URL")
	}

	// Die Frage, an der der Betreiber tatsächlich hängenblieb: fester oder
	// automatisch erzeugter OAuth-Client. Die Seite muss sie beantworten.
	if !strings.Contains(body, "automatisch erzeugten") {
		t.Error("Die Seite sagt nicht, welcher OAuth-Client gemeint ist")
	}
}

// Auffindbar heißt: von der Übersicht und aus dem Menü erreichbar. Eine Seite,
// die man nur kennt, wenn man ihre Adresse schon kennt, löst nichts.
func TestConnectorPageIsLinkedFromOverviewAndMenu(t *testing.T) {
	_, h, _, sitzung := aufbau(t)

	w := hole(t, h, "/admin/", sitzung)
	if strings.Count(w.Body.String(), `href="/admin/connector/"`) < 2 {
		t.Fatalf("Übersicht verlinkt den Bereich nicht aus Kachel und Menü:\n%s", w.Body.String())
	}
}

// Die Seite steht hinter derselben Schranke wie jeder andere Bereich. Das ist
// keine Geheimhaltung — die Adresse ist öffentlich —, sondern Gleichlauf:
// Wer nicht angemeldet ist, sieht die Verwaltung nicht.
func TestConnectorPageRequiresAdmin(t *testing.T) {
	_, h, _, _ := aufbau(t)

	w := hole(t, h, "/admin/connector/")
	if w.Code != http.StatusSeeOther {
		t.Fatalf("ohne Anmeldung: %d statt einer Weiterleitung", w.Code)
	}
}

// Ein Bereich ist auch ohne Schrägstrich erreichbar; sonst führt ein von Hand
// getipptes „/admin/connector" ins Leere.
func TestConnectorPathWithoutSlashRedirects(t *testing.T) {
	_, h, _, sitzung := aufbau(t)

	w := hole(t, h, "/admin/connector", sitzung)
	if w.Code != http.StatusMovedPermanently || w.Header().Get("Location") != "/admin/connector/" {
		t.Fatalf("keine Weiterleitung: %d → %q", w.Code, w.Header().Get("Location"))
	}
}
