// Package admin liefert die Web-Verwaltung der Dorf-App aus.
//
// Aufbau: server-gerendertes Multi-Page-Interface (html/template) mit echter
// Seitennavigation und Post/Redirect/Get. Keine Modals, keine Overlays, kein
// clientseitiger Zustand — JavaScript wird ausschließlich für die Karte
// verwendet, die Verwaltung ist ohne JavaScript vollständig bedienbar.
//
// Die Anmeldung läuft komplett serverseitig (Authorization Code + PKCE gegen
// die Rössing-ID); es landet kein Token im Browser, sondern nur ein signiertes,
// HttpOnly-Session-Cookie.
//
// Die Verwaltung ist in Bereiche gegliedert: /admin/ zeigt die Bereiche,
// der Bereich „Mithelfen“ liegt unter /admin/mithelfen/. Weitere Bereiche
// (z.B. Dorfladen RNah) lassen sich danebensetzen, ohne URLs umzubauen.
package admin

import (
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/mitglied"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

//go:embed templates/*.html
var templatesFS embed.FS

// static enthält das gebaute Tailwind/DaisyUI-CSS (siehe package.json,
// `npm run build:css`) und das Karten-Skript. Beides wird committet, damit
// zur Laufzeit nichts von einem CDN nachgeladen wird.
//
//go:embed static
var staticFS embed.FS

// MapLibre GL liegt lokal im Repo statt auf einem CDN: keine Drittanbieter-
// Requests aus dem Browser der Nutzer und ein reproduzierbarer Browser-E2E.
//
//go:embed vendor/maplibre-gl.js vendor/maplibre-gl.css
var vendorFS embed.FS

// Config bündelt alles, was die Verwaltung braucht.
type Config struct {
	DB       *db.DB
	Verifier auth.Verifier
	// Issuer der Rössing-ID, z.B. https://id.xn--rssing-wxa.de
	Issuer string
	// ClientID der Web-Admin-App (User-Agent-App mit PKCE, ohne Secret).
	ClientID string
	// PublicURL ist die öffentliche Basis-URL; daraus entsteht die Redirect-URI.
	PublicURL string
	// SessionKey signiert die Cookies. Leer = zufälliger Schlüssel beim Start
	// (dann sind Sessions nach einem Neustart ungültig).
	SessionKey []byte
	// Mitglieder liefert die Träger-Mitgliedschaften (Zitadel-Dienst-Nutzer,
	// siehe internal/mitglied). Ohne Angabe verwaltet nur der Betreiber.
	Mitglieder mitglied.Quelle
	Now        func() time.Time
}

// App hält den Zustand der Verwaltung.
type App struct {
	db            *db.DB
	verifier      auth.Verifier
	clientID      string
	redirectURI   string
	secureCookies bool
	signer        *signer
	discovery     *discoverer
	now           func() time.Time
	pages         map[string]*template.Template
	mitglieder    mitglied.Quelle
}

// Register hängt Startseite und Verwaltung an den Mux.
func Register(mux *http.ServeMux, cfg Config) {
	newApp(cfg).register(mux)
}

func newApp(cfg Config) *App {
	base := strings.TrimSuffix(cfg.PublicURL, "/")
	a := &App{
		db:            cfg.DB,
		verifier:      cfg.Verifier,
		clientID:      cfg.ClientID,
		redirectURI:   base + "/admin/",
		secureCookies: strings.HasPrefix(base, "https://"),
		signer:        newSigner(cfg.SessionKey),
		discovery:     &discoverer{issuer: cfg.Issuer},
		now:           cfg.Now,
		pages:         parsePages(),
		mitglieder:    cfg.Mitglieder,
	}
	if a.now == nil {
		a.now = time.Now
	}
	return a
}

func (a *App) register(mux *http.ServeMux) {
	// Startseite. Muss hier liegen, weil "/" sonst als Catch-all jeden
	// unbekannten Pfad fangen würde.
	mux.HandleFunc("GET /{$}", a.handleRoot)
	mux.Handle("GET /admin/static/", cacheAssets(http.StripPrefix("/admin/", http.FileServerFS(staticFS))))

	// Ohne Client-ID gibt es keine Anmeldung — dann bleibt es bei der Startseite.
	if a.clientID == "" {
		return
	}

	mux.HandleFunc("GET /admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusMovedPermanently)
	})
	mux.Handle("GET /admin/vendor/", cacheAssets(http.StripPrefix("/admin/", http.FileServerFS(vendorFS))))

	mux.HandleFunc("GET /admin/{$}", a.handleAdminHome)
	mux.HandleFunc("GET /admin/login", a.handleLogin)
	mux.HandleFunc("POST /admin/logout", a.handleLogout)

	a.registerMithelfen(mux)
	a.registerTraeger(mux)
	a.registerIdeen(mux)
	a.registerDorfbewohner(mux)
}

// cacheAssets markiert die mitgelieferten Assets als cachebar.
func cacheAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		next.ServeHTTP(w, r)
	})
}

// --- Seiten-Grundgerüst -----------------------------------------------------

// view ist das Datenpaket jeder Seite: Rahmen (Titel, Navigation, Nutzer,
// Meldung) plus die seitenspezifischen Daten unter .Data.
type view struct {
	Title string
	// Nav markiert den aktiven Navigationspunkt ("verwaltung", "mithelfen", …).
	Nav   string
	User  *session
	Flash *flash
	Data  any
}

var funcs = template.FuncMap{
	"statusText":  statusText,
	"statusBadge": statusBadge,
	// Vergabe: Anlass einer Zustellung und Stand eines Vorgangs im Klartext.
	"zustellungsArt": zustellungsArt,
	"vergabeStand":   vergabeStandText,
	"ortsart":        ortsart,
	"aufgabenart":    aufgabenart,
	// Träger: Zulassungsstand, Sichtbarkeit und Anträge im Klartext.
	"traegerStatus":       traegerStatusText,
	"traegerBadge":        traegerBadge,
	"traegerSichtbarkeit": traegerSichtbarkeitText,
	"antragStatus":        antragStatusText,
	"antragBadge":         antragBadge,
	"aufgabenSicht":       aufgabenSichtText,
	// Alle Zeiten stehen in der Ortszeit des Dorfes (Europe/Berlin) — der
	// Server läuft in UTC, gelesen wird die Seite aber in Rössing.
	"datum":     func(t time.Time) string { return ortszeit(t).Format("02.01.2006") },
	"datumZeit": func(t time.Time) string { return ortszeit(t).Format("02.01.2006, 15:04") },
	// datumOpt formatiert einen optionalen Zeitpunkt (leer statt Nulldatum).
	"datumOpt": func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return ortszeit(*t).Format("02.01.2006")
	},
	// datumZeitOpt formatiert einen optionalen Zeitpunkt mit Uhrzeit.
	"datumZeitOpt": func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return ortszeit(*t).Format("02.01.2006, 15:04")
	},
	// datumFeld liefert einen optionalen Zeitpunkt so, wie ihn ein
	// <input type="date"> erwartet — in Ortszeit, damit aus dem 20. um
	// 23:59 nicht der 20. um 21:59 UTC und damit derselbe Tag wird.
	"datumFeld": func(t *time.Time) string {
		if t == nil {
			return ""
		}
		return ortszeit(*t).Format("2006-01-02")
	},
	"zahl": zahl,
	// zahlOpt formatiert eine optionale Zahl (leer statt 0) — für Formularfelder.
	"zahlOpt": func(v *float64) string {
		if v == nil {
			return ""
		}
		return zahl(*v)
	},
	"liter": func(v *float64) string {
		if v == nil {
			return ""
		}
		return zahl(*v) + " l"
	},
}

// parsePages baut je Seitenvorlage ein eigenes Template-Set aus Rahmen + Seite.
func parsePages() map[string]*template.Template {
	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		panic("admin: Templates nicht lesbar: " + err.Error())
	}
	out := map[string]*template.Template{}
	for _, e := range entries {
		name := e.Name()
		if name == "layout.html" {
			continue
		}
		t := template.Must(template.New(name).Funcs(funcs).
			ParseFS(templatesFS, "templates/layout.html", "templates/"+name))
		out[strings.TrimSuffix(name, ".html")] = t
	}
	return out
}

// render schreibt eine Seite. Meldungen (Flash) werden dabei eingesammelt.
func (a *App) render(w http.ResponseWriter, r *http.Request, status int, page string, v view) {
	t, ok := a.pages[page]
	if !ok {
		a.fail(w, r, http.StatusInternalServerError, errUnbekannteSeite(page))
		return
	}
	v.Flash = a.takeFlash(w, r)
	if s, ok := a.sessionOf(r); ok {
		v.User = &s
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := t.ExecuteTemplate(w, "layout", v); err != nil {
		slog.Error("admin: Template-Fehler", "seite", page, "err", err)
	}
}

type errUnbekannteSeite string

func (e errUnbekannteSeite) Error() string { return "unbekannte Seite: " + string(e) }

// fail meldet einen technischen Fehler als schlichte Textantwort. Die Ursache
// bleibt im Log: Datenbank- und Template-Fehler verraten sonst Interna an
// jeden, der eine Seite aufruft.
func (a *App) fail(w http.ResponseWriter, r *http.Request, status int, err error) {
	slog.Error("admin: Fehler", "status", status, "pfad", r.URL.Path, "err", err)
	http.Error(w, "Es ist ein technischer Fehler aufgetreten. Bitte später erneut versuchen.", status)
}

// requireAdmin schützt alle Verwaltungsseiten. Ohne Session geht es zurück
// auf die Anmeldeseite.
func (a *App) requireAdmin(h func(http.ResponseWriter, *http.Request, session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, ok := a.sessionOf(r)
		if !ok || !s.Admin {
			a.setFlash(w, "error", "Bitte zuerst mit der Rössing-ID anmelden.")
			http.Redirect(w, r, "/admin/", http.StatusSeeOther)
			return
		}
		h(w, r, s)
	}
}

// --- Allgemeine Seiten ------------------------------------------------------

func (a *App) handleRoot(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "start", view{Title: "Dorf-App Rössing"})
}

// handleAdminHome ist gleichzeitig OIDC-Callback: die Rössing-ID schickt den
// Code an genau diese, bereits registrierte Redirect-URI zurück.
func (a *App) handleAdminHome(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("code") != "" || q.Get("error") != "" {
		a.handleCallback(w, r)
		return
	}
	s, ok := a.sessionOf(r)
	if !ok || !s.Admin {
		a.render(w, r, http.StatusOK, "anmelden", view{Title: "Anmelden"})
		return
	}
	a.render(w, r, http.StatusOK, "verwaltung", view{Title: "Verwaltung", Nav: "verwaltung",
		Data: a.bereichsDaten()})
}

// --- Anzeige-Helfer ---------------------------------------------------------

func statusText(s model.Status) string {
	switch s {
	case model.StatusYellow:
		return "fällig"
	case model.StatusRed:
		return "überfällig"
	default:
		return "in Ordnung"
	}
}

// statusBadge liefert die passende DaisyUI-Badge-Klasse.
func statusBadge(s model.Status) string {
	switch s {
	case model.StatusYellow:
		return "badge-warning"
	case model.StatusRed:
		return "badge-error"
	default:
		return "badge-success"
	}
}

// --- Träger im Klartext -----------------------------------------------------

func traegerStatusText(s model.TraegerStatus) string {
	switch s {
	case model.TraegerZugelassen:
		return "zugelassen"
	case model.TraegerGesperrt:
		return "gesperrt"
	default:
		return "beantragt"
	}
}

func traegerBadge(s model.TraegerStatus) string {
	switch s {
	case model.TraegerZugelassen:
		return "badge-success"
	case model.TraegerGesperrt:
		return "badge-error"
	default:
		return "badge-warning"
	}
}

func traegerSichtbarkeitText(s model.TraegerSichtbarkeit) string {
	if s == model.TraegerGeschlossen {
		return "geschlossen"
	}
	return "offen"
}

func antragStatusText(s model.AntragStatus) string {
	switch s {
	case model.AntragErteilt:
		return "erteilt"
	case model.AntragAbgelehnt:
		return "abgelehnt"
	default:
		return "beantragt"
	}
}

func antragBadge(s model.AntragStatus) string {
	switch s {
	case model.AntragErteilt:
		return "badge-success"
	case model.AntragAbgelehnt:
		return "badge-ghost"
	default:
		return "badge-warning"
	}
}

// aufgabenSichtText benennt die Sichtbarkeit einer Aufgabe.
func aufgabenSichtText(s model.TaskSichtbarkeit) string {
	if s == model.AufgabeNurMitglieder {
		return "nur Mitglieder"
	}
	return "öffentlich"
}

func ortsart(k model.PlaceKind) string {
	switch k {
	case model.PlaceFlowerbox:
		return "Blumenkasten"
	case model.PlaceBed:
		return "Beet"
	default:
		return "Sonstiges"
	}
}

func aufgabenart(k model.TaskKind) string {
	switch k {
	case model.TaskWatering:
		return "Gießen"
	case model.TaskWeeding:
		return "Jäten"
	default:
		return "Sonstiges"
	}
}

// zahl formatiert Kommazahlen kurz (10 statt 10.000000).
func zahl(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// ortszeit rechnet einen Zeitpunkt in die Ortszeit des Dorfes um.
func ortszeit(t time.Time) time.Time { return t.In(model.Location()) }

func anzeigeName(s session) string {
	if s.Name != "" {
		return s.Name
	}
	return s.Sub
}
