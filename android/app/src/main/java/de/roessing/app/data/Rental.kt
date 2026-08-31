package de.roessing.app.data

import java.time.LocalDate
import java.util.Locale

/**
 * The "Maschinchenring": villagers lend equipment to villagers.
 *
 * This file is the vocabulary of the area. The shape of the data on the wire
 * lives in `RentalApi.kt`; whoever follows a change of the contract in
 * `docs/mieten-api.md` changes it there.
 *
 * **The app holds no rules of the rental business.** Whether a period is
 * free, whether a booking may still be cancelled, who may become a lender —
 * the rental platform decides and says so ([RentalAvailability.available],
 * [RentalBooking.canCancel], [RentalProfile.lenderStatus]). A button may be
 * greyed out because the server said so; the condition behind it does not
 * belong here. Otherwise web and app drift apart, and they do it first where
 * it hurts.
 */

/**
 * A period of whole days, **half open**: [start] belongs to it, [end] does
 * not — [end] is the day the device comes back.
 *
 * That is the contract's own definition (`docs/mieten-api.md`, section 1),
 * and it is the one place where a misunderstanding costs a whole day: a
 * booking from the 5th to the 7th occupies the 5th and the 6th, and somebody
 * else may start on the 7th. Everything the person sees is therefore built
 * from [lastDay], never from [end].
 */
data class RentalPeriod(val start: LocalDate, val end: LocalDate) {
    /** The last day the device is out — [end] minus one. */
    val lastDay: LocalDate get() = end.minusDays(1)

    /** How many days the device is out. */
    val days: Int get() = (end.toEpochDay() - start.toEpochDay()).toInt()

    /** A period whose end does not lie after its start is not one. */
    val isValid: Boolean get() = end.isAfter(start)

    /** "5. September 2026" or "5.–6. September 2026" or across months. */
    val text: String
        get() {
            val last = lastDay
            return when {
                start == last -> "${day(start)} ${month(start)} ${start.year}"
                start.year == last.year && start.month == last.month ->
                    "${day(start)}–${day(last)} ${month(start)} ${start.year}"

                start.year == last.year ->
                    "${day(start)} ${month(start)} – ${day(last)} ${month(last)} ${last.year}"

                else ->
                    "${day(start)} ${month(start)} ${start.year} – " +
                        "${day(last)} ${month(last)} ${last.year}"
            }
        }

    private fun day(date: LocalDate) = "${date.dayOfMonth}."

    private fun month(date: LocalDate) = MONTHS[date.monthValue - 1]

    companion object {
        /**
         * The period for a span of days somebody picked, both ends included.
         *
         * The calendar hands out the days a person wants the device; the
         * contract wants the day it comes back. That conversion happens here
         * and nowhere else.
         */
        fun ofPickedDays(first: LocalDate, last: LocalDate): RentalPeriod =
            RentalPeriod(first, last.plusDays(1))

        /** German month names, so device and test read the same. */
        private val MONTHS = arrayOf(
            "Januar", "Februar", "März", "April", "Mai", "Juni",
            "Juli", "August", "September", "Oktober", "November", "Dezember",
        )
    }
}

/** A device in the list, in the search results and in the detail sheet. */
data class RentalDevice(
    val id: String,
    val name: String,
    /** Markdown from the server — see [markdownAsPlainText]. May be null. */
    val description: String?,
    /** Euro per day. Null means: this tariff does not exist. Never zero. */
    val pricePerDay: Double?,
    val pricePerWeekend: Double?,
    val pricePerWeek: Double?,
    val deposit: Double?,
    val tags: List<String>,
    /** Ready-made, signed address of a picture. Null means: no picture. */
    val thumbnailUrl: String?,
    /** The manufacturer's page, if the platform knows one. */
    val productUrl: String?,
    /** The same device on the web version of the rental platform. */
    val webUrl: String?,
)

/** One picture of a device. */
data class RentalImage(val id: String, val url: String, val isThumbnail: Boolean)

/** A device with all its pictures, as the detail route delivers it. */
data class RentalDeviceDetail(val device: RentalDevice, val images: List<RentalImage>)

/**
 * A period in which a device is not to be had.
 *
 * All three [status] values mean the same for the calendar: taken. The
 * distinction is for drawing only — it is **not** a hint that "pending" might
 * still be available.
 */
data class RentalOccupancy(
    val deviceId: String?,
    val setId: String?,
    val period: RentalPeriod,
    val status: OccupancyStatus,
)

enum class OccupancyStatus {
    /** Somebody has asked, the owner has not decided. Taken all the same. */
    PENDING,

    /** Confirmed. */
    APPROVED,

    /** The owner needs it themselves. */
    BLOCKED,

    /** A value this version of the app does not know. Taken all the same. */
    UNKNOWN,
}

/**
 * The server's answer to "is it free in my week?".
 *
 * [reason] is a machine-readable token (`occupied`), not a sentence — the
 * platform deliberately does not say who or what is in the way.
 */
data class RentalAvailability(
    val period: RentalPeriod,
    val available: Boolean,
    val reason: String?,
)

/** The state of a booking, as the contract fixes it. */
enum class BookingStatus {
    /** Asked, the owner has not decided. */
    PENDING,
    APPROVED,
    REJECTED,
    CANCELLED,

    /** A value this version of the app does not know. */
    UNKNOWN,
}

/**
 * Where the device is picked up. Only present once the owner has approved,
 * and only if they left an address.
 *
 * This is the one place in the whole interface carrying another person's
 * address and telephone number. It is not cached and shown nowhere else.
 */
data class RentalPickup(val address: String?, val phone: String?)

/** One of my bookings. */
data class RentalBooking(
    val id: String,
    val deviceId: String?,
    val setId: String?,
    /** The device's name — or the set's name for a set booking. */
    val deviceName: String,
    val period: RentalPeriod,
    val status: BookingStatus,
    /** What the server literally wrote, for a state we do not know. */
    val rawStatus: String,
    val notes: String?,
    /** Whether cancelling has a prospect of succeeding. The server decides. */
    val canCancel: Boolean,
    val pickup: RentalPickup?,
)

/** Whether somebody may offer devices. Decided over there, by hand. */
enum class LenderStatus { NONE, PENDING, APPROVED, UNKNOWN }

/** The own account in the rental platform. */
data class RentalProfile(
    val name: String?,
    val email: String?,
    val phone: String?,
    val addressStreet: String?,
    val addressZip: String?,
    val addressCity: String?,
    val lender: Boolean,
    val lenderStatus: LenderStatus,
    val profileComplete: Boolean,
    /** What the server misses. The app never works this out itself. */
    val missingFields: List<String>,
)

/**
 * A booking request.
 *
 * The three personal fields are the exception, not the rule: normally the
 * server takes name and telephone from the profile. They are filled in only
 * after the server has said that it misses them — see
 * [RentalErrorCode.PROFILE_INCOMPLETE].
 */
data class BookingRequest(
    val deviceId: String,
    val period: RentalPeriod,
    val notes: String? = null,
    val firstName: String? = null,
    val lastName: String? = null,
    val phone: String? = null,
)

/**
 * The stable error codes of the rental platform.
 *
 * The app branches on the code, never on the message — that is the contract's
 * rule and the reason the wording over there can change without breaking
 * anything here.
 */
enum class RentalErrorCode {
    BAD_REQUEST,
    INVALID_PERIOD,
    PROFILE_INCOMPLETE,

    /** No token, or an expired one → ordinary sign-in. */
    UNAUTHORIZED,

    /**
     * The token does not name the rental platform as an audience → the person
     * has to sign in again. See [de.roessing.app.auth.RentalAudience].
     */
    TOKEN_AUDIENCE,
    FORBIDDEN,
    NOT_A_LENDER,
    NOT_FOUND,

    /** The period was taken in the meantime — the ordinary race. */
    OCCUPIED,
    CONFLICT,
    RATE_LIMITED,
    INTERNAL,

    /** A code this version of the app does not know. */
    UNKNOWN,
}

/**
 * The rental platform refused, and named a reason.
 *
 * [message] is German and written over there; it may be shown as it stands.
 * [missingFields] is filled for [RentalErrorCode.PROFILE_INCOMPLETE] only.
 */
class RentalApiException(
    val code: RentalErrorCode,
    override val message: String?,
    val missingFields: List<String> = emptyList(),
) : RuntimeException(message)

/**
 * What the app can do with the rental platform.
 *
 * Reading needs no sign-in — the device list is public on their website as
 * well, which is what lets the area work before anybody signs in. Only the
 * personal calls carry a token.
 *
 * The lender's side of the contract (routes 13 to 19) is deliberately absent:
 * creating and maintaining devices happens in the chat and the web version of
 * the rental platform, and it is meant to stay there.
 */
interface RentalRepository {
    /** All active devices, sorted by name. No sign-in. */
    suspend fun devices(): List<RentalDevice>

    /** One device with its pictures. No sign-in. */
    suspend fun device(id: String): RentalDeviceDetail

    /** Hybrid search on the server. Its ranking, not ours. No sign-in. */
    suspend fun search(query: String): List<RentalDevice>

    /** The taken periods of one device, for the calendar. No sign-in. */
    suspend fun occupancy(deviceId: String): List<RentalOccupancy>

    /** Is the period free? The server decides. No sign-in. */
    suspend fun availability(deviceId: String, period: RentalPeriod): RentalAvailability

    /** The own account. Signed in. */
    suspend fun profile(): RentalProfile

    /** My bookings, oldest start first. Signed in. */
    suspend fun myBookings(): List<RentalBooking>

    /** Ask for a device. The owner decides afterwards. Signed in. */
    suspend fun book(request: BookingRequest): RentalBooking

    /** Withdraw one of my bookings. Signed in. */
    suspend fun cancel(bookingId: String)
}

/**
 * An empty rental platform. The default for user interfaces that are built
 * without one (tests about something else entirely) — it fetches nothing and
 * claims nothing.
 */
object NoRental : RentalRepository {
    override suspend fun devices(): List<RentalDevice> = emptyList()

    override suspend fun device(id: String): RentalDeviceDetail =
        throw RentalApiException(RentalErrorCode.NOT_FOUND, null)

    override suspend fun search(query: String): List<RentalDevice> = emptyList()

    override suspend fun occupancy(deviceId: String): List<RentalOccupancy> = emptyList()

    override suspend fun availability(deviceId: String, period: RentalPeriod) =
        RentalAvailability(period, available = false, reason = null)

    override suspend fun profile(): RentalProfile =
        throw RentalApiException(RentalErrorCode.UNAUTHORIZED, null)

    override suspend fun myBookings(): List<RentalBooking> = emptyList()

    override suspend fun book(request: BookingRequest): RentalBooking =
        throw RentalApiException(RentalErrorCode.UNAUTHORIZED, null)

    override suspend fun cancel(bookingId: String) =
        throw RentalApiException(RentalErrorCode.UNAUTHORIZED, null)
}

/**
 * Markdown as readable plain text.
 *
 * Device descriptions arrive as Markdown. Showing them raw, with asterisks
 * and hash marks, is not an option; a full renderer is a library this app
 * does not have. So the app does the third thing the contract allows and
 * shows the text **deliberately as plain text**: emphasis marks fall away,
 * headings become plain lines, list markers become bullets, and a link keeps
 * its label.
 *
 * A pure function, so the whole conversion is covered by plain unit tests.
 */
fun markdownAsPlainText(markdown: String): String = markdown
    .lineSequence()
    .map { line ->
        var text = line.trim()
        // Headings: the hash marks are Markdown's, the words are the author's.
        text = text.removePrefix("#").removePrefix("#").removePrefix("#")
            .removePrefix("#").removePrefix("#").removePrefix("#").trim()
        // List markers become a bullet that survives being read aloud.
        text = LIST_MARKER.replace(text) { "• " }
        // Links keep their label; the address would only be noise on screen.
        text = LINK.replace(text) { it.groupValues[1] }
        // Emphasis, bold and inline code lose their marks, keep their words.
        text = text.replace("**", "").replace("__", "")
        text = EMPHASIS.replace(text) { it.groupValues[1] }
        text = text.replace("`", "")
        text
    }
    .joinToString("\n")
    .replace(Regex("\n{3,}"), "\n\n")
    .trim()

/**
 * A price in euro, as it is written in German.
 *
 * The platform sends plain numbers (`25`, `12.5`) and no currency — it is
 * always euro. Whole amounts lose their decimals, the rest keep two: "25 €",
 * "12,50 €". This formats **one** tariff and never adds tariffs up: which one
 * applies for which duration is nowhere laid down, and an invented rule here
 * would be the very drift between web and app the contract warns about.
 */
fun euroText(value: Double): String {
    val rounded = Math.round(value * 100.0) / 100.0
    return if (rounded == Math.floor(rounded)) {
        "${rounded.toLong()} €"
    } else {
        String.format(Locale.GERMANY, "%.2f €", rounded)
    }
}

private val LIST_MARKER = Regex("^[-*+]\\s+")
private val LINK = Regex("\\[([^]]*)]\\([^)]*\\)")
private val EMPHASIS = Regex("(?<![\\p{L}\\d])[*_]([^*_]+)[*_](?![\\p{L}\\d])")
