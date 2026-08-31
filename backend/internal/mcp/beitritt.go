package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Beitritte über MCP: sehen, entscheiden, jemanden aufnehmen.
//
// MCP ist der Verwaltungsweg (#62): Was in der Web-Verwaltung geht, muss auch
// hier gehen. Sonst müsste der Betreiber für eine einzige Aufnahme einen
// Browser mit angemeldeter Sitzung suchen — auf dem Telefon ist das eine
// Sackgasse, für eine Maschine gar kein Weg.
//
// Alles Schreibende geht durch api.Aufnehmen bzw. api.BeitrittEntscheiden.
// Dort steht der Schritt, auf den es ankommt: die Rollenzuweisung in der
// Rössing-ID. Ein zweiter Weg daneben würde ihn früher oder später vergessen.

// beitrittAnsicht nennt Träger und Person beim Namen. Wer eine Liste
// vorgelesen bekommt, kann mit Kennungen nichts anfangen — die Kennung steht
// trotzdem dabei, denn beitritt_entscheiden braucht sie.
type beitrittAnsicht struct {
	model.Beitritt
	TraegerName string `json:"traegerName,omitempty"`
}

func (s *Server) toolListBeitritte(args json.RawMessage, _ auth.User) (any, error) {
	var in struct {
		TraegerID int64  `json:"traegerId"`
		Status    string `json:"status"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	status := model.AntragStatus(in.Status)
	if status != "" && !model.ValidAntragStatus(status) {
		return nil, fmt.Errorf("status muss beantragt, erteilt oder abgelehnt sein, nicht %q", in.Status)
	}
	traeger, err := s.DB.TraegerIndex()
	if err != nil {
		return nil, err
	}
	namen, err := s.DB.NameResolver()
	if err != nil {
		return nil, err
	}
	out := []beitrittAnsicht{}
	for id, t := range traeger {
		if in.TraegerID > 0 && id != in.TraegerID {
			continue
		}
		liste, err := s.DB.ListBeitritte(id, status)
		if err != nil {
			return nil, err
		}
		for _, b := range liste {
			b.UserName = namen.Resolve(b.UserSub, "")
			out = append(out, beitrittAnsicht{Beitritt: b, TraegerName: t.Name})
		}
	}
	return out, nil
}

func (s *Server) toolBeitrittEntscheiden(args json.RawMessage, u auth.User) (any, error) {
	if err := nurBetreiber(u, "über Beitritte entscheiden"); err != nil {
		return nil, err
	}
	var in struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
		Notiz  string `json:"notiz"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	b, err := s.DB.GetBeitritt(in.ID)
	if err != nil {
		return nil, fmt.Errorf("Beitrittsantrag %d nicht gefunden", in.ID)
	}
	t, err := s.DB.GetTraeger(b.TraegerID)
	if err != nil {
		return nil, fmt.Errorf("Träger %d nicht gefunden", b.TraegerID)
	}
	status := model.AntragStatus(in.Status)
	if status == "" {
		status = model.AntragErteilt
	}
	// Ein Werkzeug bekommt (noch) keinen Context durchgereicht; der Aufruf
	// nach Zitadel hat seine eigene Frist (siehe internal/mitglied).
	return api.BeitrittEntscheiden(context.Background(), s.DB, s.Mitglieder, *t, *b,
		status, u.Sub, in.Notiz, s.now())
}

// toolMitgliedAufnehmen trägt jemanden ohne vorherigen Antrag ein.
//
// Wie beim Erteilen einer Befähigung fehlt hier der erste Schritt, und aus
// demselben Grund: Zugesagt wird im Dorf am Gartenzaun, nicht in der App. Wer
// eine geschlossene Gruppe verwaltet, hat überhaupt keinen anderen Weg — sie
// nimmt keine Anträge entgegen.
func (s *Server) toolMitgliedAufnehmen(args json.RawMessage, u auth.User) (any, error) {
	if err := nurBetreiber(u, "Mitglieder aufnehmen"); err != nil {
		return nil, err
	}
	var in struct {
		TraegerID int64  `json:"traegerId"`
		UserSub   string `json:"userSub"`
		Notiz     string `json:"notiz"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	t, err := s.DB.GetTraeger(in.TraegerID)
	if err != nil {
		return nil, fmt.Errorf("Träger %d nicht gefunden", in.TraegerID)
	}
	return api.Aufnehmen(context.Background(), s.DB, s.Mitglieder, *t,
		in.UserSub, u.Sub, in.Notiz, s.now())
}

// beitrittTools sind die Werkzeuge dieses Bereichs. Sie stehen hier und nicht
// in der großen Liste, damit der Bereich für sich lesbar bleibt.
func (s *Server) beitrittTools() []tool {
	return []tool{
		{
			Name: "beitritte_liste",
			Description: "Listet Beitrittsanträge („ich will bei euch mitmachen“) samt Stand. " +
				"Ohne Angabe alle Träger und alle Stände.",
			Schema: obj(nil, map[string]any{
				"traegerId": integer("Nur die Anträge an diesen Träger"),
				"status":    enum("Nur Anträge in diesem Stand", "beantragt", "erteilt", "abgelehnt"),
			}),
			Handler: s.toolListBeitritte,
		},
		{
			Name: "beitritt_entscheiden",
			Description: "Gibt einen Beitrittsantrag frei oder lehnt ihn ab. Die Freigabe trägt " +
				"die Rolle 'mitglied' unmittelbar in der Rössing-ID ein — niemand muss dafür " +
				"in die Zitadel-Konsole. Klappt das Eintragen nicht, bleibt der Antrag offen " +
				"und die Meldung sagt, woran es lag. Die IDs stehen in beitritte_liste.",
			Schema: obj([]string{"id"}, map[string]any{
				"id":     integer("ID des Beitrittsantrags"),
				"status": enum("erteilt (Standard) oder abgelehnt", "erteilt", "abgelehnt"),
				"notiz":  str("Notiz zur Entscheidung"),
			}),
			Handler: s.toolBeitrittEntscheiden,
		},
		{
			Name: "mitglied_aufnehmen",
			Description: "Nimmt jemanden in einen Träger auf, auch ohne vorherigen Antrag — " +
				"z.B. wenn auf der Versammlung zugesagt wurde. Bei einer geschlossenen Gruppe " +
				"ist das der einzige Weg: Anträge nimmt sie nicht entgegen. Die Kennung " +
				"(userSub) steht in der Rangliste und in jeder Erledigung.",
			Schema: obj([]string{"traegerId", "userSub"}, map[string]any{
				"traegerId": integer("ID des Trägers"),
				"userSub":   str("Kennung der Person aus der Rössing-ID"),
				"notiz":     str("Notiz, z.B. wann und wo zugesagt wurde"),
			}),
			Handler: s.toolMitgliedAufnehmen,
		},
	}
}
