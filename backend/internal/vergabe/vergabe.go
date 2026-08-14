// Package vergabe verteilt fällige Pflegeaufgaben an die Angemeldeten.
//
// Der Ablauf in einem Satz: Wird eine Aufgabe fällig und ist mindestens eine
// Person für den Ort angemeldet, entsteht ein Vorgang; die Angemeldeten
// werden nacheinander im Abstand einer Stunde gefragt, wer zusagt, hält den
// Vorgang 24 Stunden, und sobald irgendwer die Erledigung meldet, ist alles
// vorbei.
//
// Die vier Regeln im Einzelnen:
//
//   - Reihenfolge: Wer am längsten nichts erledigt hat bzw. am längsten
//     nicht gefragt wurde, kommt zuerst (model.OrderCandidates). Wer noch
//     nie an der Reihe war, steht ganz vorn; bei Gleichstand entscheidet die
//     ältere Anmeldung.
//   - Staffelung: Zwischen zwei Anfragen liegt eine Stunde. Wer gefragt
//     wurde, behält seine Anfrage — der Vortritt endet, das Zusagen nicht.
//   - Ruhezeiten: Zwischen 21 und 7 Uhr Ortszeit wird nichts zugestellt. Die
//     Staffelung pausiert und läuft morgens weiter.
//   - Verfall: Eine Zusage hält 24 Stunden. Danach wird der Vorgang wieder
//     freigegeben und die Warteschlange läuft weiter — der Verfallene wird
//     nicht erneut gefragt.
//
// Ist die Liste durch, ohne dass jemand zugesagt hat, geht ein Rundruf an
// alle Angemeldeten gleichzeitig (ohne die, deren Zusage verfallen ist).
// Bleibt es dabei, passiert nichts weiter: Der Ort bleibt fällig und wird
// rot wie bisher.
//
// Alle Zeiten kommen aus Config.Now, damit sich Staffelung, Ruhezeiten und
// Verfall ohne echtes Warten prüfen lassen.
package vergabe

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Zusteller bekommt jede neu erzeugte Benachrichtigung zu sehen.
//
// Die Benachrichtigung liegt unabhängig davon in der Datenbank; die App holt
// sie über GET /api/v1/me/notifications ab. Diese Schnittstelle ist der
// Platz für zusätzliche Wege: Ein Push-Dienst wird später schlicht als
// weiterer Zusteller danebengesetzt, ohne dass sich an der Vergabelogik
// etwas ändert.
type Zusteller interface {
	Zustellen(n model.Notification) error
}

// Abruf ist der voreingestellte Zusteller: Die Ablage in der Datenbank ist
// die Zustellung, mehr passiert nicht.
type Abruf struct{}

func (Abruf) Zustellen(model.Notification) error { return nil }

// Config beschreibt den Betrieb der Vergabe.
type Config struct {
	// Now ist die Zeitquelle (Tests stellen die Uhr).
	Now func() time.Time
	// Zusteller bekommt jede neue Benachrichtigung (Vorgabe: Abruf).
	Zusteller Zusteller
	// Takt ist der Abstand der Hintergrund-Durchläufe (Vorgabe 1 Minute).
	Takt time.Duration
}

// DefaultTakt: einmal je Minute reicht — die Staffelung rechnet in Stunden.
const DefaultTakt = time.Minute

type Engine struct {
	db        *db.DB
	now       func() time.Time
	zusteller Zusteller
}

func New(d *db.DB, cfg Config) *Engine {
	e := &Engine{db: d, now: cfg.Now, zusteller: cfg.Zusteller}
	if e.now == nil {
		e.now = time.Now
	}
	if e.zusteller == nil {
		e.zusteller = Abruf{}
	}
	return e
}

// Abweisung ist ein Fehler mit passendem HTTP-Status und deutschem Text.
type Abweisung struct {
	Status  int
	Message string
}

func (a *Abweisung) Error() string { return a.Message }

func abweisung(status int, format string, args ...any) *Abweisung {
	return &Abweisung{Status: status, Message: fmt.Sprintf(format, args...)}
}

// --- Hintergrund-Zeitgeber --------------------------------------------------

// Start lässt die Vergabe im Hintergrund takten. Der gelieferte Kanal wird
// geschlossen, sobald der Takt nach dem Ende des Contexts steht — nach
// demselben Muster wie der Sicherungs-Zeitplan.
func Start(ctx context.Context, d *db.DB, cfg Config) <-chan struct{} {
	if cfg.Takt <= 0 {
		cfg.Takt = DefaultTakt
	}
	e := New(d, cfg)
	fertig := make(chan struct{})
	go func() {
		defer close(fertig)
		t := time.NewTicker(cfg.Takt)
		defer t.Stop()
		lauf := func() {
			if err := e.Durchlauf(); err != nil {
				// Ein misslungener Durchlauf ist ärgerlich, aber kein Grund,
				// den Dorf-Server anzuhalten: Der nächste Takt versucht es
				// erneut, verlorene Zeit holt die Warteschlange nach.
				slog.Error("Vergabe: Durchlauf fehlgeschlagen", "err", err)
			}
		}
		lauf()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				lauf()
			}
		}
	}()
	return fertig
}

// FromEnv liest die Betriebseinstellungen:
//
//	VERGABE=off    schaltet den Zeitgeber ab (dann wird niemand gefragt)
//	VERGABE_TAKT   Prüfabstand als Go-Dauer, z.B. „1m" (Vorgabe 1m)
func FromEnv() (Config, bool) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("VERGABE"))) {
	case "off", "0", "false", "aus", "nein":
		return Config{}, false
	}
	cfg := Config{Takt: DefaultTakt}
	if v := strings.TrimSpace(os.Getenv("VERGABE_TAKT")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.Takt = d
		}
	}
	return cfg, true
}

// --- Ein Durchlauf ----------------------------------------------------------

// Durchlauf schaltet fällige Anfragen frei, lässt Zusagen verfallen und
// schiebt die Warteschlangen weiter. Der Durchlauf ist beliebig oft
// wiederholbar: Er arbeitet nur, wenn wirklich etwas ansteht.
func (e *Engine) Durchlauf() error {
	now := e.now()
	regeln, err := e.db.AssignmentRules()
	if err != nil {
		return err
	}
	if err := e.entfalleneBeenden(now); err != nil {
		return err
	}
	if err := e.zusagenPruefen(now, regeln); err != nil {
		return err
	}
	if err := e.vorgaengeEroeffnen(now, regeln); err != nil {
		return err
	}
	return e.anfragenZustellen(now, regeln)
}

// entfalleneBeenden räumt Vorgänge ab, die ihren Anlass verloren haben:
// erledigt, stillgelegt oder gelöscht.
func (e *Engine) entfalleneBeenden(now time.Time) error {
	laufende, err := e.db.ActiveAssignments()
	if err != nil {
		return err
	}
	for _, a := range laufende {
		task, err := e.db.GetTask(a.TaskID)
		if err != nil {
			// Aufgabe gelöscht: Der Vorgang verschwindet mit ihr (Cascade),
			// hier bleibt nichts zu tun.
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return err
		}
		place, err := e.db.GetPlace(task.PlaceID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		faellig, letzte, err := e.istFaellig(*task, place, now)
		if err != nil {
			return err
		}
		if faellig {
			continue
		}
		grund := model.EndObsolete
		if letzte != nil && letzte.DoneAt.After(a.CreatedAt) {
			grund = model.EndDone
		}
		if err := e.vorgangBeenden(a, grund, "", now); err != nil {
			return err
		}
	}
	return nil
}

// zusagenPruefen lässt abgelaufene Zusagen verfallen und gibt den Vorgang
// wieder frei. Der Verfallene bekommt einen Hinweis — und keine neue Anfrage.
func (e *Engine) zusagenPruefen(now time.Time, regeln model.AssignmentRules) error {
	laufende, err := e.db.ActiveAssignments()
	if err != nil {
		return err
	}
	for _, a := range laufende {
		if a.ClaimedBy == "" || a.ClaimedUntil == nil || now.Before(*a.ClaimedUntil) {
			continue
		}
		hinweis, err := e.benachrichtigen(a, a.ClaimedBy, model.NotifyClaimExpired, nil, now)
		if err != nil {
			return err
		}
		if err := e.db.CloseOpenNotifications(a.ID, now, model.CloseExpired, hinweis.ID); err != nil {
			return err
		}
		if err := e.db.ReleaseAssignment(a.ID, regeln.NextDelivery(now)); err != nil {
			return err
		}
		slog.Info("Vergabe: Zusage verfallen", "vorgang", a.ID, "person", a.ClaimedBy)
	}
	return nil
}

// vorgaengeEroeffnen legt für jede fällige Aufgabe einen Vorgang an — aber
// nur, wenn überhaupt jemand angemeldet ist. Ohne Angemeldete gibt es nichts
// zu vergeben; die Aufgabe wird wie bisher einfach gelb und dann rot.
func (e *Engine) vorgaengeEroeffnen(now time.Time, regeln model.AssignmentRules) error {
	tasks, err := e.db.ListTasks()
	if err != nil {
		return err
	}
	orte, err := e.db.ListPlaces()
	if err != nil {
		return err
	}
	byID := map[int64]model.Place{}
	for _, p := range orte {
		byID[p.ID] = p
	}
	anmeldungen, err := e.db.ListSignups()
	if err != nil {
		return err
	}

	for _, task := range tasks {
		ort, ok := byID[task.PlaceID]
		if !ok {
			continue
		}
		faellig, _, err := e.istFaellig(task, &ort, now)
		if err != nil {
			return err
		}
		if !faellig || !passtJemand(anmeldungen, task) {
			continue
		}
		vorhanden, err := e.db.ActiveAssignment(task.ID)
		if err != nil {
			return err
		}
		if vorhanden != nil {
			continue
		}
		// Der erste Anfragezeitpunkt achtet schon auf die Ruhezeit: Ein
		// Vorgang, der um 23 Uhr entsteht, weckt niemanden.
		naechste := regeln.NextDelivery(now)
		neu := model.Assignment{TaskID: task.ID, State: model.AssignmentOpen,
			CreatedAt: now, NextOfferAt: &naechste}
		if err := e.db.InsertAssignment(&neu); err != nil {
			return err
		}
		slog.Info("Vergabe: Vorgang eröffnet", "vorgang", neu.ID, "aufgabe", task.ID, "ort", ort.Name)
	}
	return nil
}

// anfragenZustellen verschickt die nächste fällige Anfrage je Vorgang.
func (e *Engine) anfragenZustellen(now time.Time, regeln model.AssignmentRules) error {
	laufende, err := e.db.ActiveAssignments()
	if err != nil {
		return err
	}
	for _, a := range laufende {
		if a.State != model.AssignmentOpen || a.ClaimedBy != "" {
			continue
		}
		if a.NextOfferAt == nil || now.Before(*a.NextOfferAt) {
			continue
		}
		task, err := e.db.GetTask(a.TaskID)
		if err != nil {
			continue
		}
		naechster, err := e.naechsterKandidat(a, *task)
		if err != nil {
			return err
		}
		if naechster == nil {
			if err := e.rundruf(a, *task, now); err != nil {
				return err
			}
			continue
		}
		// Vortritt: Bis dahin wird niemand sonst gefragt. Die Anfrage bleibt
		// danach gültig — wer später zusagt, bekommt den Vorgang trotzdem,
		// solange ihn nicht jemand anderes genommen hat.
		frist := now.Add(regeln.OfferInterval)
		if _, err := e.benachrichtigen(a, naechster.UserSub, model.NotifyRequest, &frist, now); err != nil {
			return err
		}
		if err := e.db.SetAssignmentQueue(a.ID, model.AssignmentOpen, zeiger(regeln.NextDelivery(frist))); err != nil {
			return err
		}
	}
	return nil
}

// rundruf fragt am Ende der Liste alle Angemeldeten gleichzeitig. Wessen
// Zusage verfallen ist, bleibt außen vor.
func (e *Engine) rundruf(a model.Assignment, task model.CareTask, now time.Time) error {
	kandidaten, err := e.kandidaten(task)
	if err != nil {
		return err
	}
	verfallen, err := e.verfalleneEmpfaenger(a.ID)
	if err != nil {
		return err
	}
	for _, k := range kandidaten {
		if verfallen[k.UserSub] {
			continue
		}
		if _, err := e.benachrichtigen(a, k.UserSub, model.NotifyBroadcast, nil, now); err != nil {
			return err
		}
	}
	slog.Info("Vergabe: Rundruf", "vorgang", a.ID, "aufgabe", task.ID)
	return e.db.SetAssignmentQueue(a.ID, model.AssignmentBroadcast, nil)
}

// --- Kandidaten -------------------------------------------------------------

// kandidaten liefert alle für diese Aufgabe angemeldeten Personen in der
// fairen Reihenfolge.
func (e *Engine) kandidaten(task model.CareTask) ([]model.Candidate, error) {
	anmeldungen, err := e.db.ListSignupsForPlace(task.PlaceID)
	if err != nil {
		return nil, err
	}
	erledigt, err := e.db.LastCompletionPerUser()
	if err != nil {
		return nil, err
	}
	gefragt, err := e.db.LastRequestPerUser()
	if err != nil {
		return nil, err
	}
	// Je Person zählt die älteste passende Anmeldung (jemand kann für den
	// ganzen Ort und zusätzlich für eine Aufgabenart angemeldet sein).
	proPerson := map[string]model.Candidate{}
	for _, s := range anmeldungen {
		if !s.Matches(task) {
			continue
		}
		vorhanden, schon := proPerson[s.UserSub]
		if schon && !s.CreatedAt.Before(vorhanden.SignedUpAt) {
			continue
		}
		proPerson[s.UserSub] = model.Candidate{
			UserSub: s.UserSub, SignedUpAt: s.CreatedAt,
			LastDone: erledigt[s.UserSub], LastAsked: gefragt[s.UserSub],
		}
	}
	liste := make([]model.Candidate, 0, len(proPerson))
	for _, c := range proPerson {
		liste = append(liste, c)
	}
	return model.OrderCandidates(liste), nil
}

// naechsterKandidat liefert die Person, die als Nächste gefragt wird — oder
// nil, wenn alle schon dran waren.
func (e *Engine) naechsterKandidat(a model.Assignment, task model.CareTask) (*model.Candidate, error) {
	kandidaten, err := e.kandidaten(task)
	if err != nil {
		return nil, err
	}
	schonGefragt, err := e.empfaenger(a.ID)
	if err != nil {
		return nil, err
	}
	for _, k := range kandidaten {
		if !schonGefragt[k.UserSub] {
			return &k, nil
		}
	}
	return nil, nil
}

// empfaenger sind alle, die in diesem Vorgang schon etwas bekommen haben.
func (e *Engine) empfaenger(assignmentID int64) (map[string]bool, error) {
	ns, err := e.db.NotificationsForAssignment(assignmentID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, n := range ns {
		out[n.UserSub] = true
	}
	return out, nil
}

// verfalleneEmpfaenger sind die, deren Zusage in diesem Vorgang verfallen ist.
func (e *Engine) verfalleneEmpfaenger(assignmentID int64) (map[string]bool, error) {
	ns, err := e.db.NotificationsForAssignment(assignmentID)
	if err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for _, n := range ns {
		if n.Kind == model.NotifyClaimExpired {
			out[n.UserSub] = true
		}
	}
	return out, nil
}

func passtJemand(anmeldungen []model.Signup, task model.CareTask) bool {
	for _, s := range anmeldungen {
		if s.Matches(task) {
			return true
		}
	}
	return false
}

// --- Zusagen, Zurückgeben, Beenden ------------------------------------------

// Zusagen übernimmt einen Vorgang für die Dauer der Zusagefrist.
//
// Zusagen darf, wer eine offene Anfrage hat oder für den Ort angemeldet ist
// — auch ohne Anfrage: Wer in der App sieht, dass etwas ansteht, soll
// zugreifen dürfen. Die Staffelung regelt, wen wir von uns aus ansprechen,
// nicht, wer helfen darf.
func (e *Engine) Zusagen(assignmentID int64, userSub, userName string) (*model.Assignment, error) {
	now := e.now()
	regeln, err := e.db.AssignmentRules()
	if err != nil {
		return nil, err
	}
	a, err := e.db.GetAssignment(assignmentID)
	if err != nil {
		return nil, abweisung(http.StatusNotFound, "Diesen Vorgang gibt es nicht (mehr).")
	}
	if !a.Active() {
		return nil, abweisung(http.StatusConflict, "Dieser Vorgang ist schon abgeschlossen.")
	}
	if a.ClaimedBy != "" {
		return nil, e.schonVergeben(a)
	}
	darf, err := e.darfZusagen(a, userSub)
	if err != nil {
		return nil, err
	}
	if !darf {
		return nil, abweisung(http.StatusForbidden,
			"Für diesen Ort bist du nicht angemeldet — melde dich zuerst zum Mithelfen an.")
	}

	ok, err := e.db.ClaimAssignment(a.ID, userSub, userName, now, now.Add(regeln.ClaimDuration))
	if err != nil {
		return nil, err
	}
	if !ok {
		// Zwischen Prüfen und Setzen war jemand schneller.
		aktuell, ferr := e.db.GetAssignment(a.ID)
		if ferr != nil {
			return nil, ferr
		}
		return nil, e.schonVergeben(aktuell)
	}
	if err := e.db.CloseOpenNotifications(a.ID, now, model.CloseClaimed); err != nil {
		return nil, err
	}
	slog.Info("Vergabe: Zusage", "vorgang", a.ID, "person", userSub)
	return e.assignmentMitNamen(a.ID)
}

func (e *Engine) schonVergeben(a *model.Assignment) error {
	if a == nil {
		return abweisung(http.StatusConflict, "Dieser Vorgang ist bereits vergeben.")
	}
	if !a.Active() {
		return abweisung(http.StatusConflict, "Dieser Vorgang ist schon abgeschlossen.")
	}
	name := nameOder(e.namen(), a.ClaimedBy, a.ClaimedByName)
	if name == "" || name == unbekannt {
		name = "jemand anderem"
	}
	bis := ""
	if a.ClaimedUntil != nil {
		bis = " (bis " + a.ClaimedUntil.In(model.Location()).Format("02.01.2006, 15:04") + ")"
	}
	return abweisung(http.StatusConflict, "Diese Aufgabe wurde gerade schon von %s übernommen%s.", name, bis)
}

func (e *Engine) darfZusagen(a *model.Assignment, userSub string) (bool, error) {
	ns, err := e.db.NotificationsForAssignment(a.ID)
	if err != nil {
		return false, err
	}
	for _, n := range ns {
		if n.UserSub == userSub && n.Kind.IsRequest() {
			return true, nil
		}
	}
	task, err := e.db.GetTask(a.TaskID)
	if err != nil {
		return false, nil
	}
	anmeldungen, err := e.db.ListSignupsByUser(userSub)
	if err != nil {
		return false, err
	}
	for _, s := range anmeldungen {
		if s.Matches(*task) {
			return true, nil
		}
	}
	return false, nil
}

// Zurueckgeben gibt eine Zusage zurück — freiwillig durch die Person selbst
// oder durch die Verwaltung. Danach läuft die Warteschlange sofort weiter.
func (e *Engine) Zurueckgeben(assignmentID int64, userSub string, istAdmin bool) (*model.Assignment, error) {
	now := e.now()
	regeln, err := e.db.AssignmentRules()
	if err != nil {
		return nil, err
	}
	a, err := e.db.GetAssignment(assignmentID)
	if err != nil {
		return nil, abweisung(http.StatusNotFound, "Diesen Vorgang gibt es nicht (mehr).")
	}
	if !a.Active() {
		return nil, abweisung(http.StatusConflict, "Dieser Vorgang ist schon abgeschlossen.")
	}
	if a.ClaimedBy == "" {
		return nil, abweisung(http.StatusConflict, "Für diesen Vorgang liegt gar keine Zusage vor.")
	}
	fremd := a.ClaimedBy != userSub
	if fremd && !istAdmin {
		return nil, abweisung(http.StatusForbidden, "Nur die zusagende Person selbst kann die Zusage zurückgeben.")
	}
	if fremd {
		// Wem die Verwaltung die Zusage nimmt, der erfährt davon.
		if _, err := e.benachrichtigen(*a, a.ClaimedBy, model.NotifyClaimRevoked, nil, now); err != nil {
			return nil, err
		}
	}
	if err := e.db.ReleaseAssignment(a.ID, regeln.NextDelivery(now)); err != nil {
		return nil, err
	}
	slog.Info("Vergabe: Zusage zurückgegeben", "vorgang", a.ID, "person", a.ClaimedBy, "durch", userSub)
	return e.assignmentMitNamen(a.ID)
}

// Beenden schließt den laufenden Vorgang einer Aufgabe samt aller offenen
// Anfragen. Wird aufgerufen, sobald irgendwer die Erledigung meldet — auch
// jemand ohne Zusage und auch die Verwaltung über MCP.
//
// melder darf leer sein; er selbst bekommt keinen Hinweis.
func (e *Engine) Beenden(taskID int64, grund, melder string) error {
	a, err := e.db.ActiveAssignment(taskID)
	if err != nil || a == nil {
		return err
	}
	return e.vorgangBeenden(*a, grund, melder, e.now())
}

func (e *Engine) vorgangBeenden(a model.Assignment, grund, melder string, now time.Time) error {
	var ausser []int64
	// Wer zugesagt hatte und nicht selbst gemeldet hat, soll wissen, dass er
	// nicht mehr losziehen muss.
	if grund == model.EndDone && a.ClaimedBy != "" && a.ClaimedBy != melder {
		hinweis, err := e.benachrichtigen(a, a.ClaimedBy, model.NotifyAssignmentDone, nil, now)
		if err != nil {
			return err
		}
		ausser = append(ausser, hinweis.ID)
	}
	schliessgrund := model.CloseObsolete
	if grund == model.EndDone {
		schliessgrund = model.CloseDone
	}
	if err := e.db.CloseOpenNotifications(a.ID, now, schliessgrund, ausser...); err != nil {
		return err
	}
	if err := e.db.EndAssignment(a.ID, now, grund); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	slog.Info("Vergabe: Vorgang beendet", "vorgang", a.ID, "grund", grund)
	return nil
}

// --- Benachrichtigungen -----------------------------------------------------

// benachrichtigen legt eine Zustellung an und übergibt sie dem Zusteller.
func (e *Engine) benachrichtigen(a model.Assignment, userSub string, kind model.NotificationKind,
	frist *time.Time, now time.Time,
) (*model.Notification, error) {
	task, err := e.db.GetTask(a.TaskID)
	if err != nil {
		return nil, err
	}
	n := model.Notification{
		AssignmentID: a.ID, TaskID: task.ID, PlaceID: task.PlaceID, UserSub: userSub,
		Kind: kind, CreatedAt: now, ExpiresAt: frist,
	}
	if err := e.db.InsertNotification(&n); err != nil {
		return nil, err
	}
	angereichert, err := e.anreichern(n)
	if err != nil {
		return nil, err
	}
	if err := e.zusteller.Zustellen(*angereichert); err != nil {
		// Ein fehlgeschlagener Zustellweg darf die Vergabe nicht anhalten —
		// die Benachrichtigung steht in der Datenbank und wird abgeholt.
		slog.Warn("Vergabe: Zustellung fehlgeschlagen", "benachrichtigung", n.ID, "err", err)
	}
	return &n, nil
}

// OffeneBenachrichtigungen liefert alles, was für eine Person offen ist —
// mit Ort, Aufgabe und fertigem deutschen Text.
func (e *Engine) OffeneBenachrichtigungen(userSub string) ([]model.Notification, error) {
	ns, err := e.db.OpenNotifications(userSub)
	if err != nil {
		return nil, err
	}
	out := make([]model.Notification, 0, len(ns))
	for _, n := range ns {
		angereichert, err := e.anreichern(n)
		if err != nil {
			return nil, err
		}
		out = append(out, *angereichert)
	}
	return out, nil
}

// Bestaetigen quittiert den Empfang. Hinweise sind damit erledigt und
// verschwinden aus der Liste; Anfragen bleiben stehen, bis der Vorgang sie
// schließt — sonst wäre die Aufgabe aus der App verschwunden, bevor
// jemand zugesagt hat.
func (e *Engine) Bestaetigen(id int64, userSub string) error {
	n, err := e.db.GetNotification(id)
	if err != nil {
		return abweisung(http.StatusNotFound, "Diese Benachrichtigung gibt es nicht.")
	}
	if n.UserSub != userSub {
		return abweisung(http.StatusForbidden, "Das ist nicht deine Benachrichtigung.")
	}
	return e.db.AckNotification(id, e.now(), !n.Kind.IsRequest())
}

// anreichern ergänzt Ort, Aufgabe und den Text, der in der App steht.
func (e *Engine) anreichern(n model.Notification) (*model.Notification, error) {
	task, err := e.db.GetTask(n.TaskID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if task != nil {
		n.TaskName = task.DisplayName()
		n.TaskKind = task.Kind
	}
	place, err := e.db.GetPlace(n.PlaceID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if place != nil {
		n.PlaceName = place.Name
	}
	regeln, err := e.db.AssignmentRules()
	if err != nil {
		return nil, err
	}
	n.Title, n.Text = texte(n, regeln)
	return &n, nil
}

// texte formuliert Überschrift und Text einer Benachrichtigung.
func texte(n model.Notification, regeln model.AssignmentRules) (titel, text string) {
	wo := n.TaskName + " an " + zitat(n.PlaceName)
	stunden := int(regeln.ClaimDuration.Hours())
	switch n.Kind {
	case model.NotifyRequest:
		return wo + " ist dran",
			"Du bist als Nächste(r) an der Reihe: " + wo + ". Wenn du zusagst, hast du " +
				fmt.Sprintf("%d Stunden", stunden) + " Zeit."
	case model.NotifyBroadcast:
		return wo + " sucht noch jemanden",
			"Bisher hat niemand zugesagt: " + wo + " ist weiterhin offen. Wer kann?"
	case model.NotifyClaimExpired:
		return "Zusage abgelaufen",
			"Deine Zusage für " + wo + " ist abgelaufen. Die Aufgabe wurde wieder freigegeben."
	case model.NotifyClaimRevoked:
		return "Zusage aufgehoben",
			"Die Verwaltung hat deine Zusage für " + wo + " aufgehoben."
	case model.NotifyAssignmentDone:
		return "Schon erledigt",
			wo + " wurde bereits erledigt — du musst nichts mehr tun. Danke trotzdem!"
	}
	return wo, wo
}

// --- Stand für Verwaltung und MCP -------------------------------------------

// Stand ist die vollständige Sicht auf die Vergabe einer Aufgabe.
type Stand struct {
	Task model.CareTask `json:"task"`
	// TaskName ist der Anzeigename der Aufgabe.
	TaskName  string `json:"taskName"`
	PlaceID   int64  `json:"placeId"`
	PlaceName string `json:"placeName"`
	// Angemeldete sind alle, die hier mithelfen wollen (mit Namen).
	Angemeldete []model.Signup `json:"signups"`
	// Vorgang ist der laufende Vergabe-Vorgang (nil = gerade keiner).
	Vorgang *model.Assignment `json:"assignment,omitempty"`
	// Zustellungen sind die Anfragen und Hinweise des laufenden Vorgangs.
	Zustellungen []model.Notification `json:"notifications"`
}

// Stand liefert den Vergabestand einer Aufgabe.
func (e *Engine) Stand(taskID int64) (*Stand, error) {
	task, err := e.db.GetTask(taskID)
	if err != nil {
		return nil, abweisung(http.StatusNotFound, "Diese Aufgabe gibt es nicht.")
	}
	place, err := e.db.GetPlace(task.PlaceID)
	if err != nil {
		return nil, abweisung(http.StatusNotFound, "Den Ort dieser Aufgabe gibt es nicht.")
	}
	namen := e.namen()
	st := &Stand{Task: *task, TaskName: task.DisplayName(), PlaceID: place.ID, PlaceName: place.Name,
		Angemeldete: []model.Signup{}, Zustellungen: []model.Notification{}}

	anmeldungen, err := e.db.ListSignupsForPlace(task.PlaceID)
	if err != nil {
		return nil, err
	}
	for _, s := range anmeldungen {
		if !s.Matches(*task) {
			continue
		}
		s.UserName = nameOder(namen, s.UserSub, "")
		s.PlaceName = place.Name
		st.Angemeldete = append(st.Angemeldete, s)
	}

	a, err := e.db.ActiveAssignment(taskID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return st, nil
	}
	a.ClaimedByName = nameOder(namen, a.ClaimedBy, a.ClaimedByName)
	st.Vorgang = a
	ns, err := e.db.NotificationsForAssignment(a.ID)
	if err != nil {
		return nil, err
	}
	for _, n := range ns {
		n.PlaceName = place.Name
		n.TaskName = task.DisplayName()
		n.TaskKind = task.Kind
		n.Title = nameOder(namen, n.UserSub, "")
		st.Zustellungen = append(st.Zustellungen, n)
	}
	return st, nil
}

// AssignmentFor liefert den laufenden Vorgang einer Aufgabe mit aufgelöstem
// Namen — für die Orts-Liste in API und App.
func (e *Engine) AssignmentFor(taskID int64) (*model.Assignment, error) {
	a, err := e.db.ActiveAssignment(taskID)
	if err != nil || a == nil {
		return nil, err
	}
	a.ClaimedByName = nameOder(e.namen(), a.ClaimedBy, a.ClaimedByName)
	return a, nil
}

func (e *Engine) assignmentMitNamen(id int64) (*model.Assignment, error) {
	a, err := e.db.GetAssignment(id)
	if err != nil {
		return nil, err
	}
	a.ClaimedByName = nameOder(e.namen(), a.ClaimedBy, a.ClaimedByName)
	return a, nil
}

// namen liefert die Namensauflösung über die Profile. Wie überall sonst
// gilt: Der Name kommt aus dem Profil, nicht aus dem, was irgendwann einmal
// eingefroren wurde.
func (e *Engine) namen() model.NameResolver {
	namen, err := e.db.NameResolver()
	if err != nil {
		return model.NameResolver{}
	}
	return namen
}

// unbekannt steht dort, wo jemand (noch) kein Profil hat.
const unbekannt = "Unbekannt"

// nameOder löst den Namen auf; gespeichert ist der Rückfall (z.B. der Name,
// der bei der Zusage galt).
func nameOder(namen model.NameResolver, sub, gespeichert string) string {
	if sub == "" {
		return ""
	}
	if n := namen.Resolve(sub, gespeichert); n != "" {
		return n
	}
	return unbekannt
}

// --- Hilfen -----------------------------------------------------------------

// istFaellig sagt, ob an einer Aufgabe gerade etwas zu tun ist (gelb oder
// rot, aktiv, an einem aktiven Ort). Der zweite Rückgabewert ist die letzte
// Erledigung.
func (e *Engine) istFaellig(task model.CareTask, place *model.Place, now time.Time) (bool, *model.Completion, error) {
	letzte, err := e.db.LastCompletion(task.ID)
	if err != nil {
		return false, nil, err
	}
	if !task.Active || place == nil || !place.Active {
		return false, letzte, nil
	}
	faktor := 1.0
	if task.Kind == model.TaskWatering {
		f, err := e.db.WateringFactor()
		if err != nil {
			return false, letzte, err
		}
		faktor = f
	}
	status, _, _ := model.ComputeStatus(task, letzte, now, faktor)
	return status != model.StatusGreen, letzte, nil
}

func zeiger(t time.Time) *time.Time { return &t }

func zitat(s string) string { return "„" + s + "“" }
