package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Werkzeuge für die Ideen-Sammlung: die Wünsche aus dem Dorf aus Claude
// heraus durchsehen und einordnen, ohne die Verwaltung im Browser zu öffnen.
// Wie überall am MCP-Endpoint ist dafür die admin-Rolle nötig (siehe auth.go).

func (s *Server) ideenTools() []tool {
	staende := make([]string, 0, len(model.IdeeStatusWerte))
	for _, st := range model.IdeeStatusWerte {
		staende = append(staende, string(st))
	}
	return []tool{
		{
			Name: "ideen_liste",
			Description: "Listet die Ideen aus dem Dorf („Was soll die App können?“) mit Datum, " +
				"Name, E-Mail, Wunsch, Weg (website/app), Stand und interner Notiz. " +
				"Optional nach Stand gefiltert (" + strings.Join(staende, ", ") + "). " +
				"Neueste zuerst.",
			Schema: obj(nil, map[string]any{
				"status": enum("Nur Ideen mit diesem Stand (Standard: alle)", staende...),
				"limit":  integer("Höchstens so viele Einträge ausgeben (Standard: alle)"),
			}),
			Handler: s.toolIdeenListe,
		},
		{
			Name: "idee_status_setzen",
			Description: "Setzt den Stand einer Idee (" + strings.Join(staende, ", ") + ") und " +
				"optional die interne Notiz. Die IDs stehen in ideen_liste. Der " +
				"eingereichte Wunsch selbst bleibt unverändert.",
			Schema: obj([]string{"id", "status"}, map[string]any{
				"id":     integer("ID der Idee"),
				"status": enum("Neuer Stand", staende...),
				"notiz":  str("Interne Bemerkung (ersetzt die bisherige)"),
			}),
			Handler: s.toolIdeeStatusSetzen,
		},
	}
}

func (s *Server) toolIdeenListe(args json.RawMessage, _ auth.User) (any, error) {
	var in struct {
		Status string `json:"status"`
		Limit  int    `json:"limit"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	status, err := api.IdeeStatusAus(in.Status)
	if err != nil {
		return nil, err
	}
	ideen, err := s.DB.ListIdeen(status)
	if err != nil {
		return nil, err
	}
	if in.Limit > 0 && in.Limit < len(ideen) {
		ideen = ideen[:in.Limit]
	}
	anzahl, err := s.DB.CountIdeen()
	if err != nil {
		return nil, err
	}
	return map[string]any{"ideen": ideen, "anzahl": anzahl}, nil
}

func (s *Server) toolIdeeStatusSetzen(args json.RawMessage, _ auth.User) (any, error) {
	var in struct {
		ID     int64   `json:"id"`
		Status string  `json:"status"`
		Notiz  *string `json:"notiz"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	status, err := api.IdeeStatusAus(in.Status)
	if err != nil {
		return nil, err
	}
	if status == "" {
		return nil, fmt.Errorf("status fehlt")
	}
	idee, err := s.DB.GetIdee(in.ID)
	if err != nil {
		return nil, fmt.Errorf("Idee %d nicht gefunden", in.ID)
	}
	idee.Status = status
	if in.Notiz != nil {
		idee.Notiz = strings.TrimSpace(*in.Notiz)
	}
	if err := s.DB.UpdateIdee(idee); err != nil {
		return nil, err
	}
	return idee, nil
}
