package db

import (
	"fmt"
	"strings"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Auswertungen für die Rangliste. Alles wird per GROUP BY in SQLite
// aggregiert — Go iteriert nie über einzelne Erledigungen.
//
// Zeitstempel liegen als RFC3339 in UTC in der Datenbank ("2026-08-12T06:00:00Z");
// weil das Format fixe Breite hat, ist der lexikografische Vergleich in SQL
// identisch mit dem chronologischen.

func sqlTime(t time.Time) string { return t.UTC().Format(timeFormat) }

// Gruppiert wird nach Person UND gemeldetem Namen: Erledigungen, die ein
// Admin telefonisch für jemanden einträgt (MCP-Tool erledigung_melden mit
// name), laufen unter dessen Zitadel-Kennung, sollen aber der genannten
// Person gutgeschrieben werden — nicht dem Admin.
const personKey = "user_sub || char(10) || user_name"

// PersonKey bildet denselben Schlüssel in Go (für die Zuordnung der
// Auszeichnungen zu den Ranglisten-Zeilen).
func PersonKey(userSub, userName string) string { return userSub + "\n" + userName }

// Leaderboard aggregiert die Erledigungen im Zeitraum [from, to) je Nutzer,
// sortiert nach Anzahl, bei Gleichstand nach Litern (dann nach Name).
func (d *DB) Leaderboard(from, to time.Time) ([]model.LeaderboardEntry, error) {
	// Bare-Column-Regel von SQLite: steht MAX() in der Auswahl, stammen die
	// nicht aggregierten Spalten (hier user_name) aus genau dieser Zeile —
	// wir bekommen also den zuletzt gemeldeten Anzeigenamen.
	rows, err := d.sql.Query(`
SELECT c.user_sub AS user_sub,
       c.user_name AS user_name,
       COUNT(*) AS n,
       SUM(CASE WHEN t.kind='giessen'   THEN 1 ELSE 0 END) AS n_giessen,
       SUM(CASE WHEN t.kind='jaeten'    THEN 1 ELSE 0 END) AS n_jaeten,
       SUM(CASE WHEN t.kind='sonstiges' THEN 1 ELSE 0 END) AS n_sonstiges,
       COALESCE(SUM(c.liters), 0) AS liters,
       MAX(c.done_at) AS last_done
  FROM completions c
  JOIN care_tasks t ON t.id = c.task_id
 WHERE c.done_at >= ? AND c.done_at < ?
 GROUP BY c.user_sub, c.user_name
 ORDER BY n DESC, liters DESC, user_name ASC`, sqlTime(from), sqlTime(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []model.LeaderboardEntry{}
	for rows.Next() {
		var e model.LeaderboardEntry
		var giessen, jaeten, sonstiges int
		var last string
		if err := rows.Scan(&e.UserSub, &e.UserName, &e.Completions,
			&giessen, &jaeten, &sonstiges, &e.Liters, &last); err != nil {
			return nil, err
		}
		e.Rank = len(out) + 1
		e.ByKind = map[string]int{"giessen": giessen, "jaeten": jaeten, "sonstiges": sonstiges}
		if ts, err := time.Parse(timeFormat, last); err == nil {
			e.LastCompletion = &ts
		}
		e.Badges = []model.Badge{}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LeaderboardTotals liefert die Gesamtsummen des Dorfes im Zeitraum.
func (d *DB) LeaderboardTotals(from, to time.Time) (model.LeaderboardTotals, error) {
	var t model.LeaderboardTotals
	var giessen, jaeten, sonstiges int
	err := d.sql.QueryRow(`
SELECT COUNT(*),
       COALESCE(SUM(CASE WHEN t.kind='giessen'   THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN t.kind='jaeten'    THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(CASE WHEN t.kind='sonstiges' THEN 1 ELSE 0 END), 0),
       COALESCE(SUM(c.liters), 0),
       COUNT(DISTINCT c.user_sub || char(10) || c.user_name)
  FROM completions c
  JOIN care_tasks t ON t.id = c.task_id
 WHERE c.done_at >= ? AND c.done_at < ?`, sqlTime(from), sqlTime(to)).
		Scan(&t.Completions, &giessen, &jaeten, &sonstiges, &t.Liters, &t.Participants)
	if err != nil {
		return t, err
	}
	t.ByKind = map[string]int{"giessen": giessen, "jaeten": jaeten, "sonstiges": sonstiges}
	return t, nil
}

// Badges leitet die Auszeichnungen je Nutzer ab (Schlüssel: user_sub).
// Die Regeln stehen bei den Konstanten in model/stats.go; hier steht ihre
// SQL-Umsetzung. Alles wird gruppiert ausgewertet.
func (d *DB) Badges(from, to, now time.Time, loc *time.Location) (map[string][]model.Badge, error) {
	out := map[string][]model.Badge{}
	award := func(key string, persons []string) {
		badge, ok := model.BadgeByKey(key)
		if !ok {
			return
		}
		for _, person := range persons {
			out[person] = append(out[person], badge)
		}
	}

	// „Gießkanne des Monats" gilt immer für den laufenden Kalendermonat —
	// unabhängig davon, welcher Zeitraum gerade angezeigt wird.
	monthFrom, monthTo, err := model.PeriodRange(model.PeriodMonth, now, loc)
	if err != nil {
		return nil, err
	}
	champions, err := d.wateringChampions(monthFrom, monthTo)
	if err != nil {
		return nil, err
	}
	award(model.BadgeWateringCan, champions)

	offset := localOffsetSQL("done_at", loc, from, to, now)
	for key, query := range map[string]func() ([]string, error){
		model.BadgeEarlyBird: func() ([]string, error) { return d.earlyBirds(from, to, offset) },
		model.BadgeRescuer:   func() ([]string, error) { return d.rescuers(from, to) },
		model.BadgeEndurance: func() ([]string, error) { return d.enduring(from, to, offset) },
	} {
		persons, err := query()
		if err != nil {
			return nil, err
		}
		award(key, persons)
	}
	return out, nil
}

// wateringChampions: wer im Zeitraum am häufigsten gegossen hat.
// Bei Gleichstand bekommen ihn alle, die vorn liegen.
func (d *DB) wateringChampions(from, to time.Time) ([]string, error) {
	return d.queryPersons(`
SELECT person FROM (
  SELECT c.user_sub || char(10) || c.user_name AS person, COUNT(*) AS n
    FROM completions c
    JOIN care_tasks t ON t.id = c.task_id
   WHERE t.kind = 'giessen' AND c.done_at >= ? AND c.done_at < ?
   GROUP BY c.user_sub, c.user_name
)
WHERE n = (SELECT MAX(n) FROM (
  SELECT COUNT(*) AS n
    FROM completions c
    JOIN care_tasks t ON t.id = c.task_id
   WHERE t.kind = 'giessen' AND c.done_at >= ? AND c.done_at < ?
   GROUP BY c.user_sub, c.user_name))`,
		sqlTime(from), sqlTime(to), sqlTime(from), sqlTime(to))
}

// earlyBirds: genug Erledigungen vor der Frühaufsteher-Stunde (Ortszeit).
func (d *DB) earlyBirds(from, to time.Time, offset string) ([]string, error) {
	q := fmt.Sprintf(`
SELECT `+personKey+` FROM completions
 WHERE done_at >= ? AND done_at < ?
 GROUP BY user_sub, user_name
HAVING SUM(CASE WHEN CAST(strftime('%%H', done_at, %s) AS INTEGER) < %d THEN 1 ELSE 0 END) >= %d`,
		offset, model.EarlyBirdHour, model.MinEarlyCompletions)
	return d.queryPersons(q, sqlTime(from), sqlTime(to))
}

// rescuers: mindestens eine Erledigung an einer Aufgabe, die zu diesem
// Zeitpunkt bereits rot war. Bezugspunkt ist die vorherige Erledigung
// derselben Aufgabe (LAG über die ganze Historie, nicht nur den Zeitraum);
// gab es keine, zählt das Anlegedatum der Aufgabe. Der Hitzefaktor bleibt
// hier bewusst außen vor — er ist eine tagesaktuelle Einstellung und für
// eine rückblickende Auswertung nicht rekonstruierbar.
func (d *DB) rescuers(from, to time.Time) ([]string, error) {
	return d.queryPersons(`
SELECT `+personKey+` FROM (
  SELECT c.user_sub  AS user_sub,
         c.user_name AS user_name,
         c.done_at   AS done_at,
         LAG(c.done_at) OVER (PARTITION BY c.task_id ORDER BY c.done_at) AS prev_done,
         t.created_at     AS created_at,
         t.red_after_days AS red_after_days
    FROM completions c
    JOIN care_tasks t ON t.id = c.task_id
)
WHERE done_at >= ? AND done_at < ?
  AND julianday(done_at) - julianday(COALESCE(prev_done, created_at)) >= red_after_days
GROUP BY user_sub, user_name`, sqlTime(from), sqlTime(to))
}

// enduring: Erledigungen in genügend aufeinanderfolgenden Wochen.
// Klassisches „gaps and islands": die laufende Nummer der Wochen wird von
// der Wochennummer abgezogen; gleiche Differenz = lückenlose Serie.
func (d *DB) enduring(from, to time.Time, offset string) ([]string, error) {
	q := fmt.Sprintf(`
SELECT person FROM (
  SELECT person, COUNT(*) AS len FROM (
    SELECT person, wk - ROW_NUMBER() OVER (PARTITION BY person ORDER BY wk) AS grp
      FROM (SELECT DISTINCT `+personKey+` AS person, %s AS wk
              FROM completions WHERE done_at >= ? AND done_at < ?)
  ) GROUP BY person, grp
) GROUP BY person HAVING MAX(len) >= %d`,
		weekIndexSQL("done_at", offset), model.MinStreakWeeks)
	return d.queryPersons(q, sqlTime(from), sqlTime(to))
}

// queryPersons liefert die Personen-Schlüssel (siehe personKey) einer Abfrage.
func (d *DB) queryPersons(query string, args ...any) ([]string, error) {
	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var person string
		if err := rows.Scan(&person); err != nil {
			return nil, err
		}
		out = append(out, person)
	}
	return out, rows.Err()
}

// weekIndexSQL liefert einen Ausdruck, der jeden Zeitstempel auf eine
// fortlaufende Wochennummer abbildet (Woche beginnt montags, Ortszeit).
// 2440587.5 ist das julianische Datum des 1.1.1970 (ein Donnerstag) —
// die +3 verschiebt den Wochenbeginn von Donnerstag auf Montag.
func weekIndexSQL(col, offset string) string {
	return fmt.Sprintf("CAST((julianday(%s, %s) - 2440587.5 + 3) / 7 AS INTEGER)", col, offset)
}

// localOffsetSQL baut einen SQL-Ausdruck, der zu jedem Zeitstempel den in
// der Ortszeit gültigen UTC-Offset als SQLite-Modifier liefert
// ("+7200 seconds" in der Sommerzeit, "+3600 seconds" sonst). SQLite kennt
// keine Zeitzonen; die Wechsel kommen deshalb aus der Go-Zeitzonendatenbank.
//
// Sehr weite Bereiche (Zeitraum „gesamt") werden auf die Jahre um now
// begrenzt; Zeitstempel außerhalb bekommen den Offset des Bereichsanfangs.
func localOffsetSQL(col string, loc *time.Location, from, to, now time.Time) string {
	if loc == nil {
		loc = time.UTC
	}
	lo, hi := from, to
	if earliest := now.AddDate(-25, 0, 0); lo.Before(earliest) {
		lo = earliest
	}
	if latest := now.AddDate(2, 0, 0); hi.After(latest) {
		hi = latest
	}
	_, fallback := lo.In(loc).Zone()
	if !lo.Before(hi) {
		return fmt.Sprintf("'%+d seconds'", fallback)
	}

	var cases []string
	for cur := lo; cur.Before(hi) && len(cases) < 120; {
		_, off := cur.In(loc).Zone()
		_, end := cur.In(loc).ZoneBounds()
		if end.IsZero() || end.After(hi) {
			end = hi
		}
		if !end.After(cur) {
			break
		}
		cases = append(cases, fmt.Sprintf("WHEN %s >= '%s' AND %s < '%s' THEN '%+d seconds'",
			col, sqlTime(cur), col, sqlTime(end), off))
		cur = end
	}
	if len(cases) == 0 {
		return fmt.Sprintf("'%+d seconds'", fallback)
	}
	return "CASE " + strings.Join(cases, " ") + fmt.Sprintf(" ELSE '%+d seconds' END", fallback)
}

// --- Rücknahme einer Erledigung ---------------------------------------------

// GetCompletion liefert eine einzelne Erledigungs-Meldung.
func (d *DB) GetCompletion(id int64) (*model.Completion, error) {
	row := d.sql.QueryRow(`SELECT id,task_id,user_sub,user_name,liters,note,done_at,forced
		FROM completions WHERE id=?`, id)
	return scanCompletion(row)
}

// DeleteCompletion nimmt eine Meldung zurück. Liefert sql.ErrNoRows,
// wenn es sie nicht (mehr) gibt.
func (d *DB) DeleteCompletion(id int64) error {
	res, err := d.sql.Exec(`DELETE FROM completions WHERE id=?`, id)
	if err != nil {
		return err
	}
	return requireRow(res)
}
