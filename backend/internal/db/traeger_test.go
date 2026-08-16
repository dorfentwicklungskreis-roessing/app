package db

import (
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Träger, Befähigungen und Anträge in der Datenbank.

func testTraeger(t *testing.T, d *DB, name, projektID string) model.Traeger {
	t.Helper()
	tr := model.Traeger{Name: name, ProjektID: projektID, Status: model.TraegerZugelassen,
		Sichtbarkeit: model.TraegerOffen, CreatedAt: time.Now().UTC()}
	if err := d.InsertTraeger(&tr); err != nil {
		t.Fatalf("Träger anlegen: %v", err)
	}
	return tr
}

func TestTraegerSpeichernUndLesen(t *testing.T) {
	d := testDB(t)
	tr := testTraeger(t, d, "Dorfpflege", "424242")

	gelesen, err := d.GetTraeger(tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gelesen.Name != "Dorfpflege" || gelesen.ProjektID != "424242" {
		t.Fatalf("falsch gespeichert: %+v", gelesen)
	}
	if !gelesen.Zugelassen() {
		t.Errorf("Status verloren: %+v", gelesen)
	}

	// Über die Projekt-ID muss er auffindbar sein — das ist der Weg, über
	// den eine Zitadel-Rollenzuweisung zum Träger wird.
	perProjekt, err := d.GetTraegerByProjekt("424242")
	if err != nil || perProjekt == nil || perProjekt.ID != tr.ID {
		t.Fatalf("nicht über die Projekt-ID gefunden: %v %v", perProjekt, err)
	}

	gelesen.Status = model.TraegerGesperrt
	gelesen.Sichtbarkeit = model.TraegerGeschlossen
	if err := d.UpdateTraeger(gelesen); err != nil {
		t.Fatal(err)
	}
	wieder, _ := d.GetTraeger(tr.ID)
	if wieder.Status != model.TraegerGesperrt || wieder.Sichtbarkeit != model.TraegerGeschlossen {
		t.Fatalf("Änderung nicht gespeichert: %+v", wieder)
	}
}

// Zwei Träger dürfen nicht auf dasselbe Zitadel-Projekt zeigen — sonst wäre
// nicht entscheidbar, wessen Admin man ist.
func TestProjektIDIstEindeutig(t *testing.T) {
	d := testDB(t)
	testTraeger(t, d, "Erster", "555")
	zweiter := model.Traeger{Name: "Zweiter", ProjektID: "555", Status: model.TraegerBeantragt,
		Sichtbarkeit: model.TraegerOffen, CreatedAt: time.Now().UTC()}
	if err := d.InsertTraeger(&zweiter); err == nil {
		t.Fatal("dieselbe Projekt-ID wurde zweimal angenommen")
	}
	// Mehrere Träger ohne Projekt-ID sind dagegen normal (noch nicht
	// eingerichtete Platzhalter).
	for _, name := range []string{"Ohne A", "Ohne B"} {
		ohne := model.Traeger{Name: name, Status: model.TraegerBeantragt,
			Sichtbarkeit: model.TraegerOffen, CreatedAt: time.Now().UTC()}
		if err := d.InsertTraeger(&ohne); err != nil {
			t.Fatalf("Träger ohne Projekt-ID: %v", err)
		}
	}
}

func TestBefaehigungUndAntrag(t *testing.T) {
	d := testDB(t)
	tr := testTraeger(t, d, "Dorfpflege", "606")
	jetzt := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)

	b := model.Befaehigung{TraegerID: tr.ID, Name: "Motorsense",
		Beschreibung: "Einweisung am Gerät", CreatedAt: jetzt}
	if err := d.InsertBefaehigung(&b); err != nil {
		t.Fatal(err)
	}
	liste, err := d.ListBefaehigungen(tr.ID)
	if err != nil || len(liste) != 1 || liste[0].Name != "Motorsense" {
		t.Fatalf("Befähigung nicht gelistet: %+v %v", liste, err)
	}

	// Beantragen, dann freigeben.
	a := model.BefaehigungsAntrag{BefaehigungID: b.ID, UserSub: "erna",
		Status: model.AntragBeantragt, CreatedAt: jetzt}
	if err := d.InsertAntrag(&a); err != nil {
		t.Fatal(err)
	}
	if d.HatBefaehigung("erna", b.ID) {
		t.Error("ein bloßer Antrag darf noch keine Befähigung sein")
	}
	if err := d.EntscheideAntrag(a.ID, model.AntragErteilt, "vorstand", "war beim Termin", jetzt); err != nil {
		t.Fatal(err)
	}
	if !d.HatBefaehigung("erna", b.ID) {
		t.Error("erteilte Befähigung wird nicht erkannt")
	}

	// Zurückziehen (ablehnen) nimmt sie wieder.
	if err := d.EntscheideAntrag(a.ID, model.AntragAbgelehnt, "vorstand", "", jetzt); err != nil {
		t.Fatal(err)
	}
	if d.HatBefaehigung("erna", b.ID) {
		t.Error("abgelehnte Befähigung gilt weiter")
	}

	// Zweimal beantragen ergibt keinen zweiten Antrag, sondern belebt den
	// vorhandenen wieder — sonst stapeln sich Karteileichen.
	zweiter := model.BefaehigungsAntrag{BefaehigungID: b.ID, UserSub: "erna",
		Status: model.AntragBeantragt, CreatedAt: jetzt}
	if err := d.InsertAntrag(&zweiter); err != nil {
		t.Fatal(err)
	}
	offen, err := d.ListAntraege(tr.ID, model.AntragBeantragt)
	if err != nil {
		t.Fatal(err)
	}
	if len(offen) != 1 {
		t.Fatalf("%d offene Anträge statt 1: %+v", len(offen), offen)
	}
}

// Wird eine Befähigung gelöscht, verlieren die Aufgaben ihre Voraussetzung —
// sie dürfen dabei nicht unerledigbar zurückbleiben.
func TestBefaehigungLoeschenGibtAufgabenFrei(t *testing.T) {
	d := testDB(t)
	tr := testTraeger(t, d, "Dorfpflege", "707")
	jetzt := time.Now().UTC()
	b := model.Befaehigung{TraegerID: tr.ID, Name: "Motorsense", CreatedAt: jetzt}
	if err := d.InsertBefaehigung(&b); err != nil {
		t.Fatal(err)
	}
	p := model.Place{Name: "Streuobstwiese", TraegerID: tr.ID, Kind: model.PlaceOther,
		Lat: 52.2, Lon: 9.87, Active: true, CreatedAt: jetzt}
	if err := d.InsertPlace(&p); err != nil {
		t.Fatal(err)
	}
	task := model.CareTask{PlaceID: p.ID, Kind: model.TaskOther, Title: "Rasenmähen",
		IntervalDays: 14, RedAfterDays: 28, BefaehigungID: b.ID,
		Sichtbarkeit: model.AufgabeOeffentlich, Active: true, CreatedAt: jetzt}
	if err := d.InsertTask(&task); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteBefaehigung(b.ID); err != nil {
		t.Fatal(err)
	}
	wieder, err := d.GetTask(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if wieder.BefaehigungID != 0 {
		t.Fatalf("Aufgabe hängt an einer gelöschten Befähigung: %+v", wieder)
	}
}

// Orte und Aufgaben tragen ihre neuen Felder unverändert durch die Datenbank.
func TestOrtUndAufgabeMitTraegerUndSichtbarkeit(t *testing.T) {
	d := testDB(t)
	tr := testTraeger(t, d, "Dorfpflege", "808")
	jetzt := time.Now().UTC()
	p := model.Place{Name: "Gerätehaus", TraegerID: tr.ID, Kind: model.PlaceOther,
		Lat: 52.2, Lon: 9.87, Active: true, CreatedAt: jetzt}
	if err := d.InsertPlace(&p); err != nil {
		t.Fatal(err)
	}
	task := model.CareTask{PlaceID: p.ID, Kind: model.TaskOther, Title: "Interne Prüfung",
		IntervalDays: 30, RedAfterDays: 60, Sichtbarkeit: model.AufgabeNurMitglieder,
		Active: true, CreatedAt: jetzt}
	if err := d.InsertTask(&task); err != nil {
		t.Fatal(err)
	}
	gelesenerOrt, err := d.GetPlace(p.ID)
	if err != nil || gelesenerOrt.TraegerID != tr.ID {
		t.Fatalf("Träger am Ort verloren: %+v %v", gelesenerOrt, err)
	}
	geleseneAufgabe, err := d.GetTask(task.ID)
	if err != nil || geleseneAufgabe.Sichtbarkeit != model.AufgabeNurMitglieder {
		t.Fatalf("Sichtbarkeit verloren: %+v %v", geleseneAufgabe, err)
	}
}
