// Package api stellt die REST-API der Dorf-App bereit.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/httpx"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/mitglied"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/vergabe"
)

type Server struct {
	DB  *db.DB
	Now func() time.Time
	// Zusteller bekommt Benachrichtigungen, die aus einem API-Aufruf
	// entstehen (z.B. eine von der Verwaltung aufgehobene Zusage). Ohne
	// Angabe bleibt es bei der Abrufliste — siehe vergabe.Zusteller.
	Zusteller vergabe.Zusteller
	// OptionalAuth prüft ein mitgeschicktes Bearer-Token, verlangt aber
	// keines. Gebraucht wird das nur am öffentlichen Ideen-Eingang: Die
	// Website reicht anonym ein, die App angemeldet (siehe ideen.go).
	OptionalAuth func(http.Handler) http.Handler
	// IdeenLimiter begrenzt den öffentlichen Ideen-Eingang deutlich
	// strenger als der allgemeine Limiter. Leer = aus der Umgebung.
	IdeenLimiter *httpx.RateLimiter
	// IdeenRedirects sind die Ursprünge, auf die nach dem Absenden
	// weitergeleitet werden darf. Leer = aus der Umgebung.
	IdeenRedirects []string
	// Mitglieder liefert die Träger-Mitgliedschaften einer Person (Zitadel,
	// über einen Dienst-Nutzer — siehe internal/mitglied). Ohne Angabe gibt
	// es keine Träger-Rollen: Dann verwaltet der Betreiber alles, und alle
	// anderen sehen die öffentlichen Aufgaben.
	Mitglieder mitglied.Quelle

	// ideenEinmal baut die Zugriffsgrenze des Ideen-Eingangs genau einmal.
	ideenEinmal sync.Once
}

// Handler baut den HTTP-Router. authMW schützt alle /api/v1-Routen.
// extra darf zusätzliche Routen registrieren (z.B. /mcp, /admin).
func (s *Server) Handler(authMW func(http.Handler) http.Handler, extra func(mux *http.ServeMux)) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	api := http.NewServeMux()
	api.HandleFunc("GET /api/v1/me", s.handleMe)
	api.HandleFunc("PUT /api/v1/me/profile", s.handlePutProfile)
	api.HandleFunc("GET /api/v1/members", s.handleMembers)
	api.HandleFunc("GET /api/v1/places", s.handleListPlaces)
	// Orte und Aufgaben pflegt der admin ihres Trägers (und der Betreiber) —
	// die Prüfung sitzt in den Handlern, weil sie den betroffenen Träger
	// erst aus dem Datensatz kennen.
	api.HandleFunc("POST /api/v1/places", s.handleCreatePlace)
	api.HandleFunc("PUT /api/v1/places/{id}", s.handleUpdatePlace)
	api.HandleFunc("DELETE /api/v1/places/{id}", s.handleDeletePlace)
	api.HandleFunc("POST /api/v1/places/{id}/tasks", s.handleCreateTask)
	api.HandleFunc("PUT /api/v1/tasks/{id}", s.handleUpdateTask)
	api.HandleFunc("DELETE /api/v1/tasks/{id}", s.handleDeleteTask)
	api.HandleFunc("GET /api/v1/tasks/{id}/completions", s.handleListCompletions)
	api.HandleFunc("POST /api/v1/tasks/{id}/completions", s.handleCreateCompletion)
	api.HandleFunc("DELETE /api/v1/completions/{id}", s.handleDeleteCompletion)
	api.HandleFunc("GET /api/v1/stats/leaderboard", s.handleLeaderboard)
	s.registerVergabe(api)
	s.registerGeraete(api)
	api.HandleFunc("GET /api/v1/settings", s.handleGetSettings)
	api.HandleFunc("PUT /api/v1/settings", s.adminOnly(s.handlePutSettings))
	s.registerIdeenVerwaltung(api)
	s.registerTraeger(api)

	mux.Handle("/api/v1/", authMW(api))
	// Der Ideen-Eingang hängt bewusst außerhalb der Anmeldepflicht (siehe
	// ideen.go) und ist als genauere Route trotzdem vorrangig.
	s.registerIdeenEingang(mux)
	if extra != nil {
		extra(mux)
	}
	return logRequests(mux)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "dur", time.Since(start).Round(time.Millisecond))
	})
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// AssemblePlaces baut die Orts-Liste in der Sicht des Betreibers — alles,
// ohne Filter. Für die Web-Verwaltung und den MCP-Endpunkt, die beide bereits
// die globale admin-Rolle verlangen.
func AssemblePlaces(d *db.DB, now time.Time) ([]model.PlaceWithStatus, float64, error) {
	return AssemblePlacesFuer(d, now, model.Zugriff{Betreiber: true})
}

// AssemblePlacesFuer baut die Orts-Liste so, wie diese Person sie sehen darf.
//
// Hier hängt die schärfste Regel des Systems: Eine Aufgabe mit
// „nur_mitglieder“ wird aus der Liste entfernt, bevor irgendetwas anderes
// passiert — und ein Ort, an dem danach nichts Sichtbares übrig bleibt,
// verschwindet gleich mit. Sonst verriete eine leere Nadel auf der Karte,
// dass es dort intern etwas zu tun gibt.
func AssemblePlacesFuer(d *db.DB, now time.Time, z model.Zugriff) ([]model.PlaceWithStatus, float64, error) {
	filter, err := NeuerFilter(d, z)
	if err != nil {
		return nil, 0, err
	}
	places, err := d.ListPlaces()
	if err != nil {
		return nil, 0, err
	}
	tasks, err := d.ListTasks()
	if err != nil {
		return nil, 0, err
	}
	last, err := d.LastCompletions()
	if err != nil {
		return nil, 0, err
	}
	factor, _ := d.WateringFactor()
	// Namen kommen aus den Profilen, nicht aus dem, was beim Melden
	// eingefroren wurde (siehe model.NameResolver).
	namen, err := d.NameResolver()
	if err != nil {
		return nil, 0, err
	}

	// Orte einmal nachschlagbar machen — die Sichtbarkeit einer Aufgabe
	// hängt am Träger ihres Ortes.
	orteNachID := map[int64]model.Place{}
	for _, p := range places {
		orteNachID[p.ID] = p
	}
	// Namen der Befähigungen für die Anzeige.
	befaehigungen, err := d.ListAlleBefaehigungen()
	if err != nil {
		return nil, 0, err
	}
	befaehigungName := map[int64]string{}
	for _, b := range befaehigungen {
		befaehigungName[b.ID] = b.Name
	}

	// alleAufgaben je Ort — auch die unsichtbaren. Ob ein Ort erscheinen
	// darf, hängt an allen seinen Aufgaben, nicht nur an den sichtbaren.
	alleAufgaben := map[int64][]model.CareTask{}
	for _, t := range tasks {
		alleAufgaben[t.PlaceID] = append(alleAufgaben[t.PlaceID], t)
	}

	byPlace := map[int64][]model.TaskWithStatus{}
	for _, t := range tasks {
		ort, ok := orteNachID[t.PlaceID]
		if !ok || !filter.AufgabeSichtbar(ort, t) {
			continue
		}
		t.BefaehigungName = befaehigungName[t.BefaehigungID]
		var lc *model.Completion
		if c, ok := last[t.ID]; ok {
			c.UserName = namen.Resolve(c.UserSub, c.UserName)
			lc = &c
		}
		// Der Hitzefaktor beschleunigt nur das Gießen — Jäten etc. bleiben normal.
		f := 1.0
		if t.Kind == model.TaskWatering {
			f = factor
		}
		status, dueAt, redAt := model.ComputeStatus(t, lc, now, f)
		byPlace[t.PlaceID] = append(byPlace[t.PlaceID], model.TaskWithStatus{
			CareTask: t, Status: status, LastCompletion: lc, DueAt: dueAt, RedAt: redAt,
			LockedUntil: LockedUntil(t, lc, now, f),
		})
	}

	out := make([]model.PlaceWithStatus, 0, len(places))
	for _, p := range places {
		if !filter.OrtSichtbar(p, alleAufgaben[p.ID]) {
			continue
		}
		if t, ok := filter.Traeger(p); ok {
			// Nicht t.Name: Eine geschlossene Gruppe darf öffentlich
			// ausschreiben, ohne sich dabei zu offenbaren.
			p.TraegerName = z.TraegerAnzeigeName(t)
		}
		pws := model.PlaceWithStatus{Place: p, Tasks: byPlace[p.ID], Status: model.StatusGreen}
		if pws.Tasks == nil {
			pws.Tasks = []model.TaskWithStatus{}
		}
		for _, t := range pws.Tasks {
			if t.Active {
				pws.Status = model.Worst(pws.Status, t.Status)
			}
		}
		out = append(out, pws)
	}
	// Vergabestand je Aufgabe: wer hat zugesagt, wie viele helfen hier mit.
	if err := ergaenzeVergabe(d, out, namen); err != nil {
		return nil, 0, err
	}
	return out, factor, nil
}

// --- Helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) adminOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.FromContext(r.Context())
		if !ok || !u.IsAdmin() {
			writeErr(w, http.StatusForbidden, "admin-Rolle erforderlich")
			return
		}
		h(w, r)
	}
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// --- Handlers ---------------------------------------------------------------

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	roles := make([]string, 0, len(u.Roles))
	for role := range u.Roles {
		roles = append(roles, role)
	}
	// Das eigene Profil kommt mit: Die App braucht es beim Start ohnehin
	// (Anzeigename, Nickname) und spart sich so einen zweiten Aufruf.
	profil, err := ProfileFor(s.DB, u, s.now())
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sub": u.Sub, "name": u.Name, "email": u.Email, "roles": roles, "isAdmin": u.IsAdmin(),
		"profile": profil,
	})
}

func (s *Server) handleListPlaces(w http.ResponseWriter, r *http.Request) {
	places, factor, err := AssemblePlacesFuer(s.DB, s.now(), s.zugriff(r))
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	// „Bin ich hier angemeldet?" hängt an der abrufenden Person und kommt
	// deshalb erst hier dazu.
	u, _ := auth.FromContext(r.Context())
	if err := markiereEigeneAnmeldungen(s.DB, places, u.Sub); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"places": places, "wateringFactor": factor})
}

// PlaceInput ist der Eingabe-Datensatz für Orte (REST und MCP).
type PlaceInput struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Kind        string  `json:"kind"`
	Lat         float64 `json:"lat"`
	Lon         float64 `json:"lon"`
	Active      *bool   `json:"active"`
	// TraegerID: der Verein bzw. die Gruppe, der der Ort gehört. Beim
	// Anlegen Pflicht (ohne Träger gehört ein Ort niemandem); beim Ändern
	// bedeutet 0 „unverändert lassen“.
	TraegerID int64 `json:"traegerId"`
}

func (in *PlaceInput) Validate() error {
	if in.Kind == "" {
		in.Kind = string(model.PlaceFlowerbox)
	}
	switch {
	case in.Name == "":
		return errors.New("name fehlt")
	case !model.ValidPlaceKind(model.PlaceKind(in.Kind)):
		return errors.New("kind muss blumenkasten, beet oder sonstiges sein")
	}
	if err := pruefeText("name", in.Name); err != nil {
		return err
	}
	if err := pruefeText("description", in.Description); err != nil {
		return err
	}
	// endlich() fängt auch NaN/Inf ab — die bestehen jede Bereichsprüfung.
	if endlich("lat", in.Lat, -90, 90) != nil || endlich("lon", in.Lon, -180, 180) != nil {
		return errors.New("ungültige Koordinaten")
	}
	return nil
}

func (in *PlaceInput) Apply(p *model.Place) {
	p.Name, p.Description, p.Kind = in.Name, in.Description, model.PlaceKind(in.Kind)
	p.Lat, p.Lon = in.Lat, in.Lon
	if in.Active != nil {
		p.Active = *in.Active
	}
	if in.TraegerID != 0 {
		p.TraegerID = in.TraegerID
	}
}

func (s *Server) handleCreatePlace(w http.ResponseWriter, r *http.Request) {
	var in PlaceInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if err := in.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Ein Ort gehört immer einem Träger. Ohne Angabe nimmt er den einzigen,
	// den der Aufrufer verwaltet — im Alltag ist das der Normalfall und
	// erspart der App eine Auswahl.
	traeger, err := s.zielTraeger(r, in.TraegerID)
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	p := model.Place{Active: true, CreatedAt: s.now(), TraegerID: traeger.ID}
	in.Apply(&p)
	if err := s.DB.InsertPlace(&p); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleUpdatePlace(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	existing, err := s.DB.GetPlace(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Ort nicht gefunden")
		return
	}
	if _, err := s.darfOrtVerwalten(r, *existing); err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	var in PlaceInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if err := in.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// Ein Ort lässt sich nur dorthin verschieben, wo man ebenfalls verwaltet
	// — sonst könnte ein Verein dem anderen Arbeit unterschieben.
	if in.TraegerID != 0 && in.TraegerID != existing.TraegerID {
		if _, err := s.zielTraeger(r, in.TraegerID); err != nil {
			schreibeZugriffsfehler(w, r, err)
			return
		}
	}
	vorher := *existing
	in.Apply(existing)
	if err := s.DB.UpdatePlace(existing); err != nil {
		writeInternal(w, r, err)
		return
	}
	// Stillgelegt heißt: Hier ist bis auf Weiteres nichts zu tun. Wer für
	// eine Aufgabe dieses Ortes zugesagt hat, erfährt das.
	if OrtWirdPausiert(vorher, *existing) {
		OrtEntfaellt(s.DB, s.now(), s.Zusteller, existing.ID)
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeletePlace(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	vorhanden, err := s.DB.GetPlace(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Ort nicht gefunden")
		return
	}
	if _, err := s.darfOrtVerwalten(r, *vorhanden); err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	// Erst Bescheid sagen, dann löschen: Danach ist der Vorgang mitsamt
	// seinem Anlass verschwunden.
	OrtEntfaellt(s.DB, s.now(), s.Zusteller, id)
	if err := s.DB.DeletePlace(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "Ort nicht gefunden")
			return
		}
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TaskInput ist der Eingabe-Datensatz für Pflegeaufgaben (REST und MCP).
//
// Eine Aufgabe ist entweder regelmäßig (Intervall und Rot-Schwelle) oder
// einmalig (oneOff mit einem Termin). Beides zusammen gibt es nicht — sonst
// wäre nie klar, woraus sich die Ampel ergibt.
type TaskInput struct {
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Liters       *float64 `json:"liters"`
	IntervalDays float64  `json:"intervalDays"`
	RedAfterDays float64  `json:"redAfterDays"`
	// OneOff: einmalige Aufgabe statt eines wiederkehrenden Plans.
	OneOff bool `json:"oneOff"`
	// DueDate ist der Termin einer einmaligen Aufgabe. Erlaubt sind ein
	// reines Datum („2026-08-20", zählt bis zum Tagesende in Ortszeit) und
	// ein vollständiger Zeitpunkt nach RFC3339.
	DueDate string `json:"dueDate"`
	// RemoveWhenDone: nach dem Erledigen von Karte und Liste nehmen.
	RemoveWhenDone bool  `json:"removeWhenDone"`
	Active         *bool `json:"active"`
	// Sichtbarkeit: „oeffentlich“ oder „nur_mitglieder“.
	//
	// Fehlt das Feld, bleibt die Sichtbarkeit unverändert (beim Anlegen gilt
	// „oeffentlich“). Das ist wichtig: Eine ältere App-Version schickt es
	// nicht mit, und eine interne Aufgabe darf nicht dadurch öffentlich
	// werden, dass jemand ihr Gießintervall ändert.
	Sichtbarkeit string `json:"sichtbarkeit"`
	// BefaehigungID: verlangte Einweisung. Sie muss demselben Träger gehören
	// wie die Aufgabe. Aus demselben Grund ein Zeiger: fehlt das Feld, bleibt
	// die Einweisung, wie sie ist — ausdrückliche 0 nimmt sie weg.
	BefaehigungID *int64 `json:"befaehigungId"`

	// termin ist das geprüfte DueDate. Validate() setzt es, Apply() nutzt es.
	termin *time.Time
}

func (in *TaskInput) Validate() error {
	if !model.ValidTaskKind(model.TaskKind(in.Kind)) {
		return errors.New("kind muss giessen, jaeten oder sonstiges sein")
	}
	// Leer heißt „unverändert“ und wird deshalb NICHT vorbelegt — sonst
	// setzte jede Änderung durch einen älteren Client die Sichtbarkeit
	// zurück. Beim Anlegen ergänzt die Datenbank „oeffentlich“.
	if in.Sichtbarkeit != "" && !model.ValidTaskSichtbarkeit(model.TaskSichtbarkeit(in.Sichtbarkeit)) {
		return errors.New("sichtbarkeit muss oeffentlich oder nur_mitglieder sein")
	}
	if err := pruefeText("title", in.Title); err != nil {
		return err
	}
	if in.Liters != nil {
		if endlich("liters", *in.Liters, 0, MaxLiter) != nil || *in.Liters <= 0 {
			return errors.New("liters muss eine Zahl > 0 sein")
		}
	}
	if in.OneOff {
		termin, err := ParseTermin(in.DueDate)
		if err != nil {
			return err
		}
		in.termin = &termin
		// Intervalle spielen bei einem Termin keine Rolle; sie werden
		// bewusst genullt, damit nirgends zwei Wahrheiten stehen.
		in.IntervalDays, in.RedAfterDays = 0, 0
		return nil
	}
	if in.DueDate != "" {
		return errors.New("dueDate gibt es nur bei einmaligen Aufgaben (oneOff)")
	}
	in.termin = nil
	// endlich() fängt auch NaN/Inf ab — die bestehen jede Bereichsprüfung.
	if endlich("intervalDays", in.IntervalDays, 0, MaxTage) != nil || in.IntervalDays <= 0 {
		return errors.New("intervalDays muss eine Zahl > 0 und <= " + itoa(MaxTage) + " sein")
	}
	if endlich("redAfterDays", in.RedAfterDays, 0, MaxTage) != nil {
		return errors.New("redAfterDays muss eine Zahl > 0 und <= " + itoa(MaxTage) + " sein")
	}
	if in.RedAfterDays < in.IntervalDays {
		return errors.New("redAfterDays muss >= intervalDays sein")
	}
	return nil
}

// ParseTermin liest das Fälligkeitsdatum einer einmaligen Aufgabe.
//
// Ein reines Datum meint den ganzen Tag: „bis zum 20." ist am 20. um 22 Uhr
// noch nicht überfällig. Maßgeblich ist die Ortszeit des Dorfes — der Server
// läuft in UTC, den Termin hat aber jemand in Rössing im Kopf.
func ParseTermin(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("dueDate fehlt: eine einmalige Aufgabe braucht ein Fälligkeitsdatum")
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		ende := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 0, model.Location())
		return ende, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, errors.New("dueDate muss ein Datum (2026-08-20) oder ein Zeitpunkt nach RFC3339 sein")
	}
	if t.Year() < 2000 || t.Year() > 2200 {
		return time.Time{}, errors.New("dueDate liegt außerhalb des zulässigen Bereichs")
	}
	return t, nil
}

func (in *TaskInput) Apply(t *model.CareTask) {
	// Nur übernehmen, was auch geschickt wurde — siehe TaskInput.
	if in.Sichtbarkeit != "" {
		t.Sichtbarkeit = model.TaskSichtbarkeit(in.Sichtbarkeit)
	}
	if in.BefaehigungID != nil {
		t.BefaehigungID = *in.BefaehigungID
	}
	t.Kind, t.Title = model.TaskKind(in.Kind), in.Title
	t.Liters = in.Liters
	t.IntervalDays, t.RedAfterDays = in.IntervalDays, in.RedAfterDays
	t.OneOff, t.DueDate, t.RemoveWhenDone = in.OneOff, in.termin, in.RemoveWhenDone
	if in.Active != nil {
		t.Active = *in.Active
	}
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	placeID, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	place, err := s.DB.GetPlace(placeID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Ort nicht gefunden")
		return
	}
	if _, err := s.darfOrtVerwalten(r, *place); err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	var in TaskInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if err := in.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.pruefeBefaehigungGehoert(wertOder(in.BefaehigungID, 0), place.TraegerID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	t := model.CareTask{PlaceID: placeID, Active: true, CreatedAt: s.now()}
	in.Apply(&t)
	if err := s.DB.InsertTask(&t); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	existing, err := s.DB.GetTask(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Aufgabe nicht gefunden")
		return
	}
	place, err := s.DB.GetPlace(existing.PlaceID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Ort nicht gefunden")
		return
	}
	if _, err := s.darfOrtVerwalten(r, *place); err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	var in TaskInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if err := in.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.pruefeBefaehigungGehoert(wertOder(in.BefaehigungID, existing.BefaehigungID),
		place.TraegerID); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	vorher := *existing
	in.Apply(existing)
	if err := s.DB.UpdateTask(existing); err != nil {
		writeInternal(w, r, err)
		return
	}
	if WirdPausiert(vorher, *existing) {
		AufgabeEntfaellt(s.DB, s.now(), s.Zusteller, existing.ID)
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	if vorhanden, err := s.DB.GetTask(id); err == nil {
		place, perr := s.DB.GetPlace(vorhanden.PlaceID)
		if perr != nil {
			writeErr(w, http.StatusNotFound, "Ort nicht gefunden")
			return
		}
		if _, aerr := s.darfOrtVerwalten(r, *place); aerr != nil {
			schreibeZugriffsfehler(w, r, aerr)
			return
		}
	}
	AufgabeEntfaellt(s.DB, s.now(), s.Zusteller, id)
	if err := s.DB.DeleteTask(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "Aufgabe nicht gefunden")
			return
		}
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListCompletions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	if err := s.pruefeAufgabeSichtbar(r, id); err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	cs, err := s.DB.ListCompletions(id, limit)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	namen, err := s.DB.NameResolver()
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	for i := range cs {
		cs[i].UserName = namen.Resolve(cs[i].UserSub, cs[i].UserName)
	}
	writeJSON(w, http.StatusOK, map[string]any{"completions": cs})
}

func (s *Server) handleCreateCompletion(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	// Was man nicht sehen darf, kann man auch nicht melden.
	if err := s.pruefeAufgabeSichtbar(r, id); err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	// Ob es die Aufgabe gibt, prüft CreateCompletion mit (404).
	var in CompletionInput
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "ungültiges JSON")
			return
		}
	}
	u, _ := auth.FromContext(r.Context())
	c, err := CreateCompletion(s.DB, s.now(), id, in, u)
	if err != nil {
		var ce *CompletionError
		if errors.As(err, &ce) {
			antwort := map[string]any{"error": ce.Message}
			if ce.RetryAfter != nil {
				antwort["retryAfter"] = ce.RetryAfter.UTC().Format(time.RFC3339)
			}
			writeJSON(w, ce.Status, antwort)
			return
		}
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

// handleDeleteCompletion nimmt eine irrtümliche Meldung zurück. Erlaubt ist
// das dem Melder selbst und Admins; die Ampel rechnet sich danach von allein
// neu, weil sie immer aus der letzten vorhandenen Erledigung folgt.
func (s *Server) handleDeleteCompletion(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	c, err := s.DB.GetCompletion(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Meldung nicht gefunden")
		return
	}
	// Zu einer Aufgabe, die es für mich nicht gibt, gibt es auch keine
	// Meldung — sonst unterschiede 403 von 404 und verriete ihre Existenz.
	if err := s.pruefeAufgabeSichtbar(r, c.TaskID); err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	u, _ := auth.FromContext(r.Context())
	if c.UserSub != u.Sub && !u.IsAdmin() {
		writeErr(w, http.StatusForbidden, "nur eigene Meldungen können zurückgenommen werden")
		return
	}
	if err := s.DB.DeleteCompletion(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "Meldung nicht gefunden")
			return
		}
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	factor, err := s.DB.WateringFactor()
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	regeln, err := s.DB.AssignmentRules()
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"wateringFactor": factor,
		"assignment":     assignmentSettingsVon(regeln),
	})
}

// handlePutSettings ändert Hitzefaktor und/oder die Vergabe-Einstellungen.
// Beide Blöcke sind einzeln zu schicken — wer nur den Hitzefaktor setzt,
// lässt die Vergabe unberührt (und umgekehrt).
func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		WateringFactor *float64            `json:"wateringFactor"`
		Assignment     *AssignmentSettings `json:"assignment"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if in.WateringFactor == nil && in.Assignment == nil {
		writeErr(w, http.StatusBadRequest, "es wurde nichts zum Ändern geschickt")
		return
	}
	if in.WateringFactor != nil {
		if *in.WateringFactor <= 0 || *in.WateringFactor > 4 {
			writeErr(w, http.StatusBadRequest, "wateringFactor muss zwischen 0 und 4 liegen")
			return
		}
		if err := s.DB.SetWateringFactor(*in.WateringFactor); err != nil {
			writeInternal(w, r, err)
			return
		}
	}
	if in.Assignment != nil {
		regeln := in.Assignment.Rules()
		if err := regeln.Validate(); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.DB.SetAssignmentRules(regeln); err != nil {
			writeInternal(w, r, err)
			return
		}
	}
	s.handleGetSettings(w, r)
}
