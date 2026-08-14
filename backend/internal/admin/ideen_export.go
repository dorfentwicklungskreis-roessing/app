package admin

import (
	"encoding/csv"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Export der Ideen als Tabelle. Gedacht zum Durchgehen außerhalb der
// Verwaltung — im Tabellenprogramm, auf Papier, in einer Sitzung.
//
// Zwei Eigenheiten, die im Alltag zählen:
//   - Semikolon statt Komma und eine BOM voran, damit deutsche
//     Tabellenprogramme die Datei ohne Import-Dialog richtig öffnen.
//   - Zellen, die mit =, +, - oder @ beginnen, werden entschärft: Excel und
//     LibreOffice würden sie sonst als Formel auswerten, und der Inhalt
//     kommt hier von außen.

// ideenExportSpalten ist die Kopfzeile — Reihenfolge wie in der Liste.
var ideenExportSpalten = []string{"ID", "Eingegangen", "Name", "E-Mail", "Wunsch", "Weg", "Stand", "Notiz"}

func (a *App) ideenExport(w http.ResponseWriter, r *http.Request, _ session) {
	status, err := api.IdeeStatusAus(r.URL.Query().Get("status"))
	if err != nil {
		http.Redirect(w, r, ideenBasis+"/export.csv", http.StatusSeeOther)
		return
	}
	ideen, err := a.db.ListIdeen(status)
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}

	name := "ideen"
	if status != "" {
		name += "-" + string(status)
	}
	name += "-" + a.now().In(model.Location()).Format("2006-01-02") + ".csv"

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	// Ein Export ist immer eine Momentaufnahme und gehört in keinen Cache.
	w.Header().Set("Cache-Control", "no-store")

	// BOM: ohne sie liest Excel die Umlaute falsch.
	if _, err := w.Write([]byte("\ufeff")); err != nil {
		return
	}
	c := csv.NewWriter(w)
	c.Comma = ';'
	zeilen := [][]string{ideenExportSpalten}
	for _, i := range ideen {
		zeilen = append(zeilen, []string{
			strconv.FormatInt(i.ID, 10),
			ortszeit(i.CreatedAt).Format("02.01.2006 15:04"),
			zelle(i.Name),
			zelle(i.Email),
			zelle(i.Wunsch),
			ideeWegText(i),
			model.IdeeStatusText(i.Status),
			zelle(i.Notiz),
		})
	}
	if err := c.WriteAll(zeilen); err != nil {
		slog.Error("admin: Ideen-Export abgebrochen", "err", err)
	}
}

// ideeWegText sagt, wie ein Wunsch hereinkam.
func ideeWegText(i model.Idee) string {
	if i.Quelle == model.IdeeQuelleApp {
		if i.UserSub != "" {
			return "App (angemeldet)"
		}
		return "App"
	}
	return "Website"
}

// zelle entschärft eine Zelle für Tabellenprogramme. Der Inhalt bleibt
// vollständig erhalten — er rutscht nur hinter ein Hochkomma, das die
// Programme als „das ist Text“ verstehen und nicht mit anzeigen.
func zelle(s string) string {
	if s == "" {
		return s
	}
	if strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
