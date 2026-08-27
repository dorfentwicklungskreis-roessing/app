// Package backup sichert die SQLite-Datenbank im laufenden Betrieb.
//
// Warum im Server und nicht als Kubernetes-CronJob: Die Datenbank liegt auf
// einem RWO-PVC, das Deployment fährt bewusst mit „Recreate" und genau einem
// Pod. Ein CronJob-Pod müsste dasselbe PVC einhängen — das geht nur, wenn er
// zufällig auf demselben Knoten landet, und selbst dann schriebe ein zweiter
// Prozess in eine WAL-Datenbank, die einem anderen Prozess gehört. Der
// eingebaute Zeitplan hat dieses Problem nicht: Er nutzt dieselbe, einzige
// Schreibverbindung und bekommt von SQLite garantiert eine in sich stimmige
// Kopie.
//
// Technik: „VACUUM INTO" schreibt eine vollständige, aufgeräumte Kopie der
// Datenbank in eine neue Datei — transaktionssicher, ohne den Betrieb
// anzuhalten. Die Kopien landen neben der Datenbank im PVC (Standard
// /data/backups) und werden rotiert.
package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/clock"
)

// Quelle ist alles, was sich selbst in eine Datei sichern kann (in der Praxis
// *db.DB). Als Schnittstelle, damit der Zeitplan ohne echte Datenbank
// prüfbar bleibt.
type Quelle interface {
	VacuumInto(pfad string) error
}

const (
	// praefix und endung bestimmen, welche Dateien zur Rotation gehören.
	praefix = "dorfapp-"
	endung  = ".sqlite"
	// zeitFormat ist sortierfreundlich (lexikografisch = chronologisch).
	zeitFormat = "20060102T150405Z"
)

// Config beschreibt den Sicherungslauf.
type Config struct {
	// Dir ist das Zielverzeichnis, z.B. /data/backups.
	Dir string
	// Keep ist die Anzahl aufbewahrter Sicherungen.
	Keep int
	// Interval ist der Abstand zwischen zwei Sicherungen.
	Interval time.Duration
	// Takt ist der Prüfabstand des Zeitplans (Vorgabe 15 Minuten). Geprüft
	// wird jedes Mal, gesichert nur, wenn die letzte Kopie zu alt ist —
	// dadurch übersteht der Plan auch Neustarts und Ausfälle.
	Takt time.Duration
	// Now ist die Zeitquelle (Tests).
	Now func() time.Time
}

// Vorgaben: täglich, 14 Kopien — knapp zwei Wochen Rückgriff.
const (
	DefaultDir      = "/data/backups"
	DefaultKeep     = 14
	DefaultInterval = 24 * time.Hour
	DefaultTakt     = 15 * time.Minute
)

func (c *Config) vollstaendig() {
	if c.Dir == "" {
		c.Dir = DefaultDir
	}
	if c.Keep <= 0 {
		c.Keep = DefaultKeep
	}
	if c.Interval <= 0 {
		c.Interval = DefaultInterval
	}
	if c.Takt <= 0 {
		c.Takt = DefaultTakt
	}
	if c.Now == nil {
		c.Now = clock.Now
	}
}

// FromEnv liest die Konfiguration aus der Umgebung:
//
//	BACKUP=off        schaltet die Sicherung ab
//	BACKUP_DIR        Zielverzeichnis (Vorgabe /data/backups)
//	BACKUP_KEEP       Anzahl aufbewahrter Kopien (Vorgabe 14)
//	BACKUP_INTERVAL   Abstand als Go-Dauer, z.B. „24h" (Vorgabe 24h)
func FromEnv() (Config, bool) {
	if istAus(os.Getenv("BACKUP")) {
		return Config{}, false
	}
	cfg := Config{Dir: os.Getenv("BACKUP_DIR")}
	if v := os.Getenv("BACKUP_KEEP"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			cfg.Keep = n
		}
	}
	if v := os.Getenv("BACKUP_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Interval = d
		}
	}
	cfg.vollstaendig()
	return cfg, true
}

func istAus(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "0", "false", "aus", "nein":
		return true
	}
	return false
}

// Einmal zieht sofort eine Sicherung und rotiert danach.
func Einmal(q Quelle, cfg Config) (string, error) {
	cfg.vollstaendig()
	if err := os.MkdirAll(cfg.Dir, 0o750); err != nil {
		return "", fmt.Errorf("Backup-Verzeichnis: %w", err)
	}
	name := praefix + cfg.Now().UTC().Format(zeitFormat) + endung
	ziel := filepath.Join(cfg.Dir, name)

	// Reste eines abgebrochenen Laufs: VACUUM INTO verlangt eine neue Datei.
	if _, err := os.Stat(ziel); err == nil {
		if err := os.Remove(ziel); err != nil {
			return "", err
		}
	}
	if err := q.VacuumInto(ziel); err != nil {
		return "", fmt.Errorf("VACUUM INTO: %w", err)
	}
	if err := rotiere(cfg.Dir, cfg.Keep); err != nil {
		// Die Sicherung steht — die Rotation ist zweitrangig.
		slog.Warn("Backup: Rotation fehlgeschlagen", "err", err)
	}
	return ziel, nil
}

// vorhandene liefert die Sicherungen, jüngste zuletzt.
func vorhandene(dir string) ([]string, error) {
	eintraege, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range eintraege {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, praefix) || !strings.HasSuffix(name, endung) {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	// Der Zeitstempel im Namen sortiert chronologisch.
	sort.Strings(out)
	return out, nil
}

// rotiere löscht die ältesten Sicherungen, bis nur noch keep übrig sind.
// Fremde Dateien im Verzeichnis bleiben unangetastet.
func rotiere(dir string, keep int) error {
	dateien, err := vorhandene(dir)
	if err != nil {
		return err
	}
	if len(dateien) <= keep {
		return nil
	}
	for _, alt := range dateien[:len(dateien)-keep] {
		if err := os.Remove(alt); err != nil {
			return err
		}
	}
	return nil
}

// faellig entscheidet, ob eine neue Sicherung nötig ist.
func faellig(cfg Config) bool {
	cfg.vollstaendig()
	dateien, err := vorhandene(cfg.Dir)
	if err != nil || len(dateien) == 0 {
		return true
	}
	juengste := dateien[len(dateien)-1]
	stempel := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(juengste), praefix), endung)
	t, err := time.Parse(zeitFormat, stempel)
	if err != nil {
		// Unlesbarer Name: lieber sichern als nicht sichern.
		return true
	}
	return cfg.Now().UTC().Sub(t) >= cfg.Interval
}

// Start lässt den Zeitplan im Hintergrund laufen. Der gelieferte Kanal wird
// geschlossen, sobald der Plan nach dem Ende des Contexts steht.
func Start(ctx context.Context, q Quelle, cfg Config) <-chan struct{} {
	cfg.vollstaendig()
	fertig := make(chan struct{})
	go func() {
		defer close(fertig)
		t := time.NewTicker(cfg.Takt)
		defer t.Stop()
		pruefen := func() {
			if !faellig(cfg) {
				return
			}
			pfad, err := Einmal(q, cfg)
			if err != nil {
				// Ein misslungenes Backup ist ärgerlich, aber kein Grund,
				// den Dorf-Server anzuhalten.
				slog.Error("Backup fehlgeschlagen", "err", err, "verzeichnis", cfg.Dir)
				return
			}
			slog.Info("Backup geschrieben", "datei", pfad, "aufbewahrt", cfg.Keep)
		}
		pruefen()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				pruefen()
			}
		}
	}()
	return fertig
}
