package model

import "testing"

// Der Kern der Umstellung: Wem gehört eine Aufgabe, und wer darf sie sehen?
//
// Die Regeln stehen bewusst im Domänenmodell und nicht in den Handlern —
// REST, Web-Verwaltung, Vergabe und Rangliste müssen sich alle auf dieselbe
// Antwort verlassen können.

func dek() Traeger {
	return Traeger{ID: 1, ProjektID: "111", Name: "Dorfentwicklungskreis",
		Status: TraegerZugelassen, Sichtbarkeit: TraegerOffen}
}

func geschlossenerTraeger() Traeger {
	return Traeger{ID: 2, ProjektID: "222", Name: "Dorfpflege",
		Status: TraegerZugelassen, Sichtbarkeit: TraegerGeschlossen}
}

// aussenstehend ist jemand aus dem Dorf ohne jede Mitgliedschaft.
func aussenstehend() Zugriff {
	return Zugriff{Sub: "fremd", Mitglied: Mitgliedschaften{}}
}

func mitgliedVon(t Traeger) Zugriff {
	return Zugriff{Sub: "mitglied", Mitglied: Mitgliedschaften{
		t.ProjektID: {RolleMitglied: true}}}
}

func adminVon(t Traeger) Zugriff {
	return Zugriff{Sub: "traeger-admin", Mitglied: Mitgliedschaften{
		t.ProjektID: {RolleAdmin: true}}}
}

func betreiber() Zugriff {
	return Zugriff{Sub: "betreiber", Betreiber: true, Mitglied: Mitgliedschaften{}}
}

// Die wichtigste Regel überhaupt: Eine Aufgabe „nur_mitglieder“ darf
// niemandem außerhalb des Trägers angezeigt werden.
func TestNurMitgliederAufgabeBleibtDrinnen(t *testing.T) {
	traeger := dek()
	faelle := []struct {
		name     string
		z        Zugriff
		erwartet bool
	}{
		{"Außenstehende sehen sie nicht", aussenstehend(), false},
		{"Mitglieder sehen sie", mitgliedVon(traeger), true},
		{"Träger-Admins sehen sie", adminVon(traeger), true},
		{"Der Betreiber sieht alles", betreiber(), true},
		{"Mitglied eines anderen Trägers sieht sie nicht", mitgliedVon(geschlossenerTraeger()), false},
	}
	for _, f := range faelle {
		t.Run(f.name, func(t *testing.T) {
			if got := f.z.SiehtAufgabe(traeger, AufgabeNurMitglieder); got != f.erwartet {
				t.Errorf("SiehtAufgabe = %v, erwartet %v", got, f.erwartet)
			}
		})
	}
}

// Umgekehrt: Eine geschlossene Gruppe darf durchaus öffentlich ausschreiben.
func TestGeschlosseneGruppeDarfOeffentlichAusschreiben(t *testing.T) {
	traeger := geschlossenerTraeger()
	if !aussenstehend().SiehtAufgabe(traeger, AufgabeOeffentlich) {
		t.Error("öffentliche Aufgabe einer geschlossenen Gruppe muss sichtbar sein")
	}
	// Die Gruppe selbst steht dabei trotzdem nicht im Verzeichnis.
	if aussenstehend().SiehtTraeger(traeger) {
		t.Error("geschlossener Träger darf Außenstehenden nicht im Verzeichnis erscheinen")
	}
	if !mitgliedVon(traeger).SiehtTraeger(traeger) {
		t.Error("Mitglieder müssen ihren geschlossenen Träger sehen")
	}
}

// Ein noch nicht zugelassener Träger ist für alle unsichtbar — nur der
// Plattform-Betreiber sieht ihn, sonst könnte sich jede Gruppe selbst
// freischalten.
func TestNichtZugelassenerTraegerIstUnsichtbar(t *testing.T) {
	beantragt := Traeger{ID: 3, ProjektID: "333", Name: "Neu", Status: TraegerBeantragt,
		Sichtbarkeit: TraegerOffen}
	for _, z := range []Zugriff{aussenstehend(), mitgliedVon(beantragt), adminVon(beantragt)} {
		if z.SiehtTraeger(beantragt) {
			t.Errorf("%s sieht einen nicht zugelassenen Träger", z.Sub)
		}
		if z.SiehtAufgabe(beantragt, AufgabeOeffentlich) {
			t.Errorf("%s sieht die Aufgabe eines nicht zugelassenen Trägers", z.Sub)
		}
	}
	if !betreiber().SiehtTraeger(beantragt) {
		t.Error("der Betreiber muss den Antrag sehen, um ihn zuzulassen")
	}
}

// Verwalten darf nur der admin des jeweiligen Trägers — und der Betreiber.
func TestNurTraegerAdminDarfVerwalten(t *testing.T) {
	traeger := dek()
	if adminVon(traeger).DarfVerwalten(traeger) != true {
		t.Error("Träger-Admin darf nicht verwalten")
	}
	if mitgliedVon(traeger).DarfVerwalten(traeger) {
		t.Error("einfaches Mitglied darf nicht verwalten")
	}
	if aussenstehend().DarfVerwalten(traeger) {
		t.Error("Außenstehende dürfen nicht verwalten")
	}
	if !betreiber().DarfVerwalten(traeger) {
		t.Error("der Betreiber muss verwalten dürfen")
	}
	// Ein gesperrter Träger wird von niemandem außer dem Betreiber gepflegt.
	gesperrt := traeger
	gesperrt.Status = TraegerGesperrt
	if adminVon(gesperrt).DarfVerwalten(gesperrt) {
		t.Error("gesperrter Träger darf nicht mehr von seinem Admin gepflegt werden")
	}
}

// Der admin eines Trägers ist zugleich Mitglied — sonst müsste man beide
// Rollen vergeben, und ein vergessenes „mitglied“ würde ihn aus seinen
// eigenen internen Aufgaben aussperren.
func TestTraegerAdminIstAuchMitglied(t *testing.T) {
	traeger := dek()
	if !adminVon(traeger).Mitglied.IstMitglied(traeger.ProjektID) {
		t.Error("admin muss als Mitglied gelten")
	}
}

// Ein Träger ohne hinterlegte Zitadel-Projekt-ID (Platzhalter, solange die
// Produktion noch keinen Zugang hat) darf niemanden zum Admin machen:
// Sonst wäre eine leere Projekt-ID der Generalschlüssel.
func TestTraegerOhneProjektIDHatKeineAdmins(t *testing.T) {
	ohne := Traeger{ID: 4, ProjektID: "", Name: "Platzhalter",
		Status: TraegerZugelassen, Sichtbarkeit: TraegerOffen}
	leer := Zugriff{Sub: "wer", Mitglied: Mitgliedschaften{"": {RolleAdmin: true}}}
	if leer.DarfVerwalten(ohne) {
		t.Error("leere Projekt-ID darf keinen Zugriff geben")
	}
	if leer.Mitglied.IstMitglied(ohne.ProjektID) {
		t.Error("leere Projekt-ID darf keine Mitgliedschaft begründen")
	}
}

// Ein Ort ist sichtbar, wenn wenigstens eine seiner Aufgaben sichtbar ist —
// oder wenn man dem Träger angehört. Sonst verriete eine leere Karte-Nadel
// die Existenz interner Aufgaben.
func TestOrtIstNurMitSichtbarerAufgabeSichtbar(t *testing.T) {
	traeger := dek()
	nurIntern := []TaskSichtbarkeit{AufgabeNurMitglieder}
	if aussenstehend().SiehtOrt(traeger, nurIntern) {
		t.Error("Ort mit ausschließlich internen Aufgaben darf außen nicht auftauchen")
	}
	if !mitgliedVon(traeger).SiehtOrt(traeger, nurIntern) {
		t.Error("Mitglieder müssen den Ort sehen")
	}
	if !aussenstehend().SiehtOrt(traeger, []TaskSichtbarkeit{AufgabeNurMitglieder, AufgabeOeffentlich}) {
		t.Error("Ort mit einer öffentlichen Aufgabe muss außen sichtbar sein")
	}
	// Ein Ort ganz OHNE Aufgaben verrät nichts — er ist frisch angelegt oder
	// seine einmalige Aufgabe ist abgeräumt („einmal zum Bahnhof fahren“,
	// erledigt). Für ihn gilt schlicht die Sichtbarkeit seines Trägers.
	if !aussenstehend().SiehtOrt(traeger, nil) {
		t.Error("Ort ohne Aufgaben eines offenen Trägers muss sichtbar bleiben")
	}
	if !mitgliedVon(traeger).SiehtOrt(traeger, nil) {
		t.Error("Mitglieder müssen den frisch angelegten Ort sehen")
	}
	// Bei einer geschlossenen Gruppe bleibt er drinnen.
	if aussenstehend().SiehtOrt(geschlossenerTraeger(), nil) {
		t.Error("Ort ohne Aufgaben einer geschlossenen Gruppe darf außen nicht auftauchen")
	}
}

// Fällt Zitadel aus, arbeiten wir mit dem letzten bekannten Stand weiter —
// aber nur lesend. Wer schreibt, muss auf gesicherte Mitgliedschaften bauen.
func TestVeralteterStandDarfNichtSchreiben(t *testing.T) {
	traeger := dek()
	z := adminVon(traeger)
	z.Veraltet = true
	if !z.SiehtAufgabe(traeger, AufgabeNurMitglieder) {
		t.Error("mit dem letzten bekannten Stand muss weiter gelesen werden können")
	}
	if z.DarfVerwalten(traeger) {
		t.Error("mit veraltetem Stand darf nicht verwaltet werden")
	}
	// Der Betreiber hängt an der Rolle im Token, nicht an Zitadels API —
	// er bleibt handlungsfähig.
	b := betreiber()
	b.Veraltet = true
	if !b.DarfVerwalten(traeger) {
		t.Error("der Betreiber muss auch bei Zitadel-Ausfall verwalten können")
	}
}

// Der Name einer geschlossenen Gruppe darf nicht über eine öffentliche
// Aufgabe nach außen sickern.
//
// Eine geschlossene Gruppe schreibt öffentlich aus — das ist gewollt. Wer die
// Aufgabe sieht, erführe dabei aber nebenbei, dass es die Gruppe gibt und wie
// sie heißt, obwohl sie ausdrücklich nicht im Verzeichnis steht. Angezeigt
// wird deshalb ein neutraler Ersatztext.
func TestGeschlosseneGruppeVerraetIhrenNamenNicht(t *testing.T) {
	geschlossen := geschlossenerTraeger()
	offen := dek()

	if got := aussenstehend().TraegerAnzeigeName(geschlossen); got == geschlossen.Name {
		t.Errorf("der Name der geschlossenen Gruppe steht außen: %q", got)
	}
	if got := aussenstehend().TraegerAnzeigeName(geschlossen); got != TraegerNameVerdeckt {
		t.Errorf("Ersatztext = %q, erwartet %q", got, TraegerNameVerdeckt)
	}

	// Für Mitglieder, Verwaltende und den Betreiber bleibt es beim Namen.
	for _, z := range []Zugriff{mitgliedVon(geschlossen), adminVon(geschlossen), betreiber()} {
		if got := z.TraegerAnzeigeName(geschlossen); got != geschlossen.Name {
			t.Errorf("%s sieht %q statt des Namens", z.Sub, got)
		}
	}
	// Eine offene Gruppe nennt sich selbstverständlich beim Namen.
	if got := aussenstehend().TraegerAnzeigeName(offen); got != offen.Name {
		t.Errorf("offene Gruppe verdeckt ihren Namen: %q", got)
	}
}

// Ein Träger unter sich selbst wäre kein Dach, sondern ein Kreis. Die
// Prüfung, ob es das Dach überhaupt gibt und ob es selbst unter einem steht,
// braucht den Bestand und sitzt deshalb in der Ablage — hier steht nur, was
// sich am Datensatz allein entscheiden lässt.
func TestTraegerIstNichtSeinEigenesDach(t *testing.T) {
	tr := Traeger{ID: 7, ParentID: 7, Name: "AK 2",
		Status: TraegerZugelassen, Sichtbarkeit: TraegerOffen}
	if err := tr.Validate(); err == nil {
		t.Fatal("ein Träger durfte sein eigenes Dach sein")
	}
}

func TestTraegerOhneDachIstGueltig(t *testing.T) {
	tr := Traeger{ID: 7, Name: "Dorfpflege",
		Status: TraegerZugelassen, Sichtbarkeit: TraegerOffen}
	if err := tr.Validate(); err != nil {
		t.Fatalf("ein Träger ohne Dach wurde abgewiesen: %v", err)
	}
	if tr.IstUnterTraeger() {
		t.Error("ohne parentId ist er kein Unter-Träger")
	}
}

// --- Selbst entschieden ------------------------------------------------------

// Wer den Träger verwaltet, darf über den eigenen Antrag entscheiden — in
// einem Dorfverein ist der Vorstand oft eine Person, und er gibt die
// Einweisung ohnehin selbst. Verboten ist es also nicht. Nachvollziehbar soll
// es trotzdem sein (#34).
func TestSelbstEntschiedenIstErkennbar(t *testing.T) {
	selbst := BefaehigungsAntrag{UserSub: "olaf", EntschiedenVon: "olaf", Status: AntragErteilt}
	if !selbst.SelbstEntschieden() {
		t.Error("selbst erteilter Antrag wird nicht erkannt")
	}

	fremd := BefaehigungsAntrag{UserSub: "erna", EntschiedenVon: "olaf", Status: AntragErteilt}
	if fremd.SelbstEntschieden() {
		t.Error("fremd entschiedener Antrag gilt als selbst erteilt")
	}

	// Noch nicht entschieden ist nicht dasselbe wie selbst entschieden.
	offen := BefaehigungsAntrag{UserSub: "olaf", Status: AntragBeantragt}
	if offen.SelbstEntschieden() {
		t.Error("ein offener Antrag gilt als selbst erteilt")
	}

	// Beim Beitritt wiegt es schwerer — erkannt wird es genauso.
	eigen := Beitritt{UserSub: "olaf", EntschiedenVon: "olaf", Status: AntragErteilt}
	if !eigen.SelbstEntschieden() {
		t.Error("selbst aufgenommener Beitritt wird nicht erkannt")
	}
	if (Beitritt{UserSub: "erna", EntschiedenVon: "olaf"}).SelbstEntschieden() {
		t.Error("fremd entschiedener Beitritt gilt als selbst aufgenommen")
	}
}
