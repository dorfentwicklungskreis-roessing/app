package vergabe

import (
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Die Vergabe ist der Weg, auf dem Aufgaben von sich aus zu den Leuten
// kommen — als Benachrichtigung und als Push. Sie muss dieselben Grenzen
// einhalten wie jede Liste: Eine interne Aufgabe darf niemanden außerhalb
// des Trägers erreichen, und eine Aufgabe mit Einweisung niemanden ohne sie.

// traegerAufbau baut die Umgebung: ein Träger mit Zitadel-Projekt „222“ und
// ein Ort, der ihm gehört.
func traegerAufbau(t *testing.T, start time.Time, sichtbarkeit model.TaskSichtbarkeit,
	befaehigungName string,
) (*db.DB, *Engine, *sammler, model.CareTask, int64) {
	t.Helper()
	d, e, _, s, _ := aufbau(t, start)

	tr := model.Traeger{Name: "Dorfpflege", ProjektID: "222", Status: model.TraegerZugelassen,
		Sichtbarkeit: model.TraegerOffen, CreatedAt: start}
	if err := d.InsertTraeger(&tr); err != nil {
		t.Fatal(err)
	}
	p := model.Place{Name: "Streuobstwiese", TraegerID: tr.ID, Kind: model.PlaceOther,
		Lat: 52.2, Lon: 9.87, Active: true, CreatedAt: start.AddDate(0, 0, -30)}
	if err := d.InsertPlace(&p); err != nil {
		t.Fatal(err)
	}
	var befaehigungID int64
	if befaehigungName != "" {
		b := model.Befaehigung{TraegerID: tr.ID, Name: befaehigungName, CreatedAt: start}
		if err := d.InsertBefaehigung(&b); err != nil {
			t.Fatal(err)
		}
		befaehigungID = b.ID
	}
	task := model.CareTask{PlaceID: p.ID, Kind: model.TaskOther, Title: "Rasenmähen",
		IntervalDays: 7, RedAfterDays: 14, Sichtbarkeit: sichtbarkeit,
		BefaehigungID: befaehigungID, Active: true, CreatedAt: start.AddDate(0, 0, -30)}
	if err := d.InsertTask(&task); err != nil {
		t.Fatal(err)
	}
	return d, e, s, task, befaehigungID
}

// Eine interne Aufgabe fragt nur Mitglieder — auch dann, wenn sich jemand
// von außen für den Ort angemeldet hat (etwa noch aus der Zeit davor).
func TestVergabeFragtBeiInternerAufgabeNurMitglieder(t *testing.T) {
	start := berlin(t, 2026, time.August, 16, 10, 0)
	d, e, s, task, _ := traegerAufbau(t, start, model.AufgabeNurMitglieder, "")

	anmelden(t, d, task, "mitglied", start.Add(-time.Hour))
	anmelden(t, d, task, "fremd", start.Add(-2*time.Hour))

	// Wer wo Mitglied ist, weiß die Vergabe von der eingehängten Quelle.
	e.Mitgliedschaften = func(sub string) (model.Mitgliedschaften, error) {
		if sub == "mitglied" {
			return model.Mitgliedschaften{"222": {model.RolleMitglied: true}}, nil
		}
		return model.Mitgliedschaften{}, nil
	}

	durchlauf(t, e)
	durchlauf(t, e)

	for _, empfaenger := range s.empfaenger(model.NotifyRequest) {
		if empfaenger == "fremd" {
			t.Fatal("eine interne Aufgabe wurde jemandem außerhalb des Trägers angeboten")
		}
	}
	// Und das Mitglied kommt sehr wohl dran.
	gefragt := false
	for _, empfaenger := range s.empfaenger(model.NotifyRequest) {
		if empfaenger == "mitglied" {
			gefragt = true
		}
	}
	if !gefragt {
		t.Fatalf("das Mitglied wurde nicht gefragt: %+v", s.empfaenger(model.NotifyRequest))
	}
}

// Eine Aufgabe mit Einweisung fragt nur, wer sie hat: Eine Anfrage an jemanden,
// der ohnehin nicht zusagen darf, wäre eine sinnlose Störung.
func TestVergabeFragtNurMitBefaehigung(t *testing.T) {
	start := berlin(t, 2026, time.August, 16, 10, 0)
	d, e, s, task, befaehigungID := traegerAufbau(t, start, model.AufgabeOeffentlich, "Motorsense")

	anmelden(t, d, task, "eingewiesen", start.Add(-time.Hour))
	anmelden(t, d, task, "ohne", start.Add(-2*time.Hour))

	a := model.BefaehigungsAntrag{BefaehigungID: befaehigungID, UserSub: "eingewiesen",
		Status: model.AntragErteilt, CreatedAt: start}
	if err := d.InsertAntrag(&a); err != nil {
		t.Fatal(err)
	}

	durchlauf(t, e)
	durchlauf(t, e)

	for _, empfaenger := range s.empfaenger(model.NotifyRequest) {
		if empfaenger == "ohne" {
			t.Fatal("jemand ohne die verlangte Einweisung wurde gefragt")
		}
	}
}

// Ohne eingehängte Mitgliedschafts-Quelle (Produktion ohne Zitadel-Zugang)
// wird eine interne Aufgabe an NIEMANDEN ausgespielt. Im Zweifel weniger.
func TestOhneQuelleKeineInterneAnfrage(t *testing.T) {
	start := berlin(t, 2026, time.August, 16, 10, 0)
	d, e, s, task, _ := traegerAufbau(t, start, model.AufgabeNurMitglieder, "")
	anmelden(t, d, task, "irgendwer", start.Add(-time.Hour))

	durchlauf(t, e)
	durchlauf(t, e)

	if empf := s.empfaenger(model.NotifyRequest); len(empf) > 0 {
		t.Fatalf("interne Aufgabe ohne gesicherte Mitgliedschaft angeboten: %v", empf)
	}
}
