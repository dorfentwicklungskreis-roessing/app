// Package model enthält das Domänenmodell der Dorf-App:
// Orte (Blumenkästen, Beete, …) mit Pflegeaufgaben (Gießen, Jäten, …)
// und der Ampel-Statuslogik (grün/gelb/rot).
package model

import "time"

// Status einer Pflegeaufgabe, abgeleitet aus der letzten Erledigung.
type Status string

const (
	StatusGreen  Status = "green"  // kürzlich erledigt
	StatusYellow Status = "yellow" // Intervall überschritten, bitte erledigen
	StatusRed    Status = "red"    // Rot-Schwelle überschritten, dringend
	// StatusDormant: außer Dienst — die Aufgabe fällt zu dieser Jahreszeit
	// gar nicht an (siehe season.go). Sie ist dann weder fällig noch gelb
	// noch rot, und niemand wird deswegen gefragt. Bewusst kein „grün":
	// Ein Beet, an dem im Winter nichts zu jäten ist, ist nicht in Ordnung
	// gebracht worden, sondern gerade nicht im Dienst.
	StatusDormant Status = "dormant"
)

// statusRank ordnet die Ampel: schlechter heißt größer. Ruhend steht unter
// grün — an einer ruhenden Aufgabe ist noch weniger zu tun als an einer
// frisch erledigten.
var statusRank = map[Status]int{StatusDormant: 0, StatusGreen: 1, StatusYellow: 2, StatusRed: 3}

// Worst liefert den schlechteren von zwei Status.
func Worst(a, b Status) Status {
	if statusRank[b] > statusRank[a] {
		return b
	}
	return a
}

// NeedsWork sagt, ob an der Aufgabe gerade wirklich etwas zu tun ist. Wer
// fragen, erinnern oder vergeben will, fragt das — nicht „ist sie nicht
// grün": Ruhend ist auch nicht grün, aber es ist nichts zu tun.
func (s Status) NeedsWork() bool { return s == StatusYellow || s == StatusRed }

// PlaceKind beschreibt die Art eines Ortes.
type PlaceKind string

const (
	PlaceFlowerbox PlaceKind = "blumenkasten"
	PlaceBed       PlaceKind = "beet"
	PlaceOther     PlaceKind = "sonstiges"
)

func ValidPlaceKind(k PlaceKind) bool {
	return k == PlaceFlowerbox || k == PlaceBed || k == PlaceOther
}

// TaskKind beschreibt die Art einer Pflegeaufgabe.
type TaskKind string

const (
	TaskWatering TaskKind = "giessen"
	TaskWeeding  TaskKind = "jaeten"
	TaskOther    TaskKind = "sonstiges"
)

func ValidTaskKind(k TaskKind) bool {
	return k == TaskWatering || k == TaskWeeding || k == TaskOther
}

// Place ist ein Ort im Dorf, der gepflegt wird.
type Place struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Kind        PlaceKind `json:"kind"`
	Lat         float64   `json:"lat"`
	Lon         float64   `json:"lon"`
	Active      bool      `json:"active"`
	CreatedAt   time.Time `json:"createdAt"`
	// TraegerID: der Verein bzw. die Gruppe, der dieser Ort gehört (siehe
	// traeger.go). Über ihn hängt auch jede Aufgabe des Ortes an einem
	// Träger — es gibt bewusst nur diese eine Zuordnung, damit Ort und
	// Aufgabe nicht auseinanderlaufen können.
	TraegerID int64 `json:"traegerId"`
	// TraegerName ist ein Anzeigefeld (nicht gespeichert).
	TraegerName string `json:"traegerName,omitempty"`
}

// CareTask ist eine wiederkehrende Pflegeaufgabe an einem Ort.
type CareTask struct {
	ID      int64    `json:"id"`
	PlaceID int64    `json:"placeId"`
	Kind    TaskKind `json:"kind"`
	// Title: optionaler Anzeigename, v.a. für Kind "sonstiges".
	Title string `json:"title,omitempty"`
	// Liters: Wassermenge pro Gießvorgang (nur für Kind "giessen").
	Liters *float64 `json:"liters,omitempty"`
	// IntervalDays: Soll-Intervall in Tagen. Danach wird die Aufgabe gelb.
	// Bei einmaligen Aufgaben ohne Bedeutung (0).
	IntervalDays float64 `json:"intervalDays"`
	// RedAfterDays: Nach so vielen Tagen ohne Erledigung wird sie rot.
	// Bei einmaligen Aufgaben ohne Bedeutung (0).
	RedAfterDays float64 `json:"redAfterDays"`
	// SeasonStartMonth/SeasonEndMonth: der Teil des Jahres, in dem die
	// Aufgabe überhaupt anfällt — jeweils ein ganzer Monat, einschließlich
	// (4 und 9 heißt 1. April bis 30. September). 0/0 heißt ganzjährig und
	// ist die Vorbelegung des gesamten Bestands. Anfang > Ende geht über den
	// Jahreswechsel (11/2 = November bis Februar). Siehe season.go.
	SeasonStartMonth int `json:"seasonStartMonth,omitempty"`
	SeasonEndMonth   int `json:"seasonEndMonth,omitempty"`
	// OneOff: einmalige Aufgabe („einmal zum Bahnhof fahren") statt eines
	// wiederkehrenden Pflegeplans. An die Stelle des Intervalls tritt dann
	// das Fälligkeitsdatum.
	OneOff bool `json:"oneOff"`
	// DueDate ist der Termin einer einmaligen Aufgabe („bis wann"). Nur bei
	// OneOff gesetzt.
	DueDate *time.Time `json:"dueDate,omitempty"`
	// RemoveWhenDone: nach dem Erledigen von Karte und Liste nehmen. Die
	// Aufgabe wird dabei nicht gelöscht, sondern abgeräumt (RemovedAt) —
	// sonst verschwänden mit ihr die Erledigungen aus der Rangliste.
	RemoveWhenDone bool `json:"removeWhenDone"`
	// RemovedAt: abgeräumt, weil sie erledigt wurde. Solche Aufgaben tauchen
	// nirgends mehr auf, ihre Erledigungen zählen aber weiter.
	RemovedAt *time.Time `json:"removedAt,omitempty"`
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"createdAt"`
	// Sichtbarkeit: „oeffentlich“ (jeder im Dorf) oder „nur_mitglieder“
	// (ausschließlich Mitglieder des Trägers). Eine interne Aufgabe darf
	// außerhalb auf keinem Weg erscheinen — siehe Zugriff.SiehtAufgabe.
	Sichtbarkeit TaskSichtbarkeit `json:"sichtbarkeit"`
	// BefaehigungID: verlangte Einweisung (0 = keine). Ohne sie kann
	// niemand zusagen; durchgesetzt wird das serverseitig.
	BefaehigungID int64 `json:"befaehigungId,omitempty"`
	// BefaehigungName ist ein Anzeigefeld (nicht gespeichert).
	BefaehigungName string `json:"befaehigungName,omitempty"`
}

// Intern sagt, ob die Aufgabe den Träger nicht verlassen darf.
func (t CareTask) Intern() bool { return t.Sichtbarkeit == AufgabeNurMitglieder }

// OneOffLeadTime ist die Vorwarnzeit einer einmaligen Aufgabe: So lange vor
// dem Termin wird sie gelb. Drei Tage sind im Dorf die richtige Größe —
// genug, um es einzuplanen, nicht so früh, dass die Karte dauernd gelb ist.
const OneOffLeadTime = 3 * 24 * time.Hour

// Faellig liefert den Termin einer einmaligen Aufgabe. Fehlt er (kann bei
// Bestandsdaten nicht vorkommen, bei fehlerhaften Eingaben schon), gilt das
// Anlegedatum — dann ist die Aufgabe sofort fällig statt nie.
func (t CareTask) Faellig() time.Time {
	if t.DueDate != nil {
		return *t.DueDate
	}
	return t.CreatedAt
}

// Abgeraeumt sagt, ob die Aufgabe erledigt und weggeräumt ist.
func (t CareTask) Abgeraeumt() bool { return t.RemovedAt != nil }

// DisplayName liefert einen menschenlesbaren Namen der Aufgabe.
func (t CareTask) DisplayName() string {
	if t.Title != "" {
		return t.Title
	}
	switch t.Kind {
	case TaskWatering:
		return "Gießen"
	case TaskWeeding:
		return "Jäten"
	default:
		return "Pflege"
	}
}

// Completion ist eine Erledigungs-Meldung eines Nutzers.
type Completion struct {
	ID       int64     `json:"id"`
	TaskID   int64     `json:"taskId"`
	UserSub  string    `json:"userSub"`
	UserName string    `json:"userName"`
	Liters   *float64  `json:"liters,omitempty"`
	Note     string    `json:"note,omitempty"`
	DoneAt   time.Time `json:"doneAt"`
	// Forced: von einem Admin trotz laufender Sperrfrist eingetragen
	// (z.B. telefonisch gemeldeter Nachtrag).
	Forced bool `json:"forced,omitempty"`
}

// TaskWithStatus ist die API-Sicht auf eine Aufgabe inklusive Zustand.
type TaskWithStatus struct {
	CareTask
	Status         Status      `json:"status"`
	LastCompletion *Completion `json:"lastCompletion,omitempty"`
	// DueAt: Zeitpunkt, ab dem die Aufgabe gelb wird (Soll-Termin).
	DueAt time.Time `json:"dueAt"`
	// RedAt: Zeitpunkt, ab dem die Aufgabe rot wird.
	RedAt time.Time `json:"redAt"`
	// LockedUntil: bis dahin greift der Spielschutz (siehe cooldown.go).
	// Fehlt, wenn gerade gemeldet werden darf.
	LockedUntil *time.Time `json:"lockedUntil,omitempty"`
	// Assignment: laufender Vergabe-Vorgang (siehe internal/vergabe).
	// Fehlt, solange niemand gefragt wurde.
	Assignment *Assignment `json:"assignment,omitempty"`
	// SignupCount: wie viele Personen sich hier zum Mithelfen angemeldet haben.
	SignupCount int `json:"signupCount"`
	// SignedUp: ob die abrufende Person selbst angemeldet ist.
	SignedUp bool `json:"signedUp"`
}

// PlaceWithStatus ist die API-Sicht auf einen Ort inklusive Aufgaben.
type PlaceWithStatus struct {
	Place
	Tasks []TaskWithStatus `json:"tasks"`
	// Status: schlechtester Status aller aktiven Aufgaben des Ortes.
	Status Status `json:"status"`
}

// ComputeStatus leitet den Status einer Aufgabe ab.
//
// Basis ist die letzte Erledigung; gab es noch keine, zählt das Anlegedatum
// der Aufgabe als Startpunkt. factor skaliert die Schwellen: 1.0 = normal,
// 0.5 = Hitzewelle (doppelt so schnell gelb/rot). Der Aufrufer entscheidet,
// für welche Aufgabenarten der Faktor gilt (typisch: nur Gießen).
//
// Hat die Aufgabe eine Jahreszeit (season.go), gilt sie nur in deren
// Fenster. Außerhalb ruht sie: StatusDormant, und die beiden Zeitpunkte
// zeigen auf den Beginn des nächsten Fensters — der Tag, ab dem wieder etwas
// zu tun ist. Der Zähler läuft dabei bewusst weiter: Wer im September zuletzt
// gejätet hat, ist am 1. April fällig, ohne dass jemand etwas anfassen muss.
// Deshalb wird die Basis nicht zurückgesetzt, sondern nur die Anzeige der
// Schwellen auf den Fensterbeginn gezogen — vor dem 1. April kann nichts
// fällig gewesen sein.
func ComputeStatus(task CareTask, last *Completion, now time.Time, factor float64) (Status, time.Time, time.Time) {
	if task.OneOff {
		return statusEinmalig(task, last, now)
	}
	if factor <= 0 {
		factor = 1
	}
	base := task.CreatedAt
	if last != nil {
		base = last.DoneAt
	}
	dayHours := 24 * factor
	dueAt := base.Add(time.Duration(task.IntervalDays * dayHours * float64(time.Hour)))
	redAt := base.Add(time.Duration(task.RedAfterDays * dayHours * float64(time.Hour)))
	if season, ok := task.SeasonOf(); ok {
		von, _, drin := season.Window(now, Location())
		dueAt, redAt = laterOf(dueAt, von), laterOf(redAt, von)
		if !drin {
			// Der Hitzefaktor ändert daran nichts: Er staucht Schwellen,
			// er verlängert keine Jahreszeit.
			return StatusDormant, dueAt, redAt
		}
	}
	// RedAfterDays < IntervalDays wäre eine Fehlkonfiguration; rot gewinnt dann.
	switch {
	case !now.Before(redAt):
		return StatusRed, dueAt, redAt
	case !now.Before(dueAt):
		return StatusYellow, dueAt, redAt
	default:
		return StatusGreen, dueAt, redAt
	}
}

// laterOf liefert den späteren der beiden Zeitpunkte.
func laterOf(a, b time.Time) time.Time {
	if a.Before(b) {
		return b
	}
	return a
}

// PlaceStatus fasst die Aufgaben eines Ortes zu dem einen Punkt zusammen,
// der auf der Karte steht: der schlechteste Status seiner aktiven Aufgaben.
//
// Ruhen sie alle, ruht der Ort — ein Beet, an dem im Winter nichts zu jäten
// ist, meldet nicht „alles gut", sondern ist außer Dienst. Ein Ort ohne
// aktive Aufgabe bleibt grün, wie bisher.
func PlaceStatus(tasks []TaskWithStatus) Status {
	status := StatusGreen
	aktive, ruhende := 0, 0
	for _, t := range tasks {
		if !t.Active {
			continue
		}
		aktive++
		if t.Status == StatusDormant {
			ruhende++
			continue
		}
		status = Worst(status, t.Status)
	}
	if aktive > 0 && aktive == ruhende {
		return StatusDormant
	}
	return status
}

// statusEinmalig ist die Ampel einer einmaligen Aufgabe. An die Stelle des
// Intervalls tritt der Termin: rot ist sie, wenn er verstrichen ist, gelb in
// den letzten Tagen davor (OneOffLeadTime), sonst grün. Ist sie erledigt,
// bleibt sie grün — eine einmalige Aufgabe wird nicht wieder fällig.
//
// Der Hitzefaktor bleibt hier bewusst außen vor: Er beschleunigt Gießpläne,
// aber ein Termin ist ein Termin.
func statusEinmalig(task CareTask, last *Completion, now time.Time) (Status, time.Time, time.Time) {
	faellig := task.Faellig()
	gelbAb := faellig.Add(-OneOffLeadTime)
	// Kurzfristig eingestellt („bis morgen"): Dann beginnt die Vorwarnung
	// sofort, statt rückwirkend vor dem Anlegen zu liegen.
	if gelbAb.Before(task.CreatedAt) {
		gelbAb = task.CreatedAt
	}
	if last != nil {
		return StatusGreen, gelbAb, faellig
	}
	switch {
	case !now.Before(faellig):
		return StatusRed, gelbAb, faellig
	case !now.Before(gelbAb):
		return StatusYellow, gelbAb, faellig
	default:
		return StatusGreen, gelbAb, faellig
	}
}
