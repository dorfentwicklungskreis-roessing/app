package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
)

// Löschen des eigenen Kontos — `DELETE /api/v1/me`.
//
// Es gibt diesen Weg, weil es ihn geben muss: Apples Richtlinie 5.1.1 (v)
// verlangt von jeder App mit Konto einen Weg zum Löschen **in der App**, und
// die DSGVO (Art. 17) verlangt ihn ohnehin. Eine E-Mail-Adresse im
// Impressum genügt beidem nicht.
//
// Was gelöscht wird: Profil, Gerätekennungen (Push hört sofort auf),
// Helfer-Eintragungen, Benachrichtigungen und Befähigungsanträge. Laufende
// Zusagen werden freigegeben, damit kein Blumenkasten auf jemanden wartet,
// den es nicht mehr gibt.
//
// Was bleibt: die **Erledigungen**, aber anonym (siehe
// internal/db/konto.go). An ihnen hängen die Gesamtsummen des Dorfes und die
// Historie der Orte; sie zu löschen hieße, die Arbeit anderer zu
// verfälschen, sie unter Namen zu behalten hieße, das Löschen zu verweigern.
// Also bleibt die Zeile, der Name wird ersetzt und die Kennung entfernt.
//
// Was dieser Endpunkt **nicht** tut: das Konto in der **Rössing-ID**
// löschen. Es gehört Zitadel, und dieselbe Anmeldung dient auch anderen
// Anwendungen des Dorfes — die Dorf-App darf sie nicht mit wegräumen. Wer
// auch die Rössing-ID loswerden will, wendet sich an sie
// (https://id.xn--rssing-wxa.de). Die Antwort sagt das im Klartext, damit
// niemand glaubt, mit dem einen Knopf sei beides erledigt.
//
// Gelöscht wird ausschließlich das **eigene** Konto. Eine fremde Kennung im
// Rumpf ergibt 403 — auch für Admins; dieselbe Regel wie bei
// `PUT /api/v1/me/profile`. Ein Konto von außen zu löschen ist kein
// Selbstbedienungs-Vorgang; wer das braucht, macht es in der Verwaltung.

// KontoLoeschenInput ist der (optionale) Rumpf von DELETE /api/v1/me.
//
// UserSub dient wie beim Profil als Sicherung gegen Verwechslung: Schickt
// eine App eine fremde Kennung mit, wird abgelehnt, statt still das eigene
// Konto zu löschen. Ganz ohne Rumpf ist der Aufruf ebenfalls gültig.
type KontoLoeschenInput struct {
	UserSub string `json:"userSub"`
}

// LoeschErsatzname steht künftig an den Erledigungen der gelöschten Person.
const LoeschErsatzname = "Ehemaliges Mitglied"

// RoessingIdHinweis geht in jeder Löschantwort mit — in der App steht er
// wörtlich so vor dem Knopf.
const RoessingIdHinweis = "Deine Rössing-ID bleibt bestehen: Sie gehört nicht zur Dorf-App, " +
	"sondern ist die gemeinsame Anmeldung fürs Dorf. Wenn du auch sie löschen möchtest, " +
	"wende dich an die Rössing-ID (https://id.xn--rssing-wxa.de)."

func (s *Server) handleDeleteKonto(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())

	// Der Rumpf ist optional — DELETE ohne Inhalt ist der Normalfall.
	var in KontoLoeschenInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil && !errors.Is(err, io.EOF) {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if in.UserSub != "" && in.UserSub != u.Sub {
		writeErr(w, http.StatusForbidden, "es lässt sich nur das eigene Konto löschen")
		return
	}

	bilanz, err := s.DB.KontoLoeschen(u.Sub, LoeschErsatzname, s.now())
	if err != nil {
		writeInternal(w, r, err)
		return
	}

	// 200 mit Erklärung statt 204: Hier gibt es etwas zu sagen, und die App
	// zeigt es an, bevor sie sich abmeldet.
	writeJSON(w, http.StatusOK, map[string]any{
		"geloescht":     true,
		"bilanz":        bilanz,
		"erledigungen":  "Deine Meldungen bleiben anonym stehen („" + LoeschErsatzname + "“) — sonst stimmten die Zahlen des Dorfes nicht mehr.",
		"roessingId":    RoessingIdHinweis,
		"roessingIdUrl": "https://id.xn--rssing-wxa.de",
	})
}
