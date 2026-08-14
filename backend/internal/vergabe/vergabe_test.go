package vergabe

import (
	"errors"
	"net/http"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Alle Proben laufen gegen eine echte SQLite-Datenbank und eine gestellte
// Uhr — Staffelung, Ruhezeiten und Verfall sind so ohne echtes Warten
// prüfbar. Gemockt wird nichts.

// uhr ist die gestellte Zeitquelle. Sie ist gesperrt, damit auch der
// Nebenläufigkeits-Test gefahrlos auf sie zugreifen kann.
type uhr struct {
	mu sync.Mutex
	t  time.Time
}

func (u *uhr) jetzt() time.Time {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.t
}

func (u *uhr) setze(t time.Time) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.t = t
}

func (u *uhr) weiter(d time.Duration) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.t = u.t.Add(d)
}

// sammler merkt sich, was der Zusteller zu sehen bekam.
type sammler struct {
	mu  sync.Mutex
	all []model.Notification
}

func (s *sammler) Zustellen(n model.Notification) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.all = append(s.all, n)
	return nil
}

func (s *sammler) empfaenger(k model.NotificationKind) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []string{}
	for _, n := range s.all {
		if n.Kind == k {
			out = append(out, n.UserSub)
		}
	}
	return out
}

func berlin(t *testing.T, jahr int, monat time.Month, tag, stunde, minute int) time.Time {
	t.Helper()
	return time.Date(jahr, monat, tag, stunde, minute, 0, 0, model.Location())
}

// aufbau legt eine Datenbank mit einem Blumenkasten samt Gießplan an. Die
// Aufgabe ist seit 30 Tagen nicht erledigt, also längst fällig.
func aufbau(t *testing.T, start time.Time) (*db.DB, *Engine, *uhr, *sammler, model.CareTask) {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatalf("db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	p := model.Place{Name: "Unter den Eichen — Kasten 1", Kind: model.PlaceFlowerbox,
		Lat: 52.211, Lon: 9.87, Active: true, CreatedAt: start.AddDate(0, 0, -30)}
	if err := d.InsertPlace(&p); err != nil {
		t.Fatalf("Ort: %v", err)
	}
	zehn := 10.0
	task := model.CareTask{PlaceID: p.ID, Kind: model.TaskWatering, Liters: &zehn,
		IntervalDays: 7, RedAfterDays: 14, Active: true, CreatedAt: start.AddDate(0, 0, -30)}
	if err := d.InsertTask(&task); err != nil {
		t.Fatalf("Aufgabe: %v", err)
	}

	u := &uhr{t: start}
	s := &sammler{}
	e := New(d, Config{Now: u.jetzt, Zusteller: s})
	return d, e, u, s, task
}

// anmelden trägt eine Person für den Ort der Aufgabe ein.
func anmelden(t *testing.T, d *db.DB, task model.CareTask, sub string, seit time.Time) {
	t.Helper()
	s := model.Signup{UserSub: sub, PlaceID: task.PlaceID, CreatedAt: seit}
	if _, err := d.InsertSignup(&s); err != nil {
		t.Fatalf("Anmeldung %s: %v", sub, err)
	}
}

// erledigt trägt eine Erledigung ein (ohne Vergabe-Logik).
func erledigt(t *testing.T, d *db.DB, taskID int64, sub string, wann time.Time) {
	t.Helper()
	c := model.Completion{TaskID: taskID, UserSub: sub, UserName: sub, DoneAt: wann}
	if err := d.InsertCompletion(&c); err != nil {
		t.Fatalf("Erledigung: %v", err)
	}
}

func durchlauf(t *testing.T, e *Engine) {
	t.Helper()
	if err := e.Durchlauf(); err != nil {
		t.Fatalf("Durchlauf: %v", err)
	}
}

// anfragen liefert die Empfänger der Anfragen eines Vorgangs, in der
// Reihenfolge der Zustellung.
func anfragen(t *testing.T, d *db.DB, assignmentID int64) []string {
	t.Helper()
	ns, err := d.NotificationsForAssignment(assignmentID)
	if err != nil {
		t.Fatalf("Benachrichtigungen: %v", err)
	}
	out := []string{}
	for _, n := range ns {
		if n.Kind == model.NotifyRequest {
			out = append(out, n.UserSub)
		}
	}
	return out
}

func vorgang(t *testing.T, d *db.DB, taskID int64) *model.Assignment {
	t.Helper()
	a, err := d.ActiveAssignment(taskID)
	if err != nil {
		t.Fatalf("Vorgang: %v", err)
	}
	return a
}

func gleich(t *testing.T, name string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, erwartet %v", name, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, erwartet %v", name, got, want)
		}
	}
}

// --- Vorgang entsteht -------------------------------------------------------

func TestOhneAngemeldeteKeinVorgang(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, _, s, task := aufbau(t, start)

	durchlauf(t, e)

	if a := vorgang(t, d, task.ID); a != nil {
		t.Fatalf("Vorgang trotz leerer Liste angelegt: %+v", a)
	}
	if len(s.all) != 0 {
		t.Fatalf("Benachrichtigungen ohne Angemeldete: %+v", s.all)
	}
}

func TestGruenerOrtErzeugtKeinenVorgang(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, _, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -10))
	erledigt(t, d, task.ID, "anna", start.Add(-2*time.Hour))

	durchlauf(t, e)

	if a := vorgang(t, d, task.ID); a != nil {
		t.Fatalf("Vorgang für frisch gegossene Aufgabe: %+v", a)
	}
}

// --- Reihenfolge und Staffelung ---------------------------------------------

// Reihenfolge: Wer am längsten nichts erledigt hat bzw. am längsten nicht
// gefragt wurde, kommt zuerst.
func TestReihenfolgeDerAnfragen(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, u, _, task := aufbau(t, start)

	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	anmelden(t, d, task, "bernd", start.AddDate(0, 0, -20))
	anmelden(t, d, task, "clara", start.AddDate(0, 0, -20))

	// Anna hat zuletzt gegossen, Bernd davor, Clara noch nie.
	erledigt(t, d, task.ID, "anna", start.AddDate(0, 0, -25))
	erledigt(t, d, task.ID, "bernd", start.AddDate(0, 0, -27))

	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	if a == nil {
		t.Fatal("kein Vorgang angelegt")
	}
	gleich(t, "erste Anfrage", anfragen(t, d, a.ID), []string{"clara"})

	u.weiter(time.Hour)
	durchlauf(t, e)
	u.weiter(time.Hour)
	durchlauf(t, e)
	gleich(t, "Reihenfolge", anfragen(t, d, a.ID), []string{"clara", "bernd", "anna"})
}

func TestZweiterErstNachEinerStunde(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, u, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	anmelden(t, d, task, "bernd", start.AddDate(0, 0, -19))

	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	if got := anfragen(t, d, a.ID); len(got) != 1 {
		t.Fatalf("sofort %d Anfragen, erwartet genau 1: %v", len(got), got)
	}

	// 59 Minuten später ist der Zweite noch nicht dran.
	u.weiter(59 * time.Minute)
	durchlauf(t, e)
	if got := anfragen(t, d, a.ID); len(got) != 1 {
		t.Fatalf("nach 59 Minuten %d Anfragen, erwartet 1: %v", len(got), got)
	}

	u.weiter(time.Minute)
	durchlauf(t, e)
	if got := anfragen(t, d, a.ID); len(got) != 2 {
		t.Fatalf("nach 60 Minuten %d Anfragen, erwartet 2: %v", len(got), got)
	}
}

// Die Anfrage nennt die Frist, bis zu der man den Vortritt hat.
func TestAnfrageNenntVortrittsfrist(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, _, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))

	durchlauf(t, e)
	ns, err := d.OpenNotifications("anna")
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 1 {
		t.Fatalf("%d offene Benachrichtigungen, erwartet 1", len(ns))
	}
	if ns[0].ExpiresAt == nil || !ns[0].ExpiresAt.Equal(start.Add(time.Hour)) {
		t.Fatalf("Frist = %v, erwartet %v", ns[0].ExpiresAt, start.Add(time.Hour))
	}
}

// --- Ruhezeiten -------------------------------------------------------------

func TestRuhezeitVerschiebtDieZustellung(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 20, 30)
	d, e, u, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	anmelden(t, d, task, "bernd", start.AddDate(0, 0, -19))

	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	gleich(t, "erste Anfrage", anfragen(t, d, a.ID), []string{"anna"})

	// 21:30 wäre der Reihe nach Bernd dran — aber es ist Ruhezeit.
	u.setze(berlin(t, 2026, time.June, 10, 21, 30))
	durchlauf(t, e)
	gleich(t, "in der Ruhezeit", anfragen(t, d, a.ID), []string{"anna"})

	u.setze(berlin(t, 2026, time.June, 11, 6, 59))
	durchlauf(t, e)
	gleich(t, "kurz vor Ruheende", anfragen(t, d, a.ID), []string{"anna"})

	u.setze(berlin(t, 2026, time.June, 11, 7, 0))
	durchlauf(t, e)
	gleich(t, "morgens", anfragen(t, d, a.ID), []string{"anna", "bernd"})
}

// Auch ein Vorgang, der in der Ruhezeit entsteht, wartet bis zum Morgen.
func TestVorgangInDerNachtWartetBisMorgens(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 23, 0)
	d, e, u, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))

	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	if a == nil {
		t.Fatal("kein Vorgang angelegt")
	}
	if got := anfragen(t, d, a.ID); len(got) != 0 {
		t.Fatalf("nachts zugestellt: %v", got)
	}

	u.setze(berlin(t, 2026, time.June, 11, 7, 0))
	durchlauf(t, e)
	gleich(t, "morgens", anfragen(t, d, a.ID), []string{"anna"})
}

// --- Zusage -----------------------------------------------------------------

func TestZusageStopptWeitereAnfragen(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, u, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	anmelden(t, d, task, "bernd", start.AddDate(0, 0, -19))
	anmelden(t, d, task, "clara", start.AddDate(0, 0, -18))

	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	gefragt := anfragen(t, d, a.ID)

	if _, err := e.Zusagen(a.ID, gefragt[0], "Anna"); err != nil {
		t.Fatalf("Zusage: %v", err)
	}

	// Auch Stunden später wird niemand sonst gefragt.
	for i := 0; i < 5; i++ {
		u.weiter(time.Hour)
		durchlauf(t, e)
	}
	gleich(t, "Anfragen trotz Zusage", anfragen(t, d, a.ID), gefragt)

	a = vorgang(t, d, task.ID)
	if a.State != model.AssignmentClaimed || a.ClaimedBy != gefragt[0] {
		t.Fatalf("Vorgang = %+v, erwartet übernommen von %s", a, gefragt[0])
	}
	if a.ClaimedUntil == nil || !a.ClaimedUntil.Equal(start.Add(24*time.Hour)) {
		t.Fatalf("Zusage läuft bis %v, erwartet %v", a.ClaimedUntil, start.Add(24*time.Hour))
	}

	// Die offenen Anfragen der anderen sind mit der Zusage erloschen.
	offen, err := d.OpenNotifications(gefragt[0])
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range offen {
		if n.AssignmentID == a.ID && n.Kind.IsRequest() {
			t.Fatalf("Anfrage blieb nach der Zusage offen: %+v", n)
		}
	}
}

func TestNurEineZusageGewinnt(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, u, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	anmelden(t, d, task, "bernd", start.AddDate(0, 0, -19))
	durchlauf(t, e)
	u.weiter(time.Hour)
	durchlauf(t, e)
	a := vorgang(t, d, task.ID)

	var wg sync.WaitGroup
	fehler := make([]error, 2)
	subs := []string{"anna", "bernd"}
	for i := range subs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, fehler[i] = e.Zusagen(a.ID, subs[i], subs[i])
		}(i)
	}
	wg.Wait()

	erfolge := 0
	for i, err := range fehler {
		if err == nil {
			erfolge++
			continue
		}
		var ab *Abweisung
		if !errors.As(err, &ab) || ab.Status != http.StatusConflict {
			t.Fatalf("%s: unerwarteter Fehler %v", subs[i], err)
		}
	}
	if erfolge != 1 {
		t.Fatalf("%d gleichzeitige Zusagen angenommen, erwartet genau 1", erfolge)
	}
	if a := vorgang(t, d, task.ID); a.ClaimedBy == "" {
		t.Fatal("nach den Zusagen hält niemand den Vorgang")
	}
}

func TestZusageOhneAnmeldungAbgewiesen(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, _, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	durchlauf(t, e)
	a := vorgang(t, d, task.ID)

	if _, err := e.Zusagen(a.ID, "fremder", "Fremder"); err == nil {
		t.Fatal("Zusage einer nicht angemeldeten Person wurde angenommen")
	}
}

func TestZurueckgebenLaeuftSofortWeiter(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, u, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	anmelden(t, d, task, "bernd", start.AddDate(0, 0, -19))
	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	erste := anfragen(t, d, a.ID)[0]
	if _, err := e.Zusagen(a.ID, erste, erste); err != nil {
		t.Fatalf("Zusage: %v", err)
	}

	u.weiter(2 * time.Hour)
	if _, err := e.Zurueckgeben(a.ID, erste, false); err != nil {
		t.Fatalf("Rückgabe: %v", err)
	}
	durchlauf(t, e)

	if got := anfragen(t, d, a.ID); len(got) != 2 || got[0] != erste {
		t.Fatalf("nach der Rückgabe: %v, erwartet zwei Anfragen mit %s zuerst", got, erste)
	}
	if a := vorgang(t, d, task.ID); a.ClaimedBy != "" {
		t.Fatalf("Vorgang hängt weiter an %s", a.ClaimedBy)
	}
}

func TestFremdeRueckgabeNurAlsAdmin(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, _, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	if _, err := e.Zusagen(a.ID, "anna", "Anna"); err != nil {
		t.Fatalf("Zusage: %v", err)
	}

	if _, err := e.Zurueckgeben(a.ID, "bernd", false); err == nil {
		t.Fatal("Fremde durften die Zusage aufheben")
	}
	if _, err := e.Zurueckgeben(a.ID, "verwaltung", true); err != nil {
		t.Fatalf("Admin darf aufheben: %v", err)
	}
	// Anna erfährt davon.
	offen, err := d.OpenNotifications("anna")
	if err != nil {
		t.Fatal(err)
	}
	gefunden := false
	for _, n := range offen {
		if n.Kind == model.NotifyClaimRevoked {
			gefunden = true
		}
	}
	if !gefunden {
		t.Fatalf("kein Hinweis auf die aufgehobene Zusage: %+v", offen)
	}
}

// --- Verfall ----------------------------------------------------------------

func TestZusageVerfaelltUndUeberspringtDenVerfallenen(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, u, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	anmelden(t, d, task, "bernd", start.AddDate(0, 0, -19))
	anmelden(t, d, task, "clara", start.AddDate(0, 0, -18))

	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	erste := anfragen(t, d, a.ID)[0]
	if _, err := e.Zusagen(a.ID, erste, erste); err != nil {
		t.Fatalf("Zusage: %v", err)
	}

	// Kurz vor Ablauf passiert nichts.
	u.weiter(24*time.Hour - time.Minute)
	durchlauf(t, e)
	if a := vorgang(t, d, task.ID); a.State != model.AssignmentClaimed {
		t.Fatalf("Zusage zu früh verfallen: %+v", a)
	}

	u.weiter(time.Minute)
	durchlauf(t, e)

	a = vorgang(t, d, task.ID)
	if a.ClaimedBy != "" || a.State == model.AssignmentClaimed {
		t.Fatalf("Zusage nicht verfallen: %+v", a)
	}
	gefragt := anfragen(t, d, a.ID)
	if len(gefragt) != 2 {
		t.Fatalf("nach dem Verfall %d Anfragen, erwartet 2: %v", len(gefragt), gefragt)
	}
	if gefragt[1] == erste {
		t.Fatalf("der Verfallene wurde erneut gefragt: %v", gefragt)
	}

	// Der Verfallene bekommt einen Hinweis, keine neue Anfrage.
	offen, err := d.OpenNotifications(erste)
	if err != nil {
		t.Fatal(err)
	}
	hinweis := false
	for _, n := range offen {
		if n.Kind == model.NotifyClaimExpired {
			hinweis = true
		}
		if n.Kind.IsRequest() && n.AssignmentID == a.ID {
			t.Fatalf("Verfallener hat wieder eine offene Anfrage: %+v", n)
		}
	}
	if !hinweis {
		t.Fatalf("kein Hinweis auf die verfallene Zusage: %+v", offen)
	}
}

// --- Rundruf ----------------------------------------------------------------

func TestRundrufWennDieListeDurchIst(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, u, s, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	anmelden(t, d, task, "bernd", start.AddDate(0, 0, -19))

	durchlauf(t, e)
	u.weiter(time.Hour)
	durchlauf(t, e)
	u.weiter(time.Hour)
	durchlauf(t, e)

	a := vorgang(t, d, task.ID)
	if a.State != model.AssignmentBroadcast {
		t.Fatalf("Stand = %s, erwartet rundruf", a.State)
	}
	empf := s.empfaenger(model.NotifyBroadcast)
	if len(empf) != 2 {
		t.Fatalf("Rundruf an %v, erwartet beide Angemeldeten", empf)
	}

	// Der Rundruf geht nur einmal raus.
	u.weiter(3 * time.Hour)
	durchlauf(t, e)
	if empf := s.empfaenger(model.NotifyBroadcast); len(empf) != 2 {
		t.Fatalf("Rundruf wiederholt: %v", empf)
	}
}

func TestRundrufLaesstDenVerfallenenAus(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, u, s, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	anmelden(t, d, task, "bernd", start.AddDate(0, 0, -19))

	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	erste := anfragen(t, d, a.ID)[0]
	if _, err := e.Zusagen(a.ID, erste, erste); err != nil {
		t.Fatalf("Zusage: %v", err)
	}
	u.weiter(24 * time.Hour)
	durchlauf(t, e) // Verfall + Anfrage an den Zweiten
	u.weiter(time.Hour)
	durchlauf(t, e) // Liste durch → Rundruf

	empf := s.empfaenger(model.NotifyBroadcast)
	for _, sub := range empf {
		if sub == erste {
			t.Fatalf("Verfallener im Rundruf: %v", empf)
		}
	}
	if len(empf) != 1 {
		t.Fatalf("Rundruf an %v, erwartet nur den Zweiten", empf)
	}
}

// --- Beenden ----------------------------------------------------------------

func TestFremdeErledigungBeendetAllesSofort(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, u, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	anmelden(t, d, task, "bernd", start.AddDate(0, 0, -19))

	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	vorher := anfragen(t, d, a.ID)

	// Jemand ganz anderes gießt (Verwaltung per MCP, Nachbar ohne Anmeldung).
	erledigt(t, d, task.ID, "hausmeister", u.jetzt())
	if err := e.Beenden(task.ID, model.EndDone, "hausmeister"); err != nil {
		t.Fatalf("Beenden: %v", err)
	}

	if a := vorgang(t, d, task.ID); a != nil {
		t.Fatalf("Vorgang läuft weiter: %+v", a)
	}
	for _, sub := range []string{"anna", "bernd"} {
		offen, err := d.OpenNotifications(sub)
		if err != nil {
			t.Fatal(err)
		}
		for _, n := range offen {
			if n.Kind.IsRequest() {
				t.Fatalf("%s hat nach der Erledigung noch eine Anfrage: %+v", sub, n)
			}
		}
	}

	// Und es kommt auch später nichts mehr nach.
	u.weiter(2 * time.Hour)
	durchlauf(t, e)
	if got := anfragen(t, d, a.ID); len(got) != len(vorher) {
		t.Fatalf("nach der Erledigung weitere Anfragen: %v", got)
	}
	if a := vorgang(t, d, task.ID); a != nil {
		t.Fatalf("neuer Vorgang trotz Erledigung: %+v", a)
	}
}

func TestErledigungDurchAndereMeldetDemZusagendenAb(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, _, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	if _, err := e.Zusagen(a.ID, "anna", "Anna"); err != nil {
		t.Fatalf("Zusage: %v", err)
	}

	erledigt(t, d, task.ID, "bernd", start.Add(time.Hour))
	if err := e.Beenden(task.ID, model.EndDone, "bernd"); err != nil {
		t.Fatalf("Beenden: %v", err)
	}

	offen, err := d.OpenNotifications("anna")
	if err != nil {
		t.Fatal(err)
	}
	gefunden := false
	for _, n := range offen {
		if n.Kind == model.NotifyAssignmentDone {
			gefunden = true
		}
	}
	if !gefunden {
		t.Fatalf("Zusagende erfährt nicht von der fremden Erledigung: %+v", offen)
	}
}

func TestStillgelegtesBeendetDenVorgang(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, _, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	durchlauf(t, e)
	if vorgang(t, d, task.ID) == nil {
		t.Fatal("kein Vorgang angelegt")
	}

	task.Active = false
	if err := d.UpdateTask(&task); err != nil {
		t.Fatal(err)
	}
	durchlauf(t, e)

	if a := vorgang(t, d, task.ID); a != nil {
		t.Fatalf("Vorgang an stillgelegter Aufgabe: %+v", a)
	}
}

// --- Anmeldung während eines laufenden Vorgangs -----------------------------

func TestSpaeteAnmeldungWirdMitgefragt(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, u, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))

	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	gleich(t, "erste Anfrage", anfragen(t, d, a.ID), []string{"anna"})

	// Bernd meldet sich an, während der Vorgang läuft.
	u.weiter(30 * time.Minute)
	anmelden(t, d, task, "bernd", u.jetzt())
	u.weiter(30 * time.Minute)
	durchlauf(t, e)

	gleich(t, "nach später Anmeldung", anfragen(t, d, a.ID), []string{"anna", "bernd"})
}

// Wer sich abmeldet, wird nicht mehr gefragt.
func TestAbmeldungNimmtAusDerWarteschlange(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, u, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	anmelden(t, d, task, "bernd", start.AddDate(0, 0, -19))

	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	erste := anfragen(t, d, a.ID)[0]
	zweiter := "bernd"
	if erste == "bernd" {
		zweiter = "anna"
	}
	if _, err := d.DeleteSignups(zweiter, task.PlaceID, ""); err != nil {
		t.Fatalf("Abmeldung: %v", err)
	}

	u.weiter(time.Hour)
	durchlauf(t, e)
	gleich(t, "nach der Abmeldung", anfragen(t, d, a.ID), []string{erste})
}

// --- Anmeldung nur für eine Aufgabenart -------------------------------------

func TestAnmeldungNurFuersGiessen(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, _, _, task := aufbau(t, start)

	jaeten := model.CareTask{PlaceID: task.PlaceID, Kind: model.TaskWeeding,
		IntervalDays: 21, RedAfterDays: 35, Active: true, CreatedAt: start.AddDate(0, 0, -40)}
	if err := d.InsertTask(&jaeten); err != nil {
		t.Fatal(err)
	}
	s := model.Signup{UserSub: "anna", PlaceID: task.PlaceID, TaskKind: model.TaskWatering,
		CreatedAt: start.AddDate(0, 0, -20)}
	if _, err := d.InsertSignup(&s); err != nil {
		t.Fatal(err)
	}

	durchlauf(t, e)

	if a := vorgang(t, d, task.ID); a == nil {
		t.Fatal("kein Vorgang fürs Gießen")
	}
	if a := vorgang(t, d, jaeten.ID); a != nil {
		t.Fatalf("Vorgang fürs Jäten trotz Einschränkung auf giessen: %+v", a)
	}
}

// --- Zusteller --------------------------------------------------------------

// Die Vergabe entscheidet, wer wann etwas erfahren soll, und übergibt das
// Ergebnis einem Zusteller. Heute ist das die Abrufliste in der Datenbank;
// ein Push-Weg lässt sich danebensetzen, ohne die Vergabelogik zu ändern.
func TestZustellerBekommtJedeBenachrichtigung(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, _, s, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))

	durchlauf(t, e)

	if len(s.all) != 1 {
		t.Fatalf("%d Zustellungen, erwartet 1", len(s.all))
	}
	n := s.all[0]
	if n.UserSub != "anna" || n.Kind != model.NotifyRequest {
		t.Fatalf("Zustellung = %+v", n)
	}
	if n.PlaceName == "" || n.TaskName == "" || n.Text == "" {
		t.Fatalf("Zustellung ohne Klartext: %+v", n)
	}
}

func TestBestaetigenSchliesstHinweiseAberNichtAnfragen(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, _, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	durchlauf(t, e)

	offen, err := e.OffeneBenachrichtigungen("anna")
	if err != nil {
		t.Fatal(err)
	}
	if len(offen) != 1 {
		t.Fatalf("%d offene Benachrichtigungen, erwartet 1", len(offen))
	}
	if err := e.Bestaetigen(offen[0].ID, "anna"); err != nil {
		t.Fatalf("Bestätigen: %v", err)
	}
	nachher, err := e.OffeneBenachrichtigungen("anna")
	if err != nil {
		t.Fatal(err)
	}
	if len(nachher) != 1 || nachher[0].AcknowledgedAt == nil {
		t.Fatalf("Anfrage nach dem Bestätigen: %+v", nachher)
	}

	// Ein Hinweis dagegen verschwindet, sobald er angekommen ist.
	a := vorgang(t, d, task.ID)
	if _, err := e.Zusagen(a.ID, "anna", "Anna"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Zurueckgeben(a.ID, "verwaltung", true); err != nil {
		t.Fatal(err)
	}
	offen, err = e.OffeneBenachrichtigungen("anna")
	if err != nil {
		t.Fatal(err)
	}
	var hinweis int64
	for _, n := range offen {
		if n.Kind == model.NotifyClaimRevoked {
			hinweis = n.ID
		}
	}
	if hinweis == 0 {
		t.Fatalf("kein Hinweis vorhanden: %+v", offen)
	}
	if err := e.Bestaetigen(hinweis, "anna"); err != nil {
		t.Fatalf("Bestätigen: %v", err)
	}
	nachher, _ = e.OffeneBenachrichtigungen("anna")
	for _, n := range nachher {
		if n.ID == hinweis {
			t.Fatalf("bestätigter Hinweis steht noch offen: %+v", n)
		}
	}
}

func TestFremdeBenachrichtigungNichtBestaetigbar(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, _, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	durchlauf(t, e)
	offen, err := d.OpenNotifications("anna")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Bestaetigen(offen[0].ID, "bernd"); err == nil {
		t.Fatal("fremde Benachrichtigung ließ sich bestätigen")
	}
}

// Wird eine Aufgabe stillgelegt, während jemand zugesagt hat, soll diese
// Person das erfahren — sonst zieht sie mit der Gießkanne los.
func TestStilllegungMeldetSichBeimZusagenden(t *testing.T) {
	start := berlin(t, 2026, time.June, 10, 9, 0)
	d, e, _, _, task := aufbau(t, start)
	anmelden(t, d, task, "anna", start.AddDate(0, 0, -20))
	durchlauf(t, e)
	a := vorgang(t, d, task.ID)
	if _, err := e.Zusagen(a.ID, "anna", "Anna"); err != nil {
		t.Fatal(err)
	}

	task.Active = false
	if err := d.UpdateTask(&task); err != nil {
		t.Fatal(err)
	}
	durchlauf(t, e)

	// Abgeholt wird der Hinweis wie in der App — mit fertigem Text.
	offen, err := e.OffeneBenachrichtigungen("anna")
	if err != nil {
		t.Fatal(err)
	}
	gefunden := false
	for _, n := range offen {
		if n.Kind == model.NotifyAssignmentDropped {
			gefunden = true
			if n.Text == "" || n.Title == "" {
				t.Error("Hinweis ohne Klartext")
			}
		}
	}
	if !gefunden {
		t.Fatalf("kein Hinweis auf die stillgelegte Aufgabe: %+v", offen)
	}
}
