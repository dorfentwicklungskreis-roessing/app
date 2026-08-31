package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/mitglied"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Träger (Vereine und Gruppen), ihre Befähigungen und die Anträge darauf.
//
// Alles, was in der App zu sehen ist, gehört einem Träger. Wer was sehen und
// ändern darf, entscheidet model.Zugriff — hier wird nur übersetzt: JSON
// rein, JSON raus, deutscher Fehlertext.

func (s *Server) registerTraeger(api *http.ServeMux) {
	api.HandleFunc("GET /api/v1/traeger", s.handleListTraeger)
	api.HandleFunc("POST /api/v1/traeger", s.betreiberOnly(s.handleCreateTraeger))
	api.HandleFunc("GET /api/v1/traeger/{id}", s.handleGetTraeger)
	api.HandleFunc("PUT /api/v1/traeger/{id}", s.handleUpdateTraeger)

	api.HandleFunc("GET /api/v1/traeger/{id}/befaehigungen", s.handleListBefaehigungen)
	api.HandleFunc("POST /api/v1/traeger/{id}/befaehigungen", s.handleCreateBefaehigung)
	api.HandleFunc("PUT /api/v1/befaehigungen/{id}", s.handleUpdateBefaehigung)
	api.HandleFunc("DELETE /api/v1/befaehigungen/{id}", s.handleDeleteBefaehigung)

	api.HandleFunc("POST /api/v1/befaehigungen/{id}/antrag", s.handleAntragStellen)
	api.HandleFunc("GET /api/v1/traeger/{id}/antraege", s.handleListAntraege)
	api.HandleFunc("POST /api/v1/antraege/{id}", s.handleAntragEntscheiden)
	api.HandleFunc("GET /api/v1/me/befaehigungen", s.handleMeineBefaehigungen)

	s.registerBeitritt(api)
}

// --- Zugriffs-Helfer --------------------------------------------------------

// zugriff baut die Berechtigungssicht des Aufrufers. Die Mitgliedschaften
// kommen aus internal/mitglied (Zitadel-Dienst-Nutzer, kurz gepuffert) und
// bewusst nicht aus dem Token — siehe dortige Paketdokumentation.
func (s *Server) zugriff(r *http.Request) model.Zugriff {
	u, _ := auth.FromContext(r.Context())
	return mitglied.Zugriff(r.Context(), s.Mitglieder, u)
}

// betreiberOnly schützt, was ausschließlich die Plattform darf: Träger
// anlegen und zulassen.
func (s *Server) betreiberOnly(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, ok := auth.FromContext(r.Context())
		if !ok || !u.IsAdmin() {
			writeErr(w, http.StatusForbidden,
				"Das darf nur der Betreiber der Dorf-App.")
			return
		}
		h(w, r)
	}
}

// traegerVon liefert den Träger eines Ortes.
func traegerVon(d *db.DB, place model.Place) (model.Traeger, error) {
	t, err := d.GetTraeger(place.TraegerID)
	if err != nil {
		return model.Traeger{}, err
	}
	return *t, nil
}

// darfOrtVerwalten prüft, ob der Aufrufer diesen Ort (und damit seine
// Aufgaben) ändern darf. Der Fehler ist fertig beantwortbar.
func (s *Server) darfOrtVerwalten(r *http.Request, place model.Place) (model.Zugriff, error) {
	z := s.zugriff(r)
	traeger, err := traegerVon(s.DB, place)
	if err != nil {
		return z, err
	}
	if !z.DarfVerwalten(traeger) {
		return z, verwaltungAbgelehnt(z, traeger)
	}
	return z, nil
}

// verwaltungAbgelehnt formuliert die Absage — und unterscheidet dabei den
// Zitadel-Ausfall vom schlichten Nein. Ein „503 später nochmal“ ist etwas
// anderes als „du darfst das nicht“, und wer davorsteht, soll es erfahren.
func verwaltungAbgelehnt(z model.Zugriff, t model.Traeger) error {
	if z.Veraltet && !z.Betreiber {
		return &CompletionError{Status: http.StatusServiceUnavailable,
			Message: "Die Rössing-ID ist gerade nicht erreichbar. Lesen geht weiter, " +
				"Änderungen sind erst wieder möglich, wenn die Mitgliedschaften gesichert abgefragt werden können."}
	}
	// Der Name kommt über den Zugriff: Wer den Träger nicht sehen darf, soll
	// ihn auch nicht aus einer Fehlermeldung erfahren — sonst ließen sich
	// geschlossene Gruppen durch Ausprobieren von Kennungen aufzählen.
	return &CompletionError{Status: http.StatusForbidden,
		Message: "Das dürfen nur die Verwaltenden von " + zitat(z.TraegerAnzeigeName(t)) + "."}
}

// schreibeZugriffsfehler beantwortet einen Zugriffsfehler.
func schreibeZugriffsfehler(w http.ResponseWriter, r *http.Request, err error) {
	var ce *CompletionError
	if errors.As(err, &ce) {
		writeErr(w, ce.Status, ce.Message)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusNotFound, "nicht gefunden")
		return
	}
	writeInternal(w, r, err)
}

// SichtVon baut den Rangliste-Filter: Welche Träger darf diese Person mitzählen?
func SichtVon(d *db.DB, z model.Zugriff) (db.Sicht, error) {
	if z.Betreiber {
		return db.SichtAlles(), nil
	}
	alle, err := d.ListTraeger()
	if err != nil {
		return db.Sicht{}, err
	}
	sicht := db.Sicht{}
	for _, t := range alle {
		if t.Zugelassen() && z.Mitglied.IstMitglied(t.ProjektID) {
			sicht.TraegerIDs = append(sicht.TraegerIDs, t.ID)
		}
	}
	return sicht, nil
}

// --- Träger -----------------------------------------------------------------

// TraegerInput ist der Eingabe-Datensatz für Träger.
type TraegerInput struct {
	Name         string `json:"name"`
	Beschreibung string `json:"beschreibung"`
	// ProjektID ist die Zitadel-Projekt-ID. Nur der Betreiber darf sie
	// setzen: Sie entscheidet, wessen Rollen hier gelten.
	ProjektID string `json:"projektId"`
	// Status darf nur der Betreiber ändern (Zulassung, Sperre).
	Status string `json:"status"`
	// Sichtbarkeit pflegt der Träger selbst.
	Sichtbarkeit string `json:"sichtbarkeit"`
}

// TraegerAnsicht ist ein Träger, wie ihn genau diese Person sieht.
//
// Ohne diese Felder müsste die App raten, ob der Knopf „Mitmachen“ etwas
// bewirkt — und die Antwort hinge an Regeln, die nur der Server kennt (Zutritt,
// Sichtbarkeit, bestehende Mitgliedschaft, ein schon gestellter Antrag).
// Deshalb steht hier, was gilt, und nicht bloß der Träger.
type TraegerAnsicht struct {
	model.Traeger
	IstMitglied   bool `json:"istMitglied"`
	DarfVerwalten bool `json:"darfVerwalten"`
	// BeitrittMoeglich: Hier kann diese Person jetzt einen Antrag stellen.
	BeitrittMoeglich bool `json:"beitrittMoeglich"`
	// BeitrittHindernis nennt den Grund, warum nicht — leer, wenn es geht.
	// Er ist deutsch und für die Anzeige gedacht.
	BeitrittHindernis string `json:"beitrittHindernis,omitempty"`
	// BeitrittStatus ist der Stand des eigenen Antrags („“ = keiner).
	BeitrittStatus model.AntragStatus `json:"beitrittStatus,omitempty"`
	// OffeneBeitritte zählt die unentschiedenen Anträge — nur für die,
	// die den Träger verwalten.
	OffeneBeitritte int `json:"offeneBeitritte,omitempty"`
}

// traegerAnsicht baut die Sicht einer Person auf einen Träger.
func (s *Server) traegerAnsicht(z model.Zugriff, t model.Traeger, offen map[int64]int) TraegerAnsicht {
	a := TraegerAnsicht{
		Traeger:           t,
		IstMitglied:       z.Mitglied.IstMitglied(t.ProjektID),
		DarfVerwalten:     z.DarfVerwalten(t),
		BeitrittHindernis: z.BeitrittsHindernis(t),
	}
	a.BeitrittMoeglich = a.BeitrittHindernis == ""
	if eigener, err := s.DB.BeitrittVon(t.ID, z.Sub); err == nil && eigener != nil {
		a.BeitrittStatus = eigener.Status
	}
	if a.DarfVerwalten {
		a.OffeneBeitritte = offen[t.ID]
	}
	return a
}

func (s *Server) handleListTraeger(w http.ResponseWriter, r *http.Request) {
	z := s.zugriff(r)
	alle, err := s.DB.ListTraeger()
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	offen, err := s.DB.OffeneBeitritte()
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	sichtbar := []TraegerAnsicht{}
	for _, t := range alle {
		if z.SiehtTraeger(t) {
			sichtbar = append(sichtbar, s.traegerAnsicht(z, t, offen))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"traeger": sichtbar})
}

func (s *Server) handleGetTraeger(w http.ResponseWriter, r *http.Request) {
	t, z, err := s.traegerAusPfad(r)
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	offen, err := s.DB.OffeneBeitritte()
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"traeger": s.traegerAnsicht(z, t, offen), "darfVerwalten": z.DarfVerwalten(t),
	})
}

// traegerAusPfad lädt den Träger aus {id} — und verschweigt seine Existenz,
// wenn der Aufrufer ihn nicht sehen darf.
func (s *Server) traegerAusPfad(r *http.Request) (model.Traeger, model.Zugriff, error) {
	z := s.zugriff(r)
	id, err := pathID(r)
	if err != nil {
		return model.Traeger{}, z, &CompletionError{Status: http.StatusBadRequest, Message: "ungültige ID"}
	}
	t, err := s.DB.GetTraeger(id)
	if err != nil {
		return model.Traeger{}, z, &CompletionError{Status: http.StatusNotFound, Message: "Diesen Träger gibt es nicht."}
	}
	if !z.SiehtTraeger(*t) {
		return model.Traeger{}, z, &CompletionError{Status: http.StatusNotFound, Message: "Diesen Träger gibt es nicht."}
	}
	return *t, z, nil
}

func (s *Server) handleCreateTraeger(w http.ResponseWriter, r *http.Request) {
	var in TraegerInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	t := model.Traeger{
		Name: in.Name, Beschreibung: in.Beschreibung, ProjektID: in.ProjektID,
		Status: model.TraegerStatus(in.Status), Sichtbarkeit: model.TraegerSichtbarkeit(in.Sichtbarkeit),
		CreatedAt: s.now(),
	}
	if t.Status == "" {
		t.Status = model.TraegerBeantragt
	}
	if t.Sichtbarkeit == "" {
		t.Sichtbarkeit = model.TraegerOffen
	}
	if err := t.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := pruefeTraegerTexte(t); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.DB.InsertTraeger(&t); err != nil {
		writeErr(w, http.StatusConflict,
			"Dieser Träger ließ sich nicht anlegen — ist die Zitadel-Projekt-ID schon vergeben?")
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) handleUpdateTraeger(w http.ResponseWriter, r *http.Request) {
	t, z, err := s.traegerAusPfad(r)
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	if !z.DarfVerwalten(t) {
		schreibeZugriffsfehler(w, r, verwaltungAbgelehnt(z, t))
		return
	}
	var in TraegerInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	neu := t
	neu.Name, neu.Beschreibung = in.Name, in.Beschreibung
	if in.Sichtbarkeit != "" {
		neu.Sichtbarkeit = model.TraegerSichtbarkeit(in.Sichtbarkeit)
	}
	// Zulassungsstand und Zitadel-Projekt bleiben dem Betreiber vorbehalten:
	// Sonst ließe sich ein Träger selbst freischalten oder an ein fremdes
	// Projekt hängen und damit dessen Mitglieder vereinnahmen.
	if !z.Betreiber {
		if (in.Status != "" && model.TraegerStatus(in.Status) != t.Status) ||
			(in.ProjektID != "" && in.ProjektID != t.ProjektID) {
			writeErr(w, http.StatusForbidden,
				"Zulassung und Zitadel-Projekt ändert nur der Betreiber der Dorf-App.")
			return
		}
	} else {
		if in.Status != "" {
			neu.Status = model.TraegerStatus(in.Status)
		}
		neu.ProjektID = in.ProjektID
	}
	if err := neu.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := pruefeTraegerTexte(neu); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.DB.UpdateTraeger(&neu); err != nil {
		writeErr(w, http.StatusConflict,
			"Änderung nicht möglich — ist die Zitadel-Projekt-ID schon vergeben?")
		return
	}
	writeJSON(w, http.StatusOK, neu)
}

func pruefeTraegerTexte(t model.Traeger) error {
	if err := pruefeText("name", t.Name); err != nil {
		return err
	}
	return pruefeText("beschreibung", t.Beschreibung)
}

// --- Befähigungen -----------------------------------------------------------

// BefaehigungInput ist der Eingabe-Datensatz für Befähigungen.
type BefaehigungInput struct {
	Name         string `json:"name"`
	Beschreibung string `json:"beschreibung"`
}

func (s *Server) handleListBefaehigungen(w http.ResponseWriter, r *http.Request) {
	t, _, err := s.traegerAusPfad(r)
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	liste, err := s.DB.ListBefaehigungen(t.ID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"befaehigungen": liste})
}

func (s *Server) handleCreateBefaehigung(w http.ResponseWriter, r *http.Request) {
	t, z, err := s.traegerAusPfad(r)
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	if !z.DarfVerwalten(t) {
		schreibeZugriffsfehler(w, r, verwaltungAbgelehnt(z, t))
		return
	}
	var in BefaehigungInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	b := model.Befaehigung{TraegerID: t.ID, Name: in.Name, Beschreibung: in.Beschreibung,
		CreatedAt: s.now()}
	if err := b.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := pruefeText("name", b.Name); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := pruefeText("beschreibung", b.Beschreibung); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.DB.InsertBefaehigung(&b); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

// befaehigungAusPfad lädt eine Befähigung samt ihrem Träger und prüft, ob der
// Aufrufer sie pflegen darf.
func (s *Server) befaehigungAusPfad(r *http.Request, zumVerwalten bool) (model.Befaehigung, model.Traeger, model.Zugriff, error) {
	z := s.zugriff(r)
	id, err := pathID(r)
	if err != nil {
		return model.Befaehigung{}, model.Traeger{}, z,
			&CompletionError{Status: http.StatusBadRequest, Message: "ungültige ID"}
	}
	b, err := s.DB.GetBefaehigung(id)
	if err != nil {
		return model.Befaehigung{}, model.Traeger{}, z,
			&CompletionError{Status: http.StatusNotFound, Message: "Diese Befähigung gibt es nicht."}
	}
	t, err := s.DB.GetTraeger(b.TraegerID)
	if err != nil {
		return *b, model.Traeger{}, z,
			&CompletionError{Status: http.StatusNotFound, Message: "Den Träger dieser Befähigung gibt es nicht."}
	}
	if !z.SiehtTraeger(*t) {
		return *b, *t, z,
			&CompletionError{Status: http.StatusNotFound, Message: "Diese Befähigung gibt es nicht."}
	}
	if zumVerwalten && !z.DarfVerwalten(*t) {
		return *b, *t, z, verwaltungAbgelehnt(z, *t)
	}
	return *b, *t, z, nil
}

func (s *Server) handleUpdateBefaehigung(w http.ResponseWriter, r *http.Request) {
	b, _, _, err := s.befaehigungAusPfad(r, true)
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	var in BefaehigungInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	b.Name, b.Beschreibung = in.Name, in.Beschreibung
	if err := b.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.DB.UpdateBefaehigung(&b); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) handleDeleteBefaehigung(w http.ResponseWriter, r *http.Request) {
	b, _, _, err := s.befaehigungAusPfad(r, true)
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	if err := s.DB.DeleteBefaehigung(b.ID); err != nil {
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Anträge ----------------------------------------------------------------

// handleAntragStellen: „Ich war bei der Einweisung, bitte freigeben.“
// Beantragen darf jede und jeder, der die Befähigung sehen kann — entschieden
// wird ohnehin vom Träger.
func (s *Server) handleAntragStellen(w http.ResponseWriter, r *http.Request) {
	b, _, _, err := s.befaehigungAusPfad(r, false)
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
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
	a := model.BefaehigungsAntrag{BefaehigungID: b.ID, UserSub: u.Sub,
		Status: model.AntragBeantragt, Begruendung: in.Begruendung, CreatedAt: s.now()}
	if err := s.DB.InsertAntrag(&a); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleListAntraege(w http.ResponseWriter, r *http.Request) {
	t, z, err := s.traegerAusPfad(r)
	if err != nil {
		schreibeZugriffsfehler(w, r, err)
		return
	}
	if !z.DarfVerwalten(t) {
		schreibeZugriffsfehler(w, r, verwaltungAbgelehnt(z, t))
		return
	}
	status := model.AntragStatus(r.URL.Query().Get("status"))
	if status != "" && !model.ValidAntragStatus(status) {
		writeErr(w, http.StatusBadRequest, "status muss beantragt, erteilt oder abgelehnt sein")
		return
	}
	liste, err := s.DB.ListAntraege(t.ID, status)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	namen, err := s.DB.NameResolver()
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	for i := range liste {
		liste[i].UserName = namen.Resolve(liste[i].UserSub, "", model.SichtVerwaltung)
	}
	writeJSON(w, http.StatusOK, map[string]any{"antraege": liste})
}

func (s *Server) handleAntragEntscheiden(w http.ResponseWriter, r *http.Request) {
	z := s.zugriff(r)
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	a, err := s.DB.GetAntrag(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Diesen Antrag gibt es nicht.")
		return
	}
	b, err := s.DB.GetBefaehigung(a.BefaehigungID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Die Befähigung zu diesem Antrag gibt es nicht mehr.")
		return
	}
	t, err := s.DB.GetTraeger(b.TraegerID)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Den Träger zu diesem Antrag gibt es nicht mehr.")
		return
	}
	if !z.DarfVerwalten(*t) {
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
	status := model.AntragStatus(in.Status)
	if status != model.AntragErteilt && status != model.AntragAbgelehnt {
		writeErr(w, http.StatusBadRequest, "status muss erteilt oder abgelehnt sein")
		return
	}
	if err := pruefeText("notiz", in.Notiz); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.DB.EntscheideAntrag(a.ID, status, z.Sub, in.Notiz, s.now()); err != nil {
		writeInternal(w, r, err)
		return
	}
	neu, err := s.DB.GetAntrag(a.ID)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, neu)
}

// handleMeineBefaehigungen zeigt, was ich habe und was noch offen ist.
func (s *Server) handleMeineBefaehigungen(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	liste, err := s.DB.ListAntraegeVonPerson(u.Sub)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"befaehigungen": liste})
}

// --- Prüfung beim Zusagen ---------------------------------------------------

// PruefeZusage entscheidet, ob jemand diese Aufgabe übernehmen darf.
//
// Zwei Hürden, beide serverseitig:
//   - Sichtbarkeit: Eine interne Aufgabe sagt nur zu, wer dem Träger angehört.
//   - Befähigung: Verlangt die Aufgabe eine Einweisung, muss sie erteilt sein.
//
// Die Prüfung sitzt bewusst hier und nicht in der Oberfläche: Ein
// selbstgebauter Client soll niemanden an die Motorsense bringen.
func PruefeZusage(d *db.DB, z model.Zugriff, task model.CareTask) error {
	place, err := d.GetPlace(task.PlaceID)
	if err != nil {
		return &CompletionError{Status: http.StatusNotFound, Message: "Den Ort dieser Aufgabe gibt es nicht."}
	}
	traeger, err := d.GetTraeger(place.TraegerID)
	if err != nil {
		return &CompletionError{Status: http.StatusNotFound, Message: "Den Träger dieser Aufgabe gibt es nicht."}
	}
	if !z.SiehtAufgabe(*traeger, task.Sichtbarkeit) {
		// 404 statt 403: Wer sie nicht sehen darf, soll nicht einmal
		// erfahren, dass es sie gibt.
		return &CompletionError{Status: http.StatusNotFound, Message: "Diese Aufgabe gibt es nicht."}
	}
	if !z.DarfZusagen(*traeger, task.Sichtbarkeit) {
		return &CompletionError{Status: http.StatusServiceUnavailable,
			Message: "Die Rössing-ID ist gerade nicht erreichbar — interne Aufgaben lassen sich " +
				"erst wieder zusagen, wenn die Mitgliedschaft gesichert geprüft werden kann."}
	}
	if task.BefaehigungID == 0 {
		return nil
	}
	if d.HatBefaehigung(z.Sub, task.BefaehigungID) {
		return nil
	}
	name := "die nötige Einweisung"
	if b, err := d.GetBefaehigung(task.BefaehigungID); err == nil {
		name = zitat(b.Name)
	}
	return &CompletionError{Status: http.StatusForbidden,
		Message: "Für diese Aufgabe fehlt dir " + name +
			". Du kannst sie beim Träger beantragen — danach geht es sofort."}
}

// --- Sichtbarkeit von Orten und Aufgaben ------------------------------------

// Sichtbarkeitsfilter beantwortet für eine ganze Ortsliste, was angezeigt
// werden darf. Er lädt die Träger einmal statt je Ort.
type Sichtbarkeitsfilter struct {
	zugriff model.Zugriff
	traeger map[int64]model.Traeger
}

// NeuerFilter baut den Filter für diese Person.
func NeuerFilter(d *db.DB, z model.Zugriff) (Sichtbarkeitsfilter, error) {
	index, err := d.TraegerIndex()
	if err != nil {
		return Sichtbarkeitsfilter{}, err
	}
	return Sichtbarkeitsfilter{zugriff: z, traeger: index}, nil
}

// AufgabeSichtbar sagt, ob diese Aufgabe an diesem Ort erscheinen darf.
func (f Sichtbarkeitsfilter) AufgabeSichtbar(place model.Place, task model.CareTask) bool {
	t, ok := f.traeger[place.TraegerID]
	if !ok {
		// Ein Ort ohne gültigen Träger ist ein Datenfehler. Er wird
		// versteckt statt gezeigt — im Zweifel weniger, nie mehr.
		return f.zugriff.Betreiber
	}
	return f.zugriff.SiehtAufgabe(t, task.Sichtbarkeit)
}

// OrtSichtbar sagt, ob der Ort mit diesen (bereits gefilterten) Aufgaben
// überhaupt erscheinen darf.
func (f Sichtbarkeitsfilter) OrtSichtbar(place model.Place, alleAufgaben []model.CareTask) bool {
	t, ok := f.traeger[place.TraegerID]
	if !ok {
		return f.zugriff.Betreiber
	}
	sichten := make([]model.TaskSichtbarkeit, 0, len(alleAufgaben))
	for _, a := range alleAufgaben {
		sichten = append(sichten, a.Sichtbarkeit)
	}
	return f.zugriff.SiehtOrt(t, sichten)
}

// Traeger liefert den Träger eines Ortes aus dem Index.
func (f Sichtbarkeitsfilter) Traeger(place model.Place) (model.Traeger, bool) {
	t, ok := f.traeger[place.TraegerID]
	return t, ok
}

// TraegerNachID liefert einen Träger aus dem Index.
func (f Sichtbarkeitsfilter) TraegerNachID(id int64) (model.Traeger, bool) {
	t, ok := f.traeger[id]
	return t, ok
}

// --- Weitere Prüfungen ------------------------------------------------------

// zielTraeger liefert den Träger, unter dem etwas angelegt werden soll.
//
// Ohne Angabe wird der einzige genommen, den der Aufrufer verwaltet — im
// Dorfalltag hat fast jede und jeder genau einen Verein, und die App soll
// deswegen keine Auswahl zeigen müssen. Verwaltet er mehrere (oder als
// Betreiber alle), muss er sich entscheiden.
func (s *Server) zielTraeger(r *http.Request, traegerID int64) (model.Traeger, error) {
	z := s.zugriff(r)
	if traegerID != 0 {
		t, err := s.DB.GetTraeger(traegerID)
		if err != nil {
			return model.Traeger{}, &CompletionError{Status: http.StatusNotFound,
				Message: "Diesen Träger gibt es nicht."}
		}
		if !z.DarfVerwalten(*t) {
			return model.Traeger{}, verwaltungAbgelehnt(z, *t)
		}
		return *t, nil
	}
	// Der Betreiber verwaltet alle Träger — ihn jedes Mal auswählen zu
	// lassen, wäre lästig. Ohne Angabe gilt für ihn der Platzhalter:
	// Was die Plattform selbst einstellt, gehört dem Dorfentwicklungskreis,
	// bis eine Gruppe es übernimmt.
	if z.Betreiber {
		dek, err := s.DB.TraegerSicherstellen(model.SchluesselDEK, model.NameDEK)
		if err != nil {
			return model.Traeger{}, err
		}
		return *dek, nil
	}
	alle, err := s.DB.ListTraeger()
	if err != nil {
		return model.Traeger{}, err
	}
	var meine []model.Traeger
	for _, t := range alle {
		if z.DarfVerwalten(t) {
			meine = append(meine, t)
		}
	}
	switch len(meine) {
	case 0:
		if z.Veraltet {
			return model.Traeger{}, verwaltungAbgelehnt(z, model.Traeger{Name: "diesem Träger"})
		}
		return model.Traeger{}, &CompletionError{Status: http.StatusForbidden,
			Message: "Du verwaltest keinen Träger — Aufgaben stellen Vereine und Gruppen ein."}
	case 1:
		return meine[0], nil
	default:
		return model.Traeger{}, &CompletionError{Status: http.StatusBadRequest,
			Message: "Bitte angeben, für welchen Träger das gelten soll (traegerId)."}
	}
}

// pruefeBefaehigungGehoert stellt sicher, dass eine Aufgabe nur eine
// Einweisung ihres EIGENEN Trägers verlangen kann. Sonst könnte ein Verein
// eine Aufgabe hinter der Befähigung eines anderen verschließen und dessen
// Mitglieder faktisch aussperren.
func (s *Server) pruefeBefaehigungGehoert(befaehigungID, traegerID int64) error {
	if befaehigungID == 0 {
		return nil
	}
	b, err := s.DB.GetBefaehigung(befaehigungID)
	if err != nil {
		return errors.New("die angegebene Befähigung gibt es nicht")
	}
	if b.TraegerID != traegerID {
		return errors.New("eine Aufgabe kann nur eine Befähigung ihres eigenen Trägers verlangen")
	}
	return nil
}

// pruefeAufgabeSichtbar weist alles ab, was der Aufrufer nicht sehen darf —
// mit 404 statt 403, damit nicht einmal die Existenz durchsickert.
func (s *Server) pruefeAufgabeSichtbar(r *http.Request, taskID int64) error {
	task, err := s.DB.GetTask(taskID)
	if err != nil {
		return &CompletionError{Status: http.StatusNotFound, Message: "Diese Aufgabe gibt es nicht."}
	}
	return s.pruefeSichtbar(r, *task)
}

func (s *Server) pruefeSichtbar(r *http.Request, task model.CareTask) error {
	z := s.zugriff(r)
	place, err := s.DB.GetPlace(task.PlaceID)
	if err != nil {
		return &CompletionError{Status: http.StatusNotFound, Message: "Diese Aufgabe gibt es nicht."}
	}
	traeger, err := s.DB.GetTraeger(place.TraegerID)
	if err != nil {
		return &CompletionError{Status: http.StatusNotFound, Message: "Diese Aufgabe gibt es nicht."}
	}
	if !z.SiehtAufgabe(*traeger, task.Sichtbarkeit) {
		return &CompletionError{Status: http.StatusNotFound, Message: "Diese Aufgabe gibt es nicht."}
	}
	return nil
}

// pruefeOrtSichtbar weist Anfragen zu Orten ab, die es für den Aufrufer nicht
// gibt (Anmelden zum Mithelfen, Helferliste).
func (s *Server) pruefeOrtSichtbar(r *http.Request, placeID int64) error {
	z := s.zugriff(r)
	place, err := s.DB.GetPlace(placeID)
	if err != nil {
		return &CompletionError{Status: http.StatusNotFound, Message: "Diesen Ort gibt es nicht."}
	}
	filter, err := NeuerFilter(s.DB, z)
	if err != nil {
		return err
	}
	tasks, err := s.DB.ListTasks()
	if err != nil {
		return err
	}
	var seine []model.CareTask
	for _, t := range tasks {
		if t.PlaceID == placeID {
			seine = append(seine, t)
		}
	}
	if !filter.OrtSichtbar(*place, seine) {
		return &CompletionError{Status: http.StatusNotFound, Message: "Diesen Ort gibt es nicht."}
	}
	return nil
}

// wertOder löst einen optionalen Wert auf: fehlt er, gilt der bisherige.
// Gebraucht wird das dort, wo „nicht geschickt“ etwas anderes heißen muss
// als „auf null gesetzt“ (siehe TaskInput.BefaehigungID).
func wertOder(v *int64, bisher int64) int64 {
	if v == nil {
		return bisher
	}
	return *v
}
