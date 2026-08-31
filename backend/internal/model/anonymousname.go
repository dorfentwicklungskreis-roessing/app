package model

import (
	"crypto/sha256"
	"encoding/binary"
)

// Spitzname für Namenlose.
//
// Wer weder Nickname noch Anzeigenamen gesetzt hat und dessen Rössing-ID
// keinen Namen mitliefert, stand bisher als Leerstelle in der Rangliste: eine
// Zeile mit Punkten, Litern und Auszeichnungen — nur ohne Namen. Statt der
// Leerstelle bekommt diese Person hier einen freundlichen Platzhalter
// („Lustiger Lurch“).
//
// Der Platzhalter ist KEIN Name. Er steht an der Stelle, an der sonst nichts
// stünde, und tritt hinter jeden selbst gewählten oder aus der Rössing-ID
// übernommenen Namen zurück (siehe Profile.EffectiveName). Er macht auch
// niemanden sichtbar, der es nicht ohnehin schon ist: Wer in einer Liste
// fehlt, fehlt weiterhin (siehe Profile.AsMember).
//
// Was er über die Person verrät: nichts. Er entsteht allein aus der
// Zitadel-Kennung, nicht aus Name, Nickname, E-Mail oder Telefonnummer — aus
// „Lustiger Lurch“ lässt sich deshalb keines dieser Felder zurückrechnen. Die
// Kennung selbst steht ohnehin in denselben Antworten (`userSub`), es kommt
// also auch keine neue Verkettung hinzu.

// anonymousNameSalt trennt diese Ableitung von jeder anderen Verwendung
// derselben Kennung. Kein Geheimnis — nur eine feste Domänentrennung, damit
// der Spitzname sich nicht mit einem anderen Hash derselben Kennung deckt.
const anonymousNameSalt = "dorf-app/spitzname/v1:"

// anonymousAdjectives sind die Adjektive des Spitznamens, in starker
// Beugung männlich Nominativ („Lustiger …“) — deshalb enden alle auf „-er“
// und deshalb sind in anonymousAnimals nur männliche Tiere aufgeführt.
//
// Die Auswahl ist bewusst eng: ausschließlich freundliche oder neutrale
// Wörter. Verboten sind nicht nur Schimpfwörter, sondern auch alles, was
// herablassend gelesen werden könnte („brav“, „putzig“, „drollig“), alles
// über Alter, Körper oder Fähigkeiten („alt“, „dick“, „langsam“, „blind“)
// und alles Ironiefähige („komisch“, „schräg“, „seltsam“).
//
// Wer die Liste ändert, ändert die Spitznamen aller Namenlosen: Die Auswahl
// läuft über den Rest einer Division durch die Listenlänge, ein eingefügtes
// oder entferntes Wort verschiebt also alle. Das ist keine Falle, sondern
// eine bewusste Abwägung — dafür braucht es keine Tabelle und keine
// Datenwanderung. Der Golden-Test in anonymousname_test.go schlägt an,
// sobald sich etwas verschiebt.
var anonymousAdjectives = []string{
	"Freundlicher",
	"Fröhlicher",
	"Flinker",
	"Emsiger",
	"Mutiger",
	"Sanfter",
	"Ruhiger",
	"Kluger",
	"Geduldiger",
	"Gelassener",
	"Herzlicher",
	"Sonniger",
	"Eifriger",
	"Froher",
	"Wachsamer",
	"Zufriedener",
	"Gemütlicher",
	"Verträumter",
	"Verspielter",
	"Geschickter",
	"Pfiffiger",
	"Quirliger",
	"Lustiger",
	"Goldener",
	"Silberner",
	"Hilfsbereiter",
	"Tapferer",
	"Aufgeweckter",
}

// anonymousAnimals sind die Tiere des Spitznamens. Alle sind männlich („der
// Dachs“), sonst passte die Endung des Adjektivs nicht.
//
// Auch hier ist die Auswahl eng, und zwar Paar für Paar geprüft: Tiere, die
// im Deutschen als Beschimpfung eines Menschen taugen, fehlen bewusst —
// Esel, Ochse, Affe, Wurm, Ratte, Bock, Pfau, Gockel, Kauz, Vogel („komischer
// Vogel“), Molch („Lustmolch“), Reiher („reihern“), Specht, Rabe, Spatz,
// Frosch („sei kein Frosch“), Maulwurf („blind wie ein …“), Krebs (Krankheit).
// Was fehlt, fehlt aus einem Grund; wer etwas hinzufügt, prüft es gegen alle
// Adjektive, nicht nur für sich allein.
var anonymousAnimals = []string{
	"Lurch",
	"Dachs",
	"Igel",
	"Otter",
	"Marder",
	"Biber",
	"Luchs",
	"Waschbär",
	"Fuchs",
	"Storch",
	"Kranich",
	"Fink",
	"Zeisig",
	"Kolibri",
	"Falter",
	"Käfer",
	"Salamander",
	"Seestern",
	"Delfin",
	"Pinguin",
	"Elch",
	"Schwan",
	"Uhu",
	"Falke",
	"Hase",
	"Grashüpfer",
	"Seehund",
	"Ameisenbär",
}

// AnonymousName bildet eine Zitadel-Kennung auf einen Spitznamen ab.
//
// Reproduzierbar statt gewürfelt: Dieselbe Kennung ergibt in jedem Prozess,
// nach jedem Neustart und in jeder Kopie der Datenbank denselben Namen. Ein
// gewürfelter und gespeicherter Wert bräuchte eine Spalte, einen Schreibweg
// beim ersten Lesen und eine Nachrüstung für die Bestandsprofile — und beim
// Einspielen einer Sicherung ohne diese Spalte hieße jemand plötzlich anders.
//
// Ohne Kennung gibt es keinen Spitznamen: Wo nichts identifiziert wird, ist
// auch nichts zu benennen.
func AnonymousName(userSub string) string {
	if userSub == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(anonymousNameSalt + userSub))
	adjective := anonymousAdjectives[binary.BigEndian.Uint32(sum[0:4])%uint32(len(anonymousAdjectives))]
	animal := anonymousAnimals[binary.BigEndian.Uint32(sum[4:8])%uint32(len(anonymousAnimals))]
	return adjective + " " + animal
}
