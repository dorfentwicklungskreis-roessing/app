package de.roessing.app.auth

import android.content.Context
import android.content.Intent
import android.net.Uri
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

    /** Baut den Browser-Intent für den OIDC-Login (mit Consent-Screen). */
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
                // prompt=consent: bewusster Zustimmungs-Screen bei jedem Login.
                .setPrompt("consent")
                .build()
            cont.resume(authService.getAuthorizationRequestIntent(request))
        }
    }

    /** Verarbeitet das Ergebnis des Browser-Logins (Code → Token-Tausch). */
    suspend fun handleAuthResult(data: Intent?): Boolean {
        data ?: return false
        val resp = AuthorizationResponse.fromIntent(data)
        val ex = AuthorizationException.fromIntent(data)
        val state = AuthState(resp, ex)
        if (resp == null) return false
        val success = suspendCancellableCoroutine { cont ->
            authService.performTokenRequest(resp.createTokenExchangeRequest()) { tokenResp, tokenEx ->
                state.update(tokenResp, tokenEx)
                cont.resume(tokenResp != null)
            }
        }
        if (success) {
            authState = state
            persist()
            _session.value = SessionState.LoggedIn()
        }
        return success
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
        fun isDevAuthAllowed(): Boolean = BuildConfig.DEBUG && BuildConfig.DEV_AUTH
    }
}
