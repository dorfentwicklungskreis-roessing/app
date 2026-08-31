package de.roessing.app.data

import java.time.LocalDate

/**
 * The "Maschinchenring": villagers lend equipment to villagers.
 *
 * This file describes what the user interface needs to know about the rental
 * platform, and nothing else. The shape of the data on the wire lives in
 * `RentalApi.kt`; whoever catches up with the contract of
 * `mieten.xn--rssing-wxa.de` changes it there and only there.
 *
 * **The app holds no rules.** Whether a device can be booked, whether a
 * period is free, whether a booking may still be cancelled — the rental
 * platform decides and says so. That is why [Device] and [Booking] carry the
 * plain German wording next to the yes/no: the app shows that wording instead
 * of inventing a reason of its own that is wrong by tomorrow.
 */

/** A period of whole days — the rental platform does not count in hours. */
data class Period(val from: LocalDate, val to: LocalDate) {
    /** "4.–6. September 2026", or a single day as "4. September 2026". */
    val text: String
        get() = when {
            from == to -> "${from.dayOfMonth}. ${MONTHS[from.monthValue - 1]} ${from.year}"
            from.year == to.year && from.month == to.month ->
                "${from.dayOfMonth}.–${to.dayOfMonth}. ${MONTHS[from.monthValue - 1]} ${from.year}"

            else -> "${from.dayOfMonth}. ${MONTHS[from.monthValue - 1]} ${from.year} – " +
                "${to.dayOfMonth}. ${MONTHS[to.monthValue - 1]} ${to.year}"
        }

    /** Number of days, both ends included. */
    val days: Int get() = (to.toEpochDay() - from.toEpochDay()).toInt() + 1

    /** A period whose end lies before its start is a slip of the finger. */
    val isSane: Boolean get() = !to.isBefore(from)
}

/** German month names, wired in so that device and test show the same thing. */
private val MONTHS = arrayOf(
    "Januar", "Februar", "März", "April", "Mai", "Juni",
    "Juli", "August", "September", "Oktober", "November", "Dezember",
)

/** A device as it appears in the list and in the detail sheet. */
data class Device(
    val id: String,
    val name: String,
    val description: String,
    /** Full address of a picture, or null. */
    val imageUrl: String?,
    /** Who owns it — without a street address; that arrives after approval. */
    val owner: String?,
    /** Rough location ("Rössing, Hinter den Höfen"), if given. */
    val location: String?,
    /** The price as finished text from the server — the app does no arithmetic. */
    val price: String?,
    /** The deposit as finished text from the server. */
    val deposit: String?,
    /** Labels the rental platform keeps for sorting things into drawers. */
    val tags: List<String>,
    /** The server says so. The app does not know the condition. */
    val bookable: Boolean,
    /** Why not — plain German from the server, otherwise null. */
    val notBookableReason: String?,
)

/** A booking: either one of mine, or an occupied period. */
data class Booking(
    val id: String,
    val deviceId: String,
    /** Name of the device, so the list reads without a second round trip. */
    val deviceName: String,
    val period: Period,
    /** Raw state of the rental platform (`pending`, `approved` …) — a fallback. */
    val status: String,
    /** The state in plain German, worded by the rental platform. */
    val statusText: String,
    /** Whether cancelling is allowed — the server decides. */
    val cancellable: Boolean,
    /** Pickup address; only filled in once the booking is approved. */
    val pickupAddress: String?,
)

/**
 * The answer to "is it free on my weekend?".
 *
 * [free] comes from the server. [hint] is its reason in plain German; the app
 * passes it through instead of guessing "occupied".
 */
data class Availability(
    val period: Period,
    val free: Boolean,
    val hint: String?,
    /** Occupied periods around the request, for display in the detail sheet. */
    val occupied: List<Period> = emptyList(),
)

/** A booking request as the user interface assembles it. */
data class BookingRequest(
    val deviceId: String,
    val period: Period,
    val message: String = "",
)

/**
 * The rental platform refused and names the reason in plain German (HTTP 400
 * or 409). It is meant for the person asking — the check deliberately sits
 * over there so that web and app follow the same rules.
 */
class RentalRejectedException(val reason: String) : RuntimeException(reason)

/**
 * The sign-in of this device does not count for the rental platform.
 *
 * This is the likely case on a device that was already signed in: a phone
 * keeps its set of tokens across an app update. Such a token was issued
 * before the changeover and does not name the rental platform's project as an
 * audience — the platform rejects it although nothing about the sign-in
 * itself is broken.
 *
 * The only remedy is signing in again; the app says so and offers it, instead
 * of showing an empty list.
 */
class StaleAudienceException :
    RuntimeException("the access token does not name the rental platform as an audience")

/** This step needs a sign-in and there is none. */
class NotSignedInException : RuntimeException("not signed in")

/**
 * What the app can do with the rental platform.
 *
 * Reading works without signing in — the device list is public on the web as
 * well. Only the personal and writing steps require a token.
 */
interface RentalRepository {
    /** Every device. No sign-in. */
    suspend fun devices(): List<Device>

    /** Search on the server — ranking is its business, not ours. */
    suspend fun search(text: String): List<Device>

    /** Is the period free? The server decides. No sign-in. */
    suspend fun availability(deviceId: String, period: Period): Availability

    /** Book. Signed in. */
    suspend fun book(request: BookingRequest): Booking

    /** My bookings. Signed in. */
    suspend fun myBookings(): List<Booking>

    /** Cancel. Signed in. */
    suspend fun cancel(bookingId: String): Booking
}

/**
 * An empty rental platform. The default for user interfaces built without one
 * (tests that check something entirely different, for instance) — it fetches
 * nothing and claims nothing.
 */
object NoRental : RentalRepository {
    override suspend fun devices(): List<Device> = emptyList()
    override suspend fun search(text: String): List<Device> = emptyList()
    override suspend fun availability(deviceId: String, period: Period) =
        Availability(period, free = false, hint = null)

    override suspend fun book(request: BookingRequest): Booking = throw NotSignedInException()
    override suspend fun myBookings(): List<Booking> = emptyList()
    override suspend fun cancel(bookingId: String): Booking = throw NotSignedInException()
}
