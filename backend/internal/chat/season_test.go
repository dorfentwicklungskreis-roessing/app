package chat

import (
	"testing"
)

// Auch im Chat lässt sich die Jahreszeit einer Aufgabe setzen (#78) — und
// eine Änderung an etwas anderem räumt sie nicht nebenbei ab.

func TestChatSetztUndBehaeltDieJahreszeit(t *testing.T) {
	dd := neuesDorf(t)
	s := dd.sitzung(t, tokenVorstand)

	if _, err := rufeWerkzeug(t, s, "aufgabe_aendern", map[string]any{
		"id": dd.Giessen.ID, "saisonVon": 4.0, "saisonBis": 9.0,
	}); err != nil {
		t.Fatal(err)
	}
	aufgabe, err := dd.DB.GetTask(dd.Giessen.ID)
	if err != nil {
		t.Fatal(err)
	}
	if aufgabe.SeasonStartMonth != 4 || aufgabe.SeasonEndMonth != 9 {
		t.Fatalf("Jahreszeit = %d/%d, erwartet 4/9",
			aufgabe.SeasonStartMonth, aufgabe.SeasonEndMonth)
	}

	if _, err := rufeWerkzeug(t, s, "aufgabe_aendern", map[string]any{
		"id": dd.Giessen.ID, "intervallTage": 10.0, "rotNachTagen": 20.0,
	}); err != nil {
		t.Fatal(err)
	}
	aufgabe, err = dd.DB.GetTask(dd.Giessen.ID)
	if err != nil {
		t.Fatal(err)
	}
	if aufgabe.SeasonStartMonth != 4 || aufgabe.SeasonEndMonth != 9 {
		t.Fatalf("die Intervalländerung hat die Jahreszeit angefasst: %d/%d",
			aufgabe.SeasonStartMonth, aufgabe.SeasonEndMonth)
	}
}
