package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Tools for the error reports: go through what broke on the phones in the
// village from inside Claude, without opening the administration in a
// browser. As everywhere at the MCP endpoint this needs the admin role (see
// auth.go).
//
// The tool names stay German like the existing ones — they are what an
// administrator reads in Claude, not an internal identifier.

func (s *Server) errorReportTools() []tool {
	staende := api.ErrorReportStatusNames()
	arten := api.ErrorReportKindNames()
	return []tool{
		{
			Name: "fehlerberichte_liste",
			Description: "Listet die Fehlerberichte aus den Apps (Android und iOS) mit Zeitpunkt, " +
				"Art (" + strings.Join(arten, ", ") + "), der Meldung, die die Person gelesen hat, " +
				"ihrer freiwilligen Ergänzung, den technischen Angaben, Gerät, App-Version, " +
				"Stand und interner Notiz. Optional nach Stand (" + strings.Join(staende, ", ") + ") " +
				"und nach Art gefiltert. Neueste zuerst. Mitgeliefert wird ein Überblick über " +
				"den GANZEN Bestand (gesamt, offen, je Art, je Plattform, je App-Version, " +
				"neuester und ältester Bericht) — auch bei gefilterter Liste, damit " +
				"„was ist kaputt?“ in einem Zug zu beantworten ist.",
			Schema: obj(nil, map[string]any{
				"status": enum("Nur Berichte mit diesem Stand (Standard: alle)", staende...),
				"art":    enum("Nur Berichte dieser Art (Standard: alle)", arten...),
				"limit":  integer("Höchstens so viele Einträge ausgeben (Standard: alle)"),
			}),
			Handler: s.toolErrorReportList,
		},
		{
			Name: "fehlerbericht_status_setzen",
			Description: "Setzt den Stand eines Fehlerberichts (" + strings.Join(staende, ", ") + ") " +
				"und optional die interne Notiz. Die IDs stehen in fehlerberichte_liste. " +
				"Der gemeldete Sachverhalt selbst bleibt unverändert — er ist das, was ein " +
				"Gerät beobachtet hat.",
			Schema: obj([]string{"id", "status"}, map[string]any{
				"id":     integer("ID des Fehlerberichts"),
				"status": enum("Neuer Stand", staende...),
				"notiz":  str("Interne Bemerkung (ersetzt die bisherige)"),
			}),
			Handler: s.toolErrorReportSetStatus,
		},
	}
}

func (s *Server) toolErrorReportList(args json.RawMessage, _ auth.User) (any, error) {
	var in struct {
		Status string `json:"status"`
		Art    string `json:"art"`
		Limit  int    `json:"limit"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	status, err := api.ErrorReportStatusFrom(in.Status)
	if err != nil {
		return nil, err
	}
	art, err := api.ErrorReportKindFrom(in.Art)
	if err != nil {
		return nil, err
	}
	berichte, err := s.DB.ListErrorReports(status, art)
	if err != nil {
		return nil, err
	}
	if in.Limit > 0 && in.Limit < len(berichte) {
		berichte = berichte[:in.Limit]
	}
	ueberblick, err := s.errorReportOverview()
	if err != nil {
		return nil, err
	}
	return map[string]any{"fehlerberichte": berichte, "ueberblick": ueberblick}, nil
}

// errorReportOverview answers „was ist kaputt?“ in one go: how much in total,
// how much per kind, platform and app version, and when the last one came in.
// It deliberately always describes the whole stock — with a filtered list
// „wie viel ist noch offen?“ could otherwise not be answered.
func (s *Server) errorReportOverview() (map[string]any, error) {
	alle, err := s.DB.ListErrorReports("", "")
	if err != nil {
		return nil, err
	}
	jeStand := map[string]int{}
	for _, st := range model.ErrorReportStatuses {
		jeStand[string(st)] = 0
	}
	jeArt := map[string]int{}
	for _, k := range model.ErrorReportKinds {
		jeArt[string(k)] = 0
	}
	jePlattform := map[string]int{}
	for _, p := range api.ErrorReportPlatforms {
		jePlattform[p] = 0
	}
	jeVersion := map[string]int{}
	var neuester, aeltester string
	for _, e := range alle {
		jeStand[string(e.Status)]++
		jeArt[string(e.Kind)]++
		jePlattform[e.Platform]++
		if e.AppVersion != "" {
			jeVersion[e.AppVersion]++
		}
		wann := e.OccurredAt.UTC().Format(time.RFC3339)
		if neuester == "" || wann > neuester {
			neuester = wann
		}
		if aeltester == "" || wann < aeltester {
			aeltester = wann
		}
	}
	return map[string]any{
		"gesamt":      len(alle),
		"offen":       jeStand[string(model.ErrorReportNew)],
		"jeStand":     jeStand,
		"jeArt":       jeArt,
		"jePlattform": jePlattform,
		"jeVersion":   jeVersion,
		"neuester":    neuester,
		"aeltester":   aeltester,
	}, nil
}

func (s *Server) toolErrorReportSetStatus(args json.RawMessage, _ auth.User) (any, error) {
	var in struct {
		ID     int64   `json:"id"`
		Status string  `json:"status"`
		Notiz  *string `json:"notiz"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	status, err := api.ErrorReportStatusFrom(in.Status)
	if err != nil {
		return nil, err
	}
	if status == "" {
		return nil, fmt.Errorf("status fehlt")
	}
	bericht, err := s.DB.GetErrorReport(in.ID)
	if err != nil {
		return nil, fmt.Errorf("Fehlerbericht %d nicht gefunden", in.ID)
	}
	bericht.Status = status
	if in.Notiz != nil {
		bericht.Note = strings.TrimSpace(*in.Notiz)
	}
	if err := s.DB.UpdateErrorReport(bericht); err != nil {
		return nil, err
	}
	return bericht, nil
}
