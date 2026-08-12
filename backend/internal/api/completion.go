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
	// Freitexte begrenzen: sie landen unverändert in der Datenbank und auf
	// jeder Ortsseite.
	if err := pruefeText("note", in.Note); err != nil {
		return nil, completionErr(http.StatusBadRequest, "%s", err.Error())
	}
	if err := pruefeText("name", in.Name); err != nil {
		return nil, completionErr(http.StatusBadRequest, "%s", err.Error())
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

	name := u.Name
	if in.Name != "" {
		name = in.Name
	}
	c := model.Completion{
		TaskID: task.ID, UserSub: u.Sub, UserName: name,
		Liters: in.Liters, Note: in.Note, DoneAt: doneAt, Forced: in.Force,
	}

	if in.Force {
		if err := d.InsertCompletion(&c); err != nil {
			return nil, err
		}
		return &c, nil
	}

	// Prüfen und Eintragen gehören zusammen: sonst kommen zwei gleichzeitige
	// Meldungen beide durch die Prüfung, bevor die erste eingetragen ist.
	frei, ok, err := d.InsertCompletionIfFree(&c, TaskCooldown(d, *task))
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &CompletionError{
			Status: http.StatusConflict,
			Message: fmt.Sprintf("Für diese Aufgabe wurde gerade erst eine Erledigung gemeldet. Wieder möglich ab %s.",
				// Ortszeit des Dorfes: der Server läuft in UTC, die Meldung
				// liest aber jemand in Rössing.
				frei.In(model.Location()).Format("02.01.2006, 15:04")),
			RetryAfter: &frei,
		}
	}
	return &c, nil
}

// TaskCooldown liefert die Sperrfrist einer Aufgabe mit der aktuellen
// Einstellung. Der Hitzefaktor verkürzt sie — im Sommer darf öfter gegossen
// werden. Für andere Aufgabenarten (Jäten …) gilt er ausdrücklich nicht.
func TaskCooldown(d *db.DB, task model.CareTask) time.Duration {
	factor := 1.0
	if task.Kind == model.TaskWatering {
		factor, _ = d.WateringFactor()
	}
	return model.CooldownFor(task, factor)
}

// TaskLockedUntil liefert das Ende der Sperrfrist einer Aufgabe (oder nil,
// wenn gerade gemeldet werden darf). Für Aufrufer, die nur die Aufgabe
// kennen — etwa die Verwaltung.
func TaskLockedUntil(d *db.DB, task model.CareTask, now time.Time) (*time.Time, error) {
	last, err := d.LastCompletion(task.ID)
	if err != nil || last == nil {
		return nil, err
	}
	frei := last.DoneAt.Add(TaskCooldown(d, task))
	if !now.Before(frei) {
		return nil, nil
	}
	return &frei, nil
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
