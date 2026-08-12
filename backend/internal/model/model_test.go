package model

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

func task() CareTask {
	// Vorgabe „Unter den Eichen": 10 l, 1×/Woche, rot spätestens nach 14 Tagen.
	return CareTask{Kind: TaskWatering, IntervalDays: 7, RedAfterDays: 14, CreatedAt: t0}
}

func done(at time.Time) *Completion { return &Completion{DoneAt: at} }

func TestComputeStatus(t *testing.T) {
	cases := []struct {
		name   string
		last   *Completion
		now    time.Time
		factor float64
		want   Status
	}{
		{"frisch erledigt", done(t0), t0.Add(1 * time.Hour), 1, StatusGreen},
		{"kurz vor Intervall", done(t0), t0.Add(7*24*time.Hour - time.Minute), 1, StatusGreen},
		{"Intervall erreicht → gelb", done(t0), t0.Add(7 * 24 * time.Hour), 1, StatusYellow},
		{"nach 10 Tagen gelb", done(t0), t0.Add(10 * 24 * time.Hour), 1, StatusYellow},
		{"14 Tage → rot", done(t0), t0.Add(14 * 24 * time.Hour), 1, StatusRed},
		{"nach 20 Tagen rot", done(t0), t0.Add(20 * 24 * time.Hour), 1, StatusRed},
		{"nie erledigt: Basis = Anlegedatum", nil, t0.Add(15 * 24 * time.Hour), 1, StatusRed},
		{"Hitze (0.5): schon nach 4 Tagen gelb", done(t0), t0.Add(4 * 24 * time.Hour), 0.5, StatusYellow},
		{"Hitze (0.5): nach 7 Tagen rot", done(t0), t0.Add(7 * 24 * time.Hour), 0.5, StatusRed},
		{"Faktor 0 fällt auf 1 zurück", done(t0), t0.Add(1 * time.Hour), 0, StatusGreen},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, _ := ComputeStatus(task(), c.last, c.now, c.factor)
			if got != c.want {
				t.Fatalf("ComputeStatus = %s, erwartet %s", got, c.want)
			}
		})
	}
}

func TestComputeStatusThresholds(t *testing.T) {
	_, dueAt, redAt := ComputeStatus(task(), done(t0), t0, 1)
	if want := t0.Add(7 * 24 * time.Hour); !dueAt.Equal(want) {
		t.Errorf("dueAt = %v, erwartet %v", dueAt, want)
	}
	if want := t0.Add(14 * 24 * time.Hour); !redAt.Equal(want) {
		t.Errorf("redAt = %v, erwartet %v", redAt, want)
	}
}

func TestWorst(t *testing.T) {
	if Worst(StatusGreen, StatusYellow) != StatusYellow ||
		Worst(StatusRed, StatusYellow) != StatusRed ||
		Worst(StatusGreen, StatusGreen) != StatusGreen {
		t.Fatal("Worst-Reihenfolge falsch")
	}
}

func TestDisplayName(t *testing.T) {
	if (CareTask{Kind: TaskWatering}).DisplayName() != "Gießen" {
		t.Error("Gießen-Name falsch")
	}
	if (CareTask{Kind: TaskWeeding}).DisplayName() != "Jäten" {
		t.Error("Jäten-Name falsch")
	}
	if (CareTask{Kind: TaskOther, Title: "Hecke schneiden"}).DisplayName() != "Hecke schneiden" {
		t.Error("Titel hat keinen Vorrang")
	}
}
