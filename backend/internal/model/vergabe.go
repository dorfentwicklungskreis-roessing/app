package model

import (
	"errors"
	"sort"
	"time"
)

// Vergabe von Pflegeaufgaben: Wer sich für einen Ort angemeldet hat, wird
// gefragt, sobald dort etwas fällig wird — nacheinander, nicht alle
// gleichzeitig. Wer zusagt, hält die Aufgabe für eine Frist; verstreicht
// sie, geht es in der Warteschlange weiter.
//
// Die Regeln stehen bewusst hier im Modell und nicht in der App: Sie müssen
// für alle gleich gelten und sind ohne echtes Warten prüfbar.

// --- Anmeldung („Ich kümmere mich mit") -------------------------------------

// Signup ist die Anmeldung einer Person für einen Ort.
//
// Warum am Ort und nicht an der einzelnen Aufgabe: Gemeint ist immer „ich
// kümmere mich um den Kasten vor meiner Tür" — Aufgaben werden umbenannt,
// gelöscht und neu angelegt, eine Anmeldung an einer Aufgaben-ID wäre nach
// der ersten Umstellung des Gießplans verschwunden.
//
// TaskKind schränkt die Anmeldung zusätzlich auf eine Aufgabenart ein (leer
// = alle Aufgaben des Ortes). Das ist der eine Unterschied, der fachlich
// zählt: Gießen ist eine kurze Sache, die jede Woche ansteht, Jäten dauert
// und kommt selten. Wer nur die Gießkanne tragen mag, meldet sich für
// giessen an, ohne die Jät-Anfragen mitzubekommen.
type Signup struct {
	ID      int64  `json:"id"`
	UserSub string `json:"userSub"`
	PlaceID int64  `json:"placeId"`
	// TaskKind: leer = alle Aufgaben des Ortes.
	TaskKind  TaskKind  `json:"taskKind,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	// PlaceName und UserName werden für Anzeigen nachgeschlagen.
	PlaceName string `json:"placeName,omitempty"`
	UserName  string `json:"userName,omitempty"`
}

// Matches sagt, ob diese Anmeldung für die Aufgabe gilt.
func (s Signup) Matches(t CareTask) bool {
	return s.PlaceID == t.PlaceID && (s.TaskKind == "" || s.TaskKind == t.Kind)
}

// --- Vorgang ----------------------------------------------------------------

// AssignmentState ist der Stand eines Vergabe-Vorgangs.
type AssignmentState string

const (
	// AssignmentOpen: die Warteschlange läuft, es wird nacheinander gefragt.
	AssignmentOpen AssignmentState = "offen"
	// AssignmentClaimed: jemand hat zugesagt und hält den Vorgang.
	AssignmentClaimed AssignmentState = "uebernommen"
	// AssignmentBroadcast: die Liste ist durch, jetzt sind alle gefragt.
	AssignmentBroadcast AssignmentState = "rundruf"
	// AssignmentEnded: erledigt oder entfallen.
	AssignmentEnded AssignmentState = "beendet"
)

// Gründe für das Ende eines Vorgangs.
const (
	// EndDone: die Aufgabe wurde gemeldet — von wem auch immer.
	EndDone = "erledigt"
	// EndObsolete: die Aufgabe ist nicht mehr fällig, stillgelegt oder weg.
	EndObsolete = "entfallen"
)

// Assignment ist ein laufender Vergabe-Vorgang zu genau einer Aufgabe.
type Assignment struct {
	ID        int64           `json:"id"`
	TaskID    int64           `json:"taskId"`
	State     AssignmentState `json:"state"`
	CreatedAt time.Time       `json:"createdAt"`
	// NextOfferAt: wann die nächste Anfrage rausgehen darf (Staffelung,
	// Ruhezeiten). Nur für die Verwaltung interessant.
	NextOfferAt *time.Time `json:"nextOfferAt,omitempty"`
	// ClaimedBy ist die Kennung dessen, der zugesagt hat (leer = niemand).
	ClaimedBy     string     `json:"claimedBy,omitempty"`
	ClaimedByName string     `json:"claimedByName,omitempty"`
	ClaimedAt     *time.Time `json:"claimedAt,omitempty"`
	// ClaimedUntil: bis dahin hält die Zusage, danach verfällt sie.
	ClaimedUntil *time.Time `json:"claimedUntil,omitempty"`
	EndedAt      *time.Time `json:"endedAt,omitempty"`
	EndReason    string     `json:"endReason,omitempty"`
	// AskedCount ist die Zahl der bisher verschickten Anfragen.
	AskedCount int `json:"askedCount"`
}

// Active sagt, ob der Vorgang noch läuft.
func (a Assignment) Active() bool { return a.EndedAt == nil }

// --- Benachrichtigungen -----------------------------------------------------

// NotificationKind ist der Anlass einer Benachrichtigung.
type NotificationKind string

const (
	// NotifyRequest: „Du bist dran" — die persönliche Anfrage der Warteschlange.
	NotifyRequest NotificationKind = "anfrage"
	// NotifyBroadcast: Rundruf an alle Angemeldeten, nachdem die Liste durch ist.
	NotifyBroadcast NotificationKind = "rundruf"
	// NotifyClaimExpired: die eigene Zusage ist abgelaufen.
	NotifyClaimExpired NotificationKind = "zusage_abgelaufen"
	// NotifyClaimRevoked: die Verwaltung hat die eigene Zusage aufgehoben.
	NotifyClaimRevoked NotificationKind = "zusage_aufgehoben"
	// NotifyAssignmentDone: der Vorgang wurde von jemand anderem erledigt.
	NotifyAssignmentDone NotificationKind = "vorgang_beendet"
)

func ValidNotificationKind(k NotificationKind) bool {
	switch k {
	case NotifyRequest, NotifyBroadcast, NotifyClaimExpired, NotifyClaimRevoked, NotifyAssignmentDone:
		return true
	}
	return false
}

// IsRequest sagt, ob die Benachrichtigung eine Anfrage ist, auf die man
// zusagen kann. Alles andere ist ein Hinweis: einmal gelesen, erledigt.
func (k NotificationKind) IsRequest() bool {
	return k == NotifyRequest || k == NotifyBroadcast
}

// Gründe, aus denen eine Benachrichtigung geschlossen wird.
const (
	CloseDone      = "erledigt"
	CloseClaimed   = "uebernommen"
	CloseExpired   = "abgelaufen"
	CloseObsolete  = "entfallen"
	CloseConfirmed = "bestaetigt"
)

// Notification ist eine Zustellung an genau eine Person. Sie liegt in der
// Datenbank und wird von der App abgeholt; ein Push-Weg kommt später
// zusätzlich dazu (siehe Zusteller im Paket vergabe).
type Notification struct {
	ID           int64            `json:"id"`
	AssignmentID int64            `json:"assignmentId"`
	TaskID       int64            `json:"taskId"`
	PlaceID      int64            `json:"placeId"`
	UserSub      string           `json:"userSub"`
	Kind         NotificationKind `json:"kind"`
	CreatedAt    time.Time        `json:"createdAt"`
	// ExpiresAt ist die Frist: bei Anfragen der Vortritt (danach wird der
	// Nächste gefragt, zusagen darf man trotzdem weiter), bei einer
	// gehaltenen Zusage deren Ende.
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt,omitempty"`
	ClosedAt       *time.Time `json:"closedAt,omitempty"`
	ClosedReason   string     `json:"closedReason,omitempty"`

	// Angereichert für die Anzeige (nicht in der Datenbank):
	PlaceName string   `json:"placeName,omitempty"`
	TaskName  string   `json:"taskName,omitempty"`
	TaskKind  TaskKind `json:"taskKind,omitempty"`
	Title     string   `json:"title,omitempty"`
	Text      string   `json:"text,omitempty"`
}

// --- Regeln der Vergabe -----------------------------------------------------

// AssignmentRules bündelt die Stellschrauben. Sie liegen in den
// Einstellungen, damit sich Staffelung und Ruhezeiten ohne neue Version
// nachjustieren lassen.
type AssignmentRules struct {
	// OfferInterval ist der Abstand zwischen zwei Anfragen (Vorgabe 1 h).
	OfferInterval time.Duration `json:"-"`
	// ClaimDuration ist die Frist einer Zusage (Vorgabe 24 h).
	ClaimDuration time.Duration `json:"-"`
	// QuietFrom/QuietTo begrenzen die Ruhezeit in Ortszeit (Vorgabe 21–7).
	// Sind beide gleich, wird rund um die Uhr zugestellt.
	QuietFrom int `json:"quietFrom"`
	QuietTo   int `json:"quietTo"`
}

// MaxOfferInterval und MaxClaimDuration sind Sicherungen gegen Eingaben, die
// die Vergabe faktisch anhalten würden.
const (
	MaxOfferInterval = 24 * time.Hour
	MaxClaimDuration = 14 * 24 * time.Hour
)

func DefaultAssignmentRules() AssignmentRules {
	return AssignmentRules{
		OfferInterval: time.Hour,
		ClaimDuration: 24 * time.Hour,
		QuietFrom:     21,
		QuietTo:       7,
	}
}

func (r AssignmentRules) Validate() error {
	switch {
	case r.OfferInterval <= 0 || r.OfferInterval > MaxOfferInterval:
		return errors.New("der Abstand zwischen zwei Anfragen muss zwischen 1 Minute und 24 Stunden liegen")
	case r.ClaimDuration <= 0 || r.ClaimDuration > MaxClaimDuration:
		return errors.New("die Zusagefrist muss zwischen 1 Stunde und 14 Tagen liegen")
	case r.QuietFrom < 0 || r.QuietFrom > 23 || r.QuietTo < 0 || r.QuietTo > 23:
		return errors.New("die Ruhezeit muss mit vollen Stunden zwischen 0 und 23 angegeben werden")
	}
	return nil
}

// InQuietHours sagt, ob zu diesem Zeitpunkt Ruhe herrscht. Maßgeblich ist
// die Ortszeit des Dorfes — der Server läuft in UTC, geschlafen wird in
// Rössing.
func (r AssignmentRules) InQuietHours(t time.Time) bool {
	if r.QuietFrom == r.QuietTo {
		return false
	}
	h := t.In(Location()).Hour()
	if r.QuietFrom < r.QuietTo {
		return h >= r.QuietFrom && h < r.QuietTo
	}
	// Über Mitternacht (21 → 7).
	return h >= r.QuietFrom || h < r.QuietTo
}

// NextDelivery liefert den frühesten Zeitpunkt ab t, zu dem zugestellt
// werden darf. Fällt t in die Ruhezeit, wartet die Zustellung bis zum
// Morgen; die Staffelung pausiert also und läuft danach weiter.
func (r AssignmentRules) NextDelivery(t time.Time) time.Time {
	if !r.InQuietHours(t) {
		return t
	}
	ort := t.In(Location())
	// Ruheende am selben Tag; liegt es nicht in der Zukunft, der nächste Tag.
	// time.Date rechnet die Sommerzeit selbst mit — 07:00 gibt es an jedem
	// Tag genau einmal.
	ziel := time.Date(ort.Year(), ort.Month(), ort.Day(), r.QuietTo, 0, 0, 0, Location())
	if !ziel.After(ort) {
		morgen := ort.AddDate(0, 0, 1)
		ziel = time.Date(morgen.Year(), morgen.Month(), morgen.Day(), r.QuietTo, 0, 0, 0, Location())
	}
	return ziel
}

// --- Reihenfolge ------------------------------------------------------------

// Candidate ist eine angemeldete Person in der Warteschlange eines Vorgangs.
type Candidate struct {
	UserSub    string
	UserName   string
	SignedUpAt time.Time
	// LastDone: letzte Erledigung dieser Person (Nullzeit = noch nie).
	LastDone time.Time
	// LastAsked: letzte Anfrage an diese Person (Nullzeit = noch nie).
	LastAsked time.Time
}

// LastBothered ist der maßgebliche Zeitpunkt der Reihenfolge: der jüngere
// von „zuletzt erledigt" und „zuletzt gefragt". Wer gerade erst etwas
// gemacht hat oder gerade erst gefragt wurde, ist zuletzt an der Reihe.
func (c Candidate) LastBothered() time.Time {
	if c.LastAsked.After(c.LastDone) {
		return c.LastAsked
	}
	return c.LastDone
}

// OrderCandidates sortiert fair: Wer am längsten nichts erledigt hat bzw. am
// längsten nicht gefragt wurde, kommt zuerst. Wer noch nie an der Reihe war,
// steht ganz vorn. Bei Gleichstand entscheidet die ältere Anmeldung, danach
// die Kennung — damit dieselbe Lage immer dieselbe Reihenfolge ergibt.
// Die übergebene Liste bleibt unverändert.
func OrderCandidates(cs []Candidate) []Candidate {
	out := make([]Candidate, len(cs))
	copy(out, cs)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i].LastBothered(), out[j].LastBothered()
		if !a.Equal(b) {
			return a.Before(b)
		}
		if !out[i].SignedUpAt.Equal(out[j].SignedUpAt) {
			return out[i].SignedUpAt.Before(out[j].SignedUpAt)
		}
		return out[i].UserSub < out[j].UserSub
	})
	return out
}
