package devmode

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/clock"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/vergabe"
)

// alleRouten sind die Pfade, die es nur im Testmodus geben darf.
var alleRouten = []struct {
	method string
	pfad   string
}{
	{"GET", "/dev/clock"},
	{"POST", "/dev/clock/set"},
	{"POST", "/dev/clock/advance"},
	{"POST", "/dev/clock/reset"},
	{"POST", "/dev/assignment/run"},
}

// TestNotMountedOutsideDevMode is the important one: in production these
// paths must not exist at all — not 403, not "unauthorized", nothing. A
// route that is merely guarded is one forgotten check away from being a way
// to move the clock of the running village.
func TestNotMountedOutsideDevMode(t *testing.T) {
	for _, modus := range []string{"oidc", "", "Insecure-Dev", "insecure-dev ", "dev", "production"} {
		t.Run("AUTH_MODE="+modus, func(t *testing.T) {
			mux := http.NewServeMux()
			if Register(mux, modus, Config{}) {
				t.Fatalf("Register hat im Modus %q eingehängt", modus)
			}
			for _, r := range alleRouten {
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest(r.method, r.pfad, nil))
				if w.Code != http.StatusNotFound {
					t.Errorf("%s %s: Status %d, erwartet 404 — der Pfad ist registriert", r.method, r.pfad, w.Code)
				}
			}
		})
	}
}

func TestMountedInDevMode(t *testing.T) {
	mux, _ := aufbau(t)
	for _, r := range alleRouten {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(r.method, r.pfad, strings.NewReader(`{"duration":"1s","time":"2026-01-01T00:00:00Z"}`)))
		if w.Code == http.StatusNotFound {
			t.Errorf("%s %s ist im Testmodus nicht erreichbar", r.method, r.pfad)
		}
	}
}

func TestClockSetAdvanceReset(t *testing.T) {
	mux, _ := aufbau(t)

	ziel := time.Date(2029, time.May, 17, 9, 30, 0, 0, time.UTC)
	a := ruf(t, mux, "POST", "/dev/clock/set", `{"time":"`+ziel.Format(time.RFC3339)+`"}`, http.StatusOK)
	if d := a.Now.Sub(ziel); d < 0 || d > time.Second {
		t.Errorf("nach set steht die Uhr %v daneben", d)
	}
	if d := clock.Now().Sub(ziel); d < 0 || d > time.Second {
		t.Errorf("die Uhr des Backends folgt dem Endpunkt nicht: %v", d)
	}

	a = ruf(t, mux, "POST", "/dev/clock/advance", `{"duration":"240h"}`, http.StatusOK)
	if d := a.Now.Sub(ziel.Add(240 * time.Hour)); d < 0 || d > time.Second {
		t.Errorf("nach advance steht die Uhr %v daneben", d)
	}

	a = ruf(t, mux, "GET", "/dev/clock", "", http.StatusOK)
	if a.OffsetSeconds == 0 {
		t.Error("GET /dev/clock meldet keinen Versatz, obwohl die Uhr gereist ist")
	}

	a = ruf(t, mux, "POST", "/dev/clock/reset", "", http.StatusOK)
	if a.OffsetSeconds != 0 || clock.Travelling() {
		t.Errorf("reset hat einen Versatz von %v gelassen", clock.Offset())
	}
}

func TestClockRejectsNonsense(t *testing.T) {
	mux, _ := aufbau(t)
	ruf(t, mux, "POST", "/dev/clock/set", `{"time":"morgen früh"}`, http.StatusBadRequest)
	ruf(t, mux, "POST", "/dev/clock/advance", `{"duration":"zehn Tage"}`, http.StatusBadRequest)
	ruf(t, mux, "POST", "/dev/clock/set", `{`, http.StatusBadRequest)
	if clock.Travelling() {
		t.Error("eine abgewiesene Eingabe hat die Uhr trotzdem verstellt")
	}
}

// TestAssignmentRunAsksTheHelper ist der Kern: kein Warten, kein Zeitfenster.
// Aufgabe anlegen, eintragen, Uhr vorstellen, Durchlauf anstoßen — danach
// liegt die Anfrage vor. Genau dieser Ablauf ersetzt die 150 Sekunden im
// Android-E2E.
func TestAssignmentRunAsksTheHelper(t *testing.T) {
	mux, d := aufbau(t)

	// Frisch angelegt ist die Aufgabe grün: Sie rechnet ab ihrer Anlage.
	p := model.Place{Name: "Unter den Eichen", Kind: model.PlaceFlowerbox, Active: true, CreatedAt: clock.Now()}
	if err := d.InsertPlace(&p); err != nil {
		t.Fatal(err)
	}
	task := model.CareTask{PlaceID: p.ID, Kind: model.TaskWatering, IntervalDays: 7, RedAfterDays: 14,
		Active: true, CreatedAt: clock.Now()}
	if err := d.InsertTask(&task); err != nil {
		t.Fatal(err)
	}
	anmeldung := model.Signup{UserSub: "anna", PlaceID: p.ID, CreatedAt: clock.Now()}
	if _, err := d.InsertSignup(&anmeldung); err != nil {
		t.Fatal(err)
	}

	// Ohne Fälligkeit passiert nichts — der Durchlauf ist beliebig oft
	// wiederholbar und arbeitet nur, wenn wirklich etwas ansteht.
	if a := laufen(t, mux); a.Notifications != 0 {
		t.Fatalf("es wurde gefragt, obwohl nichts fällig war (%d Benachrichtigungen)", a.Notifications)
	}

	// Zehn Tage weiter, mitten am Vormittag der Ortszeit — die Ruhezeit
	// zwischen 21 und 7 Uhr würde die Zustellung sonst auf den Morgen
	// verschieben, und der Test hinge wieder an der Uhrzeit der Maschine.
	vormittags(t, mux, 10)

	a := laufen(t, mux)
	if a.Notifications != 1 {
		t.Fatalf("ein Durchlauf erzeugte %d Benachrichtigungen, erwartet 1", a.Notifications)
	}
	offen, err := d.OpenNotifications("anna")
	if err != nil {
		t.Fatal(err)
	}
	if len(offen) != 1 || offen[0].Kind != model.NotifyRequest || offen[0].TaskID != task.ID {
		t.Fatalf("keine Anfrage für Anna: %+v", offen)
	}

	// Und noch einmal: ein zweiter Durchlauf fragt niemanden doppelt.
	if a := laufen(t, mux); a.Notifications != 0 {
		t.Errorf("der zweite Durchlauf erzeugte %d Benachrichtigungen", a.Notifications)
	}
}

// --- Hilfen ------------------------------------------------------------------

func aufbau(t *testing.T) (*http.ServeMux, *db.DB) {
	t.Helper()
	t.Cleanup(clock.Reset)
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	mux := http.NewServeMux()
	if !Register(mux, AuthMode, Config{DB: d, Assignment: vergabe.Config{}}) {
		t.Fatal("Register hat im Testmodus nichts eingehängt")
	}
	return mux, d
}

type laufAntwort struct {
	clockAnswer
	Notifications int `json:"notifications"`
}

func ruf(t *testing.T, mux *http.ServeMux, method, pfad, koerper string, erwartet int) clockAnswer {
	t.Helper()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(method, pfad, strings.NewReader(koerper)))
	if w.Code != erwartet {
		t.Fatalf("%s %s: Status %d, erwartet %d — %s", method, pfad, w.Code, erwartet, w.Body.String())
	}
	var a clockAnswer
	_ = json.Unmarshal(w.Body.Bytes(), &a)
	return a
}

func laufen(t *testing.T, mux *http.ServeMux) laufAntwort {
	t.Helper()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/dev/assignment/run", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("POST /dev/assignment/run: Status %d — %s", w.Code, w.Body.String())
	}
	var a laufAntwort
	if err := json.Unmarshal(w.Body.Bytes(), &a); err != nil {
		t.Fatalf("unlesbare Antwort: %v", err)
	}
	return a
}

// vormittags stellt die Uhr um so viele Tage vor und legt sie dabei auf
// 10 Uhr Ortszeit — außerhalb der Ruhezeit, damit die Zustellung nicht auf
// den nächsten Morgen wartet.
func vormittags(t *testing.T, mux *http.ServeMux, tage int) {
	t.Helper()
	ort := clock.Now().In(model.Location()).AddDate(0, 0, tage)
	ziel := time.Date(ort.Year(), ort.Month(), ort.Day(), 10, 0, 0, 0, model.Location())
	ruf(t, mux, "POST", "/dev/clock/set", `{"time":"`+ziel.Format(time.RFC3339)+`"}`, http.StatusOK)
}
