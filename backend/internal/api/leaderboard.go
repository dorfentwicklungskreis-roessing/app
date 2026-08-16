package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Rangliste: sichtbar machen, wer wie viel fürs Dorf tut.

const (
	// DefaultLeaderboardLimit: so viele Plätze werden ohne Angabe geliefert.
	DefaultLeaderboardLimit = 25
	// MaxLeaderboardLimit: mehr gibt es auch auf Wunsch nicht.
	MaxLeaderboardLimit = 200
)

// AssembleLeaderboard baut die Rangliste für einen Zeitraum.
// Wird von REST-API und MCP-Server gemeinsam genutzt.
//
// me ist der Aufrufer: sein eigener Eintrag wird immer mitgeliefert — auch
// wenn er außerhalb der ausgegebenen Plätze liegt oder noch gar nichts
// gemeldet hat (dann mit Rang 0). Bei leerem Sub entfällt er.
func AssembleLeaderboard(d *db.DB, now time.Time, period model.Period, limit int, me auth.User) (model.Leaderboard, error) {
	// Ohne Zugriffssicht zählt alles — für die Verwaltung und den
	// MCP-Endpunkt, die beide bereits die globale admin-Rolle verlangen.
	return AssembleLeaderboardFuer(d, now, period, limit, me, db.SichtAlles())
}

// AssembleLeaderboardFuer baut die Rangliste in der Sicht dieser Person.
//
// Die Sicht filtert bereits in SQL: Erledigungen an internen Aufgaben
// fließen für Außenstehende weder in die Zeilen noch in die Gesamtsummen und
// auch nicht in die Auszeichnungen ein. Eine Rangliste, in der die Summe
// nicht zu den sichtbaren Zeilen passt, verriete sonst genau das, was sie
// verbergen soll.
func AssembleLeaderboardFuer(d *db.DB, now time.Time, period model.Period, limit int,
	me auth.User, sicht db.Sicht,
) (model.Leaderboard, error) {
	loc := model.Location()
	from, to, err := model.PeriodRange(period, now, loc)
	if err != nil {
		return model.Leaderboard{}, err
	}
	// Der Hitzefaktor gehört zur Wertung: er bestimmt die Sperrfrist, und
	// nur außerhalb der Sperrfrist gemeldete Erledigungen zählen (stats.go).
	factor, err := d.WateringFactor()
	if err != nil {
		return model.Leaderboard{}, err
	}
	entries, err := d.Leaderboard(from, to, factor, sicht)
	if err != nil {
		return model.Leaderboard{}, err
	}
	totals, err := d.LeaderboardTotals(from, to, factor, sicht)
	if err != nil {
		return model.Leaderboard{}, err
	}
	awarded, err := d.Badges(from, to, now, loc, factor, sicht)
	if err != nil {
		return model.Leaderboard{}, err
	}
	for i := range entries {
		if b, ok := awarded[db.PersonKey(entries[i].UserSub, entries[i].UserName)]; ok {
			entries[i].Badges = b
		}
	}

	// Den eigenen Eintrag vor dem Kürzen heraussuchen: das ist die Zeile mit
	// gleicher Kennung UND gleichem Namen;
	// notfalls (z.B. nach einer Namensänderung) tut es die beste Zeile mit
	// der eigenen Kennung.
	var mine *model.LeaderboardEntry
	if me.Sub != "" {
		for i := range entries {
			if entries[i].UserSub != me.Sub {
				continue
			}
			e := entries[i]
			if mine == nil {
				mine = &e
			}
			if e.UserName == me.Name {
				mine = &e
				break
			}
		}
		if mine == nil {
			mine = &model.LeaderboardEntry{UserSub: me.Sub, UserName: me.Name,
				ByKind: model.EmptyByKind(), Badges: []model.Badge{}}
		}
	}

	if limit <= 0 {
		limit = DefaultLeaderboardLimit
	}
	if limit > MaxLeaderboardLimit {
		limit = MaxLeaderboardLimit
	}
	if len(entries) > limit {
		entries = entries[:limit]
	}

	// Erst ganz zum Schluss die Namen aus den Profilen einsetzen: Gruppierung
	// (SQL), Auszeichnungen und die Suche nach dem eigenen Eintrag arbeiten
	// alle mit dem in der Meldung gespeicherten Namen. Erst für die Anzeige
	// gilt der Nickname bzw. Anzeigename aus dem Profil.
	namen, err := d.NameResolver()
	if err != nil {
		return model.Leaderboard{}, err
	}
	for i := range entries {
		entries[i].UserName = namen.Resolve(entries[i].UserSub, entries[i].UserName)
	}
	if mine != nil {
		mine.UserName = namen.Resolve(mine.UserSub, mine.UserName)
	}

	return model.Leaderboard{
		Period: period, From: from.In(loc), To: to.In(loc),
		Entries: entries, Totals: totals, Me: mine,
	}, nil
}

func (s *Server) handleLeaderboard(w http.ResponseWriter, r *http.Request) {
	period, err := model.ParsePeriod(r.URL.Query().Get("period"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	u, _ := auth.FromContext(r.Context())
	sicht, err := SichtVon(s.DB, s.zugriff(r))
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	lb, err := AssembleLeaderboardFuer(s.DB, s.now(), period, limit, u, sicht)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, lb)
}
