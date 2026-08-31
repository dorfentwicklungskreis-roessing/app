package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/mitglied"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Beitritt: „Ich will bei euch mitmachen.“
//
// Bis hierher trug Mitgliedschaften ausschließlich ein Mensch in der
// Zitadel-Konsole ein. Wer im Dorf einen Verein oder einen Arbeitskreis sah
// und mitmachen wollte, konnte das nirgends sagen.
//
// Der Ablauf ist der der Befähigungen — beantragen, freigeben oder ablehnen.
// Der eine Unterschied ist der wichtige: Ein erteilter Beitritt ist NICHT die
// Mitgliedschaft. Die steht in der Rössing-ID. Deshalb wird hier zuerst dort
// geschrieben und erst danach der Vorgang abgehakt; scheitert das Schreiben,
// bleibt der Antrag offen und sagt, warum. Ein Verfahren, das am Ende nichts
// bewirkt, wäre schlimmer als gar keins: Die App behauptete „Mitglied“, und
// die Tür bliebe zu.

func (s *Server) registerBeitritt(api *http.ServeMux) {
	api.HandleFunc("POST /api/v1/traeger/{id}/beitritt", s.handleBeitrittBeantragen)
	api.HandleFunc("GET /api/v1/traeger/{id}/beitritte", s.handleListBeitritte)
	api.HandleFunc("POST /api/v1/traeger/{id}/mitglieder", s.handleMitgliedAufnehmen)
	api.HandleFunc("POST /api/v1/beitritte/{id}", s.handleBeitrittEntscheiden)
	api.HandleFunc("GET /api/v1/me/beitritte", s.handleMeineBeitritte)
}

// --- Antrag stellen ---------------------------------------------------------

func (s *Server) handleBeitrittBeantragen(w http.ResponseWriter, r *http.Request) {
	t, z, err := s.traegerAusPfad(r)
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	if hindernis := z.BeitrittsHindernis(t); hindernis != "" {
		// 409 und nicht 403: Es fehlt keine Berechtigung, es passt die Lage
		// nicht — man ist schon dabei, oder die Gruppe nimmt keine Anträge.
		writeErr(w, http.StatusConflict, hindernis)
		return
	}
	var in struct {
		Begruendung string `json:"begruendung"`
	}
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeErr(w, http.StatusBadRequest, "ungültiges JSON")
			return
		}
	}
	if err := pruefeText("begruendung", in.Begruendung); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	u, _ := auth.FromContext(r.Context())
	// Das Profil anlegen, falls es noch keins gibt: Sonst stünde beim
	// Träger-Admin eine nackte Kennung ohne Namen.
	if _, err := ProfileFor(s.DB, u, s.now()); err != nil {
		writeInternal(w, r, err)
		return
	}
	b := model.Beitritt{TraegerID: t.ID, UserSub: u.Sub, Status: model.AntragBeantragt,
		Begruendung: in.Begruendung, CreatedAt: s.now()}
	if err := s.DB.InsertBeitritt(&b); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// --- Anträge sehen ----------------------------------------------------------

func (s *Server) handleListBeitritte(w http.ResponseWriter, r *http.Request) {
	t, z, err := s.traegerAusPfad(r)
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	if !z.DarfBeitrittEntscheiden(t) {
		schreibeZugriffsfehler(w, r, verwaltungAbgelehnt(z, t))
		return
	}
	status := model.AntragStatus(r.URL.Query().Get("status"))
	if status != "" && !model.ValidAntragStatus(status) {
		writeErr(w, http.StatusBadRequest, "status muss beantragt, erteilt oder abgelehnt sein")
		return
	}
	liste, err := s.DB.ListBeitritte(t.ID, status)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	if err := s.namenEintragen(liste); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"beitritte": liste})
}

// namenEintragen löst die Kennungen zu Namen auf — ein Träger-Admin soll
// lesen, wer da fragt, und nicht eine Zeichenkette aus der Rössing-ID.
func (s *Server) namenEintragen(liste []model.Beitritt) error {
	if len(liste) == 0 {
		return nil
	}
	namen, err := s.DB.NameResolver()
	if err != nil {
		return err
	}
	for i := range liste {
		liste[i].UserName = namen.Resolve(liste[i].UserSub, "")
	}
	return nil
}

func (s *Server) handleMeineBeitritte(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	liste, err := s.DB.ListBeitritteVonPerson(u.Sub)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"beitritte": liste})
}

// --- Entscheiden ------------------------------------------------------------

func (s *Server) handleBeitrittEntscheiden(w http.ResponseWriter, r *http.Request) {
	z := s.zugriff(r)
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	b, err := s.DB.GetBeitritt(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Diesen Antrag gibt es nicht.")
		return
	}
	t, err := s.DB.GetTraeger(b.TraegerID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Den Träger zu diesem Antrag gibt es nicht mehr.")
		return
	}
	if !z.DarfBeitrittEntscheiden(*t) {
		schreibeZugriffsfehler(w, r, verwaltungAbgelehnt(z, *t))
		return
	}
	var in struct {
		Status string `json:"status"`
		Notiz  string `json:"notiz"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if err := pruefeText("notiz", in.Notiz); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	entschieden, err := BeitrittEntscheiden(r.Context(), s.DB, s.Mitglieder, *t, *b,
		model.AntragStatus(in.Status), z.Sub, in.Notiz, s.now())
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, entschieden)
}

// --- Direkt aufnehmen -------------------------------------------------------

// handleMitgliedAufnehmen nimmt jemanden ohne vorherigen Antrag auf.
//
// Warum es das gibt: Eine geschlossene Gruppe nimmt Anträge gar nicht erst
// entgegen (siehe model.Zugriff.BeitrittsHindernis) — sie holt sich ihre
// Leute selbst. Und auch bei einem offenen Träger wird im Dorf öfter am
// Gartenzaun gefragt als in der App; wer zugesagt hat, soll nicht erst noch
// einen Antrag stellen müssen. Ein offener Antrag wird dabei mitentschieden
// statt verdoppelt.
func (s *Server) handleMitgliedAufnehmen(w http.ResponseWriter, r *http.Request) {
	t, z, err := s.traegerAusPfad(r)
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	if !z.DarfBeitrittEntscheiden(t) {
		schreibeZugriffsfehler(w, r, verwaltungAbgelehnt(z, t))
		return
	}
	var in struct {
		UserSub string `json:"userSub"`
		Notiz   string `json:"notiz"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if err := pruefeText("notiz", in.Notiz); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	b, err := Aufnehmen(r.Context(), s.DB, s.Mitglieder, t, in.UserSub, z.Sub, in.Notiz, s.now())
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// --- Der eine Weg nach Zitadel ----------------------------------------------

// Aufnehmen schreibt die Mitgliedschaft in die Rössing-ID und hält den
// Vorgang fest.
//
// REST, Web-Verwaltung und MCP nehmen denselben Weg. Drei eigene Wege wären
// drei Gelegenheiten, den entscheidenden Schritt zu vergessen — und der
// vergessene wäre jedes Mal derselbe: das Zurückschreiben.
func Aufnehmen(ctx context.Context, d *db.DB, q mitglied.Quelle, t model.Traeger,
	userSub, durch, notiz string, jetzt time.Time,
) (*model.Beitritt, error) {
	if userSub == "" {
		return nil, &CompletionError{Status: http.StatusBadRequest,
			Message: "Ohne Kennung der Person (userSub) geht das nicht — " +
				"sie steht in der Rangliste und in jeder Erledigung."}
	}
	if err := nachZitadel(ctx, q, t, userSub); err != nil {
		return nil, err
	}
	b := model.Beitritt{TraegerID: t.ID, UserSub: userSub, Status: model.AntragBeantragt,
		CreatedAt: jetzt}
	if err := d.InsertBeitritt(&b); err != nil {
		return nil, err
	}
	if err := d.EntscheideBeitritt(b.ID, model.AntragErteilt, durch, notiz, jetzt); err != nil {
		return nil, err
	}
	return d.GetBeitritt(b.ID)
}

// BeitrittEntscheiden gibt einen vorliegenden Antrag frei oder lehnt ihn ab.
// Bei der Freigabe steht zuerst die Rollenzuweisung in der Rössing-ID, dann
// erst der Vermerk hier — andersherum stünde hier „Mitglied“, während die
// Tür zu bliebe.
func BeitrittEntscheiden(ctx context.Context, d *db.DB, q mitglied.Quelle, t model.Traeger,
	b model.Beitritt, status model.AntragStatus, durch, notiz string, jetzt time.Time,
) (*model.Beitritt, error) {
	if status != model.AntragErteilt && status != model.AntragAbgelehnt {
		return nil, &CompletionError{Status: http.StatusBadRequest,
			Message: "status muss erteilt oder abgelehnt sein"}
	}
	if status == model.AntragErteilt {
		if err := nachZitadel(ctx, q, t, b.UserSub); err != nil {
			return nil, err
		}
	}
	if err := d.EntscheideBeitritt(b.ID, status, durch, notiz, jetzt); err != nil {
		return nil, err
	}
	return d.GetBeitritt(b.ID)
}

// nachZitadel ist die einzige Stelle, an der eine Mitgliedschaft in die
// Rössing-ID geschrieben wird.
func nachZitadel(ctx context.Context, q mitglied.Quelle, t model.Traeger, userSub string) error {
	if t.ProjektID == "" {
		return &CompletionError{Status: http.StatusConflict,
			Message: "Dieser Träger hat in der Rössing-ID kein Projekt — " +
				"solange es keins gibt, kann er keine Mitglieder haben."}
	}
	a, ok := mitglied.AufnehmerVon(q)
	if !ok {
		return &CompletionError{Status: http.StatusServiceUnavailable, Message: NochNichtEingerichtet}
	}
	if err := a.Aufnehmen(ctx, t.ProjektID, userSub, model.RolleMitglied); err != nil {
		// Die volle Antwort der Rössing-ID gehört ins Protokoll, nicht vor
		// die Leute: Sie nennt Pfade und Kennungen.
		slog.Error("Aufnahme in die Rössing-ID gescheitert",
			"projekt", t.ProjektID, "person", userSub, "err", err)
		return &CompletionError{Status: http.StatusBadGateway,
			Message: "Die Mitgliedschaft konnte in der Rössing-ID nicht eingetragen werden — " +
				"der Antrag bleibt deshalb offen. " + aufnahmeHinweis(err)}
	}
	return nil
}

// NochNichtEingerichtet ist die Antwort für den Betriebszustand ohne
// schreibenden Dienst-Nutzer. Sie sagt ausdrücklich, dass nichts passiert
// ist — eine Freigabe, die stillschweigend ins Leere liefe, wäre die
// schlechteste aller Antworten.
const NochNichtEingerichtet = "Aufnehmen geht noch nicht: Das Backend kann in der Rössing-ID " +
	"keine Mitgliedschaften eintragen. Dafür braucht es einen Dienst-Nutzer mit Schreibrecht " +
	"auf Rollenzuweisungen; bis dahin trägt der Betreiber die Mitgliedschaft von Hand ein."

// aufnahmeHinweis sagt, ob es sich lohnt, es gleich noch einmal zu versuchen.
func aufnahmeHinweis(err error) string {
	var api *mitglied.APIFehler
	if errors.As(err, &api) && api.FehlendesRecht() {
		return "Der Dienst-Nutzer darf dort keine Rollen vergeben — das muss der Betreiber " +
			"in der Rössing-ID freischalten."
	}
	return "Bitte gleich noch einmal versuchen."
}
