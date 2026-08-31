package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/vergabe"
)

// Die verwaltenden Werkzeuge des Chats.
//
// Sie decken ab, was der Betreiber heute über den MCP-Endpunkt im Connector
// von claude.ai macht — Orte und Aufgaben ändern und löschen, eine Meldung
// zurücknehmen, die Vergabe ansehen, den Hitzefaktor stellen, die Ideen
// durchsehen. Wer nicht verwalten darf, kommt hier nicht durch: Über die
// Erlaubnis entscheidet in jedem einzelnen Fall model.Zugriff, dieselbe
// Stelle wie in REST, Karte, Rangliste und Push.
//
// Der Unterschied zum MCP-Endpunkt ist nicht der Umfang, sondern die Sicht:
// Dort sitzt ausschließlich der Betreiber (der Endpunkt verlangt die globale
// admin-Rolle), hier sitzt irgendwer aus dem Dorf.

func verwaltungsWerkzeuge() []Werkzeug {
	staende := make([]string, 0, len(model.IdeeStatusWerte))
	for _, st := range model.IdeeStatusWerte {
		staende = append(staende, string(st))
	}
	return []Werkzeug{
		{
			Name: "ort_aendern",
			Beschreibung: "Ändert einen Pflege-Ort. Nur die angegebenen Felder ändern sich; " +
				"alles andere bleibt. Mit aktiv=false wird der Ort stillgelegt — wer für " +
				"eine seiner Aufgaben zugesagt hat, bekommt einen Hinweis.",
			Schema: schemaObjekt([]string{"id"}, map[string]any{
				"id":           schemaGanzzahl("ID des Ortes (siehe orte_liste)"),
				"name":         schemaText("Neuer Anzeigename"),
				"art":          schemaAuswahl("Art des Ortes", "blumenkasten", "beet", "sonstiges"),
				"lat":          schemaZahl("Breitengrad"),
				"lon":          schemaZahl("Längengrad"),
				"beschreibung": schemaText("Beschreibung"),
				"aktiv":        schemaJaNein("Ort aktiv? Stillgelegte Orte erzeugen keine Anfragen mehr."),
				"traegerId":    schemaGanzzahl("Ort einem anderen Träger übergeben (nur an einen, den du auch verwaltest)"),
			}),
			Aendert: true,
			Handler: werkzeugOrtAendern,
		},
		{
			Name: "ort_loeschen",
			Beschreibung: "Löscht einen Pflege-Ort samt seinen Aufgaben und deren Historie. " +
				"Nicht umkehrbar — vorher nachfragen. Wer zugesagt hatte, bekommt einen Hinweis.",
			Schema: schemaObjekt([]string{"id"}, map[string]any{
				"id": schemaGanzzahl("ID des Ortes (siehe orte_liste)"),
			}),
			Aendert: true,
			Handler: werkzeugOrtLoeschen,
		},
		{
			Name: "aufgabe_aendern",
			Beschreibung: "Ändert eine Pflegeaufgabe. Nur die angegebenen Felder ändern sich. " +
				"Mit aktiv=false wird sie pausiert; wer sie zugesagt hat, bekommt einen Hinweis. " +
				"Eine regelmäßige Aufgabe wird mit einmalig=true und termin zu einer einmaligen " +
				"und umgekehrt mit einmalig=false plus intervallTage und rotNachTagen zurück.",
			Schema: schemaObjekt([]string{"id"}, map[string]any{
				"id":            schemaGanzzahl("ID der Aufgabe (siehe orte_liste)"),
				"art":           schemaAuswahl("Art der Aufgabe", "giessen", "jaeten", "sonstiges"),
				"titel":         schemaText("Titel, vor allem bei art=sonstiges"),
				"liter":         schemaZahl("Wassermenge je Gießvorgang (nur giessen)"),
				"intervallTage": schemaZahl("Nur regelmäßig: Soll-Intervall in Tagen; danach wird die Aufgabe gelb"),
				"rotNachTagen":  schemaZahl("Nur regelmäßig: nach so vielen Tagen ohne Erledigung wird sie rot"),
				"einmalig":      schemaJaNein("Auf einmalig umstellen (braucht einen Termin)"),
				"termin":        schemaText("Nur einmalig: Fälligkeitsdatum (2026-08-20 oder RFC3339)"),
				"entfernenWennErledigt": schemaJaNein("Nach dem Erledigen von Karte und Liste nehmen " +
					"(die Erledigung zählt weiter für die Rangliste)"),
				"aktiv": schemaJaNein("Aufgabe aktiv?"),
				"sichtbarkeit": schemaAuswahl("Wer sieht die Aufgabe", "oeffentlich",
					"nur_mitglieder"),
				"saisonVon": schemaGanzzahl("Erster Monat der Jahreszeit (1–12); 0 nimmt sie weg"),
				"saisonBis": schemaGanzzahl("Letzter Monat der Jahreszeit (1–12); 0 nimmt sie weg"),
			}),
			Aendert: true,
			Handler: werkzeugAufgabeAendern,
		},
		{
			Name: "aufgabe_loeschen",
			Beschreibung: "Löscht eine Pflegeaufgabe samt Historie. Nicht umkehrbar — vorher " +
				"nachfragen. Wer sie gerade zugesagt hat, bekommt einen Hinweis.",
			Schema: schemaObjekt([]string{"id"}, map[string]any{
				"id": schemaGanzzahl("ID der Aufgabe (siehe orte_liste)"),
			}),
			Aendert: true,
			Handler: werkzeugAufgabeLoeschen,
		},
		{
			Name: "erledigung_zuruecknehmen",
			Beschreibung: "Nimmt eine irrtümlich gemeldete Erledigung zurück (versehentlich " +
				"angetippt). Die Ampel rechnet danach neu. Eigene Meldungen kann jede Person " +
				"zurücknehmen, fremde nur die Verwaltung. Die IDs stehen in der Historie.",
			Schema: schemaObjekt([]string{"id"}, map[string]any{
				"id": schemaGanzzahl("ID der Erledigungs-Meldung (siehe historie)"),
			}),
			Aendert: true,
			Handler: werkzeugErledigungZuruecknehmen,
		},
		{
			Name: "vergabe_stand",
			Beschreibung: "Zeigt der Verwaltung, wie die Vergabe einer Pflegeaufgabe steht: wer " +
				"für den Ort angemeldet ist, wer wann gefragt wurde, wer zugesagt hat und bis " +
				"wann. Enthält Namen — deshalb nur für die Verwaltenden des Trägers.",
			Schema: schemaObjekt([]string{"aufgabeId"}, map[string]any{
				"aufgabeId": schemaGanzzahl("ID der Aufgabe (siehe orte_liste)"),
			}),
			Handler: werkzeugVergabeStand,
		},
		{
			Name: "zusage_aufheben",
			Beschreibung: "Hebt die Zusage zu einer Pflegeaufgabe auf (z.B. wenn jemand krank " +
				"geworden ist). Die betroffene Person bekommt einen Hinweis, und die " +
				"Warteschlange fragt sofort weiter.",
			Schema: schemaObjekt([]string{"aufgabeId"}, map[string]any{
				"aufgabeId": schemaGanzzahl("ID der Aufgabe (siehe orte_liste)"),
			}),
			Aendert: true,
			Handler: werkzeugZusageAufheben,
		},
		{
			Name: "hitzefaktor_setzen",
			Beschreibung: "Setzt den dorfweiten Hitze-Faktor für Gieß-Aufgaben. 1.0 = normal, " +
				"0.5 = Hitzewelle (die Kästen werden doppelt so schnell gelb und rot). " +
				"Bereich 0 bis 4. Gilt für das ganze Dorf, deshalb nur für den Betreiber.",
			Schema: schemaObjekt([]string{"faktor"}, map[string]any{
				"faktor": schemaZahl("Faktor, z.B. 0.5 bei Hitze"),
			}),
			Aendert: true,
			Handler: werkzeugHitzefaktor,
		},
		{
			Name: "ideen_liste",
			Beschreibung: "Listet die Ideen aus dem Dorf („Was soll die App können?“) mit Datum, " +
				"Name, Wunsch, Weg (website/app), Stand und interner Notiz — neueste zuerst. " +
				"Enthält Kontaktdaten der Einreichenden, deshalb nur für den Betreiber.",
			Schema: schemaObjekt(nil, map[string]any{
				"status": schemaAuswahl("Nur Ideen mit diesem Stand (Vorgabe: alle)", staende...),
				"anzahl": schemaGanzzahl("Höchstens so viele Einträge (Vorgabe: alle)"),
			}),
			Handler: werkzeugIdeenListe,
		},
		{
			Name: "idee_status_setzen",
			Beschreibung: "Setzt den Stand einer Idee (" + strings.Join(staende, ", ") + ") und " +
				"optional die interne Notiz. Der eingereichte Wunsch selbst bleibt unverändert. " +
				"Nur für den Betreiber.",
			Schema: schemaObjekt([]string{"id", "status"}, map[string]any{
				"id":     schemaGanzzahl("ID der Idee (siehe ideen_liste)"),
				"status": schemaAuswahl("Neuer Stand", staende...),
				"notiz":  schemaText("Interne Bemerkung (ersetzt die bisherige)"),
			}),
			Aendert: true,
			Handler: werkzeugIdeeStatus,
		},
	}
}

// --- Orte -------------------------------------------------------------------

func werkzeugOrtAendern(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		ID           int64    `json:"id"`
		Name         *string  `json:"name"`
		Art          *string  `json:"art"`
		Lat          *float64 `json:"lat"`
		Lon          *float64 `json:"lon"`
		Beschreibung *string  `json:"beschreibung"`
		Aktiv        *bool    `json:"aktiv"`
		TraegerID    int64    `json:"traegerId"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	ort, _, err := verwaltbarerOrt(s, in.ID)
	if err != nil {
		return nil, err
	}
	// Ein Ort lässt sich nur dorthin verschieben, wo man ebenfalls verwaltet
	// — sonst könnte ein Verein dem anderen Arbeit unterschieben.
	if in.TraegerID != 0 && in.TraegerID != ort.TraegerID {
		if _, err := zielTraeger(s, in.TraegerID); err != nil {
			return nil, err
		}
	}
	// Der bestehende Stand wird zur Eingabe gemacht und nur dort
	// überschrieben, wo etwas mitgeschickt wurde. So läuft die Änderung
	// durch dieselbe Prüfung wie das Anlegen, statt neben ihr her.
	eingabe := api.PlaceInput{Name: ort.Name, Description: ort.Description,
		Kind: string(ort.Kind), Lat: ort.Lat, Lon: ort.Lon, TraegerID: in.TraegerID}
	uebernimm(&eingabe.Name, in.Name)
	uebernimm(&eingabe.Description, in.Beschreibung)
	uebernimm(&eingabe.Kind, in.Art)
	uebernimm(&eingabe.Lat, in.Lat)
	uebernimm(&eingabe.Lon, in.Lon)
	eingabe.Active = in.Aktiv
	if err := eingabe.Validate(); err != nil {
		return nil, err
	}
	vorher := ort
	eingabe.Apply(&ort)
	if err := s.DB.UpdatePlace(&ort); err != nil {
		return nil, err
	}
	// Stillgelegt heißt: Hier ist bis auf Weiteres nichts zu tun. Wer für
	// eine Aufgabe dieses Ortes zugesagt hat, erfährt das.
	if api.OrtWirdPausiert(vorher, ort) {
		api.OrtEntfaellt(s.DB, s.Now, s.Zusteller, ort.ID)
	}
	return ort, nil
}

func werkzeugOrtLoeschen(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		ID int64 `json:"id"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	ort, _, err := verwaltbarerOrt(s, in.ID)
	if err != nil {
		return nil, err
	}
	// Erst Bescheid sagen, dann löschen: Danach ist der Vorgang mitsamt
	// seinem Anlass verschwunden.
	api.OrtEntfaellt(s.DB, s.Now, s.Zusteller, ort.ID)
	if err := s.DB.DeletePlace(ort.ID); err != nil {
		return nil, err
	}
	return map[string]any{"geloescht": ort.ID, "name": ort.Name}, nil
}

// --- Aufgaben ---------------------------------------------------------------

func werkzeugAufgabeAendern(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		ID                    int64    `json:"id"`
		Art                   *string  `json:"art"`
		Titel                 *string  `json:"titel"`
		Liter                 *float64 `json:"liter"`
		IntervallTage         *float64 `json:"intervallTage"`
		RotNachTagen          *float64 `json:"rotNachTagen"`
		Einmalig              *bool    `json:"einmalig"`
		Termin                *string  `json:"termin"`
		EntfernenWennErledigt *bool    `json:"entfernenWennErledigt"`
		Aktiv                 *bool    `json:"aktiv"`
		Sichtbarkeit          *string  `json:"sichtbarkeit"`
		SaisonVon             *int     `json:"saisonVon"`
		SaisonBis             *int     `json:"saisonBis"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	aufgabe, err := verwaltbareAufgabe(s, in.ID)
	if err != nil {
		return nil, err
	}
	eingabe := eingabeAus(aufgabe)
	uebernimm(&eingabe.Kind, in.Art)
	uebernimm(&eingabe.Title, in.Titel)
	uebernimm(&eingabe.IntervalDays, in.IntervallTage)
	uebernimm(&eingabe.RedAfterDays, in.RotNachTagen)
	uebernimm(&eingabe.OneOff, in.Einmalig)
	uebernimm(&eingabe.DueDate, in.Termin)
	uebernimm(&eingabe.RemoveWhenDone, in.EntfernenWennErledigt)
	uebernimm(&eingabe.Sichtbarkeit, in.Sichtbarkeit)
	if in.SaisonVon != nil {
		eingabe.SeasonStartMonth = in.SaisonVon
	}
	if in.SaisonBis != nil {
		eingabe.SeasonEndMonth = in.SaisonBis
	}
	if in.Liter != nil {
		eingabe.Liters = in.Liter
	}
	eingabe.Active = in.Aktiv
	// Wer auf „einmalig“ umstellt, ohne einen Termin zu nennen, bekommt eine
	// verständliche Absage statt des alten Intervalls als Termin.
	if !eingabe.OneOff {
		eingabe.DueDate = ""
	}
	if err := eingabe.Validate(); err != nil {
		return nil, err
	}
	vorher := aufgabe
	eingabe.Apply(&aufgabe)
	if err := s.DB.UpdateTask(&aufgabe); err != nil {
		return nil, err
	}
	// Pausieren ist für die Zusagenden dasselbe wie Löschen: Sie müssen
	// nicht mehr los und sollen das erfahren.
	if api.WirdPausiert(vorher, aufgabe) {
		api.AufgabeEntfaellt(s.DB, s.Now, s.Zusteller, aufgabe.ID)
	}
	return aufgabe, nil
}

func werkzeugAufgabeLoeschen(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		ID int64 `json:"id"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	aufgabe, err := verwaltbareAufgabe(s, in.ID)
	if err != nil {
		return nil, err
	}
	api.AufgabeEntfaellt(s.DB, s.Now, s.Zusteller, aufgabe.ID)
	if err := s.DB.DeleteTask(aufgabe.ID); err != nil {
		return nil, err
	}
	return map[string]any{"geloescht": aufgabe.ID, "name": aufgabe.DisplayName()}, nil
}

// --- Erledigungen -----------------------------------------------------------

// werkzeugErledigungZuruecknehmen nimmt eine Meldung zurück — dieselbe Regel
// wie in der REST-API: eigene immer, fremde nur mit Verwaltungsrecht.
func werkzeugErledigungZuruecknehmen(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		ID int64 `json:"id"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	nichtGefunden := fmt.Errorf("Diese Meldung gibt es nicht (%d).", in.ID)
	meldung, err := s.DB.GetCompletion(in.ID)
	if err != nil {
		return nil, nichtGefunden
	}
	// Zu einer Aufgabe, die es für mich nicht gibt, gibt es auch keine
	// Meldung — sonst unterschiede die Absage vom „gibt es nicht“ und
	// verriete die Existenz der internen Aufgabe.
	aufgabe, err := sichtbareAufgabe(s, meldung.TaskID)
	if err != nil {
		return nil, nichtGefunden
	}
	if meldung.UserSub != s.Nutzer.Sub {
		if _, err := verwaltbareAufgabe(s, aufgabe.ID); err != nil {
			return nil, errors.New("Zurücknehmen kann das nur, wer sie gemeldet hat — " +
				"oder die Verwaltung des Trägers.")
		}
	}
	if err := s.DB.DeleteCompletion(in.ID); err != nil {
		return nil, nichtGefunden
	}
	return map[string]any{"zurueckgenommen": in.ID, "aufgabeId": meldung.TaskID}, nil
}

// --- Vergabe ----------------------------------------------------------------

// vergabeEngine liefert die Vergabe mit der Zeit und dem Zusteller dieser
// Sitzung.
func vergabeEngine(s Sitzung) *vergabe.Engine {
	return vergabe.New(s.DB, vergabe.Config{
		Now:       func() time.Time { return s.Now },
		Zusteller: s.Zusteller,
	})
}

func werkzeugVergabeStand(args json.RawMessage, s Sitzung) (any, error) {
	aufgabe, err := aufgabeAusArgumenten(args, s)
	if err != nil {
		return nil, err
	}
	return vergabeEngine(s).Stand(aufgabe.ID)
}

func werkzeugZusageAufheben(args json.RawMessage, s Sitzung) (any, error) {
	aufgabe, err := aufgabeAusArgumenten(args, s)
	if err != nil {
		return nil, err
	}
	e := vergabeEngine(s)
	stand, err := e.Stand(aufgabe.ID)
	if err != nil {
		return nil, err
	}
	if stand.Vorgang == nil || stand.Vorgang.ClaimedBy == "" {
		return nil, errors.New("Für diese Aufgabe liegt gerade keine Zusage vor, " +
			"die sich aufheben ließe.")
	}
	// istAdmin=true: Wer hier ankommt, hat die Verwaltungsprüfung schon
	// bestanden — er gibt eine fremde Zusage zurück, nicht seine eigene.
	return e.Zurueckgeben(stand.Vorgang.ID, s.Nutzer.Sub, true)
}

// aufgabeAusArgumenten liest die Aufgaben-ID und prüft das Verwaltungsrecht.
// Beide Vergabe-Werkzeuge zeigen bzw. ändern Namen von Nachbarn; das ist
// Sache der Verwaltenden des Trägers, nicht des ganzen Dorfes.
func aufgabeAusArgumenten(args json.RawMessage, s Sitzung) (model.CareTask, error) {
	var in struct {
		AufgabeID int64 `json:"aufgabeId"`
	}
	if err := entpacke(args, &in); err != nil {
		return model.CareTask{}, err
	}
	return verwaltbareAufgabe(s, in.AufgabeID)
}

// --- Dorfweite Einstellungen und Ideen --------------------------------------

func werkzeugHitzefaktor(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		Faktor float64 `json:"faktor"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	if err := nurBetreiber(s, "Den Hitze-Faktor stellt der Betreiber der App — er gilt fürs "+
		"ganze Dorf und nicht nur für einen Verein."); err != nil {
		return nil, err
	}
	if in.Faktor <= 0 || in.Faktor > 4 {
		return nil, errors.New("Der Faktor muss zwischen 0 und 4 liegen.")
	}
	if err := s.DB.SetWateringFactor(in.Faktor); err != nil {
		return nil, err
	}
	return map[string]any{"giessfaktor": in.Faktor}, nil
}

func werkzeugIdeenListe(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		Status string `json:"status"`
		Anzahl int    `json:"anzahl"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	if err := nurBetreiber(s, ideenAbsage); err != nil {
		return nil, err
	}
	status, err := api.IdeeStatusAus(in.Status)
	if err != nil {
		return nil, err
	}
	ideen, err := s.DB.ListIdeen(status)
	if err != nil {
		return nil, err
	}
	if in.Anzahl > 0 && in.Anzahl < len(ideen) {
		ideen = ideen[:in.Anzahl]
	}
	return map[string]any{"ideen": ideen}, nil
}

func werkzeugIdeeStatus(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		ID     int64   `json:"id"`
		Status string  `json:"status"`
		Notiz  *string `json:"notiz"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	if err := nurBetreiber(s, ideenAbsage); err != nil {
		return nil, err
	}
	status, err := api.IdeeStatusAus(in.Status)
	if err != nil {
		return nil, err
	}
	if status == "" {
		return nil, errors.New("Welcher Stand denn?")
	}
	idee, err := s.DB.GetIdee(in.ID)
	if err != nil {
		return nil, fmt.Errorf("Diese Idee gibt es nicht (%d).", in.ID)
	}
	idee.Status = status
	uebernimm(&idee.Notiz, in.Notiz)
	if err := s.DB.UpdateIdee(idee); err != nil {
		return nil, err
	}
	return idee, nil
}

const ideenAbsage = "Die Ideen aus dem Dorf sieht der Betreiber der App — " +
	"in ihnen stehen Namen und Kontaktdaten der Einreichenden."

// --- Gemeinsame Prüfungen ---------------------------------------------------

// nurBetreiber sperrt Werkzeuge, die nicht einem Träger gehören, sondern der
// ganzen Plattform. Die Rolle kommt aus dem Token und hängt nicht an Zitadels
// Erreichbarkeit — deshalb hier kein Sonderfall für „veraltet“.
func nurBetreiber(s Sitzung, absage string) error {
	if s.Zugriff.Betreiber {
		return nil
	}
	return errors.New(absage)
}

// sichtbarerOrt lädt einen Ort, sofern er für diese Person überhaupt
// existiert — über denselben Filter wie Liste und Karte.
func sichtbarerOrt(s Sitzung, ortID int64) (model.Place, error) {
	nichtGefunden := fmt.Errorf("Diesen Ort gibt es nicht (%d).", ortID)
	ort, err := s.DB.GetPlace(ortID)
	if err != nil {
		return model.Place{}, nichtGefunden
	}
	filter, err := api.NeuerFilter(s.DB, s.Zugriff)
	if err != nil {
		return model.Place{}, err
	}
	alle, err := s.DB.ListTasks()
	if err != nil {
		return model.Place{}, err
	}
	seine := []model.CareTask{}
	for _, a := range alle {
		if a.PlaceID == ort.ID {
			seine = append(seine, a)
		}
	}
	if !filter.OrtSichtbar(*ort, seine) {
		return model.Place{}, nichtGefunden
	}
	return *ort, nil
}

// verwaltbarerOrt lädt einen Ort und prüft, ob diese Person ihn pflegen darf.
//
// Die Reihenfolge ist Absicht: Erst „gibt es das für mich?“, dann „darf ich
// das?“. Andersherum verriete die Absage, dass es dort etwas gibt.
func verwaltbarerOrt(s Sitzung, ortID int64) (model.Place, model.Traeger, error) {
	ort, err := sichtbarerOrt(s, ortID)
	if err != nil {
		return model.Place{}, model.Traeger{}, err
	}
	traeger, err := s.DB.GetTraeger(ort.TraegerID)
	if err != nil {
		return model.Place{}, model.Traeger{}, fmt.Errorf("Diesen Ort gibt es nicht (%d).", ortID)
	}
	if !s.Zugriff.DarfVerwalten(*traeger) {
		return model.Place{}, model.Traeger{}, abgelehnt(s, *traeger)
	}
	return ort, *traeger, nil
}

// verwaltbareAufgabe lädt eine Aufgabe und prüft dasselbe an ihrem Ort.
func verwaltbareAufgabe(s Sitzung, aufgabeID int64) (model.CareTask, error) {
	aufgabe, err := sichtbareAufgabe(s, aufgabeID)
	if err != nil {
		return model.CareTask{}, err
	}
	if _, _, err := verwaltbarerOrt(s, aufgabe.PlaceID); err != nil {
		return model.CareTask{}, err
	}
	return aufgabe, nil
}

// --- Kleine Helfer ----------------------------------------------------------

// uebernimm setzt den Wert, wenn er mitgeschickt wurde. Fehlt das Feld,
// bleibt der bisherige Stand — eine Änderung soll nur ändern, wovon die Rede
// war.
func uebernimm[T any](ziel *T, wert *T) {
	if wert != nil {
		*ziel = *wert
	}
}

// eingabeAus macht aus dem bestehenden Stand einer Aufgabe wieder eine
// Eingabe. Damit läuft jede Änderung durch api.TaskInput.Validate — dieselbe
// Prüfung wie beim Anlegen über App und Web-Verwaltung.
func eingabeAus(t model.CareTask) api.TaskInput {
	in := api.TaskInput{
		Kind:           string(t.Kind),
		Title:          t.Title,
		Liters:         t.Liters,
		IntervalDays:   t.IntervalDays,
		RedAfterDays:   t.RedAfterDays,
		OneOff:         t.OneOff,
		RemoveWhenDone: t.RemoveWhenDone,
		Sichtbarkeit:   string(t.Sichtbarkeit),
		// Die Jahreszeit wird mitgeführt, damit eine Änderung an etwas
		// anderem sie nicht stillschweigend abräumt.
		SeasonStartMonth: &t.SeasonStartMonth,
		SeasonEndMonth:   &t.SeasonEndMonth,
	}
	if t.OneOff && t.DueDate != nil {
		in.DueDate = t.DueDate.Format(time.RFC3339)
	}
	return in
}
