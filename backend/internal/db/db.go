// Package db kapselt den SQLite-Zugriff (WAL-Modus) und die Migrationen.
package db

import (
	"database/sql"
	"fmt"
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
  done_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_completions_task_time ON completions(task_id, done_at DESC);
CREATE TABLE IF NOT EXISTS settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`)
	return err
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
	res, err := d.sql.Exec(`INSERT INTO places(name,description,kind,lat,lon,active,created_at)
		VALUES(?,?,?,?,?,?,?)`,
		p.Name, p.Description, string(p.Kind), p.Lat, p.Lon, boolToInt(p.Active), p.CreatedAt.UTC().Format(timeFormat))
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	return err
}

func (d *DB) UpdatePlace(p *model.Place) error {
	res, err := d.sql.Exec(`UPDATE places SET name=?,description=?,kind=?,lat=?,lon=?,active=? WHERE id=?`,
		p.Name, p.Description, string(p.Kind), p.Lat, p.Lon, boolToInt(p.Active), p.ID)
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
	row := d.sql.QueryRow(`SELECT id,name,description,kind,lat,lon,active,created_at FROM places WHERE id=?`, id)
	return scanPlace(row)
}

func (d *DB) ListPlaces() ([]model.Place, error) {
	rows, err := d.sql.Query(`SELECT id,name,description,kind,lat,lon,active,created_at FROM places ORDER BY name`)
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

func scanPlace(row scannable) (*model.Place, error) {
	var p model.Place
	var active int
	var created, kind string
	err := row.Scan(&p.ID, &p.Name, &p.Description, &kind, &p.Lat, &p.Lon, &active, &created)
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
	res, err := d.sql.Exec(`INSERT INTO care_tasks(place_id,kind,title,liters,interval_days,red_after_days,active,created_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		t.PlaceID, string(t.Kind), t.Title, t.Liters, t.IntervalDays, t.RedAfterDays, boolToInt(t.Active), t.CreatedAt.UTC().Format(timeFormat))
	if err != nil {
		return err
	}
	t.ID, err = res.LastInsertId()
	return err
}

func (d *DB) UpdateTask(t *model.CareTask) error {
	res, err := d.sql.Exec(`UPDATE care_tasks SET kind=?,title=?,liters=?,interval_days=?,red_after_days=?,active=? WHERE id=?`,
		string(t.Kind), t.Title, t.Liters, t.IntervalDays, t.RedAfterDays, boolToInt(t.Active), t.ID)
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

func (d *DB) GetTask(id int64) (*model.CareTask, error) {
	row := d.sql.QueryRow(`SELECT id,place_id,kind,title,liters,interval_days,red_after_days,active,created_at FROM care_tasks WHERE id=?`, id)
	return scanTask(row)
}

// ListTasks liefert alle Aufgaben, gruppierbar über PlaceID.
func (d *DB) ListTasks() ([]model.CareTask, error) {
	rows, err := d.sql.Query(`SELECT id,place_id,kind,title,liters,interval_days,red_after_days,active,created_at FROM care_tasks ORDER BY place_id, id`)
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
	var active int
	var created, kind string
	err := row.Scan(&t.ID, &t.PlaceID, &kind, &t.Title, &t.Liters, &t.IntervalDays, &t.RedAfterDays, &active, &created)
	if err != nil {
		return nil, err
	}
	t.Kind = model.TaskKind(kind)
	t.Active = active != 0
	t.CreatedAt, _ = time.Parse(timeFormat, created)
	return &t, nil
}

// --- Completions ------------------------------------------------------------

func (d *DB) InsertCompletion(c *model.Completion) error {
	res, err := d.sql.Exec(`INSERT INTO completions(task_id,user_sub,user_name,liters,note,done_at) VALUES(?,?,?,?,?,?)`,
		c.TaskID, c.UserSub, c.UserName, c.Liters, c.Note, c.DoneAt.UTC().Format(timeFormat))
	if err != nil {
		return err
	}
	c.ID, err = res.LastInsertId()
	return err
}

// LastCompletions liefert je Aufgabe die letzte Erledigung.
func (d *DB) LastCompletions() (map[int64]model.Completion, error) {
	rows, err := d.sql.Query(`SELECT c.id,c.task_id,c.user_sub,c.user_name,c.liters,c.note,c.done_at
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
	rows, err := d.sql.Query(`SELECT id,task_id,user_sub,user_name,liters,note,done_at
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
	err := row.Scan(&c.ID, &c.TaskID, &c.UserSub, &c.UserName, &c.Liters, &c.Note, &done)
	if err != nil {
		return nil, err
	}
	c.DoneAt, _ = time.Parse(timeFormat, done)
	return &c, nil
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
