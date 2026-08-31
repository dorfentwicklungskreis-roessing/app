package de.roessing.app.auth

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.util.Base64

/**
 * Does the access token of this device count for the rental platform?
 *
 * Zitadel only puts a second project into the `aud` claim when the app asks
 * for it, with a scope of the shape
 * `urn:zitadel:iam:org:project:id:<id>:aud`. Adding that scope is one line —
 * but it only takes effect at the *next* sign-in. A phone that is already
 * signed in keeps its set of tokens across the app update, so it keeps
 * handing out tokens without the new audience, and the rental platform
 * answers 401 to every one of them.
 *
 * The app therefore looks into its own token before it asks: is the project
 * in there? That is not a business rule — it is the app reading its own
 * credential — and it means the sentence "please sign in again" appears
 * before the first pointless round trip rather than after it.
 *
 * Deliberately pure functions without Android dependencies, so the whole
 * decision is covered by plain JVM unit tests.
 */
object RentalAudience {
    private val json = Json { ignoreUnknownKeys = true }

    /**
     * The scope that makes Zitadel add [projectId] to the `aud` claim.
     *
     * The project id belongs in the build settings, never in source — see
     * `BuildConfig.MIETEN_PROJECT_ID`.
     */
    fun scopeFor(projectId: String): String = "urn:zitadel:iam:org:project:id:$projectId:aud"

    /**
     * The audiences of a JWT access token, or null when the token cannot be
     * read as a JWT.
     *
     * No signature is verified here, and none needs to be: the token was
     * handed to us by our own [AuthManager], and the only question is which
     * recipients it names. Whoever forges an `aud` for themselves gains
     * nothing — the rental platform verifies the signature.
     *
     * `aud` is a string or an array of strings (RFC 7519, 4.1.3); both occur
     * in the wild, so both are read.
     */
    fun audiences(token: String): List<String>? {
        val parts = token.split('.')
        if (parts.size < 2) return null
        val payload = runCatching {
            String(Base64.getUrlDecoder().decode(parts[1]), Charsets.UTF_8)
        }.getOrNull() ?: return null
        val claims = runCatching { json.parseToJsonElement(payload) as? JsonObject }
            .getOrNull() ?: return null
        return when (val aud = claims["aud"]) {
            is JsonPrimitive -> listOf(aud.content)
            is JsonArray -> aud.mapNotNull { (it as? JsonPrimitive)?.content }
            else -> emptyList()
        }
    }

    /**
     * Whether the token names [projectId] as an audience.
     *
     * Returns null when the token is not readable as a JWT. That is not the
     * same as "no": an opaque token may well be valid over there, and the app
     * must not tell someone to sign in again on a hunch. In that case the
     * request goes out, and a 401 from the server has the final word.
     */
    fun namesProject(token: String, projectId: String): Boolean? {
        if (projectId.isBlank()) return true
        return audiences(token)?.contains(projectId)
    }

    /**
     * The state of the sign-in as far as the rental platform is concerned.
     *
     * @param token what [AuthManager.freshToken] currently offers
     * @param projectId Zitadel project of the rental platform, from BuildConfig
     */
    fun state(token: TokenResult, projectId: String): RentalSignIn = when (token) {
        is TokenResult.Token ->
            if (namesProject(token.value, projectId) == false) RentalSignIn.STALE
            else RentalSignIn.VALID

        TokenResult.LoggedOut -> RentalSignIn.MISSING
        // Not being able to ask is not a no: the sign-in stands, the network
        // does not. The area shows "offline", not "sign in again".
        TokenResult.Unreachable -> RentalSignIn.UNREACHABLE
    }
}

/** How the sign-in of this device relates to the rental platform. */
enum class RentalSignIn {
    /** A token is there and names the rental platform (or we cannot tell). */
    VALID,

    /** A token is there, but it was issued before the changeover. */
    STALE,

    /** Nobody is signed in. */
    MISSING,

    /** Somebody is signed in, but the token could not be renewed right now. */
    UNREACHABLE,
}
