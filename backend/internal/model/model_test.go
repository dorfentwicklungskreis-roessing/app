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

// --- Einmalige Aufgaben (#6) -------------------------------------------------

func zeitpunkt(t time.Time) *time.Time { return &t }

// einmalig: „einmal zum Bahnhof fahren" — kein Intervall, sondern ein
// Fälligkeitsdatum. Angelegt am 1. August, fällig am 20. August.
func einmalig() CareTask {
	return CareTask{
		Kind: TaskOther, Title: "Zum Bahnhof fahren", CreatedAt: t0,
		OneOff: true, DueDate: zeitpunkt(t0.Add(19 * 24 * time.Hour)),
	}
}

func TestComputeStatusEinmalig(t *testing.T) {
	faellig := t0.Add(19 * 24 * time.Hour)
	cases := []struct {
		name string
		last *Completion
		now  time.Time
		want Status
	}{
		{"lange vorher grün", nil, t0.Add(1 * time.Hour), StatusGreen},
		{"noch außerhalb der Vorwarnzeit", nil, faellig.Add(-OneOffLeadTime - time.Minute), StatusGreen},
		{"Vorwarnzeit erreicht → gelb", nil, faellig.Add(-OneOffLeadTime), StatusYellow},
		{"kurz vor dem Termin gelb", nil, faellig.Add(-time.Hour), StatusYellow},
		{"Termin erreicht → rot", nil, faellig, StatusRed},
		{"überfällig bleibt rot", nil, faellig.Add(30 * 24 * time.Hour), StatusRed},
		{"erledigt bleibt grün, auch nach dem Termin", done(faellig.Add(-time.Hour)),
			faellig.Add(90 * 24 * time.Hour), StatusGreen},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, _ := ComputeStatus(einmalig(), c.last, c.now, 1)
			if got != c.want {
				t.Fatalf("ComputeStatus = %s, erwartet %s", got, c.want)
			}
		})
	}
}

// Der Hitzefaktor beschleunigt Gießpläne. Ein einmaliger Termin ist ein
// Termin — er verschiebt sich nicht, weil es heiß ist.
func TestEinmaligIgnoriertHitzefaktor(t *testing.T) {
	faellig := t0.Add(19 * 24 * time.Hour)
	got, dueAt, redAt := ComputeStatus(einmalig(), nil, faellig.Add(-2*24*time.Hour), 0.5)
	if got != StatusYellow {
		t.Fatalf("Status = %s, erwartet %s", got, StatusYellow)
	}
	if !redAt.Equal(faellig) {
		t.Errorf("redAt = %v, erwartet den Termin %v", redAt, faellig)
	}
	if want := faellig.Add(-OneOffLeadTime); !dueAt.Equal(want) {
		t.Errorf("dueAt = %v, erwartet %v", dueAt, want)
	}
}

// Wer eine Aufgabe für übermorgen einstellt, hat keine drei Tage Vorlauf.
// Dann beginnt die Vorwarnung eben sofort — grün wäre gelogen.
func TestEinmaligKurzfristigStartetGelb(t *testing.T) {
	kurz := CareTask{Kind: TaskOther, CreatedAt: t0, OneOff: true,
		DueDate: zeitpunkt(t0.Add(12 * time.Hour))}
	if got, _, _ := ComputeStatus(kurz, nil, t0, 1); got != StatusYellow {
		t.Fatalf("Status = %s, erwartet %s", got, StatusYellow)
	}
}

// --- Was vor der App war -----------------------------------------------------

// Jeder Ort im Dorf hat eine Geschichte, die älter ist als die App. Wird eine
// Aufgabe heute angelegt, rechnet der Server sonst ab heute — und ein Beet,
// das im Juni zuletzt gejätet wurde, stünde bis Ende Oktober auf grün.
func TestZuletztErledigtIstDerStartpunkt(t *testing.T) {
	jetzt := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	juni := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)

	// Alle acht Wochen jäten, angelegt heute.
	aufgabe := CareTask{
		Kind: TaskWeeding, IntervalDays: 56, RedAfterDays: 77,
		Active: true, CreatedAt: jetzt,
	}

	// Ohne die Angabe: fällig erst in acht Wochen.
	if status, _, _ := ComputeStatus(aufgabe, nil, jetzt, 1); status != StatusGreen {
		t.Fatalf("ohne Angabe erwartet grün, ist %q", status)
	}

	// Mit „im Juni gemacht": längst überfällig.
	aufgabe.LastKnownDoneAt = &juni
	status, dueAt, _ := ComputeStatus(aufgabe, nil, jetzt, 1)
	if status != StatusRed {
		t.Fatalf("mit Juni erwartet rot, ist %q (fällig %s)", status, dueAt)
	}
	if want := juni.AddDate(0, 0, 56); !dueAt.Equal(want) {
		t.Errorf("fällig %s, erwartet %s", dueAt, want)
	}
}

// Die erste echte Meldung löst die Angabe ab — sonst bliebe eine einmal
// eingetragene Vergangenheit für immer der Bezugspunkt.
func TestEchteMeldungSchlaegtDieAngabe(t *testing.T) {
	jetzt := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	juni := time.Date(2026, 6, 15, 10, 0, 0, 0, time.UTC)
	gestern := jetzt.AddDate(0, 0, -1)

	aufgabe := CareTask{
		Kind: TaskWeeding, IntervalDays: 56, RedAfterDays: 77,
		Active: true, CreatedAt: juni, LastKnownDoneAt: &juni,
	}
	status, _, _ := ComputeStatus(aufgabe, &Completion{DoneAt: gestern}, jetzt, 1)
	if status != StatusGreen {
		t.Fatalf("nach der Meldung von gestern erwartet grün, ist %q", status)
	}
}
