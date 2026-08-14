package vergabe

import (
	"context"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Der Zeitgeber folgt dem Muster des Sicherungs-Zeitplans: im Hintergrund
// takten, beim Herunterfahren sauber stehen bleiben.

func TestFromEnvLiestDieUmgebung(t *testing.T) {
	t.Setenv("VERGABE", "")
	t.Setenv("VERGABE_TAKT", "")
	cfg, an := FromEnv()
	if !an || cfg.Takt != DefaultTakt {
		t.Fatalf("Vorgabe = %v, an=%v", cfg.Takt, an)
	}

	t.Setenv("VERGABE_TAKT", "10s")
	if cfg, _ := FromEnv(); cfg.Takt != 10*time.Second {
		t.Errorf("VERGABE_TAKT=10s → %v", cfg.Takt)
	}
	t.Setenv("VERGABE_TAKT", "quatsch")
	if cfg, _ := FromEnv(); cfg.Takt != DefaultTakt {
		t.Errorf("kaputter Takt → %v, erwartet Vorgabe", cfg.Takt)
	}

	t.Setenv("VERGABE", "off")
	if _, an := FromEnv(); an {
		t.Error("VERGABE=off schaltet den Zeitgeber nicht ab")
	}
}

func TestZeitgeberArbeitetUndHaeltAn(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, _, u, s, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))

	ctx, stop := context.WithCancel(context.Background())
	fertig := Start(ctx, d, Config{Now: u.jetzt, Zusteller: s, Takt: 5 * time.Millisecond})

	// Der erste Durchlauf passiert sofort beim Start.
	frist := time.Now().Add(2 * time.Second)
	for time.Now().Before(frist) {
		if a, _ := d.ActiveAssignment(task.ID); a != nil && a.AskedCount > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	a, err := d.ActiveAssignment(task.ID)
	if err != nil || a == nil || a.AskedCount == 0 {
		t.Fatalf("Zeitgeber hat nichts getan: %+v (%v)", a, err)
	}
	if empf := s.empfaenger(model.NotifyRequest); len(empf) != 1 || empf[0] != "anna" {
		t.Fatalf("zugestellt an %v", empf)
	}

	stop()
	select {
	case <-fertig:
	case <-time.After(2 * time.Second):
		t.Fatal("Zeitgeber hat sich nicht beendet")
	}
}
