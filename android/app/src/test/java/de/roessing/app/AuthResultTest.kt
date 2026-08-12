package de.roessing.app

import de.roessing.app.auth.AuthManager
import de.roessing.app.auth.LoginResult
import net.openid.appauth.AuthorizationException
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Absicherung der Ergebnis-Verarbeitung des Browser-Logins.
 *
 * Hintergrund: Ein leerer bzw. abgebrochener Rückweg aus dem Browser darf weder
 * zum Absturz führen noch als „Anmeldung fehlgeschlagen" angezeigt werden.
 */
class AuthResultTest {

    @Test
    fun abbruchDurchNutzerIstKeinFehler() {
        val ex = AuthorizationException.GeneralErrors.USER_CANCELED_AUTH_FLOW
        assertEquals(
            LoginResult.Cancelled,
            AuthManager.classifyFailure(ex.type, ex.code, ex.error),
        )
    }

    @Test
    fun programmatischerAbbruchIstKeinFehler() {
        val ex = AuthorizationException.GeneralErrors.PROGRAM_CANCELED_AUTH_FLOW
        assertEquals(
            LoginResult.Cancelled,
            AuthManager.classifyFailure(ex.type, ex.code, ex.error),
        )
    }

    @Test
    fun leeresErgebnisWirdWieAbbruchBehandelt() {
        assertEquals(LoginResult.Cancelled, AuthManager.classifyFailure(null, null, null))
    }

    @Test
    fun oauthFehlerLiefertFehlerkuerzel() {
        val ex = AuthorizationException.AuthorizationRequestErrors.ACCESS_DENIED
        assertEquals(
            LoginResult.Failed("access_denied"),
            AuthManager.classifyFailure(ex.type, ex.code, ex.error),
        )
    }

    @Test
    fun tokenFehlerLiefertFehlerkuerzel() {
        val ex = AuthorizationException.TokenRequestErrors.INVALID_GRANT
        assertEquals(
            LoginResult.Failed("invalid_grant"),
            AuthManager.classifyFailure(ex.type, ex.code, ex.error),
        )
    }

    @Test
    fun fehlerOhneKuerzelNutztTypUndCode() {
        val ex = AuthorizationException.GeneralErrors.NETWORK_ERROR
        assertEquals(
            LoginResult.Failed("${ex.type}.${ex.code}"),
            AuthManager.classifyFailure(ex.type, ex.code, ex.error),
        )
    }
}
