package admin

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/mitglied"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Beitritte in der Web-Verwaltung — server-gerendert, echte Formulare, kein
// Modal. Geprüft wird, was wirklich im HTML steht und was danach in der
// Rössing-ID steht.

// roessingID spielt die Rössing-ID: lesen wie die Dev-Quelle, schreiben wie
// der Dienst-Nutzer im Betrieb.
type roessingID struct {
	mu          sync.Mutex
	aufgenommen map[string]string // "sub@projekt" → Rolle
	fehler      error
}

func (q *roessingID) Fuer(ctx context.Context, u auth.User) mitglied.Stand {
	stand := mitglied.DevQuelle{}.Fuer(ctx, u)
	q.mu.Lock()
	defer q.mu.Unlock()
	for schluessel, rolle := range q.aufgenommen {
		sub, projekt, _ := strings.Cut(schluessel, "@")
		if sub != u.Sub {
			continue
		}
		if stand.Rollen[projekt] == nil {
			stand.Rollen[projekt] = map[string]bool{}
		}
		stand.Rollen[projekt][rolle] = true
	}
	return stand
}

func (q *roessingID) Aufnehmen(_ context.Context, projektID, userSub, rolle string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.fehler != nil {
		return q.fehler
	}
	q.aufgenommen[userSub+"@"+projektID] = rolle
	return nil
}

func (q *roessingID) hat(sub, projektID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.aufgenommen[sub+"@"+projektID] == model.RolleMitglied
}

// beitrittsAufbau ist traegerAufbau mit einer Rössing-ID, in die
// zurückgeschrieben werden kann.
func beitrittsAufbau(t *testing.T) (*App, http.Handler, *db.DB, *roessingID) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	q := &roessingID{aufgenommen: map[string]string{}}
	a := newApp(Config{
		DB: d, Issuer: "https://id.invalid", ClientID: "test-client",
		PublicURL: "http://localhost:8080", SessionKey: []byte("test-schluessel"),
		Mitglieder: q,
		// Dieselbe gestellte Uhr wie in traegerAufbau: Die Sitzung im Test
		// läuft ab echter Zeit, eine vorgestellte Uhr ließe sie ablaufen.
		Now: func() time.Time { return time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC) },
	})
	mux := http.NewServeMux()
	a.register(mux)
	return a, mux, d, q
}

func offenerBeitritt(t *testing.T, d *db.DB, traegerID int64, sub string) model.Beitritt {
	t.Helper()
	b := model.Beitritt{TraegerID: traegerID, UserSub: sub, Status: model.AntragBeantragt,
		Begruendung: "Ich wohne neben dem Beet", CreatedAt: time.Now().UTC()}
	if err := d.InsertBeitritt(&b); err != nil {
		t.Fatal(err)
	}
	return b
}

// Der Durchstich in der Verwaltung: Der offene Antrag steht auf der Seite,
// ein Klick nimmt auf — und die Rolle steht danach in der Rössing-ID.
func TestBeitrittInDerVerwaltungFreigeben(t *testing.T) {
	a, h, d, roessing := beitrittsAufbau(t)
	tr := traegerAnlegen(t, d, "AK 2 Umwelt und Natur", "388", model.TraegerZugelassen)
	vorstand := sitzung(t, a, "vorstand", false, "388@admin")
	b := offenerBeitritt(t, d, tr.ID, "erna")

	w := hole(t, h, fmt.Sprintf("/admin/traeger/%d", tr.ID), vorstand)
	if w.Code != http.StatusOK {
		t.Fatalf("Trägerseite: HTTP %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), fmt.Sprintf(`id="beitritt-%d"`, b.ID)) {
		t.Fatal("der offene Beitrittsantrag steht nicht auf der Seite")
	}
	if strings.Contains(w.Body.String(), `id="keine-aufnahme"`) {
		t.Error("die Seite behauptet, es könne niemand aufgenommen werden")
	}

	w = sende(t, h, fmt.Sprintf("/admin/traeger/%d/beitritte/%d", tr.ID, b.ID),
		url.Values{"status": {"erteilt"}, "notiz": {"willkommen"}}, vorstand)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Aufnehmen: HTTP %d — %s", w.Code, w.Body.String())
	}
	nachher, err := d.GetBeitritt(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nachher.Status != model.AntragErteilt || nachher.Notiz != "willkommen" {
		t.Fatalf("Entscheidung nicht festgehalten: %+v", nachher)
	}
	if !roessing.hat("erna", "388") {
		t.Fatal("die Mitgliedschaft steht nicht in der Rössing-ID")
	}
}

// Scheitert das Eintragen, bleibt der Antrag offen — und auf dem Schirm steht,
// woran es lag. Nichts wäre schlimmer als eine Verwaltung, die „aufgenommen“
// meldet, während die Tür zu bleibt.
func TestGescheiterteAufnahmeMeldetSichUndAendertNichts(t *testing.T) {
	a, h, d, roessing := beitrittsAufbau(t)
	roessing.fehler = fmt.Errorf("Rössing-ID antwortet nicht")
	tr := traegerAnlegen(t, d, "AK 2 Umwelt und Natur", "388", model.TraegerZugelassen)
	vorstand := sitzung(t, a, "vorstand", false, "388@admin")
	b := offenerBeitritt(t, d, tr.ID, "erna")

	w := sende(t, h, fmt.Sprintf("/admin/traeger/%d/beitritte/%d", tr.ID, b.ID),
		url.Values{"status": {"erteilt"}}, vorstand)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("HTTP %d", w.Code)
	}
	nachher, _ := d.GetBeitritt(b.ID)
	if nachher.Status != model.AntragBeantragt {
		t.Fatalf("der Antrag wurde abgehakt, obwohl nichts eingetragen wurde: %+v", nachher)
	}
	// Die Meldung steht im Flash-Cookie und danach auf der Seite.
	if !strings.Contains(strings.Join(w.Header().Values("Set-Cookie"), " "), "flash") {
		t.Error("es gibt keine Rückmeldung an die Verwaltung")
	}
}

// Ohne schreibenden Dienst-Nutzer sagt die Seite das, statt einen Knopf
// anzubieten, der nichts tut.
func TestOhneDienstNutzerStehtDerHinweisAufDerSeite(t *testing.T) {
	a, h, d := traegerAufbau(t) // Dev-Quelle: liest aus dem Token, schreibt nicht
	tr := traegerAnlegen(t, d, "AK 2 Umwelt und Natur", "388", model.TraegerZugelassen)
	vorstand := sitzung(t, a, "vorstand", false, "388@admin")

	w := hole(t, h, fmt.Sprintf("/admin/traeger/%d", tr.ID), vorstand)
	if !strings.Contains(w.Body.String(), `id="keine-aufnahme"`) {
		t.Fatal("der Hinweis auf den fehlenden Dienst-Nutzer fehlt")
	}
}

// Jemanden ohne vorherigen Antrag aufnehmen — der Weg der geschlossenen
// Gruppe und die Abkürzung nach dem Gespräch am Gartenzaun.
func TestJemandenDirektAufnehmen(t *testing.T) {
	a, h, d, roessing := beitrittsAufbau(t)
	tr := traegerAnlegen(t, d, "AK 2 Umwelt und Natur", "388", model.TraegerZugelassen)
	vorstand := sitzung(t, a, "vorstand", false, "388@admin")

	w := sende(t, h, fmt.Sprintf("/admin/traeger/%d/mitglieder", tr.ID),
		url.Values{"userSub": {"erna"}, "notiz": {"auf der Versammlung gefragt"}}, vorstand)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("Aufnehmen: HTTP %d — %s", w.Code, w.Body.String())
	}
	if !roessing.hat("erna", "388") {
		t.Fatal("die Mitgliedschaft steht nicht in der Rössing-ID")
	}
	vorgang, err := d.BeitrittVon(tr.ID, "erna")
	if err != nil || vorgang == nil || vorgang.Status != model.AntragErteilt {
		t.Fatalf("der Vorgang wurde nicht festgehalten: %v %+v", err, vorgang)
	}
	if vorgang.EntschiedenVon != "vorstand" {
		t.Errorf("es steht nicht dabei, wer aufgenommen hat: %+v", vorgang)
	}
}

// Ein fremder Antrag geht die Verwaltung eines anderen Vereins nichts an.
func TestFremderAntragBleibtUnerreichbar(t *testing.T) {
	a, h, d, _ := beitrittsAufbau(t)
	meiner := traegerAnlegen(t, d, "AK 2 Umwelt und Natur", "388", model.TraegerZugelassen)
	fremder := traegerAnlegen(t, d, "Schützenverein", "999", model.TraegerZugelassen)
	vorstand := sitzung(t, a, "vorstand", false, "388@admin")
	fremd := offenerBeitritt(t, d, fremder.ID, "erna")

	w := sende(t, h, fmt.Sprintf("/admin/traeger/%d/beitritte/%d", meiner.ID, fremd.ID),
		url.Values{"status": {"erteilt"}}, vorstand)
	if w.Code != http.StatusNotFound {
		t.Fatalf("HTTP %d, erwartet 404", w.Code)
	}
	nachher, _ := d.GetBeitritt(fremd.ID)
	if nachher.Status != model.AntragBeantragt {
		t.Fatal("ein fremder Antrag wurde entschieden")
	}
}
