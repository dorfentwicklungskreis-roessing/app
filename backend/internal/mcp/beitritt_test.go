package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// Beitritte über MCP: Was in der Web-Verwaltung geht, muss auch hier gehen
// (#62). Vor allem das eine, worauf es ankommt — die Rolle landet wirklich in
// der Rössing-ID.

// roessingID spielt die Rössing-ID: Sie merkt sich, wer aufgenommen wurde.
type roessingID struct {
	mu          sync.Mutex
	aufgenommen map[string]string // "sub@projekt" → Rolle
	fehler      error
}

func (q *roessingID) Fuer(context.Context, auth.User) mitglied.Stand {
	return mitglied.Stand{Rollen: model.Mitgliedschaften{}}
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

// serverMitRoessingID ist serverMitDB, aber mit einer Rössing-ID, in die
// zurückgeschrieben werden kann.
func serverMitRoessingID(t *testing.T) (*httptest.Server, *db.DB, *roessingID) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	q := &roessingID{aufgenommen: map[string]string{}}
	s := New(d, stubVerifier{}, issuer, "https://api.example", "client-123")
	s.Now = func() time.Time { return time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC) }
	s.Mitglieder = q
	mux := http.NewServeMux()
	s.Register(mux)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts, d, q
}

func testTraeger(t *testing.T, d *db.DB, name, projektID string) model.Traeger {
	t.Helper()
	tr := model.Traeger{Name: name, ProjektID: projektID, Status: model.TraegerZugelassen,
		Sichtbarkeit: model.TraegerOffen, CreatedAt: time.Now().UTC()}
	if err := d.InsertTraeger(&tr); err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestBeitrittsWerkzeugeSindAngemeldet(t *testing.T) {
	ts := newTestServer(t)
	out := rpc(t, ts, "admin-jwt", "tools/list", nil)
	namen := map[string]bool{}
	for _, roh := range out["result"].(map[string]any)["tools"].([]any) {
		namen[roh.(map[string]any)["name"].(string)] = true
	}
	for _, name := range []string{"beitritte_liste", "beitritt_entscheiden", "mitglied_aufnehmen"} {
		if !namen[name] {
			t.Errorf("Werkzeug %q fehlt", name)
		}
	}
}

// Der Durchstich über MCP: offenen Antrag sehen, freigeben, und die Rolle
// steht in der Rössing-ID.
func TestBeitrittUeberMCPEntscheiden(t *testing.T) {
	ts, d, roessing := serverMitRoessingID(t)
	tr := testTraeger(t, d, "AK 2 Umwelt und Natur", "388")
	b := model.Beitritt{TraegerID: tr.ID, UserSub: "erna", Status: model.AntragBeantragt,
		Begruendung: "Ich wohne neben dem Beet", CreatedAt: time.Now().UTC()}
	if err := d.InsertBeitritt(&b); err != nil {
		t.Fatal(err)
	}

	text, fehler := callTool(t, ts, "beitritte_liste", map[string]any{"status": "beantragt"})
	if fehler {
		t.Fatalf("beitritte_liste: %s", text)
	}
	var liste []struct {
		ID          int64  `json:"id"`
		TraegerName string `json:"traegerName"`
	}
	if err := json.Unmarshal([]byte(text), &liste); err != nil {
		t.Fatalf("Antwort nicht lesbar: %v — %s", err, text)
	}
	if len(liste) != 1 || liste[0].TraegerName != "AK 2 Umwelt und Natur" {
		t.Fatalf("der offene Antrag fehlt oder nennt den Träger nicht: %s", text)
	}

	text, fehler = callTool(t, ts, "beitritt_entscheiden",
		map[string]any{"id": b.ID, "notiz": "willkommen"})
	if fehler {
		t.Fatalf("beitritt_entscheiden: %s", text)
	}
	nachher, err := d.GetBeitritt(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nachher.Status != model.AntragErteilt {
		t.Fatalf("Antrag nicht erteilt: %+v", nachher)
	}
	if !roessing.hat("erna", "388") {
		t.Fatal("die Mitgliedschaft wurde nicht in die Rössing-ID zurückgeschrieben")
	}
}

// Aufnehmen ohne vorherigen Antrag — der Weg für die geschlossene Gruppe.
func TestMitgliedUeberMCPAufnehmen(t *testing.T) {
	ts, d, roessing := serverMitRoessingID(t)
	tr := testTraeger(t, d, "Dorfpflege Rössing e.V.", "377")

	text, fehler := callTool(t, ts, "mitglied_aufnehmen",
		map[string]any{"traegerId": tr.ID, "userSub": "erna", "notiz": "Versammlung"})
	if fehler {
		t.Fatalf("mitglied_aufnehmen: %s", text)
	}
	if !roessing.hat("erna", "377") {
		t.Fatal("die Mitgliedschaft steht nicht in der Rössing-ID")
	}
	vorgang, err := d.BeitrittVon(tr.ID, "erna")
	if err != nil || vorgang == nil || vorgang.Status != model.AntragErteilt {
		t.Fatalf("der Vorgang wurde nicht festgehalten: %v %+v", err, vorgang)
	}
}

// Ohne schreibende Rössing-ID sagt das Werkzeug, was fehlt — und tut nichts.
func TestOhneRoessingIDMeldetDasWerkzeugWasFehlt(t *testing.T) {
	ts, d := serverMitDB(t) // ohne Mitglieder-Quelle
	tr := testTraeger(t, d, "Dorfpflege Rössing e.V.", "377")

	text, fehler := callTool(t, ts, "mitglied_aufnehmen",
		map[string]any{"traegerId": tr.ID, "userSub": "erna"})
	if !fehler {
		t.Fatalf("die Aufnahme gelang, obwohl niemand schreiben kann: %s", text)
	}
	if !strings.Contains(text, "Dienst-Nutzer") {
		t.Errorf("die Meldung sagt nicht, was einzurichten ist: %s", text)
	}
	if vorgang, _ := d.BeitrittVon(tr.ID, "erna"); vorgang != nil {
		t.Fatalf("es wurde ein Vorgang festgehalten, obwohl nichts geschah: %+v", vorgang)
	}
}
