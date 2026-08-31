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
