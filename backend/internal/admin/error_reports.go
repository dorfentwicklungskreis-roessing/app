package admin

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Area "Fehlerberichte" — what went wrong on someone's phone.
//
// They come in from both apps (see internal/api/error_reports.go), with one
// tap and without anybody having to describe the problem. Here they are read,
// sorted and cleared away: server rendered, no modals, deleting through its
// own confirmation page — the same shape the ideas already have.
//
// The URL prefix stays German like the other areas of the administration:
// somebody reads it in the address bar.
const errorReportsBase = "/admin/fehlerberichte"

func (a *App) registerErrorReports(mux *http.ServeMux) {
	get := func(pfad string, h func(http.ResponseWriter, *http.Request, session)) {
		mux.HandleFunc("GET "+errorReportsBase+pfad, a.requireAdmin(h))
	}
	post := func(pfad string, h func(http.ResponseWriter, *http.Request, session)) {
		mux.HandleFunc("POST "+errorReportsBase+pfad, a.requireAdmin(h))
	}
	get("/{$}", a.errorReportList)
	get("/{id}", a.errorReportDetail)
	post("/{id}", a.errorReportSave)
	get("/{id}/loeschen", a.errorReportDeleteAsk)
	post("/{id}/loeschen", a.errorReportDelete)
}

// --- List ---------------------------------------------------------------------

type errorReportListData struct {
	Reports []errorReportRow
	// Filter are the clickable states with their counts — real links, no
	// client-side filtering.
	Filter []errorReportFilter
	// Kinds filter by sort of malfunction, next to the states.
	Kinds []errorReportFilter
	Aktiv string
	// AktivTitel is the chosen state in everyday German (for empty states).
	AktivTitel string
	AktiveArt  string
}

type errorReportRow struct {
	model.ErrorReport
	Badge      string
	StatusText string
	KindText   string
	KindBadge  string
	// Anonym marks a report that came in without a login — that happens
	// exactly when signing in is what broke.
	Anonym bool
}

type errorReportFilter struct {
	Wert   string
	Titel  string
	Anzahl int
	Aktiv  bool
	// Href is the ready-made link, so the template does not have to stitch
	// two query parameters together.
	Href string
}

func (a *App) errorReportList(w http.ResponseWriter, r *http.Request, _ session) {
	status, err := api.ErrorReportStatusFrom(r.URL.Query().Get("status"))
	if err != nil {
		http.Redirect(w, r, errorReportsBase+"/", http.StatusSeeOther)
		return
	}
	kind, err := api.ErrorReportKindFrom(r.URL.Query().Get("art"))
	if err != nil {
		http.Redirect(w, r, errorReportsBase+"/", http.StatusSeeOther)
		return
	}
	reports, err := a.db.ListErrorReports(status, kind)
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	anzahl, err := a.db.CountErrorReports()
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	// Die Zahlen je Art beschreiben immer den ganzen Bestand — sonst zeigte
	// ein gewählter Filter nur noch sich selbst.
	alle, err := a.db.ListErrorReports("", "")
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	zeilen := make([]errorReportRow, 0, len(reports))
	for _, e := range reports {
		zeilen = append(zeilen, errorReportRowOf(e))
	}
	a.render(w, r, http.StatusOK, "fehlerberichte", view{
		Title: "Fehlerberichte", Nav: "fehlerberichte",
		Data: errorReportListData{
			Reports:    zeilen,
			Filter:     errorReportStatusFilter(anzahl, string(status), string(kind)),
			Kinds:      errorReportKindFilter(alle, string(status), string(kind)),
			Aktiv:      string(status),
			AktivTitel: model.ErrorReportStatusText(status),
			AktiveArt:  string(kind),
		},
	})
}

func errorReportRowOf(e model.ErrorReport) errorReportRow {
	return errorReportRow{
		ErrorReport: e,
		Badge:       errorReportBadge(e.Status),
		StatusText:  model.ErrorReportStatusText(e.Status),
		KindText:    model.ErrorReportKindText(e.Kind),
		KindBadge:   errorReportKindBadge(e.Kind),
		Anonym:      e.UserSub == "",
	}
}

// errorReportLink builds a filter link out of state and kind. Both stay set
// at the same time, so „nur die neuen Abstürze" is one click away.
func errorReportLink(status, kind string) string {
	teile := []string{}
	if status != "" {
		teile = append(teile, "status="+status)
	}
	if kind != "" {
		teile = append(teile, "art="+kind)
	}
	if len(teile) == 0 {
		return errorReportsBase + "/"
	}
	return errorReportsBase + "/?" + strings.Join(teile, "&")
}

func errorReportStatusFilter(anzahl map[model.ErrorReportStatus]int, aktiv, art string) []errorReportFilter {
	gesamt := 0
	for _, n := range anzahl {
		gesamt += n
	}
	out := []errorReportFilter{{
		Wert: "", Titel: "Alle", Anzahl: gesamt, Aktiv: aktiv == "",
		Href: errorReportLink("", art),
	}}
	for _, s := range model.ErrorReportStatuses {
		out = append(out, errorReportFilter{
			Wert: string(s), Titel: model.ErrorReportStatusText(s),
			Anzahl: anzahl[s], Aktiv: aktiv == string(s),
			Href: errorReportLink(string(s), art),
		})
	}
	return out
}

func errorReportKindFilter(alle []model.ErrorReport, status, aktiv string) []errorReportFilter {
	jeArt := map[model.ErrorReportKind]int{}
	for _, e := range alle {
		jeArt[e.Kind]++
	}
	out := []errorReportFilter{{
		Wert: "", Titel: "Jede Art", Anzahl: len(alle), Aktiv: aktiv == "",
		Href: errorReportLink(status, ""),
	}}
	for _, k := range model.ErrorReportKinds {
		out = append(out, errorReportFilter{
			Wert: string(k), Titel: model.ErrorReportKindText(k),
			Anzahl: jeArt[k], Aktiv: aktiv == string(k),
			Href: errorReportLink(status, string(k)),
		})
	}
	return out
}

// errorReportBadge gives the DaisyUI badge class for a state.
func errorReportBadge(s model.ErrorReportStatus) string {
	switch s {
	case model.ErrorReportSeen:
		return "badge-info"
	case model.ErrorReportFixed:
		return "badge-success"
	case model.ErrorReportDropped:
		return "badge-ghost"
	default:
		return "badge-warning"
	}
}

// errorReportKindBadge makes a crash stand out from a phone without
// reception — in a list of fifty that is the difference between reading and
// scrolling past.
func errorReportKindBadge(k model.ErrorReportKind) string {
	switch k {
	case model.ErrorReportCrash:
		return "badge-error"
	case model.ErrorReportServer:
		return "badge-warning"
	case model.ErrorReportNetwork:
		return "badge-ghost"
	default:
		return "badge-outline"
	}
}

// --- Detail and saving ----------------------------------------------------------

type errorReportDetailData struct {
	Report  errorReportRow
	Staende []errorReportState
	Fehler  string
}

type errorReportState struct {
	Wert     string
	Titel    string
	Gewaehlt bool
}

func (a *App) errorReportDetail(w http.ResponseWriter, r *http.Request, _ session) {
	report, ok := a.errorReportFromPath(w, r)
	if !ok {
		return
	}
	a.showErrorReport(w, r, http.StatusOK, *report, "")
}

func (a *App) showErrorReport(w http.ResponseWriter, r *http.Request, status int,
	e model.ErrorReport, fehler string,
) {
	staende := make([]errorReportState, 0, len(model.ErrorReportStatuses))
	for _, s := range model.ErrorReportStatuses {
		staende = append(staende, errorReportState{
			Wert: string(s), Titel: model.ErrorReportStatusText(s), Gewaehlt: s == e.Status,
		})
	}
	a.render(w, r, status, "fehlerbericht", view{
		Title: "Fehlerbericht", Nav: "fehlerberichte",
		Data: errorReportDetailData{Report: errorReportRowOf(e), Staende: staende, Fehler: fehler},
	})
}

func (a *App) errorReportSave(w http.ResponseWriter, r *http.Request, _ session) {
	report, ok := a.errorReportFromPath(w, r)
	if !ok {
		return
	}
	if err := r.ParseForm(); err != nil {
		a.fail(w, r, http.StatusBadRequest, err)
		return
	}
	status, err := api.ErrorReportStatusFrom(r.FormValue("status"))
	if err != nil || status == "" {
		entwurf := *report
		entwurf.Note = strings.TrimSpace(r.FormValue("notiz"))
		a.showErrorReport(w, r, http.StatusBadRequest, entwurf, "Bitte einen gültigen Stand auswählen.")
		return
	}
	notiz := strings.TrimSpace(r.FormValue("notiz"))
	if len([]rune(notiz)) > api.MaxErrorNoteLen {
		entwurf := *report
		entwurf.Note = notiz
		a.showErrorReport(w, r, http.StatusBadRequest, entwurf, "Die Notiz ist zu lang.")
		return
	}
	report.Status, report.Note = status, notiz
	if err := a.db.UpdateErrorReport(report); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Der Fehlerbericht wurde gespeichert.")
	http.Redirect(w, r, fmt.Sprintf("%s/%d", errorReportsBase, report.ID), http.StatusSeeOther)
}

// --- Deleting -------------------------------------------------------------------

func (a *App) errorReportDeleteAsk(w http.ResponseWriter, r *http.Request, _ session) {
	report, ok := a.errorReportFromPath(w, r)
	if !ok {
		return
	}
	a.render(w, r, http.StatusOK, "bestaetigen", view{
		Title: "Fehlerbericht löschen", Nav: "fehlerberichte",
		Data: bestaetigenDaten{
			Ueberschrift: "Fehlerbericht löschen",
			Text: "Der Bericht " + zitat(kurz(report.Message, 80)) +
				" wird dauerhaft gelöscht — samt technischer Angaben, Ergänzung und interner Notiz.",
			Aktion:  fmt.Sprintf("%s/%d/loeschen", errorReportsBase, report.ID),
			Knopf:   "Ja, Bericht löschen",
			Zurueck: fmt.Sprintf("%s/%d", errorReportsBase, report.ID),
		},
	})
}

func (a *App) errorReportDelete(w http.ResponseWriter, r *http.Request, _ session) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.db.DeleteErrorReport(id); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Der Fehlerbericht wurde gelöscht.")
	http.Redirect(w, r, errorReportsBase+"/", http.StatusSeeOther)
}

func (a *App) errorReportFromPath(w http.ResponseWriter, r *http.Request) (*model.ErrorReport, bool) {
	id, err := pfadID(r)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	report, err := a.db.GetErrorReport(id)
	if err != nil {
		http.NotFound(w, r)
		return nil, false
	}
	return report, true
}
