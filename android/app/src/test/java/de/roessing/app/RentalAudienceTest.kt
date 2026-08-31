package de.roessing.app

import de.roessing.app.auth.RentalAudience
import de.roessing.app.auth.RentalSignIn
import de.roessing.app.auth.TokenResult
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.util.Base64

/**
 * Gilt das Token dieses Geräts für die Mietplattform?
 *
 * Der Scope, der ihr Projekt in die `aud` legt, wirkt erst bei der
 * **nächsten** Anmeldung. Ein Gerät, das schon angemeldet war, behält seinen
 * Token-Satz über die Aktualisierung hinweg — und bekommt von der
 * Mietplattform auf jede Anfrage ein 401. Wer das nicht vorher ansieht, zeigt
 * eine leere Liste und weiß nicht, warum.
 */
class RentalAudienceTest {
    private val projekt = "377276525071827047"

    private fun token(nutzlast: String): String {
        val teil = Base64.getUrlEncoder().withoutPadding()
            .encodeToString(nutzlast.toByteArray(Charsets.UTF_8))
        return "kopf.$teil.unterschrift"
    }

    @Test
    fun `der Scope traegt die Projektkennung`() {
        assertEquals(
            "urn:zitadel:iam:org:project:id:$projekt:aud",
            RentalAudience.scopeFor(projekt),
        )
    }

    @Test
    fun `eine Empfaengerliste wird gelesen`() {
        val jwt = token("""{"aud":["dorf-app","$projekt"],"sub":"erna"}""")

        assertEquals(listOf("dorf-app", projekt), RentalAudience.audiences(jwt))
        assertEquals(true, RentalAudience.namesProject(jwt, projekt))
    }

    /** RFC 7519 lässt `aud` auch als einzelne Zeichenkette zu. */
    @Test
    fun `ein einzelner Empfaenger wird ebenso gelesen`() {
        val jwt = token("""{"aud":"dorf-app"}""")

        assertEquals(listOf("dorf-app"), RentalAudience.audiences(jwt))
        assertEquals(false, RentalAudience.namesProject(jwt, projekt))
    }

    /**
     * Ein undurchsichtiges Token ist kein Nein. Die App darf niemandem auf
     * Verdacht sagen, er solle sich neu anmelden — dann entscheidet der
     * Server.
     */
    @Test
    fun `ein unlesbares Token laesst die Frage offen`() {
        assertNull(RentalAudience.audiences("kein-jwt"))
        assertNull(RentalAudience.namesProject("kein-jwt", projekt))
        assertEquals(RentalSignIn.VALID, RentalAudience.state(TokenResult.Token("x"), projekt))
    }

    @Test
    fun `ein Token von vor der Umstellung heisst neu anmelden`() {
        val alt = TokenResult.Token(token("""{"aud":["dorf-app"]}"""))

        assertEquals(RentalSignIn.STALE, RentalAudience.state(alt, projekt))
    }

    @Test
    fun `ein Token mit der Mietplattform gilt`() {
        val neu = TokenResult.Token(token("""{"aud":["dorf-app","$projekt"]}"""))

        assertEquals(RentalSignIn.VALID, RentalAudience.state(neu, projekt))
    }

    /**
     * Nicht fragen zu können ist kein Nein: Die Anmeldung steht, das Netz
     * nicht. Der Bereich zeigt „nicht erreichbar", nicht „neu anmelden".
     */
    @Test
    fun `ein Funkloch ist keine abgelaufene Anmeldung`() {
        assertEquals(RentalSignIn.UNREACHABLE, RentalAudience.state(TokenResult.Unreachable, projekt))
        assertEquals(RentalSignIn.MISSING, RentalAudience.state(TokenResult.LoggedOut, projekt))
    }
}
