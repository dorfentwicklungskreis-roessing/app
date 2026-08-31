package model

import (
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// --- Stabilität ---------------------------------------------------------------

// TestAnonymousNameIstStabil: Dieselbe Kennung ergibt denselben Namen — im
// selben Aufruf, im nächsten und nach einem Neustart des Servers. Die
// erwarteten Werte stehen hier fest verdrahtet: Sie sind der Alarm, der
// anschlägt, wenn jemand die Wortlisten umsortiert oder ein Wort einfügt und
// damit alle Namenlosen umbenennt.
func TestAnonymousNameIstStabil(t *testing.T) {
	golden := map[string]string{
		"111111111111111111": "Ruhiger Schwan",
		"222222222222222222": "Freundlicher Dachs",
		"333333333333333333": "Goldener Falke",
		"444444444444444444": "Sonniger Uhu",
		"erna-sub":           "Tapferer Salamander",
	}
	for sub, want := range golden {
		if got := AnonymousName(sub); got != want {
			t.Errorf("AnonymousName(%q) = %q, erwartet %q — wurden die Wortlisten geändert? "+
				"Dann heißen alle Namenlosen ab jetzt anders.", sub, got, want)
		}
	}
	// Zweimal gefragt, zweimal dasselbe: keine Zufallsquelle, keine Uhr.
	for sub := range golden {
		if AnonymousName(sub) != AnonymousName(sub) {
			t.Errorf("AnonymousName(%q) ist nicht reproduzierbar", sub)
		}
	}
}

// TestAnonymousNameBrauchtEineKennung: Ohne Kennung gibt es nichts zu
// benennen — und damit auch keinen Spitznamen für ein leeres Profil.
func TestAnonymousNameBrauchtEineKennung(t *testing.T) {
	if got := AnonymousName(""); got != "" {
		t.Errorf("AnonymousName(\"\") = %q, erwartet den leeren String", got)
	}
	if got := (Profile{}).EffectiveName(); got != "" {
		t.Errorf("leeres Profil = %q, erwartet den leeren String", got)
	}
}

// TestAnonymousNameHaengtNurAnDerKennung: Der Spitzname darf nichts über die
// Person verraten. Er entsteht deshalb allein aus der Kennung — Name,
// Nickname, E-Mail, Telefon und Notiz gehen nicht ein.
func TestAnonymousNameHaengtNurAnDerKennung(t *testing.T) {
	sub := "555555555555555555"
	erwartet := AnonymousName(sub)
	p := Profile{UserSub: sub, Phone: "05066 123456", Email: "erna@example.org",
		Note: "erreichbar abends"}
	if got := p.EffectiveName(); got != erwartet {
		t.Errorf("EffectiveName mit Kontaktdaten = %q, erwartet %q", got, erwartet)
	}
	// Und umgekehrt: zwei verschiedene Kennungen dürfen nicht auf denselben
	// Namen zusammenfallen, nur weil sie sich ähnlich sehen.
	if AnonymousName("600000000000000001") == AnonymousName("600000000000000002") {
		t.Error("benachbarte Kennungen ergeben denselben Spitznamen")
	}
}

// TestAnonymousNameStreut: Die Namen verteilen sich über beide Wortlisten.
// Ein Fehler in der Indexrechnung (etwa dieselben vier Bytes für beide
// Wörter) fiele sonst nicht auf — alle hießen dann „Freundlicher Lurch“.
func TestAnonymousNameStreut(t *testing.T) {
	adjektive := map[string]bool{}
	tiere := map[string]bool{}
	for i := 0; i < 2000; i++ {
		name := AnonymousName("sub-" + strconv.Itoa(i))
		teile := strings.SplitN(name, " ", 2)
		if len(teile) != 2 {
			t.Fatalf("Spitzname ohne zwei Teile: %q", name)
		}
		adjektive[teile[0]] = true
		tiere[teile[1]] = true
	}
	if len(adjektive) != len(anonymousAdjectives) {
		t.Errorf("%d von %d Adjektiven kamen vor", len(adjektive), len(anonymousAdjectives))
	}
	if len(tiere) != len(anonymousAnimals) {
		t.Errorf("%d von %d Tieren kamen vor", len(tiere), len(anonymousAnimals))
	}
}

// --- Vorrang echter Namen -----------------------------------------------------

// TestEchteNamenGehenVor: Der Spitzname ist der letzte Ausweg. Wer einen
// Nickname, einen Anzeigenamen oder einen Namen in der Rössing-ID hat, behält
// ihn — sonst wäre aus einem Platzhalter eine Umbenennung geworden.
func TestEchteNamenGehenVor(t *testing.T) {
	sub := "777777777777777777"
	spitzname := AnonymousName(sub)
	faelle := []struct {
		was     string
		profil  Profile
		erwartt string
	}{
		{"Nickname schlägt alles",
			Profile{UserSub: sub, Nickname: "Gießmeisterin", DisplayName: "Erna Beispiel", TokenName: "Erna"},
			"Gießmeisterin"},
		{"Anzeigename ohne Nickname",
			Profile{UserSub: sub, DisplayName: "Erna Beispiel", TokenName: "Erna"},
			"Erna Beispiel"},
		{"Name aus der Rössing-ID",
			Profile{UserSub: sub, TokenName: "Erna"},
			"Erna"},
		{"gar nichts — erst hier greift der Spitzname",
			Profile{UserSub: sub},
			spitzname},
	}
	for _, f := range faelle {
		if got := f.profil.EffectiveName(); got != f.erwartt {
			t.Errorf("%s: EffectiveName = %q, erwartet %q", f.was, got, f.erwartt)
		}
	}
	if spitzname == "" {
		t.Fatal("der Spitzname ist leer — dann füllt er keine Leerstelle")
	}
}

// TestSpitznameFuelltDieRangliste: Der Weg, den Rangliste, Historie und
// Vergabe gehen (NameResolver.Resolve), liefert für ein namenloses Profil den
// Spitznamen — und lässt einen eingefrorenen fremden Namen in Ruhe.
func TestSpitznameFuelltDieRangliste(t *testing.T) {
	sub := "888888888888888888"
	namen := NameResolver{sub: {UserSub: sub}}

	if got := namen.Resolve(sub, ""); got != AnonymousName(sub) {
		t.Errorf("Rangliste = %q, erwartet den Spitznamen %q", got, AnonymousName(sub))
	}
	// Eine von der Verwaltung nachgetragene Meldung läuft unter fremdem
	// Namen; der bleibt stehen (siehe MatchesStoredName).
	if got := namen.Resolve(sub, "Karl Nachbar"); got != "Karl Nachbar" {
		t.Errorf("Nachtrag = %q, erwartet den eingefrorenen Namen", got)
	}
	// Wer gar kein Profil hat, bekommt keinen Spitznamen: Über ihn ist
	// nichts bekannt, nicht einmal, dass er die App je geöffnet hat.
	if got := namen.Resolve("fremde-kennung", "Alte Meldung"); got != "Alte Meldung" {
		t.Errorf("ohne Profil = %q, erwartet den gespeicherten Namen", got)
	}
}

// TestSpitznameHoltNiemandenInsVerzeichnis: Wer weder Anzeigenamen noch
// Nickname freigegeben hat, taucht in der Dorfbewohner-Liste weiterhin nicht
// auf. Der Spitzname füllt Leerstellen; er darf niemanden sichtbarer machen,
// als er es vorher war.
func TestSpitznameHoltNiemandenInsVerzeichnis(t *testing.T) {
	sub := "999999999999999999"
	ohneAlles := Profile{UserSub: sub, Visibility: DefaultVisibility()}
	if _, ok := ohneAlles.AsMember(false); ok {
		t.Error("ein namenloses Profil steht plötzlich im Verzeichnis")
	}
	zurueckhaltend := Profile{UserSub: sub, DisplayName: "Erna Beispiel", Nickname: "Gießmeisterin",
		Visibility: ProfileVisibility{DisplayName: VisibilityAdmins, Nickname: VisibilityAdmins,
			Phone: VisibilityAdmins, Email: VisibilityAdmins, Note: VisibilityAdmins}}
	if _, ok := zurueckhaltend.AsMember(false); ok {
		t.Error("ein zurückhaltendes Profil steht plötzlich im Verzeichnis")
	}
	// Für die Verwaltung dagegen ist die Leerstelle nur unpraktisch: Sie
	// sieht die Zeile ohnehin und bekommt jetzt einen Namen dazu.
	m, ok := ohneAlles.AsMember(true)
	if !ok || m.Name != AnonymousName(sub) {
		t.Errorf("Verwaltungssicht = %q (ok=%v), erwartet den Spitznamen", m.Name, ok)
	}
}

// --- Beleidigungsfreiheit -----------------------------------------------------

// verbotenePaare sind Wortkombinationen, die im Deutschen als Herabsetzung
// gelesen werden — auch dann, wenn beide Wörter für sich harmlos wirken.
// Genau darum wird hier Paar für Paar geprüft und nicht Wort für Wort.
var verbotenePaare = []string{
	"komischer vogel", "schräger vogel", "schräger typ", "seltsamer kauz",
	"komischer kauz", "alter kauz", "lustiger molch", "geiler bock",
	"sturer bock", "fauler hund", "armer hund", "armer wurm", "armer tropf",
	"dummer esel", "blöder esel", "eitler pfau", "stolzer gockel",
	"lahme ente", "blinde kuh", "dumme gans", "alte schachtel", "alter sack",
	"fetter wal", "flotter otto", "toter fisch", "kalter fisch",
	"trauriger wicht", "armer wicht", "dicker brocken", "blinder maulwurf",
	"frecher dachs", "schmutziger fink", "brummiger bär", "faules stück",
	"tapsiger bär", "lahmer sack", "müder krieger",
}

// verboteneTiere sind Tiere, die im Deutschen als Beschimpfung eines
// Menschen dienen. Sie haben in der Liste nichts zu suchen — egal, wie
// freundlich das Adjektiv davor ist.
var verboteneTiere = []string{
	"esel", "ochse", "affe", "wurm", "ratte", "bock", "pfau", "gockel",
	"kauz", "vogel", "molch", "reiher", "specht", "rabe", "spatz", "kater",
	"krebs", "sau", "schwein", "eber", "keiler", "ziege", "kuh", "gans",
	"ente", "hund", "hammel", "schaf", "made", "laus", "zecke", "wanze",
	"floh", "kröte", "schlange", "geier", "hyäne", "walross", "nilpferd",
	"maulwurf", "frosch", "wicht", "tropf",
}

// verboteneAdjektive stehen in derselben Beugung wie die Liste selbst, damit
// exakt verglichen werden kann. Sie treffen Alter, Körper, Verstand,
// Fähigkeiten — und alles, was herablassend oder ironisch gemeint sein kann.
var verboteneAdjektive = []string{
	"alter", "junger", "dicker", "fetter", "dünner", "dürrer", "kleiner",
	"großer", "hässlicher", "dummer", "blöder", "fauler", "lahmer",
	"langsamer", "blinder", "tauber", "krummer", "schiefer", "schräger",
	"komischer", "seltsamer", "sonderbarer", "schrulliger", "armer",
	"trauriger", "einsamer", "müder", "sturer", "eitler", "frecher",
	"wilder", "schmutziger", "dreckiger", "tollpatschiger", "ängstlicher",
	"schüchterner", "braver", "artiger", "putziger", "drolliger",
	"possierlicher", "tapsiger", "nasser", "geiler", "stolzer", "brummiger",
}

// TestWortlistenSindHarmlos prüft die Wortlisten so, wie sie gelesen werden:
// nicht Wort für Wort, sondern in jeder Kombination, die entstehen kann.
// Verglichen wird auf ganze Wörter, nicht auf Teilstücke — sonst schlüge
// „Falter“ wegen „alt“ und „Seehund“ wegen „hund“ an, und der Test wäre nach
// dem dritten Fehlalarm abgeschaltet.
func TestWortlistenSindHarmlos(t *testing.T) {
	enthaelt := func(liste []string, wort string) bool {
		for _, x := range liste {
			if x == wort {
				return true
			}
		}
		return false
	}
	for _, adjektiv := range anonymousAdjectives {
		for _, tier := range anonymousAnimals {
			paar := adjektiv + " " + tier
			klein := strings.ToLower(paar)
			if enthaelt(verbotenePaare, klein) {
				t.Errorf("die Paarung %q ist eine feststehende Herabsetzung", paar)
			}
			if enthaelt(verboteneAdjektive, strings.ToLower(adjektiv)) {
				t.Errorf("%q: das Adjektiv %q setzt herab", paar, adjektiv)
			}
			if enthaelt(verboteneTiere, strings.ToLower(tier)) {
				t.Errorf("%q: %q dient im Deutschen als Beschimpfung", paar, tier)
			}
		}
	}
}

// TestWortlistenSindWohlgeformt hält die Form fest, von der der Rest abhängt:
// Adjektive in starker Beugung männlich („-er“), Tiere männlich und
// einzelnes Wort, alles groß geschrieben, nichts doppelt.
func TestWortlistenSindWohlgeformt(t *testing.T) {
	if len(anonymousAdjectives) == 0 || len(anonymousAnimals) == 0 {
		t.Fatal("eine der Wortlisten ist leer — dann gibt es keinen Spitznamen")
	}
	pruefe := func(was string, liste []string) {
		gesehen := map[string]bool{}
		for _, wort := range liste {
			if gesehen[wort] {
				t.Errorf("%s: %q steht doppelt in der Liste", was, wort)
			}
			gesehen[wort] = true
			if wort == "" || strings.ContainsAny(wort, " \t\n") {
				t.Errorf("%s: %q ist kein einzelnes Wort", was, wort)
			}
			for _, r := range wort {
				if unicode.IsControl(r) {
					t.Errorf("%s: %q enthält ein Steuerzeichen", was, wort)
				}
			}
			if r := []rune(wort); len(r) > 0 && !unicode.IsUpper(r[0]) {
				t.Errorf("%s: %q fängt klein an", was, wort)
			}
		}
	}
	pruefe("Adjektiv", anonymousAdjectives)
	pruefe("Tier", anonymousAnimals)

	for _, adjektiv := range anonymousAdjectives {
		if !strings.HasSuffix(adjektiv, "er") {
			t.Errorf("das Adjektiv %q endet nicht auf „-er“ — dann passt es nicht zu einem "+
				"männlichen Tier („Lustiger Lurch“)", adjektiv)
		}
	}
}
