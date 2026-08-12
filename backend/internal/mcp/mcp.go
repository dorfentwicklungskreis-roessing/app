// Package mcp implementiert einen minimalen MCP-Server (Streamable HTTP,
// JSON-Antworten) für die Admin-Verwaltung der Dorf-App aus Claude heraus.
//
// Unterstützt werden initialize, ping, tools/list und tools/call — das ist
// alles, was Claude (claude.ai Connectors und Claude Code) zum Arbeiten
// braucht. Auth: Bearer-Token oder Token als Pfadsegment (/mcp/<token>).
package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

const protocolVersion = "2025-03-26"

type Server struct {
	DB    *db.DB
	Token string
	Now   func() time.Time
	tools []tool
}

type tool struct {
	Name        string
	Description string
	Schema      map[string]any
	Handler     func(args json.RawMessage) (any, error)
}

func New(database *db.DB, token string) *Server {
	s := &Server{DB: database, Token: token}
	s.registerTools()
	return s
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Register hängt den MCP-Endpoint an den Mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /mcp", s.withAuth(s.handlePost, false))
	mux.HandleFunc("POST /mcp/{token}", s.withAuth(s.handlePost, true))
	// GET (SSE-Stream) wird nicht unterstützt — sauber ablehnen.
	reject := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
	mux.HandleFunc("GET /mcp", reject)
	mux.HandleFunc("GET /mcp/{token}", reject)
	mux.HandleFunc("DELETE /mcp", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func (s *Server) withAuth(h http.HandlerFunc, fromPath bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Token == "" {
			http.Error(w, "MCP ist nicht konfiguriert (MCP_TOKEN fehlt)", http.StatusServiceUnavailable)
			return
		}
		got := ""
		if fromPath {
			got = r.PathValue("token")
		} else if ah := r.Header.Get("Authorization"); len(ah) > 7 && ah[:7] == "Bearer " {
			got = ah[7:]
		}
		if got != s.Token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h(w, r)
	}
}

// --- JSON-RPC ---------------------------------------------------------------

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRPC(w, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error"}})
		return
	}
	// Notifications (ohne ID) nur bestätigen.
	if len(req.ID) == 0 || string(req.ID) == "null" {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	resp := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := p.ProtocolVersion
		if v == "" {
			v = protocolVersion
		}
		resp.Result = map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "dorf-app", "version": "0.1.0"},
			"instructions": "Verwaltung der Dorfpflege in Rössing: Orte (Blumenkästen, Beete) " +
				"mit Pflegeaufgaben (Gießen, Jäten) und Ampel-Status (green/yellow/red).",
		}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/list":
		resp.Result = s.toolsList()
	case "tools/call":
		resp.Result = s.toolsCall(req.Params)
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
	writeRPC(w, resp)
}

func writeRPC(w http.ResponseWriter, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) toolsList() map[string]any {
	list := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		list = append(list, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": t.Schema,
		})
	}
	return map[string]any{"tools": list}
}

func (s *Server) toolsCall(params json.RawMessage) map[string]any {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return toolError("ungültige Parameter")
	}
	for _, t := range s.tools {
		if t.Name != p.Name {
			continue
		}
		result, err := t.Handler(p.Arguments)
		if err != nil {
			return toolError(err.Error())
		}
		text, _ := json.MarshalIndent(result, "", "  ")
		return map[string]any{
			"content": []map[string]any{{"type": "text", "text": string(text)}},
			"isError": false,
		}
	}
	return toolError("unbekanntes Tool: " + p.Name)
}

func toolError(msg string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": "Fehler: " + msg}},
		"isError": true,
	}
}

// --- Schema-Helfer ----------------------------------------------------------

func obj(required []string, props map[string]any) map[string]any {
	m := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		m["required"] = required
	}
	return m
}

func str(desc string) map[string]any { return map[string]any{"type": "string", "description": desc} }
func num(desc string) map[string]any { return map[string]any{"type": "number", "description": desc} }
func integer(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}
func boolean(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

func enum(desc string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": desc, "enum": values}
}

// --- Tools ------------------------------------------------------------------

func (s *Server) registerTools() {
	s.tools = []tool{
		{
			Name: "orte_liste",
			Description: "Listet alle Pflege-Orte (Blumenkästen, Beete, …) mit ihren Aufgaben, " +
				"letzter Erledigung und Ampel-Status (green/yellow/red).",
			Schema:  obj(nil, map[string]any{}),
			Handler: s.toolListPlaces,
		},
		{
			Name: "ort_anlegen",
			Description: "Legt einen neuen Pflege-Ort an (z.B. Blumenkasten oder Beet). " +
				"Koordinaten in WGS84 (Rössing liegt bei ca. lat 52.211, lon 9.870). " +
				"Aufgaben (Gießplan etc.) danach mit aufgabe_anlegen ergänzen.",
			Schema: obj([]string{"name", "lat", "lon"}, map[string]any{
				"name":        str("Anzeigename, z.B. 'Unter den Eichen — Kasten 1'"),
				"kind":        enum("Art des Ortes (Standard: blumenkasten)", "blumenkasten", "beet", "sonstiges"),
				"lat":         num("Breitengrad"),
				"lon":         num("Längengrad"),
				"description": str("Optionale Beschreibung"),
			}),
			Handler: s.toolCreatePlace,
		},
		{
			Name:        "ort_aendern",
			Description: "Ändert Felder eines Ortes. Nur angegebene Felder werden geändert.",
			Schema: obj([]string{"id"}, map[string]any{
				"id":          integer("ID des Ortes"),
				"name":        str("Neuer Name"),
				"kind":        enum("Art des Ortes", "blumenkasten", "beet", "sonstiges"),
				"lat":         num("Breitengrad"),
				"lon":         num("Längengrad"),
				"description": str("Beschreibung"),
				"active":      boolean("Ort aktiv? Inaktive Orte erzeugen keine Erinnerungen."),
			}),
			Handler: s.toolUpdatePlace,
		},
		{
			Name:        "ort_loeschen",
			Description: "Löscht einen Ort samt Aufgaben und Historie. Nicht umkehrbar!",
			Schema:      obj([]string{"id"}, map[string]any{"id": integer("ID des Ortes")}),
			Handler:     s.toolDeletePlace,
		},
		{
			Name: "aufgabe_anlegen",
			Description: "Legt eine Pflegeaufgabe für einen Ort an, z.B. Gießplan " +
				"(kind=giessen, liters=10, intervalDays=7, redAfterDays=14) oder Jäten " +
				"(kind=jaeten, intervalDays=21, redAfterDays=35).",
			Schema: obj([]string{"placeId", "kind", "intervalDays", "redAfterDays"}, map[string]any{
				"placeId":      integer("ID des Ortes"),
				"kind":         enum("Art der Aufgabe", "giessen", "jaeten", "sonstiges"),
				"title":        str("Optionaler Titel (v.a. für kind=sonstiges)"),
				"liters":       num("Wassermenge pro Gießvorgang in Litern (nur giessen)"),
				"intervalDays": num("Soll-Intervall in Tagen; danach wird die Aufgabe gelb"),
				"redAfterDays": num("Nach so vielen Tagen ohne Erledigung wird sie rot"),
			}),
			Handler: s.toolCreateTask,
		},
		{
			Name:        "aufgabe_aendern",
			Description: "Ändert eine Pflegeaufgabe. Nur angegebene Felder werden geändert.",
			Schema: obj([]string{"id"}, map[string]any{
				"id":           integer("ID der Aufgabe"),
				"kind":         enum("Art der Aufgabe", "giessen", "jaeten", "sonstiges"),
				"title":        str("Titel"),
				"liters":       num("Wassermenge in Litern"),
				"intervalDays": num("Soll-Intervall in Tagen"),
				"redAfterDays": num("Rot-Schwelle in Tagen"),
				"active":       boolean("Aufgabe aktiv?"),
			}),
			Handler: s.toolUpdateTask,
		},
		{
			Name:        "aufgabe_loeschen",
			Description: "Löscht eine Pflegeaufgabe samt Historie. Nicht umkehrbar!",
			Schema:      obj([]string{"id"}, map[string]any{"id": integer("ID der Aufgabe")}),
			Handler:     s.toolDeleteTask,
		},
		{
			Name: "erledigung_melden",
			Description: "Meldet eine Aufgabe als erledigt (gegossen/gejätet), " +
				"z.B. wenn jemand telefonisch Vollzug meldet.",
			Schema: obj([]string{"taskId"}, map[string]any{
				"taskId": integer("ID der Aufgabe"),
				"name":   str("Wer hat's gemacht? (Standard: 'Admin via Claude')"),
				"liters": num("Tatsächlich gegossene Liter (optional)"),
				"note":   str("Optionale Notiz"),
			}),
			Handler: s.toolCreateCompletion,
		},
		{
			Name: "hitzefaktor_setzen",
			Description: "Setzt den globalen Hitze-Faktor für Gieß-Aufgaben. 1.0 = normal, " +
				"0.5 = Hitzewelle (Kästen werden doppelt so schnell gelb/rot). Bereich 0–4.",
			Schema:  obj([]string{"factor"}, map[string]any{"factor": num("Faktor, z.B. 0.5 bei Hitze")}),
			Handler: s.toolSetFactor,
		},
	}
}

func (s *Server) toolListPlaces(json.RawMessage) (any, error) {
	places, factor, err := api.AssemblePlaces(s.DB, s.now())
	if err != nil {
		return nil, err
	}
	return map[string]any{"places": places, "wateringFactor": factor}, nil
}

func (s *Server) toolCreatePlace(args json.RawMessage) (any, error) {
	var in api.PlaceInput
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}
	p := model.Place{Active: true, CreatedAt: s.now()}
	in.Apply(&p)
	if err := s.DB.InsertPlace(&p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Server) toolUpdatePlace(args json.RawMessage) (any, error) {
	var in struct {
		ID          int64    `json:"id"`
		Name        *string  `json:"name"`
		Kind        *string  `json:"kind"`
		Lat         *float64 `json:"lat"`
		Lon         *float64 `json:"lon"`
		Description *string  `json:"description"`
		Active      *bool    `json:"active"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	p, err := s.DB.GetPlace(in.ID)
	if err != nil {
		return nil, fmt.Errorf("Ort %d nicht gefunden", in.ID)
	}
	applyIf(&p.Name, in.Name)
	applyIf(&p.Description, in.Description)
	if in.Kind != nil {
		if !model.ValidPlaceKind(model.PlaceKind(*in.Kind)) {
			return nil, fmt.Errorf("ungültige Art: %s", *in.Kind)
		}
		p.Kind = model.PlaceKind(*in.Kind)
	}
	applyIf(&p.Lat, in.Lat)
	applyIf(&p.Lon, in.Lon)
	applyIf(&p.Active, in.Active)
	if err := s.DB.UpdatePlace(p); err != nil {
		return nil, err
	}
	return p, nil
}

func (s *Server) toolDeletePlace(args json.RawMessage) (any, error) {
	var in struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if err := s.DB.DeletePlace(in.ID); err != nil {
		return nil, fmt.Errorf("Ort %d nicht gefunden", in.ID)
	}
	return map[string]any{"deleted": in.ID}, nil
}

func (s *Server) toolCreateTask(args json.RawMessage) (any, error) {
	var in struct {
		PlaceID int64 `json:"placeId"`
		api.TaskInput
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if _, err := s.DB.GetPlace(in.PlaceID); err != nil {
		return nil, fmt.Errorf("Ort %d nicht gefunden", in.PlaceID)
	}
	if err := in.Validate(); err != nil {
		return nil, err
	}
	t := model.CareTask{PlaceID: in.PlaceID, Active: true, CreatedAt: s.now()}
	in.Apply(&t)
	if err := s.DB.InsertTask(&t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Server) toolUpdateTask(args json.RawMessage) (any, error) {
	var in struct {
		ID           int64    `json:"id"`
		Kind         *string  `json:"kind"`
		Title        *string  `json:"title"`
		Liters       *float64 `json:"liters"`
		IntervalDays *float64 `json:"intervalDays"`
		RedAfterDays *float64 `json:"redAfterDays"`
		Active       *bool    `json:"active"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	t, err := s.DB.GetTask(in.ID)
	if err != nil {
		return nil, fmt.Errorf("Aufgabe %d nicht gefunden", in.ID)
	}
	if in.Kind != nil {
		if !model.ValidTaskKind(model.TaskKind(*in.Kind)) {
			return nil, fmt.Errorf("ungültige Art: %s", *in.Kind)
		}
		t.Kind = model.TaskKind(*in.Kind)
	}
	applyIf(&t.Title, in.Title)
	if in.Liters != nil {
		t.Liters = in.Liters
	}
	applyIf(&t.IntervalDays, in.IntervalDays)
	applyIf(&t.RedAfterDays, in.RedAfterDays)
	applyIf(&t.Active, in.Active)
	if t.IntervalDays <= 0 || t.RedAfterDays < t.IntervalDays {
		return nil, fmt.Errorf("ungültige Intervalle: intervalDays > 0 und redAfterDays >= intervalDays nötig")
	}
	if err := s.DB.UpdateTask(t); err != nil {
		return nil, err
	}
	return t, nil
}

func (s *Server) toolDeleteTask(args json.RawMessage) (any, error) {
	var in struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if err := s.DB.DeleteTask(in.ID); err != nil {
		return nil, fmt.Errorf("Aufgabe %d nicht gefunden", in.ID)
	}
	return map[string]any{"deleted": in.ID}, nil
}

func (s *Server) toolCreateCompletion(args json.RawMessage) (any, error) {
	var in struct {
		TaskID int64    `json:"taskId"`
		Name   string   `json:"name"`
		Liters *float64 `json:"liters"`
		Note   string   `json:"note"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	task, err := s.DB.GetTask(in.TaskID)
	if err != nil {
		return nil, fmt.Errorf("Aufgabe %d nicht gefunden", in.TaskID)
	}
	if in.Name == "" {
		in.Name = "Admin via Claude"
	}
	c := model.Completion{TaskID: task.ID, UserSub: "mcp-admin", UserName: in.Name,
		Liters: in.Liters, Note: in.Note, DoneAt: s.now()}
	if err := s.DB.InsertCompletion(&c); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Server) toolSetFactor(args json.RawMessage) (any, error) {
	var in struct {
		Factor float64 `json:"factor"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if in.Factor <= 0 || in.Factor > 4 {
		return nil, fmt.Errorf("factor muss zwischen 0 und 4 liegen")
	}
	if err := s.DB.SetWateringFactor(in.Factor); err != nil {
		return nil, err
	}
	return map[string]any{"wateringFactor": in.Factor}, nil
}

func applyIf[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}
