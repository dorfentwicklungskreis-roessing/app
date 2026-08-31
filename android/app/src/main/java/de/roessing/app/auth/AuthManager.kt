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

/**
 * Der Scope, mit dem Zitadel die Projektrollen ins Token legt.
 *
 * Angefordert wird er mit „projects" (Plural), zurück kommt der Claim
 * `urn:zitadel:iam:org:project:roles` mit „project" (Singular) — daraus liest
 * das Backend die Rolle `admin` (siehe backend/internal/auth). Ohne diesen
 * Scope stellt Zitadel ein Token ganz ohne Rollen aus: Dann ist in der App
 * niemand Verwaltung, und der Bereich „Verwaltung" antwortet nur mit 403.
 *
 * Dieselbe Schreibweise nutzt die Web-Verwaltung seit jeher
 * (backend/internal/admin/oidc.go) — sie bekommt damit nachweislich Rollen.
 */
const val ROLLEN_SCOPE = "urn:zitadel:iam:org:projects:roles"

/**
 * Der Scope, mit dem das Token auch für die Mietplattform gilt.
 *
 * Ein Zitadel-Token gilt nur für die Projekte, die in seiner `aud` stehen.
 * Die Mietplattform („Maschinchenring") ist ein eigenes Projekt derselben
 * Rössing-ID; ohne diesen Scope weist sie jede angemeldete Anfrage der App
 * ab. Die Projektkennung steht in den Build-Einstellungen, nicht hier.
 */
val MIETEN_AUDIENCE_SCOPE: String = RentalAudience.scopeFor(BuildConfig.MIETEN_PROJECT_ID)

/**
 * Die Scopes, mit denen sich die App anmeldet.
 *
 * offline_access hält die Sitzung über den Neustart hinweg, die Rollen
 * entscheiden, wer verwalten darf, und die Empfänger-Angabe macht das Token
 * zusätzlich für die Mietplattform gültig.
 */
val LOGIN_SCOPES = listOf(
    "openid",
    "profile",
    "email",
    "offline_access",
    ROLLEN_SCOPE,
    MIETEN_AUDIENCE_SCOPE,
)

/**
 * Was die App gerade als Access-Token anbieten kann.
 *
 * Der Unterschied zwischen [LoggedOut] und [Unreachable] ist der ganze Punkt:
 * „Der Server sagt nein" ist eine Entscheidung, „ich konnte den Server nicht
 * fragen" ein Umstand. Nur das Erste beendet eine Anmeldung. Solange beides
 * `null` hieß, kostete jedes Funkloch die Anmeldung.
 */
sealed interface TokenResult {
    data class Token(val value: String) : TokenResult

    /** Niemand ist angemeldet. */
    data object LoggedOut : TokenResult

    /** Jemand ist angemeldet, aber das Token ließ sich gerade nicht erneuern. */
    data object Unreachable : TokenResult
}

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

    /**
     * AppAuth spricht standardmäßig nur https. Für den E2E-Lauf steht der
     * Aussteller lokal (`http://10.0.2.2:8123`), damit sich kein Test an der
     * Produktion anmeldet — [appAuthKonfiguration] macht daraus die eng
     * begrenzte Ausnahme. Im Release-Build ist es die strenge Vorbelegung.
     */
    private val authService by lazy { AuthorizationService(context, appAuthKonfiguration) }
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
        AuthorizationServiceConfiguration.fetchFromIssuer(
            Uri.parse(BuildConfig.OIDC_ISSUER),
            { config, ex ->
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
                    .setScopes(LOGIN_SCOPES)
                    // Bewusst KEIN prompt-Parameter: Zitadel kennt nur none/login/
                    // select_account/create. Ein unbekannter Wert wie "consent" ist
                    // laut Spec zwar zu ignorieren, ist aber unnötiges Risiko.
                    .build()
                cont.resume(authService.getAuthorizationRequestIntent(request))
            },
            // Die Discovery läuft über denselben Aufbau wie der Token-Tausch —
            // sonst scheitert sie am lokalen Aussteller, bevor der Browser
            // überhaupt aufgeht.
            oidcVerbindungsaufbau,
        )
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

    /**
     * Liefert die Tokenlage: ein gültiges Access-Token (refresht bei Bedarf),
     * „niemand angemeldet" oder „gerade nicht erreichbar".
     *
     * Um die gleichzeitigen Erneuerungen muss sich hier niemand kümmern:
     * [AuthState.performActionWithFreshTokens] bündelt sie selbst (es reiht
     * weitere Aufrufe in `mPendingActions` ein, solange eine Erneuerung
     * läuft). Ohne das schickte jeder Abruf beim Kaltstart seine eigene
     * Erneuerung los — und Zitadel gibt bei jeder ein neues Refresh-Token aus
     * und weist das alte danach ab.
     */
    suspend fun freshToken(): TokenResult {
        devToken?.let { return TokenResult.Token(it) }
        val state = authState ?: return TokenResult.LoggedOut
        return suspendCancellableCoroutine { cont ->
            state.performActionWithFreshTokens(authService) { accessToken, _, ex ->
                when {
                    ex == null && accessToken != null -> {
                        scope.launch { persist() }
                        cont.resume(TokenResult.Token(accessToken))
                    }
                    // Ohne Refresh-Token gibt es nichts mehr zu erneuern —
                    // das ist wirklich das Ende der Sitzung, kein Umstand.
                    state.refreshToken == null || isSessionEnded(ex?.type, ex?.error) -> {
                        Log.w(TAG, "Die Rössing-ID hat die Sitzung beendet (${ex?.error})")
                        scope.launch { logout() }
                        cont.resume(TokenResult.LoggedOut)
                    }
                    else -> {
                        // Nicht fragen zu können ist kein Nein: Die Anmeldung
                        // bleibt stehen, der nächste Abruf versucht es erneut.
                        Log.i(TAG, "Erneuerung gerade nicht möglich (${ex?.type}.${ex?.code})")
                        cont.resume(TokenResult.Unreachable)
                    }
                }
            }
        }
    }

    /**
     * Liefert ein gültiges Access-Token oder null.
     *
     * Bleibt für den E2E-Lauf, der nur wissen will, ob eines herauskommt. Der
     * Unterschied zwischen „abgemeldet" und „nicht erreichbar" geht dabei
     * verloren — wer ihn braucht, nimmt [freshToken].
     */
    suspend fun freshAccessToken(): String? = (freshToken() as? TokenResult.Token)?.value

    /**
     * Das zuletzt erhaltene ID-Token — nur zur Diagnose im Login-Test.
     *
     * Zitadel legt Rollen je nach Einstellung ins Access-Token, ins ID-Token
     * oder in keines von beiden. Wer wissen will, warum niemand Verwaltung
     * ist, muss beide ansehen können.
     */
    val letztesIdToken: String? get() = authState?.idToken

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
         * Ob eine gescheiterte Erneuerung das Ende der Sitzung bedeutet.
         *
         * RFC 6749 lässt den Token-Endpunkt eine abgelehnte Zuteilung mit
         * einem Kürzel beantworten. `invalid_grant` ist das einzige, das
         * „diese Sitzung ist vorbei" heißt — widerrufenes Refresh-Token,
         * gesperrtes Konto, geändertes Passwort. Alles andere ist entweder
         * ein Fehler in unserer Einrichtung (`invalid_client`) oder der
         * Zustand des Netzes bzw. des Servers, und beides darf keine gültige
         * Anmeldung kosten: Nach `invalid_client` scheiterte die neue
         * Anmeldung an derselben Stelle wieder, und nach einem Funkloch war
         * nie etwas mit der Sitzung.
         *
         * Bewusst als reine Funktion ohne Android-Abhängigkeiten, damit sie
         * im JVM-Unit-Test abgedeckt werden kann.
         *
         * @param type AppAuth-Fehlertyp (AuthorizationException.TYPE_*), oder null.
         * @param error OAuth-Fehlerkürzel aus der Antwort, oder null.
         */
        fun isSessionEnded(type: Int?, error: String?): Boolean =
            type == AuthorizationException.TYPE_OAUTH_TOKEN_ERROR && error == "invalid_grant"

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
