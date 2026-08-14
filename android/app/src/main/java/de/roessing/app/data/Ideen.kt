package de.roessing.app.data

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import retrofit2.HttpException

/**
 * Die Ideen-Sammlung: „Was soll die App noch können?"
 *
 * Derselbe Eingang, den auch das Formular auf der Website benutzt
 * (`POST /api/v1/ideen`). Aus der App geht die Einreichung angemeldet
 * hinaus — das Backend hängt sie dann an das Konto und vermerkt als Weg
 * „app". Name und E-Mail sind auch hier freiwillig; vorbelegt werden sie
 * aus dem Profil, überschreiben lässt sie sich jederzeit.
 */

/** Eingabe von POST /api/v1/ideen. */
@Serializable
data class IdeeInput(
    val wunsch: String,
    val name: String = "",
    val email: String = "",
)

/** Antwort von POST /api/v1/ideen. */
@Serializable
data class IdeeDto(
    val id: Long = 0,
    val wunsch: String = "",
    val name: String = "",
    val email: String = "",
    /** website · app */
    val quelle: String = "app",
    /** neu · gelesen · umgesetzt · abgelehnt */
    val status: String = "neu",
    val createdAt: String = "",
)

/**
 * Das Backend hat die Einreichung abgewiesen (HTTP 400) und nennt den Grund
 * im Klartext. Er ist für die Person gedacht — die Prüfung sitzt bewusst im
 * Backend, damit für Website und App dieselben Regeln gelten.
 */
class IdeenAblehnungException(val grund: String) : RuntimeException(grund)

/** Zu viele Einreichungen in kurzer Zeit (HTTP 429). */
class IdeenZuVieleException : RuntimeException("zu viele Einreichungen")

interface IdeenRepository {
    suspend fun einreichen(input: IdeeInput): IdeeDto
}

class ApiIdeenRepository(private val api: DorfApi) : IdeenRepository {
    override suspend fun einreichen(input: IdeeInput): IdeeDto =
        try {
            api.idee(input)
        } catch (e: HttpException) {
            when (e.code()) {
                400 -> throw IdeenAblehnungException(fehlertextAus(e))
                429 -> throw IdeenZuVieleException()
                else -> throw e
            }
        }

    private fun fehlertextAus(e: HttpException): String = runCatching {
        val roh = e.response()?.errorBody()?.string().orEmpty()
        Json { ignoreUnknownKeys = true }.decodeFromString<ApiErrorDto>(roh).error
    }.getOrNull()?.takeIf { it.isNotBlank() } ?: "Die Eingabe wurde abgelehnt."
}
