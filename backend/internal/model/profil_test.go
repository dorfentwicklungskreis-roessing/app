package model

import "testing"

// --- Der Schalter am Namen gilt überall ---------------------------------------

// Der Schalter heißt „für alle Dorfbewohner" oder „nur für Verwaltende". Er
// ist eine Entscheidung über den Namen, nicht über eine einzelne Liste. Eine
// Zeitlang galt er nur im Verzeichnis der Dorfbewohner — in Rangliste,
// Historie, Vergabe und Chat stand der Name trotzdem (#80).
func TestZurueckgezogenerNameStehtNichtInDerRangliste(t *testing.T) {
	p := Profile{
		UserSub: "777777777777777777", DisplayName: "Erna Beispiel", TokenName: "Erna Beispiel",
		Visibility: ProfileVisibility{
			DisplayName: VisibilityAdmins, Nickname: VisibilityVillage,
			Phone: VisibilityAdmins, Email: VisibilityAdmins, Note: VisibilityAdmins,
		},
	}

	// Das Dorf sieht den Spitznamen — nicht den Anzeigenamen und erst recht
	// nicht den Namen aus der Rössing-ID, den niemand freigegeben hat.
	if got := p.NameFuer(SichtDorf); got != AnonymousName(p.UserSub) {
		t.Errorf("das Dorf sieht %q, erwartet den Spitznamen %q", got, AnonymousName(p.UserSub))
	}
	// Die Verwaltung muss zuordnen können.
	if got := p.NameFuer(SichtVerwaltung); got != "Erna Beispiel" {
		t.Errorf("die Verwaltung sieht %q, erwartet den Anzeigenamen", got)
	}
	if p.EffectiveName() != "Erna Beispiel" {
		t.Errorf("EffectiveName ist die Sicht der Verwaltung: %q", p.EffectiveName())
	}
}

// Ein freigegebener Name steht überall — der Schalter nimmt nichts weg, was
// jemand hergegeben hat.
func TestFreigegebenerNameStehtUeberall(t *testing.T) {
	p := Profile{
		UserSub: "666666666666666666", DisplayName: "Olaf Beispiel",
		Visibility: DefaultVisibility(),
	}
	if got := p.NameFuer(SichtDorf); got != "Olaf Beispiel" {
		t.Errorf("das Dorf sieht %q, erwartet den freigegebenen Namen", got)
	}
	if got := p.NameFuer(SichtVerwaltung); got != "Olaf Beispiel" {
		t.Errorf("die Verwaltung sieht %q", got)
	}
}

// Der Nickname geht dem Anzeigenamen vor — aber nur, wenn er freigegeben ist.
func TestZurueckgezogenerNicknameFaelltAufDenAnzeigenamen(t *testing.T) {
	p := Profile{
		UserSub: "555555555555555555", Nickname: "Ernie", DisplayName: "Erna Beispiel",
		Visibility: ProfileVisibility{
			Nickname: VisibilityAdmins, DisplayName: VisibilityVillage,
			Phone: VisibilityAdmins, Email: VisibilityAdmins, Note: VisibilityAdmins,
		},
	}
	if got := p.NameFuer(SichtDorf); got != "Erna Beispiel" {
		t.Errorf("das Dorf sieht %q, erwartet den freigegebenen Anzeigenamen", got)
	}
	if got := p.NameFuer(SichtVerwaltung); got != "Ernie" {
		t.Errorf("die Verwaltung sieht %q, erwartet den Nickname", got)
	}
}
