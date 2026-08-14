package push

import (
	"os"
	"strings"
	"testing"
)

// Ein Versuch gegen das echte Google — nur von Hand, nie in der CI:
//
//	FCM_CREDENTIALS_FILE=… PUSH_ECHT=1 go test ./internal/push/ -run Echt -v
//
// Geprüft wird der Weg, der sich lokal überhaupt prüfen lässt: Aus dem
// Dienstkonto-Schlüssel ein Zugriffstoken erzeugen und damit bei FCM
// anklopfen. Die Gerätekennung ist erfunden, Google muss sie also als
// ungültig zurückweisen — genau daran ist zu erkennen, dass die
// Anmeldung geklappt hat und nur das Gerät nicht existiert.
//
// Mit einer echten Kennung (PUSH_ECHT_TOKEN) geht stattdessen eine
// richtige Nachricht auf ein richtiges Handy.
func TestEchterVersand(t *testing.T) {
	if os.Getenv("PUSH_ECHT") != "1" {
		t.Skip("nur von Hand: PUSH_ECHT=1 setzen")
	}
	pfad := os.Getenv("FCM_CREDENTIALS_FILE")
	if pfad == "" {
		t.Skip("FCM_CREDENTIALS_FILE fehlt")
	}
	geraeteToken := os.Getenv("PUSH_ECHT_TOKEN")
	echtesGeraet := geraeteToken != ""
	if !echtesGeraet {
		geraeteToken = "erfundene-kennung-zum-pruefen-der-anmeldung"
	}
	sp := neuerSpeicher(map[string][]string{"anna": {geraeteToken}})
	z, err := FromEnv(sp)
	if err != nil {
		t.Fatalf("Zugangsdaten: %v", err)
	}
	if z == nil {
		t.Skip("kein Zusteller")
	}
	err = z.Zustellen(beispielAnfrage())
	if echtesGeraet {
		if err != nil {
			t.Fatalf("Versand an echtes Gerät: %v", err)
		}
		t.Log("Nachricht verschickt")
		return
	}
	// Ohne echtes Gerät: Der Versand muss an der Kennung scheitern, nicht an
	// der Anmeldung. Der Zusteller wirft die Kennung dabei weg.
	if err != nil {
		if strings.Contains(err.Error(), "Zugriffstoken abgelehnt") {
			t.Fatalf("Anmeldung bei Google fehlgeschlagen: %v", err)
		}
		t.Fatalf("unerwarteter Fehler: %v", err)
	}
	if len(sp.tokens("anna")) != 0 {
		t.Fatal("die erfundene Kennung hätte verworfen werden müssen")
	}
	t.Log("Anmeldung bei Google in Ordnung, erfundene Kennung wie erwartet verworfen")
}
