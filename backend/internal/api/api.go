// Package api stellt die REST-API der Dorf-App bereit.
package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

type Server struct {
	DB  *db.DB
	Now func() time.Time
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
	api.HandleFunc("GET /api/v1/places", s.handleListPlaces)
	api.HandleFunc("POST /api/v1/places", s.adminOnly(s.handleCreatePlace))
	api.HandleFunc("PUT /api/v1/places/{id}", s.adminOnly(s.handleUpdatePlace))
	api.HandleFunc("DELETE /api/v1/places/{id}", s.adminOnly(s.handleDeletePlace))
	api.HandleFunc("POST /api/v1/places/{id}/tasks", s.adminOnly(s.handleCreateTask))
	api.HandleFunc("PUT /api/v1/tasks/{id}", s.adminOnly(s.handleUpdateTask))
	api.HandleFunc("DELETE /api/v1/tasks/{id}", s.adminOnly(s.handleDeleteTask))
	api.HandleFunc("GET /api/v1/tasks/{id}/completions", s.handleListCompletions)
	api.HandleFunc("POST /api/v1/tasks/{id}/completions", s.handleCreateCompletion)
	api.HandleFunc("GET /api/v1/settings", s.handleGetSettings)
	api.HandleFunc("PUT /api/v1/settings", s.adminOnly(s.handlePutSettings))

	mux.Handle("/api/v1/", authMW(api))
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

// AssemblePlaces baut die Orts-Liste mit Aufgaben und Ampel-Status.
// Wird von REST-API und MCP-Server gemeinsam genutzt.
func AssemblePlaces(d *db.DB, now time.Time) ([]model.PlaceWithStatus, float64, error) {
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

	byPlace := map[int64][]model.TaskWithStatus{}
	for _, t := range tasks {
		var lc *model.Completion
		if c, ok := last[t.ID]; ok {
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
		})
	}

	out := make([]model.PlaceWithStatus, 0, len(places))
	for _, p := range places {
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
	writeJSON(w, http.StatusOK, map[string]any{
		"sub": u.Sub, "name": u.Name, "email": u.Email, "roles": roles, "isAdmin": u.IsAdmin(),
	})
}

func (s *Server) handleListPlaces(w http.ResponseWriter, _ *http.Request) {
	places, factor, err := AssemblePlaces(s.DB, s.now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
	case in.Lat < -90 || in.Lat > 90 || in.Lon < -180 || in.Lon > 180:
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
	p := model.Place{Active: true, CreatedAt: s.now()}
	in.Apply(&p)
	if err := s.DB.InsertPlace(&p); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
	var in PlaceInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if err := in.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	in.Apply(existing)
	if err := s.DB.UpdatePlace(existing); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeletePlace(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	if err := s.DB.DeletePlace(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "Ort nicht gefunden")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// TaskInput ist der Eingabe-Datensatz für Pflegeaufgaben (REST und MCP).
type TaskInput struct {
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Liters       *float64 `json:"liters"`
	IntervalDays float64  `json:"intervalDays"`
	RedAfterDays float64  `json:"redAfterDays"`
	Active       *bool    `json:"active"`
}

func (in *TaskInput) Validate() error {
	switch {
	case !model.ValidTaskKind(model.TaskKind(in.Kind)):
		return errors.New("kind muss giessen, jaeten oder sonstiges sein")
	case in.IntervalDays <= 0:
		return errors.New("intervalDays muss > 0 sein")
	case in.RedAfterDays < in.IntervalDays:
		return errors.New("redAfterDays muss >= intervalDays sein")
	case in.Liters != nil && *in.Liters <= 0:
		return errors.New("liters muss > 0 sein")
	}
	return nil
}

func (in *TaskInput) Apply(t *model.CareTask) {
	t.Kind, t.Title = model.TaskKind(in.Kind), in.Title
	t.Liters = in.Liters
	t.IntervalDays, t.RedAfterDays = in.IntervalDays, in.RedAfterDays
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
	if _, err := s.DB.GetPlace(placeID); err != nil {
		writeErr(w, http.StatusNotFound, "Ort nicht gefunden")
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
	t := model.CareTask{PlaceID: placeID, Active: true, CreatedAt: s.now()}
	in.Apply(&t)
	if err := s.DB.InsertTask(&t); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
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
	var in TaskInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if err := in.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	in.Apply(existing)
	if err := s.DB.UpdateTask(existing); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	if err := s.DB.DeleteTask(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "Aufgabe nicht gefunden")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
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
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	cs, err := s.DB.ListCompletions(id, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"completions": cs})
}

func (s *Server) handleCreateCompletion(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	task, err := s.DB.GetTask(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Aufgabe nicht gefunden")
		return
	}
	var in struct {
		Liters *float64 `json:"liters"`
		Note   string   `json:"note"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "ungültiges JSON")
			return
		}
	}
	u, _ := auth.FromContext(r.Context())
	c := model.Completion{TaskID: task.ID, UserSub: u.Sub, UserName: u.Name,
		Liters: in.Liters, Note: in.Note, DoneAt: s.now()}
	if err := s.DB.InsertCompletion(&c); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *Server) handleGetSettings(w http.ResponseWriter, _ *http.Request) {
	factor, err := s.DB.WateringFactor()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"wateringFactor": factor})
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	var in struct {
		WateringFactor float64 `json:"wateringFactor"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if in.WateringFactor <= 0 || in.WateringFactor > 4 {
		writeErr(w, http.StatusBadRequest, "wateringFactor muss zwischen 0 und 4 liegen")
		return
	}
	if err := s.DB.SetWateringFactor(in.WateringFactor); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"wateringFactor": in.WateringFactor})
}
