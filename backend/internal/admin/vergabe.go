package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/vergabe"
)

// Vergabe in der Verwaltung: Auf der Ortsseite steht je Aufgabe, wer
// angemeldet ist, wer wann gefragt wurde und wer zugesagt hat. Verwaltende
// können eine Zusage aufheben — auf einer eigenen Bestätigungsseite, wie
// überall in dieser Verwaltung.

// vergabeEngine baut die Vergabe mit der Zeitquelle der Verwaltung.
func (a *App) vergabeEngine() *vergabe.Engine {
	return vergabe.New(a.db, vergabe.Config{Now: a.now})
}

// vergabeStaende sammelt den Vergabestand aller Aufgaben eines Ortes.
func (a *App) vergabeStaende(ort model.PlaceWithStatus) (map[int64]*vergabe.Stand, error) {
	e := a.vergabeEngine()
	out := map[int64]*vergabe.Stand{}
	for _, t := range ort.Tasks {
		stand, err := e.Stand(t.ID)
		if err != nil {
			return nil, err
		}
		out[t.ID] = stand
	}
	return out, nil
}

// --- Zusage aufheben --------------------------------------------------------

func (a *App) zusageAufhebenFrage(w http.ResponseWriter, r *http.Request, _ session) {
	vorgang, task, ort, ok := a.vorgangLaden(w, r)
	if !ok {
		return
	}
	if vorgang.ClaimedBy == "" {
		a.setFlash(w, "error", "Für diese Aufgabe liegt gerade keine Zusage vor.")
		http.Redirect(w, r, fmt.Sprintf("%s/orte/%d", mithelfenBasis, ort.ID), http.StatusSeeOther)
		return
	}
	bis := ""
	if vorgang.ClaimedUntil != nil {
		bis = " (Frist bis " + ortszeit(*vorgang.ClaimedUntil).Format("02.01.2006, 15:04") + ")"
	}
	a.render(w, r, http.StatusOK, "bestaetigen", view{
		Title: "Zusage aufheben", Nav: "mithelfen",
		Data: bestaetigenDaten{
			Ueberschrift: "Zusage aufheben",
			Text: "Die Zusage von " + vorgang.ClaimedByName + " für " + zitat(aufgabenName(*task)) +
				" an " + zitat(ort.Name) + bis + " wird aufgehoben. " +
				vorgang.ClaimedByName + " bekommt einen Hinweis, und die Warteschlange fragt weiter.",
			Aktion:  fmt.Sprintf("%s/vorgaenge/%d/zusage-aufheben", mithelfenBasis, vorgang.ID),
			Knopf:   "Ja, Zusage aufheben",
			Zurueck: fmt.Sprintf("%s/orte/%d", mithelfenBasis, ort.ID),
		},
	})
}

func (a *App) zusageAufheben(w http.ResponseWriter, r *http.Request, s session) {
	vorgang, _, ort, ok := a.vorgangLaden(w, r)
	if !ok {
		return
	}
	if _, err := a.vergabeEngine().Zurueckgeben(vorgang.ID, s.Sub, true); err != nil {
		var abgewiesen *vergabe.Abweisung
		if errors.As(err, &abgewiesen) {
			a.setFlash(w, "error", abgewiesen.Message)
			http.Redirect(w, r, fmt.Sprintf("%s/orte/%d", mithelfenBasis, ort.ID), http.StatusSeeOther)
			return
		}
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Die Zusage wurde aufgehoben.")
	http.Redirect(w, r, fmt.Sprintf("%s/orte/%d", mithelfenBasis, ort.ID), http.StatusSeeOther)
}

// vorgangLaden holt Vorgang, Aufgabe und Ort; antwortet selbst mit 404.
func (a *App) vorgangLaden(w http.ResponseWriter, r *http.Request) (*model.Assignment, *model.CareTask, *model.Place, bool) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, nil, false
	}
	vorgang, err := a.db.GetAssignment(id)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, nil, false
	}
	task, err := a.db.GetTask(vorgang.TaskID)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, nil, false
	}
	ort, err := a.db.GetPlace(task.PlaceID)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, nil, false
	}
	// Der Name der zusagenden Person kommt aus den Profilen.
	if stand, err := a.vergabeEngine().Stand(task.ID); err == nil && stand.Vorgang != nil &&
		stand.Vorgang.ID == vorgang.ID {
		vorgang = stand.Vorgang
	}
	return vorgang, task, ort, true
}

// --- Anzeige-Helfer für die Templates ---------------------------------------

// zustellungsArt beschreibt, was jemand bekommen hat.
func zustellungsArt(k model.NotificationKind) string {
	switch k {
	case model.NotifyRequest:
		return "gefragt"
	case model.NotifyBroadcast:
		return "im Rundruf gefragt"
	case model.NotifyClaimExpired:
		return "Zusage verfallen"
	case model.NotifyClaimRevoked:
		return "Zusage aufgehoben"
	case model.NotifyAssignmentDone:
		return "Hinweis: schon erledigt"
	}
	return string(k)
}

// vergabeStand beschreibt den Stand eines Vorgangs in einem Wort.
func vergabeStandText(s model.AssignmentState) string {
	switch s {
	case model.AssignmentOpen:
		return "Anfragen laufen"
	case model.AssignmentClaimed:
		return "übernommen"
	case model.AssignmentBroadcast:
		return "Rundruf an alle"
	case model.AssignmentEnded:
		return "beendet"
	}
	return string(s)
}

// --- Einstellungen der Vergabe ----------------------------------------------

// vergabeFormular hält die Rohwerte des Einstellungsformulars, damit eine
// zurückgewiesene Eingabe stehen bleibt.
type vergabeFormular struct {
	Abstand     string
	Zusagefrist string
	RuheVon     string
	RuheBis     string
}

func vergabeFormularVon(r model.AssignmentRules) vergabeFormular {
	return vergabeFormular{
		Abstand:     strconv.Itoa(int(r.OfferInterval / time.Minute)),
		Zusagefrist: strconv.Itoa(int(r.ClaimDuration / time.Hour)),
		RuheVon:     strconv.Itoa(r.QuietFrom),
		RuheBis:     strconv.Itoa(r.QuietTo),
	}
}

// regelnAusFormular liest die Vergabe-Einstellungen aus dem Formular.
// angegeben=false heißt: Das Formular enthielt die Felder gar nicht — dann
// bleiben die gespeicherten Regeln unangetastet (ältere Formulare, Skripte).
func regelnAusFormular(r *http.Request) (regeln model.AssignmentRules, roh vergabeFormular, angegeben bool, err error) {
	roh = vergabeFormular{
		Abstand:     strings.TrimSpace(r.FormValue("abstand")),
		Zusagefrist: strings.TrimSpace(r.FormValue("zusagefrist")),
		RuheVon:     strings.TrimSpace(r.FormValue("ruhe-von")),
		RuheBis:     strings.TrimSpace(r.FormValue("ruhe-bis")),
	}
	if roh.Abstand == "" && roh.Zusagefrist == "" && roh.RuheVon == "" && roh.RuheBis == "" {
		return model.AssignmentRules{}, roh, false, nil
	}
	ganz := func(wert, name string) (int, error) {
		n, err := strconv.Atoi(wert)
		if err != nil {
			return 0, fmt.Errorf("%s muss eine ganze Zahl sein", name)
		}
		return n, nil
	}
	abstand, err := ganz(roh.Abstand, "Der Abstand zwischen zwei Anfragen")
	if err != nil {
		return model.AssignmentRules{}, roh, true, err
	}
	frist, err := ganz(roh.Zusagefrist, "Die Zusagefrist")
	if err != nil {
		return model.AssignmentRules{}, roh, true, err
	}
	von, err := ganz(roh.RuheVon, "Der Beginn der Ruhezeit")
	if err != nil {
		return model.AssignmentRules{}, roh, true, err
	}
	bis, err := ganz(roh.RuheBis, "Das Ende der Ruhezeit")
	if err != nil {
		return model.AssignmentRules{}, roh, true, err
	}
	regeln = model.AssignmentRules{
		OfferInterval: time.Duration(abstand) * time.Minute,
		ClaimDuration: time.Duration(frist) * time.Hour,
		QuietFrom:     von, QuietTo: bis,
	}
	if err := regeln.Validate(); err != nil {
		return regeln, roh, true, err
	}
	return regeln, roh, true, nil
}
