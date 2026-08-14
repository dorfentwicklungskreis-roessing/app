package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/db"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Profilverwaltung: eigenes Profil lesen und ändern, Liste der Dorfbewohner
// mit den jeweils freigegebenen Feldern.

const (
	// MaxNameLen begrenzt den Anzeigenamen.
	MaxNameLen = 80
	// MaxNickLen begrenzt den Nickname für Rangliste und Meldungen.
	MaxNickLen = 40
	// MaxPhoneLen begrenzt die Telefonnummer.
	MaxPhoneLen = 40
	// MaxMailLen begrenzt die E-Mail-Adresse.
	MaxMailLen = 120
	// MaxNoteLen begrenzt den kurzen Freitext („erreichbar abends“).
	MaxNoteLen = 200
	// MinPhoneDigits ist die kleinste Ziffernzahl, die noch eine Rufnummer
	// sein kann. Großzügig, aber „12“ ist keine.
	MinPhoneDigits = 5
)

// telefonZeichen lässt alles zu, was in aufgeschriebenen Rufnummern üblich
// ist: Ziffern, Leerzeichen, +, /, -, Klammern, Punkt. Bewusst großzügig —
// wir wollen erreichbar sein, nicht normieren.
var telefonZeichen = regexp.MustCompile(`^[0-9+()/.\-\s]+$`)

// mailForm ist eine bewusst einfache Plausibilitätsprüfung: lokaler Teil,
// @, Domain mit mindestens einem Punkt. Ob die Adresse wirklich zustellbar
// ist, kann ohnehin nur eine Mail beantworten.
var mailForm = regexp.MustCompile(`^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$`)

// ProfileInput ist die Eingabe von PUT /api/v1/me/profile.
//
// PUT ersetzt den ganzen Datensatz: Ein Feld, das nicht mitkommt, wird
// geleert. Wer nur eine Kleinigkeit ändern will, schickt das Profil aus
// GET /api/v1/me vollständig zurück — App und Verwaltung tun genau das.
// Einzige Ausnahme ist visibility: Fehlt der Block ganz, bleiben die
// bisherigen Schalter stehen, statt auf die Vorbelegung zurückzufallen.
//
// UserSub ist optional und dient als Sicherung: Schickt eine App versehentlich
// (oder absichtlich) eine fremde Kennung mit, wird die Änderung abgelehnt,
// statt still das eigene Profil zu überschreiben.
type ProfileInput struct {
	UserSub     string                   `json:"userSub"`
	DisplayName string                   `json:"displayName"`
	Nickname    string                   `json:"nickname"`
	Phone       string                   `json:"phone"`
	Email       string                   `json:"email"`
	Note        string                   `json:"note"`
	Visibility  *model.ProfileVisibility `json:"visibility"`
}

// Validate prüft Längen, Formate und Steuerzeichen.
func (in *ProfileInput) Validate() error {
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	in.Nickname = strings.TrimSpace(in.Nickname)
	in.Phone = strings.TrimSpace(in.Phone)
	in.Email = strings.TrimSpace(in.Email)
	in.Note = strings.TrimSpace(in.Note)

	for _, feld := range []struct {
		name, wert string
		max        int
	}{
		{"displayName", in.DisplayName, MaxNameLen},
		{"nickname", in.Nickname, MaxNickLen},
		{"phone", in.Phone, MaxPhoneLen},
		{"email", in.Email, MaxMailLen},
		{"note", in.Note, MaxNoteLen},
	} {
		if utf8.RuneCountInString(feld.wert) > feld.max {
			return errors.New(feld.name + " ist zu lang (höchstens " + itoa(feld.max) + " Zeichen)")
		}
		if err := ohneSteuerzeichen(feld.name, feld.wert); err != nil {
			return err
		}
	}

	if in.Phone != "" {
		if !telefonZeichen.MatchString(in.Phone) {
			return errors.New("phone darf nur Ziffern und die üblichen Trennzeichen enthalten")
		}
		if ziffern(in.Phone) < MinPhoneDigits {
			return errors.New("phone sieht nicht nach einer Rufnummer aus")
		}
	}
	if in.Email != "" && !mailForm.MatchString(in.Email) {
		return errors.New("email ist keine gültige E-Mail-Adresse")
	}
	if in.Visibility != nil && !in.Visibility.Valid() {
		return errors.New("visibility muss je Feld dorf oder verwaltung sein")
	}
	return nil
}

// ohneSteuerzeichen weist Steuerzeichen ab — auch Zeilenumbrüche und
// Tabulatoren. Diese Texte stehen später einzeilig in Listen; ein
// eingeschmuggelter Umbruch (oder ein Nullbyte) hat dort nichts zu suchen.
func ohneSteuerzeichen(feld, wert string) error {
	for _, r := range wert {
		if unicode.IsControl(r) {
			return errors.New(feld + " darf keine Steuerzeichen enthalten")
		}
	}
	return nil
}

func ziffern(s string) int {
	n := 0
	for _, r := range s {
		if r >= '0' && r <= '9' {
			n++
		}
	}
	return n
}

// Apply überträgt die Eingabe auf ein bestehendes Profil.
func (in *ProfileInput) Apply(p *model.Profile) {
	p.DisplayName, p.Nickname = in.DisplayName, in.Nickname
	p.Phone, p.Email, p.Note = in.Phone, in.Email, in.Note
	if in.Visibility != nil {
		p.Visibility = *in.Visibility
	}
}

// ProfileFor liefert das Profil einer Person und legt es beim ersten Mal aus
// den Angaben der Rössing-ID an (Anzeigename und E-Mail vorbelegt,
// Kontaktdaten nicht veröffentlicht).
//
// Das Anlegen beim Lesen ist Absicht: So steht jede und jeder, die die App
// öffnet, in der Dorfbewohner-Liste („wer macht mit"), und die Rangliste
// findet den Namen aus der Rössing-ID wieder, um Bestandsdaten zuzuordnen.
// Sichtbar wird dadurch nichts, was nicht ohnehin schon in der Rangliste
// stünde.
func ProfileFor(d *db.DB, u auth.User, now time.Time) (*model.Profile, error) {
	p, err := d.GetProfile(u.Sub)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		neu := model.Profile{
			UserSub: u.Sub, DisplayName: u.Name, Email: u.Email,
			Visibility: model.DefaultVisibility(), TokenName: u.Name, UpdatedAt: now,
		}
		if err := d.UpsertProfile(&neu); err != nil {
			return nil, err
		}
		return &neu, nil
	}
	// Wer sich in der Rössing-ID umbenennt, soll seine alten Meldungen
	// behalten: Der Name aus dem Token wird deshalb mitgeführt.
	if u.Name != "" && p.TokenName != u.Name {
		p.TokenName = u.Name
		if err := d.UpsertProfile(p); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// --- Handler ------------------------------------------------------------------

func (s *Server) handlePutProfile(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	var in ProfileInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	// Nur das eigene Profil — auch Verwaltende ändern hier nichts Fremdes.
	if in.UserSub != "" && in.UserSub != u.Sub {
		writeErr(w, http.StatusForbidden, "es lässt sich nur das eigene Profil ändern")
		return
	}
	if err := in.Validate(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	p, err := ProfileFor(s.DB, u, s.now())
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	in.Apply(p)
	p.UpdatedAt = s.now()
	if err := s.DB.UpsertProfile(p); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleMembers(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.FromContext(r.Context())
	profile, err := s.DB.ListProfiles()
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	mitglieder := make([]model.Member, 0, len(profile))
	for _, p := range profile {
		if m, ok := p.AsMember(u.IsAdmin()); ok {
			mitglieder = append(mitglieder, m)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"members": mitglieder, "adminView": u.IsAdmin(),
	})
}
