package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Bereich „Dorfpflege“: Orte (Blumenkästen, Beete …) mit Pflegeaufgaben.
// Alles unter /admin/dorfpflege/, damit weitere Bereiche (z.B. Dorfladen RNah)
// später danebengesetzt werden können.
const pflegeBasis = "/admin/dorfpflege"

// Kartenmitte: Rössing, Unter den Eichen.
var kartenMitte = [2]float64{9.8700, 52.2110}

func (a *App) registerDorfpflege(mux *http.ServeMux) {
	get := func(pfad string, h func(http.ResponseWriter, *http.Request, session)) {
		mux.HandleFunc("GET "+pflegeBasis+pfad, a.requireAdmin(h))
	}
	post := func(pfad string, h func(http.ResponseWriter, *http.Request, session)) {
		mux.HandleFunc("POST "+pflegeBasis+pfad, a.requireAdmin(h))
	}

	get("/{$}", a.pflegeUebersicht)

	get("/orte/neu", a.ortNeuFormular)
	post("/orte/neu", a.ortAnlegen)
	get("/orte/{id}", a.ortDetail)
	post("/orte/{id}", a.ortSpeichern)
	get("/orte/{id}/loeschen", a.ortLoeschenFrage)
	post("/orte/{id}/loeschen", a.ortLoeschen)

	get("/orte/{id}/aufgaben/neu", a.aufgabeNeuFormular)
	post("/orte/{id}/aufgaben/neu", a.aufgabeAnlegen)
	get("/aufgaben/{id}", a.aufgabeFormular)
	post("/aufgaben/{id}", a.aufgabeSpeichern)
	get("/aufgaben/{id}/loeschen", a.aufgabeLoeschenFrage)
	post("/aufgaben/{id}/loeschen", a.aufgabeLoeschen)
	get("/aufgaben/{id}/erledigt", a.erledigtFrage)
	post("/aufgaben/{id}/erledigt", a.erledigtMelden)

	get("/erledigungen/{id}/zuruecknehmen", a.erledigungZuruecknehmenFrage)
	post("/erledigungen/{id}/zuruecknehmen", a.erledigungZuruecknehmen)

	get("/rangliste", a.rangliste)

	get("/einstellungen", a.einstellungenFormular)
	post("/einstellungen", a.einstellungenSpeichern)
}

// --- Übersicht --------------------------------------------------------------

type uebersichtDaten struct {
	Orte        []model.PlaceWithStatus
	Hitzefaktor float64
	KarteJSON   string
}

func (a *App) pflegeUebersicht(w http.ResponseWriter, r *http.Request, _ session) {
	orte, faktor, err := api.AssemblePlaces(a.db, a.now())
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.render(w, r, http.StatusOK, "pflege_uebersicht", view{
		Title: "Dorfpflege", Nav: "dorfpflege",
		Data: uebersichtDaten{Orte: orte, Hitzefaktor: faktor, KarteJSON: karteJSON(orte)},
	})
}

// karteJSON baut die GeoJSON-Punkte für die eingebettete Karte.
func karteJSON(orte []model.PlaceWithStatus) string {
	type feature struct {
		Type       string         `json:"type"`
		Geometry   map[string]any `json:"geometry"`
		Properties map[string]any `json:"properties"`
	}
	fs := make([]feature, 0, len(orte))
	for _, p := range orte {
		fs = append(fs, feature{
			Type:       "Feature",
			Geometry:   map[string]any{"type": "Point", "coordinates": []float64{p.Lon, p.Lat}},
			Properties: map[string]any{"id": p.ID, "name": p.Name, "status": string(p.Status)},
		})
	}
	// json.Marshal escapt <, > und & — der Inhalt kann das Skript-Tag nicht verlassen.
	raw, err := json.Marshal(map[string]any{"type": "FeatureCollection", "features": fs})
	if err != nil {
		return `{"type":"FeatureCollection","features":[]}`
	}
	return string(raw)
}

// --- Orte -------------------------------------------------------------------

type ortFormularDaten struct {
	Neu    bool
	Ort    model.Place
	Fehler string
	// Ziel ist die Formular-Action.
	Ziel      string
	KarteJSON string
}

type ortDetailDaten struct {
	Formular ortFormularDaten
	Ort      model.PlaceWithStatus
	Historie []historieEintrag
}

type historieEintrag struct {
	Aufgabe    string
	Erledigung model.Completion
}

func (a *App) ortNeuFormular(w http.ResponseWriter, r *http.Request, _ session) {
	a.zeigeOrtNeu(w, r, http.StatusOK, model.Place{
		Kind: model.PlaceFlowerbox, Active: true,
		Lon: kartenMitte[0], Lat: kartenMitte[1],
	}, "")
}

func (a *App) zeigeOrtNeu(w http.ResponseWriter, r *http.Request, status int, p model.Place, fehler string) {
	a.render(w, r, status, "pflege_ort_neu", view{
		Title: "Neuer Ort", Nav: "dorfpflege",
		Data: ortFormularDaten{Neu: true, Ort: p, Fehler: fehler, Ziel: pflegeBasis + "/orte/neu"},
	})
}

func (a *App) ortAnlegen(w http.ResponseWriter, r *http.Request, _ session) {
	p, in, err := ortAusFormular(r)
	if err != nil {
		a.zeigeOrtNeu(w, r, http.StatusBadRequest, p, err.Error())
		return
	}
	neu := model.Place{Active: true, CreatedAt: a.now()}
	in.Apply(&neu)
	if err := a.db.InsertPlace(&neu); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Ort "+zitat(neu.Name)+" wurde angelegt.")
	http.Redirect(w, r, fmt.Sprintf("%s/orte/%d", pflegeBasis, neu.ID), http.StatusSeeOther)
}

func (a *App) ortDetail(w http.ResponseWriter, r *http.Request, _ session) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.zeigeOrt(w, r, http.StatusOK, id, nil, "")
}

// zeigeOrt rendert die Detailseite. entwurf überschreibt die Formularwerte,
// wenn eine Eingabe zurückgewiesen wurde.
func (a *App) zeigeOrt(w http.ResponseWriter, r *http.Request, status int, id int64, entwurf *model.Place, fehler string) {
	orte, _, err := api.AssemblePlaces(a.db, a.now())
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	var ort *model.PlaceWithStatus
	for i := range orte {
		if orte[i].ID == id {
			ort = &orte[i]
		}
	}
	if ort == nil {
		http.NotFound(w, r)
		return
	}

	historie := []historieEintrag{}
	for _, t := range ort.Tasks {
		cs, err := a.db.ListCompletions(t.ID, 20)
		if err != nil {
			a.fail(w, r, http.StatusInternalServerError, err)
			return
		}
		for _, c := range cs {
			historie = append(historie, historieEintrag{Aufgabe: aufgabenName(t.CareTask), Erledigung: c})
		}
	}
	sort.Slice(historie, func(i, j int) bool {
		return historie[i].Erledigung.DoneAt.After(historie[j].Erledigung.DoneAt)
	})

	formularOrt := ort.Place
	if entwurf != nil {
		formularOrt = *entwurf
		formularOrt.ID = ort.ID
	}
	a.render(w, r, status, "pflege_ort", view{
		Title: ort.Name, Nav: "dorfpflege",
		Data: ortDetailDaten{
			Formular: ortFormularDaten{
				Ort: formularOrt, Fehler: fehler,
				Ziel:      fmt.Sprintf("%s/orte/%d", pflegeBasis, ort.ID),
				KarteJSON: karteJSON([]model.PlaceWithStatus{*ort}),
			},
			Ort:      *ort,
			Historie: historie,
		},
	})
}

func (a *App) ortSpeichern(w http.ResponseWriter, r *http.Request, _ session) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	vorhanden, err := a.db.GetPlace(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, in, err := ortAusFormular(r)
	if err != nil {
		a.zeigeOrt(w, r, http.StatusBadRequest, id, &p, err.Error())
		return
	}
	in.Apply(vorhanden)
	if err := a.db.UpdatePlace(vorhanden); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Ort gespeichert.")
	http.Redirect(w, r, fmt.Sprintf("%s/orte/%d", pflegeBasis, id), http.StatusSeeOther)
}

// ortAusFormular liest und prüft die Formulareingaben. Der zurückgegebene
// Place dient bei Fehlern als Wiedervorlage im Formular.
func ortAusFormular(r *http.Request) (model.Place, api.PlaceInput, error) {
	if err := r.ParseForm(); err != nil {
		return model.Place{}, api.PlaceInput{}, err
	}
	aktiv := r.FormValue("aktiv") != ""
	in := api.PlaceInput{
		Name:        strings.TrimSpace(r.FormValue("name")),
		Description: strings.TrimSpace(r.FormValue("beschreibung")),
		Kind:        r.FormValue("art"),
		Active:      &aktiv,
	}
	entwurf := model.Place{Name: in.Name, Description: in.Description, Kind: model.PlaceKind(in.Kind), Active: aktiv}

	lat, errLat := formularZahl(r, "lat")
	lon, errLon := formularZahl(r, "lon")
	entwurf.Lat, entwurf.Lon = lat, lon
	if errLat != nil || errLon != nil {
		return entwurf, in, fmt.Errorf("Bitte gültige Koordinaten eingeben (z.B. 52.2110 und 9.8700)")
	}
	in.Lat, in.Lon = lat, lon
	if err := in.Validate(); err != nil {
		entwurf.Kind = model.PlaceKind(in.Kind)
		return entwurf, in, err
	}
	entwurf.Kind = model.PlaceKind(in.Kind)
	return entwurf, in, nil
}

func (a *App) ortLoeschenFrage(w http.ResponseWriter, r *http.Request, _ session) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := a.db.GetPlace(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.render(w, r, http.StatusOK, "bestaetigen", view{
		Title: "Ort löschen", Nav: "dorfpflege",
		Data: bestaetigenDaten{
			Ueberschrift: "Ort löschen",
			Text:         zitat(p.Name) + " wird dauerhaft gelöscht — samt aller Aufgaben und der kompletten Historie der Erledigungen.",
			Aktion:       fmt.Sprintf("%s/orte/%d/loeschen", pflegeBasis, id),
			Knopf:        "Ja, Ort löschen",
			Zurueck:      fmt.Sprintf("%s/orte/%d", pflegeBasis, id),
		},
	})
}

func (a *App) ortLoeschen(w http.ResponseWriter, r *http.Request, _ session) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.db.DeletePlace(id); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Der Ort wurde gelöscht.")
	http.Redirect(w, r, pflegeBasis+"/", http.StatusSeeOther)
}

// --- Aufgaben ---------------------------------------------------------------

type aufgabeFormularDaten struct {
	Neu     bool
	Ort     model.Place
	Aufgabe model.CareTask
	// LiterText hält die Rohangabe, damit „leer“ nicht zu 0 wird.
	LiterText string
	Fehler    string
	Ziel      string
}

func (a *App) aufgabeNeuFormular(w http.ResponseWriter, r *http.Request, _ session) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := a.db.GetPlace(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a.zeigeAufgabeNeu(w, r, http.StatusOK, *p,
		model.CareTask{Kind: model.TaskWatering, IntervalDays: 7, RedAfterDays: 14, Active: true}, "10", "")
}

func (a *App) zeigeAufgabeNeu(w http.ResponseWriter, r *http.Request, status int, p model.Place, t model.CareTask, liter, fehler string) {
	a.render(w, r, status, "pflege_aufgabe", view{
		Title: "Neue Aufgabe", Nav: "dorfpflege",
		Data: aufgabeFormularDaten{
			Neu: true, Ort: p, Aufgabe: t, LiterText: liter, Fehler: fehler,
			Ziel: fmt.Sprintf("%s/orte/%d/aufgaben/neu", pflegeBasis, p.ID),
		},
	})
}

func (a *App) aufgabeAnlegen(w http.ResponseWriter, r *http.Request, _ session) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	p, err := a.db.GetPlace(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	entwurf, liter, in, err := aufgabeAusFormular(r)
	if err != nil {
		a.zeigeAufgabeNeu(w, r, http.StatusBadRequest, *p, entwurf, liter, err.Error())
		return
	}
	t := model.CareTask{PlaceID: p.ID, Active: true, CreatedAt: a.now()}
	in.Apply(&t)
	if err := a.db.InsertTask(&t); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Aufgabe "+zitat(aufgabenName(t))+" wurde angelegt.")
	http.Redirect(w, r, fmt.Sprintf("%s/orte/%d", pflegeBasis, p.ID), http.StatusSeeOther)
}

func (a *App) aufgabeFormular(w http.ResponseWriter, r *http.Request, _ session) {
	t, p, ok := a.aufgabeUndOrt(w, r)
	if !ok {
		return
	}
	liter := ""
	if t.Liters != nil {
		liter = zahl(*t.Liters)
	}
	a.zeigeAufgabe(w, r, http.StatusOK, *p, *t, liter, "")
}

func (a *App) zeigeAufgabe(w http.ResponseWriter, r *http.Request, status int, p model.Place, t model.CareTask, liter, fehler string) {
	a.render(w, r, status, "pflege_aufgabe", view{
		Title: "Aufgabe bearbeiten", Nav: "dorfpflege",
		Data: aufgabeFormularDaten{
			Ort: p, Aufgabe: t, LiterText: liter, Fehler: fehler,
			Ziel: fmt.Sprintf("%s/aufgaben/%d", pflegeBasis, t.ID),
		},
	})
}

func (a *App) aufgabeSpeichern(w http.ResponseWriter, r *http.Request, _ session) {
	t, p, ok := a.aufgabeUndOrt(w, r)
	if !ok {
		return
	}
	entwurf, liter, in, err := aufgabeAusFormular(r)
	if err != nil {
		entwurf.ID = t.ID
		a.zeigeAufgabe(w, r, http.StatusBadRequest, *p, entwurf, liter, err.Error())
		return
	}
	in.Apply(t)
	if err := a.db.UpdateTask(t); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Aufgabe gespeichert.")
	http.Redirect(w, r, fmt.Sprintf("%s/orte/%d", pflegeBasis, p.ID), http.StatusSeeOther)
}

func (a *App) aufgabeLoeschenFrage(w http.ResponseWriter, r *http.Request, _ session) {
	t, p, ok := a.aufgabeUndOrt(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "bestaetigen", view{
		Title: "Aufgabe löschen", Nav: "dorfpflege",
		Data: bestaetigenDaten{
			Ueberschrift: "Aufgabe löschen",
			Text:         "Die Aufgabe " + zitat(aufgabenName(*t)) + " an " + zitat(p.Name) + " wird samt Historie gelöscht.",
			Aktion:       fmt.Sprintf("%s/aufgaben/%d/loeschen", pflegeBasis, t.ID),
			Knopf:        "Ja, Aufgabe löschen",
			Zurueck:      fmt.Sprintf("%s/orte/%d", pflegeBasis, p.ID),
		},
	})
}

func (a *App) aufgabeLoeschen(w http.ResponseWriter, r *http.Request, _ session) {
	t, p, ok := a.aufgabeUndOrt(w, r)
	if !ok {
		return
	}
	if err := a.db.DeleteTask(t.ID); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Die Aufgabe wurde gelöscht.")
	http.Redirect(w, r, fmt.Sprintf("%s/orte/%d", pflegeBasis, p.ID), http.StatusSeeOther)
}

// erledigtFrage zeigt die Bestätigungsseite vor dem Melden. Ein Klick allein
// meldet nichts — sonst steht schnell eine Erledigung in der Historie, die es
// nie gegeben hat.
func (a *App) erledigtFrage(w http.ResponseWriter, r *http.Request, _ session) {
	t, p, ok := a.aufgabeUndOrt(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "pflege_erledigt", view{
		Title: "Erledigt melden", Nav: "dorfpflege",
		Data: erledigtDaten{
			Ort: *p, Aufgabe: *t, LiterText: zahlOderLeer(t.Liters),
			Ziel:    fmt.Sprintf("%s/aufgaben/%d/erledigt", pflegeBasis, t.ID),
			Zurueck: fmt.Sprintf("%s/orte/%d", pflegeBasis, p.ID),
		},
	})
}

type erledigtDaten struct {
	Ort     model.Place
	Aufgabe model.CareTask
	// LiterText ist die vorgeschlagene Menge aus dem Gießplan.
	LiterText string
	Ziel      string
	Zurueck   string
}

func (a *App) erledigtMelden(w http.ResponseWriter, r *http.Request, s session) {
	t, p, ok := a.aufgabeUndOrt(w, r)
	if !ok {
		return
	}
	c := model.Completion{
		TaskID: t.ID, UserSub: s.Sub, UserName: anzeigeName(s),
		Note: strings.TrimSpace(r.FormValue("notiz")), DoneAt: a.now(),
	}
	if v, err := formularZahl(r, "liter"); err == nil && v > 0 {
		c.Liters = &v
	} else if t.Liters != nil {
		c.Liters = t.Liters
	}
	if err := a.db.InsertCompletion(&c); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Erledigung für "+zitat(aufgabenName(*t))+" wurde eingetragen.")
	http.Redirect(w, r, fmt.Sprintf("%s/orte/%d", pflegeBasis, p.ID), http.StatusSeeOther)
}

// --- Rücknahme einer Erledigung ---------------------------------------------

// erledigungZuruecknehmenFrage fragt vor der Rücknahme nach — wieder auf einer
// eigenen Seite statt per confirm().
func (a *App) erledigungZuruecknehmenFrage(w http.ResponseWriter, r *http.Request, _ session) {
	c, t, p, ok := a.erledigungMitAufgabe(w, r)
	if !ok {
		return
	}
	menge := ""
	if c.Liters != nil {
		menge = " (" + zahl(*c.Liters) + " l)"
	}
	a.render(w, r, http.StatusOK, "bestaetigen", view{
		Title: "Erledigung zurücknehmen", Nav: "dorfpflege",
		Data: bestaetigenDaten{
			Ueberschrift: "Erledigung zurücknehmen",
			Text: "Die Meldung von " + c.UserName + " vom " + c.DoneAt.Local().Format("02.01.2006, 15:04") +
				" für " + zitat(aufgabenName(*t)) + " an " + zitat(p.Name) + menge + " wird gelöscht. " +
				"Der Ampel-Status rechnet sich danach neu.",
			Aktion:  fmt.Sprintf("%s/erledigungen/%d/zuruecknehmen", pflegeBasis, c.ID),
			Knopf:   "Ja, Meldung zurücknehmen",
			Zurueck: fmt.Sprintf("%s/orte/%d", pflegeBasis, p.ID),
		},
	})
}

func (a *App) erledigungZuruecknehmen(w http.ResponseWriter, r *http.Request, _ session) {
	c, _, p, ok := a.erledigungMitAufgabe(w, r)
	if !ok {
		return
	}
	if err := a.db.DeleteCompletion(c.ID); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Die Meldung wurde zurückgenommen.")
	http.Redirect(w, r, fmt.Sprintf("%s/orte/%d", pflegeBasis, p.ID), http.StatusSeeOther)
}

// erledigungMitAufgabe lädt Erledigung, Aufgabe und Ort; antwortet mit 404.
func (a *App) erledigungMitAufgabe(w http.ResponseWriter, r *http.Request) (*model.Completion, *model.CareTask, *model.Place, bool) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, nil, false
	}
	c, err := a.db.GetCompletion(id)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, nil, false
	}
	t, err := a.db.GetTask(c.TaskID)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, nil, false
	}
	p, err := a.db.GetPlace(t.PlaceID)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, nil, false
	}
	return c, t, p, true
}

// --- Rangliste --------------------------------------------------------------

type zeitraumWahl struct {
	Wert  model.Period
	Name  string
	URL   string
	Aktiv bool
}

type ranglisteDaten struct {
	Zeitraum   model.Period
	Zeitraeume []zeitraumWahl
	// Podest: die ersten drei Plätze, Rest: alle weiteren.
	Podest  []model.LeaderboardEntry
	Alle    []model.LeaderboardEntry
	Summen  model.LeaderboardTotals
	Ich     *model.LeaderboardEntry
	Von     time.Time
	Bis     time.Time
	Einzeln bool
}

// zeitraeume sind die auswählbaren Zeiträume in der Reihenfolge der Anzeige.
var zeitraeume = []struct {
	Wert model.Period
	Name string
}{
	{model.PeriodWeek, "Diese Woche"},
	{model.PeriodMonth, "Dieser Monat"},
	{model.PeriodSeason, "Saison"},
	{model.PeriodYear, "Dieses Jahr"},
	{model.PeriodAll, "Gesamt"},
}

func (a *App) rangliste(w http.ResponseWriter, r *http.Request, s session) {
	zeitraum, err := model.ParsePeriod(r.URL.Query().Get("zeitraum"))
	if err != nil {
		a.fail(w, r, http.StatusBadRequest, err)
		return
	}
	// Der eigene Rang wird für die angemeldete Person mitgeliefert.
	liste, err := api.AssembleLeaderboard(a.db, a.now(), zeitraum, 0,
		auth.User{Sub: s.Sub, Name: anzeigeName(s)})
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}

	wahl := make([]zeitraumWahl, 0, len(zeitraeume))
	for _, z := range zeitraeume {
		wahl = append(wahl, zeitraumWahl{
			Wert: z.Wert, Name: z.Name,
			URL:   pflegeBasis + "/rangliste?zeitraum=" + string(z.Wert),
			Aktiv: z.Wert == zeitraum,
		})
	}
	podest := liste.Entries
	if len(podest) > 3 {
		podest = podest[:3]
	}
	// Bis ist die exklusive Obergrenze — angezeigt wird der letzte Tag davor.
	a.render(w, r, http.StatusOK, "pflege_rangliste", view{
		Title: "Rangliste", Nav: "dorfpflege",
		Data: ranglisteDaten{
			Zeitraum: zeitraum, Zeitraeume: wahl,
			Podest: podest, Alle: liste.Entries, Summen: liste.Totals, Ich: liste.Me,
			Von: liste.From, Bis: liste.To.Add(-time.Second),
			Einzeln: zeitraum != model.PeriodAll,
		},
	})
}

// aufgabeUndOrt lädt Aufgabe samt zugehörigem Ort; antwortet selbst mit 404.
func (a *App) aufgabeUndOrt(w http.ResponseWriter, r *http.Request) (*model.CareTask, *model.Place, bool) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, false
	}
	t, err := a.db.GetTask(id)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, false
	}
	p, err := a.db.GetPlace(t.PlaceID)
	if err != nil {
		http.NotFound(w, r)
		return nil, nil, false
	}
	return t, p, true
}

// aufgabeAusFormular liest und prüft die Formulareingaben einer Aufgabe.
func aufgabeAusFormular(r *http.Request) (model.CareTask, string, api.TaskInput, error) {
	if err := r.ParseForm(); err != nil {
		return model.CareTask{}, "", api.TaskInput{}, err
	}
	aktiv := r.FormValue("aktiv") != ""
	literText := strings.TrimSpace(r.FormValue("liter"))
	in := api.TaskInput{
		Kind:   r.FormValue("art"),
		Title:  strings.TrimSpace(r.FormValue("titel")),
		Active: &aktiv,
	}
	entwurf := model.CareTask{Kind: model.TaskKind(in.Kind), Title: in.Title, Active: aktiv}

	if literText != "" {
		v, err := formularZahl(r, "liter")
		if err != nil {
			return entwurf, literText, in, fmt.Errorf("Liter muss eine Zahl sein")
		}
		in.Liters = &v
		entwurf.Liters = &v
	}
	intervall, errI := formularZahl(r, "intervall")
	rot, errR := formularZahl(r, "rot")
	entwurf.IntervalDays, entwurf.RedAfterDays = intervall, rot
	if errI != nil || errR != nil {
		return entwurf, literText, in, fmt.Errorf("Intervall und Rot-Schwelle müssen Zahlen sein")
	}
	in.IntervalDays, in.RedAfterDays = intervall, rot
	if err := in.Validate(); err != nil {
		return entwurf, literText, in, err
	}
	return entwurf, literText, in, nil
}

// --- Einstellungen ----------------------------------------------------------

type einstellungenDaten struct {
	Hitzefaktor string
	Fehler      string
}

func (a *App) einstellungenFormular(w http.ResponseWriter, r *http.Request, _ session) {
	f, err := a.db.WateringFactor()
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.render(w, r, http.StatusOK, "pflege_einstellungen", view{
		Title: "Einstellungen", Nav: "dorfpflege",
		Data: einstellungenDaten{Hitzefaktor: zahl(f)},
	})
}

func (a *App) einstellungenSpeichern(w http.ResponseWriter, r *http.Request, _ session) {
	roh := strings.TrimSpace(r.FormValue("hitzefaktor"))
	f, err := formularZahl(r, "hitzefaktor")
	if err != nil || f <= 0 || f > 4 {
		a.render(w, r, http.StatusBadRequest, "pflege_einstellungen", view{
			Title: "Einstellungen", Nav: "dorfpflege",
			Data: einstellungenDaten{Hitzefaktor: roh, Fehler: "Der Hitzefaktor muss eine Zahl größer 0 und höchstens 4 sein."},
		})
		return
	}
	if err := a.db.SetWateringFactor(f); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Hitzefaktor auf "+zahl(f)+" gesetzt.")
	http.Redirect(w, r, pflegeBasis+"/einstellungen", http.StatusSeeOther)
}

// --- Kleinkram --------------------------------------------------------------

type bestaetigenDaten struct {
	Ueberschrift string
	Text         string
	Aktion       string
	Knopf        string
	Zurueck      string
}

func pfadID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// formularZahl liest ein Zahlenfeld und akzeptiert auch das deutsche Komma.
func formularZahl(r *http.Request, name string) (float64, error) {
	s := strings.ReplaceAll(strings.TrimSpace(r.FormValue(name)), ",", ".")
	if s == "" {
		return 0, fmt.Errorf("%s fehlt", name)
	}
	return strconv.ParseFloat(s, 64)
}

func aufgabenName(t model.CareTask) string {
	if t.Title != "" {
		return t.Title
	}
	return aufgabenart(t.Kind)
}

// zahlOderLeer formatiert eine optionale Zahl (leer statt 0).
func zahlOderLeer(v *float64) string {
	if v == nil {
		return ""
	}
	return zahl(*v)
}

// zitat setzt einen Namen in deutsche Anführungszeichen.
func zitat(s string) string { return "\u201e" + s + "\u201c" }
