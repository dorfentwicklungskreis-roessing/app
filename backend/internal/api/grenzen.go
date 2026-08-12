package api

import (
	"errors"
	"log/slog"
	"math"
	"net/http"
	"unicode/utf8"
)

// Grenzen und Fehlerausgabe der API — bewusst in einer eigenen Datei, damit
// beides an einer Stelle steht und nachvollziehbar bleibt.

const (
	// MaxTextLen ist die Obergrenze für freie Texte (Name, Beschreibung,
	// Titel, Notiz) in Zeichen. Großzügig für den Alltag, aber weit weg von
	// Größen, die die kleine SQLite-Datei oder die Oberfläche sprengen.
	MaxTextLen = 500
	// MaxTage begrenzt Intervall- und Rot-Schwellen (rund 27 Jahre). Größere
	// Werte sind kein Betrieb, sondern nur ein Rechenrisiko.
	MaxTage = 10000
	// MaxLiter begrenzt die Gießmenge je Meldung.
	MaxLiter = 100000
)

// pruefeText stellt sicher, dass ein freier Text nicht zu lang ist.
func pruefeText(feld, wert string) error {
	if utf8.RuneCountInString(wert) > MaxTextLen {
		return errors.New(feld + " ist zu lang (höchstens " + itoa(MaxTextLen) + " Zeichen)")
	}
	return nil
}

// endlich prüft, ob eine Zahl überhaupt eine brauchbare Zahl ist. NaN und
// Unendlich bestehen sonst jede Bereichsprüfung, weil Vergleiche mit NaN
// immer falsch sind.
func endlich(feld string, v float64, min, max float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return errors.New(feld + " muss eine Zahl sein")
	}
	if v < min || v > max {
		return errors.New(feld + " liegt außerhalb des zulässigen Bereichs")
	}
	return nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// writeInternal beantwortet einen technischen Fehler. Nach außen geht nur eine
// nichtssagende Meldung; die Ursache steht ausschließlich im Log. Sonst
// verrieten Datenbank- und Dateipfadfehler Interna an jeden Aufrufer.
func writeInternal(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("api: interner Fehler", "methode", r.Method, "pfad", r.URL.Path, "err", err)
	writeErr(w, http.StatusInternalServerError, "interner Fehler")
}
