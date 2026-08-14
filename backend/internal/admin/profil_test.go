package admin

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Tests der Profilseiten in der Verwaltung. Wie überall hier: echte SQLite,
// echte Handler, echte Templates — nichts wird gemockt.

// profilAnlegen legt ein Profil direkt in der Datenbank an (so, wie es die
// App über die REST-API täte).
func profilAnlegen(t *testing.T, a *App, p model.Profile) {
	t.Helper()
	if p.Visibility == (model.ProfileVisibility{}) {
		p.Visibility = model.DefaultVisibility()
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	}
	if err := a.db.UpsertProfile(&p); err != nil {
		t.Fatalf("Profil anlegen: %v", err)
	}
}

func TestVerwaltungZeigtEigenesProfil(t *testing.T) {
	_, h, _, sitzung := aufbau(t)

	// Ohne Anmeldung ist nichts zu holen.
	if w := hole(t, h, "/admin/dorfbewohner/profil"); w.Code != http.StatusSeeOther {
		t.Fatalf("Profilseite ohne Anmeldung: %d, erwartet 303", w.Code)
	}

	w := hole(t, h, "/admin/dorfbewohner/profil", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Profilseite: %d %s", w.Code, w.Body.String())
	}
	koerper := w.Body.String()
	// Beim ersten Aufruf steht der Name aus der Rössing-ID im Formular.
	if !strings.Contains(koerper, `value="Testadmin"`) {
		t.Errorf("Anzeigename ist nicht vorbelegt: %s", koerper)
	}
	for _, feld := range []string{"feld-anzeigename", "feld-nickname", "feld-telefon", "feld-email", "feld-notiz"} {
		if !strings.Contains(koerper, feld) {
			t.Errorf("Feld %s fehlt auf der Profilseite", feld)
		}
	}
	// Der Hinweis auf die Sichtbarkeit muss auf der Seite stehen, nicht im
	// Kleingedruckten irgendwo anders.
	if !strings.Contains(koerper, "sichtbarkeitshinweis") {
		t.Error("Der Hinweis auf die Sichtbarkeit fehlt")
	}
}

func TestVerwaltungSpeichertProfil(t *testing.T) {
	a, h, _, sitzung := aufbau(t)

	w := sende(t, h, "/admin/dorfbewohner/profil", url.Values{
		"anzeigename":   {"Levin Keller"},
		"nickname":      {"Gießmeister"},
		"telefon":       {"05066 123456"},
		"email":         {"levin@example.org"},
		"notiz":         {"erreichbar abends"},
		"sicht_telefon": {"dorf"},
	}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Speichern: %d %s", w.Code, w.Body.String())
	}

	p, err := a.db.GetProfile("u1")
	if err != nil {
		t.Fatalf("Profil nicht gespeichert: %v", err)
	}
	if p.Nickname != "Gießmeister" || p.Phone != "05066 123456" {
		t.Fatalf("Profil falsch übernommen: %+v", p)
	}
	if p.Visibility.Phone != model.VisibilityVillage {
		t.Errorf("Sichtbarkeit Telefon = %v, erwartet dorf", p.Visibility.Phone)
	}
	// Ohne gesetzten Haken bleibt es bei „nur Verwaltende“ — kein stilles
	// Veröffentlichen.
	if p.Visibility.Email != model.VisibilityAdmins {
		t.Errorf("Sichtbarkeit E-Mail = %v, erwartet verwaltung", p.Visibility.Email)
	}
}

func TestVerwaltungWeistUnsinnigesProfilAb(t *testing.T) {
	a, h, _, sitzung := aufbau(t)

	w := sende(t, h, "/admin/dorfbewohner/profil", url.Values{
		"anzeigename": {"Levin Keller"},
		"email":       {"keine-adresse"},
	}, sitzung)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "formularfehler") {
		t.Fatalf("kaputte E-Mail wurde angenommen: %d", w.Code)
	}
	if _, err := a.db.GetProfile("u1"); err == nil {
		t.Fatal("trotz Fehler wurde ein Profil gespeichert")
	}
}

func TestVerwaltungZeigtMitgliederMitAllemUndKennzeichnet(t *testing.T) {
	a, h, _, sitzung := aufbau(t)
	profilAnlegen(t, a, model.Profile{
		UserSub: "erna", DisplayName: "Erna Beispiel", Nickname: "Gießmeisterin",
		Phone: "05066 123456", Email: "erna@example.org", Note: "erreichbar abends",
		TokenName: "Erna Beispiel",
		Visibility: model.ProfileVisibility{
			DisplayName: model.VisibilityVillage, Nickname: model.VisibilityVillage,
			Phone: model.VisibilityAdmins, Email: model.VisibilityVillage,
			Note: model.VisibilityAdmins,
		},
	})

	w := hole(t, h, "/admin/dorfbewohner/", sitzung)
	if w.Code != http.StatusOK {
		t.Fatalf("Mitgliederliste: %d %s", w.Code, w.Body.String())
	}
	koerper := w.Body.String()
	for _, erwartet := range []string{"Gießmeisterin", "05066 123456", "erna@example.org", "erreichbar abends"} {
		if !strings.Contains(koerper, erwartet) {
			t.Errorf("Verwaltende sehen %q nicht", erwartet)
		}
	}
	// Was nur die Verwaltung sieht, ist als solches gekennzeichnet.
	if !strings.Contains(koerper, "nur-verwaltung") {
		t.Error("nicht freigegebene Felder sind nicht gekennzeichnet")
	}
	// Verlinkt wird die Rufnummer, damit man direkt anrufen kann.
	if !strings.Contains(koerper, "tel:") || !strings.Contains(koerper, "mailto:") {
		t.Error("Telefon und E-Mail sind nicht anwählbar")
	}
}

func TestMitgliederNavigationVorhanden(t *testing.T) {
	_, h, _, sitzung := aufbau(t)
	w := hole(t, h, "/admin/", sitzung)
	if !strings.Contains(w.Body.String(), "/admin/dorfbewohner/") {
		t.Fatalf("Bereich Dorfbewohner fehlt in der Verwaltung: %s", w.Body.String())
	}
}

// TestVerwaltungsHistorieNutztProfilnamen: Auch in der Verwaltung steht der
// Profilname, nicht der bei der Meldung eingefrorene.
func TestVerwaltungsHistorieNutztProfilnamen(t *testing.T) {
	a, h, d, sitzung := aufbau(t)

	w := sende(t, h, "/admin/dorfpflege/orte/neu", url.Values{
		"name": {"Teststelle"}, "art": {"beet"}, "lat": {"52.2"}, "lon": {"9.8"}, "aktiv": {"1"},
	}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Ort anlegen: %d", w.Code)
	}
	orte, _ := d.ListPlaces()
	pfad := "/admin/dorfpflege/orte/" + itoa64(orte[0].ID)
	w = sende(t, h, pfad+"/aufgaben/neu", url.Values{
		"art": {"giessen"}, "liter": {"10"}, "intervall": {"7"}, "rot": {"14"}, "aktiv": {"1"},
	}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Aufgabe anlegen: %d", w.Code)
	}
	aufgaben, _ := d.ListTasks()
	w = sende(t, h, "/admin/dorfpflege/aufgaben/"+itoa64(aufgaben[0].ID)+"/erledigt",
		url.Values{"notiz": {"gegossen"}}, sitzung)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Erledigung: %d", w.Code)
	}

	// Vorher: der Name aus der Session.
	if !strings.Contains(hole(t, h, pfad, sitzung).Body.String(), "Testadmin") {
		t.Fatal("Historie zeigt den gemeldeten Namen nicht")
	}

	// Nach dem Anlegen eines Profils mit Nickname zieht die Historie nach.
	profilAnlegen(t, a, model.Profile{
		UserSub: "u1", DisplayName: "Testadmin", Nickname: "Der Gärtner", TokenName: "Testadmin",
	})
	koerper := hole(t, h, pfad, sitzung).Body.String()
	if !strings.Contains(koerper, "Der Gärtner") {
		t.Fatalf("Historie nutzt den Profilnamen nicht: %s", koerper)
	}
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
