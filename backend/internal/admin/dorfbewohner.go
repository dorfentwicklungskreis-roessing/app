package admin

import (
	"net/http"
	"strings"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/api"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Bereich „Dorfbewohner“: das eigene Profil und die Liste aller, die
// mitmachen. Beides server-gerendert mit echter Seitennavigation.
const bewohnerBasis = "/admin/dorfbewohner"

func (a *App) registerDorfbewohner(mux *http.ServeMux) {
	mux.HandleFunc("GET "+bewohnerBasis+"/{$}", a.requireAdmin(a.mitgliederListe))
	mux.HandleFunc("GET "+bewohnerBasis+"/profil", a.requireAdmin(a.profilFormular))
	mux.HandleFunc("POST "+bewohnerBasis+"/profil", a.requireAdmin(a.profilSpeichern))
}

// --- Eigenes Profil -----------------------------------------------------------

type profilDaten struct {
	Profil model.Profile
	Fehler string
	// Felder beschreibt die Formularfelder samt Sichtbarkeits-Schalter, damit
	// die Vorlage nicht fünfmal dasselbe wiederholt.
	Felder []profilFeld
}

// profilFeld ist ein Eingabefeld mit seinem Sichtbarkeits-Schalter.
type profilFeld struct {
	// Name ist der Formularname ("anzeigename"), ID der HTML-Anker
	// ("feld-anzeigename"), Sicht der Name des Schalters ("sicht_anzeigename").
	Name    string
	ID      string
	Sicht   string
	Titel   string
	Hinweis string
	// Art ist der HTML-Eingabetyp (text, tel, email).
	Art  string
	Wert string
	// Oeffentlich: Schalter steht auf „für alle Dorfbewohner“.
	Oeffentlich bool
}

func felderAus(p model.Profile) []profilFeld {
	oeffentlich := func(v model.Visibility) bool { return v == model.VisibilityVillage }
	return []profilFeld{
		{"anzeigename", "feld-anzeigename", "sicht_anzeigename", "Anzeigename",
			"Dein Name, wie ihn andere im Dorf lesen. Vorbelegt aus der Rössing-ID.",
			"text", p.DisplayName, oeffentlich(p.Visibility.DisplayName)},
		{"nickname", "feld-nickname", "sicht_nickname", "Nickname für die Rangliste",
			"Steht statt des Anzeigenamens in Rangliste und Erledigungen. Leer lassen = Anzeigename.",
			"text", p.Nickname, oeffentlich(p.Visibility.Nickname)},
		{"telefon", "feld-telefon", "sicht_telefon", "Telefon (freiwillig)",
			"Nur ausfüllen, wenn du erreichbar sein möchtest.",
			"tel", p.Phone, oeffentlich(p.Visibility.Phone)},
		{"email", "feld-email", "sicht_email", "E-Mail (freiwillig)",
			"Vorbelegt aus der Rössing-ID, überschreibbar.",
			"email", p.Email, oeffentlich(p.Visibility.Email)},
		{"notiz", "feld-notiz", "sicht_notiz", "Notiz (freiwillig)",
			"Ein kurzer Hinweis, z.B. „erreichbar abends“.",
			"text", p.Note, oeffentlich(p.Visibility.Note)},
	}
}

func (a *App) profilFormular(w http.ResponseWriter, r *http.Request, s session) {
	p, err := api.ProfileFor(a.db, nutzerAus(s), a.now())
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.zeigeProfil(w, r, http.StatusOK, *p, "")
}

func (a *App) zeigeProfil(w http.ResponseWriter, r *http.Request, status int, p model.Profile, fehler string) {
	a.render(w, r, status, "profil", view{
		Title: "Mein Profil", Nav: "dorfbewohner",
		Data: profilDaten{Profil: p, Fehler: fehler, Felder: felderAus(p)},
	})
}

func (a *App) profilSpeichern(w http.ResponseWriter, r *http.Request, s session) {
	if err := r.ParseForm(); err != nil {
		a.fail(w, r, http.StatusBadRequest, err)
		return
	}
	in := api.ProfileInput{
		DisplayName: strings.TrimSpace(r.FormValue("anzeigename")),
		Nickname:    strings.TrimSpace(r.FormValue("nickname")),
		Phone:       strings.TrimSpace(r.FormValue("telefon")),
		Email:       strings.TrimSpace(r.FormValue("email")),
		Note:        strings.TrimSpace(r.FormValue("notiz")),
	}
	// Ein nicht gesetzter Haken heißt „nur Verwaltende“ — ein fehlendes Feld
	// darf nie zu mehr Sichtbarkeit führen.
	sicht := model.ProfileVisibility{
		DisplayName: schalter(r, "sicht_anzeigename"),
		Nickname:    schalter(r, "sicht_nickname"),
		Phone:       schalter(r, "sicht_telefon"),
		Email:       schalter(r, "sicht_email"),
		Note:        schalter(r, "sicht_notiz"),
	}
	in.Visibility = &sicht

	if err := in.Validate(); err != nil {
		entwurf := model.Profile{
			UserSub: s.Sub, DisplayName: in.DisplayName, Nickname: in.Nickname,
			Phone: in.Phone, Email: in.Email, Note: in.Note, Visibility: sicht,
		}
		a.zeigeProfil(w, r, http.StatusBadRequest, entwurf, err.Error())
		return
	}

	p, err := api.ProfileFor(a.db, nutzerAus(s), a.now())
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	in.Apply(p)
	p.UpdatedAt = a.now()
	if err := a.db.UpsertProfile(p); err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	a.setFlash(w, "success", "Dein Profil wurde gespeichert.")
	http.Redirect(w, r, bewohnerBasis+"/profil", http.StatusSeeOther)
}

// schalter liest einen Sichtbarkeits-Haken. Alles außer einem ausdrücklichen
// „dorf“ bedeutet: bleibt bei der Verwaltung.
func schalter(r *http.Request, name string) model.Visibility {
	if r.FormValue(name) == string(model.VisibilityVillage) {
		return model.VisibilityVillage
	}
	return model.VisibilityAdmins
}

// nutzerAus baut aus der Session den Nutzer, wie ihn die API-Schicht erwartet.
func nutzerAus(s session) auth.User {
	return auth.User{Sub: s.Sub, Name: anzeigeName(s), Email: s.Email,
		Roles: map[string]bool{"admin": s.Admin}}
}

// --- Mitgliederliste ----------------------------------------------------------

type mitgliederDaten struct {
	Mitglieder []mitgliedZeile
}

// mitgliedZeile ergänzt die Mitgliedersicht um die Kennzeichnung, welche
// Angaben nur die Verwaltung sieht.
type mitgliedZeile struct {
	model.Member
	NurVerwaltung map[string]bool
}

func (a *App) mitgliederListe(w http.ResponseWriter, r *http.Request, _ session) {
	profile, err := a.db.ListProfiles()
	if err != nil {
		a.fail(w, r, http.StatusInternalServerError, err)
		return
	}
	zeilen := make([]mitgliedZeile, 0, len(profile))
	for _, p := range profile {
		// Die Verwaltung sieht alles — gekennzeichnet.
		m, ok := p.AsMember(true)
		if !ok {
			continue
		}
		nur := map[string]bool{}
		for _, feld := range m.Restricted {
			nur[feld] = true
		}
		zeilen = append(zeilen, mitgliedZeile{Member: m, NurVerwaltung: nur})
	}
	a.render(w, r, http.StatusOK, "mitglieder", view{
		Title: "Dorfbewohner", Nav: "dorfbewohner",
		Data: mitgliederDaten{Mitglieder: zeilen},
	})
}
