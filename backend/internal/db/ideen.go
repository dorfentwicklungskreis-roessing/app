package db

import (
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Ideen-Sammlung: Wünsche aus dem Dorf, was die App können soll. Rein
// additiv — die laufende Datenbank bekommt die Tabelle beim nächsten Start
// einfach dazu, an allen bestehenden Tabellen ändert sich nichts.
func (d *DB) migrateIdeen() error {
	_, err := d.sql.Exec(`
CREATE TABLE IF NOT EXISTS ideen (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  name       TEXT NOT NULL DEFAULT '',
  email      TEXT NOT NULL DEFAULT '',
  wunsch     TEXT NOT NULL,
  quelle     TEXT NOT NULL DEFAULT 'website',
  user_sub   TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  status     TEXT NOT NULL DEFAULT 'neu',
  notiz      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ideen_stand ON ideen(status, created_at DESC);
`)
	return err
}

const ideenSpalten = `id,name,email,wunsch,quelle,user_sub,created_at,status,notiz`

func (d *DB) InsertIdee(i *model.Idee) error {
	res, err := d.sql.Exec(`INSERT INTO ideen(name,email,wunsch,quelle,user_sub,created_at,status,notiz)
		VALUES(?,?,?,?,?,?,?,?)`,
		i.Name, i.Email, i.Wunsch, string(i.Quelle), i.UserSub,
		i.CreatedAt.UTC().Format(timeFormat), string(i.Status), i.Notiz)
	if err != nil {
		return err
	}
	i.ID, err = res.LastInsertId()
	return err
}

// ListIdeen liefert die Wünsche, neueste zuerst. Ein leerer Stand heißt
// „alle“.
func (d *DB) ListIdeen(status model.IdeeStatus) ([]model.Idee, error) {
	abfrage := `SELECT ` + ideenSpalten + ` FROM ideen ORDER BY created_at DESC, id DESC`
	args := []any{}
	if status != "" {
		abfrage = `SELECT ` + ideenSpalten + ` FROM ideen WHERE status=? ORDER BY created_at DESC, id DESC`
		args = append(args, string(status))
	}
	rows, err := d.sql.Query(abfrage, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Idee{}
	for rows.Next() {
		i, err := scanIdee(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

func (d *DB) GetIdee(id int64) (*model.Idee, error) {
	return scanIdee(d.sql.QueryRow(`SELECT `+ideenSpalten+` FROM ideen WHERE id=?`, id))
}

// UpdateIdee schreibt Stand und interne Notiz zurück. Am eingereichten Text
// wird bewusst nichts geändert — er ist das, was jemand gesagt hat.
func (d *DB) UpdateIdee(i *model.Idee) error {
	res, err := d.sql.Exec(`UPDATE ideen SET status=?, notiz=? WHERE id=?`,
		string(i.Status), i.Notiz, i.ID)
	if err != nil {
		return err
	}
	return requireRow(res)
}

func (d *DB) DeleteIdee(id int64) error {
	res, err := d.sql.Exec(`DELETE FROM ideen WHERE id=?`, id)
	if err != nil {
		return err
	}
	return requireRow(res)
}

// CountIdeen zählt die Wünsche je Stand — für den Zähler in der
// Bereichsübersicht der Verwaltung.
func (d *DB) CountIdeen() (map[model.IdeeStatus]int, error) {
	rows, err := d.sql.Query(`SELECT status, COUNT(*) FROM ideen GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[model.IdeeStatus]int{}
	for rows.Next() {
		var stand string
		var n int
		if err := rows.Scan(&stand, &n); err != nil {
			return nil, err
		}
		out[model.IdeeStatus(stand)] = n
	}
	return out, rows.Err()
}

func scanIdee(row scannable) (*model.Idee, error) {
	var i model.Idee
	var quelle, status, erstellt string
	err := row.Scan(&i.ID, &i.Name, &i.Email, &i.Wunsch, &quelle, &i.UserSub, &erstellt, &status, &i.Notiz)
	if err != nil {
		return nil, err
	}
	i.Quelle = model.IdeeQuelle(quelle)
	i.Status = model.IdeeStatus(status)
	i.CreatedAt, _ = time.Parse(timeFormat, erstellt)
	return &i, nil
}
