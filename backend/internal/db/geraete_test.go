package db

import (
	"path/filepath"
	"testing"
	"time"
)

// Tests der Gerätekennungen für den Push-Versand — gegen eine echte
// SQLite-Datei, nicht gegen eine Attrappe: Genau die Eindeutigkeit des
// Tokens und das Verhalten beim Weitergeben eines Geräts entscheiden
// darüber, ob jemand fremde Benachrichtigungen bekommt.

func testDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestGeraetRegistrierenUndAuffrischen(t *testing.T) {
	d := testDB(t)
	jetzt := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)

	neu, err := d.UpsertDevice("anna", "token-1", "android", jetzt)
	if err != nil {
		t.Fatal(err)
	}
	if !neu {
		t.Fatal("erste Registrierung sollte neu sein")
	}

	// Auffrischen: dasselbe Gerät, neuer Zeitstempel, kein zweiter Eintrag.
	spaeter := jetzt.Add(48 * time.Hour)
	neu, err = d.UpsertDevice("anna", "token-1", "android", spaeter)
	if err != nil {
		t.Fatal(err)
	}
	if neu {
		t.Fatal("Auffrischen sollte keinen neuen Eintrag anlegen")
	}
	geraete, err := d.DevicesForUser("anna")
	if err != nil {
		t.Fatal(err)
	}
	if len(geraete) != 1 {
		t.Fatalf("erwartet 1 Gerät, bekommen %d", len(geraete))
	}
	if !geraete[0].UpdatedAt.Equal(spaeter) {
		t.Errorf("Zeitstempel nicht aufgefrischt: %v", geraete[0].UpdatedAt)
	}
	if geraete[0].Token != "token-1" || geraete[0].Platform != "android" {
		t.Errorf("unerwartetes Gerät: %+v", geraete[0])
	}
}

func TestMehrereGeraeteJePerson(t *testing.T) {
	d := testDB(t)
	jetzt := time.Now().UTC().Truncate(time.Second)
	for _, tok := range []string{"handy", "tablet"} {
		if _, err := d.UpsertDevice("anna", tok, "android", jetzt); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.UpsertDevice("bernd", "berndhandy", "android", jetzt); err != nil {
		t.Fatal(err)
	}
	geraete, err := d.DevicesForUser("anna")
	if err != nil {
		t.Fatal(err)
	}
	if len(geraete) != 2 {
		t.Fatalf("Anna sollte 2 Geräte haben, hat %d", len(geraete))
	}
	fremde, err := d.DevicesForUser("bernd")
	if err != nil {
		t.Fatal(err)
	}
	if len(fremde) != 1 || fremde[0].Token != "berndhandy" {
		t.Fatalf("Bernds Geräte falsch: %+v", fremde)
	}
}

// Ein weitergegebenes Handy darf keine Benachrichtigungen des Vorbesitzers
// mehr bekommen: Dasselbe Token gehört immer nur einer Person.
func TestGeraetWechseltDieBesitzerin(t *testing.T) {
	d := testDB(t)
	jetzt := time.Now().UTC().Truncate(time.Second)
	if _, err := d.UpsertDevice("anna", "token-1", "android", jetzt); err != nil {
		t.Fatal(err)
	}
	if _, err := d.UpsertDevice("bernd", "token-1", "android", jetzt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	annas, err := d.DevicesForUser("anna")
	if err != nil {
		t.Fatal(err)
	}
	if len(annas) != 0 {
		t.Fatalf("Anna sollte das Gerät verloren haben: %+v", annas)
	}
	bernds, err := d.DevicesForUser("bernd")
	if err != nil {
		t.Fatal(err)
	}
	if len(bernds) != 1 {
		t.Fatalf("Bernd sollte das Gerät haben: %+v", bernds)
	}
}

func TestGeraetAbmelden(t *testing.T) {
	d := testDB(t)
	jetzt := time.Now().UTC().Truncate(time.Second)
	if _, err := d.UpsertDevice("anna", "token-1", "android", jetzt); err != nil {
		t.Fatal(err)
	}
	// Fremde Kennungen lassen sich nicht abmelden.
	n, err := d.DeleteDevice("bernd", "token-1")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("fremdes Gerät durfte nicht abgemeldet werden")
	}
	n, err = d.DeleteDevice("anna", "token-1")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("erwartet 1 abgemeldetes Gerät, bekommen %d", n)
	}
	geraete, _ := d.DevicesForUser("anna")
	if len(geraete) != 0 {
		t.Fatalf("Gerät noch da: %+v", geraete)
	}
}

// Was Google als ungültig meldet, fliegt raus — ohne dass wir wissen
// müssten, wem es gehörte.
func TestUngueltigesTokenEntfernen(t *testing.T) {
	d := testDB(t)
	jetzt := time.Now().UTC().Truncate(time.Second)
	if _, err := d.UpsertDevice("anna", "token-1", "android", jetzt); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteDeviceToken("token-1"); err != nil {
		t.Fatal(err)
	}
	geraete, _ := d.DevicesForUser("anna")
	if len(geraete) != 0 {
		t.Fatalf("Gerät noch da: %+v", geraete)
	}
	// Ein zweiter Aufruf ist kein Fehler (zwei Versände, dieselbe Antwort).
	if err := d.DeleteDeviceToken("token-1"); err != nil {
		t.Fatalf("zweites Entfernen soll still durchgehen: %v", err)
	}
}
