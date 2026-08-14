package model

import (
	"testing"
	"time"
)

// berlinZeit baut einen Zeitpunkt in der Ortszeit des Dorfes.
func berlinZeit(t *testing.T, jahr int, monat time.Month, tag, stunde, minute int) time.Time {
	t.Helper()
	return time.Date(jahr, monat, tag, stunde, minute, 0, 0, Location())
}

func TestVergaberegelnVorgabeUndPruefung(t *testing.T) {
	r := DefaultAssignmentRules()
	if r.OfferInterval != time.Hour {
		t.Errorf("Staffelabstand = %v, erwartet 1h", r.OfferInterval)
	}
	if r.ClaimDuration != 24*time.Hour {
		t.Errorf("Zusagefrist = %v, erwartet 24h", r.ClaimDuration)
	}
	if r.QuietFrom != 21 || r.QuietTo != 7 {
		t.Errorf("Ruhezeit = %d–%d, erwartet 21–7", r.QuietFrom, r.QuietTo)
	}
	if err := r.Validate(); err != nil {
		t.Errorf("Vorgabe ist ungültig: %v", err)
	}
	for name, kaputt := range map[string]AssignmentRules{
		"Abstand 0":       {OfferInterval: 0, ClaimDuration: time.Hour, QuietFrom: 21, QuietTo: 7},
		"Frist 0":         {OfferInterval: time.Hour, ClaimDuration: 0, QuietFrom: 21, QuietTo: 7},
		"Stunde zu groß":  {OfferInterval: time.Hour, ClaimDuration: time.Hour, QuietFrom: 24, QuietTo: 7},
		"Stunde negativ":  {OfferInterval: time.Hour, ClaimDuration: time.Hour, QuietFrom: 21, QuietTo: -1},
		"Abstand zu lang": {OfferInterval: 48 * time.Hour, ClaimDuration: time.Hour, QuietFrom: 21, QuietTo: 7},
	} {
		if err := kaputt.Validate(); err == nil {
			t.Errorf("%s wurde akzeptiert", name)
		}
	}
}

func TestRuhezeitVerschiebtZustellung(t *testing.T) {
	r := DefaultAssignmentRules()
	faelle := []struct {
		name     string
		zeit     time.Time
		erwartet time.Time
	}{
		{"mittags unverändert", berlinZeit(t, 2026, time.June, 10, 12, 0), berlinZeit(t, 2026, time.June, 10, 12, 0)},
		{"kurz vor Ruhe unverändert", berlinZeit(t, 2026, time.June, 10, 20, 59), berlinZeit(t, 2026, time.June, 10, 20, 59)},
		{"Ruhebeginn wartet bis morgens", berlinZeit(t, 2026, time.June, 10, 21, 0), berlinZeit(t, 2026, time.June, 11, 7, 0)},
		{"spät abends wartet bis morgens", berlinZeit(t, 2026, time.June, 10, 23, 30), berlinZeit(t, 2026, time.June, 11, 7, 0)},
		{"nachts wartet auf denselben Morgen", berlinZeit(t, 2026, time.June, 11, 2, 15), berlinZeit(t, 2026, time.June, 11, 7, 0)},
		{"Ruheende ist frei", berlinZeit(t, 2026, time.June, 11, 7, 0), berlinZeit(t, 2026, time.June, 11, 7, 0)},
	}
	for _, f := range faelle {
		if got := r.NextDelivery(f.zeit); !got.Equal(f.erwartet) {
			t.Errorf("%s: NextDelivery(%s) = %s, erwartet %s", f.name,
				f.zeit.In(Location()).Format(time.RFC3339),
				got.In(Location()).Format(time.RFC3339),
				f.erwartet.In(Location()).Format(time.RFC3339))
		}
	}
}

// Die Ruhezeit hängt an der Ortszeit, nicht an UTC: In der Nacht der
// Sommerzeit-Umstellung (29.03.2026, 02:00 → 03:00) ist 07:00 Ortszeit
// = 05:00 UTC, im Winter dagegen 06:00 UTC.
func TestRuhezeitFolgtDerSommerzeit(t *testing.T) {
	r := DefaultAssignmentRules()

	nachtsVorUmstellung := time.Date(2026, time.March, 29, 0, 30, 0, 0, time.UTC) // 01:30 MEZ
	got := r.NextDelivery(nachtsVorUmstellung).UTC()
	if want := time.Date(2026, time.March, 29, 5, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Umstellungsnacht: %s, erwartet %s (= 07:00 MESZ)", got, want)
	}

	nachtsImWinter := time.Date(2026, time.January, 15, 1, 30, 0, 0, time.UTC) // 02:30 MEZ
	got = r.NextDelivery(nachtsImWinter).UTC()
	if want := time.Date(2026, time.January, 15, 6, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Winternacht: %s, erwartet %s (= 07:00 MEZ)", got, want)
	}
}

func TestRuhezeitIstKonfigurierbar(t *testing.T) {
	r := AssignmentRules{OfferInterval: time.Hour, ClaimDuration: 24 * time.Hour, QuietFrom: 22, QuietTo: 6}
	if got := r.NextDelivery(berlinZeit(t, 2026, time.June, 10, 21, 30)); !got.Equal(berlinZeit(t, 2026, time.June, 10, 21, 30)) {
		t.Errorf("21:30 muss bei Ruhe ab 22 Uhr durchgehen, war %s", got.In(Location()))
	}
	if got, want := r.NextDelivery(berlinZeit(t, 2026, time.June, 10, 22, 30)), berlinZeit(t, 2026, time.June, 11, 6, 0); !got.Equal(want) {
		t.Errorf("22:30 = %s, erwartet %s", got.In(Location()), want.In(Location()))
	}

	// Ohne Ruhezeit (von == bis) wird rund um die Uhr zugestellt.
	ohne := AssignmentRules{OfferInterval: time.Hour, ClaimDuration: 24 * time.Hour, QuietFrom: 0, QuietTo: 0}
	nachts := berlinZeit(t, 2026, time.June, 10, 3, 0)
	if got := ohne.NextDelivery(nachts); !got.Equal(nachts) {
		t.Errorf("ohne Ruhezeit verschoben auf %s", got.In(Location()))
	}
}

// Die Reihenfolge ist die Kernregel der Vergabe: Wer am längsten nichts
// erledigt hat bzw. am längsten nicht gefragt wurde, kommt zuerst.
func TestReihenfolgeIstFair(t *testing.T) {
	t0 := berlinZeit(t, 2026, time.June, 1, 12, 0)
	tag := func(n int) time.Time { return t0.AddDate(0, 0, n) }

	kandidaten := []Candidate{
		// Gestern gegossen — kommt zuletzt.
		{UserSub: "vielgiesser", SignedUpAt: tag(0), LastDone: tag(9), LastAsked: tag(2)},
		// Vor einer Woche gefragt, nie erledigt.
		{UserSub: "gefragt", SignedUpAt: tag(0), LastDone: time.Time{}, LastAsked: tag(3)},
		// Noch nie gefragt, noch nie erledigt — ganz nach vorn.
		{UserSub: "neuling", SignedUpAt: tag(5), LastDone: time.Time{}, LastAsked: time.Time{}},
		// Lange her, aber es gab schon mal etwas.
		{UserSub: "langeher", SignedUpAt: tag(0), LastDone: tag(1), LastAsked: tag(1)},
	}
	got := OrderCandidates(kandidaten)
	want := []string{"neuling", "langeher", "gefragt", "vielgiesser"}
	for i := range want {
		if got[i].UserSub != want[i] {
			t.Fatalf("Reihenfolge = %v, erwartet %v", subs(got), want)
		}
	}

	// Die Eingabe darf nicht verändert werden.
	if kandidaten[0].UserSub != "vielgiesser" {
		t.Error("OrderCandidates hat die übergebene Liste umsortiert")
	}
}

// Der maßgebliche Zeitpunkt ist der jüngere von „zuletzt erledigt" und
// „zuletzt gefragt": Wer gerade erst gefragt wurde, rutscht nach hinten,
// auch wenn er seit Monaten nichts erledigt hat.
func TestReihenfolgeZaehltAuchDasGefragtwerden(t *testing.T) {
	t0 := berlinZeit(t, 2026, time.June, 1, 12, 0)
	got := OrderCandidates([]Candidate{
		{UserSub: "eben-gefragt", SignedUpAt: t0, LastDone: t0.AddDate(0, -6, 0), LastAsked: t0},
		{UserSub: "vor-monat-erledigt", SignedUpAt: t0, LastDone: t0.AddDate(0, -1, 0)},
	})
	if got[0].UserSub != "vor-monat-erledigt" {
		t.Fatalf("Reihenfolge = %v, erwartet vor-monat-erledigt zuerst", subs(got))
	}
}

// Gleichstand: Wer sich früher angemeldet hat, kommt zuerst; danach
// entscheidet die Kennung, damit die Reihenfolge reproduzierbar bleibt.
func TestReihenfolgeBeiGleichstand(t *testing.T) {
	t0 := berlinZeit(t, 2026, time.June, 1, 12, 0)
	got := OrderCandidates([]Candidate{
		{UserSub: "c", SignedUpAt: t0.Add(time.Hour)},
		{UserSub: "a", SignedUpAt: t0.Add(2 * time.Hour)},
		{UserSub: "b", SignedUpAt: t0.Add(time.Hour)},
	})
	want := []string{"b", "c", "a"}
	for i := range want {
		if got[i].UserSub != want[i] {
			t.Fatalf("Reihenfolge = %v, erwartet %v", subs(got), want)
		}
	}
}

func subs(cs []Candidate) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.UserSub)
	}
	return out
}

func TestAnmeldungPasstZurAufgabe(t *testing.T) {
	giessen := CareTask{ID: 1, PlaceID: 7, Kind: TaskWatering}
	jaeten := CareTask{ID: 2, PlaceID: 7, Kind: TaskWeeding}
	fremd := CareTask{ID: 3, PlaceID: 8, Kind: TaskWatering}

	ganzerOrt := Signup{PlaceID: 7}
	nurGiessen := Signup{PlaceID: 7, TaskKind: TaskWatering}

	if !ganzerOrt.Matches(giessen) || !ganzerOrt.Matches(jaeten) {
		t.Error("Anmeldung für den Ort muss für alle seine Aufgaben gelten")
	}
	if ganzerOrt.Matches(fremd) {
		t.Error("Anmeldung darf nicht auf einen anderen Ort wirken")
	}
	if !nurGiessen.Matches(giessen) || nurGiessen.Matches(jaeten) {
		t.Error("Auf Gießen eingeschränkte Anmeldung darf nicht fürs Jäten gelten")
	}
}

func TestNotificationTexteSindDeutsch(t *testing.T) {
	for _, k := range []NotificationKind{
		NotifyRequest, NotifyBroadcast, NotifyClaimExpired, NotifyClaimRevoked, NotifyAssignmentDone,
	} {
		if !ValidNotificationKind(k) {
			t.Errorf("%q gilt als ungültig", k)
		}
	}
	if ValidNotificationKind("quatsch") {
		t.Error("unbekannte Art wurde akzeptiert")
	}
}
