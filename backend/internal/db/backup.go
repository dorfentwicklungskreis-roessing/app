package db

import (
	"fmt"
	"strings"
)

// VacuumInto schreibt eine vollständige, in sich stimmige Kopie der Datenbank
// nach pfad („SQLite Online Backup" per VACUUM INTO). Die Kopie ist aufgeräumt
// (kein freier Platz, kein WAL) und lässt sich direkt wieder öffnen.
//
// Die Datei darf noch nicht existieren — das verlangt SQLite selbst.
func (d *DB) VacuumInto(pfad string) error {
	// Der Dateiname ist in VACUUM INTO ein Ausdruck; Platzhalter sind dort
	// nicht erlaubt. Deshalb wird er als SQL-Zeichenkette eingesetzt, mit
	// verdoppelten Hochkommata — der Pfad kommt ohnehin aus der eigenen
	// Konfiguration und nie von außen.
	literal := "'" + strings.ReplaceAll(pfad, "'", "''") + "'"
	if _, err := d.sql.Exec("VACUUM INTO " + literal); err != nil {
		return fmt.Errorf("VACUUM INTO %s: %w", pfad, err)
	}
	return nil
}
