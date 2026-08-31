package mitglied

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/auth"
	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Das Zurückschreiben ist der Punkt, an dem ein Beitrittsverfahren steht oder
// fällt: Ohne es sagt die App „Mitglied“ und die Rössing-ID weiß nichts
// davon. Geprüft wird hier gegen einen nachgebauten Zitadel-Endpunkt — gegen
// ein echtes Zitadel läuft dasselbe im E2E (backend/e2e).

// aufnahmeServer spielt die Endpunkte, die eine Aufnahme braucht: Token,
// Rollen-Suche, Zuweisung anlegen, ändern, wieder in Betrieb nehmen.
type aufnahmeServer struct {
	*httptest.Server
	mu sync.Mutex
	// grants sind die Zuweisungen je Person.
	grants map[string][]zuweisung
	// verboten: Der Dienst-Nutzer darf nicht schreiben (fehlendes Recht).
	verboten bool
	// protokoll hält fest, was wirklich geschrieben wurde.
	protokoll []string
}

func neuerAufnahmeServer(t *testing.T) *aufnahmeServer {
	t.Helper()
	as := &aufnahmeServer{grants: map[string][]zuweisung{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/v2/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "dienst-token", "expires_in": 3600})
	})
	mux.HandleFunc("POST /management/v1/users/grants/_search", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Queries []struct {
				UserIDQuery struct {
					UserID string `json:"userId"`
				} `json:"userIdQuery"`
			} `json:"queries"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		sub := ""
		if len(in.Queries) > 0 {
			sub = in.Queries[0].UserIDQuery.UserID
		}
		as.mu.Lock()
		defer as.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"result": as.grants[sub]})
	})
	mux.HandleFunc("POST /management/v1/users/{sub}/grants", func(w http.ResponseWriter, r *http.Request) {
		if as.abweisen(w) {
			return
		}
		var in struct {
			ProjectID string   `json:"projectId"`
			RoleKeys  []string `json:"roleKeys"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		sub := r.PathValue("sub")
		as.mu.Lock()
		defer as.mu.Unlock()
		as.grants[sub] = append(as.grants[sub],
			zuweisung{ID: "g1", ProjectID: in.ProjectID, RoleKeys: in.RoleKeys})
		as.protokoll = append(as.protokoll, "anlegen "+sub+" "+in.ProjectID+" "+strings.Join(in.RoleKeys, ","))
		_ = json.NewEncoder(w).Encode(map[string]any{"userGrantId": "g1"})
	})
	mux.HandleFunc("PUT /management/v1/users/{sub}/grants/{gid}", func(w http.ResponseWriter, r *http.Request) {
		if as.abweisen(w) {
			return
		}
		var in struct {
			RoleKeys []string `json:"roleKeys"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		sub, gid := r.PathValue("sub"), r.PathValue("gid")
		as.mu.Lock()
		defer as.mu.Unlock()
		for i, g := range as.grants[sub] {
			if g.ID == gid {
				as.grants[sub][i].RoleKeys = in.RoleKeys
			}
		}
		as.protokoll = append(as.protokoll, "aendern "+sub+" "+gid+" "+strings.Join(in.RoleKeys, ","))
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	mux.HandleFunc("POST /management/v1/users/{sub}/grants/{gid}/_reactivate", func(w http.ResponseWriter, r *http.Request) {
		if as.abweisen(w) {
			return
		}
		sub, gid := r.PathValue("sub"), r.PathValue("gid")
		as.mu.Lock()
		defer as.mu.Unlock()
		for i, g := range as.grants[sub] {
			if g.ID == gid {
				as.grants[sub][i].State = "USER_GRANT_STATE_ACTIVE"
			}
		}
		as.protokoll = append(as.protokoll, "beleben "+sub+" "+gid)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	as.Server = httptest.NewServer(mux)
	t.Cleanup(as.Close)
	return as
}

func (as *aufnahmeServer) abweisen(w http.ResponseWriter) bool {
	as.mu.Lock()
	defer as.mu.Unlock()
	if !as.verboten {
		return false
	}
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"message":"No matching permissions found"}`))
	return true
}

func (as *aufnahmeServer) setzen(sub string, g ...zuweisung) {
	as.mu.Lock()
	defer as.mu.Unlock()
	as.grants[sub] = g
}

func (as *aufnahmeServer) geschrieben() []string {
	as.mu.Lock()
	defer as.mu.Unlock()
	return append([]string{}, as.protokoll...)
}

func aufnahmeZitadel(t *testing.T, as *aufnahmeServer, jetzt func() time.Time) *Zitadel {
	t.Helper()
	z, err := New(Config{Issuer: as.URL, SchluesselDatei: schluesselDatei(t),
		TTL: 30 * time.Second, Now: jetzt})
	if err != nil {
		t.Fatal(err)
	}
	return z
}

// Der Normalfall: Jemand hatte mit diesem Verein noch nie zu tun. Danach ist
// er Mitglied — und das gilt sofort, ohne Ab- und Anmelden.
func TestAufnahmeLegtEineZuweisungAn(t *testing.T) {
	as := neuerAufnahmeServer(t)
	jetzt := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	z := aufnahmeZitadel(t, as, func() time.Time { return jetzt })

	// Vorher gefragt und gemerkt: keine Mitgliedschaft.
	if z.Fuer(context.Background(), auth.User{Sub: "erna"}).Rollen.IstMitglied("377") {
		t.Fatal("Vorbedingung: Erna ist noch nicht dabei")
	}

	if err := z.Aufnehmen(context.Background(), "377", "erna", model.RolleMitglied); err != nil {
		t.Fatalf("Aufnahme: %v", err)
	}
	if got := as.geschrieben(); len(got) != 1 || !strings.HasPrefix(got[0], "anlegen erna 377 mitglied") {
		t.Fatalf("in Zitadel wurde nicht das Richtige geschrieben: %v", got)
	}
	// Der gemerkte Stand darf die frische Mitgliedschaft nicht verdecken.
	if !z.Fuer(context.Background(), auth.User{Sub: "erna"}).Rollen.IstMitglied("377") {
		t.Error("die Aufnahme wirkt nicht sofort — der Zwischenspeicher hält den alten Stand fest")
	}
}

// Wer im Verein schon eine Rolle hat, verliert sie nicht: Aus dem Vorstand
// wird kein bloßes Mitglied, nur weil jemand „aufnehmen“ drückt.
func TestAufnahmeErgaenztDieRolleUndNimmtKeineWeg(t *testing.T) {
	as := neuerAufnahmeServer(t)
	as.setzen("vorstand", zuweisung{ID: "g7", ProjectID: "377", RoleKeys: []string{"admin"}})
	z := aufnahmeZitadel(t, as, nil)

	if err := z.Aufnehmen(context.Background(), "377", "vorstand", model.RolleMitglied); err != nil {
		t.Fatalf("Aufnahme: %v", err)
	}
	got := as.geschrieben()
	if len(got) != 1 || got[0] != "aendern vorstand g7 admin,mitglied" {
		t.Fatalf("die vorhandene Rolle wurde nicht bewahrt: %v", got)
	}
}

// Eine stillgelegte Zuweisung zählt nicht (siehe zuweisung.aktiv). Wer wieder
// aufgenommen wird, muss sie wieder in Betrieb bekommen — sonst stünde die
// Rolle da und wirkte trotzdem nicht.
func TestAufnahmeBelebtEineStillgelegteZuweisung(t *testing.T) {
	as := neuerAufnahmeServer(t)
	as.setzen("erna", zuweisung{ID: "g9", ProjectID: "377",
		RoleKeys: []string{"mitglied"}, State: "USER_GRANT_STATE_INACTIVE"})
	z := aufnahmeZitadel(t, as, nil)

	if err := z.Aufnehmen(context.Background(), "377", "erna", model.RolleMitglied); err != nil {
		t.Fatalf("Aufnahme: %v", err)
	}
	got := as.geschrieben()
	if len(got) != 1 || got[0] != "beleben erna g9" {
		t.Fatalf("die stillgelegte Zuweisung wurde nicht wiederbelebt: %v", got)
	}
	if !z.Fuer(context.Background(), auth.User{Sub: "erna"}).Rollen.IstMitglied("377") {
		t.Error("nach dem Wiederbeleben gilt die Mitgliedschaft nicht")
	}
}

// Ein zweites Mal aufnehmen ändert nichts — und meldet keinen Fehler. Sonst
// stünde nach einem Doppelklick eine Absage auf dem Schirm, obwohl alles in
// Ordnung ist.
func TestAufnahmeIstWiederholbar(t *testing.T) {
	as := neuerAufnahmeServer(t)
	as.setzen("erna", zuweisung{ID: "g1", ProjectID: "377", RoleKeys: []string{"mitglied"}})
	z := aufnahmeZitadel(t, as, nil)

	if err := z.Aufnehmen(context.Background(), "377", "erna", model.RolleMitglied); err != nil {
		t.Fatalf("Aufnahme: %v", err)
	}
	if got := as.geschrieben(); len(got) != 0 {
		t.Fatalf("es wurde unnötig geschrieben: %v", got)
	}
}

// Der Fall, an dem in der Produktion alles hängt: Der Dienst-Nutzer darf
// lesen, aber nicht schreiben. Dann muss die Meldung sagen, was fehlt —
// stillschweigend nichts zu tun wäre die schlechteste aller Antworten.
func TestOhneSchreibrechtSagtDieMeldungWasFehlt(t *testing.T) {
	as := neuerAufnahmeServer(t)
	as.verboten = true
	z := aufnahmeZitadel(t, as, nil)

	err := z.Aufnehmen(context.Background(), "377", "erna", model.RolleMitglied)
	if err == nil {
		t.Fatal("eine verweigerte Aufnahme muss ein Fehler sein")
	}
	var api *APIFehler
	if !errors.As(err, &api) || !api.FehlendesRecht() {
		t.Fatalf("das fehlende Recht ist nicht erkennbar: %v", err)
	}
	if !strings.Contains(err.Error(), "user.grant.write") {
		t.Errorf("die Meldung nennt nicht, was einzurichten ist: %v", err)
	}
}

// Ohne Zitadel-Projekt gibt es nichts, worin man Mitglied sein könnte.
func TestAufnahmeOhneProjektIstEinFehler(t *testing.T) {
	as := neuerAufnahmeServer(t)
	z := aufnahmeZitadel(t, as, nil)
	if err := z.Aufnehmen(context.Background(), "", "erna", model.RolleMitglied); err == nil {
		t.Fatal("ohne Projekt darf keine Aufnahme gelingen")
	}
	if err := z.Aufnehmen(context.Background(), "377", "", model.RolleMitglied); err == nil {
		t.Fatal("ohne Person darf keine Aufnahme gelingen")
	}
}

// Die Dev-Quelle liest die Rollen aus dem Token — in ein Token lässt sich
// nichts zurückschreiben. Genau daran erkennt der Rest des Systems, dass eine
// Freigabe nichts bewirken würde.
func TestDevQuelleKannNichtAufnehmen(t *testing.T) {
	if _, ok := AufnehmerVon(DevQuelle{}); ok {
		t.Error("die Dev-Quelle gibt sich fälschlich als Aufnehmer aus")
	}
	if _, ok := AufnehmerVon(nil); ok {
		t.Error("ohne Quelle gibt es niemanden, der aufnehmen könnte")
	}
	as := neuerAufnahmeServer(t)
	if _, ok := AufnehmerVon(aufnahmeZitadel(t, as, nil)); !ok {
		t.Error("die Zitadel-Quelle muss aufnehmen können")
	}
}
