package de.roessing.app.data

import kotlinx.serialization.Serializable

/**
 * Fehlerberichte aus der App.
 *
 * Derselbe Eingang, den auch die iOS-App benutzt
 * (`POST /api/v1/error-reports`). Er ist bewusst ohne Anmeldung erreichbar:
 * Die Ausfälle, auf die es ankommt, sind gerade die, bei denen das Anmelden
 * selbst klemmt. Ein Token geht trotzdem mit, wenn es eines gibt — dann hängt
 * der Bericht am Konto, und der Dorfentwicklungskreis kann nachfragen.
 *
 * In der Eingabe steht nur, was die App ohnehin über sich weiß. Kein
 * Protokoll, kein Bildschirmfoto, kein Standort, keine Gerätekennung.
 */

/** Eingabe von POST /api/v1/error-reports. */
@Serializable
data class ErrorReportInput(
    /** crash · network · server · unexpected */
    val kind: String,
    /** Der Satz, den die Person auf dem Schirm gelesen hat. */
    val message: String,
    val detail: String = "",
    /** Freiwillig. Meistens leer — ein Fingertipp hilft genauso. */
    val comment: String = "",
    val area: String = "",
    val platform: String = "android",
    val appVersion: String = "",
    val osVersion: String = "",
    val deviceModel: String = "",
    /** RFC3339. Ein Absturz wird beim nächsten Start gemeldet, also früher. */
    val occurredAt: String = "",
)

/** Antwort von POST /api/v1/error-reports. Mehr als die Kennung braucht es nicht. */
@Serializable
data class ErrorReportDto(
    val id: Long = 0,
    val status: String = "new",
)

interface ErrorReportRepository {
    suspend fun send(input: ErrorReportInput): ErrorReportDto
}

class ApiErrorReportRepository(private val api: DorfApi) : ErrorReportRepository {
    override suspend fun send(input: ErrorReportInput): ErrorReportDto = api.errorReport(input)
}
