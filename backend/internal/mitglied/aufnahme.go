package mitglied

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/dorfentwicklungskreis-roessing/app/backend/internal/model"
)

// Die schreibende Seite: eine Mitgliedschaft nach Zitadel zurückschreiben.
//
// # Warum das hier stehen muss und nicht in der App
//
// Bis hierher las das Backend die Mitgliedschaften nur. Wer einem Verein
// beitreten wollte, musste warten, bis ein Mensch ihn in der Zitadel-Konsole
// einträgt. Ein Antragsverfahren in der App, das am Ende nichts bewirkt, wäre
// eine Sackgasse: Der Träger-Admin drückt „aufnehmen“, die App sagt
// „Mitglied“ — und die Rössing-ID weiß nichts davon. Deshalb gilt: Der
// Vorgang gilt erst als erteilt, wenn die Rollenzuweisung dort wirklich
// steht.
//
// # Was der Betreiber dafür einrichten muss
//
// Der Dienst-Nutzer braucht in der Rössing-ID das Recht, Nutzer-Zuweisungen
// zu ändern („user.grant.write“; in Zitadel steckt es in der Manager-Rolle
// ORG_USER_PERMISSION_EDITOR). Bis dahin scheitert jede Aufnahme mit einer
// deutlichen Meldung, statt still nichts zu tun.

// Aufnehmer schreibt eine Rollenzuweisung nach Zitadel zurück.
//
// Bewusst nur aufnehmen und nicht entfernen: Zu #56 gehört der Beitritt. Wer
// jemanden wieder herausnimmt, tut das bis auf Weiteres in der Konsole — ein
// Austritt hat andere Folgen (Zusagen, interne Aufgaben) und will eigens
// bedacht werden.
type Aufnehmer interface {
	// Aufnehmen gibt der Person die Rolle im Projekt. Hat sie sie schon,
	// passiert nichts — der Aufruf ist wiederholbar.
	Aufnehmen(ctx context.Context, projektID, userSub, rolle string) error
}

// ErrKeineAufnahme heißt: Es gibt niemanden, der schreiben könnte. In der
// Produktion ist das der Zustand ohne eingerichteten Dienst-Nutzer.
var ErrKeineAufnahme = errors.New("in der Rössing-ID kann gerade niemand aufgenommen werden — " +
	"dafür braucht das Backend einen Dienst-Nutzer mit Schreibrecht auf Rollenzuweisungen")

// AufnehmerVon liefert die schreibende Seite einer Quelle, falls sie eine
// hat. Die Dev-Quelle etwa hat keine: Sie liest die Rollen aus dem Token, und
// in ein Token lässt sich nichts zurückschreiben.
func AufnehmerVon(q Quelle) (Aufnehmer, bool) {
	a, ok := q.(Aufnehmer)
	return a, ok
}

// Aufnehmen trägt die Rolle in Zitadel ein.
//
// Drei Fälle, und alle drei kommen vor:
//   - Es gibt noch keine Zuweisung zu diesem Projekt → eine neue anlegen.
//   - Es gibt eine, aber ohne die Rolle → die Rolle ergänzen, statt die
//     vorhandenen zu überschreiben. Wer schon „admin“ ist, bleibt es.
//   - Es gibt eine und sie ist stillgelegt → wieder in Betrieb nehmen,
//     sonst zählte sie nicht (siehe zuweisung.aktiv).
func (z *Zitadel) Aufnehmen(ctx context.Context, projektID, userSub, rolle string) error {
	if projektID == "" {
		return errors.New("ohne Zitadel-Projekt gibt es keine Mitgliedschaft")
	}
	if userSub == "" {
		return errors.New("ohne Kennung der Person geht das nicht")
	}
	if rolle == "" {
		rolle = model.RolleMitglied
	}
	grants, err := z.zuweisungen(ctx, userSub)
	if err != nil {
		return err
	}
	for _, g := range grants {
		if g.ProjectID != projektID {
			continue
		}
		if err := z.zuweisungErgaenzen(ctx, userSub, rolle, g); err != nil {
			return err
		}
		// Der gemerkte Stand ist jetzt falsch — die nächste Frage geht
		// wieder nach Zitadel, damit die Aufnahme sofort wirkt.
		z.cache.vergessen(userSub)
		return nil
	}
	pfad := "/management/v1/users/" + userSub + "/grants"
	rumpf := map[string]any{"projectId": projektID, "roleKeys": []string{rolle}}
	if err := z.ruf(ctx, http.MethodPost, pfad, rumpf, nil); err != nil {
		return schreibfehler(err)
	}
	z.cache.vergessen(userSub)
	return nil
}

// zuweisungErgaenzen bringt eine bestehende Zuweisung auf den Stand.
func (z *Zitadel) zuweisungErgaenzen(ctx context.Context, userSub, rolle string, g zuweisung) error {
	pfad := "/management/v1/users/" + userSub + "/grants/" + g.ID
	if !g.hat(rolle) {
		rollen := append(append([]string{}, g.RoleKeys...), rolle)
		if err := z.ruf(ctx, http.MethodPut, pfad, map[string]any{"roleKeys": rollen}, nil); err != nil {
			return schreibfehler(err)
		}
	}
	if !g.aktiv() {
		if err := z.ruf(ctx, http.MethodPost, pfad+"/_reactivate", map[string]any{}, nil); err != nil {
			return schreibfehler(err)
		}
	}
	return nil
}

// schreibfehler übersetzt ein fehlendes Recht in einen Satz, aus dem der
// Betreiber ablesen kann, was er in der Rössing-ID einzurichten hat. Alles
// andere bleibt, wie es kam.
func schreibfehler(err error) error {
	var api *APIFehler
	if errors.As(err, &api) && api.FehlendesRecht() {
		return fmt.Errorf("der Dienst-Nutzer darf in der Rössing-ID keine Rollen vergeben "+
			"(es fehlt das Recht „user.grant.write“, in Zitadel die Manager-Rolle "+
			"ORG_USER_PERMISSION_EDITOR): %w", err)
	}
	return err
}
