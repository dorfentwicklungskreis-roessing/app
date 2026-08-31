package model

import "testing"

// Wer darf wo um Aufnahme bitten? Die Frage wird an genau einer Stelle
// beantwortet (Zugriff), damit REST, Web-Verwaltung und MCP nicht drei
// verschiedene Antworten geben.

func offenerTraeger() Traeger {
	return Traeger{ID: 3, ProjektID: "388", Name: "AK 2 Umwelt und Natur",
		Status: TraegerZugelassen, Sichtbarkeit: TraegerOffen}
}

func TestBeitrittZuEinemOffenenTraeger(t *testing.T) {
	z := Zugriff{Sub: "erna", Mitglied: Mitgliedschaften{}}
	if !z.DarfBeitrittBeantragen(offenerTraeger()) {
		t.Fatalf("ein offener Träger nimmt keine Anträge entgegen: %q",
			z.BeitrittsHindernis(offenerTraeger()))
	}
}

// Wer schon dabei ist, fragt nicht noch einmal — sonst stapelten sich
// Anträge von Leuten, die längst mitmachen.
func TestWerSchonMitgliedIstBeantragtNichts(t *testing.T) {
	z := Zugriff{Sub: "erna", Mitglied: Mitgliedschaften{"388": {RolleMitglied: true}}}
	if z.DarfBeitrittBeantragen(offenerTraeger()) {
		t.Error("ein Mitglied darf noch einmal beitreten")
	}
	// Auch der Vorstand gehört dazu (siehe IstMitglied).
	z = Zugriff{Sub: "vorstand", Mitglied: Mitgliedschaften{"388": {RolleAdmin: true}}}
	if z.DarfBeitrittBeantragen(offenerTraeger()) {
		t.Error("der Träger-Admin darf sich selbst beitreten")
	}
}

// Die Entscheidung des Zuschnitts: Eine geschlossene Gruppe nimmt keine
// Anträge entgegen. Sie steht nicht im Verzeichnis; wer sie nicht findet,
// kann ihr nichts schicken, und wer ihre Kennung errät, soll daraus nichts
// erfahren. Sie nimmt selbst auf.
func TestEineGeschlosseneGruppeNimmtKeineAntraege(t *testing.T) {
	geschlossen := offenerTraeger()
	geschlossen.Sichtbarkeit = TraegerGeschlossen

	z := Zugriff{Sub: "erna", Mitglied: Mitgliedschaften{}}
	if z.DarfBeitrittBeantragen(geschlossen) {
		t.Error("einer geschlossenen Gruppe lässt sich ein Antrag schicken")
	}
	// Auch der Betreiber nicht: Die Regel gilt für die Gruppe, nicht für
	// einzelne Personen.
	betreiber := Zugriff{Sub: "levin", Betreiber: true, Mitglied: Mitgliedschaften{}}
	if betreiber.DarfBeitrittBeantragen(geschlossen) {
		t.Error("der Betreiber umgeht die Regel der geschlossenen Gruppe")
	}
}

// Ein Träger ohne Zitadel-Projekt hat keine Mitglieder. Ein Antrag dorthin
// hätte am Ende nichts, worin er wirken könnte.
func TestOhneZitadelProjektKeinBeitritt(t *testing.T) {
	ohne := offenerTraeger()
	ohne.ProjektID = ""
	z := Zugriff{Sub: "erna", Mitglied: Mitgliedschaften{}}
	if z.DarfBeitrittBeantragen(ohne) {
		t.Error("ein Träger ohne Projekt nimmt Anträge entgegen")
	}
}

// Ein noch nicht zugelassener oder gesperrter Träger tritt nicht in
// Erscheinung — beitreten kann man ihm auch nicht.
func TestNichtZugelassenerTraegerNimmtNiemandenAuf(t *testing.T) {
	z := Zugriff{Sub: "erna", Mitglied: Mitgliedschaften{}}
	for _, status := range []TraegerStatus{TraegerBeantragt, TraegerGesperrt} {
		tr := offenerTraeger()
		tr.Status = status
		if z.DarfBeitrittBeantragen(tr) {
			t.Errorf("Beitritt zu einem Träger im Stand %q", status)
		}
	}
}

// Entscheiden darf, wer den Träger verwaltet — und mit veraltetem
// Mitgliedschafts-Stand niemand: Wer ausgetreten ist, soll niemanden mehr
// aufnehmen, bloß weil die Rössing-ID gerade schweigt.
func TestUeberBeitritteEntscheidetDerTraegerAdmin(t *testing.T) {
	tr := offenerTraeger()
	admin := Zugriff{Sub: "vorstand", Mitglied: Mitgliedschaften{"388": {RolleAdmin: true}}}
	if !admin.DarfBeitrittEntscheiden(tr) {
		t.Error("der Träger-Admin darf nicht entscheiden")
	}
	nurMitglied := Zugriff{Sub: "erna", Mitglied: Mitgliedschaften{"388": {RolleMitglied: true}}}
	if nurMitglied.DarfBeitrittEntscheiden(tr) {
		t.Error("ein einfaches Mitglied entscheidet über Aufnahmen")
	}
	veraltet := admin
	veraltet.Veraltet = true
	if veraltet.DarfBeitrittEntscheiden(tr) {
		t.Error("mit veraltetem Stand wird aufgenommen")
	}
}
