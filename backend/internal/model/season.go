package model

import (
	"fmt"
	"time"
)

// Jahreszeit einer wiederkehrenden Aufgabe.
//
// Das Beet vor dem Dorfgemeinschaftshaus wird von April bis September
// gejätet — im Dezember ist dort nichts zu tun. Eine Aufgabe, die nur ihr
// Intervall kennt, zählt trotzdem weiter und färbt die Karte rot für Arbeit,
// die es gar nicht gibt (#78).
//
// Gerechnet wird in ganzen Monaten und in Ortszeit des Dorfes. Ganze Monate
// reichen: Niemand sagt „ab dem 12. April", und Monatsgrenzen kennen weder
// Schaltjahr- noch Wochentagsprobleme. Ortszeit, weil den Zeitraum jemand in
// Rössing im Kopf hat und nicht in UTC.
//
// Die Jahreszeit hängt an der Aufgabe, nicht am Ort: Am selben Beet kann
// Jäten saisonal und „Müll aufsammeln" ganzjährig sein.
type Season struct {
	// Start und End sind einschließlich. Start > End heißt: Der Zeitraum
	// geht über den Jahreswechsel (November bis Februar).
	Start time.Month
	End   time.Month
}

// SeasonOf liefert die Jahreszeit der Aufgabe. Ohne Angabe (0/0) fällt sie
// das ganze Jahr an — so laufen alle Bestandsaufgaben unverändert weiter.
func (t CareTask) SeasonOf() (Season, bool) {
	if t.SeasonStartMonth == 0 || t.SeasonEndMonth == 0 {
		return Season{}, false
	}
	return Season{Start: time.Month(t.SeasonStartMonth), End: time.Month(t.SeasonEndMonth)}, true
}

// ValidSeasonMonths prüft ein Monatspaar, wie es von außen hereinkommt
// (REST, MCP, Web-Verwaltung, Chat). 0/0 bedeutet ganzjährig; alles andere
// braucht beide Monate im Bereich 1–12.
func ValidSeasonMonths(start, end int) error {
	if start == 0 && end == 0 {
		return nil
	}
	if start == 0 || end == 0 {
		return fmt.Errorf("die Jahreszeit braucht Anfangs- und Endmonat (oder beides leer für ganzjährig)")
	}
	if start < 1 || start > 12 || end < 1 || end > 12 {
		return fmt.Errorf("Anfangs- und Endmonat müssen zwischen 1 (Januar) und 12 (Dezember) liegen")
	}
	return nil
}

// NormalizeSeasonMonths macht aus „Januar bis Dezember" das ganzjährige 0/0.
// Sonst gäbe es zwei Schreibweisen für dieselbe Sache, und die Anzeige
// müsste beide kennen.
func NormalizeSeasonMonths(start, end int) (int, int) {
	if start == 1 && end == 12 {
		return 0, 0
	}
	return start, end
}

// Window liefert das Zeitfenster [von, bis), in dem now liegt, und ob now
// wirklich darin liegt. Liegt es außerhalb, ist von der Beginn des nächsten
// Fensters — der Zeitpunkt also, ab dem die Aufgabe wieder anfällt.
//
// Über den Jahreswechsel hinweg (Start > End) beginnt das Fenster im einen
// und endet im nächsten Jahr; deshalb werden drei Jahrgänge geprüft.
func (s Season) Window(now time.Time, loc *time.Location) (von time.Time, bis time.Time, drin bool) {
	if loc == nil {
		loc = time.UTC
	}
	n := now.In(loc)
	for _, jahr := range []int{n.Year() - 1, n.Year(), n.Year() + 1} {
		von, bis := s.fenster(jahr, loc)
		if !n.Before(von) && n.Before(bis) {
			return von, bis, true
		}
	}
	// Außerhalb: das nächste Fenster, das noch beginnt.
	for _, jahr := range []int{n.Year(), n.Year() + 1} {
		von, bis := s.fenster(jahr, loc)
		if n.Before(von) {
			return von, bis, false
		}
	}
	// Unerreichbar, solange Start und End gültige Monate sind.
	von, bis = s.fenster(n.Year()+1, loc)
	return von, bis, false
}

// fenster ist das Zeitfenster des Jahrgangs, der im Jahr jahr beginnt.
// Die Obergrenze ist ausschließlich: Der 1. Oktober 00:00 gehört nicht mehr
// zu „April bis September".
func (s Season) fenster(jahr int, loc *time.Location) (time.Time, time.Time) {
	von := time.Date(jahr, s.Start, 1, 0, 0, 0, 0, loc)
	endJahr := jahr
	if s.Start > s.End {
		endJahr++
	}
	// time.Date normalisiert Monat 13 zum Januar des Folgejahres — genau
	// das ist bei End = Dezember gemeint.
	bis := time.Date(endJahr, s.End+1, 1, 0, 0, 0, 0, loc)
	return von, bis
}

// Contains sagt, ob der Zeitpunkt in die Jahreszeit fällt.
func (s Season) Contains(now time.Time, loc *time.Location) bool {
	_, _, drin := s.Window(now, loc)
	return drin
}
