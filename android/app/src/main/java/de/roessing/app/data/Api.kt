package de.roessing.app.data

import de.roessing.app.auth.TokenResult
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.HTTP
import retrofit2.http.POST
import retrofit2.http.PUT
import retrofit2.http.Path
import retrofit2.http.Query
import java.io.IOException
import java.util.concurrent.TimeUnit

/**
 * Sieht jede gescheiterte Anfrage. Als Schnittstelle, damit die Datenschicht
 * den Melder nicht kennen muss — und der Melder ohne Netz prüfbar bleibt.
 */
interface AnfragenBeobachter {
    /** Der Server hat geantwortet, aber nicht mit Erfolg. */
    fun antwort(methode: String, pfad: String, code: Int)

    /** Die Anfrage kam gar nicht erst durch. */
    fun fehlschlag(methode: String, pfad: String, grund: String)
}

interface DorfApi {
    @GET("api/v1/me")
    suspend fun me(): MeDto

    @GET("api/v1/places")
    suspend fun places(): PlacesResponse

    @GET("api/v1/tasks/{id}/completions")
    suspend fun completions(@Path("id") taskId: Long): CompletionsResponse

    @POST("api/v1/tasks/{id}/completions")
    suspend fun complete(@Path("id") taskId: Long, @Body input: CompletionInput): CompletionDto

    @GET("api/v1/stats/leaderboard")
    suspend fun leaderboard(@Query("period") period: String): LeaderboardDto

    @PUT("api/v1/me/profile")
    suspend fun saveProfile(@Body input: ProfileInput): ProfileDto

    @GET("api/v1/members")
    suspend fun members(): MembersResponse

    // --- Vergabe der Pflegeaufgaben -----------------------------------------

    /** Trägt mich als Helfer:in für einen Ort ein (taskKind leer = alle Aufgaben). */
    @POST("api/v1/places/{id}/signup")
    suspend fun signup(@Path("id") placeId: Long, @Body input: SignupInput)

    @DELETE("api/v1/places/{id}/signup")
    suspend fun signoff(@Path("id") placeId: Long, @Query("taskKind") taskKind: String?)

    @GET("api/v1/me/notifications")
    suspend fun notifications(): NotificationsResponse

    @POST("api/v1/me/notifications/{id}/ack")
    suspend fun ackNotification(@Path("id") id: Long)

    /** Zusagen. Antwortet mit 409, wenn jemand anderes schneller war. */
    @POST("api/v1/assignments/{id}/claim")
    suspend fun claim(@Path("id") assignmentId: Long): AssignmentDto

    @POST("api/v1/assignments/{id}/release")
    suspend fun release(@Path("id") assignmentId: Long): AssignmentDto

    // --- Ideen-Sammlung ------------------------------------------------------

    /**
     * Reicht einen Wunsch ein („Was soll die App noch können?"). Derselbe
     * Eingang, den auch das Formular auf der Website benutzt; er ist bewusst
     * ohne Anmeldung erreichbar. Aus der App geht das Token trotzdem mit,
     * damit die Idee dem Konto zugeordnet wird.
     */
    @POST("api/v1/ideen")
    suspend fun idee(@Body input: IdeeInput): IdeeDto

    // --- Fehlerberichte ------------------------------------------------------

    /**
     * Schickt einen Fehlerbericht. Auch dieser Eingang ist bewusst ohne
     * Anmeldung erreichbar — wenn die Anmeldung klemmt, ist genau das der
     * Bericht, der fehlt. Ein Token geht mit, wenn es eines gibt.
     */
    @POST("api/v1/error-reports")
    suspend fun errorReport(@Body input: ErrorReportInput): ErrorReportDto

    // --- Gerät für Push-Benachrichtigungen ----------------------------------

    @POST("api/v1/me/devices")
    suspend fun registerDevice(@Body input: DeviceInput)

    // DELETE mit Rumpf: die Kennung ist lang und hat in einer URL nichts zu
    // suchen (Logs, Verläufe). Das Backend nimmt sie auch als Abfrage an.
    @HTTP(method = "DELETE", path = "api/v1/me/devices", hasBody = true)
    suspend fun unregisterDevice(@Body input: DeviceInput)

    companion object {
        private val json = Json {
            ignoreUnknownKeys = true
            coerceInputValues = true
            explicitNulls = false
            encodeDefaults = true
        }

        /**
         * Hängt die Autorisierung an eine Anfrage — oder bricht ab.
         *
         * Der dritte Fall ist der wichtige: Besteht eine Anmeldung, ließ sie
         * sich aber gerade nicht erneuern, geht die Anfrage **nicht** ohne
         * Kopfzeile hinaus. Sonst käme ein 401 zurück, und die App machte
         * daraus „nicht angemeldet" — falsch, und es kostet eine Anmeldung,
         * die noch gilt. Eine IOException ist die Wahrheit: Es ist ein
         * Netzproblem, und die Bereiche zeigen dafür längst „offline" an.
         *
         * Als reine Funktion, damit sie sich ohne Netz prüfen lässt.
         */
        fun autorisiert(request: Request, token: TokenResult): Request = when (token) {
            is TokenResult.Token ->
                request.newBuilder().header("Authorization", "Bearer ${token.value}").build()
            // Ein paar Endpunkte (Ideen, Fehlerberichte) nehmen die Anfrage
            // auch ohne an.
            TokenResult.LoggedOut -> request
            TokenResult.Unreachable ->
                throw IOException("Die Anmeldung ließ sich gerade nicht erneuern")
        }

        /**
         * Baut den API-Client. tokenProvider liefert die Tokenlage und wird
         * pro Request aufgerufen.
         *
         * [beobachter] bekommt jede gescheiterte Anfrage zu sehen — an genau
         * einer Stelle, damit kein Bereich daran denken muss. Was davon
         * wirklich eine Störung ist und was eine Regel, die greift,
         * entscheidet der Melder (siehe `errors/ErrorReporter.kt`).
         */
        fun create(
            baseUrl: String,
            beobachter: AnfragenBeobachter? = null,
            tokenProvider: suspend () -> TokenResult,
        ): DorfApi {
            val client = OkHttpClient.Builder()
                .connectTimeout(10, TimeUnit.SECONDS)
                .readTimeout(20, TimeUnit.SECONDS)
                .addInterceptor { chain ->
                    // Muss vor dem Token-Interceptor stehen, damit er auch
                    // dessen Aussetzer sieht.
                    val anfrage = chain.request()
                    val pfad = anfrage.url.encodedPath
                    val methode = anfrage.method
                    try {
                        val antwort = chain.proceed(anfrage)
                        if (!antwort.isSuccessful) {
                            beobachter?.antwort(methode, pfad, antwort.code)
                        }
                        antwort
                    } catch (e: java.io.IOException) {
                        beobachter?.fehlschlag(methode, pfad, e.message.orEmpty())
                        throw e
                    }
                }
                .addInterceptor { chain ->
                    // OkHttp-Interceptoren sind synchron; der Tokenabruf ist
                    // lokal (Cache/Refresh) und läuft auf dem IO-Dispatcher.
                    val token = runBlocking { tokenProvider() }
                    chain.proceed(autorisiert(chain.request(), token))
                }
                .build()
            return Retrofit.Builder()
                .baseUrl(if (baseUrl.endsWith("/")) baseUrl else "$baseUrl/")
                .client(client)
                .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
                .build()
                .create(DorfApi::class.java)
        }
    }
}
