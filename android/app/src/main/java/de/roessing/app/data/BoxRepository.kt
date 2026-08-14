package de.roessing.app.data

import kotlinx.serialization.json.Json
import retrofit2.HttpException

/**
 * Das Backend hat die Meldung wegen der Sperrfrist abgelehnt (HTTP 409).
 * retryAfter ist der Zeitpunkt (RFC3339), ab dem wieder gemeldet werden darf.
 */
class CompletionLockedException(val retryAfter: String?) :
    RuntimeException("Aufgabe ist noch gesperrt")

/**
 * Dünne Repository-Schicht über der API. Cache des letzten Standes hält das
 * ViewModel, damit bei Netzfehlern weiter alte Daten sichtbar bleiben.
 */
interface PlacesRepository {
    suspend fun me(): MeDto
    suspend fun places(): PlacesResponse
    suspend fun complete(taskId: Long, liters: Double?, note: String = ""): CompletionDto
    suspend fun completions(taskId: Long): List<CompletionDto>
}

class ApiPlacesRepository(private val api: DorfApi) : PlacesRepository {
    override suspend fun me(): MeDto = api.me()
    override suspend fun places(): PlacesResponse = api.places()

    // Der Spielschutz des Backends antwortet mit 409 und nennt im Körper den
    // Zeitpunkt, ab dem wieder gemeldet werden darf. Daraus wird hier ein
    // Domänenfehler, damit die Oberfläche nicht in HTTP-Codes denken muss.
    override suspend fun complete(taskId: Long, liters: Double?, note: String): CompletionDto =
        try {
            api.complete(taskId, CompletionInput(liters = liters, note = note))
        } catch (e: HttpException) {
            if (e.code() == 409) throw CompletionLockedException(retryAfterAus(e)) else throw e
        }

    private fun retryAfterAus(e: HttpException): String? = runCatching {
        val roh = e.response()?.errorBody()?.string() ?: return null
        Json { ignoreUnknownKeys = true }.decodeFromString<ApiErrorDto>(roh).retryAfter
    }.getOrNull()

    override suspend fun completions(taskId: Long): List<CompletionDto> =
        api.completions(taskId).completions
}

/**
 * Das Backend hat die Profiländerung abgewiesen (HTTP 400) und nennt den
 * Grund im Klartext. Der Grund ist für die Person gedacht — die Prüfung
 * sitzt bewusst im Backend, nicht in der App.
 */
class ProfileValidationException(val grund: String) : RuntimeException(grund)

/** Zugriff auf das eigene Profil und die Dorfbewohner-Liste. */
interface ProfileRepository {
    suspend fun profile(): ProfileDto
    suspend fun saveProfile(input: ProfileInput): ProfileDto

    /** Liefert die Liste und ob sie in der Verwaltungs-Sicht kam. */
    suspend fun members(): Pair<List<MemberDto>, Boolean>
}

class ApiProfileRepository(private val api: DorfApi) : ProfileRepository {
    override suspend fun profile(): ProfileDto = api.me().profile ?: ProfileDto()

    override suspend fun saveProfile(input: ProfileInput): ProfileDto =
        try {
            api.saveProfile(input)
        } catch (e: HttpException) {
            if (e.code() == 400) throw ProfileValidationException(fehlertextAus(e)) else throw e
        }

    override suspend fun members(): Pair<List<MemberDto>, Boolean> {
        val antwort = api.members()
        return antwort.members to antwort.adminView
    }

    private fun fehlertextAus(e: HttpException): String = runCatching {
        val roh = e.response()?.errorBody()?.string().orEmpty()
        Json { ignoreUnknownKeys = true }.decodeFromString<ApiErrorDto>(roh).error
    }.getOrNull()?.takeIf { it.isNotBlank() } ?: "Die Eingabe wurde abgelehnt."
}

/** Zugriff auf die Auswertungen (Rangliste). */
interface StatsRepository {
    suspend fun leaderboard(period: String): LeaderboardDto
}

class ApiStatsRepository(private val api: DorfApi) : StatsRepository {
    override suspend fun leaderboard(period: String): LeaderboardDto = api.leaderboard(period)
}
