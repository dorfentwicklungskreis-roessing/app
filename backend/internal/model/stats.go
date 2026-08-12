package model

import (
	"fmt"
	"strings"
	"sync"
	"time"
	// Zeitzonendaten fest ins Binary einbetten: das Produktionsimage ist
	// distroless und hat kein /usr/share/zoneinfo.
	_ "time/tzdata"
)

// --- Zeiträume der Rangliste -------------------------------------------------

// Period ist ein Auswertungszeitraum der Rangliste.
type Period string

const (
	PeriodWeek   Period = "woche"  // laufende Kalenderwoche (ab Montag)
	PeriodMonth  Period = "monat"  // laufender Kalendermonat
	PeriodSeason Period = "saison" // Pflegesaison, siehe unten
	PeriodYear   Period = "jahr"   // laufendes Kalenderjahr
	PeriodAll    Period = "gesamt" // alles, seit es die App gibt
)

// DefaultPeriod ist der Zeitraum, wenn keiner angegeben wird.
const DefaultPeriod = PeriodSeason

// Die Pflegesaison läuft vom 1. März bis einschließlich 31. Oktober des
// laufenden Jahres — außerhalb davon wird im Dorf nicht gegossen.
const (
	seasonStartMonth = time.March
	seasonEndMonth   = time.November // exklusive Obergrenze
)

// ParsePeriod liest einen Zeitraum aus der Query („woche", „monat", …).
// Leer bedeutet DefaultPeriod.
func ParsePeriod(s string) (Period, error) {
	p := Period(strings.ToLower(strings.TrimSpace(s)))
	switch p {
	case "":
		return DefaultPeriod, nil
	case PeriodWeek, PeriodMonth, PeriodSeason, PeriodYear, PeriodAll:
		return p, nil
	}
	return "", fmt.Errorf("unbekannter Zeitraum %q (woche, monat, saison, jahr, gesamt)", s)
}

// PeriodRange liefert die Grenzen [from, to) eines Zeitraums — halboffen,
// damit eine Erledigung genau an der Grenze eindeutig zugeordnet ist.
// Gerechnet wird in Ortszeit (loc), nicht in UTC: der Monat beginnt um
// 00:00 Ortszeit, auch wenn dazwischen die Sommerzeit wechselt.
func PeriodRange(p Period, now time.Time, loc *time.Location) (time.Time, time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	n := now.In(loc)
	midnight := func(y int, m time.Month, d int) time.Time {
		return time.Date(y, m, d, 0, 0, 0, 0, loc)
	}
	switch p {
	case PeriodWeek:
		// Die Woche beginnt am Montag; Go zählt Sonntag als 0.
		weekday := (int(n.Weekday()) + 6) % 7
		from := midnight(n.Year(), n.Month(), n.Day()-weekday)
		return from, from.AddDate(0, 0, 7), nil
	case PeriodMonth:
		from := midnight(n.Year(), n.Month(), 1)
		return from, from.AddDate(0, 1, 0), nil
	case PeriodSeason:
		return midnight(n.Year(), seasonStartMonth, 1), midnight(n.Year(), seasonEndMonth, 1), nil
	case PeriodYear:
		from := midnight(n.Year(), time.January, 1)
		return from, from.AddDate(1, 0, 0), nil
	case PeriodAll:
		return time.Time{}, midnight(9999, time.January, 1), nil
	}
	return time.Time{}, time.Time{}, fmt.Errorf("unbekannter Zeitraum %q", p)
}

// Location liefert die Ortszeit des Dorfes. Fällt auf UTC zurück, falls die
// Zeitzonendatenbank wider Erwarten fehlt.
var Location = sync.OnceValue(func() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		return time.UTC
	}
	return loc
})

// --- Auszeichnungen ----------------------------------------------------------

// Schlüssel der Auszeichnungen. Die Regeln sind bewusst schlicht, positiv
// und ohne Punkte-Inflation — es gibt keine negativen Abzeichen.
const (
	// BadgeWateringCan: die meisten Gieß-Erledigungen im laufenden
	// Kalendermonat (bei Gleichstand bekommen ihn alle Beteiligten).
	BadgeWateringCan = "giesskanne"
	// BadgeEarlyBird: mindestens MinEarlyCompletions Erledigungen vor
	// EarlyBirdHour Uhr Ortszeit im ausgewerteten Zeitraum.
	BadgeEarlyBird = "fruehaufsteher"
	// BadgeRescuer: mindestens eine Erledigung an einer Aufgabe, die zu
	// diesem Zeitpunkt schon rot war (Rot-Schwelle überschritten).
	BadgeRescuer = "retter"
	// BadgeEndurance: Erledigungen in mindestens MinStreakWeeks
	// aufeinanderfolgenden Kalenderwochen.
	BadgeEndurance = "ausdauer"
)

const (
	// EarlyBirdHour: alles vor dieser Stunde (Ortszeit) gilt als früh.
	EarlyBirdHour = 8
	// MinEarlyCompletions: so viele frühe Erledigungen braucht das Abzeichen.
	MinEarlyCompletions = 3
	// MinStreakWeeks: so viele Wochen in Folge braucht das Ausdauer-Abzeichen.
	MinStreakWeeks = 4
)

// Badge ist eine Auszeichnung mit menschenlesbarer Erklärung.
type Badge struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

var badges = map[string]Badge{
	BadgeWateringCan: {BadgeWateringCan, "Gießkanne des Monats",
		"Die meisten Gießungen in diesem Monat."},
	BadgeEarlyBird: {BadgeEarlyBird, "Frühaufsteher",
		fmt.Sprintf("Mindestens %d Erledigungen vor %d Uhr morgens.", MinEarlyCompletions, EarlyBirdHour)},
	BadgeRescuer: {BadgeRescuer, "Retter",
		"Hat sich um einen Ort gekümmert, der schon rot war."},
	BadgeEndurance: {BadgeEndurance, "Ausdauer",
		fmt.Sprintf("In mindestens %d Wochen hintereinander im Einsatz gewesen.", MinStreakWeeks)},
}

// BadgeByKey liefert die Auszeichnung zu einem Schlüssel.
func BadgeByKey(key string) (Badge, bool) {
	b, ok := badges[key]
	return b, ok
}

// --- Rangliste ---------------------------------------------------------------

// LeaderboardEntry ist eine Zeile der Rangliste.
type LeaderboardEntry struct {
	// Rank: Platz in der Rangliste, 1-basiert. 0 = noch nichts gemeldet.
	Rank     int    `json:"rank"`
	UserSub  string `json:"userSub"`
	UserName string `json:"userName"`
	// Completions: Anzahl aller Erledigungen im Zeitraum.
	Completions int `json:"completions"`
	// ByKind: Anzahl je Aufgabenart (giessen, jaeten, sonstiges).
	ByKind map[string]int `json:"byKind"`
	// Liters: Summe der gemeldeten Liter.
	Liters         float64    `json:"liters"`
	LastCompletion *time.Time `json:"lastCompletion,omitempty"`
	Badges         []Badge    `json:"badges"`
}

// LeaderboardTotals sind die Gesamtsummen des Dorfes im Zeitraum.
type LeaderboardTotals struct {
	Completions  int            `json:"completions"`
	ByKind       map[string]int `json:"byKind"`
	Liters       float64        `json:"liters"`
	Participants int            `json:"participants"`
}

// Leaderboard ist die API-Sicht auf die Rangliste.
type Leaderboard struct {
	Period Period    `json:"period"`
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	// Entries: die vorderen Plätze (siehe limit).
	Entries []LeaderboardEntry `json:"entries"`
	Totals  LeaderboardTotals  `json:"totals"`
	// Me: der eigene Eintrag des Aufrufers — auch dann, wenn er nicht in
	// Entries steht. Rank 0 heißt: im Zeitraum noch nichts gemeldet.
	Me *LeaderboardEntry `json:"me,omitempty"`
}

// EmptyByKind liefert die Zählung aller Aufgabenarten mit 0, damit die
// Clients sich auf vorhandene Schlüssel verlassen können.
func EmptyByKind() map[string]int {
	return map[string]int{
		string(TaskWatering): 0,
		string(TaskWeeding):  0,
		string(TaskOther):    0,
	}
}
