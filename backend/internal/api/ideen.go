package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/httpx"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Ideen-Sammlung: „Sag uns, was die App können soll.“
//
// Der Eingang (POST /api/v1/ideen) ist bewusst OHNE Anmeldung erreichbar —
// die Website ist öffentlich, und wer noch keine App hat, soll trotzdem
// etwas sagen können. Alles Weitere (lesen, einordnen, löschen) darf nur
// die Verwaltung.
//
// Weil der Eingang öffentlich ist, hat er einen eigenen Missbrauchsschutz:
//
//  1. eine eigene, deutlich strengere Zugriffsgrenze je Aufrufer (IP),
//  2. ein verstecktes Feld („Honigtopf“), das kein Mensch ausfüllt,
//  3. eine Mindestzeit zwischen Formularaufruf und Absenden.
//
// Kein Captcha, kein Fremddienst: 2 und 3 kosten echte Menschen nichts und
// halten die üblichen Formular-Skripte zuverlässig auf.

const (
	// MinWunschLen/MaxWunschLen begrenzen den eigentlichen Wunsch.
	MinWunschLen = 5
	MaxWunschLen = 2000
	// MaxIdeeNameLen und MaxIdeeEmailLen begrenzen die freiwilligen Angaben.
	MaxIdeeNameLen  = 100
	MaxIdeeEmailLen = 200
	// MaxIdeeNotizLen begrenzt die interne Bemerkung der Verwaltung.
	MaxIdeeNotizLen = 2000
	// IdeenMindestdauer ist die kürzeste Zeit zwischen Formularaufruf und
	// Absenden, die noch als „von Hand getippt“ durchgeht.
	IdeenMindestdauer = 3 * time.Second
	// IdeenHoechstdauer: Wer das Formular länger offen hatte, wird nicht mehr
	// gegen die Mindestzeit geprüft (der Zeitstempel ist dann bedeutungslos).
	IdeenHoechstdauer = 24 * time.Hour
	// IdeenDankePfad ist die Dankeseite der Website.
	IdeenDankePfad = "/app/danke"
	// IdeenFormularPfad führt zurück zum Formular auf der Website.
	IdeenFormularPfad = "/app#ideen"
)

// IdeenStandardZiele sind die Ursprünge, auf die nach dem Absenden
// weitergeleitet werden darf. Alles andere wird abgewiesen — eine offene
// Weiterleitung wäre ein Geschenk an jeden Phishing-Versuch.
var IdeenStandardZiele = []string{
	"https://xn--rssing-wxa.de",
	"https://www.xn--rssing-wxa.de",
}

// Vorgaben der Zugriffsgrenze am öffentlichen Eingang: fünf Einreichungen am
// Stück, danach fünf pro Stunde nach. Ein Mensch schreibt nicht mehr, ein
// Skript ist damit sofort ausgebremst.
const (
	IdeenBurstStandard     = 5
	IdeenProStundeStandard = 5
)

// --- Registrierung ------------------------------------------------------------

// registerIdeenVerwaltung hängt die geschützten Routen an den Router der
// angemeldeten API.
func (s *Server) registerIdeenVerwaltung(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/ideen", s.adminOnly(s.handleListIdeen))
	mux.HandleFunc("PATCH /api/v1/ideen/{id}", s.adminOnly(s.handlePatchIdee))
	mux.HandleFunc("DELETE /api/v1/ideen/{id}", s.adminOnly(s.handleDeleteIdee))
}

// registerIdeenEingang hängt den öffentlichen Eingang direkt an den äußeren
// Router — vorbei an der Anmeldepflicht, aber mit eigener Zugriffsgrenze.
// Ist ein gültiges Token dabei (App), wird die Einreichung dem Konto
// zugeordnet; ohne Token geht es anonym von der Website ein.
func (s *Server) registerIdeenEingang(mux *http.ServeMux) {
	var h http.Handler = http.HandlerFunc(s.handleCreateIdee)
	if s.OptionalAuth != nil {
		h = s.OptionalAuth(h)
	}
	mux.Handle("POST /api/v1/ideen", h)
}

// grenzeFuerIdeen liefert die Zugriffsgrenze des öffentlichen Eingangs. Sie
// wird erst beim ersten Zugriff gebaut, damit sie sich vorher noch setzen
// lässt. Ein nil-Limiter reicht alles durch (siehe httpx.RateLimiter).
func (s *Server) grenzeFuerIdeen() *httpx.RateLimiter {
	s.ideenEinmal.Do(func() {
		if s.IdeenLimiter == nil {
			s.IdeenLimiter = httpx.NewRateLimiter(IdeenRateLimitFromEnv())
		}
	})
	return s.IdeenLimiter
}

// IdeenRateLimitFromEnv liest die Grenze des Ideen-Eingangs:
//
//	IDEEN_LIMIT=off       schaltet sie ab (Tests, Notfall)
//	IDEEN_BURST           Eimergröße (Vorgabe 5)
//	IDEEN_PRO_STUNDE      Nachfüllrate pro Stunde (Vorgabe 5)
//
// RATE_LIMIT=off schaltet auch diese Grenze ab, damit es in Entwicklung und
// E2E bei einem Schalter bleibt.
func IdeenRateLimitFromEnv() httpx.RateLimitConfig {
	an := !istAus(os.Getenv("IDEEN_LIMIT")) && !istAus(os.Getenv("RATE_LIMIT"))
	return httpx.RateLimitConfig{
		Burst:   umgebungZahl("IDEEN_BURST", IdeenBurstStandard),
		PerHour: umgebungZahl("IDEEN_PRO_STUNDE", IdeenProStundeStandard),
		Enabled: &an,
	}
}

func istAus(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "off", "0", "false", "aus", "nein":
		return true
	}
	return false
}

func umgebungZahl(key string, vorgabe int) int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key)))
	if err != nil || n <= 0 {
		return vorgabe
	}
	return n
}

// --- Eingabe und Prüfung ------------------------------------------------------

// IdeeEingabe ist der Datensatz des Formulars — als JSON (App) oder als
// klassisches HTML-Formular (Website, funktioniert ohne JavaScript).
type IdeeEingabe struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Wunsch string `json:"wunsch"`
	// Honigtopf ist ein Feld, das im Formular versteckt ist. Menschen sehen
	// es nicht und füllen es nie aus; Skripte füllen stumpf alles aus.
	Honigtopf string `json:"webseite"`
	// Gestartet ist der Zeitpunkt des Formularaufrufs in Unix-Millisekunden.
	// Fehlt er (kein JavaScript), wird die Mindestzeit nicht geprüft.
	Gestartet string `json:"gestartet"`
	// Redirect ist das Ziel nach dem Absenden (nur erlaubte Ursprünge).
	Redirect string `json:"redirect"`
}

// Saeubern schneidet Leerraum ab. Der Wunsch behält seine Zeilenumbrüche —
// Menschen gliedern ihren Text.
func (in *IdeeEingabe) Saeubern() {
	in.Name = strings.TrimSpace(in.Name)
	in.Email = strings.TrimSpace(in.Email)
	in.Wunsch = strings.TrimSpace(in.Wunsch)
	in.Honigtopf = strings.TrimSpace(in.Honigtopf)
	in.Gestartet = strings.TrimSpace(in.Gestartet)
	in.Redirect = strings.TrimSpace(in.Redirect)
}

// Validate prüft die Eingabe. Die Meldungen sind so formuliert, dass sie
// unverändert auf der Fehlerseite stehen können.
func (in *IdeeEingabe) Validate() error {
	if in.Wunsch == "" {
		return errors.New("Bitte schreib auf, was die App können soll.")
	}
	if n := utf8.RuneCountInString(in.Wunsch); n < MinWunschLen {
		return errors.New("Der Wunsch ist zu kurz — bitte mindestens " +
			itoa(MinWunschLen) + " Zeichen.")
	} else if n > MaxWunschLen {
		return errors.New("Der Wunsch ist zu lang — höchstens " +
			itoa(MaxWunschLen) + " Zeichen.")
	}
	if enthaeltSteuerzeichen(in.Wunsch, true) {
		return errors.New("Der Wunsch enthält Zeichen, die hier nicht hingehören.")
	}
	if utf8.RuneCountInString(in.Name) > MaxIdeeNameLen {
		return errors.New("Der Name ist zu lang — höchstens " + itoa(MaxIdeeNameLen) + " Zeichen.")
	}
	if enthaeltSteuerzeichen(in.Name, false) {
		return errors.New("Der Name enthält Zeichen, die hier nicht hingehören.")
	}
	if in.Email != "" {
		if utf8.RuneCountInString(in.Email) > MaxIdeeEmailLen {
			return errors.New("Die E-Mail-Adresse ist zu lang — höchstens " +
				itoa(MaxIdeeEmailLen) + " Zeichen.")
		}
		if enthaeltSteuerzeichen(in.Email, false) || !plausibleEmail(in.Email) {
			return errors.New("Die E-Mail-Adresse sieht nicht richtig aus.")
		}
	}
	return nil
}

// enthaeltSteuerzeichen findet Steuerzeichen im Text. In mehrzeiligen Feldern
// sind Zeilenumbruch und Tabulator erlaubt, sonst nichts.
func enthaeltSteuerzeichen(s string, mehrzeilig bool) bool {
	for _, r := range s {
		if !unicode.IsControl(r) {
			continue
		}
		if mehrzeilig && (r == '\n' || r == '\r' || r == '\t') {
			continue
		}
		return true
	}
	return false
}

// plausibleEmail prüft grob, ob die Adresse überhaupt eine sein kann.
// Bewusst keine RFC-Zerlegung: Wir stellen nichts zu, wir merken uns nur eine
// Rückfrage-Adresse — und wollen Tippfehler früh melden.
func plausibleEmail(s string) bool {
	if strings.ContainsAny(s, " \t\r\n<>,;\"") {
		return false
	}
	teile := strings.Split(s, "@")
	if len(teile) != 2 || teile[0] == "" || teile[1] == "" {
		return false
	}
	labels := strings.Split(teile[1], ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if l == "" {
			return false
		}
	}
	// Die Endung muss aus Buchstaben bestehen und mindestens zwei lang sein.
	endung := labels[len(labels)-1]
	if utf8.RuneCountInString(endung) < 2 {
		return false
	}
	for _, r := range endung {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

// --- Öffentlicher Eingang ------------------------------------------------------

func (s *Server) handleCreateIdee(w http.ResponseWriter, r *http.Request) {
	in, err := ideeEingabeLesen(r)
	if err != nil {
		s.ideeFehler(w, r, IdeeEingabe{}, "Die Eingabe war nicht lesbar.")
		return
	}
	in.Saeubern()

	// Die Zugriffsgrenze wird erst hier geprüft — nach dem Lesen der
	// Eingabe. Nur so kann die Abweisung den getippten Text zurückgeben,
	// statt ihn in einer nackten Fehlermeldung verschwinden zu lassen.
	// Dass dafür der Rumpf schon gelesen ist, kostet nichts: Er ist
	// ohnehin auf MAX_BODY_BYTES gedeckelt, und die allgemeine Grenze
	// (RATE_LIMIT) sitzt als Middleware weiterhin davor.
	if ok, warten := s.grenzeFuerIdeen().Zulassen(r); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(warten.Seconds()))))
		s.ideeAbgewiesen(w, r, in, http.StatusTooManyRequests,
			"Von hier kamen gerade viele Ideen auf einmal. Bitte in einer Stunde "+
				"noch einmal abschicken — dein Text steht unten noch.")
		return
	}

	// Erst das Ziel prüfen: Auf eine fremde Seite wird nie weitergeleitet,
	// auch nicht im Erfolgsfall.
	ziel, err := s.ideeZiel(in.Redirect)
	if err != nil {
		slog.Warn("Ideen-Eingang: unerlaubtes Weiterleitungsziel abgewiesen", "ziel", in.Redirect)
		in.Redirect = ""
		s.ideeFehler(w, r, in, "Das angegebene Weiterleitungsziel ist nicht erlaubt.")
		return
	}

	// Verworfen wird still: Wer hier hängen bleibt, ist ein Skript und soll
	// aus der freundlichen Antwort nichts lernen.
	if in.Honigtopf != "" || s.zuSchnellAbgeschickt(in.Gestartet) {
		slog.Info("Ideen-Eingang: Einreichung verworfen (Missbrauchsschutz)")
		s.ideeErfolg(w, r, ziel, nil)
		return
	}

	if err := in.Validate(); err != nil {
		s.ideeFehler(w, r, in, err.Error())
		return
	}

	idee := model.Idee{
		Name: in.Name, Email: in.Email, Wunsch: in.Wunsch,
		Quelle: model.IdeeQuelleWebsite, Status: model.IdeeNeu, CreatedAt: s.now(),
	}
	// Wer angemeldet einreicht (App), bekommt die Einreichung dem Konto
	// zugeordnet. Die Quelle wird daraus abgeleitet und nicht vom Aufrufer
	// bestimmt — sonst ließe sie sich schlicht behaupten.
	if u, ok := auth.FromContext(r.Context()); ok && u.Sub != "" {
		idee.UserSub = u.Sub
		idee.Quelle = model.IdeeQuelleApp
		if idee.Name == "" {
			idee.Name = u.Name
		}
		if idee.Email == "" {
			idee.Email = u.Email
		}
	}
	if err := s.DB.InsertIdee(&idee); err != nil {
		writeInternal(w, r, err)
		return
	}
	slog.Info("Ideen-Eingang: neue Idee", "id", idee.ID, "quelle", idee.Quelle)
	s.ideeErfolg(w, r, ziel, &idee)
}

// ideeEingabeLesen versteht JSON und klassische Formulare gleichermaßen.
func ideeEingabeLesen(r *http.Request) (IdeeEingabe, error) {
	var in IdeeEingabe
	// Nur ein ausdrücklich als Formular gekennzeichneter Rumpf wird als
	// Formular gelesen; alles andere ist JSON. So kommen auch Aufrufer
	// durch, die den Content-Type weglassen.
	typ := strings.ToLower(r.Header.Get("Content-Type"))
	istFormular := strings.HasPrefix(typ, "application/x-www-form-urlencoded") ||
		strings.HasPrefix(typ, "multipart/form-data")
	if !istFormular {
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			return in, err
		}
		return in, nil
	}
	if err := r.ParseForm(); err != nil {
		return in, err
	}
	in.Name = r.PostFormValue("name")
	in.Email = r.PostFormValue("email")
	in.Wunsch = r.PostFormValue("wunsch")
	in.Honigtopf = r.PostFormValue("webseite")
	in.Gestartet = r.PostFormValue("gestartet")
	in.Redirect = r.PostFormValue("redirect")
	return in, nil
}

// zuSchnellAbgeschickt prüft die Mindestzeit zwischen Formularaufruf und
// Absenden. Ohne Zeitstempel wird nicht geprüft — das Formular muss ohne
// JavaScript funktionieren.
func (s *Server) zuSchnellAbgeschickt(roh string) bool {
	if roh == "" {
		return false
	}
	ms, err := strconv.ParseInt(roh, 10, 64)
	if err != nil {
		return false
	}
	gebraucht := s.now().Sub(time.UnixMilli(ms))
	if gebraucht > IdeenHoechstdauer {
		return false
	}
	return gebraucht < IdeenMindestdauer
}

// ideeZiel prüft das gewünschte Weiterleitungsziel. Leer heißt „keins“.
// Erlaubt sind ausschließlich vollständige URLs auf einem der freigegebenen
// Ursprünge — keine relativen Pfade, keine Benutzerangabe im Host, kein
// anderes Schema.
func (s *Server) ideeZiel(roh string) (string, error) {
	if roh == "" {
		return "", nil
	}
	u, err := url.Parse(roh)
	if err != nil || u.Scheme == "" || u.Host == "" || u.User != nil || u.Opaque != "" {
		return "", errors.New("unerlaubtes Weiterleitungsziel")
	}
	ursprung := strings.ToLower(u.Scheme + "://" + u.Host)
	for _, erlaubt := range s.ideenZiele() {
		if ursprung == erlaubt {
			return u.String(), nil
		}
	}
	return "", errors.New("unerlaubtes Weiterleitungsziel")
}

// ideenZiele liefert die freigegebenen Ursprünge in Vergleichsform.
func (s *Server) ideenZiele() []string {
	roh := s.IdeenRedirects
	if len(roh) == 0 {
		roh = ideenZieleAusEnv()
	}
	out := make([]string, 0, len(roh))
	for _, e := range roh {
		if u, err := url.Parse(strings.TrimSpace(e)); err == nil && u.Scheme != "" && u.Host != "" {
			out = append(out, strings.ToLower(u.Scheme+"://"+u.Host))
		}
	}
	return out
}

// ideenZieleAusEnv liest IDEEN_ZIELE (kommasepariert). Leer = Vorgabe.
func ideenZieleAusEnv() []string {
	roh := strings.TrimSpace(os.Getenv("IDEEN_ZIELE"))
	if roh == "" {
		return IdeenStandardZiele
	}
	var out []string
	for _, teil := range strings.Split(roh, ",") {
		if teil = strings.TrimSpace(teil); teil != "" {
			out = append(out, teil)
		}
	}
	if len(out) == 0 {
		return IdeenStandardZiele
	}
	return out
}

// dankeseite ist das Ziel, auf dem ein Browser ohne eigenes Wunschziel landet.
func (s *Server) dankeseite() string {
	ziele := s.ideenZiele()
	if len(ziele) == 0 {
		return ""
	}
	return ziele[0] + IdeenDankePfad
}

// willHTML erkennt einen Browser, der ein Formular abgeschickt hat.
func willHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// ideeErfolg beantwortet eine angenommene (oder still verworfene) Einreichung.
// Browser bekommen eine Weiterleitung auf die Dankeseite der Website, alle
// anderen eine 201 mit JSON.
func (s *Server) ideeErfolg(w http.ResponseWriter, r *http.Request, ziel string, idee *model.Idee) {
	if ziel == "" && willHTML(r) {
		ziel = s.dankeseite()
	}
	if ziel != "" {
		http.Redirect(w, r, ziel, http.StatusSeeOther)
		return
	}
	if idee == nil {
		writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
		return
	}
	// Die interne Notiz geht nie nach außen.
	antwort := *idee
	antwort.Notiz = ""
	writeJSON(w, http.StatusCreated, antwort)
}

// ideeFehler meldet eine abgewiesene Einreichung. Browser bekommen eine
// verständliche Seite, auf der ihr Text noch steht — ohne JavaScript ginge
// er beim Zurückgehen sonst verloren.
func (s *Server) ideeFehler(w http.ResponseWriter, r *http.Request, in IdeeEingabe, meldung string) {
	s.ideeAbgewiesen(w, r, in, http.StatusBadRequest, meldung)
}

// ideeAbgewiesen ist der gemeinsame Weg für alles, was nicht angenommen
// wird. Browser bekommen die Seite mit ihrem Text, alle anderen JSON.
func (s *Server) ideeAbgewiesen(w http.ResponseWriter, r *http.Request, in IdeeEingabe, status int, meldung string) {
	if willHTML(r) {
		s.ideeFehlerSeite(w, in, status, meldung)
		return
	}
	writeErr(w, status, meldung)
}

// --- Verwaltung ---------------------------------------------------------------

func (s *Server) handleListIdeen(w http.ResponseWriter, r *http.Request) {
	status, err := ideeStatusAus(r.URL.Query().Get("status"))
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	ideen, err := s.DB.ListIdeen(status)
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	anzahl, err := s.DB.CountIdeen()
	if err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ideen": ideen, "anzahl": anzahl})
}

func (s *Server) handlePatchIdee(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	idee, err := s.DB.GetIdee(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Idee nicht gefunden")
		return
	}
	var in struct {
		Status *string `json:"status"`
		Notiz  *string `json:"notiz"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeErr(w, http.StatusBadRequest, "ungültiges JSON")
		return
	}
	if in.Status == nil && in.Notiz == nil {
		writeErr(w, http.StatusBadRequest, "es wurde nichts zum Ändern geschickt")
		return
	}
	if in.Status != nil {
		st, err := ideeStatusAus(*in.Status)
		if err != nil || st == "" {
			writeErr(w, http.StatusBadRequest, ideeStatusFehler())
			return
		}
		idee.Status = st
	}
	if in.Notiz != nil {
		notiz := strings.TrimSpace(*in.Notiz)
		if utf8.RuneCountInString(notiz) > MaxIdeeNotizLen {
			writeErr(w, http.StatusBadRequest, "Die Notiz ist zu lang.")
			return
		}
		if enthaeltSteuerzeichen(notiz, true) {
			writeErr(w, http.StatusBadRequest, "Die Notiz enthält Zeichen, die hier nicht hingehören.")
			return
		}
		idee.Notiz = notiz
	}
	if err := s.DB.UpdateIdee(idee); err != nil {
		writeInternal(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, idee)
}

func (s *Server) handleDeleteIdee(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "ungültige ID")
		return
	}
	if err := s.DB.DeleteIdee(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, "Idee nicht gefunden")
			return
		}
		writeInternal(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// IdeeStatusAus liest einen Stand aus einer Zeichenkette. Leer = „alle“.
func IdeeStatusAus(roh string) (model.IdeeStatus, error) { return ideeStatusAus(roh) }

func ideeStatusAus(roh string) (model.IdeeStatus, error) {
	roh = strings.TrimSpace(strings.ToLower(roh))
	if roh == "" || roh == "alle" {
		return "", nil
	}
	st := model.IdeeStatus(roh)
	if !model.ValidIdeeStatus(st) {
		return "", errors.New(ideeStatusFehler())
	}
	return st, nil
}

func ideeStatusFehler() string {
	namen := make([]string, 0, len(model.IdeeStatusWerte))
	for _, s := range model.IdeeStatusWerte {
		namen = append(namen, string(s))
	}
	return "status muss " + strings.Join(namen, ", ") + " sein"
}
