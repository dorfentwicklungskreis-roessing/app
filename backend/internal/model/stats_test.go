package model

import (
	"testing"
	"time"
)

// berlin ist die Ortszeit des Dorfes — alle Zeiträume der Rangliste
// werden in Ortszeit abgegrenzt, nicht in UTC.
func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func TestParsePeriod(t *testing.T) {
	for in, want := range map[string]Period{
		"":        PeriodSeason, // Standard ist die Saison
		"woche":   PeriodWeek,
		"monat":   PeriodMonth,
		"saison":  PeriodSeason,
		"jahr":    PeriodYear,
		"gesamt":  PeriodAll,
		"SAISON ": PeriodSeason, // tolerant gegenüber Groß-/Kleinschreibung
	} {
		got, err := ParsePeriod(in)
		if err != nil {
			t.Errorf("ParsePeriod(%q): unerwarteter Fehler %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParsePeriod(%q) = %q, erwartet %q", in, got, want)
		}
	}
	if _, err := ParsePeriod("jahrzehnt"); err == nil {
		t.Error("ParsePeriod(\"jahrzehnt\") sollte einen Fehler liefern")
	}
}

func TestPeriodRange(t *testing.T) {
	loc := berlin(t)
	at := func(y int, m time.Month, d, h, min int) time.Time {
		return time.Date(y, m, d, h, min, 0, 0, loc)
	}

	cases := []struct {
		name       string
		period     Period
		now        time.Time
		from, to   time.Time
		fromIsZero bool
	}{
		{
			name: "Woche beginnt am Montag", period: PeriodWeek,
			now:  at(2026, time.August, 12, 12, 0), // Mittwoch
			from: at(2026, time.August, 10, 0, 0), to: at(2026, time.August, 17, 0, 0),
		},
		{
			name: "Woche am Montag um 0 Uhr beginnt genau jetzt", period: PeriodWeek,
			now:  at(2026, time.August, 10, 0, 0),
			from: at(2026, time.August, 10, 0, 0), to: at(2026, time.August, 17, 0, 0),
		},
		{
			name: "Woche am Sonntagabend gehört noch zur alten Woche", period: PeriodWeek,
			now:  at(2026, time.August, 16, 23, 59),
			from: at(2026, time.August, 10, 0, 0), to: at(2026, time.August, 17, 0, 0),
		},
		{
			name: "Monat", period: PeriodMonth,
			now:  at(2026, time.August, 12, 12, 0),
			from: at(2026, time.August, 1, 0, 0), to: at(2026, time.September, 1, 0, 0),
		},
		{
			name: "Monatswechsel: letzte Minute im August", period: PeriodMonth,
			now:  at(2026, time.August, 31, 23, 59),
			from: at(2026, time.August, 1, 0, 0), to: at(2026, time.September, 1, 0, 0),
		},
		{
			name: "Monatswechsel: erste Minute im September", period: PeriodMonth,
			now:  at(2026, time.September, 1, 0, 0),
			from: at(2026, time.September, 1, 0, 0), to: at(2026, time.October, 1, 0, 0),
		},
		{
			name: "Monat über den Jahreswechsel", period: PeriodMonth,
			now:  at(2026, time.December, 24, 18, 0),
			from: at(2026, time.December, 1, 0, 0), to: at(2027, time.January, 1, 0, 0),
		},
		{
			name: "Saison: 1. März bis 31. Oktober", period: PeriodSeason,
			now:  at(2026, time.August, 12, 12, 0),
			from: at(2026, time.March, 1, 0, 0), to: at(2026, time.November, 1, 0, 0),
		},
		{
			name: "Saison bleibt beim laufenden Jahr, auch im Winter", period: PeriodSeason,
			now:  at(2026, time.December, 24, 18, 0),
			from: at(2026, time.March, 1, 0, 0), to: at(2026, time.November, 1, 0, 0),
		},
		{
			name: "Jahr", period: PeriodYear,
			now:  at(2026, time.August, 12, 12, 0),
			from: at(2026, time.January, 1, 0, 0), to: at(2027, time.January, 1, 0, 0),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			from, to, err := PeriodRange(c.period, c.now, loc)
			if err != nil {
				t.Fatal(err)
			}
			if !from.Equal(c.from) {
				t.Errorf("from = %s, erwartet %s", from.Format(time.RFC3339), c.from.Format(time.RFC3339))
			}
			if !to.Equal(c.to) {
				t.Errorf("to = %s, erwartet %s", to.Format(time.RFC3339), c.to.Format(time.RFC3339))
			}
		})
	}

	t.Run("Gesamt umfasst alles", func(t *testing.T) {
		now := at(2026, time.August, 12, 12, 0)
		from, to, err := PeriodRange(PeriodAll, now, loc)
		if err != nil {
			t.Fatal(err)
		}
		if !from.Before(at(2000, time.January, 1, 0, 0)) {
			t.Errorf("from = %s, erwartet weit in der Vergangenheit", from)
		}
		if !to.After(now) {
			t.Errorf("to = %s, erwartet nach jetzt", to)
		}
	})

	t.Run("Saisonstart liegt in der Normalzeit (MEZ)", func(t *testing.T) {
		from, _, err := PeriodRange(PeriodSeason, at(2026, time.August, 12, 12, 0), loc)
		if err != nil {
			t.Fatal(err)
		}
		// 1. März 00:00 Ortszeit = 28. Februar 23:00 UTC (MEZ = UTC+1).
		want := time.Date(2026, time.February, 28, 23, 0, 0, 0, time.UTC)
		if !from.UTC().Equal(want) {
			t.Errorf("Saisonstart in UTC = %s, erwartet %s", from.UTC(), want)
		}
	})

	t.Run("Sommerzeit-Grenze im Monat Oktober", func(t *testing.T) {
		// Der Oktober beginnt in der Sommerzeit (MESZ = UTC+2) und endet
		// in der Normalzeit — die Grenzen müssen trotzdem 00:00 Ortszeit sein.
		from, to, err := PeriodRange(PeriodMonth, at(2026, time.October, 15, 12, 0), loc)
		if err != nil {
			t.Fatal(err)
		}
		if got := from.UTC(); !got.Equal(time.Date(2026, time.September, 30, 22, 0, 0, 0, time.UTC)) {
			t.Errorf("Oktoberbeginn in UTC = %s", got)
		}
		if got := to.UTC(); !got.Equal(time.Date(2026, time.October, 31, 23, 0, 0, 0, time.UTC)) {
			t.Errorf("Oktoberende in UTC = %s", got)
		}
	})

	t.Run("Unbekannter Zeitraum", func(t *testing.T) {
		if _, _, err := PeriodRange(Period("jahrzehnt"), time.Now(), loc); err == nil {
			t.Error("erwartet Fehler bei unbekanntem Zeitraum")
		}
	})
}
