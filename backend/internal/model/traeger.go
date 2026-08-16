package model

import (
	"errors"
	"strings"
	"time"
)

// Träger: Verein oder Gruppe, die Aufgaben der Allmende kuratiert einstellt.
//
// Die Dorf-App vermittelt nicht zwischen Privatleuten. Sie verwaltet das,
// was dem Dorf gemeinsam gehört — und wer dafür zuständig ist, sind die
// bestehenden Vereine und Gruppen. Es entsteht deshalb keine Parallelstruktur
// neben ihnen: Jede Aufgabe hat einen Träger, und der Träger entscheidet.
//
// Ein Träger = ein Zitadel-Projekt mit genau zwei Rollen, „admin“ und
// „mitglied“. Die Rollennamen tragen bewusst KEIN Vereinspräfix: Sie sind im
// Projekt eindeutig, und ein Präfix müsste bei jedem neuen Verein überall
// nachgezogen werden.
//
// Die Mitgliedschaften kommen NICHT aus dem Token (siehe Mitgliedschaften).

// TraegerStatus ist der Zulassungsstand. Zulassen darf ausschließlich der
// Plattform-Betreiber — sonst könnte sich jede Gruppe selbst freischalten
// und im Namen des Dorfes auftreten.
type TraegerStatus string

const (
	// TraegerBeantragt: angelegt, aber noch nicht zugelassen — unsichtbar.
	TraegerBeantragt TraegerStatus = "beantragt"
	// TraegerZugelassen: vom Betreiber freigegeben.
	TraegerZugelassen TraegerStatus = "zugelassen"
	// TraegerGesperrt: war zugelassen, ist es nicht mehr. Die Daten bleiben,
	// sichtbar ist nichts mehr.
	TraegerGesperrt TraegerStatus = "gesperrt"
)

func ValidTraegerStatus(s TraegerStatus) bool {
	return s == TraegerBeantragt || s == TraegerZugelassen || s == TraegerGesperrt
}

// TraegerSichtbarkeit steuert, ob der Träger selbst im Verzeichnis steht.
// Sie sagt nichts über seine Aufgaben: Auch eine geschlossene Gruppe kann
// öffentliche Aufgaben ausschreiben (siehe TaskSichtbarkeit).
type TraegerSichtbarkeit string

const (
	// TraegerOffen: steht im Verzeichnis, jeder kann ihn finden.
	TraegerOffen TraegerSichtbarkeit = "offen"
	// TraegerGeschlossen: nur Mitglieder sehen ihn im Verzeichnis.
	TraegerGeschlossen TraegerSichtbarkeit = "geschlossen"
)

func ValidTraegerSichtbarkeit(s TraegerSichtbarkeit) bool {
	return s == TraegerOffen || s == TraegerGeschlossen
}

// Die beiden Rollen je Träger-Projekt in Zitadel.
const (
	RolleAdmin    = "admin"
	RolleMitglied = "mitglied"
)

// Traeger ist ein Verein oder eine Gruppe.
type Traeger struct {
	ID int64 `json:"id"`
	// Schluessel benennt fest eingebaute Träger (z.B. „dorfentwicklungskreis“).
	// Er macht die Migration wiederholbar und bleibt sonst leer.
	Schluessel string `json:"schluessel,omitempty"`
	// ProjektID ist die Zitadel-Projekt-ID. Leer heißt: noch nicht
	// eingerichtet — dann gibt es zu diesem Träger keine Mitglieder und
	// keine Admins (nur der Betreiber kann ihn pflegen).
	ProjektID    string              `json:"projektId"`
	Name         string              `json:"name"`
	Beschreibung string              `json:"beschreibung"`
	Status       TraegerStatus       `json:"status"`
	Sichtbarkeit TraegerSichtbarkeit `json:"sichtbarkeit"`
	CreatedAt    time.Time           `json:"createdAt"`
}

// Zugelassen sagt, ob der Träger überhaupt in Erscheinung treten darf.
func (t Traeger) Zugelassen() bool { return t.Status == TraegerZugelassen }

// Validate prüft einen Träger-Datensatz vor dem Speichern.
func (t *Traeger) Validate() error {
	t.Name = strings.TrimSpace(t.Name)
	t.ProjektID = strings.TrimSpace(t.ProjektID)
	if t.Name == "" {
		return errors.New("name fehlt")
	}
	if !ValidTraegerStatus(t.Status) {
		return errors.New("status muss beantragt, zugelassen oder gesperrt sein")
	}
	if !ValidTraegerSichtbarkeit(t.Sichtbarkeit) {
		return errors.New("sichtbarkeit muss offen oder geschlossen sein")
	}
	// Die Projekt-ID ist bei Zitadel eine Zahl; alles andere wäre ein
	// Tippfehler und liefe später still ins Leere.
	if t.ProjektID != "" && strings.Trim(t.ProjektID, "0123456789") != "" {
		return errors.New("projektId muss die numerische Zitadel-Projekt-ID sein")
	}
	return nil
}

// SchluesselDEK ist der fest eingebaute erste Träger. Der
// Dorfentwicklungskreis hält die bestehenden Aufgaben („Blumengießen Unter
// den Eichen“), bis die Dorfpflege offiziell zugestimmt hat und ihre Orte
// übernimmt.
const SchluesselDEK = "dorfentwicklungskreis"

// NameDEK ist sein Anzeigename bei der Erstanlage.
const NameDEK = "Dorfentwicklungskreis"

// --- Befähigungen -----------------------------------------------------------

// Befaehigung ist eine Einweisung, die ein Träger vergibt („Motorsense“,
// „Schlüssel Gerätehaus“). Eine Aufgabe kann eine voraussetzen; wer sie nicht
// hat, kann nicht zusagen.
//
// Warum als Befähigung der Person und nicht je Aufgabe: Wer einmal an der
// Motorsense eingewiesen wurde, ist es überall. Sonst müsste jede einzelne
// Wiese neu freigegeben werden.
type Befaehigung struct {
	ID           int64     `json:"id"`
	TraegerID    int64     `json:"traegerId"`
	Name         string    `json:"name"`
	Beschreibung string    `json:"beschreibung"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (b *Befaehigung) Validate() error {
	b.Name = strings.TrimSpace(b.Name)
	if b.Name == "" {
		return errors.New("name fehlt")
	}
	if b.TraegerID <= 0 {
		return errors.New("die Befähigung braucht einen Träger")
	}
	return nil
}

// AntragStatus ist der Stand eines Befähigungsantrags.
type AntragStatus string

const (
	AntragBeantragt AntragStatus = "beantragt"
	AntragErteilt   AntragStatus = "erteilt"
	AntragAbgelehnt AntragStatus = "abgelehnt"
)

func ValidAntragStatus(s AntragStatus) bool {
	return s == AntragBeantragt || s == AntragErteilt || s == AntragAbgelehnt
}

// BefaehigungsAntrag ist zugleich Antrag und Nachweis: Ein erteilter Antrag
// IST die Befähigung. Das erspart eine zweite Tabelle, in der derselbe
// Sachverhalt ein zweites Mal stünde.
type BefaehigungsAntrag struct {
	ID            int64        `json:"id"`
	BefaehigungID int64        `json:"befaehigungId"`
	UserSub       string       `json:"userSub"`
	UserName      string       `json:"userName,omitempty"`
	Status        AntragStatus `json:"status"`
	// Begruendung schreibt die antragstellende Person.
	Begruendung string `json:"begruendung,omitempty"`
	// Notiz schreibt der Träger-Admin bei der Entscheidung.
	Notiz          string     `json:"notiz,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	EntschiedenAm  *time.Time `json:"entschiedenAm,omitempty"`
	EntschiedenVon string     `json:"entschiedenVon,omitempty"`
	// Anzeigefelder, aus der Befähigung nachgeladen.
	BefaehigungName string `json:"befaehigungName,omitempty"`
	TraegerID       int64  `json:"traegerId,omitempty"`
}

// TaskSichtbarkeit steuert, wem eine Aufgabe angezeigt wird.
type TaskSichtbarkeit string

const (
	// AufgabeOeffentlich: jede und jeder im Dorf sieht sie.
	AufgabeOeffentlich TaskSichtbarkeit = "oeffentlich"
	// AufgabeNurMitglieder: ausschließlich Mitglieder des Trägers. Sie darf
	// außerhalb auf KEINEM Weg erscheinen — nicht in Listen, nicht auf der
	// Karte, nicht in der Rangliste und nicht als Push.
	AufgabeNurMitglieder TaskSichtbarkeit = "nur_mitglieder"
)

func ValidTaskSichtbarkeit(s TaskSichtbarkeit) bool {
	return s == AufgabeOeffentlich || s == AufgabeNurMitglieder
}

// --- Mitgliedschaften -------------------------------------------------------

// Mitgliedschaften bildet Zitadel-Projekt-ID → Rollen ab.
//
// Woher das kommt und warum nicht aus dem Token: Zitadel legt Rollen fremder
// Projekte nur dann ins Token, wenn die Anwendung je Projekt einen eigenen
// Scope anfordert („urn:zitadel:iam:org:project:id:<id>:aud“). Das hieße, bei
// jedem neuen Verein die App zu aktualisieren und alle Geräte neu anzumelden.
// Stattdessen fragt das Backend die Rollenzuweisungen mit einem Dienst-Nutzer
// über die Management-API ab (siehe internal/mitglied) — eine neue
// Mitgliedschaft wirkt damit sofort, ohne Ab- und Anmelden.
type Mitgliedschaften map[string]map[string]bool

// Hat sagt, ob jemand in diesem Projekt die genannte Rolle trägt.
//
// Eine leere Projekt-ID ergibt immer false: Ein Träger ohne eingerichtetes
// Zitadel-Projekt darf niemanden berechtigen, sonst wäre „noch nicht
// eingerichtet“ der Generalschlüssel.
func (m Mitgliedschaften) Hat(projektID, rolle string) bool {
	if projektID == "" || m == nil {
		return false
	}
	return m[projektID][rolle]
}

// IstMitglied gilt für beide Rollen: Wer verwaltet, gehört erst recht dazu.
// Sonst müsste jede Vorstandsperson zwei Rollen bekommen, und ein vergessenes
// „mitglied“ sperrte sie aus den eigenen internen Aufgaben aus.
func (m Mitgliedschaften) IstMitglied(projektID string) bool {
	return m.Hat(projektID, RolleMitglied) || m.Hat(projektID, RolleAdmin)
}

// Projekte liefert alle Projekt-IDs mit mindestens einer Rolle.
func (m Mitgliedschaften) Projekte() []string {
	out := make([]string, 0, len(m))
	for id, rollen := range m {
		if id == "" || len(rollen) == 0 {
			continue
		}
		out = append(out, id)
	}
	return out
}

// --- Zugriff ----------------------------------------------------------------

// Zugriff bündelt alles, was über die abrufende Person bekannt ist. Er ist
// die einzige Stelle, an der entschieden wird, wer was sieht und darf —
// REST, Web-Verwaltung, Vergabe und Rangliste fragen ihn.
type Zugriff struct {
	Sub string
	// Betreiber ist die globale admin-Rolle der Plattform („dorf-app“).
	// Sie kommt aus dem Token und ist von Zitadels API unabhängig.
	Betreiber bool
	// Mitglied sind die Rollen je Träger-Projekt.
	Mitglied Mitgliedschaften
	// Veraltet heißt: Die Mitgliedschaften stammen aus dem Zwischenspeicher,
	// weil Zitadel gerade nicht erreichbar war. Gelesen wird damit weiter,
	// geschrieben nicht (siehe DarfVerwalten).
	Veraltet bool
}

// SiehtTraeger sagt, ob der Träger für diese Person überhaupt existiert.
func (z Zugriff) SiehtTraeger(t Traeger) bool {
	if z.Betreiber {
		return true
	}
	if !t.Zugelassen() {
		return false
	}
	if t.Sichtbarkeit == TraegerOffen {
		return true
	}
	return z.Mitglied.IstMitglied(t.ProjektID)
}

// SiehtAufgabe ist die schärfste Regel des Systems: Eine Aufgabe mit
// „nur_mitglieder“ verlässt den Träger nicht.
//
// Umgekehrt hängt eine öffentliche Aufgabe NICHT an der Sichtbarkeit des
// Trägers: Eine geschlossene Gruppe darf sehr wohl öffentlich ausschreiben —
// sie steht dann nur selbst nicht im Verzeichnis.
func (z Zugriff) SiehtAufgabe(t Traeger, sicht TaskSichtbarkeit) bool {
	if z.Betreiber {
		return true
	}
	if !t.Zugelassen() {
		return false
	}
	if sicht == AufgabeOeffentlich {
		return true
	}
	return z.Mitglied.IstMitglied(t.ProjektID)
}

// SiehtOrt sagt, ob ein Ort mit diesen Aufgaben-Sichtbarkeiten erscheinen
// darf.
//
// Die Regel dreht sich um den verräterischen Fall: Ein Ort, der Aufgaben hat,
// von denen mir keine einzige gezeigt werden darf, verschwindet ganz. Eine
// leere Nadel auf der Karte verriete sonst, dass es dort intern etwas gibt.
//
// Ein Ort ganz OHNE Aufgaben verrät dagegen nichts — er ist frisch angelegt
// oder seine einmalige Aufgabe ist abgeräumt. Für ihn gilt schlicht die
// Sichtbarkeit seines Trägers.
func (z Zugriff) SiehtOrt(t Traeger, aufgaben []TaskSichtbarkeit) bool {
	if z.Betreiber {
		return true
	}
	if !t.Zugelassen() {
		return false
	}
	if z.Mitglied.IstMitglied(t.ProjektID) {
		return true
	}
	if len(aufgaben) == 0 {
		return z.SiehtTraeger(t)
	}
	for _, s := range aufgaben {
		if s == AufgabeOeffentlich {
			return true
		}
	}
	return false
}

// TraegerNameVerdeckt steht dort, wo der echte Name nicht hingehört.
//
// Bewusst kein leerer String: Die Aufgabe gehört ja jemandem, und wer sie
// sieht, soll erkennen, dass sie kuratiert ist und nicht von irgendwem
// stammt — nur eben nicht, von wem.
const TraegerNameVerdeckt = "Eine Gruppe aus dem Dorf"

// TraegerAnzeigeName liefert den Namen, der dieser Person gezeigt werden darf.
//
// Eine geschlossene Gruppe darf öffentlich ausschreiben, ohne sich dabei zu
// offenbaren: Sonst erführe über jede öffentliche Aufgabe die halbe Welt, dass
// es die Gruppe gibt und wie sie heißt — obwohl sie ausdrücklich nicht im
// Verzeichnis stehen wollte.
//
// Diese Funktion ist die einzige Stelle, an der ein Trägername für die Ausgabe
// bestimmt wird. Wer einen Namen anzeigen oder in eine Meldung schreiben will,
// holt ihn hier.
func (z Zugriff) TraegerAnzeigeName(t Traeger) string {
	if z.SiehtTraeger(t) {
		return t.Name
	}
	return TraegerNameVerdeckt
}

// DarfVerwalten sagt, ob diese Person Orte, Aufgaben und Befähigungen des
// Trägers anlegen, ändern und löschen darf.
//
// Zwei Einschränkungen gegenüber dem Lesen:
//   - Ein gesperrter oder noch nicht zugelassener Träger wird von niemandem
//     außer dem Betreiber gepflegt.
//   - Mit veraltetem Mitgliedschafts-Stand wird nicht geschrieben. Wer aus
//     einem Verein ausgetreten ist, soll dessen Allmende nicht ändern, bloß
//     weil Zitadel gerade nicht antwortet. Lesen bleibt erlaubt (siehe
//     SiehtAufgabe) — ein veralteter Lesezugriff ist heilbar, eine
//     unberechtigte Änderung nicht.
func (z Zugriff) DarfVerwalten(t Traeger) bool {
	if z.Betreiber {
		return true
	}
	if !t.Zugelassen() || z.Veraltet {
		return false
	}
	return z.Mitglied.Hat(t.ProjektID, RolleAdmin)
}

// DarfZusagen sagt, ob diese Person eine Aufgabe des Trägers übernehmen darf.
// Sichtbar heißt zusagbar — mit einer Ausnahme: Bei einer internen Aufgabe
// muss die Mitgliedschaft gesichert sein, nicht nur erinnert.
func (z Zugriff) DarfZusagen(t Traeger, sicht TaskSichtbarkeit) bool {
	if !z.SiehtAufgabe(t, sicht) {
		return false
	}
	if z.Betreiber || sicht == AufgabeOeffentlich {
		return true
	}
	return !z.Veraltet
}
