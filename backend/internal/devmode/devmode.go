// Package devmode mounts the handful of knobs that exist only while the
// backend runs with AUTH_MODE=insecure-dev — the same switch the developer
// login sits behind.
//
// They exist for one reason: a test that waits is a test that lies. The
// assignment of care tasks is a matter of days and hours, so an end-to-end
// test used to create an overdue task, sign someone up and then sleep for up
// to 150 seconds hoping the background timer would get around to it. That
// test does not assert "an overdue task gets its helper asked", it asserts
// "…within 150 seconds of wall clock" — a property nobody cares about, that
// depends on how busy the machine is, and that made the same commit go green
// on one emulator and red on the other.
//
// With these two knobs the test says what it means instead: move the clock
// to the day the task is due, run one assignment pass, look at the result.
// No sleeping, same answer every time.
//
//	GET  /dev/clock            what time the backend thinks it is
//	POST /dev/clock/set        {"time":"2026-09-06T10:00:00+02:00"}
//	POST /dev/clock/advance    {"duration":"240h"}
//	POST /dev/clock/reset      back to the system clock
//	POST /dev/assignment/run   one synchronous vergabe.Engine.Durchlauf
//
// Reset matters as much as the rest: the clock is process-wide, so a test
// that travels and does not come back hands the next test a village living
// in the future.
//
// In production none of this is registered — not guarded, not 403, simply
// absent. Register refuses to mount unless the auth mode is insecure-dev, so
// even a wrong call site cannot bring the routes into a production process.
package devmode

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/clock"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/vergabe"
)

// AuthMode is the only value of AUTH_MODE that unlocks these routes.
const AuthMode = "insecure-dev"

// Config is what the knobs need to do their work.
type Config struct {
	DB *db.DB
	// Assignment is the very same configuration the background timer runs
	// with. Running a pass by hand has to be the identical piece of work —
	// a test that drives a second, differently configured engine would
	// prove nothing about the one that runs in production.
	Assignment vergabe.Config
}

// Register mounts the dev routes and reports whether it did. It mounts
// nothing unless authMode is insecure-dev.
func Register(mux *http.ServeMux, authMode string, cfg Config) bool {
	if authMode != AuthMode {
		return false
	}
	h := &handler{cfg: cfg}
	mux.HandleFunc("GET /dev/clock", h.clockState)
	mux.HandleFunc("POST /dev/clock/set", h.clockSet)
	mux.HandleFunc("POST /dev/clock/advance", h.clockAdvance)
	mux.HandleFunc("POST /dev/clock/reset", h.clockReset)
	mux.HandleFunc("POST /dev/assignment/run", h.assignmentRun)
	slog.Warn("Test-Endpunkte unter /dev aktiv (AUTH_MODE=insecure-dev) — " +
		"Uhr stellen und Vergabe anstoßen. Im Betrieb gibt es diese Wege nicht.")
	return true
}

type handler struct{ cfg Config }

// clockAnswer is what every clock route replies with, so a caller always
// learns where the clock ended up.
type clockAnswer struct {
	Now    time.Time `json:"now"`
	Offset string    `json:"offset"`
	// OffsetSeconds is the same number for callers that would rather not
	// parse a Go duration.
	OffsetSeconds float64 `json:"offsetSeconds"`
}

func (h *handler) clockState(w http.ResponseWriter, _ *http.Request) {
	antwort(w, http.StatusOK, jetzt())
}

func (h *handler) clockSet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Time string `json:"time"`
	}
	if !lies(w, r, &in) {
		return
	}
	t, err := time.Parse(time.RFC3339, in.Time)
	if err != nil {
		fehler(w, http.StatusBadRequest, "time muss ein Zeitpunkt im Format RFC3339 sein")
		return
	}
	clock.Set(t)
	slog.Warn("Test-Uhr gestellt", "now", clock.Now(), "offset", clock.Offset().String())
	antwort(w, http.StatusOK, jetzt())
}

func (h *handler) clockAdvance(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Duration string `json:"duration"`
	}
	if !lies(w, r, &in) {
		return
	}
	d, err := time.ParseDuration(in.Duration)
	if err != nil {
		fehler(w, http.StatusBadRequest, "duration muss eine Go-Dauer sein, z.B. „240h“")
		return
	}
	clock.Advance(d)
	slog.Warn("Test-Uhr vorgestellt", "um", d.String(), "now", clock.Now(), "offset", clock.Offset().String())
	antwort(w, http.StatusOK, jetzt())
}

func (h *handler) clockReset(w http.ResponseWriter, _ *http.Request) {
	clock.Reset()
	slog.Info("Test-Uhr zurückgesetzt — wieder Systemzeit")
	antwort(w, http.StatusOK, jetzt())
}

// assignmentRun runs exactly one pass of the assignment engine and only
// returns once the notifications are written. That is what makes the waiting
// in the test unnecessary: Durchlauf is synchronous and, by its own
// documentation, repeatable at will — it only works when something is due.
func (h *handler) assignmentRun(w http.ResponseWriter, _ *http.Request) {
	cfg := h.cfg.Assignment
	zaehler := &countingZusteller{weiter: cfg.Zusteller}
	cfg.Zusteller = zaehler
	if err := vergabe.New(h.cfg.DB, cfg).Durchlauf(); err != nil {
		fehler(w, http.StatusInternalServerError, "Vergabe-Durchlauf fehlgeschlagen: "+err.Error())
		return
	}
	antwort(w, http.StatusOK, struct {
		clockAnswer
		Notifications int `json:"notifications"`
	}{jetzt(), zaehler.n})
}

// countingZusteller counts the notifications one pass produced and hands
// them on to the real delivery. One instance per request, so no locking is
// needed — Durchlauf works through its list in a single goroutine.
type countingZusteller struct {
	weiter vergabe.Zusteller
	n      int
}

func (c *countingZusteller) Zustellen(n model.Notification) error {
	c.n++
	if c.weiter == nil {
		return nil
	}
	return c.weiter.Zustellen(n)
}

func jetzt() clockAnswer {
	o := clock.Offset()
	return clockAnswer{Now: clock.Now(), Offset: o.String(), OffsetSeconds: o.Seconds()}
}

// lies decodes a JSON body. An empty body is fine — every field is optional
// until the handler says otherwise.
func lies(w http.ResponseWriter, r *http.Request, ziel any) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	if err := json.NewDecoder(r.Body).Decode(ziel); err != nil {
		fehler(w, http.StatusBadRequest, "unlesbarer JSON-Körper: "+err.Error())
		return false
	}
	return true
}

func antwort(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func fehler(w http.ResponseWriter, status int, text string) {
	antwort(w, status, map[string]string{"error": text})
}
