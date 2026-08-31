package vergabe

import (
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Eine Aufgabe außerhalb ihrer Jahreszeit ruht (#78). Ruhen heißt: Niemand
// wird gefragt und niemand bekommt eine Erinnerung — die Vergabe darf sie
// nicht für überfällig halten, nur weil sie „nicht grün" ist.

func TestRuhendeAufgabeWirdNichtVergeben(t *testing.T) {
	start := berlin(t, 2026, time.November, 16, 9, 0)
	d, e, _, s, task := aufbau(t, start)

	// Jäten von April bis September — im November ist daran nichts zu tun.
	task.Kind = model.TaskWeeding
	task.SeasonStartMonth, task.SeasonEndMonth = 4, 9
	if err := d.UpdateTask(&task); err != nil {
		t.Fatal(err)
	}
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -10))

	durchlauf(t, e)

	if v := vorgang(t, d, task.ID); v != nil {
		t.Fatalf("Vorgang eröffnet, obwohl die Aufgabe ruht: %+v", v)
	}
	if empf := s.empfaenger(model.NotifyRequest); len(empf) != 0 {
		t.Fatalf("Anfragen verschickt: %v", empf)
	}
}

// Innerhalb ihrer Jahreszeit läuft die Vergabe wie immer.
func TestAufgabeInDerJahreszeitWirdVergeben(t *testing.T) {
	start := berlin(t, 2026, time.June, 16, 9, 0)
	d, e, _, s, task := aufbau(t, start)

	task.Kind = model.TaskWeeding
	task.SeasonStartMonth, task.SeasonEndMonth = 4, 9
	if err := d.UpdateTask(&task); err != nil {
		t.Fatal(err)
	}
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -10))

	durchlauf(t, e)

	if v := vorgang(t, d, task.ID); v == nil {
		t.Fatal("kein Vorgang eröffnet, obwohl die Aufgabe fällig ist")
	}
	if empf := s.empfaenger(model.NotifyRequest); len(empf) != 1 || empf[0] != "anna" {
		t.Fatalf("Anfragen = %v, erwartet [anna]", empf)
	}
}
