package model

import "time"

// Idee ist ein Wunsch aus dem Dorf: „Was soll die App können?“ Eingereicht
// wird aus der App (angemeldet) oder über das Formular auf der Website
// (öffentlich, ohne Anmeldung) — deshalb sind Name und E-Mail freiwillig.
type Idee struct {
	ID     int64      `json:"id"`
	Name   string     `json:"name"`
	Email  string     `json:"email"`
	Wunsch string     `json:"wunsch"`
	Quelle IdeeQuelle `json:"quelle"`
	// UserSub ist das Konto der Rössing-ID, falls angemeldet eingereicht
	// wurde — sonst leer.
	UserSub   string     `json:"userSub"`
	CreatedAt time.Time  `json:"createdAt"`
	Status    IdeeStatus `json:"status"`
	// Notiz ist eine interne Bemerkung der Verwaltung. Sie verlässt die
	// Verwaltung nicht: Der öffentliche Eingang gibt sie nie zurück.
	Notiz string `json:"notiz"`
}

// IdeeStatus beschreibt, wie weit die Verwaltung mit einem Wunsch ist.
type IdeeStatus string

const (
	IdeeNeu       IdeeStatus = "neu"
	IdeeGelesen   IdeeStatus = "gelesen"
	IdeeUmgesetzt IdeeStatus = "umgesetzt"
	IdeeAbgelehnt IdeeStatus = "abgelehnt"
)

// IdeeStatusWerte sind alle Stände in der Reihenfolge, in der sie im
// Alltag durchlaufen werden.
var IdeeStatusWerte = []IdeeStatus{IdeeNeu, IdeeGelesen, IdeeUmgesetzt, IdeeAbgelehnt}

// ValidIdeeStatus prüft einen Stand.
func ValidIdeeStatus(s IdeeStatus) bool {
	for _, v := range IdeeStatusWerte {
		if v == s {
			return true
		}
	}
	return false
}

// IdeeQuelle sagt, auf welchem Weg ein Wunsch hereinkam.
type IdeeQuelle string

const (
	IdeeQuelleWebsite IdeeQuelle = "website"
	IdeeQuelleApp     IdeeQuelle = "app"
)

// ValidIdeeQuelle prüft den Weg.
func ValidIdeeQuelle(q IdeeQuelle) bool {
	return q == IdeeQuelleWebsite || q == IdeeQuelleApp
}

// IdeeStatusText liefert den Stand in Alltagssprache.
func IdeeStatusText(s IdeeStatus) string {
	switch s {
	case IdeeGelesen:
		return "gelesen"
	case IdeeUmgesetzt:
		return "umgesetzt"
	case IdeeAbgelehnt:
		return "abgelehnt"
	default:
		return "neu"
	}
}
