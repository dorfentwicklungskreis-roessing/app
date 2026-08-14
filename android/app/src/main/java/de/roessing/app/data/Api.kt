package de.roessing.app.data

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
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

    // --- Verwaltung von Orten und Aufgaben ----------------------------------
    //
    // Alle diese Endpunkte verlangen die Rolle „admin" im Token. Geprüft wird
    // das im Backend (403 ohne Rolle) — die App blendet die Knöpfe nur
    // zusätzlich aus.

    @POST("api/v1/places")
    suspend fun ortAnlegen(@Body eingabe: PlaceEingabe): PlaceDto

    @PUT("api/v1/places/{id}")
    suspend fun ortAendern(@Path("id") id: Long, @Body eingabe: PlaceEingabe): PlaceDto

    @DELETE("api/v1/places/{id}")
    suspend fun ortLoeschen(@Path("id") id: Long)

    @POST("api/v1/places/{id}/tasks")
    suspend fun aufgabeAnlegen(@Path("id") placeId: Long, @Body eingabe: TaskEingabe): TaskDto

    @PUT("api/v1/tasks/{id}")
    suspend fun aufgabeAendern(@Path("id") id: Long, @Body eingabe: TaskEingabe): TaskDto

    @DELETE("api/v1/tasks/{id}")
    suspend fun aufgabeLoeschen(@Path("id") id: Long)

    // --- Ideen-Sammlung ------------------------------------------------------

    /**
     * Reicht einen Wunsch ein („Was soll die App noch können?"). Derselbe
     * Eingang, den auch das Formular auf der Website benutzt; er ist bewusst
     * ohne Anmeldung erreichbar. Aus der App geht das Token trotzdem mit,
     * damit die Idee dem Konto zugeordnet wird.
     */
    @POST("api/v1/ideen")
    suspend fun idee(@Body input: IdeeInput): IdeeDto

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
         * Baut den API-Client. tokenProvider liefert ein frisches Access-Token
         * (oder null, wenn nicht eingeloggt) und wird pro Request aufgerufen.
         */
        fun create(baseUrl: String, tokenProvider: suspend () -> String?): DorfApi {
            val client = OkHttpClient.Builder()
                .connectTimeout(10, TimeUnit.SECONDS)
                .readTimeout(20, TimeUnit.SECONDS)
                .addInterceptor { chain ->
                    // OkHttp-Interceptoren sind synchron; der Tokenabruf ist
                    // lokal (Cache/Refresh) und läuft auf dem IO-Dispatcher.
                    val token = runBlocking { tokenProvider() }
                    val req = if (token != null) {
                        chain.request().newBuilder()
                            .header("Authorization", "Bearer $token").build()
                    } else chain.request()
                    chain.proceed(req)
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
