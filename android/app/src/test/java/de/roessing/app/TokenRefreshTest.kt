package de.roessing.app

import de.roessing.app.auth.AuthManager
import de.roessing.app.auth.TokenResult
import de.roessing.app.data.DorfApi
import net.openid.appauth.AuthorizationException
import okhttp3.Request
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertThrows
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

/**
 * Dass eine einmal erteilte Anmeldung hält.
 *
 * Der Kern ist ein einziger Unterschied: **„Der Server sagt nein" ist etwas
 * anderes als „ich konnte den Server nicht fragen."** Das Erste ist eine
 * Entscheidung der Rössing-ID — widerrufenes Refresh-Token, gesperrtes Konto,
 * geändertes Passwort —, das Zweite ein Funkloch. Solange beides zum selben
 * Ergebnis führte, kostete jeder schlechte Augenblick im Mobilfunknetz die
 * Anmeldung, und die Person stand ohne Grund wieder vor dem Login-Screen.
 *
 * Kein Test hier fasst einen Server an: Geprüft werden zwei reine Funktionen.
 */
class TokenRefreshTest {

    // --- Was die Rössing-ID wirklich entschieden hat -------------------------

    @Test
    fun einWiderrufenesRefreshTokenBeendetDieSitzung() {
        val ex = AuthorizationException.TokenRequestErrors.INVALID_GRANT
        assertTrue(AuthManager.isSessionEnded(ex.type, ex.error))
    }

    // --- Was bloß ein Umstand war -------------------------------------------

    @Test
    fun einFunklochBeendetKeineSitzung() {
        val ex = AuthorizationException.GeneralErrors.NETWORK_ERROR
        assertFalse(AuthManager.isSessionEnded(ex.type, ex.error))
    }

    @Test
    fun eineUnlesbareAntwortBeendetKeineSitzung() {
        val ex = AuthorizationException.GeneralErrors.JSON_DESERIALIZATION_ERROR
        assertFalse(AuthManager.isSessionEnded(ex.type, ex.error))
    }

    @Test
    fun einFehlerInUnsererEinrichtungBeendetKeineSitzung() {
        // `invalid_client` heißt: An unserer Anmeldung ist etwas falsch. Wer
        // deswegen ausloggt, wirft eine gültige Sitzung weg — und die neue
        // Anmeldung scheiterte an derselben Stelle wieder.
        val ex = AuthorizationException.TokenRequestErrors.INVALID_CLIENT
        assertFalse(AuthManager.isSessionEnded(ex.type, ex.error))
    }

    @Test
    fun einServerfehlerBeendetKeineSitzung() {
        // 5xx ist der Zustand des Servers, nicht sein Urteil über die Sitzung.
        assertFalse(AuthManager.isSessionEnded(AuthorizationException.TYPE_GENERAL_ERROR, "server_error"))
    }

    @Test
    fun garKeineAngabeBeendetKeineSitzung() {
        assertFalse(AuthManager.isSessionEnded(null, null))
    }

    // --- Was daraus für die Anfrage folgt -----------------------------------

    private val anfrage = Request.Builder().url("http://127.0.0.1:8099/api/v1/me").build()

    @Test
    fun mitTokenGehtDieKopfzeileMit() {
        val fertig = DorfApi.autorisiert(anfrage, TokenResult.Token("abc"))
        assertEquals("Bearer abc", fertig.header("Authorization"))
    }

    @Test
    fun ohneAnmeldungGehtDieAnfrageOhneKopfzeileHinaus() {
        val fertig = DorfApi.autorisiert(anfrage, TokenResult.LoggedOut)
        assertNull(fertig.header("Authorization"))
    }

    @Test
    fun eineNichtErneuerbareAnmeldungGehtGarNichtErstHinaus() {
        // Ohne Kopfzeile käme ein 401 zurück, und die App machte daraus
        // „nicht angemeldet". Eine IOException ist die Wahrheit: ein
        // Netzproblem — und die Bereiche zeigen dafür „offline".
        assertThrows(IOException::class.java) {
            DorfApi.autorisiert(anfrage, TokenResult.Unreachable)
        }
    }
}
