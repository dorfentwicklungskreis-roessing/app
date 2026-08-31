package de.roessing.app

import de.roessing.app.auth.RentalSignIn
import de.roessing.app.data.BookingRequest
import de.roessing.app.data.BookingStatus
import de.roessing.app.data.LenderStatus
import de.roessing.app.data.OccupancyStatus
import de.roessing.app.data.RentalApiException
import de.roessing.app.data.RentalAvailability
import de.roessing.app.data.RentalBooking
import de.roessing.app.data.RentalDevice
import de.roessing.app.data.RentalDeviceDetail
import de.roessing.app.data.RentalErrorCode
import de.roessing.app.data.RentalImage
import de.roessing.app.data.RentalOccupancy
import de.roessing.app.data.RentalPeriod
import de.roessing.app.data.RentalProfile
import de.roessing.app.data.RentalRepository
import de.roessing.app.ui.RentalEvent
import de.roessing.app.ui.RentalViewModel
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
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
 * Der Bereich „Maschinchenring" in der App.
 *
 * Geprüft wird vor allem, was leicht schiefgeht und was ausdrücklich **nicht**
 * die App entscheidet: Der Knopf zum Anfragen hängt an der Antwort des
 * Servers, nicht an einer eigenen Rechnung; ein Token von vor der Umstellung
 * führt zu einem Satz, den jemand befolgen kann, statt zu einer leeren Liste;
 * und ein Ausfall der Mietplattform lässt die zuletzt geholte Liste stehen.
 */
private class FakeRental : RentalRepository {
    var devices: List<RentalDevice> = emptyList()
    var searchResults: List<RentalDevice> = emptyList()
    var bookings: List<RentalBooking> = emptyList()
    var occupancy: List<RentalOccupancy> = emptyList()
    var availability: RentalAvailability? = null
    var profile: RentalProfile = profil()

    /**
     * Hält eine Verfügbarkeitsprüfung an, solange sie nicht freigegeben ist.
     * Ohne sie ist eine abgelöste Prüfung schon vorbei, bevor sie abgebrochen
     * werden kann — und der Fall, um den es geht, träte gar nicht ein.
     */
    var availabilityGate: CompletableDeferred<Unit>? = null

    /** Was der nächste Abruf werfen soll, statt zu antworten. */
    var devicesFailure: Throwable? = null
    var bookingsFailure: Throwable? = null
    var profileFailure: Throwable? = null
    var bookFailure: Throwable? = null
    var cancelFailure: Throwable? = null
    var availabilityFailure: Throwable? = null

    var deviceCalls = 0
    var searchCalls = 0
    var occupancyCalls = 0
    var bookingCalls = 0
    var lastRequest: BookingRequest? = null
    var cancelled = mutableListOf<String>()

    override suspend fun devices(): List<RentalDevice> {
        deviceCalls++
        devicesFailure?.let { throw it }
        return devices
    }

    override suspend fun device(id: String): RentalDeviceDetail =
        RentalDeviceDetail(
            device = devices.firstOrNull { it.id == id } ?: geraet(id),
            images = listOf(RentalImage("img-1", "https://bild.example.invalid/1.jpg", true)),
        )

    override suspend fun search(query: String): List<RentalDevice> {
        searchCalls++
        devicesFailure?.let { throw it }
        return searchResults
    }

    override suspend fun occupancy(deviceId: String): List<RentalOccupancy> {
        occupancyCalls++
        return occupancy
    }

    override suspend fun availability(
        deviceId: String,
        period: RentalPeriod,
    ): RentalAvailability {
        availabilityGate?.await()
        availabilityFailure?.let { throw it }
        // Die Antwort trägt den Zeitraum, für den gefragt wurde — genau
        // darauf kommt es an, wenn zwei Prüfungen unterwegs sind.
        return availability?.copy(period = period)
            ?: RentalAvailability(period, available = false, reason = "occupied")
    }

    override suspend fun profile(): RentalProfile {
        profileFailure?.let { throw it }
        return profile
    }

    override suspend fun myBookings(): List<RentalBooking> {
        bookingCalls++
        bookingsFailure?.let { throw it }
        return bookings
    }

    override suspend fun book(request: BookingRequest): RentalBooking {
        lastRequest = request
        bookFailure?.let { throw it }
        return buchung("neu", request.period)
    }

    override suspend fun cancel(bookingId: String) {
        cancelFailure?.let { throw it }
        cancelled += bookingId
    }
}

private fun geraet(id: String, name: String = "Kreiselmäher") = RentalDevice(
    id = id,
    name = name,
    description = "Für hohes Gras.",
    pricePerDay = 25.0,
    pricePerWeekend = null,
    pricePerWeek = null,
    deposit = 100.0,
    tags = listOf("garten"),
    thumbnailUrl = null,
    productUrl = null,
    webUrl = "https://mieten.example.invalid/geraete/$id/",
)

private fun zeitraum(von: String = "2026-09-05", bis: String = "2026-09-07") =
    RentalPeriod(LocalDate.parse(von), LocalDate.parse(bis))

private fun buchung(
    id: String,
    period: RentalPeriod = zeitraum(),
    canCancel: Boolean = true,
) = RentalBooking(
    id = id,
    deviceId = "maeher",
    setId = null,
    deviceName = "Kreiselmäher",
    period = period,
    status = BookingStatus.PENDING,
    rawStatus = "pending",
    notes = null,
    canCancel = canCancel,
    pickup = null,
)

private fun profil(missing: List<String> = emptyList()) = RentalProfile(
    name = "Erika Musterfrau",
    email = "erika@example.invalid",
    phone = "+49 5069 123456",
    addressStreet = "Hauptstraße 1",
    addressZip = "31171",
    addressCity = "Nordstemmen",
    lender = false,
    lenderStatus = LenderStatus.NONE,
    profileComplete = missing.isEmpty(),
    missingFields = missing,
)

@OptIn(ExperimentalCoroutinesApi::class)
class RentalViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before fun vorher() = Dispatchers.setMain(dispatcher)

    @After fun nachher() = Dispatchers.resetMain()

    private fun vm(
        repo: RentalRepository,
        signIn: RentalSignIn = RentalSignIn.MISSING,
    ) = RentalViewModel(repo, signIn = { signIn }, searchDelay = 0L)

    // --- Ansehen und suchen, ohne Anmeldung ---------------------------------

    @Test
    fun `ohne Anmeldung stehen die Geraete trotzdem da`() = runTest(dispatcher) {
        val repo = FakeRental().apply { devices = listOf(geraet("maeher"), geraet("walze")) }
        val vm = vm(repo)

        vm.load()
        advanceUntilIdle()

        assertEquals(listOf("maeher", "walze"), vm.state.value.devices.map { it.id })
        assertFalse(vm.state.value.signedIn)
        assertFalse(vm.state.value.offline)
        // Ohne Anmeldung wird nichts Persönliches abgerufen.
        assertEquals(0, repo.bookingCalls)
    }

    @Test
    fun `ein leerer Maschinchenring ist kein Fehler`() = runTest(dispatcher) {
        val vm = vm(FakeRental())

        vm.load()
        advanceUntilIdle()

        assertTrue(vm.state.value.empty)
        assertFalse(vm.state.value.offline)
    }

    @Test
    fun `ohne Netz steht ein Hinweis ueber der zuletzt geholten Liste`() = runTest(dispatcher) {
        val repo = FakeRental().apply { devices = listOf(geraet("maeher")) }
        val vm = vm(repo)
        vm.load()
        advanceUntilIdle()
        assertEquals(1, vm.state.value.devices.size)

        repo.devicesFailure = IOException("kein Netz")
        vm.refresh()
        advanceUntilIdle()

        // Lieber ein Hinweis über einer alten Liste als eine leere Seite.
        assertTrue(vm.state.value.offline)
        assertEquals(listOf("maeher"), vm.state.value.devices.map { it.id })
        assertFalse(vm.state.value.empty)
    }

    /** Ein Ausfall der Mietplattform ist für die Liste dasselbe wie kein Netz. */
    @Test
    fun `auch ein Serverfehler laesst die alte Liste stehen`() = runTest(dispatcher) {
        val repo = FakeRental().apply { devices = listOf(geraet("maeher")) }
        val vm = vm(repo)
        vm.load()
        advanceUntilIdle()

        repo.devicesFailure = RentalApiException(RentalErrorCode.INTERNAL, "Kaputt")
        vm.refresh()
        advanceUntilIdle()

        assertTrue(vm.state.value.offline)
        assertEquals(1, vm.state.value.devices.size)
    }

    @Test
    fun `gesucht wird auf dem Server, nicht in der Liste`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            devices = listOf(geraet("maeher"), geraet("walze"))
            searchResults = listOf(geraet("walze", "Rasenwalze"))
        }
        val vm = vm(repo)
        vm.load()
        advanceUntilIdle()

        vm.setQuery("rasen")
        advanceUntilIdle()

        assertEquals(1, repo.searchCalls)
        assertEquals(listOf("walze"), vm.state.value.devices.map { it.id })
    }

    @Test
    fun `eine leere Suche bringt die ganze Liste zurueck`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            devices = listOf(geraet("maeher"), geraet("walze"))
            searchResults = emptyList()
        }
        val vm = vm(repo)
        vm.load()
        advanceUntilIdle()

        vm.setQuery("nichts")
        advanceUntilIdle()
        assertTrue(vm.state.value.empty || vm.state.value.devices.isEmpty())

        vm.setQuery("")
        advanceUntilIdle()

        assertEquals(2, vm.state.value.devices.size)
    }

    @Test
    fun `ein zweites Oeffnen holt die Liste nicht noch einmal`() = runTest(dispatcher) {
        val repo = FakeRental().apply { devices = listOf(geraet("maeher")) }
        val vm = vm(repo)

        vm.load()
        advanceUntilIdle()
        vm.load()
        advanceUntilIdle()

        assertEquals(1, repo.deviceCalls)

        vm.refresh()
        advanceUntilIdle()
        assertEquals(2, repo.deviceCalls)
    }

    // --- Anmeldung ----------------------------------------------------------

    @Test
    fun `angemeldet kommen Profil und eigene Buchungen dazu`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            devices = listOf(geraet("maeher"))
            bookings = listOf(buchung("b-1"))
        }
        val vm = vm(repo, RentalSignIn.VALID)

        vm.load()
        advanceUntilIdle()

        assertTrue(vm.state.value.signedIn)
        assertFalse(vm.state.value.staleSignIn)
        assertEquals(listOf("b-1"), vm.state.value.bookings.map { it.id })
        assertEquals("Erika Musterfrau", vm.state.value.profile?.name)
    }

    /**
     * Der Stolperstein: Ein Gerät, das schon angemeldet war, behält seinen
     * Token-Satz über die Aktualisierung hinweg. Das Token gilt für die
     * Mietplattform nicht — und daran ist von hier aus nichts zu reparieren
     * außer einer neuen Anmeldung. Eine leere Liste wäre keine Antwort.
     */
    @Test
    fun `ein Token von vor der Umstellung fuehrt zur Bitte um neue Anmeldung`() =
        runTest(dispatcher) {
            val repo = FakeRental().apply { devices = listOf(geraet("maeher")) }
            val vm = vm(repo, RentalSignIn.STALE)

            vm.load()
            advanceUntilIdle()

            assertTrue(vm.state.value.staleSignIn)
            assertFalse(vm.state.value.signedIn)
            assertTrue(vm.state.value.bookings.isEmpty())
            // Die Geräte stehen trotzdem da — sie sind öffentlich.
            assertEquals(1, vm.state.value.devices.size)
            assertEquals(0, repo.bookingCalls)
        }

    /**
     * Dasselbe, nur andersherum: Das Token ließ sich nicht ansehen, der
     * Server sagt nein. Auch das heißt „neu anmelden", nicht „abgemeldet".
     */
    @Test
    fun `sagt der Server token_audience, wird ebenso neu angemeldet`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            bookingsFailure = RentalApiException(RentalErrorCode.TOKEN_AUDIENCE, null)
        }
        val vm = vm(repo, RentalSignIn.VALID)

        vm.load()
        advanceUntilIdle()

        assertTrue(vm.state.value.staleSignIn)
        assertFalse(vm.state.value.signedIn)
    }

    @Test
    fun `ein abgelaufenes Token ist kein Fall fuer die Neuanmeldung`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            bookingsFailure = RentalApiException(RentalErrorCode.UNAUTHORIZED, null)
        }
        val vm = vm(repo, RentalSignIn.VALID)

        vm.load()
        advanceUntilIdle()

        // Das ist die gewöhnliche Anmeldung, nicht der Sonderfall.
        assertFalse(vm.state.value.staleSignIn)
        assertFalse(vm.state.value.signedIn)
    }

    @Test
    fun `Buchungen ohne Netz kosten nicht die ganze Ansicht`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            devices = listOf(geraet("maeher"))
            bookingsFailure = IOException("kein Netz")
        }
        val vm = vm(repo, RentalSignIn.VALID)

        vm.load()
        advanceUntilIdle()

        assertTrue(vm.state.value.bookingsOffline)
        assertTrue(vm.state.value.signedIn)
        assertEquals(1, vm.state.value.devices.size)
    }

    // --- Zeitraum und Buchen ------------------------------------------------

    @Test
    fun `belegte Zeitraeume kommen vom Server, sobald ein Geraet offen ist`() =
        runTest(dispatcher) {
            val repo = FakeRental().apply {
                devices = listOf(geraet("maeher"))
                occupancy = listOf(
                    RentalOccupancy("maeher", null, zeitraum(), OccupancyStatus.APPROVED),
                )
            }
            val vm = vm(repo)
            vm.load()
            advanceUntilIdle()

            vm.open(repo.devices.first())
            advanceUntilIdle()

            assertEquals(1, vm.state.value.occupancy.size)
            assertEquals(1, vm.state.value.images.size)
            assertEquals(1, repo.occupancyCalls)
        }

    /**
     * Der Kern der Sache: Ob angefragt werden darf, sagt der Server. Die App
     * liest es nicht aus den belegten Zeiträumen ab, die sie eben gezeichnet
     * hat — sonst stünde die Regel in zwei Fassungen im Haus.
     */
    @Test
    fun `angefragt werden darf erst, wenn der Server frei sagt`() = runTest(dispatcher) {
        val repo = FakeRental().apply { devices = listOf(geraet("maeher")) }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())
        advanceUntilIdle()

        // Ohne Zeitraum: nichts.
        assertFalse(vm.state.value.canBook)

        repo.availability = RentalAvailability(zeitraum(), available = false, reason = "occupied")
        vm.setPeriod(zeitraum())
        advanceUntilIdle()
        assertFalse(vm.state.value.canBook)

        repo.availability = RentalAvailability(zeitraum(), available = true, reason = null)
        vm.setPeriod(zeitraum())
        advanceUntilIdle()
        assertTrue(vm.state.value.canBook)
    }

    /**
     * Eine Antwort auf „ist es frei?" gilt für **den** Zeitraum, für den sie
     * kam. Wer die Tage verschiebt, hat keine Antwort mehr — sonst fragte
     * jemand ein freies Wochenende auf einem belegten an.
     */
    @Test
    fun `eine Antwort auf einen alten Zeitraum zaehlt nicht`() = runTest(dispatcher) {
        val repo = FakeRental().apply { devices = listOf(geraet("maeher")) }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())
        advanceUntilIdle()

        repo.availability = RentalAvailability(zeitraum(), available = true, reason = null)
        vm.setPeriod(zeitraum())
        advanceUntilIdle()
        assertTrue(vm.state.value.canBook)

        // Ein anderer Zeitraum: Die alte Zusage darf ihn nicht mittragen —
        // weder gleich beim Umstellen noch, wenn die neue Antwort da ist.
        val anderer = zeitraum("2026-10-01", "2026-10-03")
        vm.setPeriod(anderer)
        assertNull(vm.state.value.availability)
        assertFalse(vm.state.value.canBook)

        advanceUntilIdle()
        assertEquals(anderer, vm.state.value.availability?.period)
    }

    /**
     * Und derselbe Fall von der anderen Seite: Eine Antwort, die zu einem
     * inzwischen abgelösten Zeitraum gehört, darf den neuen nicht freigeben.
     */
    @Test
    fun `eine ueberholte Zusage gibt den neuen Zeitraum nicht frei`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            devices = listOf(geraet("maeher"))
            availability = RentalAvailability(zeitraum(), available = true, reason = null)
            availabilityGate = CompletableDeferred()
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())
        advanceUntilIdle()

        val alter = zeitraum()
        vm.setPeriod(alter)
        advanceUntilIdle()
        // Die Prüfung hängt noch an der Schranke.
        assertTrue(vm.state.value.checking)
        assertNull(vm.state.value.availability)

        val neuer = zeitraum("2026-10-01", "2026-10-03")
        vm.setPeriod(neuer)
        repo.availabilityGate?.complete(Unit)
        advanceUntilIdle()

        // Die Antwort, die ankommt, gehört zum neuen Zeitraum.
        assertEquals(neuer, vm.state.value.availability?.period)
    }

    /**
     * Wer schnell zweimal einen Zeitraum wählt, bricht die erste Prüfung ab.
     * Ein Abbruch ist kein Ausfall — sonst stünde „nicht erreichbar" da,
     * obwohl nichts fehlt.
     */
    @Test
    fun `eine abgeloeste Pruefung meldet keinen Ausfall`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            devices = listOf(geraet("maeher"))
            availability = RentalAvailability(zeitraum(), available = true, reason = null)
            availabilityGate = CompletableDeferred()
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())
        advanceUntilIdle()

        vm.setPeriod(zeitraum())
        // Erst laufen lassen, damit die Prüfung wirklich an der Schranke
        // hängt — sonst wird sie abgebrochen, bevor sie begonnen hat, und der
        // Fall, um den es geht, träte nie ein.
        advanceUntilIdle()
        assertTrue(vm.state.value.checking)

        vm.setPeriod(zeitraum("2026-10-01", "2026-10-03"))
        repo.availabilityGate?.complete(Unit)
        advanceUntilIdle()

        // Der Abbruch der ersten Prüfung ist kein Ausfall der Mietplattform.
        assertFalse(vm.state.value.offline)
        assertFalse(vm.state.value.checking)
    }

    @Test
    fun `ohne Anmeldung fuehrt kein Weg zum Anfragen`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            devices = listOf(geraet("maeher"))
            availability = RentalAvailability(zeitraum(), available = true, reason = null)
        }
        val vm = vm(repo)
        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())
        vm.setPeriod(zeitraum())
        advanceUntilIdle()

        assertFalse(vm.state.value.canBook)
        vm.book("egal")
        advanceUntilIdle()
        assertNull(repo.lastRequest)
    }

    @Test
    fun `eine Anfrage geht hinaus und die eigenen Buchungen kommen neu`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            devices = listOf(geraet("maeher"))
            availability = RentalAvailability(zeitraum(), available = true, reason = null)
        }
        val vm = vm(repo, RentalSignIn.VALID)
        val ereignisse = mutableListOf<RentalEvent>()
        val lauscher = launch { ereignisse += vm.events.first() }

        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())
        vm.setPeriod(zeitraum())
        advanceUntilIdle()

        val vorher = repo.bookingCalls
        vm.book("Hole ich Samstag früh ab.")
        advanceUntilIdle()
        lauscher.join()

        assertEquals("Hole ich Samstag früh ab.", repo.lastRequest?.notes)
        // Der Normalfall: Name und Telefon holt sich der Server aus dem Profil.
        assertNull(repo.lastRequest?.firstName)
        assertNull(repo.lastRequest?.phone)
        assertEquals(listOf(RentalEvent.Booked), ereignisse)
        assertNull(vm.state.value.selected)
        assertEquals(vorher + 1, repo.bookingCalls)
    }

    /**
     * Der gewöhnliche Wettlauf: Zwischen dem Zeichnen des Kalenders und dem
     * Tippen kann eine Minute liegen. Das ist ein Hinweis und ein neuer
     * Kalender, kein Absturz.
     */
    @Test
    fun `ein inzwischen belegter Zeitraum ist ein Hinweis, kein Absturz`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            devices = listOf(geraet("maeher"))
            availability = RentalAvailability(zeitraum(), available = true, reason = null)
            bookFailure = RentalApiException(RentalErrorCode.OCCUPIED, "Schon vergeben")
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())
        vm.setPeriod(zeitraum())
        advanceUntilIdle()
        val vorher = repo.occupancyCalls

        vm.book()
        advanceUntilIdle()

        assertEquals(RentalErrorCode.OCCUPIED, vm.state.value.notice?.code)
        assertEquals("Schon vergeben", vm.state.value.notice?.text)
        // Das Gerät bleibt offen, damit ein anderer Zeitraum gewählt werden kann.
        assertEquals("maeher", vm.state.value.selected?.id)
        assertEquals(vorher + 1, repo.occupancyCalls)
    }

    /**
     * Was dem Server am Profil fehlt, sagt er selbst. Die App fragt genau das
     * nach und denkt sich keine eigene Liste aus.
     */
    @Test
    fun `fehlt dem Profil etwas, sagt der Server welche Felder`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            devices = listOf(geraet("maeher"))
            availability = RentalAvailability(zeitraum(), available = true, reason = null)
            bookFailure = RentalApiException(
                RentalErrorCode.PROFILE_INCOMPLETE,
                "Dein Profil ist unvollständig",
                missingFields = listOf("phone"),
            )
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.open(repo.devices.first())
        vm.setPeriod(zeitraum())
        advanceUntilIdle()

        vm.book()
        advanceUntilIdle()

        assertEquals(listOf("phone"), vm.state.value.missingFields)
        assertEquals(RentalErrorCode.PROFILE_INCOMPLETE, vm.state.value.notice?.code)

        // Beim zweiten Anlauf gehen die nachgefragten Angaben mit.
        repo.bookFailure = null
        vm.book(notes = "", firstName = "Erika", lastName = "Musterfrau", phone = "0123")
        advanceUntilIdle()

        assertEquals("0123", repo.lastRequest?.phone)
        assertEquals("Erika", repo.lastRequest?.firstName)
    }

    @Test
    fun `eine Buchung ohne Verbindung meldet den Ausfall, nicht eine Absage`() =
        runTest(dispatcher) {
            val repo = FakeRental().apply {
                devices = listOf(geraet("maeher"))
                availability = RentalAvailability(zeitraum(), available = true, reason = null)
                bookFailure = IOException("kein Netz")
            }
            val vm = vm(repo, RentalSignIn.VALID)
            vm.load()
            advanceUntilIdle()
            vm.open(repo.devices.first())
            vm.setPeriod(zeitraum())
            advanceUntilIdle()

            vm.book()
            advanceUntilIdle()

            assertTrue(vm.state.value.offline)
            assertNull(vm.state.value.notice)
            assertFalse(vm.state.value.booking)
        }

    // --- Stornieren ---------------------------------------------------------

    @Test
    fun `eine Buchung laesst sich stornieren, wenn der Server es erlaubt`() = runTest(dispatcher) {
        val repo = FakeRental().apply { bookings = listOf(buchung("b-1")) }
        val vm = vm(repo, RentalSignIn.VALID)
        val ereignisse = mutableListOf<RentalEvent>()
        val lauscher = launch { ereignisse += vm.events.first() }
        vm.load()
        advanceUntilIdle()

        vm.cancel("b-1")
        advanceUntilIdle()
        lauscher.join()

        assertEquals(listOf("b-1"), repo.cancelled)
        assertEquals(listOf(RentalEvent.Cancelled), ereignisse)
        assertTrue(vm.state.value.cancelling.isEmpty())
    }

    @Test
    fun `eine schon stornierte Buchung meldet den Grund und holt die Liste neu`() =
        runTest(dispatcher) {
            val repo = FakeRental().apply {
                bookings = listOf(buchung("b-1"))
                cancelFailure = RentalApiException(RentalErrorCode.CONFLICT, "Schon storniert")
            }
            val vm = vm(repo, RentalSignIn.VALID)
            vm.load()
            advanceUntilIdle()
            val vorher = repo.bookingCalls

            vm.cancel("b-1")
            advanceUntilIdle()

            assertEquals(RentalErrorCode.CONFLICT, vm.state.value.notice?.code)
            assertEquals("Schon storniert", vm.state.value.notice?.text)
            assertEquals(vorher + 1, repo.bookingCalls)
            assertTrue(vm.state.value.cancelling.isEmpty())
        }

    @Test
    fun `zweimal auf Stornieren tippen storniert nicht zweimal`() = runTest(dispatcher) {
        val repo = FakeRental().apply { bookings = listOf(buchung("b-1")) }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.cancel("b-1")
        vm.cancel("b-1")
        advanceUntilIdle()

        assertEquals(listOf("b-1"), repo.cancelled)
    }
}
