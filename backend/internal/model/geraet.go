package model

import "time"

// Device ist ein angemeldetes Gerät für den Push-Versand.
//
// Warum überhaupt: Die App holt ihre Benachrichtigungen zwar von sich aus ab,
// aber nur, während sie läuft. Wer gefragt wird, soll es merken, ohne die App
// zu öffnen — dafür braucht Google eine Kennung des Geräts.
//
// Die Kennung (Token) stammt von Firebase, gehört immer genau einer Person
// und wird nirgends ausgeliefert: Sie ist ein Schlüssel zum Gerät, kein
// Anzeigedatum. Wer sie hat, kann dem Gerät Meldungen schicken.
type Device struct {
	ID      int64  `json:"id"`
	UserSub string `json:"-"`
	// Token: die Kennung von Firebase. Nie in einer Antwort — deshalb "-".
	Token string `json:"-"`
	// Platform ist heute immer "android"; das Feld hält die Tür für später auf.
	Platform  string    `json:"platform"`
	CreatedAt time.Time `json:"createdAt"`
	// UpdatedAt sagt, wann sich das Gerät zuletzt gemeldet hat. Firebase
	// tauscht Kennungen von Zeit zu Zeit aus; wer sich lange nicht mehr
	// gemeldet hat, ist vermutlich nicht mehr da.
	UpdatedAt time.Time `json:"updatedAt"`
}

// MaxDeviceTokenLen begrenzt die Kennung. Echte FCM-Kennungen liegen bei rund
// 160 Zeichen; alles jenseits von 4 KiB ist keine Kennung, sondern Unfug.
const MaxDeviceTokenLen = 4096
