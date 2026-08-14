package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Speicherung der Vergabe: Anmeldungen, Vorgänge und Zustellungen. Die
// Regeln selbst stehen im Paket vergabe; hier liegt nur, was SQL braucht.
//
// Zeitfelder sind Text im RFC3339-Format, ein leerer String heißt „nicht
// gesetzt". Das passt zum bestehenden Schema und erspart NULL-Sonderfälle
// beim Scannen.

// --- Anmeldungen ------------------------------------------------------------

const signupSpalten = `id,user_sub,place_id,task_kind,created_at`

// InsertSignup trägt eine Anmeldung ein. neu=false heißt: gab es schon
// (Doppeltipp, zweites Gerät) — das ist kein Fehler.
func (d *DB) InsertSignup(s *model.Signup) (neu bool, err error) {
	res, err := d.sql.Exec(`INSERT INTO care_signups(user_sub,place_id,task_kind,created_at)
		VALUES(?,?,?,?) ON CONFLICT(user_sub,place_id,task_kind) DO NOTHING`,
		s.UserSub, s.PlaceID, string(s.TaskKind), s.CreatedAt.UTC().Format(timeFormat))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		vorhanden, err := d.signup(s.UserSub, s.PlaceID, string(s.TaskKind))
		if err != nil {
			return false, err
		}
		*s = *vorhanden
		return false, nil
	}
	s.ID, err = res.LastInsertId()
	return true, err
}

func (d *DB) signup(userSub string, placeID int64, kind string) (*model.Signup, error) {
	row := d.sql.QueryRow(`SELECT `+signupSpalten+` FROM care_signups
		WHERE user_sub=? AND place_id=? AND task_kind=?`, userSub, placeID, kind)
	return scanSignup(row)
}

// DeleteSignups meldet eine Person wieder ab. kind leer = alle Anmeldungen
// dieser Person an diesem Ort.
func (d *DB) DeleteSignups(userSub string, placeID int64, kind string) (int, error) {
	abfrage := `DELETE FROM care_signups WHERE user_sub=? AND place_id=?`
	args := []any{userSub, placeID}
	if kind != "" {
		abfrage += ` AND task_kind=?`
		args = append(args, kind)
	}
	res, err := d.sql.Exec(abfrage, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (d *DB) ListSignups() ([]model.Signup, error) {
	return d.signupsWhere(``)
}

func (d *DB) ListSignupsByUser(userSub string) ([]model.Signup, error) {
	return d.signupsWhere(`WHERE user_sub=?`, userSub)
}

func (d *DB) ListSignupsForPlace(placeID int64) ([]model.Signup, error) {
	return d.signupsWhere(`WHERE place_id=?`, placeID)
}

func (d *DB) signupsWhere(where string, args ...any) ([]model.Signup, error) {
	rows, err := d.sql.Query(`SELECT `+signupSpalten+` FROM care_signups `+where+
		` ORDER BY place_id, task_kind, created_at, id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Signup{}
	for rows.Next() {
		s, err := scanSignup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

func scanSignup(row scannable) (*model.Signup, error) {
	var s model.Signup
	var kind, created string
	if err := row.Scan(&s.ID, &s.UserSub, &s.PlaceID, &kind, &created); err != nil {
		return nil, err
	}
	s.TaskKind = model.TaskKind(kind)
	s.CreatedAt, _ = time.Parse(timeFormat, created)
	return &s, nil
}

// --- Vorgänge ---------------------------------------------------------------

const assignmentSpalten = `a.id,a.task_id,a.state,a.created_at,a.next_offer_at,
	a.claimed_by,a.claimed_name,a.claimed_at,a.claim_until,a.ended_at,a.end_reason,
	(SELECT COUNT(*) FROM care_notifications n WHERE n.assignment_id=a.id AND n.kind='anfrage')`

func (d *DB) InsertAssignment(a *model.Assignment) error {
	res, err := d.sql.Exec(`INSERT INTO care_assignments(task_id,state,created_at,next_offer_at)
		VALUES(?,?,?,?)`, a.TaskID, string(a.State), a.CreatedAt.UTC().Format(timeFormat), zeitText(a.NextOfferAt))
	if err != nil {
		return err
	}
	a.ID, err = res.LastInsertId()
	return err
}

// ActiveAssignment liefert den laufenden Vorgang einer Aufgabe oder nil.
func (d *DB) ActiveAssignment(taskID int64) (*model.Assignment, error) {
	row := d.sql.QueryRow(`SELECT `+assignmentSpalten+` FROM care_assignments a
		WHERE a.task_id=? AND a.ended_at=''`, taskID)
	a, err := scanAssignment(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return a, err
}

// ActiveAssignments liefert alle laufenden Vorgänge.
func (d *DB) ActiveAssignments() ([]model.Assignment, error) {
	rows, err := d.sql.Query(`SELECT ` + assignmentSpalten + ` FROM care_assignments a
		WHERE a.ended_at='' ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Assignment{}
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (d *DB) GetAssignment(id int64) (*model.Assignment, error) {
	row := d.sql.QueryRow(`SELECT `+assignmentSpalten+` FROM care_assignments a WHERE a.id=?`, id)
	return scanAssignment(row)
}

// SetAssignmentQueue schreibt Stand und nächsten Anfragezeitpunkt fort.
func (d *DB) SetAssignmentQueue(id int64, state model.AssignmentState, nextOfferAt *time.Time) error {
	res, err := d.sql.Exec(`UPDATE care_assignments SET state=?,next_offer_at=? WHERE id=? AND ended_at=''`,
		string(state), zeitText(nextOfferAt), id)
	if err != nil {
		return err
	}
	return requireRow(res)
}

// ClaimAssignment vergibt den Vorgang — atomar. ok=false heißt: Jemand war
// schneller oder der Vorgang ist beendet. Prüfen und Setzen in einer Anweisung
// ist hier zwingend: Sagen zwei Leute im selben Moment zu, darf nur einer
// gewinnen.
func (d *DB) ClaimAssignment(id int64, userSub, userName string, at, until time.Time) (ok bool, err error) {
	res, err := d.sql.Exec(`UPDATE care_assignments
		SET state=?, claimed_by=?, claimed_name=?, claimed_at=?, claim_until=?, next_offer_at=''
		WHERE id=? AND ended_at='' AND claimed_by=''`,
		string(model.AssignmentClaimed), userSub, userName,
		at.UTC().Format(timeFormat), until.UTC().Format(timeFormat), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ReleaseAssignment gibt einen übernommenen Vorgang wieder frei.
func (d *DB) ReleaseAssignment(id int64, nextOfferAt time.Time) error {
	res, err := d.sql.Exec(`UPDATE care_assignments
		SET state=?, claimed_by='', claimed_name='', claimed_at='', claim_until='', next_offer_at=?
		WHERE id=? AND ended_at=''`,
		string(model.AssignmentOpen), nextOfferAt.UTC().Format(timeFormat), id)
	if err != nil {
		return err
	}
	return requireRow(res)
}

// EndAssignment beendet einen Vorgang.
func (d *DB) EndAssignment(id int64, at time.Time, grund string) error {
	res, err := d.sql.Exec(`UPDATE care_assignments
		SET state=?, ended_at=?, end_reason=?, next_offer_at=''
		WHERE id=? AND ended_at=''`,
		string(model.AssignmentEnded), at.UTC().Format(timeFormat), grund, id)
	if err != nil {
		return err
	}
	return requireRow(res)
}

func scanAssignment(row scannable) (*model.Assignment, error) {
	var a model.Assignment
	var state, created, next, claimedAt, claimUntil, ended string
	// ClaimedByName kommt zunächst aus dem, was bei der Zusage galt; die
	// Namensauflösung über die Profile setzt ihn danach ggf. neu (wie bei
	// den Erledigungen).
	err := row.Scan(&a.ID, &a.TaskID, &state, &created, &next,
		&a.ClaimedBy, &a.ClaimedByName, &claimedAt, &claimUntil, &ended, &a.EndReason, &a.AskedCount)
	if err != nil {
		return nil, err
	}
	a.State = model.AssignmentState(state)
	a.CreatedAt, _ = time.Parse(timeFormat, created)
	a.NextOfferAt = zeitWert(next)
	a.ClaimedAt = zeitWert(claimedAt)
	a.ClaimedUntil = zeitWert(claimUntil)
	a.EndedAt = zeitWert(ended)
	return &a, nil
}

// --- Zustellungen -----------------------------------------------------------

const notificationSpalten = `id,assignment_id,task_id,place_id,user_sub,kind,
	created_at,expires_at,ack_at,closed_at,closed_reason,place_name,task_name`

// InsertNotification legt eine Zustellung an. Ort und Aufgabe werden im
// Klartext mitgeschrieben: Wird die Aufgabe später gelöscht, ist der Hinweis
// sonst namenlos („ an “).
func (d *DB) InsertNotification(n *model.Notification) error {
	res, err := d.sql.Exec(`INSERT INTO care_notifications
		(assignment_id,task_id,place_id,user_sub,kind,created_at,expires_at,place_name,task_name)
		VALUES(?,?,?,?,?,?,?,?,?)`,
		n.AssignmentID, n.TaskID, n.PlaceID, n.UserSub, string(n.Kind),
		n.CreatedAt.UTC().Format(timeFormat), zeitText(n.ExpiresAt), n.PlaceName, n.TaskName)
	if err != nil {
		return err
	}
	n.ID, err = res.LastInsertId()
	return err
}

// OpenNotifications liefert alles, was für eine Person noch offen ist.
func (d *DB) OpenNotifications(userSub string) ([]model.Notification, error) {
	return d.notificationsWhere(`WHERE user_sub=? AND closed_at='' ORDER BY created_at, id`, userSub)
}

// NotificationsForAssignment liefert alle Zustellungen eines Vorgangs in der
// Reihenfolge, in der sie verschickt wurden.
func (d *DB) NotificationsForAssignment(assignmentID int64) ([]model.Notification, error) {
	return d.notificationsWhere(`WHERE assignment_id=? ORDER BY created_at, id`, assignmentID)
}

func (d *DB) GetNotification(id int64) (*model.Notification, error) {
	row := d.sql.QueryRow(`SELECT `+notificationSpalten+` FROM care_notifications WHERE id=?`, id)
	return scanNotification(row)
}

func (d *DB) notificationsWhere(where string, args ...any) ([]model.Notification, error) {
	rows, err := d.sql.Query(`SELECT `+notificationSpalten+` FROM care_notifications `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Notification{}
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

// CloseOpenNotifications schließt alle offenen Zustellungen eines Vorgangs
// (der Vorgang ist erledigt, übernommen oder entfallen). ausser bleibt
// stehen — damit ein Hinweis, den wir gerade erst angelegt haben, nicht im
// selben Zug wieder verschwindet.
func (d *DB) CloseOpenNotifications(assignmentID int64, at time.Time, grund string, ausser ...int64) error {
	abfrage := `UPDATE care_notifications SET closed_at=?, closed_reason=?
		WHERE assignment_id=? AND closed_at=''`
	args := []any{at.UTC().Format(timeFormat), grund, assignmentID}
	if len(ausser) > 0 {
		platzhalter := make([]string, len(ausser))
		for i, id := range ausser {
			platzhalter[i] = "?"
			args = append(args, id)
		}
		abfrage += ` AND id NOT IN (` + strings.Join(platzhalter, ",") + `)`
	}
	_, err := d.sql.Exec(abfrage, args...)
	return err
}

// CloseNotification schließt eine einzelne Zustellung.
func (d *DB) CloseNotification(id int64, at time.Time, grund string) error {
	_, err := d.sql.Exec(`UPDATE care_notifications SET closed_at=?, closed_reason=?
		WHERE id=? AND closed_at=''`, at.UTC().Format(timeFormat), grund, id)
	return err
}

// AckNotification bestätigt den Empfang. schliessen=true beendet die
// Zustellung zusätzlich — das gilt für Hinweise, die nach dem Lesen erledigt
// sind. Anfragen bleiben offen, bis der Vorgang sie schließt.
func (d *DB) AckNotification(id int64, at time.Time, schliessen bool) error {
	abfrage := `UPDATE care_notifications SET ack_at=? WHERE id=?`
	args := []any{at.UTC().Format(timeFormat), id}
	if schliessen {
		abfrage = `UPDATE care_notifications SET ack_at=?, closed_at=?, closed_reason=? WHERE id=?`
		args = []any{at.UTC().Format(timeFormat), at.UTC().Format(timeFormat), model.CloseConfirmed, id}
	}
	res, err := d.sql.Exec(abfrage, args...)
	if err != nil {
		return err
	}
	return requireRow(res)
}

func scanNotification(row scannable) (*model.Notification, error) {
	var n model.Notification
	var kind, created, expires, ack, closed string
	err := row.Scan(&n.ID, &n.AssignmentID, &n.TaskID, &n.PlaceID, &n.UserSub, &kind,
		&created, &expires, &ack, &closed, &n.ClosedReason, &n.PlaceName, &n.TaskName)
	if err != nil {
		return nil, err
	}
	n.Kind = model.NotificationKind(kind)
	n.CreatedAt, _ = time.Parse(timeFormat, created)
	n.ExpiresAt = zeitWert(expires)
	n.AcknowledgedAt = zeitWert(ack)
	n.ClosedAt = zeitWert(closed)
	return &n, nil
}

// --- Zeitpunkte für die Reihenfolge ----------------------------------------

// LastCompletionPerUser liefert je Person die letzte Erledigung — egal an
// welchem Ort. Grundlage der fairen Reihenfolge.
func (d *DB) LastCompletionPerUser() (map[string]time.Time, error) {
	return d.zeitpunkteJeNutzer(`SELECT user_sub, MAX(done_at) FROM completions GROUP BY user_sub`)
}

// LastRequestPerUser liefert je Person die letzte Anfrage (auch Rundrufe).
func (d *DB) LastRequestPerUser() (map[string]time.Time, error) {
	return d.zeitpunkteJeNutzer(`SELECT user_sub, MAX(created_at) FROM care_notifications
		WHERE kind IN ('anfrage','rundruf') GROUP BY user_sub`)
}

func (d *DB) zeitpunkteJeNutzer(abfrage string) (map[string]time.Time, error) {
	rows, err := d.sql.Query(abfrage)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]time.Time{}
	for rows.Next() {
		var sub string
		var wann sql.NullString
		if err := rows.Scan(&sub, &wann); err != nil {
			return nil, err
		}
		if wann.Valid {
			if t, err := time.Parse(timeFormat, wann.String); err == nil {
				out[sub] = t
			}
		}
	}
	return out, rows.Err()
}

// --- Einstellungen der Vergabe ---------------------------------------------

const (
	keyOfferMinutes = "assign_offer_minutes"
	keyClaimHours   = "assign_claim_hours"
	keyQuietFrom    = "assign_quiet_from"
	keyQuietTo      = "assign_quiet_to"
)

// AssignmentRules liest die Stellschrauben; fehlende Werte bleiben bei der
// Vorgabe (1 h Staffelung, 24 h Zusagefrist, Ruhe von 21 bis 7 Uhr).
func (d *DB) AssignmentRules() (model.AssignmentRules, error) {
	r := model.DefaultAssignmentRules()
	werte, err := d.settings(keyOfferMinutes, keyClaimHours, keyQuietFrom, keyQuietTo)
	if err != nil {
		return r, err
	}
	if v, ok := zahlWert(werte[keyOfferMinutes]); ok {
		r.OfferInterval = time.Duration(v) * time.Minute
	}
	if v, ok := zahlWert(werte[keyClaimHours]); ok {
		r.ClaimDuration = time.Duration(v) * time.Hour
	}
	if v, ok := zahlWert(werte[keyQuietFrom]); ok {
		r.QuietFrom = v
	}
	if v, ok := zahlWert(werte[keyQuietTo]); ok {
		r.QuietTo = v
	}
	// Ein kaputter Wert in der Datenbank darf die Vergabe nicht anhalten.
	if err := r.Validate(); err != nil {
		return model.DefaultAssignmentRules(), nil
	}
	return r, nil
}

func (d *DB) SetAssignmentRules(r model.AssignmentRules) error {
	if err := r.Validate(); err != nil {
		return err
	}
	werte := map[string]int{
		keyOfferMinutes: int(r.OfferInterval / time.Minute),
		keyClaimHours:   int(r.ClaimDuration / time.Hour),
		keyQuietFrom:    r.QuietFrom,
		keyQuietTo:      r.QuietTo,
	}
	for key, v := range werte {
		if _, err := d.sql.Exec(`INSERT INTO settings(key,value) VALUES(?,?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, fmt.Sprintf("%d", v)); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) settings(keys ...string) (map[string]string, error) {
	platzhalter := make([]string, len(keys))
	args := make([]any, len(keys))
	for i, k := range keys {
		platzhalter[i] = "?"
		args[i] = k
	}
	rows, err := d.sql.Query(`SELECT key,value FROM settings WHERE key IN (`+
		strings.Join(platzhalter, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func zahlWert(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	var n int
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, false
	}
	return n, true
}

// --- Zeit-Hilfen ------------------------------------------------------------

func zeitText(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(timeFormat)
}

func zeitWert(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(timeFormat, s)
	if err != nil {
		return nil
	}
	return &t
}
