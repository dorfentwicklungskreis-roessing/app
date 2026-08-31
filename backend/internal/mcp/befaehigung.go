package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Befähigungen über MCP: anlegen, ändern, löschen, erteilen.
//
// Eine Befähigung ist eine Einweisung, die ein Träger vergibt („Motorsense“,
// „Schlüssel Gerätehaus“, „Einweisung Jäten“). Hängt sie an einer Aufgabe,
// kann nur zusagen, wer sie hat — durchgesetzt wird das serverseitig.
//
// Sie gehört der **Person**, nicht der Aufgabe: Wer einmal an der Motorsense
// eingewiesen wurde, ist es überall. Sonst müsste jede einzelne Wiese neu
// freigegeben werden.
//
// Bis hierher ging das ausschließlich über die Web-Verwaltung. Damit war eine
// Aufgabe „nur mit Einweisung“ über den Connector nicht anzulegen: Das Feld
// `befaehigungId` gab es an `aufgabe_anlegen` zwar, aber keine Möglichkeit,
// die Kennung zu erfahren oder eine Befähigung überhaupt anzulegen.
//
// Wer das darf: dieselbe Antwort wie bei den Trägern — der MCP-Endpunkt
// verlangt für jede Anfrage die admin-Rolle, also den Betreiber.

// befaehigungAnsicht nennt den Träger beim Namen. Wer eine Liste vorgelesen
// bekommt, kann mit einer Kennung nichts anfangen.
type befaehigungAnsicht struct {
	model.Befaehigung
	TraegerName string `json:"traegerName,omitempty"`
}

func (s *Server) toolListBefaehigungen(args json.RawMessage, _ auth.User) (any, error) {
	var in struct {
		TraegerID int64 `json:"traegerId"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	var (
		liste []model.Befaehigung
		err   error
	)
	if in.TraegerID > 0 {
		liste, err = s.DB.ListBefaehigungen(in.TraegerID)
	} else {
		liste, err = s.DB.ListAlleBefaehigungen()
	}
	if err != nil {
		return nil, err
	}
	traeger, err := s.DB.TraegerIndex()
	if err != nil {
		return nil, err
	}
	out := make([]befaehigungAnsicht, 0, len(liste))
	for _, b := range liste {
		out = append(out, befaehigungAnsicht{
			Befaehigung: b,
			TraegerName: traeger[b.TraegerID].Name,
		})
	}
	return out, nil
}

func (s *Server) toolCreateBefaehigung(args json.RawMessage, u auth.User) (any, error) {
	if err := nurBetreiber(u, "Befähigungen anlegen"); err != nil {
		return nil, err
	}
	var in struct {
		TraegerID    int64  `json:"traegerId"`
		Name         string `json:"name"`
		Beschreibung string `json:"beschreibung"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if _, err := s.DB.GetTraeger(in.TraegerID); err != nil {
		return nil, fmt.Errorf("Träger %d nicht gefunden", in.TraegerID)
	}
	b := model.Befaehigung{
		TraegerID:    in.TraegerID,
		Name:         in.Name,
		Beschreibung: in.Beschreibung,
		CreatedAt:    s.now(),
	}
	if err := b.Validate(); err != nil {
		return nil, err
	}
	if err := s.DB.InsertBefaehigung(&b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Server) toolUpdateBefaehigung(args json.RawMessage, u auth.User) (any, error) {
	if err := nurBetreiber(u, "Befähigungen ändern"); err != nil {
		return nil, err
	}
	var in struct {
		ID           int64   `json:"id"`
		Name         *string `json:"name"`
		Beschreibung *string `json:"beschreibung"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	b, err := s.DB.GetBefaehigung(in.ID)
	if err != nil {
		return nil, fmt.Errorf("Befähigung %d nicht gefunden", in.ID)
	}
	applyIf(&b.Name, in.Name)
	applyIf(&b.Beschreibung, in.Beschreibung)
	if err := b.Validate(); err != nil {
		return nil, err
	}
	if err := s.DB.UpdateBefaehigung(b); err != nil {
		return nil, err
	}
	return b, nil
}

// toolErteileBefaehigung trägt eine Einweisung für eine Person ein.
//
// Der übliche Weg ist der Antrag: Jemand bittet um die Einweisung, der
// Träger-Admin entscheidet. Hier fehlt der erste Schritt — und das ist
// Absicht: Eingewiesen wird an der Motorsense, nicht in der App. Wer die
// Einweisung gegeben hat, trägt sie hinterher ein, ohne dass die Person
// vorher etwas beantragt haben muss. Ein bereits gestellter Antrag wird dabei
// entschieden statt verdoppelt (siehe InsertAntrag).
func (s *Server) toolErteileBefaehigung(args json.RawMessage, u auth.User) (any, error) {
	if err := nurBetreiber(u, "Befähigungen erteilen oder entziehen"); err != nil {
		return nil, err
	}
	var in struct {
		BefaehigungID int64  `json:"befaehigungId"`
		UserSub       string `json:"userSub"`
		Status        string `json:"status"`
		Notiz         string `json:"notiz"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if in.UserSub == "" {
		return nil, fmt.Errorf("userSub fehlt — die Kennung steht in der Rangliste und in jeder Erledigung")
	}
	status := model.AntragStatus(in.Status)
	if status == "" {
		status = model.AntragErteilt
	}
	if status != model.AntragErteilt && status != model.AntragAbgelehnt {
		return nil, fmt.Errorf("status muss erteilt oder abgelehnt sein, nicht %q", in.Status)
	}
	if _, err := s.DB.GetBefaehigung(in.BefaehigungID); err != nil {
		return nil, fmt.Errorf("Befähigung %d nicht gefunden", in.BefaehigungID)
	}

	// Erst den Antrag anlegen (oder den vorhandenen wiederbeleben), dann
	// entscheiden. Ein erteilter Antrag IST die Befähigung — deshalb gibt es
	// nur diese eine Tabelle und keinen zweiten Weg daneben.
	antrag := model.BefaehigungsAntrag{
		BefaehigungID: in.BefaehigungID,
		UserSub:       in.UserSub,
		Status:        model.AntragBeantragt,
		CreatedAt:     s.now(),
	}
	if err := s.DB.InsertAntrag(&antrag); err != nil {
		return nil, err
	}
	if err := s.DB.EntscheideAntrag(antrag.ID, status, u.Sub, in.Notiz, s.now()); err != nil {
		return nil, err
	}
	return s.DB.GetAntrag(antrag.ID)
}
