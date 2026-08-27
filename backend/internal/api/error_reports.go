package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/httpx"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Error reports from the apps.
//
// The app is under test in the village. When something goes wrong, the person
// is supposed to see it in plain German and be able to send a report with one
// tap — without having to describe anything. Whoever wants to add a sentence
// may; whoever only presses the button helps just as much.
//
// The entrance (POST /api/v1/error-reports) deliberately does NOT require a
// login. That is not convenience: the interesting failures are exactly the
// ones where signing in itself is broken, and a report that cannot be sent
// then is worth nothing. If a token comes along, the report is attached to
// the account — the app cannot claim that itself.
//
// Because the entrance is open, it has its own, tighter access limit. There
// is no honeypot and no minimum typing time as with the ideas: nobody types
// this form, it is filled by the app itself.
const (
	// MaxErrorMessageLen limits the sentence the person read on screen.
	MaxErrorMessageLen = 500
	// MaxErrorDetailLen limits the technical context. Generous, because a
	// crash brings its stack trace along — but not unbounded.
	MaxErrorDetailLen = 8000
	// MaxErrorCommentLen limits what the person adds voluntarily.
	MaxErrorCommentLen = 2000
	// MaxErrorAreaLen limits the name of the part of the app.
	MaxErrorAreaLen = 100
	// MaxErrorVersionLen limits app version, system version and device model.
	MaxErrorVersionLen = 100
	// MaxErrorNoteLen limits the internal remark of the administration.
	MaxErrorNoteLen = 2000
)

// Access limit of the open entrance: ten reports in a row, then ten per hour
// refilling. A crashing app produces a handful of reports, not hundreds.
const (
	ErrorReportBurstDefault   = 10
	ErrorReportPerHourDefault = 10
)

// ErrorReportPlatforms are the surfaces that may report. Deliberately a
// closed list: an unknown value would silently mess up every statistic.
var ErrorReportPlatforms = []string{"android", "ios"}

// --- Registration -------------------------------------------------------------

// registerErrorReports hangs the entrance directly onto the outer router —
// past the login requirement, but with its own access limit. Reading and
// sorting the reports does not happen here at all: that is what the web
// administration and the MCP endpoint are for.
func (s *Server) registerErrorReports(mux *http.ServeMux) {
	var h http.Handler = http.HandlerFunc(s.handleCreateErrorReport)
	if s.OptionalAuth != nil {
		h = s.OptionalAuth(h)
	}
	mux.Handle("POST /api/v1/error-reports", h)
}

// limitForErrorReports returns the access limit of the entrance. Built at
// first use, so it can still be set before. A nil limiter lets everything
// through (see httpx.RateLimiter).
func (s *Server) limitForErrorReports() *httpx.RateLimiter {
	s.errorReportOnce.Do(func() {
		if s.ErrorReportLimiter == nil {
			s.ErrorReportLimiter = httpx.NewRateLimiter(ErrorReportRateLimitFromEnv())
		}
	})
	return s.ErrorReportLimiter
}

// ErrorReportRateLimitFromEnv reads the limit of the report entrance:
//
//	FEHLERBERICHT_LIMIT=off   turns it off (tests, emergency)
//	FEHLERBERICHT_BURST       bucket size (default 10)
//	FEHLERBERICHT_PRO_STUNDE  refill rate per hour (default 10)
//
// RATE_LIMIT=off turns this limit off as well, so one switch stays enough in
// development and E2E.
func ErrorReportRateLimitFromEnv() httpx.RateLimitConfig {
	on := !istAus(os.Getenv("FEHLERBERICHT_LIMIT")) && !istAus(os.Getenv("RATE_LIMIT"))
	return httpx.RateLimitConfig{
		Burst:   umgebungZahl("FEHLERBERICHT_BURST", ErrorReportBurstDefault),
		PerHour: umgebungZahl("FEHLERBERICHT_PRO_STUNDE", ErrorReportPerHourDefault),
		Enabled: &on,
	}
}

// --- Input and validation -----------------------------------------------------

// ErrorReportInput is what an app sends. Everything in here is what the app
// already knows about itself — there is no log file, no screenshot and no
// device identifier in it.
type ErrorReportInput struct {
	// Kind is crash, network, server or unexpected.
	Kind string `json:"kind"`
	// Message is the German sentence that was on screen.
	Message string `json:"message"`
	// Detail is the technical context. Never a request or response body.
	Detail string `json:"detail"`
	// Comment is voluntary and usually empty.
	Comment string `json:"comment"`
	// Area names the part of the app ("Mithelfen", "Anmeldung").
	Area        string `json:"area"`
	Platform    string `json:"platform"`
	AppVersion  string `json:"appVersion"`
	OSVersion   string `json:"osVersion"`
	DeviceModel string `json:"deviceModel"`
	// OccurredAt is when it happened on the device, RFC3339. Missing or
	// unreadable means "now" — a report without a time is still a report.
	OccurredAt string `json:"occurredAt"`
}

// Trim cuts whitespace. Detail and comment keep their line breaks: a stack
// trace without them is unreadable, and people structure their text.
func (in *ErrorReportInput) Trim() {
	in.Kind = strings.ToLower(strings.TrimSpace(in.Kind))
	in.Message = strings.TrimSpace(in.Message)
	in.Detail = strings.TrimSpace(in.Detail)
	in.Comment = strings.TrimSpace(in.Comment)
	in.Area = strings.TrimSpace(in.Area)
	in.Platform = strings.ToLower(strings.TrimSpace(in.Platform))
	in.AppVersion = strings.TrimSpace(in.AppVersion)
	in.OSVersion = strings.TrimSpace(in.OSVersion)
	in.DeviceModel = strings.TrimSpace(in.DeviceModel)
	in.OccurredAt = strings.TrimSpace(in.OccurredAt)
}

// Validate checks the input. The messages are worded so they can stand
// unchanged in front of a person — the app shows what the backend says.
func (in *ErrorReportInput) Validate() error {
	if !model.ValidErrorReportKind(model.ErrorReportKind(in.Kind)) {
		return errors.New("Die Art des Fehlers ist unbekannt.")
	}
	if !gueltigePlattform(in.Platform) {
		return errors.New("Die Plattform ist unbekannt.")
	}
	if in.Message == "" {
		return errors.New("Zu dem Bericht fehlt die Meldung.")
	}
	felder := []struct {
		wert       string
		grenze     int
		mehrzeilig bool
		meldung    string
	}{
		{in.Message, MaxErrorMessageLen, false, "Die Meldung ist zu lang."},
		{in.Detail, MaxErrorDetailLen, true, "Die technischen Angaben sind zu lang."},
		{in.Comment, MaxErrorCommentLen, true, "Deine Ergänzung ist zu lang."},
		{in.Area, MaxErrorAreaLen, false, "Der Bereich ist zu lang."},
		{in.AppVersion, MaxErrorVersionLen, false, "Die App-Version ist zu lang."},
		{in.OSVersion, MaxErrorVersionLen, false, "Die Systemversion ist zu lang."},
		{in.DeviceModel, MaxErrorVersionLen, false, "Die Gerätebezeichnung ist zu lang."},
	}
	for _, f := range felder {
		if utf8.RuneCountInString(f.wert) > f.grenze {
			return errors.New(f.meldung)
		}
		if enthaeltSteuerzeichen(f.wert, f.mehrzeilig) {
			return errors.New("Der Bericht enthält Zeichen, die hier nicht hingehören.")
		}
	}
	return nil
}

func gueltigePlattform(p string) bool {
	for _, v := range ErrorReportPlatforms {
		if v == p {
			return true
		}
	}
	return false
}

// --- Entrance -----------------------------------------------------------------

func (s *Server) handleCreateErrorReport(w http.ResponseWriter, r *http.Request) {
	var in ErrorReportInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "Der Bericht war nicht lesbar.")
		return
	}
	in.Trim()

	if ok, warten := s.limitForErrorReports().Zulassen(r); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(warten.Seconds()))))
		writeErr(w, http.StatusTooManyRequests,
			"Von hier kamen gerade viele Berichte auf einmal. Bitte später noch einmal.")
		return
	}

	if err := in.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	jetzt := s.now()
	bericht := model.ErrorReport{
		Kind: model.ErrorReportKind(in.Kind), Message: in.Message, Detail: in.Detail,
		Comment: in.Comment, Area: in.Area, Platform: in.Platform,
		AppVersion: in.AppVersion, OSVersion: in.OSVersion, DeviceModel: in.DeviceModel,
		OccurredAt: zeitpunktOder(in.OccurredAt, jetzt), CreatedAt: jetzt,
		Status: model.ErrorReportNew,
	}
	// Who reported something is taken from the token, never from the body —
	// otherwise it could simply be claimed. Without a token the report stays
	// anonymous, and that is on purpose: a broken login must still be
	// reportable.
	if u, ok := auth.FromContext(r.Context()); ok && u.Sub != "" {
		bericht.UserSub = u.Sub
		bericht.UserName = u.Name
	}
	if err := s.DB.InsertErrorReport(&bericht); err != nil {
		writeInternal(w, r, err)
		return
	}
	slog.Info("Fehlerbericht eingegangen", "id", bericht.ID, "art", bericht.Kind,
		"plattform", bericht.Platform, "version", bericht.AppVersion)
	// The internal note never goes back out — it does not exist yet at this
	// point, but the shape of the answer should not depend on that.
	antwort := bericht
	antwort.Note = ""
	writeJSON(w, http.StatusCreated, antwort)
}

// zeitpunktOder reads an RFC3339 time stamp. Anything unreadable becomes the
// fallback: a report without a usable time is still worth having.
func zeitpunktOder(roh string, fallback time.Time) time.Time {
	if roh == "" {
		return fallback
	}
	t, err := time.Parse(time.RFC3339, roh)
	if err != nil {
		return fallback
	}
	// A device with a wrong clock must not push the report into the future.
	if t.After(fallback) {
		return fallback
	}
	return t
}

// ErrorReportStatusFrom reads a state from a string. Empty means "all".
func ErrorReportStatusFrom(roh string) (model.ErrorReportStatus, error) {
	roh = strings.TrimSpace(strings.ToLower(roh))
	if roh == "" || roh == "alle" {
		return "", nil
	}
	st := model.ErrorReportStatus(roh)
	if !model.ValidErrorReportStatus(st) {
		return "", errors.New("status muss " + namen(errorReportStatusNames()) + " sein")
	}
	return st, nil
}

// ErrorReportKindFrom reads a kind from a string. Empty means "all".
func ErrorReportKindFrom(roh string) (model.ErrorReportKind, error) {
	roh = strings.TrimSpace(strings.ToLower(roh))
	if roh == "" || roh == "alle" {
		return "", nil
	}
	k := model.ErrorReportKind(roh)
	if !model.ValidErrorReportKind(k) {
		return "", errors.New("art muss " + namen(ErrorReportKindNames()) + " sein")
	}
	return k, nil
}

func errorReportStatusNames() []string {
	out := make([]string, 0, len(model.ErrorReportStatuses))
	for _, s := range model.ErrorReportStatuses {
		out = append(out, string(s))
	}
	return out
}

// ErrorReportKindNames lists the kinds as strings — for schemas and messages.
func ErrorReportKindNames() []string {
	out := make([]string, 0, len(model.ErrorReportKinds))
	for _, k := range model.ErrorReportKinds {
		out = append(out, string(k))
	}
	return out
}

// ErrorReportStatusNames lists the states as strings.
func ErrorReportStatusNames() []string { return errorReportStatusNames() }

func namen(werte []string) string { return strings.Join(werte, ", ") }
