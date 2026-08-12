package de.roessing.app.auth

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.util.Log
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import de.roessing.app.BuildConfig
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import net.openid.appauth.AuthState
import net.openid.appauth.AuthorizationException
import net.openid.appauth.AuthorizationRequest
import net.openid.appauth.AuthorizationResponse
import net.openid.appauth.AuthorizationService
import net.openid.appauth.AuthorizationServiceConfiguration
import net.openid.appauth.ResponseTypeValues
import kotlin.coroutines.resume

private val Context.authDataStore by preferencesDataStore(name = "auth")

/** Login-Zustand der App. */
sealed interface SessionState {
    data object Loading : SessionState
    data object LoggedOut : SessionState
    data class LoggedIn(val devMode: Boolean = false) : SessionState
}

/**
 * Ergebnis des Browser-Logins.
 *
 * [Cancelled] ist bewusst KEIN Fehler: Bricht die Nutzerin den Login per
 * Zurück-Taste oder Schließen-Button ab, landet sie kommentarlos wieder auf dem
 * Login-Screen — ohne rote Fehlermeldung.
 */
sealed interface LoginResult {
    data object Success : LoginResult
    data object Cancelled : LoginResult

    /** Echter Fehler; [code] ist ein technisches Kürzel für die Anzeige/Diagnose. */
    data class Failed(val code: String) : LoginResult
}

/**
 * Kapselt den OIDC-Login gegen die Rössing-ID (Zitadel) via AppAuth:
 * Authorization Code + PKCE im System-Browser, Refresh-Tokens, Persistenz.
 *
 * Im Debug-Build kann zusätzlich ein „Entwickler-Login" aktiviert werden
 * (BuildConfig.DEV_AUTH), der ohne Zitadel ein statisches Dev-Token nutzt —
 * ausschließlich für lokale Entwicklung und E2E-Tests.
 */
class AuthManager(private val context: Context) {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val authService by lazy { AuthorizationService(context) }
    private val stateKey = stringPreferencesKey("authState")
    private val devKey = stringPreferencesKey("devToken")

    private val _session = MutableStateFlow<SessionState>(SessionState.Loading)
    val session: StateFlow<SessionState> = _session

    @Volatile
    private var authState: AuthState? = null

    @Volatile
    private var devToken: String? = null

    init {
        scope.launch { restore() }
    }

    private suspend fun restore() {
        val prefs = context.authDataStore.data.first()
        val dev = prefs[devKey]
        if (isDevAuthAllowed() && dev != null) {
            devToken = dev
            _session.value = SessionState.LoggedIn(devMode = true)
            return
        }
        val json = prefs[stateKey]
        if (json != null) {
            runCatching { AuthState.jsonDeserialize(json) }.onSuccess {
                if (it.isAuthorized) {
                    authState = it
                    _session.value = SessionState.LoggedIn()
                    return
                }
            }
        }
        _session.value = SessionState.LoggedOut
    }

    /** Baut den Browser-Intent für den OIDC-Login. */
    suspend fun buildLoginIntent(): Intent = suspendCancellableCoroutine { cont ->
        AuthorizationServiceConfiguration.fetchFromIssuer(Uri.parse(BuildConfig.OIDC_ISSUER)) { config, ex ->
            if (config == null) {
                cont.cancel(ex ?: IllegalStateException("OIDC-Discovery fehlgeschlagen"))
                return@fetchFromIssuer
            }
            val request = AuthorizationRequest.Builder(
                config,
                BuildConfig.OIDC_CLIENT_ID,
                ResponseTypeValues.CODE,
                Uri.parse(BuildConfig.OIDC_REDIRECT_URI),
            )
                .setScopes("openid", "profile", "email", "offline_access")
                // Bewusst KEIN prompt-Parameter: Zitadel kennt nur none/login/
                // select_account/create. Ein unbekannter Wert wie "consent" ist
                // laut Spec zwar zu ignorieren, ist aber unnötiges Risiko.
                .build()
            cont.resume(authService.getAuthorizationRequestIntent(request))
        }
    }

    /**
     * Verarbeitet das Ergebnis des Browser-Logins (Code → Token-Tausch).
     *
     * Bewusst komplett defensiv: Auf dem Rückweg aus dem Browser darf NIE eine
     * unbehandelte Exception fliegen (sonst stirbt der Prozess und die App wirkt
     * „wird wiederholt beendet"). Insbesondere darf [AuthState] erst gebaut werden,
     * wenn feststeht, dass genau eines von Response/Exception vorliegt — der
     * Konstruktor wirft sonst eine IllegalArgumentException.
     */
    suspend fun handleAuthResult(data: Intent?): LoginResult = try {
        handleAuthResultInternal(data)
    } catch (t: Throwable) {
        Log.e(TAG, "Unerwarteter Fehler beim Verarbeiten des Login-Ergebnisses", t)
        LoginResult.Failed(t::class.java.simpleName)
    }

    private suspend fun handleAuthResultInternal(data: Intent?): LoginResult {
        // Leerer Result-Intent = Abbruch (z.B. Zurück-Taste im Custom Tab).
        data ?: return LoginResult.Cancelled

        val resp = runCatching { AuthorizationResponse.fromIntent(data) }
            .onFailure { Log.w(TAG, "AuthorizationResponse nicht lesbar", it) }
            .getOrNull()
        val ex = runCatching { AuthorizationException.fromIntent(data) }
            .onFailure { Log.w(TAG, "AuthorizationException nicht lesbar", it) }
            .getOrNull()

        if (resp == null) {
            val result = classifyFailure(ex?.type, ex?.code, ex?.error)
            Log.w(TAG, "Login ohne Autorisierungs-Code beendet: $result (${ex?.errorDescription})")
            return result
        }

        // Erst hier ist garantiert: resp != null → AuthState-Konstruktor ist zulässig.
        val state = AuthState(resp, null)
        val tokenError = suspendCancellableCoroutine { cont ->
            authService.performTokenRequest(resp.createTokenExchangeRequest()) { tokenResp, tokenEx ->
                runCatching { state.update(tokenResp, tokenEx) }
                    .onFailure { Log.w(TAG, "AuthState-Update fehlgeschlagen", it) }
                cont.resume(if (tokenResp != null) null else tokenEx)
            }
        }
        if (tokenError != null || !state.isAuthorized) {
            val result = classifyFailure(tokenError?.type, tokenError?.code, tokenError?.error)
            Log.e(TAG, "Token-Tausch fehlgeschlagen: $result (${tokenError?.errorDescription})")
            return result
        }

        authState = state
        persist()
        _session.value = SessionState.LoggedIn()
        return LoginResult.Success
    }

    /** Entwickler-Login ohne Zitadel (nur Debug + DEV_AUTH). */
    suspend fun devLogin(asAdmin: Boolean) {
        check(isDevAuthAllowed()) { "Dev-Login ist in diesem Build deaktiviert" }
        val roles = if (asAdmin) "admin" else ""
        devToken = "e2e-user:E2E Tester:$roles"
        context.authDataStore.edit { it[devKey] = devToken!! }
        _session.value = SessionState.LoggedIn(devMode = true)
    }

    /** Liefert ein gültiges Access-Token (refresht bei Bedarf) oder null. */
    suspend fun freshAccessToken(): String? {
        devToken?.let { return it }
        val state = authState ?: return null
        return suspendCancellableCoroutine { cont ->
            state.performActionWithFreshTokens(authService) { accessToken, _, ex ->
                if (ex != null) {
                    // Refresh fehlgeschlagen (z.B. Token widerrufen) → ausloggen.
                    scope.launch { logout() }
                    cont.resume(null)
                } else {
                    scope.launch { persist() }
                    cont.resume(accessToken)
                }
            }
        }
    }

    suspend fun logout() {
        authState = null
        devToken = null
        context.authDataStore.edit { it.clear() }
        _session.value = SessionState.LoggedOut
    }

    private suspend fun persist() {
        val json = authState?.jsonSerializeString() ?: return
        context.authDataStore.edit { it[stateKey] = json }
    }

    companion object {
        private const val TAG = "AuthManager"

        fun isDevAuthAllowed(): Boolean = BuildConfig.DEBUG && BuildConfig.DEV_AUTH

        /**
         * Übersetzt eine AppAuth-Fehlerkennung in ein [LoginResult].
         *
         * Bewusst als reine Funktion ohne Android-Abhängigkeiten, damit sie im
         * JVM-Unit-Test abgedeckt werden kann.
         *
         * @param type AppAuth-Fehlertyp (AuthorizationException.TYPE_*), oder null.
         * @param code AppAuth-Fehlercode innerhalb des Typs, oder null.
         * @param error OAuth-Fehlerkürzel aus der Antwort (z.B. „invalid_grant"), oder null.
         */
        fun classifyFailure(type: Int?, code: Int?, error: String?): LoginResult {
            // TYPE_GENERAL_ERROR (0) / CODE 1 = USER_CANCELED_AUTH_FLOW.
            if (type == AuthorizationException.TYPE_GENERAL_ERROR &&
                (code == AuthorizationException.GeneralErrors.USER_CANCELED_AUTH_FLOW.code ||
                    code == AuthorizationException.GeneralErrors.PROGRAM_CANCELED_AUTH_FLOW.code)
            ) {
                return LoginResult.Cancelled
            }
            // Weder Response noch Fehler: leerer Rückweg → wie Abbruch behandeln.
            if (type == null && code == null && error == null) return LoginResult.Cancelled
            return LoginResult.Failed(error ?: "${type ?: "?"}.${code ?: "?"}")
        }
    }
}
