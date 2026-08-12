package api

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// aufbauInaktiv liefert eine DB mit einem Ort und einer Aufgabe; beide lassen
// sich einzeln stilllegen.
func aufbauInaktiv(t *testing.T, ortAktiv, aufgabeAktiv bool) (*db.DB, model.Place, model.CareTask) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })

	p := model.Place{Name: "Unter den Eichen — Kasten 1", Kind: model.PlaceFlowerbox,
		Lat: 52.211, Lon: 9.87, Active: ortAktiv, CreatedAt: time.Now()}
	if err := d.InsertPlace(&p); err != nil {
		t.Fatal(err)
	}
	a := model.CareTask{PlaceID: p.ID, Kind: model.TaskWatering, IntervalDays: 7, RedAfterDays: 14,
		Active: aufgabeAktiv, CreatedAt: time.Now()}
	if err := d.InsertTask(&a); err != nil {
		t.Fatal(err)
	}
	return d, p, a
}

func statusVon(t *testing.T, err error) (int, string) {
	t.Helper()
	if err == nil {
		t.Fatal("Fehler erwartet, keiner gekommen")
	}
	var ce *CompletionError
	if !errors.As(err, &ce) {
		t.Fatalf("unerwarteter Fehlertyp: %v", err)
	}
	return ce.Status, ce.Message
}

// Eine stillgelegte Aufgabe (z.B. Kasten im Winter) nimmt keine Meldungen mehr
// an — weder aus der App noch über MCP oder die Verwaltung.
func TestMeldungAufInaktiverAufgabeAbgelehnt(t *testing.T) {
	d, _, aufgabe := aufbauInaktiv(t, true, false)
	erna := auth.User{Sub: "u1", Name: "Erna"}

	status, meldung := statusVon(t, mussFehlschlagen(t, d, aufgabe.ID, CompletionInput{}, erna))
	if status != http.StatusConflict {
		t.Errorf("Status %d, erwartet 409", status)
	}
	if !strings.Contains(meldung, "deaktiviert") {
		t.Errorf("Meldung nennt den Grund nicht: %q", meldung)
	}
}

// Gleiches gilt, wenn der ganze Ort stillgelegt ist — die Aufgabe daran mag
// noch aktiv sein, gepflegt wird dort trotzdem nicht.
func TestMeldungAnInaktivemOrtAbgelehnt(t *testing.T) {
	d, _, aufgabe := aufbauInaktiv(t, false, true)
	erna := auth.User{Sub: "u1", Name: "Erna"}

	status, meldung := statusVon(t, mussFehlschlagen(t, d, aufgabe.ID, CompletionInput{}, erna))
	if status != http.StatusConflict {
		t.Errorf("Status %d, erwartet 409", status)
	}
	if !strings.Contains(strings.ToLower(meldung), "ort") {
		t.Errorf("Meldung nennt den Ort nicht: %q", meldung)
	}
}

// Admins dürfen weiterhin nachtragen — dafür ist force da (z.B. eine
// telefonisch gemeldete Gießrunde von vor der Stilllegung).
func TestAdminDarfAufInaktiverAufgabeNachtragen(t *testing.T) {
	d, _, aufgabe := aufbauInaktiv(t, true, false)
	admin := auth.User{Sub: "a1", Name: "Levin", Roles: map[string]bool{"admin": true}}

	if _, err := CreateCompletion(d, time.Now(), aufgabe.ID, CompletionInput{Force: true}, admin); err != nil {
		t.Fatalf("Nachtrag abgelehnt: %v", err)
	}

	// Ohne force gilt die Sperre auch für Admins: sonst wäre die Stilllegung
	// nur eine Empfehlung.
	d2, _, aufgabe2 := aufbauInaktiv(t, true, false)
	status, _ := statusVon(t, mussFehlschlagen(t, d2, aufgabe2.ID, CompletionInput{}, admin))
	if status != http.StatusConflict {
		t.Errorf("Status %d, erwartet 409", status)
	}
}

// Ein Mitglied kann sich nicht selbst freischalten.
func TestMitgliedKannStilllegungNichtUebergehen(t *testing.T) {
	d, _, aufgabe := aufbauInaktiv(t, true, false)
	erna := auth.User{Sub: "u1", Name: "Erna"}

	status, _ := statusVon(t, mussFehlschlagen(t, d, aufgabe.ID, CompletionInput{Force: true}, erna))
	if status != http.StatusForbidden {
		t.Errorf("Status %d, erwartet 403", status)
	}
}

// Aktiver Ort, aktive Aufgabe: alles wie bisher.
func TestMeldungAufAktiverAufgabeGehtWeiterhin(t *testing.T) {
	d, _, aufgabe := aufbauInaktiv(t, true, true)
	erna := auth.User{Sub: "u1", Name: "Erna"}
	if _, err := CreateCompletion(d, time.Now(), aufgabe.ID, CompletionInput{}, erna); err != nil {
		t.Fatalf("normale Meldung abgelehnt: %v", err)
	}
}

// Bewusste Festlegung: Eine Stilllegung nimmt niemandem, was er im Sommer
// geleistet hat. Die Rangliste rechnet weiter mit den Meldungen, die zu ihrer
// Zeit gültig waren — sonst würde die Winterpause die Saison-Auswertung
// leerräumen. Neue Meldungen sind ja bereits an der Quelle gesperrt.
func TestStilllegungNimmtKeinePunkteWeg(t *testing.T) {
	d, _, aufgabe := aufbauInaktiv(t, true, true)
	erna := auth.User{Sub: "u1", Name: "Erna"}
	jetzt := time.Now()
	if _, err := CreateCompletion(d, jetzt, aufgabe.ID, CompletionInput{}, erna); err != nil {
		t.Fatal(err)
	}

	vorher, err := AssembleLeaderboard(d, jetzt, model.PeriodAll, 10, erna)
	if err != nil {
		t.Fatal(err)
	}
	if len(vorher.Entries) != 1 || vorher.Entries[0].Completions != 1 {
		t.Fatalf("Ausgangslage stimmt nicht: %+v", vorher.Entries)
	}

	aufgabe.Active = false
	if err := d.UpdateTask(&aufgabe); err != nil {
		t.Fatal(err)
	}
	nachher, err := AssembleLeaderboard(d, jetzt, model.PeriodAll, 10, erna)
	if err != nil {
		t.Fatal(err)
	}
	if len(nachher.Entries) != 1 || nachher.Entries[0].Completions != 1 {
		t.Fatalf("die Stilllegung hat geleistete Arbeit gelöscht: %+v", nachher.Entries)
	}
}

// mussFehlschlagen ruft CreateCompletion auf und gibt den Fehler zurück.
func mussFehlschlagen(t *testing.T, d *db.DB, taskID int64, in CompletionInput, u auth.User) error {
	t.Helper()
	c, err := CreateCompletion(d, time.Now(), taskID, in, u)
	if err == nil {
		t.Fatalf("Meldung wurde angenommen: %+v", c)
	}
	return err
}
