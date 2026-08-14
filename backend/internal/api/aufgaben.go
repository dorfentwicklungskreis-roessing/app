package api

import (
	"log/slog"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/vergabe"
)

// Pflege von Orten und Aufgaben — der Teil, der für REST-API, Web-Verwaltung
// und MCP gleich gelten muss.
//
// Wer eine Aufgabe zugesagt hat, wird nicht im Regen stehen gelassen: Wird
// sie gelöscht oder pausiert, geht vorher ein Hinweis an ihn raus (#7). Das
// muss vor dem Löschen passieren — danach ist der Vorgang mit der Aufgabe
// verschwunden.

// AufgabeEntfaellt sagt der Person, die zugesagt hat, dass sie nicht mehr
// losziehen muss, und beendet den Vorgang. Ein Fehler dabei hält den
// Löschvorgang nicht auf: Die Verwaltung hat entschieden, und der Zeitgeber
// der Vergabe räumt einen übrig gebliebenen Vorgang beim nächsten Takt ab.
func AufgabeEntfaellt(d *db.DB, now time.Time, zusteller vergabe.Zusteller, taskID int64) {
	e := vergabe.New(d, vergabe.Config{Now: func() time.Time { return now }, Zusteller: zusteller})
	if err := e.Entfaellt(taskID); err != nil {
		slog.Warn("Vergabe: Entfallen konnte nicht gemeldet werden", "aufgabe", taskID, "err", err)
	}
}

// OrtEntfaellt macht dasselbe für alle Aufgaben eines Ortes.
func OrtEntfaellt(d *db.DB, now time.Time, zusteller vergabe.Zusteller, placeID int64) {
	e := vergabe.New(d, vergabe.Config{Now: func() time.Time { return now }, Zusteller: zusteller})
	if err := e.OrtEntfaellt(placeID); err != nil {
		slog.Warn("Vergabe: Entfallen konnte nicht gemeldet werden", "ort", placeID, "err", err)
	}
}

// WirdPausiert sagt, ob eine Änderung eine laufende Aufgabe stilllegt. Nur
// dann ist ein Hinweis fällig — ein bloßes Speichern ist keine Nachricht wert.
func WirdPausiert(vorher, nachher model.CareTask) bool {
	return vorher.Active && !nachher.Active
}

// OrtWirdPausiert gilt entsprechend für den ganzen Ort.
func OrtWirdPausiert(vorher, nachher model.Place) bool {
	return vorher.Active && !nachher.Active
}
