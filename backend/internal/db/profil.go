package db

import (
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Speicherung der Profile. Die Tabelle kommt additiv dazu (CREATE TABLE IF
// NOT EXISTS in migrate()); an completions und care_tasks ändert sich nichts,
// die laufende Produktions-Datenbank verliert also nichts.

const profilSpalten = `user_sub,display_name,nickname,phone,email,note,
	vis_display_name,vis_nickname,vis_phone,vis_email,vis_note,token_name,updated_at`

// GetProfile liefert das Profil einer Person; sql.ErrNoRows, wenn es keins gibt.
func (d *DB) GetProfile(userSub string) (*model.Profile, error) {
	row := d.sql.QueryRow(`SELECT `+profilSpalten+` FROM profiles WHERE user_sub=?`, userSub)
	return scanProfile(row)
}

// UpsertProfile legt das Profil an oder ersetzt es.
func (d *DB) UpsertProfile(p *model.Profile) error {
	_, err := d.sql.Exec(`INSERT INTO profiles(`+profilSpalten+`)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(user_sub) DO UPDATE SET
		  display_name=excluded.display_name,
		  nickname=excluded.nickname,
		  phone=excluded.phone,
		  email=excluded.email,
		  note=excluded.note,
		  vis_display_name=excluded.vis_display_name,
		  vis_nickname=excluded.vis_nickname,
		  vis_phone=excluded.vis_phone,
		  vis_email=excluded.vis_email,
		  vis_note=excluded.vis_note,
		  token_name=excluded.token_name,
		  updated_at=excluded.updated_at`,
		p.UserSub, p.DisplayName, p.Nickname, p.Phone, p.Email, p.Note,
		string(p.Visibility.DisplayName), string(p.Visibility.Nickname),
		string(p.Visibility.Phone), string(p.Visibility.Email), string(p.Visibility.Note),
		p.TokenName, p.UpdatedAt.UTC().Format(timeFormat))
	return err
}

// ListProfiles liefert alle Profile, sortiert nach dem angezeigten Namen.
func (d *DB) ListProfiles() ([]model.Profile, error) {
	rows, err := d.sql.Query(`SELECT ` + profilSpalten + ` FROM profiles
		ORDER BY LOWER(CASE WHEN nickname <> '' THEN nickname
		                    WHEN display_name <> '' THEN display_name
		                    ELSE token_name END), user_sub`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Profile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// NameResolver lädt alle Profile für die Namensauflösung in Rangliste und
// Historie. Bei Dorfgröße ist das eine Handvoll Zeilen — eine Abfrage je
// Seitenaufbau genügt.
func (d *DB) NameResolver() (model.NameResolver, error) {
	ps, err := d.ListProfiles()
	if err != nil {
		return nil, err
	}
	out := make(model.NameResolver, len(ps))
	for _, p := range ps {
		out[p.UserSub] = p
	}
	return out, nil
}

func scanProfile(row scannable) (*model.Profile, error) {
	var p model.Profile
	var visName, visNick, visPhone, visMail, visNote, updated string
	err := row.Scan(&p.UserSub, &p.DisplayName, &p.Nickname, &p.Phone, &p.Email, &p.Note,
		&visName, &visNick, &visPhone, &visMail, &visNote, &p.TokenName, &updated)
	if err != nil {
		return nil, err
	}
	p.Visibility = model.ProfileVisibility{
		DisplayName: sichtbarkeitOder(visName, model.VisibilityVillage),
		Nickname:    sichtbarkeitOder(visNick, model.VisibilityVillage),
		Phone:       sichtbarkeitOder(visPhone, model.VisibilityAdmins),
		Email:       sichtbarkeitOder(visMail, model.VisibilityAdmins),
		Note:        sichtbarkeitOder(visNote, model.VisibilityAdmins),
	}
	p.UpdatedAt, _ = time.Parse(timeFormat, updated)
	return &p, nil
}

// sichtbarkeitOder fällt bei unbekannten Werten auf die zurückhaltendere
// Vorbelegung zurück: Ein kaputter Wert in der Datenbank darf nie dazu
// führen, dass etwas ungewollt sichtbar wird.
func sichtbarkeitOder(wert string, standard model.Visibility) model.Visibility {
	v := model.Visibility(wert)
	if model.ValidVisibility(v) {
		return v
	}
	return standard
}
