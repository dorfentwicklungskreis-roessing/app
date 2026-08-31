package de.roessing.app.data

import de.roessing.app.auth.RentalSignIn
import de.roessing.app.auth.TokenResult
import kotlinx.coroutines.runBlocking
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
 * The wire shape of `mieten.xn--rssing-wxa.de`, exactly as `docs/mieten-api.md`
 * fixes it — and the only file that changes when that contract changes.
 *
 * Everything else in this area speaks the vocabulary of `Rental.kt`.
 *
 * Three properties of this client are deliberate:
 *
 *  - **It talks to the rental platform directly.** The Go backend of the
 *    village app is not a relay and knows nothing about renting. This is the
 *    same arrangement the events use with the website — a second server, its
 *    own small client. The one difference is that a token goes along here.
 *  - **Reading carries no token.** Device list, detail, search, occupancy and
 *    availability are public over there, the same way they are public on
 *    their website. That is what lets the area work before anybody signs in,
 *    and it keeps the reading half out of every question about audiences and
 *    expiry. Only the personal calls are marked with [AUTH_MARKER].
 *  - **The app branches on `error.code`, never on `error.message`.** The
 *    message is German and may be shown; the code is what decisions hang on.
 */

/**
 * Marks the calls that need an access token. The interceptor swaps it for the
 * real `Authorization` header; without it no token leaves the device.
 */
const val AUTH_MARKER = "X-Dorf-Auth"

/** How many results the search asks for. The contract allows 1 to 20. */
const val SEARCH_LIMIT = 20

// --- The wire shape ---------------------------------------------------------

/** A device, as routes 1, 2 and 3 deliver it. */
@Serializable
data class ItemDto(
    val id: String = "",
    val name: String = "",
    /** Markdown. Often null. */
    val description: String? = null,
    val pricePerDay: Double? = null,
    val pricePerWeekend: Double? = null,
    val pricePerWeek: Double? = null,
    val deposit: Double? = null,
    val tags: List<String> = emptyList(),
    val thumbnailUrl: String? = null,
    val productUrl: String? = null,
    val webUrl: String? = null,
    /** Only in the search results: 0 to 1, for sorting, not for showing. */
    val score: Double? = null,
    /** Only in the detail route. */
    val images: List<ImageDto> = emptyList(),
)

@Serializable
data class ImageDto(
    val id: String = "",
    val url: String = "",
    val isThumbnail: Boolean = false,
)

/** Route 1: the list comes in an envelope, never as a bare array. */
@Serializable
data class ItemsDto(val items: List<ItemDto> = emptyList())

/** Route 2. */
@Serializable
data class ItemDetailDto(val item: ItemDto = ItemDto())

/** Route 3. */
@Serializable
data class SearchDto(val results: List<ItemDto> = emptyList())

/** Route 5. `reason` is a token (`occupied`), not a sentence. */
@Serializable
data class AvailabilityDto(val available: Boolean = false, val reason: String? = null)

/** Route 6. */
@Serializable
data class OccupancyDto(val periods: List<OccupancyPeriodDto> = emptyList())

@Serializable
data class OccupancyPeriodDto(
    val deviceId: String? = null,
    val setId: String? = null,
    val startDate: String = "",
    val endDate: String = "",
    /** `pending`, `approved` or `blocked` — all three mean taken. */
    val status: String = "",
)

/** Route 7 and 8. */
@Serializable
data class ProfileEnvelopeDto(val profile: ProfileDataDto = ProfileDataDto())

@Serializable
data class ProfileDataDto(
    val name: String? = null,
    val email: String? = null,
    val phone: String? = null,
    val addressStreet: String? = null,
    val addressZip: String? = null,
    val addressCity: String? = null,
    val lender: Boolean = false,
    val lenderStatus: String = "none",
    val profileComplete: Boolean = false,
    val missingFields: List<String> = emptyList(),
)

/** Route 10 and 11. */
@Serializable
data class MietenBookingDto(
    val id: String = "",
    val deviceId: String? = null,
    val setId: String? = null,
    val deviceName: String = "",
    val startDate: String = "",
    /** The return day — it does **not** belong to the period. */
    val endDate: String = "",
    val status: String = "",
    val notes: String? = null,
    val canCancel: Boolean = false,
    val pickup: PickupDto? = null,
)

@Serializable
data class PickupDto(val address: String? = null, val phone: String? = null)

@Serializable
data class MyBookingsDto(val bookings: List<MietenBookingDto> = emptyList())

@Serializable
data class BookingEnvelopeDto(val booking: MietenBookingDto = MietenBookingDto())

/** The body of route 11. Null fields are left out, not sent as null. */
@Serializable
data class BookingInputDto(
    val deviceId: String,
    val startDate: String,
    val endDate: String,
    val firstName: String? = null,
    val lastName: String? = null,
    val phone: String? = null,
    val notes: String? = null,
)

/** Every error of the platform has this shape. */
@Serializable
data class ApiErrorEnvelopeDto(val error: ApiErrorDataDto = ApiErrorDataDto())

@Serializable
data class ApiErrorDataDto(
    val code: String = "",
    val message: String = "",
    val missingFields: List<String> = emptyList(),
)

// --- The client -------------------------------------------------------------

interface MietenApi {
    @GET("api/v1/items")
    suspend fun items(): ItemsDto

    @GET("api/v1/items/{id}")
    suspend fun item(@Path("id") id: String): ItemDetailDto

    @GET("api/v1/search")
    suspend fun search(
        @Query("q") query: String,
        @Query("limit") limit: Int = SEARCH_LIMIT,
    ): SearchDto

    @GET("api/v1/availability")
    suspend fun availability(
        @Query("deviceId") deviceId: String,
        @Query("startDate") startDate: String,
        @Query("endDate") endDate: String,
    ): AvailabilityDto

    @GET("api/v1/occupancy")
    suspend fun occupancy(@Query("deviceId") deviceId: String): OccupancyDto

    @Headers("$AUTH_MARKER: required")
    @GET("api/v1/me")
    suspend fun me(): ProfileEnvelopeDto

    @Headers("$AUTH_MARKER: required")
    @GET("api/v1/bookings/mine")
    suspend fun myBookings(): MyBookingsDto

    @Headers("$AUTH_MARKER: required")
    @POST("api/v1/bookings")
    suspend fun book(@Body input: BookingInputDto): BookingEnvelopeDto

    @Headers("$AUTH_MARKER: required")
    @POST("api/v1/bookings/{id}/cancel")
    suspend fun cancel(@Path("id") bookingId: String)

    companion object {
        private val json = Json {
            ignoreUnknownKeys = true
            coerceInputValues = true
            // Leaving a field out is how the contract says "take it from my
            // profile"; sending an explicit null would be something else.
            explicitNulls = false
        }

        /**
         * Attaches the authorisation to a marked request — or lets an
         * unmarked one pass untouched.
         *
         * The third case is the important one, and it is the same reasoning
         * as in `DorfApi`: if somebody is signed in but the token could not
         * be renewed just now, the request does **not** go out without the
         * header. It would come back 401, and the app would read that as
         * "signed out" — wrong, and it costs a sign-in that still holds. An
         * IOException is the truth: it is the network.
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
 * @param signIn how this device's sign-in relates to the rental platform.
 *   Asked before every personal call, so that a token issued before the
 *   changeover produces a sentence people can act on instead of a 401 that
 *   looks like a defect.
 */
class MietenRentalRepository(
    private val api: MietenApi,
    private val signIn: suspend () -> RentalSignIn,
) : RentalRepository {

    override suspend fun devices(): List<RentalDevice> =
        translate { api.items().items.map { it.asDevice() } }

    override suspend fun device(id: String): RentalDeviceDetail = translate {
        val item = api.item(id).item
        RentalDeviceDetail(
            device = item.asDevice(),
            images = item.images
                .filter { it.url.isNotBlank() }
                .map { RentalImage(it.id, it.url, it.isThumbnail) },
        )
    }

    override suspend fun search(query: String): List<RentalDevice> = translate {
        // The ranking of the hybrid search is the server's business — the app
        // shows the order it gets and does not sort it again.
        api.search(query).results.map { it.asDevice() }
    }

    override suspend fun occupancy(deviceId: String): List<RentalOccupancy> = translate {
        api.occupancy(deviceId).periods.mapNotNull { it.asOccupancy() }
    }

    override suspend fun availability(
        deviceId: String,
        period: RentalPeriod,
    ): RentalAvailability = translate {
        val answer = api.availability(
            deviceId = deviceId,
            startDate = period.start.toString(),
            endDate = period.end.toString(),
        )
        RentalAvailability(period, answer.available, answer.reason)
    }

    override suspend fun profile(): RentalProfile = personal {
        api.me().profile.asProfile()
    }

    override suspend fun myBookings(): List<RentalBooking> = personal {
        api.myBookings().bookings.mapNotNull { it.asBooking() }
    }

    override suspend fun book(request: BookingRequest): RentalBooking = personal {
        val answer = api.book(
            BookingInputDto(
                deviceId = request.deviceId,
                startDate = request.period.start.toString(),
                endDate = request.period.end.toString(),
                firstName = request.firstName?.trim()?.takeIf { it.isNotEmpty() },
                lastName = request.lastName?.trim()?.takeIf { it.isNotEmpty() },
                phone = request.phone?.trim()?.takeIf { it.isNotEmpty() },
                notes = request.notes?.trim()?.takeIf { it.isNotEmpty() },
            ),
        )
        answer.booking.asBooking()
            ?: throw RentalApiException(RentalErrorCode.INTERNAL, null)
    }

    override suspend fun cancel(bookingId: String) = personal {
        api.cancel(bookingId)
    }

    /**
     * A personal call: first the sign-in, then the request.
     *
     * Asking beforehand saves a pointless round trip on exactly the devices
     * that have the problem — a phone that kept its tokens across the update
     * carries one without the rental platform in its `aud`, and every request
     * with it comes back 401. Where the token cannot be read the request goes
     * out anyway and the server's answer decides ([translate]).
     */
    private suspend fun <T> personal(block: suspend () -> T): T {
        when (signIn()) {
            RentalSignIn.VALID -> Unit
            RentalSignIn.STALE -> throw RentalApiException(RentalErrorCode.TOKEN_AUDIENCE, null)
            RentalSignIn.MISSING -> throw RentalApiException(RentalErrorCode.UNAUTHORIZED, null)
            RentalSignIn.UNREACHABLE ->
                throw IOException("Die Anmeldung ließ sich gerade nicht erneuern")
        }
        return translate(block)
    }

    /**
     * Turns the platform's refusals into [RentalApiException].
     *
     * Anything that is not an answer of the platform — a broken connection,
     * a timeout — stays what it is and reaches the ViewModel as "not
     * reachable right now".
     */
    private suspend fun <T> translate(block: suspend () -> T): T = try {
        block()
    } catch (e: HttpException) {
        throw e.asRentalException()
    }
}

/** Reads errors leniently: a broken body must not hide the status behind it. */
private val errorJson = Json {
    ignoreUnknownKeys = true
    coerceInputValues = true
}

/** Reads `{"error": {...}}` out of a failed answer. */
internal fun HttpException.asRentalException(): RentalApiException {
    val body = runCatching { response()?.errorBody()?.string() }.getOrNull()
    val error = body?.let {
        runCatching { errorJson.decodeFromString<ApiErrorEnvelopeDto>(it).error }.getOrNull()
    }
    return RentalApiException(
        code = rentalErrorCode(error?.code, code()),
        message = error?.message?.takeIf { it.isNotBlank() },
        missingFields = error?.missingFields.orEmpty(),
    )
}

/**
 * The code of the answer, or a sensible one derived from the HTTP status.
 *
 * The fallback matters: a proxy or a gateway can answer 502 without the
 * platform's envelope ever being written, and the area must still say
 * something better than nothing.
 */
internal fun rentalErrorCode(code: String?, httpStatus: Int): RentalErrorCode = when (code) {
    "bad_request" -> RentalErrorCode.BAD_REQUEST
    "invalid_period" -> RentalErrorCode.INVALID_PERIOD
    "profile_incomplete" -> RentalErrorCode.PROFILE_INCOMPLETE
    "unauthorized" -> RentalErrorCode.UNAUTHORIZED
    "token_audience" -> RentalErrorCode.TOKEN_AUDIENCE
    "forbidden" -> RentalErrorCode.FORBIDDEN
    "not_a_lender" -> RentalErrorCode.NOT_A_LENDER
    "not_found" -> RentalErrorCode.NOT_FOUND
    "occupied" -> RentalErrorCode.OCCUPIED
    "conflict" -> RentalErrorCode.CONFLICT
    "rate_limited" -> RentalErrorCode.RATE_LIMITED
    "internal" -> RentalErrorCode.INTERNAL
    else -> when (httpStatus) {
        // A 401 without a readable body is the ordinary "sign in" — never
        // the audience case, because that one always names itself.
        401 -> RentalErrorCode.UNAUTHORIZED
        403 -> RentalErrorCode.FORBIDDEN
        404 -> RentalErrorCode.NOT_FOUND
        409 -> RentalErrorCode.CONFLICT
        429 -> RentalErrorCode.RATE_LIMITED
        in 500..599 -> RentalErrorCode.INTERNAL
        else -> RentalErrorCode.UNKNOWN
    }
}

// --- Mapping: wire shape → vocabulary of the area ---------------------------

/** A date that cannot be read costs the one entry, never the whole list. */
private fun date(text: String): LocalDate? = runCatching { LocalDate.parse(text) }.getOrNull()

/** Blank strings are the wire's way of saying "nothing"; null is the app's. */
private fun String?.orNullIfBlank(): String? = this?.trim()?.takeIf { it.isNotEmpty() }

internal fun ItemDto.asDevice() = RentalDevice(
    id = id,
    name = name,
    description = description.orNullIfBlank(),
    // Prices are numbers in euro. A missing tariff is null and stays null —
    // it is not zero, and the app does not add the tariffs up: which one
    // applies for which duration is nowhere laid down.
    pricePerDay = pricePerDay,
    pricePerWeekend = pricePerWeekend,
    pricePerWeek = pricePerWeek,
    deposit = deposit,
    tags = tags.filter { it.isNotBlank() },
    thumbnailUrl = thumbnailUrl.orNullIfBlank(),
    productUrl = productUrl.orNullIfBlank(),
    webUrl = webUrl.orNullIfBlank(),
)

internal fun OccupancyPeriodDto.asOccupancy(): RentalOccupancy? {
    val from = date(startDate) ?: return null
    val to = date(endDate) ?: return null
    if (!to.isAfter(from)) return null
    return RentalOccupancy(
        deviceId = deviceId.orNullIfBlank(),
        setId = setId.orNullIfBlank(),
        period = RentalPeriod(from, to),
        status = when (status) {
            "pending" -> OccupancyStatus.PENDING
            "approved" -> OccupancyStatus.APPROVED
            "blocked" -> OccupancyStatus.BLOCKED
            else -> OccupancyStatus.UNKNOWN
        },
    )
}

internal fun MietenBookingDto.asBooking(): RentalBooking? {
    val from = date(startDate) ?: return null
    val to = date(endDate) ?: return null
    return RentalBooking(
        id = id,
        deviceId = deviceId.orNullIfBlank(),
        setId = setId.orNullIfBlank(),
        deviceName = deviceName,
        // Even a period the server wrote back to front must not turn into a
        // negative number of days on screen.
        period = RentalPeriod(from, if (to.isAfter(from)) to else from.plusDays(1)),
        status = when (status) {
            "pending" -> BookingStatus.PENDING
            "approved" -> BookingStatus.APPROVED
            "rejected" -> BookingStatus.REJECTED
            "cancelled" -> BookingStatus.CANCELLED
            else -> BookingStatus.UNKNOWN
        },
        rawStatus = status,
        notes = notes.orNullIfBlank(),
        canCancel = canCancel,
        // The pickup address only exists once the owner has approved. It is
        // taken as it comes and kept nowhere else.
        pickup = pickup
            ?.takeIf { it.address.orNullIfBlank() != null || it.phone.orNullIfBlank() != null }
            ?.let { RentalPickup(it.address.orNullIfBlank(), it.phone.orNullIfBlank()) },
    )
}

internal fun ProfileDataDto.asProfile() = RentalProfile(
    name = name.orNullIfBlank(),
    email = email.orNullIfBlank(),
    phone = phone.orNullIfBlank(),
    addressStreet = addressStreet.orNullIfBlank(),
    addressZip = addressZip.orNullIfBlank(),
    addressCity = addressCity.orNullIfBlank(),
    lender = lender,
    lenderStatus = when (lenderStatus) {
        "none" -> LenderStatus.NONE
        "pending" -> LenderStatus.PENDING
        "approved" -> LenderStatus.APPROVED
        else -> LenderStatus.UNKNOWN
    },
    profileComplete = profileComplete,
    missingFields = missingFields.filter { it.isNotBlank() },
)
