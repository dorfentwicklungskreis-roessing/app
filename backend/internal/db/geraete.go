package db

import (
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Speicherung der Gerätekennungen für den Push-Versand.
//
// Die Kennung ist eindeutig: Sie beschreibt genau eine App-Installation auf
// genau einem Gerät. Meldet sie sich für eine andere Person an, ist das Gerät
// weitergegeben oder jemand anderes hat sich angemeldet — dann wechselt der
// Eintrag den Besitzer und der Vorbesitzer bekommt nichts mehr.

const deviceSpalten = `id,user_sub,token,platform,created_at,updated_at`

// UpsertDevice trägt eine Kennung ein oder frischt sie auf.
// neu=false heißt: Die Kennung war schon bekannt (App-Start, Erneuerung).
//
// Nachsehen und Schreiben liegen in einer Transaktion: Die App meldet ihr
// Gerät bei jedem Start, und zwei gleichzeitige Starts (zweite Instanz,
// Wiederholung im wackeligen Netz) dürfen keinen zweiten Eintrag ergeben.
func (d *DB) UpsertDevice(userSub, token, platform string, at time.Time) (neu bool, err error) {
	zeit := at.UTC().Format(timeFormat)
	tx, err := d.sql.Begin()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var vorhanden int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM push_devices WHERE token=?`, token).Scan(&vorhanden); err != nil {
		return false, err
	}
	if vorhanden == 0 {
		_, err = tx.Exec(`INSERT INTO push_devices(user_sub,token,platform,created_at,updated_at)
			VALUES(?,?,?,?,?)`, userSub, token, platform, zeit, zeit)
	} else {
		_, err = tx.Exec(`UPDATE push_devices SET user_sub=?, platform=?, updated_at=? WHERE token=?`,
			userSub, platform, zeit, token)
	}
	if err != nil {
		return false, err
	}
	return vorhanden == 0, tx.Commit()
}

// DevicesForUser liefert alle Geräte einer Person.
func (d *DB) DevicesForUser(userSub string) ([]model.Device, error) {
	rows, err := d.sql.Query(`SELECT `+deviceSpalten+` FROM push_devices
		WHERE user_sub=? ORDER BY updated_at DESC, id`, userSub)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Device{}
	for rows.Next() {
		var g model.Device
		var erstellt, geaendert string
		if err := rows.Scan(&g.ID, &g.UserSub, &g.Token, &g.Platform, &erstellt, &geaendert); err != nil {
			return nil, err
		}
		g.CreatedAt, _ = time.Parse(timeFormat, erstellt)
		g.UpdatedAt, _ = time.Parse(timeFormat, geaendert)
		out = append(out, g)
	}
	return out, rows.Err()
}

// DeleteDevice meldet ein Gerät ab. Abgemeldet wird nur das eigene: Eine
// fremde Kennung zu kennen, darf nicht reichen, um jemanden stumm zu stellen.
func (d *DB) DeleteDevice(userSub, token string) (int, error) {
	res, err := d.sql.Exec(`DELETE FROM push_devices WHERE user_sub=? AND token=?`, userSub, token)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// DeleteDeviceToken entfernt eine Kennung, die Google als ungültig gemeldet
// hat (App deinstalliert, Kennung ausgetauscht). Dass sie schon weg ist, ist
// kein Fehler.
func (d *DB) DeleteDeviceToken(token string) error {
	_, err := d.sql.Exec(`DELETE FROM push_devices WHERE token=?`, token)
	return err
}
