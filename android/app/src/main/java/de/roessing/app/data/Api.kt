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

    // --- Chat ----------------------------------------------------------------

    /** Ob der Chat eingerichtet ist — und wenn nicht, warum. */
    @GET("api/v1/chat")
    suspend fun chatStand(): ChatStandDto

    /**
     * Stellt eine Frage. Der Verlauf geht mit, das Backend haelt keine
     * Sitzung. Diese eine Anfrage darf laenger dauern als alle anderen —
     * siehe LANGE_ANTWORT_PFAD.
     */
    @POST("api/v1/chat")
    suspend fun chatFragen(@Body input: ChatFrageInput): ChatAntwortDto

    // --- Gerät für Push-Benachrichtigungen ----------------------------------

    @POST("api/v1/me/devices")
    suspend fun registerDevice(@Body input: DeviceInput)

    // DELETE mit Rumpf: die Kennung ist lang und hat in einer URL nichts zu
    // suchen (Logs, Verläufe). Das Backend nimmt sie auch als Abfrage an.
    @HTTP(method = "DELETE", path = "api/v1/me/devices", hasBody = true)
    suspend fun unregisterDevice(@Body input: DeviceInput)

    companion object {
        /**
         * Der Pfad, der laenger dauern darf als der Rest.
         *
         * Hinter ihm steht ein Sprachmodell, das nachdenkt und dabei mehrere
         * Werkzeuge des Dorfservers befragt; das Backend gibt sich dafuer
         * selbst bis zu 50 Sekunden. Die 20 Sekunden, die fuer jede andere
         * Anfrage reichlich sind, waeren hier ein Abbruch mitten in der
         * Antwort — und zwar reproduzierbar.
         *
         * Angehoben wird die Frist deshalb genau fuer diesen einen Pfad und
         * nicht fuer den Client: Eine Ortsliste, die nach 20 Sekunden nicht
         * da ist, kommt auch nach 70 nicht mehr, und so lange soll niemand
         * auf eine Fehlermeldung warten.
         */
        private const val LANGE_ANTWORT_PFAD = "/api/v1/chat"
        private const val LANGE_ANTWORT_SEKUNDEN = 70

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
            // Ein paar Endpunkte (Ideen) nehmen die Anfrage auch ohne an.
            TokenResult.LoggedOut -> request
            TokenResult.Unreachable ->
                throw IOException("Die Anmeldung ließ sich gerade nicht erneuern")
        }

        /**
         * Baut den API-Client. tokenProvider liefert die Tokenlage und wird
         * pro Request aufgerufen.
         */
        fun create(baseUrl: String, tokenProvider: suspend () -> TokenResult): DorfApi {
            val client = OkHttpClient.Builder()
                .connectTimeout(10, TimeUnit.SECONDS)
                .readTimeout(20, TimeUnit.SECONDS)
                .addInterceptor { chain ->
                    // OkHttp-Interceptoren sind synchron; der Tokenabruf ist
                    // lokal (Cache/Refresh) und läuft auf dem IO-Dispatcher.
                    val token = runBlocking { tokenProvider() }
                    val anfrage = autorisiert(chain.request(), token)
                    if (anfrage.url.encodedPath.endsWith(LANGE_ANTWORT_PFAD)) {
                        chain.withReadTimeout(LANGE_ANTWORT_SEKUNDEN, TimeUnit.SECONDS)
                            .proceed(anfrage)
                    } else {
                        chain.proceed(anfrage)
                    }
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
