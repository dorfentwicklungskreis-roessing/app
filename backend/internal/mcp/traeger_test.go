package mcp

import (
	"encoding/json"
	"testing"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Träger über MCP — der Weg, der bis dahin nur durch den Browser führte.

func TestTraegerWerkzeugeSindAngemeldet(t *testing.T) {
	ts := newTestServer(t)
	out := rpc(t, ts, "admin-jwt", "tools/list", nil)
	namen := map[string]bool{}
	for _, roh := range out["result"].(map[string]any)["tools"].([]any) {
		namen[roh.(map[string]any)["name"].(string)] = true
	}
	for _, name := range []string{
		"traeger_liste", "traeger_anlegen", "traeger_aendern", "traeger_zulassung",
	} {
		if !namen[name] {
			t.Errorf("Werkzeug %q fehlt", name)
		}
	}
	// Bestehende Namen dürfen sich nicht ändern.
	for _, name := range []string{"orte_liste", "ort_anlegen", "ideen_liste"} {
		if !namen[name] {
			t.Errorf("bestehendes Werkzeug %q ist verschwunden", name)
		}
	}
}

func TestTraegerAnlegenUndZulassen(t *testing.T) {
	ts, d := serverMitDB(t)

	text, fehler := callTool(t, ts, "traeger_anlegen", map[string]any{
		"name":      "Dorfpflege Rössing e.V.",
		"projektId": "377270137180389479",
	})
	if fehler {
		t.Fatalf("traeger_anlegen: %s", text)
	}
	var verein model.Traeger
	if err := json.Unmarshal([]byte(text), &verein); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v — %s", err, text)
	}

	// Ein frisch angelegter Träger tritt noch nicht in Erscheinung. Zulassen
	// ist ein eigener, bewusster Schritt — auch für den Betreiber.
	if verein.Status != model.TraegerBeantragt {
		t.Fatalf("neuer Träger ist schon %q", verein.Status)
	}
	if verein.Sichtbarkeit != model.TraegerOffen {
		t.Fatalf("Sichtbarkeit ist %q statt offen", verein.Sichtbarkeit)
	}

	text, fehler = callTool(t, ts, "traeger_zulassung", map[string]any{
		"id": verein.ID, "status": "zugelassen",
	})
	if fehler {
		t.Fatalf("traeger_zulassung: %s", text)
	}
	nach, err := d.GetTraeger(verein.ID)
	if err != nil || !nach.Zugelassen() {
		t.Fatalf("nicht zugelassen: %+v (%v)", nach, err)
	}

	if _, fehler := callTool(t, ts, "traeger_zulassung", map[string]any{
		"id": verein.ID, "status": "vielleicht",
	}); !fehler {
		t.Error("unsinniger Zulassungsstand wurde angenommen")
	}
	if _, fehler := callTool(t, ts, "traeger_zulassung", map[string]any{
		"id": 99999, "status": "zugelassen",
	}); !fehler {
		t.Error("unbekannte ID wurde angenommen")
	}
}

// Der Fall, für den das gebaut wurde: die Dorfpflege mit ihrem AK 2 darunter.
func TestArbeitskreisBekommtSeinDach(t *testing.T) {
	ts, _ := serverMitDB(t)

	text, _ := callTool(t, ts, "traeger_anlegen", map[string]any{
		"name": "Dorfpflege Rössing e.V.", "projektId": "377270137180389479",
	})
	var verein model.Traeger
	if err := json.Unmarshal([]byte(text), &verein); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v — %s", err, text)
	}

	text, fehler := callTool(t, ts, "traeger_anlegen", map[string]any{
		"name":      "AK 2 Umwelt und Natur",
		"projektId": "388659726272954563",
		"parentId":  verein.ID,
	})
	if fehler {
		t.Fatalf("Arbeitskreis anlegen: %s", text)
	}
	var ak model.Traeger
	if err := json.Unmarshal([]byte(text), &ak); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v — %s", err, text)
	}
	if ak.ParentID != verein.ID {
		t.Fatalf("Dach nicht übernommen: %+v", ak)
	}

	// Die Liste nennt das Dach beim Namen — eine nackte Kennung müsste der
	// Leser selbst auflösen und würde es beim Vorlesen nicht tun.
	text, fehler = callTool(t, ts, "traeger_liste", map[string]any{})
	if fehler {
		t.Fatalf("traeger_liste: %s", text)
	}
	var liste []struct {
		ID            int64    `json:"id"`
		Name          string   `json:"name"`
		ParentName    string   `json:"parentName"`
		Arbeitskreise []string `json:"arbeitskreise"`
	}
	if err := json.Unmarshal([]byte(text), &liste); err != nil {
		t.Fatalf("Liste nicht lesbar: %v — %s", err, text)
	}
	var sahDach, sahKreis bool
	for _, e := range liste {
		if e.ID == ak.ID && e.ParentName == "Dorfpflege Rössing e.V." {
			sahDach = true
		}
		if e.ID == verein.ID && len(e.Arbeitskreise) == 1 &&
			e.Arbeitskreise[0] == "AK 2 Umwelt und Natur" {
			sahKreis = true
		}
	}
	if !sahDach {
		t.Errorf("Dach steht nicht mit Namen in der Liste: %s", text)
	}
	if !sahKreis {
		t.Errorf("Arbeitskreis fehlt beim Verein: %s", text)
	}

	// Eine dritte Ebene weist die Ablage ab — hier muss der Fehler ankommen
	// und nicht still verschluckt werden.
	if _, fehler := callTool(t, ts, "traeger_anlegen", map[string]any{
		"name": "Untergruppe", "parentId": ak.ID,
	}); !fehler {
		t.Error("eine dritte Ebene wurde angenommen")
	}
}

func TestTraegerAendernNurAngegebeneFelder(t *testing.T) {
	ts, d := serverMitDB(t)
	text, _ := callTool(t, ts, "traeger_anlegen", map[string]any{
		"name": "Kulturkreis", "beschreibung": "Feste im Dorf", "projektId": "4711",
	})
	var vorher model.Traeger
	if err := json.Unmarshal([]byte(text), &vorher); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v", err)
	}

	if text, fehler := callTool(t, ts, "traeger_aendern", map[string]any{
		"id": vorher.ID, "name": "Kulturkreis Rössing e.V.",
	}); fehler {
		t.Fatalf("traeger_aendern: %s", text)
	}
	nach, err := d.GetTraeger(vorher.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nach.Name != "Kulturkreis Rössing e.V." {
		t.Fatalf("Name nicht geändert: %+v", nach)
	}
	// Was nicht genannt wurde, bleibt stehen.
	if nach.Beschreibung != "Feste im Dorf" || nach.ProjektID != "4711" {
		t.Fatalf("ungenannte Felder verändert: %+v", nach)
	}
	// Der Zulassungsstand gehört nicht hierher — er hat sein eigenes Werkzeug.
	if nach.Status != vorher.Status {
		t.Fatalf("Zulassungsstand nebenbei geändert: %q → %q", vorher.Status, nach.Status)
	}

	// Eine Projekt-ID, die keine Zahl ist, liefe später still ins Leere.
	if _, fehler := callTool(t, ts, "traeger_aendern", map[string]any{
		"id": vorher.ID, "projektId": "Dorfpflege",
	}); !fehler {
		t.Error("unsinnige Projekt-ID wurde angenommen")
	}
}

// Mitglieder haben am MCP-Endpoint nichts verloren — auch nicht bei Trägern.
func TestTraegerWerkzeugeNurFuerAdmins(t *testing.T) {
	ts := newTestServer(t)
	resp := rpcRaw(t, ts, "member-jwt", "tools/call",
		map[string]any{"name": "traeger_liste", "arguments": map[string]any{}})
	defer resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("Mitglied bekommt Status %d, erwartet 403", resp.StatusCode)
	}
}

// --- Was vor der App war -----------------------------------------------------

// Der Anlass: Das Beet vor dem Dorfgemeinschaftshaus wurde im Juni gejätet,
// die Aufgabe dafür aber erst im August angelegt. Ohne diese Angabe stünde es
// bis Ende Oktober auf grün.
func TestZuletztErledigtUeberMCP(t *testing.T) {
	ts, d := serverMitDB(t)

	text, _ := callTool(t, ts, "ort_anlegen", map[string]any{
		"name": "Beet vor dem Dorfgemeinschaftshaus", "kind": "beet",
		"lat": 52.1829639, "lon": 9.8100629,
	})
	var ort model.Place
	if err := json.Unmarshal([]byte(text), &ort); err != nil {
		t.Fatalf("Ort nicht lesbar: %v — %s", err, text)
	}

	text, fehler := callTool(t, ts, "aufgabe_anlegen", map[string]any{
		"placeId": ort.ID, "kind": "jaeten",
		"intervalDays": 56, "redAfterDays": 77,
		"lastKnownDoneAt": "2026-06-15",
	})
	if fehler {
		t.Fatalf("aufgabe_anlegen: %s", text)
	}
	var aufgabe model.CareTask
	if err := json.Unmarshal([]byte(text), &aufgabe); err != nil {
		t.Fatalf("Aufgabe nicht lesbar: %v — %s", err, text)
	}
	if aufgabe.LastKnownDoneAt == nil {
		t.Fatal("die Angabe kam nicht an")
	}
	if got := aufgabe.LastKnownDoneAt.Format("2006-01-02"); got != "2026-06-15" {
		t.Fatalf("zuletzt erledigt am %s, erwartet 2026-06-15", got)
	}

	gespeichert, err := d.GetTask(aufgabe.ID)
	if err != nil || gespeichert.LastKnownDoneAt == nil {
		t.Fatalf("nicht gespeichert: %+v (%v)", gespeichert, err)
	}

	// Leerer Text nimmt die Angabe wieder weg.
	if text, fehler := callTool(t, ts, "aufgabe_aendern", map[string]any{
		"id": aufgabe.ID, "lastKnownDoneAt": "",
	}); fehler {
		t.Fatalf("aufgabe_aendern: %s", text)
	}
	nach, _ := d.GetTask(aufgabe.ID)
	if nach.LastKnownDoneAt != nil {
		t.Fatalf("Angabe blieb stehen: %v", nach.LastKnownDoneAt)
	}

	// „Zuletzt gemacht" ist eine Aussage über die Vergangenheit.
	if _, fehler := callTool(t, ts, "aufgabe_aendern", map[string]any{
		"id": aufgabe.ID, "lastKnownDoneAt": "2099-01-01",
	}); !fehler {
		t.Error("ein Datum in der Zukunft wurde angenommen")
	}
}

// --- Betreibersicht ----------------------------------------------------------

// Der MCP-Endpunkt zeigt die Betreibersicht: alle Orte aller Träger, auch
// deren interne Aufgaben. Das ist kein Leck — wer hier ankommt, hat die
// globale admin-Rolle und sieht dieselben Daten in der Web-Verwaltung.
//
// Damit man im Gespräch mit Claude aber *erkennt*, was intern ist, muss die
// Sichtbarkeit in der Antwort stehen. Sie steht dort heute, weil sie zum
// Datensatz gehört — dieser Test hält es fest, damit sie nicht eines Tages
// stillschweigend herausfällt (#35).
func TestOrteListeNenntDieSichtbarkeit(t *testing.T) {
	ts, d := serverMitDB(t)

	text, _ := callTool(t, ts, "ort_anlegen", map[string]any{
		"name": "Vereinsgarten", "lat": 52.18, "lon": 9.81,
	})
	var ort model.Place
	if err := json.Unmarshal([]byte(text), &ort); err != nil {
		t.Fatalf("Ort nicht lesbar: %v — %s", err, text)
	}
	if _, fehler := callTool(t, ts, "aufgabe_anlegen", map[string]any{
		"placeId": ort.ID, "kind": "jaeten", "intervalDays": 21, "redAfterDays": 35,
		"sichtbarkeit": "nur_mitglieder",
	}); fehler {
		t.Fatal("interne Aufgabe konnte nicht angelegt werden")
	}

	text, fehler := callTool(t, ts, "orte_liste", map[string]any{})
	if fehler {
		t.Fatalf("orte_liste: %s", text)
	}
	var antwort struct {
		Places []struct {
			Tasks []struct {
				Sichtbarkeit string `json:"sichtbarkeit"`
			} `json:"tasks"`
		} `json:"places"`
	}
	if err := json.Unmarshal([]byte(text), &antwort); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v — %s", err, text)
	}
	var gefunden bool
	for _, p := range antwort.Places {
		for _, a := range p.Tasks {
			if a.Sichtbarkeit == "nur_mitglieder" {
				gefunden = true
			}
		}
	}
	if !gefunden {
		t.Fatalf("die interne Aufgabe steht ohne erkennbare Sichtbarkeit da: %s", text)
	}

	// Und sie ist wirklich intern — der Test prüft nicht bloß ein Wort.
	aufgaben, err := d.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	var intern int
	for _, a := range aufgaben {
		if a.PlaceID == ort.ID && a.Sichtbarkeit == model.AufgabeNurMitglieder {
			intern++
		}
	}
	if intern != 1 {
		t.Fatalf("in der Datenbank steht etwas anderes: %+v", aufgaben)
	}
}
