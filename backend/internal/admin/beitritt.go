package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Beitritte in der Web-Verwaltung: Wer mitmachen will, steht auf der Seite
// seines Trägers — und wird dort aufgenommen oder abgelehnt.
//
// Der Knopf tut mehr, als es aussieht: Die Freigabe schreibt die
// Rollenzuweisung in die Rössing-ID zurück (siehe api.Aufnehmen). Erst wenn
// das geklappt hat, gilt der Antrag als erteilt. Scheitert es, bleibt er
// offen und die Meldung sagt, woran es lag — nichts wäre schlimmer als eine
// Verwaltung, die „aufgenommen“ meldet, während die Tür zu bleibt.

func (a *App) beitrittEntscheiden(w http.ResponseWriter, r *http.Request, s session) {
	t, z, ok := a.traegerAusPfad(w, r, s)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("bid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	b, err := a.db.GetBeitritt(id)
	// Der Antrag muss zu genau diesem Träger gehören — sonst ließe sich über
	// eine fremde Kennung an den Mitgliedern eines anderen Vereins drehen.
	if err != nil || b.TraegerID != t.ID {
		http.NotFound(w, r)
		return
	}
	if err := r.ParseForm(); err != nil {
		a.fail(w, r, http.StatusBadRequest, err)
		return
	}
	status := model.AntragStatus(r.FormValue("status"))
	if status != model.AntragErteilt && status != model.AntragAbgelehnt {
		a.setFlash(w, "error", "Unbekannte Entscheidung.")
		a.zurueckZumTraeger(w, r, t.ID)
		return
	}
	notiz := strings.TrimSpace(r.FormValue("notiz"))
	if _, err := api.BeitrittEntscheiden(r.Context(), a.db, a.mitglieder, t, *b,
		status, z.Sub, notiz, a.now()); err != nil {
		a.setFlash(w, "error", fehlertext(err))
		a.zurueckZumTraeger(w, r, t.ID)
		return
	}
	name := b.UserName
	if name == "" {
		name = b.UserSub
	}
	if status == model.AntragErteilt {
		a.setFlash(w, "success", zitat(name)+" ist jetzt Mitglied von "+zitat(t.Name)+
			" — die Rolle steht in der Rössing-ID.")
	} else {
		a.setFlash(w, "success", "Der Beitrittsantrag von "+zitat(name)+" wurde abgelehnt.")
	}
	a.zurueckZumTraeger(w, r, t.ID)
}

// mitgliedAufnehmen trägt jemanden ohne vorherigen Antrag ein. Für die
// geschlossene Gruppe, die keine Anträge entgegennimmt, ist das der einzige
// Weg — und für die offene der kurze, wenn am Gartenzaun gefragt wurde.
func (a *App) mitgliedAufnehmen(w http.ResponseWriter, r *http.Request, s session) {
	t, z, ok := a.traegerAusPfad(w, r, s)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.fail(w, r, http.StatusBadRequest, err)
		return
	}
	userSub := strings.TrimSpace(r.FormValue("userSub"))
	notiz := strings.TrimSpace(r.FormValue("notiz"))
	b, err := api.Aufnehmen(r.Context(), a.db, a.mitglieder, t, userSub, z.Sub, notiz, a.now())
	if err != nil {
		a.setFlash(w, "error", fehlertext(err))
		a.zurueckZumTraeger(w, r, t.ID)
		return
	}
	namen, nerr := a.db.NameResolver()
	name := userSub
	if nerr == nil {
		if n := namen.Resolve(b.UserSub, "", model.SichtVerwaltung); n != "" {
			name = n
		}
	}
	a.setFlash(w, "success", zitat(name)+" ist jetzt Mitglied von "+zitat(t.Name)+".")
	a.zurueckZumTraeger(w, r, t.ID)
}

func (a *App) zurueckZumTraeger(w http.ResponseWriter, r *http.Request, id int64) {
	http.Redirect(w, r, fmt.Sprintf("%s/%d", traegerBasis, id), http.StatusSeeOther)
}

// fehlertext holt die Meldung heraus, die für Menschen gedacht ist. Alles
// andere kommt als allgemeiner Satz — eine rohe Fehlermeldung auf einer
// Verwaltungsseite hilft niemandem weiter.
func fehlertext(err error) string {
	var ce *api.CompletionError
	if errors.As(err, &ce) {
		return ce.Message
	}
	return "Das hat nicht geklappt: " + err.Error()
}
