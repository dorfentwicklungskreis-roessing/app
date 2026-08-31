package model

import (
	"testing"
	"time"
)

// Das Beet vor dem Dorfgemeinschaftshaus: alle acht Wochen jäten, aber nur
// von April bis September (#78).
func beet() CareTask {
	return CareTask{
		Kind: TaskWeeding, IntervalDays: 56, RedAfterDays: 70,
		SeasonStartMonth: 4, SeasonEndMonth: 9,
		CreatedAt: dorfzeit(2026, time.April, 1),
	}
}

// dorfzeit ist Mitternacht Ortszeit des Dorfes — in dieser Zeit werden die
// Monatsgrenzen gezogen.
func dorfzeit(jahr int, monat time.Month, tag int) time.Time {
	return time.Date(jahr, monat, tag, 0, 0, 0, 0, Location())
}

func TestSeasonFensterEinschliesslich(t *testing.T) {
	s := Season{Start: time.April, End: time.September}
	cases := []struct {
		name string
		zeit time.Time
		drin bool
	}{
		{"31. März kurz vor Mitternacht", dorfzeit(2026, time.April, 1).Add(-time.Second), false},
		{"1. April 00:00", dorfzeit(2026, time.April, 1), true},
		{"Mitte Juli", dorfzeit(2026, time.July, 15), true},
		{"30. September 23:59", dorfzeit(2026, time.October, 1).Add(-time.Minute), true},
		{"1. Oktober 00:00", dorfzeit(2026, time.October, 1), false},
		{"Dezember", dorfzeit(2026, time.December, 24), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := s.Contains(c.zeit, Location()); got != c.drin {
				t.Fatalf("Contains = %v, erwartet %v", got, c.drin)
			}
		})
	}
}

// Ein Zeitraum über den Jahreswechsel: November bis Februar. Der Januar
// gehört dazu, der Juni nicht.
func TestSeasonUeberDenJahreswechsel(t *testing.T) {
	s := Season{Start: time.November, End: time.February}
	cases := []struct {
		zeit time.Time
		drin bool
	}{
		{dorfzeit(2026, time.October, 31), false},
		{dorfzeit(2026, time.November, 1), true},
		{dorfzeit(2026, time.December, 31), true},
		{dorfzeit(2027, time.January, 15), true},
		{dorfzeit(2027, time.February, 28), true},
		{dorfzeit(2027, time.March, 1), false},
		{dorfzeit(2027, time.June, 1), false},
	}
	for _, c := range cases {
		if got := s.Contains(c.zeit, Location()); got != c.drin {
			t.Errorf("%s: Contains = %v, erwartet %v", c.zeit.Format("2006-01-02"), got, c.drin)
		}
	}
}

// Außerhalb des Fensters zeigt Window auf den Beginn des nächsten — das ist
// der Tag, ab dem wieder etwas zu tun ist.
func TestSeasonNaechstesFenster(t *testing.T) {
	s := Season{Start: time.April, End: time.September}
	von, _, drin := s.Window(dorfzeit(2026, time.December, 24), Location())
	if drin {
		t.Fatal("der Heiligabend liegt nicht in der Jätesaison")
	}
	if want := dorfzeit(2027, time.April, 1); !von.Equal(want) {
		t.Fatalf("nächstes Fenster ab %v, erwartet %v", von, want)
	}

	von, _, drin = s.Window(dorfzeit(2026, time.February, 3), Location())
	if drin {
		t.Fatal("der Februar liegt nicht in der Jätesaison")
	}
	if want := dorfzeit(2026, time.April, 1); !von.Equal(want) {
		t.Fatalf("nächstes Fenster ab %v, erwartet %v", von, want)
	}
}

// Der Dezember als letzter Monat darf nicht am 1. Dezember enden — Monat 13
// ist der Januar des Folgejahres.
func TestSeasonBisDezember(t *testing.T) {
	s := Season{Start: time.October, End: time.December}
	if !s.Contains(dorfzeit(2026, time.December, 31), Location()) {
		t.Error("der 31. Dezember gehört noch zu Oktober–Dezember")
	}
	if s.Contains(dorfzeit(2027, time.January, 1), Location()) {
		t.Error("der 1. Januar gehört nicht mehr dazu")
	}
}

// Der Kern von #78: Im November ist an dem Beet nichts zu jäten. Die Aufgabe
// ist dann nicht rot, aber auch nicht grün — sie ist außer Dienst.
func TestAufgabeAusserhalbIhrerJahreszeitRuht(t *testing.T) {
	letzte := done(dorfzeit(2026, time.September, 20))
	status, _, _ := ComputeStatus(beet(), letzte, dorfzeit(2026, time.November, 16), 1)
	if status != StatusDormant {
		t.Fatalf("Status im November = %s, erwartet %s", status, StatusDormant)
	}
	if status.NeedsWork() {
		t.Fatal("an einer ruhenden Aufgabe ist nichts zu tun")
	}
}

// Am 30. September rot, am 1. Oktober außer Dienst — und nicht plötzlich
// grün, als hätte jemand gejätet.
func TestUebergangAmSaisonendeIstNichtGruen(t *testing.T) {
	letzte := done(dorfzeit(2026, time.June, 1))
	vorher, _, _ := ComputeStatus(beet(), letzte, dorfzeit(2026, time.October, 1).Add(-time.Minute), 1)
	if vorher != StatusRed {
		t.Fatalf("am 30. September = %s, erwartet %s", vorher, StatusRed)
	}
	nachher, _, _ := ComputeStatus(beet(), letzte, dorfzeit(2026, time.October, 1), 1)
	if nachher != StatusDormant {
		t.Fatalf("am 1. Oktober = %s, erwartet %s", nachher, StatusDormant)
	}
}

// Beim Start im April ist sie fällig, ohne dass jemand etwas angefasst hat:
// Der Zähler läuft über den Winter weiter, nur die Ampel schweigt.
func TestSaisonbeginnIstSofortFaellig(t *testing.T) {
	letzte := done(dorfzeit(2026, time.September, 20))
	status, dueAt, redAt := ComputeStatus(beet(), letzte, dorfzeit(2027, time.April, 1), 1)
	if !status.NeedsWork() {
		t.Fatalf("am 1. April = %s, erwartet fällig", status)
	}
	// Fällig werden kann sie frühestens mit dem Fenster — nicht im Februar.
	if want := dorfzeit(2027, time.April, 1); !dueAt.Equal(want) {
		t.Errorf("dueAt = %v, erwartet %v", dueAt, want)
	}
	if want := dorfzeit(2027, time.April, 1); !redAt.Equal(want) {
		t.Errorf("redAt = %v, erwartet %v", redAt, want)
	}
}

// Innerhalb der Jahreszeit rechnet die Ampel wie eh und je.
func TestInnerhalbDerJahreszeitGiltDasIntervall(t *testing.T) {
	letzte := done(dorfzeit(2026, time.May, 1))
	cases := []struct {
		name  string
		jetzt time.Time
		want  Status
	}{
		{"eine Woche später", dorfzeit(2026, time.May, 8), StatusGreen},
		{"nach 56 Tagen gelb", dorfzeit(2026, time.May, 1).Add(56 * 24 * time.Hour), StatusYellow},
		{"nach 70 Tagen rot", dorfzeit(2026, time.May, 1).Add(70 * 24 * time.Hour), StatusRed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _, _ := ComputeStatus(beet(), letzte, c.jetzt, 1)
			if got != c.want {
				t.Fatalf("Status = %s, erwartet %s", got, c.want)
			}
		})
	}
}

// Der Hitzefaktor staucht Schwellen, er verlängert keine Jahreszeit: Im
// Dezember bleibt auch bei Faktor 0.1 alles ruhig.
func TestHitzefaktorVerlaengertDieJahreszeitNicht(t *testing.T) {
	giessen := CareTask{
		Kind: TaskWatering, IntervalDays: 3, RedAfterDays: 6,
		SeasonStartMonth: 5, SeasonEndMonth: 9,
		CreatedAt: dorfzeit(2026, time.May, 1),
	}
	status, _, _ := ComputeStatus(giessen, done(dorfzeit(2026, time.September, 28)),
		dorfzeit(2026, time.December, 20), 0.1)
	if status != StatusDormant {
		t.Fatalf("Status = %s, erwartet %s", status, StatusDormant)
	}
}

// Ohne Angabe fällt eine Aufgabe ganzjährig an — so laufen alle
// Bestandsaufgaben unverändert weiter.
func TestOhneJahreszeitBleibtAllesWieBisher(t *testing.T) {
	ganzjaehrig := beet()
	ganzjaehrig.SeasonStartMonth, ganzjaehrig.SeasonEndMonth = 0, 0
	status, _, _ := ComputeStatus(ganzjaehrig, done(dorfzeit(2026, time.September, 20)),
		dorfzeit(2026, time.December, 16), 1)
	if status != StatusRed {
		t.Fatalf("Status = %s, erwartet %s — ohne Jahreszeit zählt nur das Intervall", status, StatusRed)
	}
}

// Eine einmalige Aufgabe hat einen Termin und keine Jahreszeit; ein
// versehentlich gesetzter Zeitraum darf ihren Termin nicht verschlucken.
func TestEinmaligeAufgabeIgnoriertDieJahreszeit(t *testing.T) {
	faellig := dorfzeit(2026, time.December, 20)
	einmalig := CareTask{
		Kind: TaskOther, OneOff: true, DueDate: &faellig,
		SeasonStartMonth: 4, SeasonEndMonth: 9,
		CreatedAt: dorfzeit(2026, time.December, 1),
	}
	status, _, _ := ComputeStatus(einmalig, nil, dorfzeit(2026, time.December, 21), 1)
	if status != StatusRed {
		t.Fatalf("Status = %s, erwartet %s", status, StatusRed)
	}
}

func TestValidSeasonMonths(t *testing.T) {
	if err := ValidSeasonMonths(0, 0); err != nil {
		t.Errorf("0/0 (ganzjährig) muss erlaubt sein: %v", err)
	}
	if err := ValidSeasonMonths(4, 9); err != nil {
		t.Errorf("April bis September muss erlaubt sein: %v", err)
	}
	if err := ValidSeasonMonths(11, 2); err != nil {
		t.Errorf("November bis Februar muss erlaubt sein: %v", err)
	}
	if err := ValidSeasonMonths(4, 0); err == nil {
		t.Error("halbe Angabe muss abgewiesen werden")
	}
	if err := ValidSeasonMonths(0, 9); err == nil {
		t.Error("halbe Angabe muss abgewiesen werden")
	}
	if err := ValidSeasonMonths(13, 2); err == nil {
		t.Error("Monat 13 gibt es nicht")
	}
	if von, bis := NormalizeSeasonMonths(1, 12); von != 0 || bis != 0 {
		t.Errorf("Januar–Dezember = %d/%d, erwartet ganzjährig (0/0)", von, bis)
	}
}

// Ein Ort, an dem alle Aufgaben ruhen, ruht mit. Steht daneben etwas
// Ganzjähriges, entscheidet das.
func TestPlaceStatus(t *testing.T) {
	ruht := TaskWithStatus{CareTask: CareTask{Active: true}, Status: StatusDormant}
	gruen := TaskWithStatus{CareTask: CareTask{Active: true}, Status: StatusGreen}
	rot := TaskWithStatus{CareTask: CareTask{Active: true}, Status: StatusRed}
	pausiert := TaskWithStatus{CareTask: CareTask{Active: false}, Status: StatusRed}

	if got := PlaceStatus([]TaskWithStatus{ruht, ruht}); got != StatusDormant {
		t.Errorf("nur ruhende Aufgaben = %s, erwartet %s", got, StatusDormant)
	}
	if got := PlaceStatus([]TaskWithStatus{ruht, gruen}); got != StatusGreen {
		t.Errorf("ruhend + grün = %s, erwartet %s", got, StatusGreen)
	}
	if got := PlaceStatus([]TaskWithStatus{ruht, rot}); got != StatusRed {
		t.Errorf("ruhend + rot = %s, erwartet %s", got, StatusRed)
	}
	if got := PlaceStatus([]TaskWithStatus{pausiert}); got != StatusGreen {
		t.Errorf("nur pausierte Aufgaben = %s, erwartet %s", got, StatusGreen)
	}
	if got := PlaceStatus(nil); got != StatusGreen {
		t.Errorf("ohne Aufgaben = %s, erwartet %s", got, StatusGreen)
	}
}
