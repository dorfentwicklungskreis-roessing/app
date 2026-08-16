// Package db kapselt den SQLite-Zugriff (WAL-Modus) und die Migrationen.
package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
	_ "modernc.org/sqlite"
)

// Open öffnet die SQLite-Datenbank im WAL-Modus und führt Migrationen aus.
func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)", path)
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// modernc.org/sqlite ist bei einer Verbindung am robustesten gegen Locks;
	// bei Dorf-Skala völlig ausreichend.
	sqlDB.SetMaxOpenConns(1)
	d := &DB{sql: sqlDB}
	if err := d.migrate(); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return d, nil
}

type DB struct {
	sql *sql.DB
}

func (d *DB) Close() error { return d.sql.Close() }

func (d *DB) migrate() error {
	_, err := d.sql.Exec(`
CREATE TABLE IF NOT EXISTS places (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  kind        TEXT NOT NULL DEFAULT 'blumenkasten',
  lat         REAL NOT NULL,
  lon         REAL NOT NULL,
  active      INTEGER NOT NULL DEFAULT 1,
  created_at  TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS care_tasks (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  place_id       INTEGER NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  kind           TEXT NOT NULL,
  title          TEXT NOT NULL DEFAULT '',
  liters         REAL,
  interval_days  REAL NOT NULL,
  red_after_days REAL NOT NULL,
  active         INTEGER NOT NULL DEFAULT 1,
  created_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_tasks_place ON care_tasks(place_id);
CREATE TABLE IF NOT EXISTS completions (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id   INTEGER NOT NULL REFERENCES care_tasks(id) ON DELETE CASCADE,
  user_sub  TEXT NOT NULL,
  user_name TEXT NOT NULL,
  liters    REAL,
  note      TEXT NOT NULL DEFAULT '',
  done_at   TEXT NOT NULL,
  forced    INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_completions_task_time ON completions(task_id, done_at DESC);
CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
-- Profile: was jede Person selbst über sich hinterlegt. Rein additiv — an
-- den bestehenden Tabellen ändert sich nichts, die laufende Datenbank
-- bekommt die Tabelle beim nächsten Start einfach dazu.
-- vis_*: Sichtbarkeit je Feld ('dorf' = alle Angemeldeten, 'verwaltung' =
-- nur Verwaltende). Kontaktdaten stehen bewusst auf 'verwaltung'.
CREATE TABLE IF NOT EXISTS profiles (
  user_sub         TEXT PRIMARY KEY,
  display_name     TEXT NOT NULL DEFAULT '',
  nickname         TEXT NOT NULL DEFAULT '',
  phone            TEXT NOT NULL DEFAULT '',
  email            TEXT NOT NULL DEFAULT '',
  note             TEXT NOT NULL DEFAULT '',
  vis_display_name TEXT NOT NULL DEFAULT 'dorf',
  vis_nickname     TEXT NOT NULL DEFAULT 'dorf',
  vis_phone        TEXT NOT NULL DEFAULT 'verwaltung',
  vis_email        TEXT NOT NULL DEFAULT 'verwaltung',
  vis_note         TEXT NOT NULL DEFAULT 'verwaltung',
  token_name       TEXT NOT NULL DEFAULT '',
  updated_at       TEXT NOT NULL
);
-- Vergabe von Pflegeaufgaben (siehe internal/vergabe). Auch das kommt rein
-- additiv dazu: Wer sich nirgends anmeldet, merkt von den drei Tabellen
-- nichts, und an places/care_tasks/completions ändert sich kein Feld.
--
-- care_signups: „Ich kümmere mich mit" — Anmeldung für einen Ort, optional
-- auf eine Aufgabenart eingeschränkt (task_kind leer = alle Aufgaben).
CREATE TABLE IF NOT EXISTS care_signups (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_sub   TEXT NOT NULL,
  place_id   INTEGER NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  task_kind  TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_signups_eindeutig ON care_signups(user_sub, place_id, task_kind);
CREATE INDEX IF NOT EXISTS idx_signups_place ON care_signups(place_id);
-- care_assignments: ein Vergabe-Vorgang je fälliger Aufgabe. Leere
-- Zeitfelder heißen „nicht gesetzt"; ended_at='' kennzeichnet die laufenden
-- Vorgänge, davon darf es je Aufgabe nur einen geben.
CREATE TABLE IF NOT EXISTS care_assignments (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id       INTEGER NOT NULL REFERENCES care_tasks(id) ON DELETE CASCADE,
  state         TEXT NOT NULL,
  created_at    TEXT NOT NULL,
  next_offer_at TEXT NOT NULL DEFAULT '',
  claimed_by    TEXT NOT NULL DEFAULT '',
  claimed_name  TEXT NOT NULL DEFAULT '',
  claimed_at    TEXT NOT NULL DEFAULT '',
  claim_until   TEXT NOT NULL DEFAULT '',
  ended_at      TEXT NOT NULL DEFAULT '',
  end_reason    TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_assignments_laufend ON care_assignments(task_id) WHERE ended_at = '';
CREATE INDEX IF NOT EXISTS idx_assignments_task ON care_assignments(task_id);
-- care_notifications: die Zustellungen an einzelne Personen (Anfragen und
-- Hinweise). Sie sind gleichzeitig das Gedächtnis der Warteschlange: Wer
-- hier steht, wurde schon gefragt.
--
-- Bewusst OHNE Fremdschlüssel auf care_assignments: Wird eine Aufgabe
-- gelöscht, verschwindet der Vorgang — der Hinweis „deine zugesagte Aufgabe
-- ist entfallen" muss aber gerade dann bleiben. Ort und Aufgabe stehen
-- deshalb zusätzlich im Klartext dabei.
CREATE TABLE IF NOT EXISTS care_notifications (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  assignment_id INTEGER NOT NULL,
  task_id       INTEGER NOT NULL,
  place_id      INTEGER NOT NULL,
  user_sub      TEXT NOT NULL,
  kind          TEXT NOT NULL,
  created_at    TEXT NOT NULL,
  expires_at    TEXT NOT NULL DEFAULT '',
  ack_at        TEXT NOT NULL DEFAULT '',
  closed_at     TEXT NOT NULL DEFAULT '',
  closed_reason TEXT NOT NULL DEFAULT '',
  place_name    TEXT NOT NULL DEFAULT '',
  task_name     TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_notifications_offen ON care_notifications(user_sub, closed_at);
CREATE INDEX IF NOT EXISTS idx_notifications_vorgang ON care_notifications(assignment_id);
-- push_devices: die Gerätekennungen von Firebase, mit denen sich eine
-- Benachrichtigung auch dann zustellen lässt, wenn die App nicht läuft.
-- Die Kennung ist eindeutig — dasselbe Gerät gehört immer nur einer Person
-- (siehe internal/db/geraete.go).
CREATE TABLE IF NOT EXISTS push_devices (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_sub   TEXT NOT NULL,
  token      TEXT NOT NULL,
  platform   TEXT NOT NULL DEFAULT 'android',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_token ON push_devices(token);
CREATE INDEX IF NOT EXISTS idx_devices_person ON push_devices(user_sub);
`)
	if err != nil {
		return err
	}
	// Nachträglich ergänzte Spalten: bestehende Datenbanken kennen sie noch
	// nicht, ein zweiter Aufruf meldet „duplicate column name".
	//
	// Die vier Spalten der einmaligen Aufgaben sind rein additiv und so
	// vorbelegt, dass jede Bestandsaufgabe unverändert als regelmäßige
	// Aufgabe weiterläuft (one_off=0, kein Termin, nichts abgeräumt).
	for _, stmt := range []string{
		`ALTER TABLE completions ADD COLUMN forced INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE care_tasks ADD COLUMN one_off INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE care_tasks ADD COLUMN due_date TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE care_tasks ADD COLUMN remove_when_done INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE care_tasks ADD COLUMN removed_at TEXT NOT NULL DEFAULT ''`,
		// Ort und Aufgabe im Klartext bei der Zustellung: Wird die Aufgabe
		// gelöscht, ist sie nicht mehr nachschlagbar — der Hinweis soll
		// trotzdem sagen können, worum es ging (siehe loeseNotificationsVomVorgang).
		`ALTER TABLE care_notifications ADD COLUMN place_name TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE care_notifications ADD COLUMN task_name TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := d.sql.Exec(stmt); err != nil && !istDoppelteSpalte(err) {
			return err
		}
	}
	if err := d.loeseNotificationsVomVorgang(); err != nil {
		return err
	}
	// Träger, Befähigungen und die Zuordnung der Bestandsdaten. Muss vor den
	// weiteren Bereichen laufen: Orte und Aufgaben bekommen hier ihre neuen
	// Spalten, und ohne sie sind sie nicht lesbar.
	if err := d.migrateTraeger(); err != nil {
		return err
	}
	// Weitere Bereiche bringen ihre Tabellen selbst mit (rein additiv).
	return d.migrateIdeen()
}

// istDoppelteSpalte erkennt den Fehler, mit dem SQLite ein bereits
// ausgeführtes ALTER TABLE ADD COLUMN quittiert. Genau dieser Fall ist beim
// Start einer bestehenden Datenbank der Normalfall.
func istDoppelteSpalte(err error) bool {
	return err != nil && strings.Contains(err.Error(), "duplicate column")
}

// loeseNotificationsVomVorgang nimmt care_notifications den Fremdschlüssel
// auf care_assignments.
//
// Warum: Wird eine Aufgabe gelöscht, räumt SQLite über zwei Kaskaden auch
// den Vorgang und mit ihm jede Zustellung weg. Genau dann muss aber jemand
// erfahren, dass die zugesagte Aufgabe entfallen ist — der Hinweis darf
// nicht mit dem Anlass verschwinden (#7). Die Spalte assignment_id bleibt
// als Bezug erhalten, nur eben ohne Zwang.
//
// Das ist der einzige Umbau am Bestand und läuft genau einmal: Fehlt der
// Fremdschlüssel schon, passiert nichts. SQLite kann Fremdschlüssel nicht
// nachträglich entfernen, deshalb der offizielle Weg über eine neue Tabelle.
func (d *DB) loeseNotificationsVomVorgang() error {
	var vorhanden int
	if err := d.sql.QueryRow(
		`SELECT COUNT(*) FROM pragma_foreign_key_list('care_notifications')`).Scan(&vorhanden); err != nil {
		return err
	}
	if vorhanden == 0 {
		return nil
	}
	// Fremdschlüsselprüfung aus, sonst zieht das DROP die Zeilen mit. Das
	// geht nur außerhalb einer Transaktion; danach wird sie wieder scharf
	// gestellt. Die Datenbank hat genau eine Verbindung (SetMaxOpenConns(1)),
	// der Server nimmt zu diesem Zeitpunkt noch keine Anfragen an.
	if _, err := d.sql.Exec(`PRAGMA foreign_keys=off`); err != nil {
		return err
	}
	defer func() { _, _ = d.sql.Exec(`PRAGMA foreign_keys=on`) }()

	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`
CREATE TABLE care_notifications_neu (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  assignment_id INTEGER NOT NULL,
  task_id       INTEGER NOT NULL,
  place_id      INTEGER NOT NULL,
  user_sub      TEXT NOT NULL,
  kind          TEXT NOT NULL,
  created_at    TEXT NOT NULL,
  expires_at    TEXT NOT NULL DEFAULT '',
  ack_at        TEXT NOT NULL DEFAULT '',
  closed_at     TEXT NOT NULL DEFAULT '',
  closed_reason TEXT NOT NULL DEFAULT '',
  place_name    TEXT NOT NULL DEFAULT '',
  task_name     TEXT NOT NULL DEFAULT ''
);
INSERT INTO care_notifications_neu
  (id,assignment_id,task_id,place_id,user_sub,kind,created_at,expires_at,ack_at,closed_at,closed_reason,place_name,task_name)
  SELECT id,assignment_id,task_id,place_id,user_sub,kind,created_at,expires_at,ack_at,closed_at,closed_reason,place_name,task_name
    FROM care_notifications;
DROP TABLE care_notifications;
ALTER TABLE care_notifications_neu RENAME TO care_notifications;
CREATE INDEX IF NOT EXISTS idx_notifications_offen ON care_notifications(user_sub, closed_at);
CREATE INDEX IF NOT EXISTS idx_notifications_vorgang ON care_notifications(assignment_id);
`); err != nil {
		return err
	}
	return tx.Commit()
}

const timeFormat = time.RFC3339

// --- Settings ---------------------------------------------------------------

// WateringFactor liest den globalen Hitze-Faktor (Standard 1.0).
// 0.5 bedeutet: Gieß-Schwellen halbieren sich (Hitzewelle).
func (d *DB) WateringFactor() (float64, error) {
	var v string
	err := d.sql.QueryRow(`SELECT value FROM settings WHERE key='watering_factor'`).Scan(&v)
	if err == sql.ErrNoRows {
		return 1.0, nil
	}
	if err != nil {
		return 1.0, err
	}
	var f float64
	if _, err := fmt.Sscanf(v, "%f", &f); err != nil || f <= 0 {
		return 1.0, nil
	}
	return f, nil
}

func (d *DB) SetWateringFactor(f float64) error {
	_, err := d.sql.Exec(`INSERT INTO settings(key,value) VALUES('watering_factor',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, fmt.Sprintf("%g", f))
	return err
}

// --- Places -----------------------------------------------------------------

func (d *DB) InsertPlace(p *model.Place) error {
	// Ein Ort ohne Träger gehört niemandem und wäre für niemanden sichtbar.
	// Statt ihn heimatlos anzulegen, landet er beim Platzhalter-Träger —
	// dieselbe Regel wie bei der Umstellung der Bestandsdaten.
	if p.TraegerID == 0 {
		dek, err := d.TraegerSicherstellen(model.SchluesselDEK, model.NameDEK)
		if err != nil {
			return err
		}
		p.TraegerID = dek.ID
	}
	res, err := d.sql.Exec(`INSERT INTO places(name,description,kind,lat,lon,active,created_at,traeger_id)
		VALUES(?,?,?,?,?,?,?,?)`,
		p.Name, p.Description, string(p.Kind), p.Lat, p.Lon, boolToInt(p.Active),
		p.CreatedAt.UTC().Format(timeFormat), p.TraegerID)
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	return err
}

func (d *DB) UpdatePlace(p *model.Place) error {
	res, err := d.sql.Exec(`UPDATE places SET name=?,description=?,kind=?,lat=?,lon=?,active=?,traeger_id=?
		WHERE id=?`,
		p.Name, p.Description, string(p.Kind), p.Lat, p.Lon, boolToInt(p.Active), p.TraegerID, p.ID)
	if err != nil {
		return err
	}
	return requireRow(res)
}

func (d *DB) DeletePlace(id int64) error {
	res, err := d.sql.Exec(`DELETE FROM places WHERE id=?`, id)
	if err != nil {
		return err
	}
	return requireRow(res)
}

func (d *DB) GetPlace(id int64) (*model.Place, error) {
	row := d.sql.QueryRow(`SELECT `+ortSpalten+` FROM places WHERE id=?`, id)
	return scanPlace(row)
}

func (d *DB) ListPlaces() ([]model.Place, error) {
	rows, err := d.sql.Query(`SELECT ` + ortSpalten + ` FROM places ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Place{}
	for rows.Next() {
		p, err := scanPlace(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

type scannable interface{ Scan(dest ...any) error }

const ortSpalten = `id,name,description,kind,lat,lon,active,created_at,traeger_id`

func scanPlace(row scannable) (*model.Place, error) {
	var p model.Place
	var active int
	var created, kind string
	err := row.Scan(&p.ID, &p.Name, &p.Description, &kind, &p.Lat, &p.Lon, &active, &created, &p.TraegerID)
	if err != nil {
		return nil, err
	}
	p.Kind = model.PlaceKind(kind)
	p.Active = active != 0
	p.CreatedAt, _ = time.Parse(timeFormat, created)
	return &p, nil
}

// --- CareTasks --------------------------------------------------------------

func (d *DB) InsertTask(t *model.CareTask) error {
	if t.Sichtbarkeit == "" {
		t.Sichtbarkeit = model.AufgabeOeffentlich
	}
	res, err := d.sql.Exec(`INSERT INTO care_tasks(place_id,kind,title,liters,interval_days,red_after_days,
			one_off,due_date,remove_when_done,active,created_at,sichtbarkeit,befaehigung_id)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.PlaceID, string(t.Kind), t.Title, t.Liters, t.IntervalDays, t.RedAfterDays,
		boolToInt(t.OneOff), zeitText(t.DueDate), boolToInt(t.RemoveWhenDone),
		boolToInt(t.Active), t.CreatedAt.UTC().Format(timeFormat),
		string(t.Sichtbarkeit), t.BefaehigungID)
	if err != nil {
		return err
	}
	t.ID, err = res.LastInsertId()
	return err
}

func (d *DB) UpdateTask(t *model.CareTask) error {
	if t.Sichtbarkeit == "" {
		t.Sichtbarkeit = model.AufgabeOeffentlich
	}
	res, err := d.sql.Exec(`UPDATE care_tasks SET kind=?,title=?,liters=?,interval_days=?,red_after_days=?,
			one_off=?,due_date=?,remove_when_done=?,active=?,sichtbarkeit=?,befaehigung_id=? WHERE id=?`,
		string(t.Kind), t.Title, t.Liters, t.IntervalDays, t.RedAfterDays,
		boolToInt(t.OneOff), zeitText(t.DueDate), boolToInt(t.RemoveWhenDone),
		boolToInt(t.Active), string(t.Sichtbarkeit), t.BefaehigungID, t.ID)
	if err != nil {
		return err
	}
	return requireRow(res)
}

// RemoveTask räumt eine erledigte einmalige Aufgabe ab: Sie verschwindet aus
// Karte, Liste und Verwaltung, bleibt aber als Zeile bestehen. Löschen wäre
// falsch — an der Aufgabe hängen die Erledigungen, und die zählen weiter für
// die Rangliste (#6).
func (d *DB) RemoveTask(id int64, at time.Time) error {
	res, err := d.sql.Exec(`UPDATE care_tasks SET removed_at=?, active=0 WHERE id=? AND removed_at=''`,
		at.UTC().Format(timeFormat), id)
	if err != nil {
		return err
	}
	return requireRow(res)
}

func (d *DB) DeleteTask(id int64) error {
	res, err := d.sql.Exec(`DELETE FROM care_tasks WHERE id=?`, id)
	if err != nil {
		return err
	}
	return requireRow(res)
}

const taskSpalten = `id,place_id,kind,title,liters,interval_days,red_after_days,
	one_off,due_date,remove_when_done,removed_at,active,created_at,sichtbarkeit,befaehigung_id`

// GetTask liefert auch abgeräumte Aufgaben — die Rangliste und die Historie
// müssen ihre Erledigungen weiter zuordnen können.
func (d *DB) GetTask(id int64) (*model.CareTask, error) {
	return scanTask(d.sql.QueryRow(`SELECT `+taskSpalten+` FROM care_tasks WHERE id=?`, id))
}

// ListTasks liefert alle laufenden Aufgaben, gruppierbar über PlaceID.
// Abgeräumte (erledigte einmalige) bleiben außen vor: Sie sind vorbei.
func (d *DB) ListTasks() ([]model.CareTask, error) {
	rows, err := d.sql.Query(`SELECT ` + taskSpalten + ` FROM care_tasks
		WHERE removed_at='' ORDER BY place_id, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.CareTask{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

func scanTask(row scannable) (*model.CareTask, error) {
	var t model.CareTask
	var active, oneOff, removeWhenDone int
	var created, kind, dueDate, removedAt, sichtbarkeit string
	err := row.Scan(&t.ID, &t.PlaceID, &kind, &t.Title, &t.Liters, &t.IntervalDays, &t.RedAfterDays,
		&oneOff, &dueDate, &removeWhenDone, &removedAt, &active, &created,
		&sichtbarkeit, &t.BefaehigungID)
	if err != nil {
		return nil, err
	}
	t.Sichtbarkeit = model.TaskSichtbarkeit(sichtbarkeit)
	t.Kind = model.TaskKind(kind)
	t.OneOff = oneOff != 0
	t.DueDate = zeitWert(dueDate)
	t.RemoveWhenDone = removeWhenDone != 0
	t.RemovedAt = zeitWert(removedAt)
	t.Active = active != 0
	t.CreatedAt, _ = time.Parse(timeFormat, created)
	return &t, nil
}

// --- Completions ------------------------------------------------------------

func (d *DB) InsertCompletion(c *model.Completion) error {
	res, err := d.sql.Exec(`INSERT INTO completions(task_id,user_sub,user_name,liters,note,done_at,forced) VALUES(?,?,?,?,?,?,?)`,
		c.TaskID, c.UserSub, c.UserName, c.Liters, c.Note, c.DoneAt.UTC().Format(timeFormat), boolToInt(c.Forced))
	if err != nil {
		return err
	}
	c.ID, err = res.LastInsertId()
	return err
}

// LastCompletions liefert je Aufgabe die letzte Erledigung.
func (d *DB) LastCompletions() (map[int64]model.Completion, error) {
	rows, err := d.sql.Query(`SELECT c.id,c.task_id,c.user_sub,c.user_name,c.liters,c.note,c.done_at,c.forced
		FROM completions c
		JOIN (SELECT task_id, MAX(done_at) AS m FROM completions GROUP BY task_id) latest
		  ON latest.task_id = c.task_id AND latest.m = c.done_at
		GROUP BY c.task_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]model.Completion{}
	for rows.Next() {
		c, err := scanCompletion(rows)
		if err != nil {
			return nil, err
		}
		out[c.TaskID] = *c
	}
	return out, rows.Err()
}

func (d *DB) ListCompletions(taskID int64, limit int) ([]model.Completion, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.sql.Query(`SELECT id,task_id,user_sub,user_name,liters,note,done_at,forced
		FROM completions WHERE task_id=? ORDER BY done_at DESC LIMIT ?`, taskID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Completion{}
	for rows.Next() {
		c, err := scanCompletion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func scanCompletion(row scannable) (*model.Completion, error) {
	var c model.Completion
	var done string
	var forced int
	err := row.Scan(&c.ID, &c.TaskID, &c.UserSub, &c.UserName, &c.Liters, &c.Note, &done, &forced)
	if err != nil {
		return nil, err
	}
	c.DoneAt, _ = time.Parse(timeFormat, done)
	c.Forced = forced != 0
	return &c, nil
}

// LastCompletion liefert die neueste Erledigung einer Aufgabe (oder nil).
func (d *DB) LastCompletion(taskID int64) (*model.Completion, error) {
	cs, err := d.ListCompletions(taskID, 1)
	if err != nil || len(cs) == 0 {
		return nil, err
	}
	return &cs[0], nil
}

// InsertCompletionIfFree trägt eine Erledigung nur ein, wenn die Aufgabe zum
// Zeitpunkt c.DoneAt nicht mehr gesperrt ist. Prüfen und Eintragen passieren
// in einer Transaktion — sonst rutschen bei einem Doppeltipp (oder einer
// Wiederholung im wackeligen Mobilfunknetz) zwei Meldungen gleichzeitig
// durch die Prüfung, und die Sperre wäre wirkungslos.
//
// ok=false heißt: die Sperre läuft noch, frei nennt ihr Ende.
func (d *DB) InsertCompletionIfFree(c *model.Completion, sperre time.Duration) (frei time.Time, ok bool, err error) {
	tx, err := d.sql.Begin()
	if err != nil {
		return time.Time{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var letzte sql.NullString
	if err := tx.QueryRow(`SELECT MAX(done_at) FROM completions WHERE task_id=?`, c.TaskID).Scan(&letzte); err != nil {
		return time.Time{}, false, err
	}
	if letzte.Valid {
		zuletzt, perr := time.Parse(timeFormat, letzte.String)
		if perr != nil {
			return time.Time{}, false, perr
		}
		if frei := zuletzt.Add(sperre); c.DoneAt.Before(frei) {
			return frei, false, nil
		}
	}
	res, err := tx.Exec(`INSERT INTO completions(task_id,user_sub,user_name,liters,note,done_at,forced) VALUES(?,?,?,?,?,?,?)`,
		c.TaskID, c.UserSub, c.UserName, c.Liters, c.Note, c.DoneAt.UTC().Format(timeFormat), boolToInt(c.Forced))
	if err != nil {
		return time.Time{}, false, err
	}
	if c.ID, err = res.LastInsertId(); err != nil {
		return time.Time{}, false, err
	}
	return time.Time{}, true, tx.Commit()
}

func requireRow(res sql.Result) error {
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
