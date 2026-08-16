package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/vergabe"
)

// Endpunkte der Vergabe: Anmeldung zum Mithelfen, offene Benachrichtigungen
// abholen, zusagen und zurückgeben. Die Regeln stehen im Paket vergabe —
// hier wird nur übersetzt: JSON rein, JSON raus, deutscher Fehlertext.

// vergabeEngine baut die Vergabe mit der Zeitquelle dieses Servers.
func (s *Server) vergabeEngine() *vergabe.Engine {
	return vergabe.New(s.DB, vergabe.Config{Now: s.now, Zusteller: s.Zusteller})
}

// registerVergabe hängt die Endpunkte an den bereits geschützten Router.
func (s *Server) registerVergabe(api *http.ServeMux) {
	api.HandleFunc("GET /api/v1/me/signups", s.handleMySignups)
	api.HandleFunc("GET /api/v1/me/notifications", s.handleMyNotifications)
	api.HandleFunc("POST /api/v1/me/notifications/{id}/ack", s.handleAckNotification)
	api.HandleFunc("POST /api/v1/places/{id}/signup", s.handleSignup)
	api.HandleFunc("DELETE /api/v1/places/{id}/signup", s.handleSignoff)
	api.HandleFunc("GET /api/v1/places/{id}/signups", s.adminOnly(s.handlePlaceSignups))
	api.HandleFunc("POST /api/v1/assignments/{id}/claim", s.handleClaim)
	api.HandleFunc("POST /api/v1/assignments/{id}/release", s.handleRelease)
}

// writeVergabeErr übersetzt eine Abweisung der Vergabe in eine Antwort.
func writeVergabeErr(w http.ResponseWriter, r *http.Request, err error) {
	var ab *vergabe.Abweisung
	if errors.As(err, &ab) {
		writeErr(w, ab.Status, ab.Message)
		return
	}
	writeInternal(w, r, err)
}

// --- Anmeldung --------------------------------------------------------------

// SignupInput ist die Eingabe beim Anmelden. taskKind schränkt auf eine
// Aufgabenart ein (leer = alle Aufgaben des Ortes). userSub ist optional und
// dient als Sicherung: Angemeldet wird immer nur die eigene Person.
type SignupInput struct {
	UserSub  string `json:"userSub"`
	TaskKind string `json:"taskKind"`
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	placeID, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	u, _ := auth.FromContext(r.Context())
	var in SignupInput
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "ungültiges JSON")
			return
		}
	}
	// Niemand meldet jemand anderen an — auch Verwaltende nicht. Wer
	// mithilft, entscheidet jede und jeder selbst.
	if in.UserSub != "" && in.UserSub != u.Sub {
		writeErr(w, http.StatusForbidden, "es lässt sich nur die eigene Person anmelden")
		return
	}
	if in.TaskKind != "" && !model.ValidTaskKind(model.TaskKind(in.TaskKind)) {
		writeErr(w, http.StatusBadRequest, "taskKind muss giessen, jaeten oder sonstiges sein")
		return
	}
	place, err := s.DB.GetPlace(placeID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Ort nicht gefunden")
		return
	}
	// Wer den Ort nicht sehen darf, meldet sich dort auch nicht zum
	// Mithelfen an — sonst ließe sich seine Existenz erraten.
	if err := s.pruefeOrtSichtbar(r, placeID); err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	// Beim Anmelden das Profil anlegen, falls es noch keins gibt: Sonst
	// stünde in der Verwaltung eine Kennung ohne Namen.
	if _, err := ProfileFor(s.DB, u, s.now()); err != nil {
		writeInternal(w, r, err)
		return
	}
	eintrag := model.Signup{UserSub: u.Sub, PlaceID: placeID,
		TaskKind: model.TaskKind(in.TaskKind), CreatedAt: s.now()}
	neu, err := s.DB.InsertSignup(&eintrag)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	eintrag.PlaceName = place.Name
	status := http.StatusOK
	if neu {
		status = http.StatusCreated
	}
	writeJSON(w, status, eintrag)
}

func (s *Server) handleSignoff(w http.ResponseWriter, r *http.Request) {
	placeID, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	u, _ := auth.FromContext(r.Context())
	art := r.URL.Query().Get("taskKind")
	if art != "" && !model.ValidTaskKind(model.TaskKind(art)) {
		writeErr(w, http.StatusBadRequest, "taskKind muss giessen, jaeten oder sonstiges sein")
		return
	}
	if _, err := s.DB.DeleteSignups(u.Sub, placeID, art); err != nil {
		writeInternal(w, r, err)
		return
	}
	// Abmelden ist auch dann in Ordnung, wenn gar keine Anmeldung bestand:
	// Das Ergebnis ist genau das gewünschte.
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMySignups(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	liste, err := s.DB.ListSignupsByUser(u.Sub)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	if err := ergaenzeOrtsnamen(s.DB, liste); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"signups": liste})
}

// handlePlaceSignups zeigt, wer sich für einen Ort gemeldet hat. Nur für
// Verwaltende: Wer mithilft, ist keine öffentliche Angabe.
func (s *Server) handlePlaceSignups(w http.ResponseWriter, r *http.Request) {
	placeID, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	if err := s.pruefeOrtSichtbar(r, placeID); err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	liste, err := s.DB.ListSignupsForPlace(placeID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	if err := ergaenzeOrtsnamen(s.DB, liste); err != nil {
		writeInternal(w, r, err)
		return
	}
	namen, err := s.DB.NameResolver()
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	for i := range liste {
		liste[i].UserName = namen.Resolve(liste[i].UserSub, "")
	}
	writeJSON(w, http.StatusOK, map[string]any{"signups": liste})
}

func ergaenzeOrtsnamen(d *db.DB, liste []model.Signup) error {
	if len(liste) == 0 {
		return nil
	}
	orte, err := d.ListPlaces()
	if err != nil {
		return err
	}
	namen := map[int64]string{}
	for _, p := range orte {
		namen[p.ID] = p.Name
	}
	for i := range liste {
		liste[i].PlaceName = namen[liste[i].PlaceID]
	}
	return nil
}

// --- Benachrichtigungen -----------------------------------------------------

// handleMyNotifications liefert alles, was für mich offen ist: Anfragen mit
// Frist und Hinweise. Die App holt das regelmäßig ab — ein Push-Weg kann
// später danebengesetzt werden, ohne dass sich hier etwas ändert.
func (s *Server) handleMyNotifications(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	ns, err := s.vergabeEngine().OffeneBenachrichtigungen(u.Sub)
	if err != nil {
		writeVergabeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": ns})
}

func (s *Server) handleAckNotification(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	u, _ := auth.FromContext(r.Context())
	if err := s.vergabeEngine().Bestaetigen(id, u.Sub); err != nil {
		writeVergabeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Zusage -----------------------------------------------------------------

func (s *Server) handleClaim(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	u, _ := auth.FromContext(r.Context())
	// Vor allem anderen: Darf diese Person die Aufgabe überhaupt sehen — und
	// hat sie die verlangte Einweisung? Beides wird hier durchgesetzt und
	// nicht in der Oberfläche.
	vorgang, err := s.DB.GetAssignment(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Diesen Vorgang gibt es nicht (mehr).")
		return
	}
	task, err := s.DB.GetTask(vorgang.TaskID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Die Aufgabe zu diesem Vorgang gibt es nicht mehr.")
		return
	}
	if err := PruefeZusage(s.DB, s.zugriff(r), *task); err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	a, err := s.vergabeEngine().Zusagen(id, u.Sub, u.Name)
	if err != nil {
		writeVergabeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// handleRelease gibt eine Zusage zurück. Die zusagende Person darf das
// jederzeit; Verwaltende dürfen eine fremde Zusage aufheben.
func (s *Server) handleRelease(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	// Auch beim Zurückgeben zuerst die Sichtbarkeit: Sonst verriete der
	// Unterschied zwischen 404 und 409 beim Durchprobieren von Kennungen,
	// dass es zu einer internen Aufgabe einen Vorgang gibt.
	if vorgang, err := s.DB.GetAssignment(id); err == nil {
		if task, terr := s.DB.GetTask(vorgang.TaskID); terr == nil {
			if serr := s.pruefeSichtbar(r, *task); serr != nil {
				schreibeZugriffsfehler(w, r, serr)
				return
			}
		}
	}
	u, _ := auth.FromContext(r.Context())
	a, err := s.vergabeEngine().Zurueckgeben(id, u.Sub, u.IsAdmin())
	if err != nil {
		writeVergabeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// --- Vergabestand in der Orts-Liste -----------------------------------------

// ergaenzeVergabe hängt an jede Aufgabe den laufenden Vorgang und die Zahl
// der Angemeldeten. Ohne Vergabe (niemand angemeldet) bleibt alles wie zuvor.
func ergaenzeVergabe(d *db.DB, orte []model.PlaceWithStatus, namen model.NameResolver) error {
	laufende, err := d.ActiveAssignments()
	if err != nil {
		return err
	}
	proAufgabe := map[int64]model.Assignment{}
	for _, a := range laufende {
		a.ClaimedByName = namen.Resolve(a.ClaimedBy, a.ClaimedByName)
		proAufgabe[a.TaskID] = a
	}
	anmeldungen, err := d.ListSignups()
	if err != nil {
		return err
	}
	for i := range orte {
		for j := range orte[i].Tasks {
			t := &orte[i].Tasks[j]
			if a, ok := proAufgabe[t.ID]; ok {
				kopie := a
				t.Assignment = &kopie
			}
			for _, s := range anmeldungen {
				if s.Matches(t.CareTask) {
					t.SignupCount++
				}
			}
		}
	}
	return nil
}

// markiereEigeneAnmeldungen setzt signedUp für die abrufende Person.
func markiereEigeneAnmeldungen(d *db.DB, orte []model.PlaceWithStatus, userSub string) error {
	meine, err := d.ListSignupsByUser(userSub)
	if err != nil {
		return err
	}
	for i := range orte {
		for j := range orte[i].Tasks {
			t := &orte[i].Tasks[j]
			for _, s := range meine {
				if s.Matches(t.CareTask) {
					t.SignedUp = true
				}
			}
		}
	}
	return nil
}

// --- Einstellungen ----------------------------------------------------------

// AssignmentSettings ist die API-Sicht auf die Stellschrauben der Vergabe.
type AssignmentSettings struct {
	// OfferMinutes: Abstand zwischen zwei Anfragen.
	OfferMinutes int `json:"offerMinutes"`
	// ClaimHours: wie lange eine Zusage hält.
	ClaimHours int `json:"claimHours"`
	// QuietFrom/QuietTo: Ruhezeit in Ortszeit (keine Zustellung).
	QuietFrom int `json:"quietFrom"`
	QuietTo   int `json:"quietTo"`
}

func assignmentSettingsVon(r model.AssignmentRules) AssignmentSettings {
	return AssignmentSettings{
		OfferMinutes: int(r.OfferInterval / time.Minute),
		ClaimHours:   int(r.ClaimDuration / time.Hour),
		QuietFrom:    r.QuietFrom,
		QuietTo:      r.QuietTo,
	}
}

func (in AssignmentSettings) Rules() model.AssignmentRules {
	return model.AssignmentRules{
		OfferInterval: time.Duration(in.OfferMinutes) * time.Minute,
		ClaimDuration: time.Duration(in.ClaimHours) * time.Hour,
		QuietFrom:     in.QuietFrom,
		QuietTo:       in.QuietTo,
	}
}
