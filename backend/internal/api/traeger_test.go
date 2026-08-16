package api

import (
	"fmt"
	"net/http"
	"testing"
)

// Der schärfste Test des ganzen Umbaus: Eine Aufgabe mit „nur_mitglieder“
// darf einem Außenstehenden auf KEINEM Weg sichtbar werden.
//
// Die Tokens haben im Dev-Modus das Format „sub:name:rollen“. Eine
// Träger-Mitgliedschaft steht darin als „<projektId>@<rolle>“ — im Betrieb
// kommt sie stattdessen vom Dienst-Nutzer über die Zitadel-Management-API
// (siehe internal/mitglied).
const (
	// Betreiber der Plattform: globale admin-Rolle, sieht alles.
	betreiberToken = "betreiber:Levin:admin"
	// Mitglied des Trägers mit Zitadel-Projekt 222.
	dorfpflegeMitglied = "erna:Erna:222@mitglied"
	// Admin desselben Trägers.
	dorfpflegeAdmin = "vorstand:Vorstand:222@admin"
	// Jemand aus dem Dorf ohne jede Mitgliedschaft.
	aussenToken = "fremd:Fremd Person:"
)

// traegerAnlegen legt einen zugelassenen Träger an und liefert seine ID.
func traegerAnlegen(t *testing.T, ts string, name, projektID string) int64 {
	t.Helper()
	resp := doReq(t, "POST", ts+"/api/v1/traeger", betreiberToken, map[string]any{
		"name": name, "projektId": projektID, "status": "zugelassen", "sichtbarkeit": "offen",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Träger anlegen: HTTP %d", resp.StatusCode)
	}
	return decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID
}

// ortMitAufgabe legt einen Ort samt einer Aufgabe der gewünschten
// Sichtbarkeit an (als Betreiber) und liefert beide Kennungen.
func ortMitAufgabe(t *testing.T, ts string, traegerID int64, name, sichtbarkeit string) (int64, int64) {
	t.Helper()
	resp := doReq(t, "POST", ts+"/api/v1/places", betreiberToken, map[string]any{
		"name": name, "kind": "beet", "lat": 52.21, "lon": 9.87, "traegerId": traegerID,
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Ort anlegen: HTTP %d", resp.StatusCode)
	}
	ortID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID

	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/places/%d/tasks", ts, ortID), betreiberToken,
		map[string]any{"kind": "jaeten", "intervalDays": 7, "redAfterDays": 14,
			"sichtbarkeit": sichtbarkeit})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Aufgabe anlegen: HTTP %d", resp.StatusCode)
	}
	aufgabeID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID
	return ortID, aufgabeID
}

func TestInterneAufgabeBleibtDrinnen(t *testing.T) {
	ts, _ := newTestServer(t)
	traegerID := traegerAnlegen(t, ts.URL, "Dorfpflege", "222")
	ortID, aufgabeID := ortMitAufgabe(t, ts.URL, traegerID, "Gerätehaus", "nur_mitglieder")

	t.Run("nicht in der Ortsliste", func(t *testing.T) {
		resp := doReq(t, "GET", ts.URL+"/api/v1/places", aussenToken, nil)
		liste := decode[placesResponse](t, resp)
		for _, p := range liste.Places {
			if p.ID == ortID {
				t.Fatalf("der Ort taucht außen auf: %+v", p)
			}
			for _, task := range p.Tasks {
				if task.ID == aufgabeID {
					t.Fatalf("die interne Aufgabe taucht außen auf: %+v", task)
				}
			}
		}
	})

	t.Run("keine Historie", func(t *testing.T) {
		resp := doReq(t, "GET", fmt.Sprintf("%s/api/v1/tasks/%d/completions", ts.URL, aufgabeID),
			aussenToken, nil)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("HTTP %d, erwartet 404 — die Historie verriete Ort und Aufgabe", resp.StatusCode)
		}
	})

	t.Run("keine Erledigung meldbar", func(t *testing.T) {
		resp := doReq(t, "POST", fmt.Sprintf("%s/api/v1/tasks/%d/completions", ts.URL, aufgabeID),
			aussenToken, map[string]any{})
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("HTTP %d, erwartet 404", resp.StatusCode)
		}
	})

	t.Run("keine Anmeldung zum Mithelfen", func(t *testing.T) {
		resp := doReq(t, "POST", fmt.Sprintf("%s/api/v1/places/%d/signup", ts.URL, ortID),
			aussenToken, map[string]any{})
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("HTTP %d, erwartet 404 — sonst ließe sich die Existenz erraten", resp.StatusCode)
		}
	})

	t.Run("Mitglieder sehen sie sehr wohl", func(t *testing.T) {
		resp := doReq(t, "GET", ts.URL+"/api/v1/places", dorfpflegeMitglied, nil)
		liste := decode[placesResponse](t, resp)
		gefunden := false
		for _, p := range liste.Places {
			for _, task := range p.Tasks {
				if task.ID == aufgabeID {
					gefunden = true
				}
			}
		}
		if !gefunden {
			t.Fatalf("Mitglieder sehen ihre eigene Aufgabe nicht: %+v", liste.Places)
		}
	})
}

// Eine geschlossene Gruppe darf öffentlich ausschreiben — dann ist die
// Aufgabe für alle da, die Gruppe selbst steht aber nicht im Verzeichnis.
func TestGeschlosseneGruppeSchreibtOeffentlichAus(t *testing.T) {
	ts, _ := newTestServer(t)
	traegerID := traegerAnlegen(t, ts.URL, "Dorfpflege", "222")
	resp := doReq(t, "PUT", fmt.Sprintf("%s/api/v1/traeger/%d", ts.URL, traegerID), betreiberToken,
		map[string]any{"name": "Dorfpflege", "projektId": "222",
			"status": "zugelassen", "sichtbarkeit": "geschlossen"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Träger schließen: HTTP %d", resp.StatusCode)
	}
	_, aufgabeID := ortMitAufgabe(t, ts.URL, traegerID, "Streuobstwiese", "oeffentlich")

	liste := decode[placesResponse](t, doReq(t, "GET", ts.URL+"/api/v1/places", aussenToken, nil))
	gefunden := false
	for _, p := range liste.Places {
		for _, task := range p.Tasks {
			if task.ID == aufgabeID {
				gefunden = true
			}
		}
	}
	if !gefunden {
		t.Error("öffentliche Aufgabe einer geschlossenen Gruppe muss außen sichtbar sein")
	}

	// Im Träger-Verzeichnis steht sie trotzdem nicht.
	verzeichnis := decode[struct {
		Traeger []struct {
			ID int64 `json:"id"`
		} `json:"traeger"`
	}](t, doReq(t, "GET", ts.URL+"/api/v1/traeger", aussenToken, nil))
	for _, tr := range verzeichnis.Traeger {
		if tr.ID == traegerID {
			t.Error("geschlossene Gruppe steht im öffentlichen Verzeichnis")
		}
	}
}

// Ein noch nicht zugelassener Träger ist unsichtbar — samt seiner Aufgaben.
// Zulassen darf ausschließlich der Plattform-Betreiber.
func TestNurBetreiberLaesstTraegerZu(t *testing.T) {
	ts, _ := newTestServer(t)
	resp := doReq(t, "POST", ts.URL+"/api/v1/traeger", betreiberToken, map[string]any{
		"name": "Neuer Verein", "projektId": "222", "status": "beantragt", "sichtbarkeit": "offen",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("HTTP %d", resp.StatusCode)
	}
	traegerID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID
	_, aufgabeID := ortMitAufgabe(t, ts.URL, traegerID, "Noch nichts", "oeffentlich")

	liste := decode[placesResponse](t, doReq(t, "GET", ts.URL+"/api/v1/places", aussenToken, nil))
	for _, p := range liste.Places {
		for _, task := range p.Tasks {
			if task.ID == aufgabeID {
				t.Fatal("Aufgabe eines nicht zugelassenen Trägers ist sichtbar")
			}
		}
	}

	// Der Träger-Admin darf sich nicht selbst zulassen. Die Antwort ist
	// 404 und nicht 403: Ein noch nicht zugelassener Träger existiert für
	// niemanden außer dem Betreiber — auch nicht für seine eigenen Leute.
	// Es gibt also nichts, dessen Zulassung man verweigern müsste.
	resp = doReq(t, "PUT", fmt.Sprintf("%s/api/v1/traeger/%d", ts.URL, traegerID), dorfpflegeAdmin,
		map[string]any{"name": "Neuer Verein", "projektId": "222",
			"status": "zugelassen", "sichtbarkeit": "offen"})
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("Selbst-Zulassung: HTTP %d, erwartet 404", resp.StatusCode)
	}

	// Nach der Zulassung durch den Betreiber darf der Träger-Admin ihn
	// pflegen — aber den Zulassungsstand weiterhin nicht ändern.
	resp = doReq(t, "PUT", fmt.Sprintf("%s/api/v1/traeger/%d", ts.URL, traegerID), betreiberToken,
		map[string]any{"name": "Neuer Verein", "projektId": "222",
			"status": "zugelassen", "sichtbarkeit": "offen"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Zulassung durch den Betreiber: HTTP %d", resp.StatusCode)
	}
	resp = doReq(t, "PUT", fmt.Sprintf("%s/api/v1/traeger/%d", ts.URL, traegerID), dorfpflegeAdmin,
		map[string]any{"name": "Umbenannt", "sichtbarkeit": "geschlossen"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Träger-Admin pflegt seinen Träger: HTTP %d", resp.StatusCode)
	}
	resp = doReq(t, "PUT", fmt.Sprintf("%s/api/v1/traeger/%d", ts.URL, traegerID), dorfpflegeAdmin,
		map[string]any{"name": "Umbenannt", "status": "gesperrt"})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("Träger-Admin ändert den Zulassungsstand: HTTP %d, erwartet 403", resp.StatusCode)
	}
}

// Verwalten darf nur der admin des jeweiligen Trägers.
func TestNurTraegerAdminAendertSeineAufgaben(t *testing.T) {
	ts, _ := newTestServer(t)
	traegerID := traegerAnlegen(t, ts.URL, "Dorfpflege", "222")
	ortID, aufgabeID := ortMitAufgabe(t, ts.URL, traegerID, "Streuobstwiese", "oeffentlich")

	aendern := func(token string) int {
		resp := doReq(t, "PUT", fmt.Sprintf("%s/api/v1/tasks/%d", ts.URL, aufgabeID), token,
			map[string]any{"kind": "jaeten", "intervalDays": 10, "redAfterDays": 20})
		return resp.StatusCode
	}
	if code := aendern(dorfpflegeAdmin); code != http.StatusOK {
		t.Errorf("Träger-Admin darf seine Aufgabe nicht ändern: HTTP %d", code)
	}
	if code := aendern(dorfpflegeMitglied); code != http.StatusForbidden {
		t.Errorf("einfaches Mitglied ändert die Aufgabe: HTTP %d, erwartet 403", code)
	}
	if code := aendern(aussenToken); code != http.StatusForbidden {
		t.Errorf("Außenstehende ändern die Aufgabe: HTTP %d, erwartet 403", code)
	}

	// Der Admin eines FREMDEN Trägers erst recht nicht.
	fremderTraeger := traegerAnlegen(t, ts.URL, "Schützenverein", "333")
	_ = fremderTraeger
	if code := aendern("wer:Wer:333@admin"); code != http.StatusForbidden {
		t.Errorf("fremder Träger-Admin ändert die Aufgabe: HTTP %d, erwartet 403", code)
	}

	// Und ein Ort lässt sich nicht in einen fremden Träger schieben.
	resp := doReq(t, "PUT", fmt.Sprintf("%s/api/v1/places/%d", ts.URL, ortID), dorfpflegeAdmin,
		map[string]any{"name": "Streuobstwiese", "kind": "beet", "lat": 52.21, "lon": 9.87,
			"traegerId": fremderTraeger})
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("Ort in fremden Träger verschoben: HTTP %d, erwartet 403", resp.StatusCode)
	}
}

// Die Rangliste ist ein Weg nach außen wie jeder andere: Erledigungen an
// internen Aufgaben dürfen dort für Außenstehende nicht auftauchen.
func TestInterneErledigungenZaehlenNurDrinnen(t *testing.T) {
	ts, _ := newTestServer(t)
	traegerID := traegerAnlegen(t, ts.URL, "Dorfpflege", "222")
	_, intern := ortMitAufgabe(t, ts.URL, traegerID, "Gerätehaus", "nur_mitglieder")

	resp := doReq(t, "POST", fmt.Sprintf("%s/api/v1/tasks/%d/completions", ts.URL, intern),
		dorfpflegeMitglied, map[string]any{})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Mitglied kann die interne Aufgabe nicht melden: HTTP %d", resp.StatusCode)
	}

	art := func(token string) int {
		liste := decode[struct {
			Totals struct {
				Completions int `json:"completions"`
			} `json:"totals"`
		}](t, doReq(t, "GET", ts.URL+"/api/v1/stats/leaderboard?period=gesamt", token, nil))
		return liste.Totals.Completions
	}
	if n := art(aussenToken); n != 0 {
		t.Errorf("Außenstehende sehen %d interne Erledigungen in der Rangliste", n)
	}
	if n := art(dorfpflegeMitglied); n != 1 {
		t.Errorf("Mitglieder sehen ihre eigene Erledigung nicht (%d)", n)
	}
}

// Auch die Nebenwege dürfen die Existenz einer internen Aufgabe nicht
// verraten: Ein Unterschied zwischen 403 und 404 wäre beim Durchprobieren
// von Kennungen schon die Auskunft „hier gibt es etwas“.
func TestInterneAufgabeVerraetSichNichtUeberFehlercodes(t *testing.T) {
	ts, srv := newTestServer(t)
	traegerID := traegerAnlegen(t, ts.URL, "Dorfpflege", "222")
	_, aufgabeID := ortMitAufgabe(t, ts.URL, traegerID, "Gerätehaus", "nur_mitglieder")

	// Ein Mitglied meldet eine Erledigung und ein Vorgang läuft.
	resp := doReq(t, "POST", fmt.Sprintf("%s/api/v1/tasks/%d/completions", ts.URL, aufgabeID),
		dorfpflegeMitglied, map[string]any{})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Erledigung: HTTP %d", resp.StatusCode)
	}
	meldungID := decode[struct {
		ID int64 `json:"id"`
	}](t, resp).ID
	vorgangID := vorgangEroeffnen(t, srv, aufgabeID)

	// Beides muss für Außenstehende schlicht nicht existieren.
	resp = doReq(t, "DELETE", fmt.Sprintf("%s/api/v1/completions/%d", ts.URL, meldungID),
		aussenToken, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Meldung zurücknehmen: HTTP %d, erwartet 404", resp.StatusCode)
	}
	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/assignments/%d/release", ts.URL, vorgangID),
		aussenToken, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Zusage zurückgeben: HTTP %d, erwartet 404", resp.StatusCode)
	}
	resp = doReq(t, "POST", fmt.Sprintf("%s/api/v1/assignments/%d/claim", ts.URL, vorgangID),
		aussenToken, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Zusage: HTTP %d, erwartet 404", resp.StatusCode)
	}

	// Die Meldung bleibt dabei natürlich bestehen.
	if _, err := srv.DB.GetCompletion(meldungID); err != nil {
		t.Fatalf("die Meldung wurde entfernt: %v", err)
	}
}

// Ein Client, der die neuen Felder nicht kennt (eine ältere App-Version),
// darf eine Aufgabe nicht versehentlich öffentlich machen.
//
// Das ist der gefährlichste Weg zurück: Die Verwaltung ändert in der App das
// Gießintervall, schickt dabei kein „sichtbarkeit“ mit — und die interne
// Aufgabe stünde plötzlich für das ganze Dorf da. Fehlende Felder müssen
// deshalb „unverändert“ heißen, nicht „Vorgabewert“.
func TestAlterClientSetztSichtbarkeitNichtZurueck(t *testing.T) {
	ts, srv := newTestServer(t)
	traegerID := traegerAnlegen(t, ts.URL, "Dorfpflege", "222")
	befaehigung := befaehigungAnlegen(t, ts.URL, traegerID, "Motorsense")
	_, aufgabeID := ortMitAufgabe(t, ts.URL, traegerID, "Gerätehaus", "nur_mitglieder")

	// Die Aufgabe verlangt zusätzlich eine Einweisung.
	resp := doReq(t, "PUT", fmt.Sprintf("%s/api/v1/tasks/%d", ts.URL, aufgabeID), dorfpflegeAdmin,
		map[string]any{"kind": "jaeten", "intervalDays": 7, "redAfterDays": 14,
			"sichtbarkeit": "nur_mitglieder", "befaehigungId": befaehigung})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Vorbereitung: HTTP %d", resp.StatusCode)
	}

	// Jetzt der alte Client: dieselbe Aufgabe, aber ohne die neuen Felder.
	resp = doReq(t, "PUT", fmt.Sprintf("%s/api/v1/tasks/%d", ts.URL, aufgabeID), dorfpflegeAdmin,
		map[string]any{"kind": "jaeten", "intervalDays": 10, "redAfterDays": 20})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Änderung: HTTP %d", resp.StatusCode)
	}

	nachher, err := srv.DB.GetTask(aufgabeID)
	if err != nil {
		t.Fatal(err)
	}
	if !nachher.Intern() {
		t.Error("die interne Aufgabe wurde stillschweigend öffentlich")
	}
	if nachher.BefaehigungID != befaehigung {
		t.Error("die verlangte Einweisung ging stillschweigend verloren")
	}
	// Die tatsächlich geschickte Änderung ist natürlich angekommen.
	if nachher.IntervalDays != 10 {
		t.Errorf("das Intervall wurde nicht geändert: %+v", nachher)
	}

	// Und wer sie ausdrücklich freigeben will, kann das weiterhin.
	resp = doReq(t, "PUT", fmt.Sprintf("%s/api/v1/tasks/%d", ts.URL, aufgabeID), dorfpflegeAdmin,
		map[string]any{"kind": "jaeten", "intervalDays": 10, "redAfterDays": 20,
			"sichtbarkeit": "oeffentlich", "befaehigungId": 0})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Freigabe: HTTP %d", resp.StatusCode)
	}
	nachher, _ = srv.DB.GetTask(aufgabeID)
	if nachher.Intern() || nachher.BefaehigungID != 0 {
		t.Errorf("die ausdrückliche Freigabe wirkte nicht: %+v", nachher)
	}
}
