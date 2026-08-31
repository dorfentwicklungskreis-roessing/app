package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Träger über MCP: anlegen, ändern, zulassen.
//
// Bis hierher ging das ausschließlich über die Web-Verwaltung im Browser.
// Das war die Lücke, die auffiel, als der Bereich „Verwaltung“ aus beiden
// Apps verschwand: Orte und Aufgaben pflegt der Betreiber im Chat über den
// Connector — für den Träger darunter musste er den Browser öffnen. Auf einem
// Telefon ohne angemeldetes Browserfenster ist das eine Sackgasse, und für
// eine Maschine ist es gar kein Weg.
//
// # Wer das darf
//
// Der MCP-Endpunkt verlangt für **jede** Anfrage die Rolle `admin` im Projekt
// `dorf-app` (siehe withAuth in auth.go) — also genau die Rolle, die
// `model.Zugriff.Betreiber` bedeutet. Wer hier ankommt, ist der Betreiber der
// Plattform. Die Prüfung steht trotzdem sichtbar in jedem schreibenden
// Werkzeug: Sie kostet nichts, und sie hält, falls der Endpunkt eines Tages
// auch Träger-Admins hereinlässt (Issue #35).

// traegerAnsicht ist ein Träger, wie MCP ihn zeigt: mit dem Namen seines
// Dachs statt bloß dessen Kennung. Eine nackte `parentId` müsste der Leser
// selbst auflösen — und würde es beim Vorlesen nicht tun.
type traegerAnsicht struct {
	model.Traeger
	// ParentName ist der Name des Dachs, leer bei einem eigenständigen Träger.
	ParentName string `json:"parentName,omitempty"`
	// Arbeitskreise sind die Träger, die unter diesem hier arbeiten.
	Arbeitskreise []string `json:"arbeitskreise,omitempty"`
}

func (s *Server) traegerAnsichten() ([]traegerAnsicht, error) {
	liste, err := s.DB.ListTraeger()
	if err != nil {
		return nil, err
	}
	nachID := make(map[int64]model.Traeger, len(liste))
	for _, t := range liste {
		nachID[t.ID] = t
	}
	out := make([]traegerAnsicht, 0, len(liste))
	for _, t := range liste {
		a := traegerAnsicht{Traeger: t}
		if dach, ok := nachID[t.ParentID]; ok {
			a.ParentName = dach.Name
		}
		for _, k := range liste {
			if k.ParentID == t.ID {
				a.Arbeitskreise = append(a.Arbeitskreise, k.Name)
			}
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *Server) toolListTraeger(_ json.RawMessage, _ auth.User) (any, error) {
	return s.traegerAnsichten()
}

// nurBetreiber ist die eine Stelle, an der die schreibenden Träger-Werkzeuge
// ihre Berechtigung prüfen. Sie entscheidet nichts selbst, sondern liest die
// Rolle, die auth.User schon trägt.
func nurBetreiber(u auth.User, was string) error {
	if !u.IsAdmin() {
		return fmt.Errorf("%s darf nur der Betreiber der Dorf-App", was)
	}
	return nil
}

func (s *Server) toolCreateTraeger(args json.RawMessage, u auth.User) (any, error) {
	if err := nurBetreiber(u, "Träger anlegen"); err != nil {
		return nil, err
	}
	var in struct {
		Name         string `json:"name"`
		Beschreibung string `json:"beschreibung"`
		ProjektID    string `json:"projektId"`
		Sichtbarkeit string `json:"sichtbarkeit"`
		Status       string `json:"status"`
		ParentID     int64  `json:"parentId"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	t := model.Traeger{
		Name:         in.Name,
		Beschreibung: in.Beschreibung,
		ProjektID:    in.ProjektID,
		Sichtbarkeit: model.TraegerSichtbarkeit(in.Sichtbarkeit),
		Status:       model.TraegerStatus(in.Status),
		ParentID:     in.ParentID,
		CreatedAt:    s.now(),
	}
	// Vorbelegungen wie im Formular der Web-Verwaltung: sichtbar im
	// Verzeichnis, aber noch nicht zugelassen. Zulassen ist ein eigener,
	// bewusster Schritt — auch für den Betreiber.
	if t.Sichtbarkeit == "" {
		t.Sichtbarkeit = model.TraegerOffen
	}
	if t.Status == "" {
		t.Status = model.TraegerBeantragt
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	if err := s.DB.InsertTraeger(&t); err != nil {
		return nil, fmt.Errorf("Träger konnte nicht angelegt werden: %w "+
			"(ist die Zitadel-Projekt-ID schon vergeben?)", err)
	}
	return t, nil
}

func (s *Server) toolUpdateTraeger(args json.RawMessage, u auth.User) (any, error) {
	if err := nurBetreiber(u, "Träger ändern"); err != nil {
		return nil, err
	}
	var in struct {
		ID           int64   `json:"id"`
		Name         *string `json:"name"`
		Beschreibung *string `json:"beschreibung"`
		ProjektID    *string `json:"projektId"`
		Sichtbarkeit *string `json:"sichtbarkeit"`
		ParentID     *int64  `json:"parentId"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	t, err := s.DB.GetTraeger(in.ID)
	if err != nil {
		return nil, fmt.Errorf("Träger %d nicht gefunden", in.ID)
	}
	applyIf(&t.Name, in.Name)
	applyIf(&t.Beschreibung, in.Beschreibung)
	applyIf(&t.ProjektID, in.ProjektID)
	applyIf(&t.ParentID, in.ParentID)
	if in.Sichtbarkeit != nil {
		t.Sichtbarkeit = model.TraegerSichtbarkeit(*in.Sichtbarkeit)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	if err := s.DB.UpdateTraeger(t); err != nil {
		return nil, err
	}
	return t, nil
}

// toolTraegerZulassung lässt zu oder sperrt.
//
// Bewusst ein eigenes Werkzeug und nicht ein Feld in traeger_aendern: Der
// Zulassungsstand ist die Frage, ob eine Gruppe im Namen des Dorfes auftreten
// darf. Sie nebenbei mit einer Umbenennung zu erledigen wäre zu leicht.
func (s *Server) toolTraegerZulassung(args json.RawMessage, u auth.User) (any, error) {
	if err := nurBetreiber(u, "Träger zulassen oder sperren"); err != nil {
		return nil, err
	}
	var in struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	status := model.TraegerStatus(in.Status)
	if !model.ValidTraegerStatus(status) {
		return nil, fmt.Errorf("unbekannter Zulassungsstand %q — "+
			"erlaubt sind beantragt, zugelassen, gesperrt", in.Status)
	}
	t, err := s.DB.GetTraeger(in.ID)
	if err != nil {
		return nil, fmt.Errorf("Träger %d nicht gefunden", in.ID)
	}
	t.Status = status
	if err := s.DB.UpdateTraeger(t); err != nil {
		return nil, err
	}
	return t, nil
}
