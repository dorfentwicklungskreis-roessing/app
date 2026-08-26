package push

import (
	"errors"
	"log/slog"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Weg ist ein einzelner Versandweg — FCM oder APNs. Dieselbe Form wie
// vergabe.Zusteller, hier noch einmal aufgeschrieben, damit das Paket push
// nicht auf vergabe zeigen muss (es ist umgekehrt gedacht).
type Weg interface {
	Zustellen(n model.Notification) error
}

// Weiche schickt jede Benachrichtigung auf alle Wege, die eingerichtet sind.
//
// Die eigentliche Entscheidung „welches Gerät über welchen Dienst" trifft
// nicht die Weiche, sondern jeder Weg für sich anhand des Feldes `platform`:
// Der APNs-Weg nimmt nur „ios", der FCM-Weg alles andere (siehe IstIOS).
// Das hat einen Grund — so bleibt jeder Weg für sich vollständig und
// verständlich, und ein Gerät kann nie doppelt bedient werden.
//
// Fehlt ein Weg (kein Schlüssel hinterlegt), fällt er hier schlicht weg: Die
// App holt ihre Benachrichtigungen ohnehin ab. Ein Ausfall bei Google darf
// den Versand an die iPhones nicht aufhalten und umgekehrt — deshalb werden
// die Fehler gesammelt, nicht beim ersten abgebrochen.
type Weiche struct {
	wege []Weg
}

// NeueWeiche bündelt die Wege. Nil-Wege werden übersprungen; bleibt keiner
// übrig, ist die Rückgabe nil — der Aufrufer bekommt damit eine ehrliche
// „kein Push"-Antwort und keinen Zusteller, der nichts tut.
//
// Achtung beim Aufruf: Ein getippter Nullzeiger (*Zusteller)(nil) in einer
// Schnittstelle ist nicht nil. NeueWeiche prüft deshalb auf beides.
func NeueWeiche(fcm *Zusteller, apns *APNsZusteller) *Weiche {
	var wege []Weg
	if fcm != nil {
		wege = append(wege, fcm)
	}
	if apns != nil {
		wege = append(wege, apns)
	}
	if len(wege) == 0 {
		return nil
	}
	return &Weiche{wege: wege}
}

// FromEnv baut den gesamten Push-Versand aus der Umgebung:
//
//	FCM_CREDENTIALS_FILE  → Android über Firebase Cloud Messaging
//	APNS_KEY_FILE (+ APNS_KEY_ID, APNS_TEAM_ID, APNS_TOPIC, APNS_UMGEBUNG)
//	                      → iOS direkt bei Apple
//
// Beide sind einzeln zuschaltbar. Fehlt beides, ist die Rückgabe (nil, nil):
// Dann wird nicht gepusht, und die App holt ihre Benachrichtigungen wie immer
// selbst ab. Ist etwas hinterlegt, aber kaputt, kommt ein Fehler zurück —
// eine halb eingerichtete Zustellung soll auffallen und nicht still
// danebenlaufen.
func FromEnv(geraete Geraetespeicher) (*Weiche, error) {
	fcm, err := FCMFromEnv(geraete)
	if err != nil {
		return nil, err
	}
	apns, err := APNsFromEnv(geraete)
	if err != nil {
		return nil, err
	}
	if fcm != nil {
		slog.Info("Push: Android über Firebase Cloud Messaging")
	}
	if apns != nil {
		slog.Info("Push: iOS direkt über APNs", "adresse", apns.basis, "topic", apns.topic)
	} else {
		slog.Info("Push: kein APNs eingerichtet (APNS_KEY_FILE fehlt) — iPhones holen ihre Benachrichtigungen ab")
	}
	return NeueWeiche(fcm, apns), nil
}

func (w *Weiche) Zustellen(n model.Notification) error {
	var fehler []error
	for _, weg := range w.wege {
		if err := weg.Zustellen(n); err != nil {
			fehler = append(fehler, err)
		}
	}
	return errors.Join(fehler...)
}
