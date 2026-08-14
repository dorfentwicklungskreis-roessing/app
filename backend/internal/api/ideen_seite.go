package api

import (
	"embed"
	"html/template"
	"log/slog"
	"net/http"
)

// Die Fehlerseite des öffentlichen Ideen-Eingangs. Sie ist nötig, weil das
// Formular auf der Website ohne JavaScript funktionieren muss: Bei einer
// abgewiesenen Eingabe gäbe es sonst nur eine nackte Fehlermeldung, und der
// getippte Text wäre beim Zurückgehen im Zweifel weg. Hier kommt er
// vollständig zurück.
//
// Das Aussehen kommt aus demselben gebauten Tailwind/DaisyUI-CSS wie die
// Verwaltung (/admin/static/app.css) — es wird nichts von einem CDN geladen.
// Damit die hier verwendeten Klassen im gebauten CSS landen, ist dieses
// Verzeichnis in internal/admin/tailwind.css als @source eingetragen.
//
//go:embed templates/idee_fehler.html
var ideenTemplateFS embed.FS

var ideenFehlerSeite = template.Must(template.ParseFS(ideenTemplateFS, "templates/idee_fehler.html"))

type ideeFehlerDaten struct {
	Fehler   string
	Eingabe  IdeeEingabe
	Redirect string
	Zurueck  string
}

func (s *Server) ideeFehlerSeite(w http.ResponseWriter, in IdeeEingabe, meldung string) {
	// Der Honigtopf und der Zeitstempel gehen bewusst nicht mit zurück: Beim
	// zweiten Anlauf soll wieder von vorn geprüft werden.
	daten := ideeFehlerDaten{
		Fehler:   meldung,
		Eingabe:  IdeeEingabe{Name: in.Name, Email: in.Email, Wunsch: in.Wunsch},
		Redirect: in.Redirect,
		Zurueck:  s.ideenFormularAdresse(),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	if err := ideenFehlerSeite.Execute(w, daten); err != nil {
		slog.Error("Ideen-Fehlerseite konnte nicht gerendert werden", "err", err)
	}
}

// ideenFormularAdresse führt zurück zum Formular auf der Website.
func (s *Server) ideenFormularAdresse() string {
	ziele := s.ideenZiele()
	if len(ziele) == 0 {
		return "/"
	}
	return ziele[0] + IdeenFormularPfad
}
