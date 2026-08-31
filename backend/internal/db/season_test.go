package db

import (
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Die Jahreszeit einer Aufgabe (#78) muss die Datei überleben — sonst stünde
// sie nach dem nächsten Neustart wieder ganzjährig da.

func TestJahreszeitUeberlebtDieDatenbank(t *testing.T) {
	d := testDB(t)
	jetzt := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)

	ort := model.Place{Name: "Dorfgemeinschaftshaus", Kind: model.PlaceBed,
		Lat: 52.211, Lon: 9.87, Active: true, CreatedAt: jetzt}
	if err := d.InsertPlace(&ort); err != nil {
		t.Fatal(err)
	}

	aufgabe := model.CareTask{PlaceID: ort.ID, Kind: model.TaskWeeding,
		IntervalDays: 56, RedAfterDays: 70,
		SeasonStartMonth: 4, SeasonEndMonth: 9,
		Active: true, CreatedAt: jetzt}
	if err := d.InsertTask(&aufgabe); err != nil {
		t.Fatal(err)
	}

	gelesen, err := d.GetTask(aufgabe.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gelesen.SeasonStartMonth != 4 || gelesen.SeasonEndMonth != 9 {
		t.Fatalf("Jahreszeit = %d/%d, erwartet 4/9",
			gelesen.SeasonStartMonth, gelesen.SeasonEndMonth)
	}

	// Zurück auf ganzjährig — auch das muss ankommen.
	gelesen.SeasonStartMonth, gelesen.SeasonEndMonth = 0, 0
	if err := d.UpdateTask(gelesen); err != nil {
		t.Fatal(err)
	}
	wieder, err := d.GetTask(aufgabe.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, saisonal := wieder.SeasonOf(); saisonal {
		t.Fatalf("Jahreszeit nicht abgeräumt: %d/%d",
			wieder.SeasonStartMonth, wieder.SeasonEndMonth)
	}
}

// Ohne Angabe ist eine Aufgabe ganzjährig — die Vorbelegung der neuen
// Spalten.
func TestAufgabeOhneAngabeIstGanzjaehrig(t *testing.T) {
	d := testDB(t)
	jetzt := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	ort := model.Place{Name: "Kasten 1", Kind: model.PlaceFlowerbox,
		Lat: 52.211, Lon: 9.87, Active: true, CreatedAt: jetzt}
	if err := d.InsertPlace(&ort); err != nil {
		t.Fatal(err)
	}
	aufgabe := model.CareTask{PlaceID: ort.ID, Kind: model.TaskWatering,
		IntervalDays: 7, RedAfterDays: 14, Active: true, CreatedAt: jetzt}
	if err := d.InsertTask(&aufgabe); err != nil {
		t.Fatal(err)
	}
	gelesen, err := d.GetTask(aufgabe.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, saisonal := gelesen.SeasonOf(); saisonal {
		t.Fatalf("unerwartete Jahreszeit: %d/%d",
			gelesen.SeasonStartMonth, gelesen.SeasonEndMonth)
	}
}
