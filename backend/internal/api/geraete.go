package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Geräteverwaltung für den Push-Versand.
//
// Die App meldet ihre Firebase-Kennung bei jedem Start an (POST) und beim
// Abmelden wieder ab (DELETE). Mehrere Geräte je Person sind der Normalfall
// — Handy und Tablet, oder ein neues Gerät neben dem alten.
//
// Die Kennung ist ein Schlüssel zum Gerät: Wer sie hat, kann diesem Gerät
// Meldungen schicken. Sie kommt deshalb in keiner Antwort vor und lässt sich
// nur von der Person abmelden, der sie gehört.

func (s *Server) registerGeraete(api *http.ServeMux) {
	api.HandleFunc("POST /api/v1/me/devices", s.handleRegisterDevice)
	api.HandleFunc("DELETE /api/v1/me/devices", s.handleDeleteDevice)
}

// DeviceInput ist die Eingabe beim An- und Abmelden eines Geräts.
type DeviceInput struct {
	Token string `json:"token"`
	// Platform ist heute immer "android" (leer = android).
	Platform string `json:"platform"`
}

func (in *DeviceInput) Validate() string {
	in.Token = strings.TrimSpace(in.Token)
	in.Platform = strings.TrimSpace(in.Platform)
	switch {
	case in.Token == "":
		return "token fehlt"
	case len(in.Token) > model.MaxDeviceTokenLen:
		return "token ist zu lang"
	case len(in.Platform) > 32:
		return "platform ist zu lang"
	}
	if in.Platform == "" {
		in.Platform = "android"
	}
	return ""
}

func (s *Server) handleRegisterDevice(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	var in DeviceInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if fehler := in.Validate(); fehler != "" {
		writeErr(w, http.StatusBadRequest, fehler)
		return
	}
	jetzt := s.now()
	neu, err := s.DB.UpsertDevice(u.Sub, in.Token, in.Platform, jetzt)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	status := http.StatusOK
	if neu {
		status = http.StatusCreated
	}
	// Ohne die Kennung: Das Gerät kennt sie ohnehin, und sonst soll sie
	// niemand aus einer Antwort mitlesen können.
	writeJSON(w, status, model.Device{
		UserSub: u.Sub, Platform: in.Platform, CreatedAt: jetzt, UpdatedAt: jetzt,
	})
}

// handleDeleteDevice meldet ein Gerät ab. Die Kennung darf im Rumpf oder als
// Abfrage stehen — nicht jedes HTTP-Werkzeug schickt bei DELETE einen Rumpf.
//
// Abgemeldet wird immer nur die eigene Kennung; ist sie unbekannt oder
// fremd, ist das Ergebnis trotzdem 204: Es ist genau der gewünschte Zustand,
// und eine andere Antwort verriete, ob es die Kennung gibt.
func (s *Server) handleDeleteDevice(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	var in DeviceInput
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "ungültiges JSON")
			return
		}
	}
	if in.Token == "" {
		in.Token = r.URL.Query().Get("token")
	}
	in.Token = strings.TrimSpace(in.Token)
	if in.Token == "" || len(in.Token) > model.MaxDeviceTokenLen {
		writeErr(w, http.StatusBadRequest, "token fehlt")
		return
	}
	if _, err := s.DB.DeleteDevice(u.Sub, in.Token); err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
