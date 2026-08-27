package model

import "time"

// Spielschutz: nach einer Erledigung bleibt dieselbe Aufgabe eine Weile
// gesperrt. Zwei Gründe:
//
//   - Fachlich ist eine zweite Gießmeldung eine Stunde später sinnlos.
//   - Seit es eine Rangliste gibt, ließe sie sich sonst durch mehrfaches
//     Tippen aufblähen.
//
// Die Sperre gilt für die Aufgabe, nicht für die Person — sonst könnten
// mehrere Leute denselben Kasten nacheinander „gießen".
const (
	// CooldownShare: Anteil des Soll-Intervalls, der gesperrt bleibt.
	CooldownShare = 0.5
	// MinCooldown: kürzeste Sperre, egal wie eng das Intervall ist.
	MinCooldown = 12 * time.Hour
	// MaxBackdate: so weit darf eine Meldung zurückdatiert werden.
	//
	// Drei Tage decken den ehrlichen Fall ab: am Samstag gegossen, erst
	// heute daran gedacht. Zwei Wochen decken ihn auch ab — sie erlauben
	// aber obendrein, die Rangliste nachträglich umzuschreiben. Gewertet
	// wird eine Meldung nämlich nur, wenn die Aufgabe zu ihrem Zeitpunkt
	// nicht frisch erledigt war; wer den Zeitpunkt frei in ein zwei Wochen
	// breites Fenster legen darf, sucht sich einen, an dem sie zählt.
	MaxBackdate = 3 * 24 * time.Hour
	// MaxBackdateAdmin: die Verwaltung darf weiter zurück. Sie trägt fremde
	// Meldungen nach — telefonisch gemeldeter Vollzug aus dem Urlaub, Zettel
	// vom Sommerfest —, und das unter ihrem Namen und sichtbar in der
	// Historie der Aufgabe.
	MaxBackdateAdmin = 14 * 24 * time.Hour
)

// CooldownFor liefert die Sperrfrist einer Aufgabe. factor ist der
// Hitzefaktor (kleiner = häufiger gießen), er verkürzt die Sperre
// entsprechend. Obergrenze ist immer das volle Soll-Intervall, damit eine
// Fehlkonfiguration die Aufgabe nicht dauerhaft blockiert.
func CooldownFor(task CareTask, factor float64) time.Duration {
	if factor <= 0 {
		factor = 1
	}
	tag := 24 * float64(time.Hour)
	sperre := time.Duration(task.IntervalDays * factor * CooldownShare * tag)
	if voll := time.Duration(task.IntervalDays * tag); sperre > voll {
		sperre = voll
	}
	if sperre < MinCooldown {
		sperre = MinCooldown
	}
	return sperre
}

// NextAllowed liefert den Zeitpunkt, ab dem wieder gemeldet werden darf.
// Der zweite Rückgabewert ist false, wenn es noch gar keine Erledigung gibt.
func NextAllowed(task CareTask, last *Completion, factor float64) (time.Time, bool) {
	if last == nil {
		return time.Time{}, false
	}
	return last.DoneAt.Add(CooldownFor(task, factor)), true
}

// Blocked sagt, ob zum Zeitpunkt now noch die Sperre greift.
func Blocked(task CareTask, last *Completion, now time.Time, factor float64) bool {
	frei, gesperrt := NextAllowed(task, last, factor)
	return gesperrt && now.Before(frei)
}
