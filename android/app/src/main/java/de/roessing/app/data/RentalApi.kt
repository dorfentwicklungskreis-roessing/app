package de.roessing.app.data

import de.roessing.app.auth.RentalSignIn
import de.roessing.app.auth.TokenResult
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import retrofit2.HttpException
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import retrofit2.http.Body
import retrofit2.http.GET
import retrofit2.http.Headers
import retrofit2.http.POST
import retrofit2.http.Path
import retrofit2.http.Query
import java.io.IOException
import java.time.LocalDate
import java.util.concurrent.TimeUnit

/**
 * The contract with `mieten.xn--rssing-wxa.de` — and the only place that
 * changes when it is nailed down.
 *
 * Everything else in this area speaks the vocabulary of `Rental.kt`. When the
 * rental platform publishes its interface (`docs/mieten-api.md`), the routes,
 * the field names and the mapping at the bottom of this file are what has to
 * follow; the repository interface, the ViewModel, the screens and their
 * tests stay untouched.
 *
 * Two properties of this client are deliberate:
 *
 *  - **It talks to the rental platform directly.** The Go backend of the
 *    village app is not a relay and knows nothing about renting. This is the
 *    same arrangement the events use with the website — a second server, its
 *    own small client. The one difference is that a token goes along here.
 *  - **Reading carries no token.** Device list, search and availability are
 *    public over there, the same way they are public on their website. That
 *    is what lets the area work before anybody signs in, and it keeps the
 *    reading half out of every question about audiences and expiry. Only the
 *    four personal calls are marked with [AUTH_MARKER].
 *
 * As long as `docs/mieten-api.md` is not on origin, the shapes below are our
 * assumption, and the fixtures under `android/e2e/fixtures/mieten/` are what
 * the tests run against. Every field has a default and unknown keys are
 * ignored, so a contract that differs only in extras still parses.
 */

/**
 * Marks the calls that need an access token. The interceptor swaps it for the
 * real `Authorization` header; without it no token leaves the device.
 */
const val AUTH_MARKER = "X-Dorf-Auth"

/** A device, as `GET /api/v1/items` delivers it. */
@Serializable
data class ItemDto(
    val id: String = "",
    val name: String = "",
    val description: String = "",
    val imageUrl: String? = null,
    val ownerName: String? = null,
    val location: String? = null,
    /** Finished text, formatted by the server ("15 € pro Tag"). */
    val priceText: String? = null,
    /** Finished text, formatted by the server ("100 € Kaution"). */
    val depositText: String? = null,
    val tags: List<String> = emptyList(),
    /** The server decides. Missing means "yes" — the list is public. */
    val bookable: Boolean = true,
    /** Plain German reason, set by the server when [bookable] is false. */
    val notBookableReason: String? = null,
)

/** The envelope of the list and search routes. */
@Serializable
data class ItemsDto(val items: List<ItemDto> = emptyList())

/** A booking — mine, or an occupied slot. */
@Serializable
data class BookingDto(
    val id: String = "",
    @SerialName("itemId") val itemId: String = "",
    @SerialName("itemName") val itemName: String = "",
    /** ISO date, `2026-09-04`. */
    val startDate: String = "",
    val endDate: String = "",
    /** `pending`, `approved`, `rejected`, `cancelled` … */
    val status: String = "",
    /** The state in plain German, worded over there. */
    val statusText: String = "",
    /** The server decides whether this booking may still be cancelled. */
    val cancellable: Boolean = false,
    /** Only filled in once approved — the platform guards addresses. */
    val pickupAddress: String? = null,
)

@Serializable
data class BookingsDto(val bookings: List<BookingDto> = emptyList())

/** The answer to "is this period free?". */
@Serializable
data class AvailabilityDto(
    val available: Boolean = false,
    /** Why not, in plain German. */
    val reason: String? = null,
    /** Occupied stretches around the request, for the detail sheet. */
    val blocked: List<BlockedDto> = emptyList(),
)

@Serializable
data class BlockedDto(val startDate: String = "", val endDate: String = "")

/** The body of `POST /api/v1/bookings`. */
@Serializable
data class BookingInput(
    val itemId: String,
    val startDate: String,
    val endDate: String,
    val note: String = "",
)

interface MietenApi {
    @GET("api/v1/items")
    suspend fun items(): ItemsDto

    @GET("api/v1/items")
    suspend fun search(@Query("q") text: String): ItemsDto

    @GET("api/v1/items/{id}/availability")
    suspend fun availability(
        @Path("id") itemId: String,
        @Query("start") start: String,
        @Query("end") end: String,
    ): AvailabilityDto

    @Headers("$AUTH_MARKER: required")
    @POST("api/v1/bookings")
    suspend fun book(@Body input: BookingInput): BookingDto

    @Headers("$AUTH_MARKER: required")
    @GET("api/v1/my-bookings")
    suspend fun myBookings(): BookingsDto

    @Headers("$AUTH_MARKER: required")
    @POST("api/v1/bookings/{id}/cancel")
    suspend fun cancel(@Path("id") bookingId: String): BookingDto

    companion object {
        private val json = Json {
            ignoreUnknownKeys = true
            coerceInputValues = true
            explicitNulls = false
            encodeDefaults = true
        }

        /**
         * Attaches the authorisation to a marked request — or lets an
         * unmarked one pass untouched.
         *
         * The third case is the important one, and it is the same reasoning
         * as in [DorfApi.autorisiert]: if somebody is signed in but the token
         * could not be renewed just now, the request does **not** go out
         * without the header. It would come back 401, and the app would read
         * that as "signed out" — wrong, and it costs a sign-in that still
         * holds. An IOException is the truth: it is the network.
         *
         * A pure function, so it can be checked without a network.
         */
        fun authorized(request: Request, token: TokenResult): Request {
            if (request.header(AUTH_MARKER) == null) return request
            val stripped = request.newBuilder().removeHeader(AUTH_MARKER)
            return when (token) {
                is TokenResult.Token ->
                    stripped.header("Authorization", "Bearer ${token.value}").build()

                TokenResult.LoggedOut -> stripped.build()
                TokenResult.Unreachable ->
                    throw IOException("Die Anmeldung ließ sich gerade nicht erneuern")
            }
        }

        fun create(baseUrl: String, tokenProvider: suspend () -> TokenResult): MietenApi {
            val client = OkHttpClient.Builder()
                .connectTimeout(10, TimeUnit.SECONDS)
                .readTimeout(20, TimeUnit.SECONDS)
                .addInterceptor { chain ->
                    // Only marked calls cost a token lookup; the public ones
                    // never touch the sign-in at all.
                    val request = chain.request()
                    if (request.header(AUTH_MARKER) == null) {
                        chain.proceed(request)
                    } else {
                        val token = runBlocking { tokenProvider() }
                        chain.proceed(authorized(request, token))
                    }
                }
                .build()
            return Retrofit.Builder()
                .baseUrl(if (baseUrl.endsWith("/")) baseUrl else "$baseUrl/")
                .client(client)
                .addConverterFactory(json.asConverterFactory("application/json".toMediaType()))
                .build()
                .create(MietenApi::class.java)
        }
    }
}

/**
 * The repository over the rental platform.
 *
 * @param api the HTTP client above
 * @param signIn how this device's sign-in relates to the rental platform;
 *   asked before every personal call, so that a token issued before the
 *   changeover produces a sentence people can act on instead of a 401.
 */
class MietenRentalRepository(
    private val api: MietenApi,
    private val signIn: suspend () -> RentalSignIn,
) : RentalRepository {

    override suspend fun devices(): List<Device> = api.items().items.map { it.asDevice() }

    override suspend fun search(text: String): List<Device> =
        api.search(text).items.map { it.asDevice() }

    override suspend fun availability(deviceId: String, period: Period): Availability = translate {
        api.availability(deviceId, period.from.toString(), period.to.toString())
            .asAvailability(period)
    }

    override suspend fun book(request: BookingRequest): Booking {
        requireSignIn()
        return translate {
            api.book(
                BookingInput(
                    itemId = request.deviceId,
                    startDate = request.period.from.toString(),
                    endDate = request.period.to.toString(),
                    note = request.message,
                ),
            ).asBooking() ?: throw IOException("Die Antwort auf die Buchung war unlesbar")
        }
    }

    override suspend fun myBookings(): List<Booking> {
        requireSignIn()
        return translate { api.myBookings().bookings.mapNotNull { it.asBooking() } }
    }

    override suspend fun cancel(bookingId: String): Booking {
        requireSignIn()
        return translate {
            api.cancel(bookingId).asBooking()
                ?: throw IOException("Die Antwort auf die Stornierung war unlesbar")
        }
    }

    /**
     * Refuses a personal call before it is made, when it cannot succeed.
     *
     * The distinction matters: "sign in again" is a request to the person,
     * "no connection" is a circumstance. Only the first belongs on a button.
     */
    private suspend fun requireSignIn() {
        when (signIn()) {
            RentalSignIn.VALID -> Unit
            RentalSignIn.STALE -> throw StaleAudienceException()
            RentalSignIn.MISSING -> throw NotSignedInException()
            RentalSignIn.UNREACHABLE ->
                throw IOException("Die Anmeldung ließ sich gerade nicht erneuern")
        }
    }

    /**
     * Turns the platform's answers into the exceptions this area speaks.
     *
     * A 401 on a call we believed to be signed in is the second half of the
     * stale-token story: on a device whose token is opaque we cannot see the
     * audience beforehand, so the server's no is what we go by.
     */
    private suspend fun <T> translate(block: suspend () -> T): T = try {
        block()
    } catch (e: HttpException) {
        when (e.code()) {
            400, 409 -> throw RentalRejectedException(reasonOf(e))
            401, 403 -> throw StaleAudienceException()
            else -> throw e
        }
    }

    private fun reasonOf(e: HttpException): String = runCatching {
        val raw = e.response()?.errorBody()?.string().orEmpty()
        Json { ignoreUnknownKeys = true }.decodeFromString<ApiErrorDto>(raw).error
    }.getOrNull()?.takeIf { it.isNotBlank() } ?: "Die Anfrage wurde abgelehnt."
}

// --- Mapping: wire shape → vocabulary of the area ---------------------------

/** A date that cannot be read costs the one entry, never the whole list. */
private fun date(text: String): LocalDate? = runCatching { LocalDate.parse(text) }.getOrNull()

internal fun ItemDto.asDevice() = Device(
    id = id,
    name = name,
    description = description,
    imageUrl = imageUrl?.takeIf { it.isNotBlank() },
    owner = ownerName?.takeIf { it.isNotBlank() },
    location = location?.takeIf { it.isNotBlank() },
    price = priceText?.takeIf { it.isNotBlank() },
    deposit = depositText?.takeIf { it.isNotBlank() },
    tags = tags.filter { it.isNotBlank() },
    bookable = bookable,
    notBookableReason = notBookableReason?.takeIf { it.isNotBlank() },
)

internal fun BookingDto.asBooking(): Booking? {
    val from = date(startDate) ?: return null
    val to = date(endDate) ?: from
    return Booking(
        id = id,
        deviceId = itemId,
        deviceName = itemName,
        period = Period(from, to),
        status = status,
        // No translation table here: inventing German for a state we do not
        // know would be a rule in the app. The raw state is honest enough.
        statusText = statusText.ifBlank { status },
        cancellable = cancellable,
        pickupAddress = pickupAddress?.takeIf { it.isNotBlank() },
    )
}

internal fun AvailabilityDto.asAvailability(period: Period) = Availability(
    period = period,
    free = available,
    hint = reason?.takeIf { it.isNotBlank() },
    occupied = blocked.mapNotNull { block ->
        val from = date(block.startDate) ?: return@mapNotNull null
        Period(from, date(block.endDate) ?: from)
    },
)
