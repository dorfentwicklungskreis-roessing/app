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
)

// Worst liefert den schlechteren von zwei Status.
func Worst(a, b Status) Status {
	rank := map[Status]int{StatusGreen: 0, StatusYellow: 1, StatusRed: 2}
	if rank[b] > rank[a] {
		return b
	}
	return a
}

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
	IntervalDays float64 `json:"intervalDays"`
	// RedAfterDays: Nach so vielen Tagen ohne Erledigung wird sie rot.
	RedAfterDays float64   `json:"redAfterDays"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"createdAt"`
}

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
func ComputeStatus(task CareTask, last *Completion, now time.Time, factor float64) (Status, time.Time, time.Time) {
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
