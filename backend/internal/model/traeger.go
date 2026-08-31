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
	// ParentID nennt das Dach, unter dem dieser Träger arbeitet — 0 heißt:
	// keins. So wird aus einem Arbeitskreis kein Nachbar seines Vereins.
	//
	// Genau **eine** Ebene, nicht beliebig tief (siehe ParentID-Prüfung in
	// der Ablage): Verein → Arbeitskreis ist das, was es im Dorf gibt. Tiefer
	// zu schachteln kostet Zyklusprüfungen und rekursive Abfragen für einen
	// Fall, den niemand hat.
	//
	// Eigenständig bleibt er trotzdem: eigenes Zitadel-Projekt, eigene
	// Mitglieder, eigene Admins. Wer den AK 2 verwalten darf, verwaltet
	// deshalb **nicht** den Verein darüber — und umgekehrt genauso wenig.
	// Das ist der ganze Grund, warum ein Arbeitskreis ein eigener Träger ist
	// und kein Feld am Verein.
	ParentID  int64     `json:"parentId,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// IstUnterTraeger sagt, ob dieser Träger unter einem Dach arbeitet.
func (t Traeger) IstUnterTraeger() bool { return t.ParentID != 0 }

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
	if t.ParentID < 0 {
		return errors.New("parentId muss ein Träger sein")
	}
	// Ein Träger unter sich selbst wäre kein Dach, sondern ein Kreis.
	if t.ParentID != 0 && t.ParentID == t.ID {
		return errors.New("ein Träger kann nicht sein eigenes Dach sein")
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

// --- Beitritte --------------------------------------------------------------

// Beitritt ist der Antrag auf Mitgliedschaft in einem Träger.
//
// Der Ablauf ist derselbe wie bei einer Befähigung — beantragen, freigeben
// oder ablehnen. Ein Unterschied ist aber wesentlich, und er ist der Grund,
// warum das hier ein eigener Typ ist und keine zweite Verwendung von
// BefaehigungsAntrag:
//
// Ein erteilter Befähigungsantrag IST die Befähigung. Ein erteilter Beitritt
// ist NICHT die Mitgliedschaft. Die steht in der Rössing-ID, und nirgends
// sonst — hier steht nur der Vorgang: wer wann gefragt hat und wer wann
// entschieden hat. Wäre es anders, hätte das Dorf zwei Wahrheiten darüber,
// wer zum Verein gehört, und irgendwann widersprächen sie sich.
//
// Deshalb wird ein Beitritt auch erst dann auf „erteilt“ gesetzt, wenn die
// Rollenzuweisung in Zitadel tatsächlich steht (siehe mitglied.Aufnehmer).
type Beitritt struct {
	ID        int64  `json:"id"`
	TraegerID int64  `json:"traegerId"`
	UserSub   string `json:"userSub"`
	UserName  string `json:"userName,omitempty"`
	// Status: beantragt, erteilt oder abgelehnt — dieselben drei Stände wie
	// beim Befähigungsantrag, damit im Dorf nicht zwei Vokabulare gelten.
	Status AntragStatus `json:"status"`
	// Begruendung schreibt die antragstellende Person („ich wohne neben dem
	// Beet und würde gern mitjäten“).
	Begruendung string `json:"begruendung,omitempty"`
	// Notiz schreibt der Träger-Admin bei der Entscheidung.
	Notiz          string     `json:"notiz,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	EntschiedenAm  *time.Time `json:"entschiedenAm,omitempty"`
	EntschiedenVon string     `json:"entschiedenVon,omitempty"`
	// TraegerName ist ein Anzeigefeld, aus dem Träger nachgeladen.
	TraegerName string `json:"traegerName,omitempty"`
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

// --- Beitritt ---------------------------------------------------------------

// BeitrittsHindernis nennt den Grund, warum diese Person diesem Träger
// gerade keinen Beitrittsantrag schicken kann — leer heißt: sie kann.
//
// Der Text ist deutsch, weil er genau so vor der Person landet. Er steht
// hier und nicht in den Handlern, damit REST, Web-Verwaltung und MCP
// dieselbe Antwort geben; eine zweite Prüfung daneben würde irgendwann
// abweichen.
func (z Zugriff) BeitrittsHindernis(t Traeger) string {
	if z.Sub == "" {
		return "Zum Beitreten musst du angemeldet sein."
	}
	if !t.Zugelassen() {
		return "Dieser Träger ist nicht zugelassen — beitreten kann man ihm noch nicht."
	}
	// Ohne Zitadel-Projekt gibt es nichts, worin man Mitglied sein könnte.
	// Eine Freigabe hätte hier nichts zurückzuschreiben.
	if t.ProjektID == "" {
		return "Dieser Träger hat in der Rössing-ID noch kein Projekt — " +
			"solange es keins gibt, hat er auch keine Mitglieder."
	}
	if z.Mitglied.IstMitglied(t.ProjektID) {
		return "Du gehörst schon dazu."
	}
	// Eine geschlossene Gruppe steht nicht im Verzeichnis. Wer sie nicht
	// findet, kann ihr auch nichts schicken — und wer ihre Kennung errät,
	// soll daraus nichts erfahren. Sie nimmt selbst auf, statt Anträge
	// entgegenzunehmen (siehe Aufnahme durch den Träger-Admin).
	if t.Sichtbarkeit != TraegerOffen {
		return "Diese Gruppe ist geschlossen: Wer dazugehören soll, wird von ihr " +
			"aufgenommen — Anträge nimmt sie nicht entgegen."
	}
	return ""
}

// DarfBeitrittBeantragen ist die Kurzform desselben.
func (z Zugriff) DarfBeitrittBeantragen(t Traeger) bool {
	return z.BeitrittsHindernis(t) == ""
}

// DarfBeitrittEntscheiden sagt, wer über einen Beitritt bestimmt: der
// Träger-Admin. Ausdrücklich nicht der Plattform-Betreiber allein — der lässt
// Träger zu, aber wer zu einem Verein gehört, entscheidet der Verein.
//
// Dass der Betreiber es technisch trotzdem kann (DarfVerwalten), ist keine
// zweite Regel, sondern dieselbe: Er verwaltet jeden Träger, solange keiner
// da ist, der es selbst tut.
func (z Zugriff) DarfBeitrittEntscheiden(t Traeger) bool { return z.DarfVerwalten(t) }

// --- Selbst entschieden ------------------------------------------------------

// SelbstEntschieden sagt, ob dieselbe Person entschieden hat, die den Antrag
// gestellt hat.
//
// Das ist erlaubt und für den Anfang richtig: In einem Dorfverein ist der
// Vorstand oft eine Person, und er ist ohnehin derjenige, der die Einweisung
// gibt. Ein erzwungenes Vier-Augen-Prinzip hieße dort, dass sich niemand
// eine Einweisung eintragen kann und die Aufgabe für alle liegen bleibt.
//
// Nachvollziehbar soll es trotzdem sein — deshalb steht es in der Liste
// dabei, statt still zu passieren (#34).
func (a BefaehigungsAntrag) SelbstEntschieden() bool {
	return a.EntschiedenVon != "" && a.EntschiedenVon == a.UserSub
}

// SelbstEntschieden — siehe BefaehigungsAntrag.SelbstEntschieden. Beim
// Beitritt wiegt es etwas schwerer: Wer sich selbst aufnimmt, verschafft sich
// eine Mitgliedschaft, nicht nur eine Einweisung. Verboten ist es trotzdem
// nicht, aus demselben Grund — sichtbar aber sehr wohl.
func (b Beitritt) SelbstEntschieden() bool {
	return b.EntschiedenVon != "" && b.EntschiedenVon == b.UserSub
}
