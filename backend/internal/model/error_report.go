package model

import "time"

// ErrorReport is what someone sends from the app when something went wrong:
// the app crashed, the server did not answer, an action failed. The person
// sees a plain sentence and presses one button — describing the problem is
// allowed, never required.
//
// Deliberately separate from Idee: a wish is a request that gets answered, a
// report is an observation about a malfunction. In one list the wishes would
// drown in the noise, and neither list would be usable.
//
// Everything here is what the app already knows about itself. There is no
// log file, no screenshot, no location, no device identifier — see
// `store/data-safety.md` and `backend/SICHERHEIT.md`.
type ErrorReport struct {
	ID int64 `json:"id"`
	// Kind says what sort of malfunction it was.
	Kind ErrorReportKind `json:"kind"`
	// Message is the German sentence the person actually read on screen.
	Message string `json:"message"`
	// Detail is the technical context: HTTP status and path, exception class,
	// stack trace of a crash. Never a request or response body.
	Detail string `json:"detail"`
	// Comment is what the person added voluntarily. Usually empty — one tap
	// helps just as much.
	Comment string `json:"comment"`
	// Area names the part of the app it happened in ("Mithelfen", "Anmeldung").
	Area string `json:"area"`
	// Platform is "android" or "ios".
	Platform    string `json:"platform"`
	AppVersion  string `json:"appVersion"`
	OSVersion   string `json:"osVersion"`
	DeviceModel string `json:"deviceModel"`
	// OccurredAt is when it happened on the device — a crash is reported at
	// the next start, so that can be noticeably earlier than CreatedAt.
	OccurredAt time.Time `json:"occurredAt"`
	CreatedAt  time.Time `json:"createdAt"`
	// UserSub and UserName come from the token, not from the client: who
	// reported something is not something an app gets to claim. Both stay
	// empty when the report goes out unauthenticated — which is exactly the
	// case that matters when the login itself is broken.
	UserSub  string `json:"userSub"`
	UserName string `json:"userName"`
	// Status is how far the Dorfentwicklungskreis got with this report.
	Status ErrorReportStatus `json:"status"`
	// Note is an internal remark. It never leaves the administration.
	Note string `json:"note"`
}

// ErrorReportKind sorts the reports roughly, so a list of fifty is still
// readable: a crash is something else than a phone without reception.
type ErrorReportKind string

const (
	// ErrorReportCrash: the app stopped and had to be restarted.
	ErrorReportCrash ErrorReportKind = "crash"
	// ErrorReportNetwork: the server could not be reached at all.
	ErrorReportNetwork ErrorReportKind = "network"
	// ErrorReportServer: the server answered, but with an error.
	ErrorReportServer ErrorReportKind = "server"
	// ErrorReportUnexpected: everything else the app could not handle.
	ErrorReportUnexpected ErrorReportKind = "unexpected"
)

// ErrorReportKinds are all kinds, in the order they are listed.
var ErrorReportKinds = []ErrorReportKind{
	ErrorReportCrash, ErrorReportServer, ErrorReportNetwork, ErrorReportUnexpected,
}

// ValidErrorReportKind checks a kind.
func ValidErrorReportKind(k ErrorReportKind) bool {
	for _, v := range ErrorReportKinds {
		if v == k {
			return true
		}
	}
	return false
}

// ErrorReportKindText names a kind in everyday German.
func ErrorReportKindText(k ErrorReportKind) string {
	switch k {
	case ErrorReportCrash:
		return "Absturz"
	case ErrorReportNetwork:
		return "keine Verbindung"
	case ErrorReportServer:
		return "Server-Fehler"
	default:
		return "unerwartet"
	}
}

// ErrorReportStatus is how far the administration got with a report.
type ErrorReportStatus string

const (
	ErrorReportNew     ErrorReportStatus = "new"
	ErrorReportSeen    ErrorReportStatus = "seen"
	ErrorReportFixed   ErrorReportStatus = "fixed"
	ErrorReportDropped ErrorReportStatus = "dropped"
)

// ErrorReportStatuses are all states in the order they are walked through.
var ErrorReportStatuses = []ErrorReportStatus{
	ErrorReportNew, ErrorReportSeen, ErrorReportFixed, ErrorReportDropped,
}

// ValidErrorReportStatus checks a state.
func ValidErrorReportStatus(s ErrorReportStatus) bool {
	for _, v := range ErrorReportStatuses {
		if v == s {
			return true
		}
	}
	return false
}

// ErrorReportStatusText names a state in everyday German.
func ErrorReportStatusText(s ErrorReportStatus) string {
	switch s {
	case ErrorReportSeen:
		return "angesehen"
	case ErrorReportFixed:
		return "behoben"
	case ErrorReportDropped:
		return "verworfen"
	default:
		return "neu"
	}
}
