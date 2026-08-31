package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Die Werkzeuge des Chats.
//
// # Die eine Tür
//
// Jedes Werkzeug arbeitet auf der Sicht der fragenden Person und fragt dafür
// model.Zugriff — dieselbe Stelle, die auch Liste, Karte, Historie, Rangliste
// und Push befragen. Hier steht KEINE zweite Sichtbarkeitsprüfung: Was
// „nur_mitglieder" heißt, ist auch im Chat nirgends, und was jemand nicht
// verwalten darf, kann er auch im Gespräch nicht verwalten.
//
// Deshalb rufen die lesenden Werkzeuge dieselben Zusammenbauer wie die
// REST-API auf (api.AssemblePlacesFuer, api.AssembleLeaderboardFuer,
// api.NeuerFilter) statt die Datenbank selbst zu befragen. Eine zweite,
// eigene Abfrage wäre genau die Stelle, an der beim nächsten Sonderfall die
// interne Aufgabe herausrutscht.
//
// # Warum eigene Werkzeuge und nicht die des MCP-Endpunkts
//
// Die Werkzeuge unter internal/mcp arbeiten in der Sicht des BETREIBERS: Der
// Endpunkt verlangt ohnehin die globale admin-Rolle, deshalb listen sie alles
// und prüfen keinen Träger. Im Chat sitzt dagegen irgendwer aus dem Dorf.
// Dieselben Werkzeuge mitzubenutzen hieße, ihnen die Trägerprüfung
// nachzurüsten — und jeder Aufruf des MCP-Endpunkts müsste dann einen
// Betreiber-Zugriff mitgeben, der dort nur Zierde wäre. Die Regel selbst
// bleibt so oder so einfach: Sie steht in model.Zugriff.

// Sitzung ist alles, was ein Werkzeugaufruf über den Aufrufer weiß.
type Sitzung struct {
	DB      *db.DB
	Now     time.Time
	Zugriff model.Zugriff
	Nutzer  auth.User
}

// Werkzeug ist ein Werkzeug des Chats.
type Werkzeug struct {
	Name         string
	Beschreibung string
	Schema       map[string]any
	// Aendert markiert Werkzeuge, die schreiben. Nur für das Protokoll der
	// Antwort — die Erlaubnis entscheidet immer model.Zugriff.
	Aendert bool
	Handler func(args json.RawMessage, s Sitzung) (any, error)
}

// Werkzeuge liefert den vollständigen Satz.
//
// Bewusst NICHT dabei: alles zum Verleih von Geräten. Die Mietplattform ist
// ein eigener Dienst unter mieten.xn--rssing-wxa.de mit eigenen Werkzeugen;
// hier gäbe es davon eine zweite, schlechtere Fassung.
func Werkzeuge() []Werkzeug {
	return []Werkzeug{
		{
			Name: "orte_liste",
			Beschreibung: "Listet die Pflege-Orte des Dorfes (Blumenkästen, Beete, …) mit ihren " +
				"Aufgaben, der letzten Erledigung und dem Ampel-Status (green/yellow/red). " +
				"Enthält nur, was die fragende Person sehen darf. Erste Anlaufstelle für " +
				"Fragen wie „was steht gerade an?“ oder „wo muss gegossen werden?“.",
			Schema:  schemaObjekt(nil, map[string]any{}),
			Handler: werkzeugOrte,
		},
		{
			Name: "historie",
			Beschreibung: "Die letzten Erledigungen einer Pflegeaufgabe: wer wann wie viel " +
				"gemacht hat. Beantwortet „wer hat den Kasten zuletzt gegossen?“. " +
				"Die Aufgaben-IDs stehen in orte_liste.",
			Schema: schemaObjekt([]string{"aufgabeId"}, map[string]any{
				"aufgabeId": schemaGanzzahl("ID der Pflegeaufgabe"),
				"anzahl":    schemaGanzzahl("Wie viele Meldungen (Vorgabe 10, höchstens 50)"),
			}),
			Handler: werkzeugHistorie,
		},
		{
			Name: "rangliste",
			Beschreibung: "Rangliste des Mithelfens: wer hat im Zeitraum wie viele Erledigungen " +
				"und wie viele Liter geschafft, samt Gesamtsummen des Dorfes.",
			Schema: schemaObjekt(nil, map[string]any{
				"zeitraum": schemaAuswahl("Zeitraum (Vorgabe: saison = 1. März bis 31. Oktober)",
					"woche", "monat", "saison", "jahr", "gesamt"),
				"anzahl": schemaGanzzahl("Wie viele Plätze (Vorgabe 25)"),
			}),
			Handler: werkzeugRangliste,
		},
		{
			Name: "traeger_liste",
			Beschreibung: "Die Träger (Vereine und Gruppen), denen Orte und Aufgaben gehören — " +
				"und ob die fragende Person sie verwalten darf. Vor dem Anlegen eines Ortes " +
				"aufrufen, wenn die Zugehörigkeit unklar ist.",
			Schema:  schemaObjekt(nil, map[string]any{}),
			Handler: werkzeugTraeger,
		},
		{
			Name: "ort_anlegen",
			Beschreibung: "Legt einen neuen Pflege-Ort an (Blumenkasten, Beet). Koordinaten in " +
				"WGS84; Rössing liegt bei ungefähr lat 52.211, lon 9.870. Ohne traegerId " +
				"wird der Träger genommen, den die Person als einzigen verwaltet. " +
				"Aufgaben kommen danach mit aufgabe_anlegen dazu.",
			Schema: schemaObjekt([]string{"name", "lat", "lon"}, map[string]any{
				"name":         schemaText("Anzeigename, z.B. „Unter den Eichen — Kasten 1“"),
				"art":          schemaAuswahl("Art des Ortes (Vorgabe: blumenkasten)", "blumenkasten", "beet", "sonstiges"),
				"lat":          schemaZahl("Breitengrad"),
				"lon":          schemaZahl("Längengrad"),
				"beschreibung": schemaText("Optionale Beschreibung"),
				"traegerId":    schemaGanzzahl("ID des Trägers (siehe traeger_liste)"),
			}),
			Aendert: true,
			Handler: werkzeugOrtAnlegen,
		},
		{
			Name: "aufgabe_anlegen",
			Beschreibung: "Legt eine Pflegeaufgabe an einem Ort an — entweder REGELMÄSSIG " +
				"(art=giessen, liter=10, intervallTage=7, rotNachTagen=14) oder EINMALIG mit " +
				"Termin statt Intervall (einmalig=true, termin=2026-08-20). Beides zusammen " +
				"geht nicht.",
			Schema: schemaObjekt([]string{"ortId", "art"}, map[string]any{
				"ortId":         schemaGanzzahl("ID des Ortes (siehe orte_liste)"),
				"art":           schemaAuswahl("Art der Aufgabe", "giessen", "jaeten", "sonstiges"),
				"titel":         schemaText("Optionaler Titel, vor allem bei art=sonstiges"),
				"liter":         schemaZahl("Wassermenge je Gießvorgang (nur giessen)"),
				"intervallTage": schemaZahl("Nur regelmäßig: Soll-Intervall in Tagen; danach wird die Aufgabe gelb"),
				"rotNachTagen":  schemaZahl("Nur regelmäßig: nach so vielen Tagen ohne Erledigung wird sie rot"),
				"einmalig":      schemaJaNein("Einmalige Aufgabe statt eines wiederkehrenden Plans"),
				"termin":        schemaText("Nur einmalig: Fälligkeitsdatum (2026-08-20 oder RFC3339)"),
				"sichtbarkeit": schemaAuswahl("Wer sieht die Aufgabe (Vorgabe: oeffentlich)",
					"oeffentlich", "nur_mitglieder"),
			}),
			Aendert: true,
			Handler: werkzeugAufgabeAnlegen,
		},
		{
			Name: "erledigung_melden",
			Beschreibung: "Meldet eine Pflegeaufgabe als erledigt — im Namen der fragenden " +
				"Person. Nur nach ausdrücklicher Aufforderung benutzen, nie auf Verdacht.",
			Schema: schemaObjekt([]string{"aufgabeId"}, map[string]any{
				"aufgabeId": schemaGanzzahl("ID der Pflegeaufgabe"),
				"liter":     schemaZahl("Tatsächlich gegossene Liter (optional)"),
				"notiz":     schemaText("Optionale Notiz"),
			}),
			Aendert: true,
			Handler: werkzeugErledigungMelden,
		},
	}
}

// Beschreibungen liefert die Werkzeuge in der Form, die die API erwartet.
func Beschreibungen(werkzeuge []Werkzeug) []Werkzeugbeschreibung {
	out := make([]Werkzeugbeschreibung, 0, len(werkzeuge))
	for _, w := range werkzeuge {
		out = append(out, Werkzeugbeschreibung{
			Name: w.Name, Description: w.Beschreibung, InputSchema: w.Schema,
		})
	}
	return out
}

// --- Lesende Werkzeuge ------------------------------------------------------

func werkzeugOrte(_ json.RawMessage, s Sitzung) (any, error) {
	// AssemblePlacesFuer filtert bereits: interne Aufgaben fallen heraus, und
	// ein Ort, an dem danach nichts Sichtbares bleibt, verschwindet mit.
	orte, giessfaktor, err := api.AssemblePlacesFuer(s.DB, s.Now, s.Zugriff)
	if err != nil {
		return nil, err
	}
	return map[string]any{"orte": orte, "giessfaktor": giessfaktor}, nil
}

func werkzeugHistorie(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		AufgabeID int64 `json:"aufgabeId"`
		Anzahl    int   `json:"anzahl"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	if _, err := sichtbareAufgabe(s, in.AufgabeID); err != nil {
		return nil, err
	}
	anzahl := in.Anzahl
	if anzahl <= 0 {
		anzahl = 10
	}
	if anzahl > 50 {
		anzahl = 50
	}
	meldungen, err := s.DB.ListCompletions(in.AufgabeID, anzahl)
	if err != nil {
		return nil, err
	}
	// Namen kommen aus den Profilen und nicht aus dem, was beim Melden
	// eingefroren wurde — genau wie in der REST-API.
	namen, err := s.DB.NameResolver()
	if err != nil {
		return nil, err
	}
	for i := range meldungen {
		meldungen[i].UserName = namen.Resolve(meldungen[i].UserSub, meldungen[i].UserName)
	}
	return map[string]any{"erledigungen": meldungen}, nil
}

func werkzeugRangliste(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		Zeitraum string `json:"zeitraum"`
		Anzahl   int    `json:"anzahl"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	zeitraum, err := model.ParsePeriod(in.Zeitraum)
	if err != nil {
		return nil, err
	}
	// Die Sicht filtert schon in SQL: Erledigungen an internen Aufgaben
	// fließen für Außenstehende weder in die Zeilen noch in die Summen ein.
	sicht, err := api.SichtVon(s.DB, s.Zugriff)
	if err != nil {
		return nil, err
	}
	return api.AssembleLeaderboardFuer(s.DB, s.Now, zeitraum, in.Anzahl, s.Nutzer, sicht)
}

func werkzeugTraeger(_ json.RawMessage, s Sitzung) (any, error) {
	alle, err := s.DB.ListTraeger()
	if err != nil {
		return nil, err
	}
	type zeile struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		Beschreibung  string `json:"beschreibung,omitempty"`
		DarfVerwalten bool   `json:"darfVerwalten"`
	}
	out := []zeile{}
	for _, t := range alle {
		if !s.Zugriff.SiehtTraeger(t) {
			continue
		}
		out = append(out, zeile{ID: t.ID, Name: t.Name, Beschreibung: t.Beschreibung,
			DarfVerwalten: s.Zugriff.DarfVerwalten(t)})
	}
	return map[string]any{"traeger": out}, nil
}

// --- Ändernde Werkzeuge -----------------------------------------------------

func werkzeugOrtAnlegen(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		Name         string  `json:"name"`
		Art          string  `json:"art"`
		Lat          float64 `json:"lat"`
		Lon          float64 `json:"lon"`
		Beschreibung string  `json:"beschreibung"`
		TraegerID    int64   `json:"traegerId"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	traeger, err := zielTraeger(s, in.TraegerID)
	if err != nil {
		return nil, err
	}
	eingabe := api.PlaceInput{Name: in.Name, Description: in.Beschreibung, Kind: in.Art,
		Lat: in.Lat, Lon: in.Lon}
	if err := eingabe.Validate(); err != nil {
		return nil, err
	}
	ort := model.Place{Active: true, CreatedAt: s.Now, TraegerID: traeger.ID}
	eingabe.Apply(&ort)
	if err := s.DB.InsertPlace(&ort); err != nil {
		return nil, err
	}
	return ort, nil
}

func werkzeugAufgabeAnlegen(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		OrtID         int64    `json:"ortId"`
		Art           string   `json:"art"`
		Titel         string   `json:"titel"`
		Liter         *float64 `json:"liter"`
		IntervallTage float64  `json:"intervallTage"`
		RotNachTagen  float64  `json:"rotNachTagen"`
		Einmalig      bool     `json:"einmalig"`
		Termin        string   `json:"termin"`
		Sichtbarkeit  string   `json:"sichtbarkeit"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	ort, err := s.DB.GetPlace(in.OrtID)
	if err != nil {
		return nil, fmt.Errorf("Diesen Ort gibt es nicht (%d).", in.OrtID)
	}
	traeger, err := s.DB.GetTraeger(ort.TraegerID)
	if err != nil {
		return nil, err
	}
	// Was man nicht sehen darf, gibt es für einen nicht — der Ort wird nicht
	// über eine Fehlermeldung verraten.
	if !s.Zugriff.SiehtTraeger(*traeger) && !s.Zugriff.Mitglied.IstMitglied(traeger.ProjektID) {
		return nil, fmt.Errorf("Diesen Ort gibt es nicht (%d).", in.OrtID)
	}
	if !s.Zugriff.DarfVerwalten(*traeger) {
		return nil, abgelehnt(s, *traeger)
	}
	eingabe := api.TaskInput{Kind: in.Art, Title: in.Titel, Liters: in.Liter,
		IntervalDays: in.IntervallTage, RedAfterDays: in.RotNachTagen,
		OneOff: in.Einmalig, DueDate: in.Termin, Sichtbarkeit: in.Sichtbarkeit}
	if err := eingabe.Validate(); err != nil {
		return nil, err
	}
	aufgabe := model.CareTask{PlaceID: ort.ID, Active: true, CreatedAt: s.Now}
	eingabe.Apply(&aufgabe)
	if err := s.DB.InsertTask(&aufgabe); err != nil {
		return nil, err
	}
	return aufgabe, nil
}

func werkzeugErledigungMelden(args json.RawMessage, s Sitzung) (any, error) {
	var in struct {
		AufgabeID int64    `json:"aufgabeId"`
		Liter     *float64 `json:"liter"`
		Notiz     string   `json:"notiz"`
	}
	if err := entpacke(args, &in); err != nil {
		return nil, err
	}
	// Was man nicht sehen darf, kann man auch nicht melden — dieselbe Regel
	// wie in der REST-API.
	if _, err := sichtbareAufgabe(s, in.AufgabeID); err != nil {
		return nil, err
	}
	// Der Spielschutz (Sperrfrist), das Nachtragen und die Rechte der
	// Meldung stecken in api.CreateCompletion. Sie werden hier nicht
	// nachgebaut: Sonst gälte im Chat eine andere Regel als in der App.
	erledigung, err := api.CreateCompletion(s.DB, s.Now, in.AufgabeID,
		api.CompletionInput{Liters: in.Liter, Note: in.Notiz}, s.Nutzer)
	if err != nil {
		return nil, err
	}
	return erledigung, nil
}

// --- Helfer -----------------------------------------------------------------

// sichtbareAufgabe lädt eine Aufgabe, sofern sie für diese Person überhaupt
// existiert. Sie tut das über denselben Filter wie die Ortsliste.
func sichtbareAufgabe(s Sitzung, aufgabeID int64) (model.CareTask, error) {
	nichtGefunden := fmt.Errorf("Diese Aufgabe gibt es nicht (%d).", aufgabeID)
	aufgabe, err := s.DB.GetTask(aufgabeID)
	if err != nil {
		return model.CareTask{}, nichtGefunden
	}
	ort, err := s.DB.GetPlace(aufgabe.PlaceID)
	if err != nil {
		return model.CareTask{}, nichtGefunden
	}
	filter, err := api.NeuerFilter(s.DB, s.Zugriff)
	if err != nil {
		return model.CareTask{}, err
	}
	if !filter.AufgabeSichtbar(*ort, *aufgabe) {
		// Bewusst dieselbe Meldung wie „gibt es nicht": Ein eigener
		// Ablehnungstext verriete, dass es dort intern etwas gibt.
		return model.CareTask{}, nichtGefunden
	}
	return *aufgabe, nil
}

// zielTraeger liefert den Träger, unter dem etwas angelegt werden soll.
//
// Ohne Angabe wird der genommen, den die Person als einzigen verwaltet — im
// Dorfalltag hat fast jede und jeder genau einen Verein, und danach zu fragen
// wäre lästig. Die Erlaubnis selbst entscheidet immer model.Zugriff; hier
// wird nur ausgewählt, wonach gefragt wird.
func zielTraeger(s Sitzung, traegerID int64) (model.Traeger, error) {
	if traegerID != 0 {
		t, err := s.DB.GetTraeger(traegerID)
		if err != nil {
			return model.Traeger{}, errors.New("Diesen Träger gibt es nicht.")
		}
		if !s.Zugriff.DarfVerwalten(*t) {
			return model.Traeger{}, abgelehnt(s, *t)
		}
		return *t, nil
	}
	alle, err := s.DB.ListTraeger()
	if err != nil {
		return model.Traeger{}, err
	}
	var meine []model.Traeger
	for _, t := range alle {
		if s.Zugriff.DarfVerwalten(t) {
			meine = append(meine, t)
		}
	}
	switch len(meine) {
	case 0:
		if s.Zugriff.Veraltet {
			return model.Traeger{}, errors.New("Die Rössing-ID ist gerade nicht erreichbar. " +
				"Lesen geht weiter, Änderungen erst wieder, wenn die Mitgliedschaften " +
				"gesichert abgefragt werden können.")
		}
		return model.Traeger{}, errors.New("Du verwaltest keinen Verein und keine Gruppe — " +
			"anlegen kann das nur, wer für einen Träger zuständig ist.")
	case 1:
		return meine[0], nil
	default:
		namen := make([]string, 0, len(meine))
		for _, t := range meine {
			namen = append(namen, fmt.Sprintf("%s (traegerId %d)", t.Name, t.ID))
		}
		return model.Traeger{}, fmt.Errorf("Für welchen Träger? Zur Auswahl stehen: %s.",
			strings.Join(namen, ", "))
	}
}

// abgelehnt formuliert die Absage — und unterscheidet dabei den Ausfall der
// Rössing-ID vom schlichten Nein. Ein „später nochmal" ist etwas anderes als
// „du darfst das nicht".
func abgelehnt(s Sitzung, t model.Traeger) error {
	if s.Zugriff.Veraltet && !s.Zugriff.Betreiber {
		return errors.New("Die Rössing-ID ist gerade nicht erreichbar. Lesen geht weiter, " +
			"Änderungen sind erst wieder möglich, wenn die Mitgliedschaften gesichert " +
			"abgefragt werden können.")
	}
	// Der Name kommt über den Zugriff: Wer den Träger nicht sehen darf, soll
	// ihn auch nicht aus einer Fehlermeldung erfahren.
	return fmt.Errorf("Das dürfen nur die Verwaltenden von „%s“.",
		s.Zugriff.TraegerAnzeigeName(t))
}

// entpacke liest die Werkzeug-Argumente. Ein fehlender Rumpf ist zulässig —
// Werkzeuge ohne Pflichtfelder werden auch ohne Argumente aufgerufen.
func entpacke(args json.RawMessage, ziel any) error {
	if len(args) == 0 || string(args) == "null" {
		return nil
	}
	if err := json.Unmarshal(args, ziel); err != nil {
		return errors.New("Die Angaben waren nicht lesbar.")
	}
	return nil
}

// --- Schema-Helfer ----------------------------------------------------------

func schemaObjekt(pflicht []string, felder map[string]any) map[string]any {
	m := map[string]any{"type": "object", "properties": felder}
	if len(pflicht) > 0 {
		m["required"] = pflicht
	}
	return m
}

func schemaText(beschreibung string) map[string]any {
	return map[string]any{"type": "string", "description": beschreibung}
}

func schemaZahl(beschreibung string) map[string]any {
	return map[string]any{"type": "number", "description": beschreibung}
}

func schemaGanzzahl(beschreibung string) map[string]any {
	return map[string]any{"type": "integer", "description": beschreibung}
}

func schemaJaNein(beschreibung string) map[string]any {
	return map[string]any{"type": "boolean", "description": beschreibung}
}

func schemaAuswahl(beschreibung string, werte ...string) map[string]any {
	return map[string]any{"type": "string", "description": beschreibung, "enum": werte}
}
