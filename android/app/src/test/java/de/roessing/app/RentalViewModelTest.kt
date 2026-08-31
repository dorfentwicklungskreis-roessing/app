package de.roessing.app

import de.roessing.app.auth.RentalSignIn
import de.roessing.app.data.Availability
import de.roessing.app.data.Booking
import de.roessing.app.data.BookingRequest
import de.roessing.app.data.Device
import de.roessing.app.data.NotSignedInException
import de.roessing.app.data.Period
import de.roessing.app.data.RentalRejectedException
import de.roessing.app.data.RentalRepository
import de.roessing.app.data.StaleAudienceException
import de.roessing.app.ui.RentalViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.IOException
import java.time.LocalDate

/**
 * The area "Maschinchenring" in the app.
 *
 * What is checked is what breaks easily: that an outage leaves the previous
 * list standing, that a device may only be booked once the *server* has
 * called the period free, that a refusal is passed on in the server's own
 * words, and that a token from before the changeover produces the sentence
 * "please sign in again" instead of an empty page.
 */
private class FakeRental(
    var devices: List<Device> = emptyList(),
    var bookings: List<Booking> = emptyList(),
) : RentalRepository {
    var failure: Throwable? = null
    var bookFailure: Throwable? = null
    var availability: Availability? = null
    var fetches = 0
    var lastQuery: String? = null
    var lastRequest: BookingRequest? = null
    var cancelled: String? = null

    override suspend fun devices(): List<Device> {
        fetches++
        lastQuery = null
        failure?.let { throw it }
        return devices
    }

    override suspend fun search(text: String): List<Device> {
        fetches++
        lastQuery = text
        failure?.let { throw it }
        return devices
    }

    override suspend fun availability(deviceId: String, period: Period): Availability {
        failure?.let { throw it }
        return availability ?: Availability(period, free = false, hint = null)
    }

    override suspend fun book(request: BookingRequest): Booking {
        lastRequest = request
        bookFailure?.let { throw it }
        return booking("neu", request.deviceId, request.period)
    }

    override suspend fun myBookings(): List<Booking> {
        bookFailure?.let { throw it }
        return bookings
    }

    override suspend fun cancel(bookingId: String): Booking {
        cancelled = bookingId
        bookFailure?.let { throw it }
        return bookings.first { it.id == bookingId }
    }
}

private fun device(
    id: String,
    name: String = "Kreiselmäher AS 585",
    bookable: Boolean = true,
    reason: String? = null,
) = Device(
    id = id,
    name = name,
    description = "Für hohes Gras.",
    imageUrl = null,
    owner = "Familie Beispiel",
    location = "Rössing",
    price = "15 € pro Tag",
    deposit = null,
    tags = emptyList(),
    bookable = bookable,
    notBookableReason = reason,
)

private fun booking(
    id: String,
    deviceId: String = "maeher",
    period: Period = weekend,
    cancellable: Boolean = true,
) = Booking(
    id = id,
    deviceId = deviceId,
    deviceName = "Kreiselmäher AS 585",
    period = period,
    status = "pending",
    statusText = "Warte auf die Zusage des Eigentümers",
    cancellable = cancellable,
    pickupAddress = null,
)

/** Friday to Sunday — the weekend somebody wants the mower for. */
private val weekend = Period(LocalDate.of(2026, 9, 4), LocalDate.of(2026, 9, 6))

@OptIn(ExperimentalCoroutinesApi::class)
class RentalViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    private fun vm(
        repo: RentalRepository,
        signIn: RentalSignIn = RentalSignIn.MISSING,
    ) = RentalViewModel(repo, signIn = { signIn })

    @Before fun before() = Dispatchers.setMain(dispatcher)

    @After fun after() = Dispatchers.resetMain()

    @Test
    fun `devices are readable without signing in`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher"), device("walze", "Rasenwalze")))
        val vm = vm(repo, RentalSignIn.MISSING)
        vm.load()
        advanceUntilIdle()

        assertEquals(listOf("maeher", "walze"), vm.state.value.devices.map { it.id })
        assertFalse(vm.state.value.signedIn)
        assertFalse(vm.state.value.offline)
        // Nobody is signed in, so there is nothing personal to show either.
        assertTrue(vm.state.value.bookings.isEmpty())
    }

    @Test
    fun `searching asks the server and only once per typed word`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher")))
        val vm = RentalViewModel(repo, signIn = { RentalSignIn.MISSING }, searchDelay = 300)
        vm.load()
        advanceUntilIdle()
        assertEquals(1, repo.fetches)

        vm.setQuery("r")
        advanceTimeBy(50)
        vm.setQuery("ra")
        advanceTimeBy(50)
        vm.setQuery("ras")
        advanceUntilIdle()

        // Three keystrokes, one request — and the ranking stays over there.
        assertEquals(2, repo.fetches)
        assertEquals("ras", repo.lastQuery)
    }

    @Test
    fun `an emptied search field brings the whole list back`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher")))
        val vm = RentalViewModel(repo, signIn = { RentalSignIn.MISSING }, searchDelay = 0)
        vm.load()
        advanceUntilIdle()
        vm.setQuery("walze")
        advanceUntilIdle()
        assertEquals("walze", repo.lastQuery)

        vm.setQuery("")
        advanceUntilIdle()

        assertNull(repo.lastQuery)
    }

    @Test
    fun `an outage leaves the previous list standing with a hint above it`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher")))
        val vm = vm(repo)
        vm.load()
        advanceUntilIdle()
        assertEquals(1, vm.state.value.devices.size)

        repo.failure = IOException("no network")
        vm.refresh()
        advanceUntilIdle()

        // Better a hint over an old list than an empty page.
        assertTrue(vm.state.value.offline)
        assertEquals(listOf("maeher"), vm.state.value.devices.map { it.id })
        assertFalse(vm.state.value.loading)
    }

    @Test
    fun `an empty rental platform is not an outage`() = runTest(dispatcher) {
        val vm = vm(FakeRental(emptyList()))
        vm.load()
        advanceUntilIdle()

        assertTrue(vm.state.value.empty)
        assertFalse(vm.state.value.offline)
    }

    @Test
    fun `signed in, the own bookings come along`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher")), bookings = listOf(booking("b-1")))
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        assertTrue(vm.state.value.signedIn)
        assertEquals(listOf("b-1"), vm.state.value.bookings.map { it.id })
    }

    @Test
    fun `a token from before the changeover asks for a fresh sign-in`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher")))
        val vm = vm(repo, RentalSignIn.STALE)
        vm.load()
        advanceUntilIdle()

        // This is the case that looks like a defect and is not one: the phone
        // is signed in, but with a token issued before the rental platform
        // joined the Rössing-ID. The list still reads — only the personal
        // half is missing, and the app says why.
        assertTrue(vm.state.value.staleSignIn)
        assertFalse(vm.state.value.signedIn)
        assertEquals(1, vm.state.value.devices.size)
        assertFalse(vm.state.value.offline)
    }

    @Test
    fun `not being able to ask does not end a sign-in`() = runTest(dispatcher) {
        val vm = vm(FakeRental(listOf(device("maeher"))), RentalSignIn.UNREACHABLE)
        vm.load()
        advanceUntilIdle()

        // A dead spot is a circumstance, not a decision — nobody is told to
        // sign in again over it.
        assertFalse(vm.state.value.staleSignIn)
        assertFalse(vm.state.value.signedIn)
    }

    @Test
    fun `booking waits for the server to call the period free`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher")))
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())

        // A period alone is not enough — the answer belongs to the server.
        vm.setPeriod(weekend)
        repo.availability = Availability(weekend, free = false, hint = "Am 5.9. schon vergeben.")
        advanceUntilIdle()
        assertFalse(vm.state.value.canBook)
        assertEquals("Am 5.9. schon vergeben.", vm.state.value.availability?.hint)

        repo.availability = Availability(weekend, free = true, hint = null)
        vm.setPeriod(weekend)
        advanceUntilIdle()
        assertTrue(vm.state.value.canBook)
    }

    @Test
    fun `a device the server calls unbookable stays unbookable`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("matten", bookable = false, reason = "In Reparatur.")))
        repo.availability = Availability(weekend, free = true, hint = null)
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())
        vm.setPeriod(weekend)
        advanceUntilIdle()

        // The app does not know the condition; it only knows the answer.
        assertFalse(vm.state.value.canBook)
        assertEquals("In Reparatur.", vm.state.value.selected?.notBookableReason)
    }

    @Test
    fun `a successful booking closes the sheet and refreshes the list`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher")))
        repo.availability = Availability(weekend, free = true, hint = null)
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())
        vm.setPeriod(weekend)
        advanceUntilIdle()

        repo.bookings = listOf(booking("b-neu"))
        vm.book("Ich hole ihn Freitagabend ab.")
        advanceUntilIdle()

        assertEquals(weekend, repo.lastRequest?.period)
        assertEquals("Ich hole ihn Freitagabend ab.", repo.lastRequest?.message)
        assertNull(vm.state.value.selected)
        assertEquals(listOf("b-neu"), vm.state.value.bookings.map { it.id })
    }

    @Test
    fun `a refusal is shown in the words of the server`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher")))
        repo.availability = Availability(weekend, free = true, hint = null)
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())
        vm.setPeriod(weekend)
        advanceUntilIdle()

        repo.bookFailure = RentalRejectedException("In diesem Zeitraum ist er bereits vergeben.")
        vm.book()
        advanceUntilIdle()

        // Verbatim: the platform's reason is better than anything we could
        // invent, and it keeps web and app saying the same thing.
        assertEquals("In diesem Zeitraum ist er bereits vergeben.", vm.state.value.message)
        // What was typed stays; the sheet remains open.
        assertEquals("maeher", vm.state.value.selected?.id)
    }

    @Test
    fun `a stale token during booking asks for a fresh sign-in`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher")))
        repo.availability = Availability(weekend, free = true, hint = null)
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())
        vm.setPeriod(weekend)
        advanceUntilIdle()

        repo.bookFailure = StaleAudienceException()
        vm.book()
        advanceUntilIdle()

        assertTrue(vm.state.value.staleSignIn)
        assertFalse(vm.state.value.signedIn)
    }

    @Test
    fun `cancelling asks the server and refreshes the list`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher")), bookings = listOf(booking("b-1")))
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.cancel("b-1")
        advanceUntilIdle()

        assertEquals("b-1", repo.cancelled)
        assertTrue(vm.state.value.cancelling.isEmpty())
    }

    @Test
    fun `signing out clears the bookings but not the devices`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher")), bookings = listOf(booking("b-1")))
        var signIn = RentalSignIn.VALID
        val vm = RentalViewModel(repo, signIn = { signIn })
        vm.load()
        advanceUntilIdle()
        assertEquals(1, vm.state.value.bookings.size)

        signIn = RentalSignIn.MISSING
        vm.refresh()
        advanceUntilIdle()

        assertTrue(vm.state.value.bookings.isEmpty())
        assertEquals(1, vm.state.value.devices.size)
    }

    @Test
    fun `bookings that cannot be fetched do not look like an empty shelf`() = runTest(dispatcher) {
        val repo = FakeRental(listOf(device("maeher")))
        repo.bookFailure = IOException("no network")
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        assertTrue(vm.state.value.bookingsOffline)
        assertTrue(vm.state.value.signedIn)
    }

    @Test
    fun `a personal call without a sign-in says so and does not sign anybody out`() =
        runTest(dispatcher) {
            val repo = FakeRental(listOf(device("maeher")))
            repo.bookFailure = NotSignedInException()
            val vm = vm(repo, RentalSignIn.VALID)
            vm.load()
            advanceUntilIdle()

            assertFalse(vm.state.value.signedIn)
            assertFalse(vm.state.value.staleSignIn)
            assertFalse(vm.state.value.offline)
        }

    @Test
    fun `a period spans whole days and reads as one line`() {
        assertEquals("4.–6. September 2026", weekend.text)
        assertEquals(3, weekend.days)
        assertEquals(
            "19. September 2026",
            Period(LocalDate.of(2026, 9, 19), LocalDate.of(2026, 9, 19)).text,
        )
        assertEquals(
            "28. Dezember 2026 – 3. Januar 2027",
            Period(LocalDate.of(2026, 12, 28), LocalDate.of(2027, 1, 3)).text,
        )
        assertFalse(Period(LocalDate.of(2026, 9, 6), LocalDate.of(2026, 9, 4)).isSane)
    }
}
