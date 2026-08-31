package db

import (
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Beitritte in der Datenbank: der Vorgang, nicht die Mitgliedschaft. Wer
// dazugehört, sagt die Rössing-ID — hier steht, wer gefragt und wer
// entschieden hat.

func beitrittStellen(t *testing.T, d *DB, traegerID int64, sub, grund string) model.Beitritt {
	t.Helper()
	b := model.Beitritt{TraegerID: traegerID, UserSub: sub, Status: model.AntragBeantragt,
		Begruendung: grund, CreatedAt: time.Now().UTC()}
	if err := d.InsertBeitritt(&b); err != nil {
		t.Fatalf("Beitritt stellen: %v", err)
	}
	return b
}

func TestBeitrittStellenUndEntscheiden(t *testing.T) {
	d := testDB(t)
	tr := testTraeger(t, d, "AK 2 Umwelt und Natur", "388")
	b := beitrittStellen(t, d, tr.ID, "erna", "Ich wohne neben dem Beet")

	offen, err := d.ListBeitritte(tr.ID, model.AntragBeantragt)
	if err != nil {
		t.Fatal(err)
	}
	if len(offen) != 1 || offen[0].Begruendung != "Ich wohne neben dem Beet" {
		t.Fatalf("offener Antrag fehlt: %+v", offen)
	}

	entschieden := time.Date(2026, 8, 31, 9, 0, 0, 0, time.UTC)
	if err := d.EntscheideBeitritt(b.ID, model.AntragErteilt, "vorstand", "willkommen", entschieden); err != nil {
		t.Fatal(err)
	}
	nachher, err := d.GetBeitritt(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nachher.Status != model.AntragErteilt || nachher.EntschiedenVon != "vorstand" ||
		nachher.Notiz != "willkommen" || nachher.EntschiedenAm == nil {
		t.Fatalf("Entscheidung nicht festgehalten: %+v", nachher)
	}
	if rest, _ := d.ListBeitritte(tr.ID, model.AntragBeantragt); len(rest) != 0 {
		t.Errorf("der entschiedene Antrag steht noch als offen: %+v", rest)
	}
}

// Je Person und Träger genau eine Zeile: Ein zweiter Antrag belebt den
// bestehenden wieder, statt Karteileichen zu stapeln.
func TestZweiterAntragBelebtDenErsten(t *testing.T) {
	d := testDB(t)
	tr := testTraeger(t, d, "Dorfpflege", "377")
	erst := beitrittStellen(t, d, tr.ID, "erna", "erster Versuch")
	if err := d.EntscheideBeitritt(erst.ID, model.AntragAbgelehnt, "vorstand", "später", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	zweit := beitrittStellen(t, d, tr.ID, "erna", "jetzt aber")
	if zweit.ID != erst.ID {
		t.Fatalf("es entstand eine zweite Zeile (%d statt %d)", zweit.ID, erst.ID)
	}
	nachher, err := d.GetBeitritt(erst.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nachher.Status != model.AntragBeantragt || nachher.Begruendung != "jetzt aber" {
		t.Fatalf("der Antrag wurde nicht wiederbelebt: %+v", nachher)
	}
	if nachher.Notiz != "" || nachher.EntschiedenVon != "" || nachher.EntschiedenAm != nil {
		t.Errorf("die alte Entscheidung klebt am neuen Antrag: %+v", nachher)
	}
}

// Auch ein erteilter Antrag lässt sich neu stellen — anders als bei der
// Befähigung. Wer aus dem Verein ausgetreten ist, steht in der Rössing-ID
// nicht mehr, und der alte Vermerk hier macht ihn nicht wieder zum Mitglied.
func TestAuchNachAufnahmeLaesstSichNeuFragen(t *testing.T) {
	d := testDB(t)
	tr := testTraeger(t, d, "Dorfpflege", "377")
	b := beitrittStellen(t, d, tr.ID, "erna", "bitte")
	if err := d.EntscheideBeitritt(b.ID, model.AntragErteilt, "vorstand", "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	beitrittStellen(t, d, tr.ID, "erna", "wieder da")
	nachher, err := d.GetBeitritt(b.ID)
	if err != nil {
		t.Fatal(err)
	}
	if nachher.Status != model.AntragBeantragt {
		t.Fatalf("der erteilte Antrag ließ sich nicht neu stellen: %+v", nachher)
	}
}

func TestBeitritteJePersonUndZaehler(t *testing.T) {
	d := testDB(t)
	pflege := testTraeger(t, d, "Dorfpflege", "377")
	ak := testTraeger(t, d, "AK 2 Umwelt und Natur", "388")
	beitrittStellen(t, d, pflege.ID, "erna", "")
	beitrittStellen(t, d, ak.ID, "erna", "")
	beitrittStellen(t, d, ak.ID, "hans", "")

	meine, err := d.ListBeitritteVonPerson("erna")
	if err != nil {
		t.Fatal(err)
	}
	if len(meine) != 2 {
		t.Fatalf("%d eigene Anträge statt 2: %+v", len(meine), meine)
	}
	// Der Trägername gehört dazu — sonst müsste die App Kennungen auflösen.
	for _, b := range meine {
		if b.TraegerName == "" {
			t.Errorf("Antrag ohne Trägernamen: %+v", b)
		}
	}

	zaehler, err := d.OffeneBeitritte()
	if err != nil {
		t.Fatal(err)
	}
	if zaehler[pflege.ID] != 1 || zaehler[ak.ID] != 2 {
		t.Fatalf("falsch gezählt: %+v", zaehler)
	}

	// Ein eigener Antrag ist über Träger und Person auffindbar.
	eigener, err := d.BeitrittVon(ak.ID, "hans")
	if err != nil || eigener == nil {
		t.Fatalf("eigener Antrag nicht gefunden: %v %+v", err, eigener)
	}
	keiner, err := d.BeitrittVon(pflege.ID, "hans")
	if err != nil || keiner != nil {
		t.Fatalf("aus dem Nichts entstandener Antrag: %v %+v", err, keiner)
	}
}
