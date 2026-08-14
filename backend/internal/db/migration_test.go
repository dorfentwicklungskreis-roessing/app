package db

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Migrationstest gegen eine Datenbank im ALTEN Schema.
//
// Im Cluster liegt eine gefüllte SQLite-Datei mit den echten Blumenkästen des
// Dorfes. Sie muss die Umstellung überstehen: Alle Bestandsdaten bleiben,
// alle Bestandsaufgaben laufen als regelmäßige Aufgaben weiter, und die
// Benachrichtigungen hängen danach nicht mehr am Vorgang (siehe unten).

// altesSchema ist das Schema, wie es vor den einmaligen Aufgaben aussah —
// wörtlich aus der bisherigen migrate() übernommen, gekürzt auf die
// Tabellen, um die es hier geht.
const altesSchema = `
CREATE TABLE places (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  kind        TEXT NOT NULL DEFAULT 'blumenkasten',
  lat         REAL NOT NULL,
  lon         REAL NOT NULL,
  active      INTEGER NOT NULL DEFAULT 1,
  created_at  TEXT NOT NULL
);
CREATE TABLE care_tasks (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  place_id       INTEGER NOT NULL REFERENCES places(id) ON DELETE CASCADE,
  kind           TEXT NOT NULL,
  title          TEXT NOT NULL DEFAULT '',
  liters         REAL,
  interval_days  REAL NOT NULL,
  red_after_days REAL NOT NULL,
  active         INTEGER NOT NULL DEFAULT 1,
  created_at     TEXT NOT NULL
);
CREATE TABLE completions (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id   INTEGER NOT NULL REFERENCES care_tasks(id) ON DELETE CASCADE,
  user_sub  TEXT NOT NULL,
  user_name TEXT NOT NULL,
  liters    REAL,
  note      TEXT NOT NULL DEFAULT '',
  done_at   TEXT NOT NULL,
  forced    INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE care_assignments (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  task_id       INTEGER NOT NULL REFERENCES care_tasks(id) ON DELETE CASCADE,
  state         TEXT NOT NULL,
  created_at    TEXT NOT NULL,
  next_offer_at TEXT NOT NULL DEFAULT '',
  claimed_by    TEXT NOT NULL DEFAULT '',
  claimed_name  TEXT NOT NULL DEFAULT '',
  claimed_at    TEXT NOT NULL DEFAULT '',
  claim_until   TEXT NOT NULL DEFAULT '',
  ended_at      TEXT NOT NULL DEFAULT '',
  end_reason    TEXT NOT NULL DEFAULT ''
);
CREATE TABLE care_notifications (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  assignment_id INTEGER NOT NULL REFERENCES care_assignments(id) ON DELETE CASCADE,
  task_id       INTEGER NOT NULL,
  place_id      INTEGER NOT NULL,
  user_sub      TEXT NOT NULL,
  kind          TEXT NOT NULL,
  created_at    TEXT NOT NULL,
  expires_at    TEXT NOT NULL DEFAULT '',
  ack_at        TEXT NOT NULL DEFAULT '',
  closed_at     TEXT NOT NULL DEFAULT '',
  closed_reason TEXT NOT NULL DEFAULT ''
);
`

// bestandsDatenbank legt eine Datei im alten Schema an und füllt sie mit dem,
// was im Cluster steht: ein Blumenkasten, ein Gießplan, eine gemeldete
// Erledigung, ein laufender Vorgang mit Zusage und die Anfrage dazu.
func bestandsDatenbank(t *testing.T) string {
	t.Helper()
	pfad := filepath.Join(t.TempDir(), "bestand.sqlite")
	roh, err := sql.Open("sqlite", "file:"+pfad+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	defer roh.Close()
	if _, err := roh.Exec(altesSchema); err != nil {
		t.Fatal(err)
	}
	anweisungen := []string{
		`INSERT INTO places(id,name,description,kind,lat,lon,active,created_at)
		 VALUES(1,'Unter den Eichen — Kasten 1','vor dem Gemeindehaus','blumenkasten',52.2110,9.8700,1,'2026-05-01T08:00:00Z')`,
		`INSERT INTO care_tasks(id,place_id,kind,title,liters,interval_days,red_after_days,active,created_at)
		 VALUES(1,1,'giessen','',10,7,14,1,'2026-05-01T08:00:00Z')`,
		`INSERT INTO completions(id,task_id,user_sub,user_name,liters,note,done_at,forced)
		 VALUES(1,1,'erna','Erna Beispiel',10,'','2026-08-01T06:00:00Z',0)`,
		`INSERT INTO care_assignments(id,task_id,state,created_at,claimed_by,claimed_name,claimed_at,claim_until)
		 VALUES(1,1,'uebernommen','2026-08-10T09:00:00Z','erna','Erna Beispiel','2026-08-10T09:30:00Z','2026-08-11T09:30:00Z')`,
		`INSERT INTO care_notifications(id,assignment_id,task_id,place_id,user_sub,kind,created_at)
		 VALUES(1,1,1,1,'erna','anfrage','2026-08-10T09:00:00Z')`,
	}
	for _, a := range anweisungen {
		if _, err := roh.Exec(a); err != nil {
			t.Fatalf("Bestandsdaten anlegen: %v", err)
		}
	}
	return pfad
}

func TestMigrationBestandsdatenBleiben(t *testing.T) {
	pfad := bestandsDatenbank(t)

	d, err := Open(pfad)
	if err != nil {
		t.Fatalf("Migration der Bestandsdatenbank: %v", err)
	}
	defer d.Close()

	orte, err := d.ListPlaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(orte) != 1 || orte[0].Name != "Unter den Eichen — Kasten 1" {
		t.Fatalf("Ort verloren: %+v", orte)
	}

	task, err := d.GetTask(1)
	if err != nil {
		t.Fatal(err)
	}
	if task.OneOff {
		t.Error("Bestandsaufgabe wurde fälschlich zur einmaligen Aufgabe")
	}
	if task.RemoveWhenDone {
		t.Error("Bestandsaufgabe soll nicht nach dem Erledigen verschwinden")
	}
	if task.Abgeraeumt() {
		t.Error("Bestandsaufgabe wurde fälschlich als abgeräumt markiert")
	}
	if task.IntervalDays != 7 || task.RedAfterDays != 14 {
		t.Errorf("Intervalle verändert: %+v", task)
	}

	cs, err := d.ListCompletions(1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].UserName != "Erna Beispiel" {
		t.Fatalf("Erledigung verloren: %+v", cs)
	}

	a, err := d.ActiveAssignment(1)
	if err != nil || a == nil {
		t.Fatalf("laufender Vorgang verloren: %v %v", a, err)
	}
	if a.ClaimedBy != "erna" {
		t.Errorf("Zusage verloren: %+v", a)
	}
	ns, err := d.NotificationsForAssignment(1)
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 1 || ns[0].UserSub != "erna" {
		t.Fatalf("Zustellung verloren: %+v", ns)
	}
}

// Der Kern der Umstellung: Eine Benachrichtigung darf nicht mit ihrem Vorgang
// verschwinden. Wird eine Aufgabe gelöscht, an der jemand eine Zusage hält,
// räumt SQLite bisher Vorgang UND Hinweis mit weg — die Person erführe nie,
// dass sie nicht mehr losziehen muss (#7).
func TestMigrationHinweisUeberlebtGeloeschteAufgabe(t *testing.T) {
	pfad := bestandsDatenbank(t)
	d, err := Open(pfad)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// So läuft es künftig: erst der Hinweis, dann die Aufgabe weg.
	hinweis := model.Notification{
		AssignmentID: 1, TaskID: 1, PlaceID: 1, UserSub: "erna",
		Kind: model.NotifyAssignmentDropped, CreatedAt: time.Now().UTC(),
		PlaceName: "Unter den Eichen — Kasten 1", TaskName: "Gießen",
	}
	if err := d.InsertNotification(&hinweis); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteTask(1); err != nil {
		t.Fatal(err)
	}

	offen, err := d.OpenNotifications("erna")
	if err != nil {
		t.Fatal(err)
	}
	var gefunden *model.Notification
	for i := range offen {
		if offen[i].ID == hinweis.ID {
			gefunden = &offen[i]
		}
	}
	if gefunden == nil {
		t.Fatalf("Der Hinweis ist mit der Aufgabe verschwunden (offen: %+v)", offen)
	}
	// Ort und Aufgabe gibt es nicht mehr — die Namen müssen aus dem Hinweis
	// selbst kommen, sonst steht dort „ an “.
	if gefunden.PlaceName != "Unter den Eichen — Kasten 1" || gefunden.TaskName != "Gießen" {
		t.Errorf("Namen im Hinweis fehlen: %+v", *gefunden)
	}
}

// Zweimal migrieren muss folgenlos sein — der Server startet oft neu.
func TestMigrationIstWiederholbar(t *testing.T) {
	pfad := bestandsDatenbank(t)
	for i := 0; i < 3; i++ {
		d, err := Open(pfad)
		if err != nil {
			t.Fatalf("Lauf %d: %v", i, err)
		}
		var anzahl int
		if err := d.sql.QueryRow(`SELECT COUNT(*) FROM care_notifications`).Scan(&anzahl); err != nil {
			t.Fatal(err)
		}
		if anzahl != 1 {
			t.Fatalf("Lauf %d: %d Zustellungen statt 1", i, anzahl)
		}
		d.Close()
	}
}

// Eine einmalige Aufgabe muss sich speichern und unverändert wieder auslesen
// lassen — Termin und Schalter inklusive.
func TestEinmaligeAufgabeSpeichern(t *testing.T) {
	d := testDB(t)
	jetzt := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	p := model.Place{Name: "Bahnhof", Kind: model.PlaceOther, Lat: 52.2, Lon: 9.87, Active: true, CreatedAt: jetzt}
	if err := d.InsertPlace(&p); err != nil {
		t.Fatal(err)
	}
	termin := jetzt.Add(5 * 24 * time.Hour)
	t1 := model.CareTask{
		PlaceID: p.ID, Kind: model.TaskOther, Title: "Zum Bahnhof fahren",
		OneOff: true, DueDate: &termin, RemoveWhenDone: true, Active: true, CreatedAt: jetzt,
	}
	if err := d.InsertTask(&t1); err != nil {
		t.Fatal(err)
	}
	gelesen, err := d.GetTask(t1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !gelesen.OneOff || !gelesen.RemoveWhenDone {
		t.Fatalf("Schalter nicht gespeichert: %+v", gelesen)
	}
	if gelesen.DueDate == nil || !gelesen.DueDate.Equal(termin) {
		t.Fatalf("Termin nicht gespeichert: %v", gelesen.DueDate)
	}

	// Abräumen: die Aufgabe verschwindet aus der Liste, bleibt aber lesbar,
	// damit die Rangliste ihre Erledigungen weiter zuordnen kann.
	if err := d.RemoveTask(t1.ID, jetzt); err != nil {
		t.Fatal(err)
	}
	liste, err := d.ListTasks()
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range liste {
		if x.ID == t1.ID {
			t.Fatal("abgeräumte Aufgabe steht noch in ListTasks")
		}
	}
	wieder, err := d.GetTask(t1.ID)
	if err != nil {
		t.Fatalf("abgeräumte Aufgabe ist nicht mehr lesbar: %v", err)
	}
	if !wieder.Abgeraeumt() {
		t.Fatal("RemovedAt fehlt")
	}
}
