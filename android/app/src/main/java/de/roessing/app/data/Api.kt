package de.roessing.app.data

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.POST
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
