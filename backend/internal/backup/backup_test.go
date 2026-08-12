package backup

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Eine echte Datenbank, keine Attrappe: Das Backup muss eine Datei liefern,
// die sich wieder öffnen lässt und dieselben Daten enthält.
func TestBackupIstEineBrauchbareKopie(t *testing.T) {
	verzeichnis := t.TempDir()
	quelle, err := db.Open(filepath.Join(verzeichnis, "quelle.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer quelle.Close()

	ort := model.Place{Name: "Unter den Eichen — Kasten 1", Kind: model.PlaceFlowerbox,
		Lat: 52.211, Lon: 9.87, Active: true, CreatedAt: time.Now()}
	if err := quelle.InsertPlace(&ort); err != nil {
		t.Fatal(err)
	}

	ziel := filepath.Join(verzeichnis, "backups")
	pfad, err := Einmal(quelle, Config{Dir: ziel, Keep: 14,
		Now: func() time.Time { return time.Date(2026, 8, 12, 3, 15, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatalf("Backup fehlgeschlagen: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(pfad), "dorfapp-") || !strings.HasSuffix(pfad, ".sqlite") {
		t.Fatalf("unerwarteter Dateiname: %s", pfad)
	}
	if info, err := os.Stat(pfad); err != nil || info.Size() == 0 {
		t.Fatalf("Backup-Datei fehlt oder ist leer: %v", err)
	}

	// Gegenprobe: Die Kopie lässt sich öffnen und enthält den Ort.
	kopie, err := db.Open(pfad)
	if err != nil {
		t.Fatalf("Kopie nicht lesbar: %v", err)
	}
	defer kopie.Close()
	orte, err := kopie.ListPlaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(orte) != 1 || orte[0].Name != ort.Name {
		t.Fatalf("Kopie enthält nicht dieselben Daten: %+v", orte)
	}

	// Die Quelle ist danach unverändert benutzbar.
	if _, err := quelle.ListPlaces(); err != nil {
		t.Fatalf("Quelle nach dem Backup kaputt: %v", err)
	}
}

// Rotation: Es bleiben nur die neuesten Kopien stehen, und zwar genau die.
func TestRotationBehaeltNurDieNeuesten(t *testing.T) {
	ziel := t.TempDir()
	jetzt := time.Date(2026, 8, 1, 3, 15, 0, 0, time.UTC)
	q := &attrappe{}

	var alle []string
	for i := 0; i < 20; i++ {
		tag := jetzt.AddDate(0, 0, i)
		pfad, err := Einmal(q, Config{Dir: ziel, Keep: 14, Now: func() time.Time { return tag }})
		if err != nil {
			t.Fatal(err)
		}
		alle = append(alle, filepath.Base(pfad))
	}

	dateien, err := vorhandene(ziel)
	if err != nil {
		t.Fatal(err)
	}
	if len(dateien) != 14 {
		t.Fatalf("%d Sicherungen statt 14: %v", len(dateien), dateien)
	}
	// Erwartet sind genau die letzten 14 Zeitpunkte.
	erwartet := map[string]bool{}
	for _, n := range alle[len(alle)-14:] {
		erwartet[n] = true
	}
	for _, d := range dateien {
		if !erwartet[filepath.Base(d)] {
			t.Errorf("unerwartete Datei überlebt: %s", d)
		}
	}

	// Fremde Dateien im Verzeichnis werden nicht angefasst.
	fremd := filepath.Join(ziel, "notizen.txt")
	if err := os.WriteFile(fremd, []byte("nicht anfassen"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Einmal(q, Config{Dir: ziel, Keep: 1, Now: func() time.Time { return jetzt.AddDate(0, 1, 0) }}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fremd); err != nil {
		t.Fatalf("fremde Datei wurde gelöscht: %v", err)
	}
}

// Der Zeitplan darf nicht bei jedem Neustart eine Sicherung schreiben: Nur
// wenn die jüngste Kopie älter als das Intervall ist, wird gesichert.
func TestFaelligkeit(t *testing.T) {
	ziel := t.TempDir()
	jetzt := time.Date(2026, 8, 12, 3, 15, 0, 0, time.UTC)
	cfg := Config{Dir: ziel, Keep: 14, Interval: 24 * time.Hour, Now: func() time.Time { return jetzt }}

	if !faellig(cfg) {
		t.Fatal("ohne jede Sicherung muss sofort gesichert werden")
	}
	if _, err := Einmal(&attrappe{}, cfg); err != nil {
		t.Fatal(err)
	}
	if faellig(cfg) {
		t.Fatal("direkt nach einer Sicherung ist nichts fällig")
	}

	cfg.Now = func() time.Time { return jetzt.Add(25 * time.Hour) }
	if !faellig(cfg) {
		t.Fatal("nach mehr als einem Tag muss wieder gesichert werden")
	}
}

// Der Zeitplan läuft im Hintergrund, sichert und lässt sich sauber beenden.
func TestZeitplanSichertUndEndet(t *testing.T) {
	ziel := t.TempDir()
	ctx, stop := context.WithCancel(context.Background())
	fertig := Start(ctx, &attrappe{}, Config{
		Dir: ziel, Keep: 3, Interval: time.Hour, Takt: 5 * time.Millisecond,
	})

	frist := time.Now().Add(5 * time.Second)
	for {
		dateien, _ := vorhandene(ziel)
		if len(dateien) > 0 {
			break
		}
		if time.Now().After(frist) {
			t.Fatal("der Zeitplan hat nichts gesichert")
		}
		time.Sleep(10 * time.Millisecond)
	}

	stop()
	select {
	case <-fertig:
	case <-time.After(5 * time.Second):
		t.Fatal("der Zeitplan hat sich nicht beenden lassen")
	}
}

// Ein fehlerhaftes Backup darf den Server nicht mitreißen.
func TestFehlerBeendetDenServerNicht(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	fertig := Start(ctx, &kaputt{}, Config{
		Dir: filepath.Join(t.TempDir(), "backups"), Keep: 3, Interval: time.Hour, Takt: time.Millisecond,
	})
	time.Sleep(50 * time.Millisecond)
	stop()
	<-fertig
}

// --- Hilfen ------------------------------------------------------------------

// attrappe schreibt statt einer echten Kopie eine Datei mit Inhalt.
type attrappe struct{}

func (attrappe) VacuumInto(pfad string) error {
	return os.WriteFile(pfad, []byte("sqlite"), 0o600)
}

type kaputt struct{}

func (kaputt) VacuumInto(string) error { return os.ErrPermission }
