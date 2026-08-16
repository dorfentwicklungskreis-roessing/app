package db

import (
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Migrationstest für die Umstellung auf Träger.
//
// Im Cluster liegt eine gefüllte SQLite-Datei mit den echten Blumenkästen
// „Unter den Eichen“ und den gemeldeten Erledigungen des Dorfes. Sie muss die
// Umstellung heil überstehen: Kein Datensatz geht verloren, alle Orte wandern
// zum Dorfentwicklungskreis, und alles bleibt sichtbar wie zuvor.
//
// bestandsDatenbank() legt eine Datei im ALTEN Schema an (siehe
// migration_test.go) — also ohne jede Spalte, die es hier gibt.

func TestMigrationBestandWandertZumDorfentwicklungskreis(t *testing.T) {
	pfad := bestandsDatenbank(t)

	d, err := Open(pfad)
	if err != nil {
		t.Fatalf("Migration der Bestandsdatenbank: %v", err)
	}
	defer d.Close()

	// Der Dorfentwicklungskreis ist der Platzhalter-Träger, bis die
	// Dorfpflege offiziell zugestimmt hat.
	dek, err := d.GetTraegerBySchluessel(model.SchluesselDEK)
	if err != nil || dek == nil {
		t.Fatalf("Dorfentwicklungskreis wurde nicht angelegt: %v", err)
	}
	if !dek.Zugelassen() {
		t.Errorf("der erste Träger muss zugelassen sein, sonst ist das Dorf sofort blind: %+v", dek)
	}

	// Jeder Bestands-Ort gehört jetzt ihm.
	orte, err := d.ListPlaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(orte) != 1 {
		t.Fatalf("Ort verloren: %+v", orte)
	}
	if orte[0].TraegerID != dek.ID {
		t.Errorf("Bestands-Ort hat keinen Träger: %+v", orte[0])
	}
	if orte[0].Name != "Unter den Eichen — Kasten 1" {
		t.Errorf("Ortsname verändert: %q", orte[0].Name)
	}

	// Die Bestandsaufgabe „Blumengießen Unter den Eichen“ bleibt öffentlich
	// und ohne Befähigung — sonst verschwände sie über Nacht aus der App.
	task, err := d.GetTask(1)
	if err != nil {
		t.Fatal(err)
	}
	if task.Sichtbarkeit != model.AufgabeOeffentlich {
		t.Errorf("Bestandsaufgabe wurde intern: %+v", task)
	}
	if task.BefaehigungID != 0 {
		t.Errorf("Bestandsaufgabe verlangt plötzlich eine Einweisung: %+v", task)
	}
	if task.IntervalDays != 7 || task.RedAfterDays != 14 {
		t.Errorf("Gießplan verändert: %+v", task)
	}

	// Und alles, was daran hängt, ist noch da.
	cs, err := d.ListCompletions(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].UserName != "Erna Beispiel" {
		t.Fatalf("Erledigung verloren: %+v", cs)
	}
	a, err := d.ActiveAssignment(1)
	if err != nil || a == nil || a.ClaimedBy != "erna" {
		t.Fatalf("laufende Zusage verloren: %+v %v", a, err)
	}
}

// Der Server startet oft neu — die Zuordnung darf sich dabei nicht
// vervielfachen und kein zweiter Dorfentwicklungskreis entstehen.
func TestMigrationTraegerIstWiederholbar(t *testing.T) {
	pfad := bestandsDatenbank(t)
	for i := 0; i < 3; i++ {
		d, err := Open(pfad)
		if err != nil {
			t.Fatalf("Lauf %d: %v", i, err)
		}
		traeger, err := d.ListTraeger()
		if err != nil {
			t.Fatal(err)
		}
		if len(traeger) != 1 {
			t.Fatalf("Lauf %d: %d Träger statt 1: %+v", i, len(traeger), traeger)
		}
		d.Close()
	}
}

// Ein Ort, den jemand nach der Umstellung ohne Träger anlegt (etwa ein alter
// Client), darf nicht heimatlos herumliegen: Er landet beim
// Dorfentwicklungskreis, statt für niemanden sichtbar zu sein.
func TestOrtOhneTraegerLandetBeimPlatzhalter(t *testing.T) {
	pfad := bestandsDatenbank(t)
	d, err := Open(pfad)
	if err != nil {
		t.Fatal(err)
	}
	dek, err := d.GetTraegerBySchluessel(model.SchluesselDEK)
	if err != nil || dek == nil {
		t.Fatalf("Platzhalter fehlt: %v", err)
	}
	if _, err := d.sql.Exec(
		`INSERT INTO places(name,description,kind,lat,lon,active,created_at,traeger_id)
		 VALUES('Nachzügler','','sonstiges',52.2,9.87,1,'2026-08-16T10:00:00Z',0)`); err != nil {
		t.Fatal(err)
	}
	d.Close()

	d2, err := Open(pfad)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()
	orte, err := d2.ListPlaces()
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range orte {
		if o.TraegerID == 0 {
			t.Fatalf("heimatloser Ort nach dem Neustart: %+v", o)
		}
	}
}
