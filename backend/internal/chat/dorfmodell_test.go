package chat

import (
	"strings"
	"testing"
)

// Das Dorfmodell: das billigste Modell, das es gibt.
//
// Es läuft im Testprozess, kostet nichts und antwortet immer gleich. Es
// versteht nur Deutsch im Umfang eines Dutzends Stichwörter, aber es macht
// alles, was ein Modell an dieser Stelle tut: Es liest die Frage, sucht sich
// aus den ANGEBOTENEN Werkzeugen eines aus, ruft es auf, liest das Ergebnis
// und formuliert daraus eine Antwort.
//
// Der Unterschied zu einem Drehbuch (siehe lokalesmodell_test.go) ist der
// Punkt: Das Dorfmodell kennt keine fest verdrahteten Werkzeugnamen. Es
// nimmt nur, was in der Anfrage steht. Wird ein Werkzeug umbenannt, aus der
// Liste vergessen oder mit einem Schema ausgeliefert, das seine Pflichtfelder
// nicht nennt, dann fällt das hier auf — und nicht erst dem ersten Menschen,
// der die App aufmacht.
//
// Wer stattdessen mit einem echten kleinen Modell arbeiten will (llama.cpp,
// Ollama & Co. mit Anthropic-kompatiblem /v1/messages), setzt CHAT_BASIS_URL
// auf dessen Adresse — denselben Weg benutzt dieser Testserver.

// stichwoerter ordnet Wörtern der Frage ein Werkzeug zu. Die Reihenfolge
// entscheidet: Das erste passende Stichwort gewinnt.
var stichwoerter = []struct {
	wort     string
	werkzeug string
}{
	{"gegossen", "erledigung_melden"},
	{"erledigt", "erledigung_melden"},
	{"zuletzt", "historie"},
	{"rangliste", "rangliste"},
	{"meisten", "rangliste"},
	{"verein", "traeger_liste"},
	{"gruppe", "traeger_liste"},
	{"an", "orte_liste"},
	{"orte", "orte_liste"},
	{"gießen", "orte_liste"},
}

// starteDorfmodell startet ein Modell, das sich sein Werkzeug selbst sucht.
// Die vorgabe wird benutzt, wenn kein Stichwort passt.
func starteDorfmodell(t *testing.T, vorgabe map[string]any) *lokalesModell {
	t.Helper()
	return starteModell(t, func(_ int, ein modellAnfrage) any {
		// Liegt schon ein Werkzeugergebnis vor, ist die Runde zu Ende: Das
		// Modell antwortet mit dem, was es gesehen hat.
		if ergebnisse := ein.Werkzeugergebnisse(); len(ergebnisse) > 0 {
			letztes := ergebnisse[len(ergebnisse)-1]
			if strings.HasPrefix(letztes, "Fehler: ") {
				// Die Absage des Backends geht im Wortlaut weiter — genau
				// das steht auch im Systemtext.
				return antwortText(strings.TrimPrefix(letztes, "Fehler: "))
			}
			return antwortText("Ich habe nachgesehen: " + gekuerzt(letztes))
		}
		angeboten := map[string]bool{}
		for _, w := range ein.Tools {
			angeboten[w.Name] = true
		}
		name := werkzeugFuer(ein.LetzterText(), angeboten)
		if name == "" {
			return antwortText("Dazu fällt mir nichts ein.")
		}
		eingabe := map[string]any{}
		for schluessel, wert := range vorgabe {
			eingabe[schluessel] = wert
		}
		return antwortWerkzeug(name, eingabe)
	})
}

// werkzeugFuer sucht das Werkzeug zur Frage — aber nur unter denen, die
// wirklich angeboten wurden.
func werkzeugFuer(frage string, angeboten map[string]bool) string {
	klein := strings.ToLower(frage)
	for _, s := range stichwoerter {
		if strings.Contains(klein, s.wort) && angeboten[s.werkzeug] {
			return s.werkzeug
		}
	}
	return ""
}

func gekuerzt(text string) string {
	if len(text) > 4000 {
		return text[:4000]
	}
	return text
}

// --- Die Proben --------------------------------------------------------------

// Der ganze Weg an einem Stück: Frage auf Deutsch, Werkzeug aus der
// Werkzeugliste, echte Daten aus der Datenbank, Antwort in der App.
func TestDorfmodellBeantwortetDieFrageAusEchtenDaten(t *testing.T) {
	dd := neuesDorf(t)
	ts := dd.server(t, starteDorfmodell(t, nil))

	aus := lies[frageAusgabe](t, frage(t, ts, tokenNachbarin, "Was steht gerade an?"))
	if len(aus.Werkzeuge) != 1 || aus.Werkzeuge[0] != "orte_liste" {
		t.Fatalf("Werkzeuge = %v, erwartet [orte_liste]", aus.Werkzeuge)
	}
	if !strings.Contains(aus.Antwort, "Kirchplatz") {
		t.Fatalf("Antwort = %q — sie muss aus der Datenbank kommen", aus.Antwort)
	}
	// Und dieselbe Frage verrät auch hier nicht, was intern ist.
	if strings.Contains(aus.Antwort, "Vereinsbeet") || strings.Contains(aus.Antwort, "Vereinsheim") {
		t.Fatalf("die interne Aufgabe steht in der Antwort: %q", aus.Antwort)
	}
}

// Ein Mitglied bekommt bei derselben Frage mehr zu sehen — der Chat ist
// dicht, nicht blind.
func TestDorfmodellZeigtDemMitgliedMehr(t *testing.T) {
	dd := neuesDorf(t)
	ts := dd.server(t, starteDorfmodell(t, nil))

	aus := lies[frageAusgabe](t, frage(t, ts, tokenMitglied, "Was steht gerade an?"))
	if !strings.Contains(aus.Antwort, "Vereinsbeet") {
		t.Fatalf("Antwort = %q — das Mitglied muss seine interne Aufgabe sehen", aus.Antwort)
	}
}

// Eine Absage des Backends kommt bei der Person im Wortlaut an, statt vom
// Modell umformuliert oder verschluckt zu werden.
func TestDorfmodellReichtDieAbsageWeiter(t *testing.T) {
	dd := neuesDorf(t)
	// Das Modell greift nach der internen Aufgabe, die es gar nicht geben darf.
	ts := dd.server(t, starteDorfmodell(t, map[string]any{"aufgabeId": dd.Internes.ID}))

	aus := lies[frageAusgabe](t, frage(t, ts, tokenNachbarin, "Wer war da zuletzt?"))
	if !strings.Contains(aus.Antwort, "gibt es nicht") {
		t.Fatalf("Antwort = %q, erwartet die Absage des Backends", aus.Antwort)
	}
	if strings.Contains(aus.Antwort, "Vereinsbeet") {
		t.Fatalf("die interne Aufgabe steht in der Antwort: %q", aus.Antwort)
	}
}
