package de.roessing.app

import de.roessing.app.auth.LOGIN_SCOPES
import de.roessing.app.auth.MIETEN_AUDIENCE_SCOPE
import de.roessing.app.auth.ROLLEN_SCOPE
import de.roessing.app.auth.RentalAudience
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Ohne den Rollen-Scope stellt Zitadel ein Token ohne Rollen-Claim aus — und
 * damit ist in der App **niemand** Admin, auch die Verwaltung nicht. Alle
 * Endpunkte, die die Rolle verlangen (Orte und Aufgaben pflegen), antworten
 * dann mit 403.
 *
 * Das ist beim ersten Mal genau so passiert: Die App forderte nur
 * „openid profile email offline_access" an, und der Bereich „Verwaltung" war
 * für alle tot. Der Fehler fällt in keinem Test auf, der die Rechte prüft —
 * denn dort ist das 403 ja das erwartete Ergebnis. Er fällt nur hier auf.
 */
class LoginScopesTest {
    @Test
    fun `der Login fordert die Rollen mit an`() {
        assertTrue(
            "Die App fordert $ROLLEN_SCOPE nicht an — dann ist niemand Admin: $LOGIN_SCOPES",
            LOGIN_SCOPES.contains(ROLLEN_SCOPE),
        )
    }

    /**
     * Die Schreibweise ist heikel: Angefordert wird der Scope mit „projects"
     * (Plural), zurück kommt der Claim mit „project" (Singular). Wer das
     * verwechselt, bekommt wieder ein Token ohne Rollen — und merkt es erst
     * an der ausgelieferten App.
     */
    @Test
    fun `der Rollen-Scope hat die Schreibweise, die Zitadel befuellt`() {
        assertTrue(
            "Erwartet wird der Scope mit „projects\" (Plural): $ROLLEN_SCOPE",
            ROLLEN_SCOPE == "urn:zitadel:iam:org:projects:roles",
        )
    }

    /**
     * Ohne den Empfänger-Scope gilt das Token für die Mietplattform nicht:
     * Sie prüft die `aud` und weist jede angemeldete Anfrage ab. Der Bereich
     * „Maschinchenring" wäre dann für alle lesend und für niemanden buchbar —
     * und niemand sähe, woran es liegt.
     */
    @Test
    fun `der Login fordert die Mietplattform als Empfaenger mit an`() {
        assertTrue(
            "Die App fordert $MIETEN_AUDIENCE_SCOPE nicht an — dann kann niemand " +
                "buchen: $LOGIN_SCOPES",
            LOGIN_SCOPES.contains(MIETEN_AUDIENCE_SCOPE),
        )
    }

    /**
     * Die Projektkennung gehört in die Build-Einstellungen, nicht in den
     * Quelltext — sonst lässt sie sich in CI und Tests nicht übersteuern.
     */
    @Test
    fun `die Projektkennung kommt aus den Build-Einstellungen`() {
        assertEquals(
            RentalAudience.scopeFor(BuildConfig.MIETEN_PROJECT_ID),
            MIETEN_AUDIENCE_SCOPE,
        )
    }

    /** Das Übliche muss bleiben — sonst fehlt hinterher der Name oder das Neuanmelden. */
    @Test
    fun `die bisherigen Scopes bleiben erhalten`() {
        for (noetig in listOf("openid", "profile", "email", "offline_access")) {
            assertTrue("Scope $noetig fehlt: $LOGIN_SCOPES", LOGIN_SCOPES.contains(noetig))
        }
    }
}
