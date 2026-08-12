package model

import (
	"testing"
	"time"
)

// Spielschutz: nach einer Erledigung ist dieselbe Aufgabe eine Weile gesperrt.
// Sonst lässt sich die Rangliste durch mehrfaches Tippen aufblähen — und eine
// zweite Gießmeldung eine Stunde später ist ohnehin unsinnig.

func TestCooldown(t *testing.T) {
	task := func(interval float64) CareTask {
		return CareTask{Kind: TaskWatering, IntervalDays: interval, RedAfterDays: interval * 2}
	}
	cases := []struct {
		name     string
		task     CareTask
		factor   float64
		erwartet time.Duration
	}{
		{"Hälfte des Intervalls", task(7), 1, 84 * time.Hour},            // 3,5 Tage
		{"Untergrenze 12 Stunden", task(0.5), 1, 12 * time.Hour},         // 6h wäre zu kurz
		{"Hitze halbiert die Sperre", task(7), 0.5, 42 * time.Hour},      // Sommer: öfter gießen
		{"Obergrenze ist das Intervall", task(7), 4, 7 * 24 * time.Hour}, // nie länger als das Soll
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CooldownFor(c.task, c.factor); got != c.erwartet {
				t.Errorf("CooldownFor = %v, erwartet %v", got, c.erwartet)
			}
		})
	}
}

func TestNextAllowed(t *testing.T) {
	basis := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)
	task := CareTask{Kind: TaskWatering, IntervalDays: 7, RedAfterDays: 14, CreatedAt: basis}

	t.Run("ohne vorherige Erledigung sofort erlaubt", func(t *testing.T) {
		if _, gesperrt := NextAllowed(task, nil, 1); gesperrt {
			t.Error("frische Aufgabe ist gesperrt")
		}
	})

	letzte := &Completion{DoneAt: basis}
	frei, gesperrt := NextAllowed(task, letzte, 1)
	if !gesperrt {
		t.Fatal("nach einer Erledigung muss gesperrt sein")
	}
	if want := basis.Add(84 * time.Hour); !frei.Equal(want) {
		t.Errorf("frei ab %v, erwartet %v", frei, want)
	}

	t.Run("Grenzfälle um die Schwelle", func(t *testing.T) {
		for _, c := range []struct {
			name    string
			zeit    time.Time
			erlaubt bool
		}{
			{"eine Sekunde später", basis.Add(time.Second), false},
			{"kurz vor der Schwelle", frei.Add(-time.Second), false},
			{"genau an der Schwelle", frei, true},
			{"danach", frei.Add(time.Second), true},
		} {
			if got := Blocked(task, letzte, c.zeit, 1); got == c.erlaubt {
				t.Errorf("%s: gesperrt=%v, erwartet gesperrt=%v", c.name, got, !c.erlaubt)
			}
		}
	})
}
