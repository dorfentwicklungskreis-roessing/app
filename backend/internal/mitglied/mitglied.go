// Package mitglied beantwortet die Frage „In welchen Vereinen ist diese
// Person, und mit welcher Rolle?“
//
// # Warum das nicht aus dem Token kommt
//
// Zitadel legt Rollen eines Projekts nur dann in ein Access-Token, wenn die
// anfragende Anwendung genau dieses Projekt als Empfänger anfordert
// („urn:zitadel:iam:org:project:id:<id>:aud“). Für jeden neuen Verein müsste
// die Android-App also einen weiteren Scope lernen, neu veröffentlicht werden
// und jedes Gerät sich neu anmelden. Das ist für ein Dorf, in dem alle paar
// Wochen eine Gruppe dazukommen soll, die falsche Bauweise.
//
// Stattdessen fragt das Backend die Rollenzuweisungen mit einem eigenen
// Dienst-Nutzer (Machine User) über die Management-API ab:
//
//	POST /management/v1/users/grants/_search   {"queries":[{"userIdQuery":{...}}]}
//
// Das Ergebnis wird kurz zwischengespeichert (Vorgabe 45 Sekunden). Der
// gewollte Vorteil: Eine neue Mitgliedschaft wirkt fast sofort — ohne Ab- und
// Anmelden, ohne App-Update.
//
// # Verhalten bei einem Zitadel-Ausfall
//
// Die Dorf-App darf nicht stehenbleiben, nur weil der Anmeldedienst hakt.
// Deshalb gilt:
//
//   - Es wird der letzte bekannte Stand aus dem Zwischenspeicher geliefert,
//     auch wenn er älter als die Frist ist. Er ist als „veraltet“ markiert.
//   - Mit einem veralteten Stand wird weiter GELESEN, aber nicht GESCHRIEBEN
//     (siehe model.Zugriff.DarfVerwalten). Ein zu lange gültiger Lesezugriff
//     ist heilbar; eine Änderung, die jemand nach seinem Austritt vornimmt,
//     nicht.
//   - Ist gar nichts zwischengespeichert, gibt es keine Mitgliedschaften.
//     Dann sieht man genau das, was ohnehin öffentlich ist — nie mehr.
//   - Die globale Betreiber-Rolle („admin“ im Projekt dorf-app) kommt aus dem
//     Token und ist davon unberührt. Der Betreiber bleibt handlungsfähig.
//
// Kurz: Ein Ausfall macht die App vorübergehend vorsichtiger, nie großzügiger.
package mitglied

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/clock"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Stand ist die Antwort: Rollen je Zitadel-Projekt, plus die Angabe, ob sie
// gesichert oder nur erinnert sind.
type Stand struct {
	Rollen model.Mitgliedschaften
	// Veraltet: Zitadel war nicht erreichbar, das hier ist der letzte
	// bekannte Stand (oder gar keiner).
	Veraltet bool
	// Geholt ist der Zeitpunkt der letzten erfolgreichen Abfrage.
	Geholt time.Time
}

// Quelle liefert die Mitgliedschaften einer Person.
//
// Bewusst ohne Fehlerwert: Ein Ausfall ist kein Sonderfall, den jeder
// Aufrufer einzeln behandeln müsste, sondern führt zu einem als „veraltet“
// gekennzeichneten Stand. Was das bedeutet, entscheidet model.Zugriff an
// genau einer Stelle.
type Quelle interface {
	Fuer(ctx context.Context, u auth.User) Stand
}

// Zugriff baut aus Nutzer und Quelle die Berechtigungssicht, mit der alle
// Sichtbarkeits- und Verwaltungsfragen beantwortet werden.
//
// Ist keine Quelle eingerichtet (Produktion ohne Dienst-Nutzer), gibt es
// schlicht keine Träger-Mitgliedschaften: Der Betreiber verwaltet dann alles,
// und alle anderen sehen die öffentlichen Aufgaben. Das ist ein bewusster
// Betriebszustand und kein Ausfall — deshalb NICHT „veraltet“.
func Zugriff(ctx context.Context, q Quelle, u auth.User) model.Zugriff {
	z := model.Zugriff{Sub: u.Sub, Betreiber: u.IsAdmin(), Mitglied: model.Mitgliedschaften{}}
	if q == nil {
		return z
	}
	stand := q.Fuer(ctx, u)
	if stand.Rollen != nil {
		z.Mitglied = stand.Rollen
	}
	z.Veraltet = stand.Veraltet
	return z
}

// --- Dev-Quelle -------------------------------------------------------------

// DevQuelle liest die Mitgliedschaften aus den Rollen des Tokens. Sie ist für
// AUTH_MODE=insecure-dev und für Tests gedacht — im Betrieb würde damit jede
// Rolle im Projekt dorf-app zur Vereinsmitgliedschaft.
//
// Schreibweise: „<projektId>@<rolle>“, z.B. „222@admin“.
type DevQuelle struct{}

func (DevQuelle) Fuer(_ context.Context, u auth.User) Stand {
	rollen := model.Mitgliedschaften{}
	for rolle := range u.Roles {
		projekt, name, ok := strings.Cut(rolle, "@")
		if !ok || projekt == "" || name == "" {
			continue
		}
		if rollen[projekt] == nil {
			rollen[projekt] = map[string]bool{}
		}
		rollen[projekt][name] = true
	}
	return Stand{Rollen: rollen, Geholt: clock.Now()}
}

// --- Zwischenspeicher -------------------------------------------------------

// DefaultTTL: so lange gilt eine Auskunft als frisch. Kurz genug, dass eine
// neue Mitgliedschaft praktisch sofort wirkt, lang genug, dass ein Blick auf
// die Karte nicht ein Dutzend Zitadel-Abfragen auslöst.
const DefaultTTL = 45 * time.Second

type eintrag struct {
	rollen model.Mitgliedschaften
	geholt time.Time
}

// speicher hält die letzten Auskünfte je Person.
type speicher struct {
	mu   sync.Mutex
	ttl  time.Duration
	nach map[string]eintrag
}

func neuerSpeicher(ttl time.Duration) *speicher {
	if ttl <= 0 {
		ttl = DefaultTTL
	}
	return &speicher{ttl: ttl, nach: map[string]eintrag{}}
}

// frisch liefert eine noch gültige Auskunft.
func (s *speicher) frisch(sub string, jetzt time.Time) (eintrag, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.nach[sub]
	if !ok || jetzt.Sub(e.geholt) > s.ttl {
		return eintrag{}, false
	}
	return e, true
}

// letzter liefert die letzte Auskunft, gleich wie alt.
func (s *speicher) letzter(sub string) (eintrag, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.nach[sub]
	return e, ok
}

func (s *speicher) merken(sub string, rollen model.Mitgliedschaften, jetzt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nach[sub] = eintrag{rollen: rollen, geholt: jetzt}
	// Der Speicher wächst mit der Zahl der Dorfbewohner — ein paar hundert
	// Einträge. Aufgeräumt wird trotzdem, damit ein langlaufender Prozess
	// keine Karteileichen von Ausgetretenen mitschleppt.
	if len(s.nach) > 2000 {
		for k, v := range s.nach {
			if jetzt.Sub(v.geholt) > time.Hour {
				delete(s.nach, k)
			}
		}
	}
}
