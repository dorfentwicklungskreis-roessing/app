package admin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Bereich „Ideen“ — die Wünsche aus dem Dorf: „Was soll die App können?“
// Sie kommen aus dem Formular auf der Website und aus der App herein
// (siehe internal/api/ideen.go). Hier werden sie gelesen, eingeordnet und
// aufgeräumt: server-gerendert, ohne Modals, Löschen über eine eigene
// Bestätigungsseite.
const ideenBasis = "/admin/ideen"

func (a *App) registerIdeen(mux *http.ServeMux) {
	get := func(pfad string, h func(http.ResponseWriter, *http.Request, session)) {
		mux.HandleFunc("GET "+ideenBasis+pfad, a.requireAdmin(h))
	}
	post := func(pfad string, h func(http.ResponseWriter, *http.Request, session)) {
		mux.HandleFunc("POST "+ideenBasis+pfad, a.requireAdmin(h))
	}
	get("/{$}", a.ideenListe)
	get("/{id}", a.ideeDetail)
	post("/{id}", a.ideeSpeichern)
	get("/{id}/loeschen", a.ideeLoeschenFrage)
	post("/{id}/loeschen", a.ideeLoeschen)
}

// --- Liste --------------------------------------------------------------------

type ideenDaten struct {
	Ideen []ideeZeile
	// Filter sind die anklickbaren Staende samt Anzahl — echte Links, kein
	// clientseitiges Filtern.
	Filter []ideenFilter
	Aktiv  string
}

type ideeZeile struct {
	model.Idee
	Badge      string
	StatusText string
	// AusDerApp kennzeichnet Einreichungen aus der angemeldeten App.
	AusDerApp bool
}

type ideenFilter struct {
	Wert   string
	Titel  string
	Anzahl int
	Aktiv  bool
}

func (a *App) ideenListe(w http.ResponseWriter, r *http.Request, _ session) {
	gewaehlt := strings.TrimSpace(r.URL.Query().Get("status"))
	status, err := api.IdeeStatusAus(gewaehlt)
	if err != nil {
		http.Redirect(w, r, ideenBasis+"/", http.StatusSeeOther)
		return
	}
	ideen, err := a.db.ListIdeen(status)
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	anzahl, err := a.db.CountIdeen()
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	zeilen := make([]ideeZeile, 0, len(ideen))
	for _, i := range ideen {
		zeilen = append(zeilen, zeileAus(i))
	}
	a.render(w, r, http.StatusOK, "ideen", view{
		Title: "Ideen", Nav: "ideen",
		Data: ideenDaten{Ideen: zeilen, Filter: filterAus(anzahl, string(status)), Aktiv: string(status)},
	})
}

func zeileAus(i model.Idee) ideeZeile {
	return ideeZeile{
		Idee: i, Badge: ideeBadge(i.Status), StatusText: model.IdeeStatusText(i.Status),
		AusDerApp: i.Quelle == model.IdeeQuelleApp,
	}
}

func filterAus(anzahl map[model.IdeeStatus]int, aktiv string) []ideenFilter {
	gesamt := 0
	for _, n := range anzahl {
		gesamt += n
	}
	out := []ideenFilter{{Wert: "", Titel: "Alle", Anzahl: gesamt, Aktiv: aktiv == ""}}
	for _, s := range model.IdeeStatusWerte {
		out = append(out, ideenFilter{
			Wert: string(s), Titel: model.IdeeStatusText(s),
			Anzahl: anzahl[s], Aktiv: aktiv == string(s),
		})
	}
	return out
}

// ideeBadge liefert die passende DaisyUI-Badge-Klasse zum Stand.
func ideeBadge(s model.IdeeStatus) string {
	switch s {
	case model.IdeeGelesen:
		return "badge-info"
	case model.IdeeUmgesetzt:
		return "badge-success"
	case model.IdeeAbgelehnt:
		return "badge-ghost"
	default:
		return "badge-warning"
	}
}

// --- Detail und Speichern -------------------------------------------------------

type ideeDetailDaten struct {
	Idee    ideeZeile
	Staende []ideeStand
	Fehler  string
}

type ideeStand struct {
	Wert     string
	Titel    string
	Gewaehlt bool
}

func (a *App) ideeDetail(w http.ResponseWriter, r *http.Request, _ session) {
	idee, ok := a.ideeAusPfad(w, r)
	if !ok {
		return
	}
	a.zeigeIdee(w, r, http.StatusOK, *idee, "")
}

func (a *App) zeigeIdee(w http.ResponseWriter, r *http.Request, status int, i model.Idee, fehler string) {
	staende := make([]ideeStand, 0, len(model.IdeeStatusWerte))
	for _, s := range model.IdeeStatusWerte {
		staende = append(staende, ideeStand{
			Wert: string(s), Titel: model.IdeeStatusText(s), Gewaehlt: s == i.Status,
		})
	}
	a.render(w, r, status, "idee", view{
		Title: "Idee", Nav: "ideen",
		Data: ideeDetailDaten{Idee: zeileAus(i), Staende: staende, Fehler: fehler},
	})
}

func (a *App) ideeSpeichern(w http.ResponseWriter, r *http.Request, _ session) {
	idee, ok := a.ideeAusPfad(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.fail(w, r, http.StatusBadRequest, err)
		return
	}
	status, err := api.IdeeStatusAus(r.FormValue("status"))
	if err != nil || status == "" {
		entwurf := *idee
		entwurf.Notiz = strings.TrimSpace(r.FormValue("notiz"))
		a.zeigeIdee(w, r, http.StatusBadRequest, entwurf, "Bitte einen gültigen Stand auswählen.")
		return
	}
	notiz := strings.TrimSpace(r.FormValue("notiz"))
	if len([]rune(notiz)) > api.MaxIdeeNotizLen {
		entwurf := *idee
		entwurf.Notiz = notiz
		a.zeigeIdee(w, r, http.StatusBadRequest, entwurf, "Die Notiz ist zu lang.")
		return
	}
	idee.Status, idee.Notiz = status, notiz
	if err := a.db.UpdateIdee(idee); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Die Idee wurde gespeichert.")
	http.Redirect(w, r, fmt.Sprintf("%s/%d", ideenBasis, idee.ID), http.StatusSeeOther)
}

// --- Löschen ------------------------------------------------------------------

func (a *App) ideeLoeschenFrage(w http.ResponseWriter, r *http.Request, _ session) {
	idee, ok := a.ideeAusPfad(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "bestaetigen", view{
		Title: "Idee löschen", Nav: "ideen",
		Data: bestaetigenDaten{
			Ueberschrift: "Idee löschen",
			Text:         "Der Wunsch " + zitat(kurz(idee.Wunsch, 80)) + " wird dauerhaft gelöscht — samt Name, E-Mail und interner Notiz.",
			Aktion:       fmt.Sprintf("%s/%d/loeschen", ideenBasis, idee.ID),
			Knopf:        "Ja, Idee löschen",
			Zurueck:      fmt.Sprintf("%s/%d", ideenBasis, idee.ID),
		},
	})
}

func (a *App) ideeLoeschen(w http.ResponseWriter, r *http.Request, _ session) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.db.DeleteIdee(id); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Die Idee wurde gelöscht.")
	http.Redirect(w, r, ideenBasis+"/", http.StatusSeeOther)
}

// --- Kleinkram ------------------------------------------------------------------

func (a *App) ideeAusPfad(w http.ResponseWriter, r *http.Request) (*model.Idee, bool) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	idee, err := a.db.GetIdee(id)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	return idee, true
}

// kurz schneidet einen Text für Überschriften auf eine handliche Länge.
func kurz(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return strings.TrimSpace(string(r[:max])) + "…"
}

// bereichsDaten liefert die Kennzahlen der Bereichsübersicht. Bislang ist
// das nur der Zähler der noch ungelesenen Ideen — er gehört auf die
// Startseite, weil dort sonst niemand merkt, dass etwas hereingekommen ist.
type bereichsDaten struct {
	NeueIdeen int
}

func (a *App) bereichsDaten() bereichsDaten {
	anzahl, err := a.db.CountIdeen()
	if err != nil {
		return bereichsDaten{}
	}
	return bereichsDaten{NeueIdeen: anzahl[model.IdeeNeu]}
}
