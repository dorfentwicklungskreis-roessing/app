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
	entries, err := d.Leaderboard(from, to, factor)
	if err != nil {
		return model.Leaderboard{}, err
	}
	totals, err := d.LeaderboardTotals(from, to, factor)
	if err != nil {
		return model.Leaderboard{}, err
	}
	awarded, err := d.Badges(from, to, now, loc, factor)
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
	lb, err := AssembleLeaderboard(s.DB, s.now(), period, limit, u)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, lb)
}
