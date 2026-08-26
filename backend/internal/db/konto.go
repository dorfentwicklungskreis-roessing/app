package db

import (
	"time"
)

// Löschen eines Kontos: alles, was eine Person ausmacht, verschwindet — die
// Arbeit, die sie fürs Dorf getan hat, bleibt anonym stehen.
//
// Warum die Erledigungen *nicht* mitgelöscht werden: An ihnen hängen die
// Gesamtsummen des Dorfes und die Historie der Orte („zuletzt gegossen am
// …“). Sie zu löschen hieße, die Arbeit anderer zu verfälschen — eine
// gemeinsame Bilanz, aus der jemand nachträglich Zeilen entfernt, stimmt
// nicht mehr. Sie unter Namen zu behalten hieße umgekehrt, das Löschen zu
// verweigern. Also bleibt die Zeile, der Name wird ersetzt und die Kennung
// entfernt: Die Rangliste bleibt stimmig, und die Person verschwindet.
//
// Dass danach alle Gelöschten in der Rangliste zu einer Zeile
// zusammenfallen (Gruppierung nach Kennung und Name), ist gewollt: Ein
// Ersatzschlüssel je Person wäre wieder ein Personenbezug.

// Loeschbilanz sagt, was beim Löschen eines Kontos passiert ist. Nur zur
// Rückmeldung und für die Tests — sie enthält keine personenbezogenen Daten.
type Loeschbilanz struct {
	Profil             bool `json:"profil"`
	Geraete            int  `json:"geraete"`
	Anmeldungen        int  `json:"anmeldungen"`
	Benachrichtigungen int  `json:"benachrichtigungen"`
	Erledigungen       int  `json:"erledigungen"`
	Zusagen            int  `json:"zusagen"`
	Befaehigungen      int  `json:"befaehigungen"`
	Ideen              int  `json:"ideen"`
}

// KontoLoeschen entfernt alle Daten einer Person und anonymisiert das, was
// aus fachlichen Gründen bleiben muss. Alles in einer Transaktion: Ein halb
// gelöschtes Konto wäre das Schlechteste von beidem.
//
// ersatzName ist der Name, unter dem Erledigungen und beendete Zusagen
// weiterlaufen („Ehemaliges Mitglied“).
//
// Ein zweiter Aufruf ist unschädlich — dann ist schlicht nichts mehr da.
func (d *DB) KontoLoeschen(userSub, ersatzName string, now time.Time) (Loeschbilanz, error) {
	var b Loeschbilanz
	if userSub == "" {
		return b, nil
	}
	zeit := now.UTC().Format(timeFormat)

	tx, err := d.sql.Begin()
	if err != nil {
		return b, err
	}
	defer func() { _ = tx.Rollback() }()

	zaehle := func(abfrage string, args ...any) (int, error) {
		res, err := tx.Exec(abfrage, args...)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		return int(n), nil
	}

	// 1. Erledigungen: bleiben, aber ohne Kennung und ohne Namen.
	if b.Erledigungen, err = zaehle(
		`UPDATE completions SET user_sub='', user_name=? WHERE user_sub=?`,
		ersatzName, userSub); err != nil {
		return b, err
	}

	// 2. Laufende Zusagen werden freigegeben, statt anonym festzuhängen:
	// Sonst wartet ein Blumenkasten auf jemanden, den es nicht mehr gibt.
	// Die Vergabe fragt beim nächsten Durchlauf die Übrigen.
	frei, err := zaehle(`UPDATE care_assignments
		SET state='offen', claimed_by='', claimed_name='', claimed_at='', claim_until='',
		    next_offer_at=?
		WHERE claimed_by=? AND ended_at=''`, zeit, userSub)
	if err != nil {
		return b, err
	}
	// Beendete Vorgänge sind Historie — dort bleibt nur der Ersatzname.
	anonym, err := zaehle(`UPDATE care_assignments
		SET claimed_by='', claimed_name=? WHERE claimed_by=?`, ersatzName, userSub)
	if err != nil {
		return b, err
	}
	b.Zusagen = frei + anonym

	// 3. Zustellungen (Anfragen und Hinweise) sind reine Postfach-Daten und
	// gehen ersatzlos weg.
	if b.Benachrichtigungen, err = zaehle(
		`DELETE FROM care_notifications WHERE user_sub=?`, userSub); err != nil {
		return b, err
	}

	// 4. Helfer-Eintragungen („ich kümmere mich mit“).
	if b.Anmeldungen, err = zaehle(
		`DELETE FROM care_signups WHERE user_sub=?`, userSub); err != nil {
		return b, err
	}

	// 5. Gerätekennungen — damit hört auch der Push sofort auf.
	if b.Geraete, err = zaehle(
		`DELETE FROM push_devices WHERE user_sub=?`, userSub); err != nil {
		return b, err
	}

	// 6. Befähigungen samt Antrag, Begründung und interner Notiz. Eine
	// Einweisung gilt der Person; ohne sie hat der Nachweis keinen Zweck.
	if b.Befaehigungen, err = zaehle(
		`DELETE FROM befaehigungs_antraege WHERE user_sub=?`, userSub); err != nil {
		return b, err
	}

	// 7. Eingereichte Wünsche bleiben (die Verwaltung arbeitet sie ab),
	// verlieren aber Kennung, Name und E-Mail.
	if b.Ideen, err = zaehle(
		`UPDATE ideen SET user_sub='', name='', email='' WHERE user_sub=?`, userSub); err != nil {
		return b, err
	}

	// 8. Zuletzt das Profil selbst.
	n, err := zaehle(`DELETE FROM profiles WHERE user_sub=?`, userSub)
	if err != nil {
		return b, err
	}
	b.Profil = n > 0

	return b, tx.Commit()
}
