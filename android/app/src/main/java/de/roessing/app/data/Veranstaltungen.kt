package de.roessing.app.data

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import retrofit2.http.GET
import java.util.concurrent.TimeUnit

/**
 * „Was ist los in Rössing" — die Veranstaltungen kommen von der Website.
 *
 * Gepflegt werden sie dort (`src/content/events/` auf rössing.de) und nur
 * dort; die Website legt sie beim Bauen zusätzlich als `/events.json` ab.
 * Damit gibt es keine zweite Pflegestelle, keine neue Verwaltungsoberfläche
 * und im Dorf-Backend nichts, was veralten könnte.
 *
 * Wichtig: Diese Abfrage geht an einen **anderen** Server als die Dorf-API
 * und läuft deshalb über einen eigenen, schlichten Client — **ohne** das
 * Zugangstoken. Die Website ist öffentlich und hat mit unserer Anmeldung
 * nichts zu tun.
 */

/** Ort einer Veranstaltung, wie ihn `/events.json` liefert. */
@Serializable
data class OrtDto(
    val name: String = "",
    val address: String = "",
    val lat: Double? = null,
    val lon: Double? = null,
)

/** Veranstalter einer Veranstaltung. */
@Serializable
data class VeranstalterDto(val name: String = "")

/** Eine Veranstaltung aus `/events.json`. */
@Serializable
data class VeranstaltungDto(
    val id: String = "",
    val name: String = "",
    val description: String = "",
    /**
     * Ganztägig nur das Datum (`2026-08-17`), sonst die Ortszeit mit Offset
     * (`2026-08-20T18:00:00+02:00`).
     */
    val start: String = "",
    val end: String? = null,
    val allDay: Boolean = false,
    /** Externe Primärquelle, falls `external`, sonst die Seite auf rössing.de. */
    val url: String = "",
    val external: Boolean = false,
    val location: OrtDto? = null,
    val organizer: VeranstalterDto? = null,
    val image: String? = null,
)

@Serializable
data class VeranstaltungenFeedDto(
    val version: Int = 1,
    val generatedAt: String = "",
    val events: List<VeranstaltungDto> = emptyList(),
)

interface VeranstaltungenRepository {
    /** Holt die Liste, wie sie beim letzten Bauen der Website entstanden ist. */
    suspend fun kommende(): List<VeranstaltungDto>
}

interface WebsiteApi {
    @GET("events.json")
    suspend fun events(): VeranstaltungenFeedDto

    companion object {
        private val json = Json {
            ignoreUnknownKeys = true
            coerceInputValues = true
            explicitNulls = false
        }

        fun create(baseUrl: String): WebsiteApi {
            // Bewusst ohne Interceptor: An die Website geht kein Token.
            val client = OkHttpClient.Builder()
                .connectTimeout(10, TimeUnit.SECONDS)
                .readTimeout(20, TimeUnit.SECONDS)
                .build()
            return Retrofit.Builder()
                .baseUrl(if (baseUrl.endsWith("/")) baseUrl else "$baseUrl/")
                .client(client)
                .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
                .build()
                .create(WebsiteApi::class.java)
        }
    }
}

class WebsiteVeranstaltungenRepository(private val api: WebsiteApi) : VeranstaltungenRepository {
    override suspend fun kommende(): List<VeranstaltungDto> = api.events().events
}

/**
 * Ein leerer Kalender. Vorgabe für Oberflächen, die ohne Veranstaltungen
 * aufgebaut werden (etwa in Tests, die etwas ganz anderes prüfen) — er ruft
 * nichts ab und behauptet nichts.
 */
object KeineVeranstaltungen : VeranstaltungenRepository {
    override suspend fun kommende(): List<VeranstaltungDto> = emptyList()
}
