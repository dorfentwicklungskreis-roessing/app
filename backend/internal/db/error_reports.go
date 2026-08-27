package db

import (
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Error reports from the apps: what went wrong, on which device, and what the
// person wrote about it. Purely additive — a running database simply gets the
// table at the next start, nothing changes on the existing ones.
func (d *DB) migrateErrorReports() error {
	_, err := d.sql.Exec(`
CREATE TABLE IF NOT EXISTS error_reports (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  kind         TEXT NOT NULL DEFAULT 'unexpected',
  message      TEXT NOT NULL DEFAULT '',
  detail       TEXT NOT NULL DEFAULT '',
  comment      TEXT NOT NULL DEFAULT '',
  area         TEXT NOT NULL DEFAULT '',
  platform     TEXT NOT NULL DEFAULT '',
  app_version  TEXT NOT NULL DEFAULT '',
  os_version   TEXT NOT NULL DEFAULT '',
  device_model TEXT NOT NULL DEFAULT '',
  occurred_at  TEXT NOT NULL,
  created_at   TEXT NOT NULL,
  user_sub     TEXT NOT NULL DEFAULT '',
  user_name    TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'new',
  note         TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_error_reports_state ON error_reports(status, created_at DESC);
`)
	return err
}

const errorReportColumns = `id,kind,message,detail,comment,area,platform,app_version,os_version,` +
	`device_model,occurred_at,created_at,user_sub,user_name,status,note`

func (d *DB) InsertErrorReport(e *model.ErrorReport) error {
	res, err := d.sql.Exec(`INSERT INTO error_reports(kind,message,detail,comment,area,platform,
		app_version,os_version,device_model,occurred_at,created_at,user_sub,user_name,status,note)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		string(e.Kind), e.Message, e.Detail, e.Comment, e.Area, e.Platform,
		e.AppVersion, e.OSVersion, e.DeviceModel,
		e.OccurredAt.UTC().Format(timeFormat), e.CreatedAt.UTC().Format(timeFormat),
		e.UserSub, e.UserName, string(e.Status), e.Note)
	if err != nil {
		return err
	}
	e.ID, err = res.LastInsertId()
	return err
}

// ListErrorReports returns the reports, newest first. An empty status means
// "all", an empty kind likewise.
func (d *DB) ListErrorReports(status model.ErrorReportStatus, kind model.ErrorReportKind) ([]model.ErrorReport, error) {
	query := `SELECT ` + errorReportColumns + ` FROM error_reports`
	where, args := []string{}, []any{}
	if status != "" {
		where = append(where, `status=?`)
		args = append(args, string(status))
	}
	if kind != "" {
		where = append(where, `kind=?`)
		args = append(args, string(kind))
	}
	for i, w := range where {
		if i == 0 {
			query += ` WHERE ` + w
			continue
		}
		query += ` AND ` + w
	}
	query += ` ORDER BY created_at DESC, id DESC`
	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.ErrorReport{}
	for rows.Next() {
		e, err := scanErrorReport(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (d *DB) GetErrorReport(id int64) (*model.ErrorReport, error) {
	return scanErrorReport(d.sql.QueryRow(`SELECT `+errorReportColumns+
		` FROM error_reports WHERE id=?`, id))
}

// UpdateErrorReport writes back state and internal note. The reported facts
// stay untouched — they are what a device observed.
func (d *DB) UpdateErrorReport(e *model.ErrorReport) error {
	res, err := d.sql.Exec(`UPDATE error_reports SET status=?, note=? WHERE id=?`,
		string(e.Status), e.Note, e.ID)
	if err != nil {
		return err
	}
	return requireRow(res)
}

func (d *DB) DeleteErrorReport(id int64) error {
	res, err := d.sql.Exec(`DELETE FROM error_reports WHERE id=?`, id)
	if err != nil {
		return err
	}
	return requireRow(res)
}

// CountErrorReports counts the reports per state — for the filters and for
// the counter on the overview page of the administration.
func (d *DB) CountErrorReports() (map[model.ErrorReportStatus]int, error) {
	rows, err := d.sql.Query(`SELECT status, COUNT(*) FROM error_reports GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[model.ErrorReportStatus]int{}
	for rows.Next() {
		var state string
		var n int
		if err := rows.Scan(&state, &n); err != nil {
			return nil, err
		}
		out[model.ErrorReportStatus(state)] = n
	}
	return out, rows.Err()
}

func scanErrorReport(row scannable) (*model.ErrorReport, error) {
	var e model.ErrorReport
	var kind, occurred, created, status string
	err := row.Scan(&e.ID, &kind, &e.Message, &e.Detail, &e.Comment, &e.Area, &e.Platform,
		&e.AppVersion, &e.OSVersion, &e.DeviceModel, &occurred, &created,
		&e.UserSub, &e.UserName, &status, &e.Note)
	if err != nil {
		return nil, err
	}
	e.Kind = model.ErrorReportKind(kind)
	e.Status = model.ErrorReportStatus(status)
	e.OccurredAt, _ = time.Parse(timeFormat, occurred)
	e.CreatedAt, _ = time.Parse(timeFormat, created)
	return &e, nil
}
