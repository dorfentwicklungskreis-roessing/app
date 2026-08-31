package model

import "time"

// Profil einer Person im Dorf: der Datensatz, den jede und jeder selbst
// pflegt. Die Anmeldung liefert nur Kennung, Name und E-Mail aus der
// Rössing-ID; alles Weitere (Nickname für die Rangliste, Telefon, Notiz)
// steht hier — und zwar nur, soweit die Person es selbst einträgt.
//
// Jedes Feld trägt einen eigenen Sichtbarkeits-Schalter. Nichts wird still
// veröffentlicht: Kontaktdaten stehen in der Vorbelegung ausdrücklich auf
// „nur für Verwaltende“ und werden erst nach einer bewussten Entscheidung
// für die anderen Dorfbewohner sichtbar.

// Visibility beschreibt, wer ein Profilfeld sehen darf.
type Visibility string

const (
	// VisibilityVillage: alle angemeldeten Dorfbewohner.
	VisibilityVillage Visibility = "dorf"
	// VisibilityAdmins: nur Verwaltende.
	VisibilityAdmins Visibility = "verwaltung"
)

func ValidVisibility(v Visibility) bool {
	return v == VisibilityVillage || v == VisibilityAdmins
}

// ProfileVisibility bündelt die Schalter je Feld.
type ProfileVisibility struct {
	DisplayName Visibility `json:"displayName"`
	Nickname    Visibility `json:"nickname"`
	Phone       Visibility `json:"phone"`
	Email       Visibility `json:"email"`
	Note        Visibility `json:"note"`
}

// DefaultVisibility ist die Vorbelegung: Name und Nickname sind für das Dorf
// sichtbar (unter einem davon erscheint man ohnehin in der Rangliste),
// Telefon, E-Mail und Notiz bleiben bei der Verwaltung, bis die Person sie
// selbst freigibt.
func DefaultVisibility() ProfileVisibility {
	return ProfileVisibility{
		DisplayName: VisibilityVillage,
		Nickname:    VisibilityVillage,
		Phone:       VisibilityAdmins,
		Email:       VisibilityAdmins,
		Note:        VisibilityAdmins,
	}
}

// Valid prüft alle Schalter.
func (v ProfileVisibility) Valid() bool {
	return ValidVisibility(v.DisplayName) && ValidVisibility(v.Nickname) &&
		ValidVisibility(v.Phone) && ValidVisibility(v.Email) && ValidVisibility(v.Note)
}

// Profile ist der Profildatensatz einer Person (Schlüssel: Zitadel-Kennung).
type Profile struct {
	UserSub     string            `json:"userSub"`
	DisplayName string            `json:"displayName"`
	Nickname    string            `json:"nickname"`
	Phone       string            `json:"phone"`
	Email       string            `json:"email"`
	Note        string            `json:"note"`
	Visibility  ProfileVisibility `json:"visibility"`
	UpdatedAt   time.Time         `json:"updatedAt"`

	// TokenName ist der Name aus der Rössing-ID. Er wird bei jeder Anmeldung
	// mitgeführt und nie angezeigt: Er dient allein dazu, Bestandsdaten
	// zuzuordnen — Erledigungen tragen den Namen, der zum Zeitpunkt der
	// Meldung galt (siehe MatchesStoredName).
	TokenName string `json:"-"`
}

// EffectiveName ist der Name, unter dem die Person im Dorf auftritt:
// Nickname, sonst Anzeigename, sonst der Name aus der Rössing-ID — und wenn
// nichts davon da ist, der Spitzname (siehe AnonymousName).
//
// Der Spitzname steht ausdrücklich am Ende: Er ersetzt keinen Namen, er
// vertritt ihn nur dort, wo sonst eine Leerstelle stünde. Genau das war der
// Fall — die Rangliste zeigte Zeilen mit Punkten, Litern und Auszeichnungen,
// aber ohne Namen.
func (p Profile) EffectiveName() string {
	switch {
	case p.Nickname != "":
		return p.Nickname
	case p.DisplayName != "":
		return p.DisplayName
	case p.TokenName != "":
		return p.TokenName
	default:
		return AnonymousName(p.UserSub)
	}
}

// MatchesStoredName sagt, ob ein in einer Erledigung eingefrorener Name zu
// dieser Person gehört.
//
// Hintergrund: Erledigungen speichern seit jeher den Namen, der beim Melden
// galt. Meldungen, die ein Admin für jemand anderen einträgt (Vollzug per
// Telefon), laufen unter seiner Kennung, aber unter fremdem Namen — die
// dürfen nicht plötzlich seinen Nickname tragen. Deshalb wird der Profilname
// nur dort eingesetzt, wo der gespeicherte Name einer der Namen dieser
// Person ist.
func (p Profile) MatchesStoredName(gespeichert string) bool {
	if gespeichert == "" {
		return true
	}
	return gespeichert == p.TokenName ||
		(p.DisplayName != "" && gespeichert == p.DisplayName) ||
		(p.Nickname != "" && gespeichert == p.Nickname)
}

// NameResolver bildet Kennungen auf Profile ab und liefert den anzuzeigenden
// Namen einer Erledigung. Ohne Profil (Bestandsdaten, Leute, die die App nie
// geöffnet haben) bleibt es beim gespeicherten Namen.
type NameResolver map[string]Profile

func (r NameResolver) Resolve(userSub, gespeichert string) string {
	p, ok := r[userSub]
	if !ok {
		return gespeichert
	}
	if !p.MatchesStoredName(gespeichert) {
		return gespeichert
	}
	if name := p.EffectiveName(); name != "" {
		return name
	}
	return gespeichert
}

// --- Sicht anderer auf ein Profil --------------------------------------------

// Member ist die Sicht anderer Angemeldeter auf eine Person: nur die Felder,
// die freigegeben sind. Verwaltende sehen alles; Restricted nennt dann die
// Felder, die nur ihnen gezeigt werden.
type Member struct {
	UserSub string `json:"userSub"`
	// Name ist der Name, unter dem die Person in Rangliste und Historie steht.
	Name        string    `json:"name"`
	DisplayName string    `json:"displayName,omitempty"`
	Nickname    string    `json:"nickname,omitempty"`
	Phone       string    `json:"phone,omitempty"`
	Email       string    `json:"email,omitempty"`
	Note        string    `json:"note,omitempty"`
	Restricted  []string  `json:"restricted"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// AsMember blendet ein Profil für einen Betrachter aus.
//
// istVerwaltung=false: nur freigegebene Felder, Restricted bleibt leer.
// ok=false heißt: Diese Person möchte in der Liste der Dorfbewohner nicht
// erscheinen — sie hat weder Anzeigenamen noch Nickname freigegeben.
// Für Verwaltende ist ok immer true.
func (p Profile) AsMember(istVerwaltung bool) (Member, bool) {
	m := Member{UserSub: p.UserSub, UpdatedAt: p.UpdatedAt, Restricted: []string{}}
	sichtbar := func(feld string, v Visibility, wert string) string {
		if wert == "" {
			return ""
		}
		if v == VisibilityVillage {
			return wert
		}
		if !istVerwaltung {
			return ""
		}
		m.Restricted = append(m.Restricted, feld)
		return wert
	}
	m.DisplayName = sichtbar("displayName", p.Visibility.DisplayName, p.DisplayName)
	m.Nickname = sichtbar("nickname", p.Visibility.Nickname, p.Nickname)
	m.Phone = sichtbar("phone", p.Visibility.Phone, p.Phone)
	m.Email = sichtbar("email", p.Visibility.Email, p.Email)
	m.Note = sichtbar("note", p.Visibility.Note, p.Note)

	if istVerwaltung {
		m.Name = p.EffectiveName()
		return m, true
	}
	// Der angezeigte Name folgt derselben Regel wie in der Rangliste, nutzt
	// aber nur die freigegebenen Felder.
	//
	// Hier greift der Spitzname bewusst NICHT. In der Rangliste steht die
	// Person ohnehin schon, nur ohne Namen — dort füllt er eine Leerstelle.
	// Im Verzeichnis dagegen fehlt sie ganz, und zwar weil sie weder
	// Anzeigenamen noch Nickname freigegeben hat. Sie als „Lustiger Lurch“
	// aufzunehmen wäre keine Kosmetik, sondern ein neuer Eintrag: Er verriete,
	// dass es dieses Konto gibt, und machte aus einer Entscheidung gegen die
	// Liste eine Aufnahme in sie. Ein Eintrag ohne Namen und ohne Kontaktdaten
	// nützt dem Verzeichnis („wer macht mit, wie erreiche ich sie“) auch
	// nichts.
	switch {
	case m.Nickname != "":
		m.Name = m.Nickname
	case m.DisplayName != "":
		m.Name = m.DisplayName
	default:
		return Member{}, false
	}
	return m, true
}
