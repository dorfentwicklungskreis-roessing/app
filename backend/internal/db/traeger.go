package db

import (
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Träger, Befähigungen und Anträge.
//
// Alles hier kommt rein additiv zum Bestand dazu: neue Tabellen und Spalten
// mit Vorbelegungen, die die laufende Datenbank unverändert weiterlaufen
// lassen (siehe migrateTraeger).

func (d *DB) migrateTraeger() error {
	if _, err := d.sql.Exec(`
-- Träger: Verein oder Gruppe, die Aufgaben kuratiert einstellt. Ein Träger
-- entspricht genau einem Zitadel-Projekt mit den Rollen admin und mitglied.
CREATE TABLE IF NOT EXISTS traeger (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  schluessel   TEXT NOT NULL DEFAULT '',
  projekt_id   TEXT NOT NULL DEFAULT '',
  name         TEXT NOT NULL,
  beschreibung TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'beantragt',
  sichtbarkeit TEXT NOT NULL DEFAULT 'offen',
  created_at   TEXT NOT NULL
);
-- Eindeutig, aber nur für gesetzte Werte: Mehrere Platzhalter ohne
-- eingerichtetes Zitadel-Projekt müssen nebeneinander möglich sein.
CREATE UNIQUE INDEX IF NOT EXISTS idx_traeger_projekt
  ON traeger(projekt_id) WHERE projekt_id <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_traeger_schluessel
  ON traeger(schluessel) WHERE schluessel <> '';

-- Befähigungen („Einweisung nötig“) gehören dem Träger, nicht der Aufgabe.
CREATE TABLE IF NOT EXISTS befaehigungen (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  traeger_id   INTEGER NOT NULL REFERENCES traeger(id) ON DELETE CASCADE,
  name         TEXT NOT NULL,
  beschreibung TEXT NOT NULL DEFAULT '',
  created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_befaehigungen_traeger ON befaehigungen(traeger_id);

-- Ein erteilter Antrag IST die Befähigung — deshalb nur eine Tabelle.
-- Je Person und Befähigung gibt es genau eine Zeile; ein erneuter Antrag
-- belebt sie wieder, statt Karteileichen zu stapeln.
CREATE TABLE IF NOT EXISTS befaehigungs_antraege (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  befaehigung_id  INTEGER NOT NULL REFERENCES befaehigungen(id) ON DELETE CASCADE,
  user_sub        TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'beantragt',
  begruendung     TEXT NOT NULL DEFAULT '',
  notiz           TEXT NOT NULL DEFAULT '',
  created_at      TEXT NOT NULL,
  entschieden_am  TEXT NOT NULL DEFAULT '',
  entschieden_von TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_antrag_person
  ON befaehigungs_antraege(befaehigung_id, user_sub);
CREATE INDEX IF NOT EXISTS idx_antrag_status ON befaehigungs_antraege(status);
`); err != nil {
		return err
	}

	// Nachträgliche Spalten am Bestand. Die Vorbelegungen sind so gewählt,
	// dass sich für die laufende App zunächst nichts ändert: Jede vorhandene
	// Aufgabe bleibt öffentlich und verlangt keine Einweisung.
	for _, stmt := range []string{
		`ALTER TABLE places ADD COLUMN traeger_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE care_tasks ADD COLUMN sichtbarkeit TEXT NOT NULL DEFAULT 'oeffentlich'`,
		`ALTER TABLE care_tasks ADD COLUMN befaehigung_id INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := d.sql.Exec(stmt); err != nil && !istDoppelteSpalte(err) {
			return err
		}
	}
	return d.bestandZuweisen()
}

// bestandZuweisen sorgt dafür, dass kein Ort ohne Träger dasteht.
//
// Die Bestandsdaten aus dem Cluster (die echten Blumenkästen „Unter den
// Eichen“) wandern zum Dorfentwicklungskreis — er ist der Platzhalter, bis
// die Dorfpflege offiziell zugestimmt hat. Der Lauf ist wiederholbar: Gibt
// es nichts Heimatloses, passiert nichts.
func (d *DB) bestandZuweisen() error {
	var offen int
	if err := d.sql.QueryRow(`SELECT COUNT(*) FROM places WHERE traeger_id = 0`).Scan(&offen); err != nil {
		return err
	}
	if offen == 0 {
		return nil
	}
	dek, err := d.TraegerSicherstellen(model.SchluesselDEK, model.NameDEK)
	if err != nil {
		return err
	}
	res, err := d.sql.Exec(`UPDATE places SET traeger_id = ? WHERE traeger_id = 0`, dek.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	slog.Info("Träger-Umstellung: Bestands-Orte zugewiesen", "traeger", dek.Name, "orte", n)
	return nil
}

// TraegerSicherstellen legt einen fest eingebauten Träger an, falls er fehlt.
// Er ist von Anfang an zugelassen: Wäre er es nicht, wäre das Dorf nach der
// Umstellung schlagartig blind für seine eigenen Blumenkästen.
func (d *DB) TraegerSicherstellen(schluessel, name string) (*model.Traeger, error) {
	vorhanden, err := d.GetTraegerBySchluessel(schluessel)
	if err != nil {
		return nil, err
	}
	if vorhanden != nil {
		return vorhanden, nil
	}
	neu := model.Traeger{
		Schluessel: schluessel, Name: name,
		Status: model.TraegerZugelassen, Sichtbarkeit: model.TraegerOffen,
		CreatedAt: time.Now().UTC(),
	}
	if err := d.InsertTraeger(&neu); err != nil {
		return nil, err
	}
	slog.Info("Träger angelegt", "schluessel", schluessel, "name", name)
	return &neu, nil
}

// --- Träger -----------------------------------------------------------------

const traegerSpalten = `id,schluessel,projekt_id,name,beschreibung,status,sichtbarkeit,created_at`

func (d *DB) InsertTraeger(t *model.Traeger) error {
	res, err := d.sql.Exec(`INSERT INTO traeger(schluessel,projekt_id,name,beschreibung,status,sichtbarkeit,created_at)
		VALUES(?,?,?,?,?,?,?)`,
		t.Schluessel, t.ProjektID, t.Name, t.Beschreibung, string(t.Status), string(t.Sichtbarkeit),
		t.CreatedAt.UTC().Format(timeFormat))
	if err != nil {
		return err
	}
	t.ID, err = res.LastInsertId()
	return err
}

func (d *DB) UpdateTraeger(t *model.Traeger) error {
	res, err := d.sql.Exec(`UPDATE traeger SET projekt_id=?,name=?,beschreibung=?,status=?,sichtbarkeit=?
		WHERE id=?`,
		t.ProjektID, t.Name, t.Beschreibung, string(t.Status), string(t.Sichtbarkeit), t.ID)
	if err != nil {
		return err
	}
	return requireRow(res)
}

// DeleteTraeger gibt es bewusst nicht: An einem Träger hängen Orte, Aufgaben
// und Erledigungen. Ein Träger, der nicht mehr auftreten soll, wird gesperrt
// (model.TraegerGesperrt) — dann bleibt die Historie erhalten.

func (d *DB) GetTraeger(id int64) (*model.Traeger, error) {
	return scanTraeger(d.sql.QueryRow(`SELECT `+traegerSpalten+` FROM traeger WHERE id=?`, id))
}

// GetTraegerByProjekt findet den Träger zu einer Zitadel-Projekt-ID. Das ist
// der Weg, über den aus einer Rollenzuweisung eine Berechtigung wird.
// Liefert (nil, nil), wenn es keinen gibt.
func (d *DB) GetTraegerByProjekt(projektID string) (*model.Traeger, error) {
	if projektID == "" {
		return nil, nil
	}
	t, err := scanTraeger(d.sql.QueryRow(`SELECT `+traegerSpalten+` FROM traeger WHERE projekt_id=?`, projektID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

// GetTraegerBySchluessel findet einen fest eingebauten Träger. (nil, nil),
// wenn es ihn noch nicht gibt.
func (d *DB) GetTraegerBySchluessel(schluessel string) (*model.Traeger, error) {
	if schluessel == "" {
		return nil, nil
	}
	t, err := scanTraeger(d.sql.QueryRow(`SELECT `+traegerSpalten+` FROM traeger WHERE schluessel=?`, schluessel))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (d *DB) ListTraeger() ([]model.Traeger, error) {
	rows, err := d.sql.Query(`SELECT ` + traegerSpalten + ` FROM traeger ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Traeger{}
	for rows.Next() {
		t, err := scanTraeger(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// TraegerIndex liefert alle Träger nach ID — die Sichtbarkeitsprüfungen
// schlagen für jeden Ort einmal nach.
func (d *DB) TraegerIndex() (map[int64]model.Traeger, error) {
	liste, err := d.ListTraeger()
	if err != nil {
		return nil, err
	}
	out := make(map[int64]model.Traeger, len(liste))
	for _, t := range liste {
		out[t.ID] = t
	}
	return out, nil
}

func scanTraeger(row scannable) (*model.Traeger, error) {
	var t model.Traeger
	var status, sicht, created string
	if err := row.Scan(&t.ID, &t.Schluessel, &t.ProjektID, &t.Name, &t.Beschreibung,
		&status, &sicht, &created); err != nil {
		return nil, err
	}
	t.Status = model.TraegerStatus(status)
	t.Sichtbarkeit = model.TraegerSichtbarkeit(sicht)
	t.CreatedAt, _ = time.Parse(timeFormat, created)
	return &t, nil
}

// --- Befähigungen -----------------------------------------------------------

const befaehigungSpalten = `id,traeger_id,name,beschreibung,created_at`

func (d *DB) InsertBefaehigung(b *model.Befaehigung) error {
	res, err := d.sql.Exec(`INSERT INTO befaehigungen(traeger_id,name,beschreibung,created_at)
		VALUES(?,?,?,?)`, b.TraegerID, b.Name, b.Beschreibung, b.CreatedAt.UTC().Format(timeFormat))
	if err != nil {
		return err
	}
	b.ID, err = res.LastInsertId()
	return err
}

func (d *DB) UpdateBefaehigung(b *model.Befaehigung) error {
	res, err := d.sql.Exec(`UPDATE befaehigungen SET name=?,beschreibung=? WHERE id=?`,
		b.Name, b.Beschreibung, b.ID)
	if err != nil {
		return err
	}
	return requireRow(res)
}

// DeleteBefaehigung entfernt eine Einweisung und gibt die Aufgaben frei, die
// sie verlangt haben. Sonst bliebe eine Aufgabe zurück, die niemand mehr
// zusagen kann, weil es die verlangte Befähigung gar nicht mehr gibt.
func (d *DB) DeleteBefaehigung(id int64) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`UPDATE care_tasks SET befaehigung_id=0 WHERE befaehigung_id=?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM befaehigungen WHERE id=?`, id)
	if err != nil {
		return err
	}
	if err := requireRow(res); err != nil {
		return err
	}
	return tx.Commit()
}

func (d *DB) GetBefaehigung(id int64) (*model.Befaehigung, error) {
	return scanBefaehigung(d.sql.QueryRow(`SELECT `+befaehigungSpalten+` FROM befaehigungen WHERE id=?`, id))
}

func (d *DB) ListBefaehigungen(traegerID int64) ([]model.Befaehigung, error) {
	rows, err := d.sql.Query(`SELECT `+befaehigungSpalten+`
		FROM befaehigungen WHERE traeger_id=? ORDER BY name`, traegerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return sammleBefaehigungen(rows)
}

// ListAlleBefaehigungen liefert den ganzen Bestand — die Verwaltung und die
// Aufgaben-Anzeige lösen damit Namen auf, ohne je Aufgabe nachzufragen.
func (d *DB) ListAlleBefaehigungen() ([]model.Befaehigung, error) {
	rows, err := d.sql.Query(`SELECT ` + befaehigungSpalten + ` FROM befaehigungen ORDER BY traeger_id, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return sammleBefaehigungen(rows)
}

func sammleBefaehigungen(rows *sql.Rows) ([]model.Befaehigung, error) {
	out := []model.Befaehigung{}
	for rows.Next() {
		b, err := scanBefaehigung(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func scanBefaehigung(row scannable) (*model.Befaehigung, error) {
	var b model.Befaehigung
	var created string
	if err := row.Scan(&b.ID, &b.TraegerID, &b.Name, &b.Beschreibung, &created); err != nil {
		return nil, err
	}
	b.CreatedAt, _ = time.Parse(timeFormat, created)
	return &b, nil
}

// --- Anträge ----------------------------------------------------------------

const antragSpalten = `id,befaehigung_id,user_sub,status,begruendung,notiz,created_at,entschieden_am,entschieden_von`

// InsertAntrag stellt einen Antrag. Gibt es für diese Person und Befähigung
// schon einen, wird er wiederbelebt: Ein abgelehnter Antrag darf neu gestellt
// werden, ein bereits erteilter bleibt, wie er ist.
func (d *DB) InsertAntrag(a *model.BefaehigungsAntrag) error {
	res, err := d.sql.Exec(`INSERT INTO befaehigungs_antraege
		(befaehigung_id,user_sub,status,begruendung,created_at) VALUES(?,?,?,?,?)
		ON CONFLICT(befaehigung_id,user_sub) DO UPDATE SET
		  status = CASE WHEN befaehigungs_antraege.status = 'erteilt'
		                THEN 'erteilt' ELSE excluded.status END,
		  begruendung = excluded.begruendung,
		  created_at  = excluded.created_at,
		  entschieden_am = '', entschieden_von = '', notiz = ''`,
		a.BefaehigungID, a.UserSub, string(a.Status), a.Begruendung,
		a.CreatedAt.UTC().Format(timeFormat))
	if err != nil {
		return err
	}
	if a.ID, err = res.LastInsertId(); err != nil || a.ID > 0 {
		return err
	}
	// Beim Aktualisieren liefert SQLite keine brauchbare ID zurück.
	return d.sql.QueryRow(`SELECT id FROM befaehigungs_antraege WHERE befaehigung_id=? AND user_sub=?`,
		a.BefaehigungID, a.UserSub).Scan(&a.ID)
}

// EntscheideAntrag gibt frei oder lehnt ab.
func (d *DB) EntscheideAntrag(id int64, status model.AntragStatus, durch, notiz string, at time.Time) error {
	res, err := d.sql.Exec(`UPDATE befaehigungs_antraege
		SET status=?, notiz=?, entschieden_am=?, entschieden_von=? WHERE id=?`,
		string(status), notiz, at.UTC().Format(timeFormat), durch, id)
	if err != nil {
		return err
	}
	return requireRow(res)
}

func (d *DB) GetAntrag(id int64) (*model.BefaehigungsAntrag, error) {
	return scanAntrag(d.sql.QueryRow(`SELECT `+antragSpalten+` FROM befaehigungs_antraege WHERE id=?`, id))
}

// ListAntraege liefert die Anträge eines Trägers, optional auf einen Stand
// gefiltert (leerer Status = alle).
func (d *DB) ListAntraege(traegerID int64, status model.AntragStatus) ([]model.BefaehigungsAntrag, error) {
	query := `SELECT a.id,a.befaehigung_id,a.user_sub,a.status,a.begruendung,a.notiz,
	                 a.created_at,a.entschieden_am,a.entschieden_von,b.name,b.traeger_id
	            FROM befaehigungs_antraege a
	            JOIN befaehigungen b ON b.id = a.befaehigung_id
	           WHERE b.traeger_id = ?`
	args := []any{traegerID}
	if status != "" {
		query += ` AND a.status = ?`
		args = append(args, string(status))
	}
	query += ` ORDER BY a.created_at DESC`
	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.BefaehigungsAntrag{}
	for rows.Next() {
		var a model.BefaehigungsAntrag
		var status, created, entschieden string
		if err := rows.Scan(&a.ID, &a.BefaehigungID, &a.UserSub, &status, &a.Begruendung, &a.Notiz,
			&created, &entschieden, &a.EntschiedenVon, &a.BefaehigungName, &a.TraegerID); err != nil {
			return nil, err
		}
		a.Status = model.AntragStatus(status)
		a.CreatedAt, _ = time.Parse(timeFormat, created)
		a.EntschiedenAm = zeitWert(entschieden)
		out = append(out, a)
	}
	return out, rows.Err()
}

// ListAntraegeVonPerson liefert alles, was jemand beantragt oder bekommen hat.
func (d *DB) ListAntraegeVonPerson(userSub string) ([]model.BefaehigungsAntrag, error) {
	rows, err := d.sql.Query(`SELECT a.id,a.befaehigung_id,a.user_sub,a.status,a.begruendung,a.notiz,
	                 a.created_at,a.entschieden_am,a.entschieden_von,b.name,b.traeger_id
	            FROM befaehigungs_antraege a
	            JOIN befaehigungen b ON b.id = a.befaehigung_id
	           WHERE a.user_sub = ? ORDER BY b.name`, userSub)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.BefaehigungsAntrag{}
	for rows.Next() {
		var a model.BefaehigungsAntrag
		var status, created, entschieden string
		if err := rows.Scan(&a.ID, &a.BefaehigungID, &a.UserSub, &status, &a.Begruendung, &a.Notiz,
			&created, &entschieden, &a.EntschiedenVon, &a.BefaehigungName, &a.TraegerID); err != nil {
			return nil, err
		}
		a.Status = model.AntragStatus(status)
		a.CreatedAt, _ = time.Parse(timeFormat, created)
		a.EntschiedenAm = zeitWert(entschieden)
		out = append(out, a)
	}
	return out, rows.Err()
}

// HatBefaehigung ist die Prüfung, an der eine Zusage hängt.
//
// Sie ist bewusst als einfaches bool gebaut und schlägt im Zweifel nach
// „nein“ aus: Ein Datenbankfehler darf niemanden an die Motorsense lassen.
func (d *DB) HatBefaehigung(userSub string, befaehigungID int64) bool {
	if befaehigungID == 0 {
		return true // keine Einweisung verlangt
	}
	var n int
	err := d.sql.QueryRow(`SELECT COUNT(*) FROM befaehigungs_antraege
		WHERE befaehigung_id=? AND user_sub=? AND status='erteilt'`, befaehigungID, userSub).Scan(&n)
	if err != nil {
		slog.Error("Befähigung konnte nicht geprüft werden — im Zweifel nein",
			"befaehigung", befaehigungID, "err", err)
		return false
	}
	return n > 0
}

// ErteilteBefaehigungen liefert die Kennungen aller erteilten Befähigungen
// einer Person — für die Vergabe, die viele Aufgaben auf einmal prüft.
func (d *DB) ErteilteBefaehigungen(userSub string) (map[int64]bool, error) {
	rows, err := d.sql.Query(`SELECT befaehigung_id FROM befaehigungs_antraege
		WHERE user_sub=? AND status='erteilt'`, userSub)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

func scanAntrag(row scannable) (*model.BefaehigungsAntrag, error) {
	var a model.BefaehigungsAntrag
	var status, created, entschieden string
	if err := row.Scan(&a.ID, &a.BefaehigungID, &a.UserSub, &status, &a.Begruendung, &a.Notiz,
		&created, &entschieden, &a.EntschiedenVon); err != nil {
		return nil, err
	}
	a.Status = model.AntragStatus(status)
	a.CreatedAt, _ = time.Parse(timeFormat, created)
	a.EntschiedenAm = zeitWert(entschieden)
	return &a, nil
}
