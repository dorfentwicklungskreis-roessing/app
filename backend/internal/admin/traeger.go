package admin

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/mitglied"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Bereich „Träger“ — die Vereine und Gruppen, die Aufgaben einstellen.
//
// Zwei Rollen treffen sich hier: Der Plattform-Betreiber lässt Träger zu und
// sperrt sie; die Verwaltenden eines Trägers pflegen ihren eigenen Eintrag,
// ihre Befähigungen und die Anträge darauf. Beide sehen dieselben Seiten,
// nur mit unterschiedlich vielen Feldern.
const traegerBasis = "/admin/traeger"

func (a *App) registerTraeger(mux *http.ServeMux) {
	get := func(pfad string, h func(http.ResponseWriter, *http.Request, session)) {
		mux.HandleFunc("GET "+traegerBasis+pfad, a.requireVerwaltung(h))
	}
	post := func(pfad string, h func(http.ResponseWriter, *http.Request, session)) {
		mux.HandleFunc("POST "+traegerBasis+pfad, a.requireVerwaltung(h))
	}

	get("/{$}", a.traegerListe)
	get("/neu", a.traegerNeuFormular)
	post("/neu", a.traegerAnlegen)
	get("/{id}", a.traegerDetail)
	post("/{id}", a.traegerSpeichern)
	post("/{id}/zulassung", a.traegerZulassung)

	post("/{id}/befaehigungen", a.befaehigungAnlegen)
	post("/{id}/befaehigungen/{bid}", a.befaehigungSpeichern)
	get("/{id}/befaehigungen/{bid}/loeschen", a.befaehigungLoeschenFrage)
	post("/{id}/befaehigungen/{bid}/loeschen", a.befaehigungLoeschen)

	post("/{id}/antraege/{aid}", a.antragEntscheiden)

	post("/{id}/beitritte/{bid}", a.beitrittEntscheiden)
	post("/{id}/mitglieder", a.mitgliedAufnehmen)
}

// requireVerwaltung lässt herein, wer irgendetwas zu verwalten hat: den
// Betreiber und die Verwaltenden eines Trägers. Wer nur Mitglied ist, hat
// hier nichts zu suchen — die App ist der Ort zum Mitmachen.
func (a *App) requireVerwaltung(h func(http.ResponseWriter, *http.Request, session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, ok := a.sessionOf(r)
		if !ok {
			a.setFlash(w, "error", "Bitte zuerst mit der Rössing-ID anmelden.")
			http.Redirect(w, r, "/admin/", http.StatusSeeOther)
			return
		}
		z := a.zugriff(r, s)
		if !z.Betreiber && len(a.meineTraeger(z)) == 0 {
			a.setFlash(w, "error", "Du verwaltest keinen Träger.")
			http.Redirect(w, r, "/admin/", http.StatusSeeOther)
			return
		}
		h(w, r, s)
	}
}

// zugriff baut die Berechtigungssicht aus der Session. Die Betreiber-Rolle
// steckt in der Session (sie kam beim Anmelden aus dem Token); die
// Träger-Mitgliedschaften kommen aus der Rössing-ID.
func (a *App) zugriff(r *http.Request, s session) model.Zugriff {
	u := auth.User{Sub: s.Sub, Name: s.Name, Email: s.Email, Roles: map[string]bool{}}
	if s.Admin {
		u.Roles["admin"] = true
	}
	for _, rolle := range s.Rollen {
		u.Roles[rolle] = true
	}
	return mitglied.Zugriff(r.Context(), a.mitglieder, u)
}

// meineTraeger sind die Träger, die diese Person verwalten darf.
func (a *App) meineTraeger(z model.Zugriff) []model.Traeger {
	alle, err := a.db.ListTraeger()
	if err != nil {
		return nil
	}
	out := []model.Traeger{}
	for _, t := range alle {
		if z.DarfVerwalten(t) {
			out = append(out, t)
		}
	}
	return out
}

// --- Liste ------------------------------------------------------------------

type traegerListeDaten struct {
	Traeger []traegerZeile
	// Betreiber blendet die Betreiber-Werkzeuge ein (Anlegen, Zulassen).
	Betreiber bool
	// Veraltet: Die Mitgliedschaften stammen aus dem Zwischenspeicher, weil
	// die Rössing-ID gerade nicht erreichbar ist.
	Veraltet bool
}

type traegerZeile struct {
	model.Traeger
	OffeneAntraege  int
	OffeneBeitritte int
	Orte            int
	DarfVerwalten   bool
}

func (a *App) traegerListe(w http.ResponseWriter, r *http.Request, s session) {
	z := a.zugriff(r, s)
	alle, err := a.db.ListTraeger()
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	orte, err := a.db.ListPlaces()
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	orteJe := map[int64]int{}
	for _, o := range orte {
		orteJe[o.TraegerID]++
	}
	offeneBeitritte, err := a.db.OffeneBeitritte()
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}

	daten := traegerListeDaten{Betreiber: z.Betreiber, Veraltet: z.Veraltet}
	for _, t := range alle {
		// Der Betreiber sieht alles, alle anderen nur, was sie verwalten.
		if !z.Betreiber && !z.DarfVerwalten(t) {
			continue
		}
		zeile := traegerZeile{Traeger: t, Orte: orteJe[t.ID], DarfVerwalten: z.DarfVerwalten(t)}
		if offen, err := a.db.ListAntraege(t.ID, model.AntragBeantragt); err == nil {
			zeile.OffeneAntraege = len(offen)
		}
		zeile.OffeneBeitritte = offeneBeitritte[t.ID]
		daten.Traeger = append(daten.Traeger, zeile)
	}
	a.render(w, r, http.StatusOK, "traeger_liste", view{
		Title: "Träger", Nav: "traeger", Data: daten})
}

// --- Anlegen ----------------------------------------------------------------

type traegerFormularDaten struct {
	Neu     bool
	Traeger model.Traeger
	Fehler  string
	Ziel    string
	// Betreiber blendet Zulassungsstand und Zitadel-Projekt ein.
	Betreiber bool
}

func (a *App) traegerNeuFormular(w http.ResponseWriter, r *http.Request, s session) {
	if !a.zugriff(r, s).Betreiber {
		a.setFlash(w, "error", "Träger legt nur der Betreiber der Dorf-App an.")
		http.Redirect(w, r, traegerBasis+"/", http.StatusSeeOther)
		return
	}
	a.zeigeTraegerNeu(w, r, http.StatusOK, model.Traeger{
		Status: model.TraegerBeantragt, Sichtbarkeit: model.TraegerOffen}, "")
}

func (a *App) zeigeTraegerNeu(w http.ResponseWriter, r *http.Request, status int,
	t model.Traeger, fehler string,
) {
	a.render(w, r, status, "traeger_neu", view{
		Title: "Neuer Träger", Nav: "traeger",
		Data: traegerFormularDaten{Neu: true, Traeger: t, Fehler: fehler,
			Ziel: traegerBasis + "/neu", Betreiber: true},
	})
}

func (a *App) traegerAnlegen(w http.ResponseWriter, r *http.Request, s session) {
	if !a.zugriff(r, s).Betreiber {
		a.setFlash(w, "error", "Träger legt nur der Betreiber der Dorf-App an.")
		http.Redirect(w, r, traegerBasis+"/", http.StatusSeeOther)
		return
	}
	t, err := traegerAusFormular(r, true, model.Traeger{})
	if err != nil {
		a.zeigeTraegerNeu(w, r, http.StatusBadRequest, t, err.Error())
		return
	}
	t.CreatedAt = a.now()
	if err := a.db.InsertTraeger(&t); err != nil {
		a.zeigeTraegerNeu(w, r, http.StatusConflict, t,
			"Anlegen nicht möglich — ist die Zitadel-Projekt-ID schon vergeben?")
		return
	}
	a.setFlash(w, "success", "Träger "+zitat(t.Name)+" wurde angelegt.")
	http.Redirect(w, r, fmt.Sprintf("%s/%d", traegerBasis, t.ID), http.StatusSeeOther)
}

// --- Detail -----------------------------------------------------------------

type traegerDetailDaten struct {
	Formular       traegerFormularDaten
	Traeger        model.Traeger
	Befaehigungen  []model.Befaehigung
	OffeneAntraege []model.BefaehigungsAntrag
	Entschieden    []model.BefaehigungsAntrag
	Orte           []model.Place
	Betreiber      bool
	// Beitritte: wer mitmachen will, und wer schon aufgenommen ist.
	OffeneBeitritte []model.Beitritt
	Mitglieder      []model.Beitritt
	// AufnahmeMoeglich sagt, ob eine Freigabe überhaupt etwas bewirken kann.
	// Ohne schreibenden Dienst-Nutzer bliebe sie wirkungslos, und dann soll
	// hier ein Hinweis stehen statt eines Knopfes, der nichts tut.
	AufnahmeMoeglich bool
	// KeineAufnahme ist der Hinweistext für genau diesen Fall.
	KeineAufnahme string
}

// traegerAusPfad lädt den Träger und prüft, ob er gepflegt werden darf.
func (a *App) traegerAusPfad(w http.ResponseWriter, r *http.Request, s session) (model.Traeger, model.Zugriff, bool) {
	z := a.zugriff(r, s)
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return model.Traeger{}, z, false
	}
	t, err := a.db.GetTraeger(id)
	if err != nil {
		http.NotFound(w, r)
		return model.Traeger{}, z, false
	}
	if !z.DarfVerwalten(*t) {
		// Der Betreiber sieht jeden Träger; für alle anderen gibt es einen
		// fremden Träger hier schlicht nicht.
		if z.Veraltet && !z.Betreiber {
			a.setFlash(w, "error",
				"Die Rössing-ID ist gerade nicht erreichbar — Änderungen sind vorübergehend gesperrt.")
			http.Redirect(w, r, traegerBasis+"/", http.StatusSeeOther)
			return model.Traeger{}, z, false
		}
		http.NotFound(w, r)
		return model.Traeger{}, z, false
	}
	return *t, z, true
}

func (a *App) traegerDetail(w http.ResponseWriter, r *http.Request, s session) {
	t, z, ok := a.traegerAusPfad(w, r, s)
	if !ok {
		return
	}
	a.zeigeTraeger(w, r, http.StatusOK, t, z, nil, "")
}

func (a *App) zeigeTraeger(w http.ResponseWriter, r *http.Request, status int,
	t model.Traeger, z model.Zugriff, entwurf *model.Traeger, fehler string,
) {
	befaehigungen, err := a.db.ListBefaehigungen(t.ID)
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	alleAntraege, err := a.db.ListAntraege(t.ID, "")
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	namen, err := a.db.NameResolver()
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	offen := []model.BefaehigungsAntrag{}
	entschieden := []model.BefaehigungsAntrag{}
	for _, antrag := range alleAntraege {
		antrag.UserName = namen.Resolve(antrag.UserSub, "")
		if antrag.UserName == "" {
			antrag.UserName = antrag.UserSub
		}
		if antrag.Status == model.AntragBeantragt {
			offen = append(offen, antrag)
		} else {
			entschieden = append(entschieden, antrag)
		}
	}

	alleOrte, err := a.db.ListPlaces()
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	seine := []model.Place{}
	for _, o := range alleOrte {
		if o.TraegerID == t.ID {
			seine = append(seine, o)
		}
	}

	alleBeitritte, err := a.db.ListBeitritte(t.ID, "")
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	offeneBeitritte := []model.Beitritt{}
	mitglieder := []model.Beitritt{}
	for _, b := range alleBeitritte {
		b.UserName = namen.Resolve(b.UserSub, "")
		if b.UserName == "" {
			b.UserName = b.UserSub
		}
		if b.Status == model.AntragBeantragt {
			offeneBeitritte = append(offeneBeitritte, b)
		} else {
			mitglieder = append(mitglieder, b)
		}
	}
	_, kannAufnehmen := mitglied.AufnehmerVon(a.mitglieder)

	formular := t
	if entwurf != nil {
		formular = *entwurf
		formular.ID = t.ID
	}
	a.render(w, r, status, "traeger", view{
		Title: t.Name, Nav: "traeger",
		Data: traegerDetailDaten{
			Formular: traegerFormularDaten{Traeger: formular, Fehler: fehler,
				Ziel: fmt.Sprintf("%s/%d", traegerBasis, t.ID), Betreiber: z.Betreiber},
			Traeger:        t,
			Befaehigungen:  befaehigungen,
			OffeneAntraege: offen,
			Entschieden:    entschieden,
			Orte:           seine,
			Betreiber:      z.Betreiber,

			OffeneBeitritte:  offeneBeitritte,
			Mitglieder:       mitglieder,
			AufnahmeMoeglich: kannAufnehmen && t.ProjektID != "",
			KeineAufnahme:    api.NochNichtEingerichtet,
		},
	})
}

func (a *App) traegerSpeichern(w http.ResponseWriter, r *http.Request, s session) {
	t, z, ok := a.traegerAusPfad(w, r, s)
	if !ok {
		return
	}
	neu, err := traegerAusFormular(r, z.Betreiber, t)
	if err != nil {
		a.zeigeTraeger(w, r, http.StatusBadRequest, t, z, &neu, err.Error())
		return
	}
	neu.ID = t.ID
	if err := a.db.UpdateTraeger(&neu); err != nil {
		a.zeigeTraeger(w, r, http.StatusConflict, t, z, &neu,
			"Speichern nicht möglich — ist die Zitadel-Projekt-ID schon vergeben?")
		return
	}
	a.setFlash(w, "success", "Träger gespeichert.")
	http.Redirect(w, r, fmt.Sprintf("%s/%d", traegerBasis, t.ID), http.StatusSeeOther)
}

// traegerZulassung lässt zu oder sperrt. Ausschließlich der Betreiber:
// Sonst könnte sich jede Gruppe selbst freischalten und im Namen des Dorfes
// auftreten.
func (a *App) traegerZulassung(w http.ResponseWriter, r *http.Request, s session) {
	z := a.zugriff(r, s)
	if !z.Betreiber {
		a.setFlash(w, "error", "Träger lässt nur der Betreiber der Dorf-App zu.")
		http.Redirect(w, r, traegerBasis+"/", http.StatusSeeOther)
		return
	}
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	t, err := a.db.GetTraeger(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	status := model.TraegerStatus(r.FormValue("status"))
	if !model.ValidTraegerStatus(status) {
		a.setFlash(w, "error", "Unbekannter Zulassungsstand.")
		http.Redirect(w, r, fmt.Sprintf("%s/%d", traegerBasis, id), http.StatusSeeOther)
		return
	}
	t.Status = status
	if err := a.db.UpdateTraeger(t); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	meldung := map[model.TraegerStatus]string{
		model.TraegerZugelassen: "Träger " + zitat(t.Name) + " ist zugelassen.",
		model.TraegerGesperrt:   "Träger " + zitat(t.Name) + " ist gesperrt — seine Aufgaben sind nicht mehr sichtbar.",
		model.TraegerBeantragt:  "Träger " + zitat(t.Name) + " steht wieder auf „beantragt“.",
	}
	a.setFlash(w, "success", meldung[status])
	http.Redirect(w, r, fmt.Sprintf("%s/%d", traegerBasis, id), http.StatusSeeOther)
}

// traegerAusFormular liest die Eingaben. betreiber entscheidet, ob
// Zulassungsstand und Zitadel-Projekt übernommen werden — beides bleibt
// sonst unverändert, statt still zurückgesetzt zu werden.
func traegerAusFormular(r *http.Request, betreiber bool, vorhanden model.Traeger) (model.Traeger, error) {
	if err := r.ParseForm(); err != nil {
		return vorhanden, err
	}
	t := vorhanden
	t.Name = strings.TrimSpace(r.FormValue("name"))
	t.Beschreibung = strings.TrimSpace(r.FormValue("beschreibung"))
	if v := r.FormValue("sichtbarkeit"); v != "" {
		t.Sichtbarkeit = model.TraegerSichtbarkeit(v)
	}
	if t.Sichtbarkeit == "" {
		t.Sichtbarkeit = model.TraegerOffen
	}
	if betreiber {
		t.ProjektID = strings.TrimSpace(r.FormValue("projektId"))
		if v := r.FormValue("status"); v != "" {
			t.Status = model.TraegerStatus(v)
		}
	}
	if t.Status == "" {
		t.Status = model.TraegerBeantragt
	}
	if err := t.Validate(); err != nil {
		return t, err
	}
	return t, nil
}

// --- Befähigungen -----------------------------------------------------------

func (a *App) befaehigungAnlegen(w http.ResponseWriter, r *http.Request, s session) {
	t, z, ok := a.traegerAusPfad(w, r, s)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.fail(w, r, http.StatusBadRequest, err)
		return
	}
	b := model.Befaehigung{
		TraegerID:    t.ID,
		Name:         strings.TrimSpace(r.FormValue("name")),
		Beschreibung: strings.TrimSpace(r.FormValue("beschreibung")),
		CreatedAt:    a.now(),
	}
	if err := b.Validate(); err != nil {
		a.zeigeTraeger(w, r, http.StatusBadRequest, t, z, nil, err.Error())
		return
	}
	if err := a.db.InsertBefaehigung(&b); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Befähigung "+zitat(b.Name)+" wurde angelegt.")
	http.Redirect(w, r, fmt.Sprintf("%s/%d", traegerBasis, t.ID), http.StatusSeeOther)
}

// befaehigungAusPfad lädt eine Befähigung und stellt sicher, dass sie dem
// Träger aus dem Pfad gehört — sonst ließe sich über eine fremde Kennung an
// den Befähigungen eines anderen Vereins drehen.
func (a *App) befaehigungAusPfad(w http.ResponseWriter, r *http.Request, t model.Traeger) (model.Befaehigung, bool) {
	id, err := strconv.ParseInt(r.PathValue("bid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return model.Befaehigung{}, false
	}
	b, err := a.db.GetBefaehigung(id)
	if err != nil || b.TraegerID != t.ID {
		http.NotFound(w, r)
		return model.Befaehigung{}, false
	}
	return *b, true
}

func (a *App) befaehigungSpeichern(w http.ResponseWriter, r *http.Request, s session) {
	t, z, ok := a.traegerAusPfad(w, r, s)
	if !ok {
		return
	}
	b, ok := a.befaehigungAusPfad(w, r, t)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.fail(w, r, http.StatusBadRequest, err)
		return
	}
	b.Name = strings.TrimSpace(r.FormValue("name"))
	b.Beschreibung = strings.TrimSpace(r.FormValue("beschreibung"))
	if err := b.Validate(); err != nil {
		a.zeigeTraeger(w, r, http.StatusBadRequest, t, z, nil, err.Error())
		return
	}
	if err := a.db.UpdateBefaehigung(&b); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Befähigung gespeichert.")
	http.Redirect(w, r, fmt.Sprintf("%s/%d", traegerBasis, t.ID), http.StatusSeeOther)
}

// befaehigungLoeschenFrage ist eine eigene Seite statt eines Popups — die
// Verwaltung kommt ohne Modals aus.
func (a *App) befaehigungLoeschenFrage(w http.ResponseWriter, r *http.Request, s session) {
	t, _, ok := a.traegerAusPfad(w, r, s)
	if !ok {
		return
	}
	b, ok := a.befaehigungAusPfad(w, r, t)
	if !ok {
		return
	}
	betroffen, err := a.aufgabenMitBefaehigung(b.ID)
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	hinweis := "Die Befähigung wird entfernt."
	if betroffen > 0 {
		hinweis = fmt.Sprintf(
			"%d Aufgabe(n) verlangen sie derzeit. Sie bleiben bestehen und sind danach ohne Einweisung zusagbar.",
			betroffen)
	}
	a.render(w, r, http.StatusOK, "bestaetigen", view{
		Title: "Befähigung löschen?", Nav: "traeger",
		Data: bestaetigenDaten{
			Ueberschrift: "Befähigung löschen",
			Text:         zitat(b.Name) + " wird gelöscht. " + hinweis,
			Aktion:       fmt.Sprintf("%s/%d/befaehigungen/%d/loeschen", traegerBasis, t.ID, b.ID),
			Knopf:        "Ja, Befähigung löschen",
			Zurueck:      fmt.Sprintf("%s/%d", traegerBasis, t.ID),
		},
	})
}

func (a *App) befaehigungLoeschen(w http.ResponseWriter, r *http.Request, s session) {
	t, _, ok := a.traegerAusPfad(w, r, s)
	if !ok {
		return
	}
	b, ok := a.befaehigungAusPfad(w, r, t)
	if !ok {
		return
	}
	if err := a.db.DeleteBefaehigung(b.ID); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Befähigung "+zitat(b.Name)+" wurde gelöscht.")
	http.Redirect(w, r, fmt.Sprintf("%s/%d", traegerBasis, t.ID), http.StatusSeeOther)
}

// aufgabenMitBefaehigung zählt, wie viele Aufgaben diese Einweisung verlangen.
func (a *App) aufgabenMitBefaehigung(id int64) (int, error) {
	tasks, err := a.db.ListTasks()
	if err != nil {
		return 0, err
	}
	n := 0
	for _, t := range tasks {
		if t.BefaehigungID == id {
			n++
		}
	}
	return n, nil
}

// --- Anträge ----------------------------------------------------------------

func (a *App) antragEntscheiden(w http.ResponseWriter, r *http.Request, s session) {
	t, z, ok := a.traegerAusPfad(w, r, s)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("aid"), 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	antrag, err := a.db.GetAntrag(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Der Antrag muss zu einer Befähigung dieses Trägers gehören.
	b, err := a.db.GetBefaehigung(antrag.BefaehigungID)
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
		http.Redirect(w, r, fmt.Sprintf("%s/%d", traegerBasis, t.ID), http.StatusSeeOther)
		return
	}
	notiz := strings.TrimSpace(r.FormValue("notiz"))
	if err := a.db.EntscheideAntrag(antrag.ID, status, z.Sub, notiz, a.now()); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	if status == model.AntragErteilt {
		a.setFlash(w, "success", "Befähigung "+zitat(b.Name)+" wurde erteilt.")
	} else {
		a.setFlash(w, "success", "Antrag auf "+zitat(b.Name)+" wurde abgelehnt.")
	}
	http.Redirect(w, r, fmt.Sprintf("%s/%d", traegerBasis, t.ID), http.StatusSeeOther)
}

// offeneAntraege zählt die unentschiedenen Anträge über alle Träger hinweg.
// Der Zähler steht auf der Bereichsübersicht; ein Fehler beim Zählen darf sie
// nicht anhalten und ergibt dann schlicht 0.
func (a *App) offeneAntraege() int {
	alle, err := a.db.ListTraeger()
	if err != nil {
		return 0
	}
	n := 0
	for _, t := range alle {
		offen, err := a.db.ListAntraege(t.ID, model.AntragBeantragt)
		if err != nil {
			continue
		}
		n += len(offen)
	}
	return n
}

// befaehigungenVon liefert die Einweisungen des Trägers, dem ein Ort gehört.
// Nur aus ihnen lässt sich für eine Aufgabe eine auswählen.
func (a *App) befaehigungenVon(p model.Place) []model.Befaehigung {
	liste, err := a.db.ListBefaehigungen(p.TraegerID)
	if err != nil {
		return nil
	}
	return liste
}

// pruefeBefaehigung stellt sicher, dass eine Aufgabe nur eine Einweisung
// ihres eigenen Trägers verlangt — dieselbe Regel wie in der REST-API.
func (a *App) pruefeBefaehigung(befaehigungID, traegerID int64) error {
	if befaehigungID == 0 {
		return nil
	}
	b, err := a.db.GetBefaehigung(befaehigungID)
	if err != nil {
		return errors.New("Die angegebene Befähigung gibt es nicht.")
	}
	if b.TraegerID != traegerID {
		return errors.New("Eine Aufgabe kann nur eine Befähigung ihres eigenen Trägers verlangen.")
	}
	return nil
}

// verwaltetIrgendwas sagt, ob diese Person mindestens einen Träger verwaltet.
// Gebraucht wird das bei der Anmeldung: Ein Vereinsvorstand ohne globale
// admin-Rolle soll herein, ein bloßes Mitglied nicht.
func (a *App) verwaltetIrgendwas(r *http.Request, u auth.User) bool {
	z := mitglied.Zugriff(r.Context(), a.mitglieder, u)
	return len(a.meineTraeger(z)) > 0
}

// rollenListe macht aus den Rollen im Token eine speicherbare Liste.
func rollenListe(u auth.User) []string {
	out := make([]string, 0, len(u.Roles))
	for rolle := range u.Roles {
		out = append(out, rolle)
	}
	sort.Strings(out)
	return out
}

// wertOderNull liest einen optionalen Wert für die Wiedervorlage im Formular.
func wertOderNull(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}
