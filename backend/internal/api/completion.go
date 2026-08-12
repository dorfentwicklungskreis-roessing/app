package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// CompletionInput ist die Eingabe einer Erledigungs-Meldung (REST und MCP).
type CompletionInput struct {
	Liters *float64 `json:"liters"`
	Note   string   `json:"note"`
	// Name: abweichender Melder — nur Admins (telefonisch gemeldeter Vollzug).
	Name string `json:"name"`
	// Force: Spielschutz übergehen — nur Admins.
	Force bool `json:"force"`
	// DoneAt: abweichender Zeitpunkt (RFC3339), nur Admins, höchstens
	// model.MaxBackdate in der Vergangenheit und nie in der Zukunft.
	DoneAt string `json:"doneAt"`
}

// CompletionError trägt den passenden HTTP-Status; RetryAfter ist gesetzt,
// wenn die Meldung nur an der Sperrfrist gescheitert ist.
type CompletionError struct {
	Status     int
	Message    string
	RetryAfter *time.Time
}

func (e *CompletionError) Error() string { return e.Message }

func completionErr(status int, format string, args ...any) *CompletionError {
	return &CompletionError{Status: status, Message: fmt.Sprintf(format, args...)}
}

// CreateCompletion legt eine Erledigung an und setzt dabei den Spielschutz
// durch. Die Regeln sitzen bewusst hier und nicht in der App: sonst ließen
// sie sich mit einem eigenen Client umgehen.
func CreateCompletion(d *db.DB, now time.Time, taskID int64, in CompletionInput, u auth.User) (*model.Completion, error) {
	task, err := d.GetTask(taskID)
	if err != nil {
		return nil, completionErr(http.StatusNotFound, "Aufgabe %d nicht gefunden", taskID)
	}
	if (in.Force || in.DoneAt != "" || in.Name != "") && !u.IsAdmin() {
		return nil, completionErr(http.StatusForbidden,
			"nur Admins dürfen Meldungen nachtragen oder die Sperrfrist übergehen")
	}

	doneAt := now
	if in.DoneAt != "" {
		t, err := time.Parse(time.RFC3339, in.DoneAt)
		if err != nil {
			return nil, completionErr(http.StatusBadRequest, "doneAt muss ein Zeitpunkt im Format RFC3339 sein")
		}
		switch {
		case t.After(now):
			return nil, completionErr(http.StatusBadRequest, "doneAt darf nicht in der Zukunft liegen")
		case now.Sub(t) > model.MaxBackdate:
			return nil, completionErr(http.StatusBadRequest,
				"doneAt liegt zu weit zurück (höchstens %d Tage)", int(model.MaxBackdate.Hours()/24))
		}
		doneAt = t
	}

	if !in.Force {
		last, err := d.LastCompletion(task.ID)
		if err != nil {
			return nil, err
		}
		// Der Hitzefaktor verkürzt die Sperre — im Sommer darf öfter
		// gegossen werden. Für andere Aufgabenarten gilt er nicht.
		factor := 1.0
		if task.Kind == model.TaskWatering {
			factor, _ = d.WateringFactor()
		}
		if model.Blocked(*task, last, doneAt, factor) {
			frei, _ := model.NextAllowed(*task, last, factor)
			return nil, &CompletionError{
				Status: http.StatusConflict,
				Message: fmt.Sprintf("Für diese Aufgabe wurde gerade erst eine Erledigung gemeldet. Wieder möglich ab %s.",
					frei.Local().Format("02.01.2006, 15:04")),
				RetryAfter: &frei,
			}
		}
	}

	name := u.Name
	if in.Name != "" {
		name = in.Name
	}
	c := model.Completion{
		TaskID: task.ID, UserSub: u.Sub, UserName: name,
		Liters: in.Liters, Note: in.Note, DoneAt: doneAt, Forced: in.Force,
	}
	if err := d.InsertCompletion(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

// LockedUntil liefert den Zeitpunkt, bis zu dem der Spielschutz greift —
// oder nil, wenn gerade gemeldet werden darf.
func LockedUntil(task model.CareTask, last *model.Completion, now time.Time, factor float64) *time.Time {
	if !model.Blocked(task, last, now, factor) {
		return nil
	}
	frei, _ := model.NextAllowed(task, last, factor)
	return &frei
}
