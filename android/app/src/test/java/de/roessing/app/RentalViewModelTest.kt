package de.roessing.app

import de.roessing.app.auth.RentalSignIn
import de.roessing.app.data.BlockRequest
import de.roessing.app.data.BookingRequest
import de.roessing.app.data.BookingStatus
import de.roessing.app.data.LenderStatus
import de.roessing.app.data.LenderRequest
import de.roessing.app.data.OccupancyStatus
import de.roessing.app.data.ProfilePatch
import de.roessing.app.data.RentalApiException
import de.roessing.app.data.RentalBlock
import de.roessing.app.data.RentalAvailability
import de.roessing.app.data.RentalBooking
import de.roessing.app.data.RentalDevice
import de.roessing.app.data.RentalDeviceDetail
import de.roessing.app.data.RentalErrorCode
import de.roessing.app.data.RentalImage
import de.roessing.app.data.RentalOccupancy
import de.roessing.app.data.RentalOwnerBooking
import de.roessing.app.data.RentalOwnerDevice
import de.roessing.app.data.RentalPeriod
import de.roessing.app.data.RentalProfile
import de.roessing.app.data.RentalRepository
import de.roessing.app.data.RentalSet
import de.roessing.app.ui.RentalEvent
import de.roessing.app.ui.RentalPage
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

    // --- Sets, Profil und die Vermieterseite --------------------------------

    var sets: List<RentalSet> = emptyList()
    var setsFailure: Throwable? = null
    var lenderReceipt = LenderRequest(LenderStatus.PENDING, "Deine Anfrage wurde weitergeleitet.")
    var ownerBookings: List<RentalOwnerBooking> = emptyList()
    var ownerDevices: List<RentalOwnerDevice> = emptyList()
    var blocks: List<RentalBlock> = emptyList()

    var updateFailure: Throwable? = null
    var lenderFailure: Throwable? = null
    var ownerFailure: Throwable? = null
    var decideFailure: Throwable? = null
    var blockFailure: Throwable? = null

    var setsCalls = 0
    var ownerCalls = 0
    var lastPatch: ProfilePatch? = null
    var lastBlock: BlockRequest? = null
    var approved = mutableListOf<String>()
    var rejected = mutableListOf<String>()
    var removedBlocks = mutableListOf<String>()

    override suspend fun sets(): List<RentalSet> {
        setsCalls++
        setsFailure?.let { throw it }
        return sets
    }

    override suspend fun updateProfile(patch: ProfilePatch): RentalProfile {
        lastPatch = patch
        updateFailure?.let { throw it }
        // Die Plattform antwortet mit dem geänderten Profil — hier: das, was
        // sie behalten hat.
        profile = profile.copy(
            name = patch.name ?: profile.name,
            phone = patch.phone ?: profile.phone,
            addressStreet = patch.addressStreet ?: profile.addressStreet,
            addressZip = patch.addressZip ?: profile.addressZip,
            addressCity = patch.addressCity ?: profile.addressCity,
        )
        return profile
    }

    override suspend fun requestLender(): LenderRequest {
        lenderFailure?.let { throw it }
        return lenderReceipt
    }

    override suspend fun ownerBookings(): List<RentalOwnerBooking> {
        ownerCalls++
        ownerFailure?.let { throw it }
        return ownerBookings
    }

    override suspend fun approve(bookingId: String) {
        decideFailure?.let { throw it }
        approved += bookingId
    }

    override suspend fun reject(bookingId: String) {
        decideFailure?.let { throw it }
        rejected += bookingId
    }

    override suspend fun ownerDevices(): List<RentalOwnerDevice> {
        ownerFailure?.let { throw it }
        return ownerDevices
    }

    override suspend fun blocks(): List<RentalBlock> {
        ownerFailure?.let { throw it }
        return blocks
    }

    override suspend fun addBlock(request: BlockRequest): RentalBlock {
        lastBlock = request
        blockFailure?.let { throw it }
        val neu = RentalBlock(
            id = "sperre-neu",
            deviceId = request.deviceId,
            deviceName = "Kreiselmäher",
            period = request.period,
            reason = request.reason,
        )
        blocks = blocks + neu
        return neu
    }

    override suspend fun removeBlock(blockId: String) {
        blockFailure?.let { throw it }
        removedBlocks += blockId
        blocks = blocks.filterNot { it.id == blockId }
    }
}

private fun anfrage(
    id: String,
    canDecide: Boolean = true,
    canCancel: Boolean = true,
) = RentalOwnerBooking(
    id = id,
    deviceId = "maeher",
    deviceName = "Kreiselmäher",
    period = zeitraum(),
    status = if (canDecide) BookingStatus.PENDING else BookingStatus.APPROVED,
    rawStatus = if (canDecide) "pending" else "approved",
    renterName = "Erika Musterfrau",
    renterPhone = "+49 5069 123456",
    notes = null,
    canDecide = canDecide,
    canCancel = canCancel,
)

private fun sperre(id: String) = RentalBlock(
    id = id,
    deviceId = "maeher",
    deviceName = "Kreiselmäher",
    period = RentalPeriod(LocalDate.parse("2026-10-01"), LocalDate.parse("2026-10-08")),
    reason = "Eigener Einsatz",
)

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

private fun satz(id: String, name: String = "Gartenset") = RentalSet(
    id = id,
    name = name,
    description = "Vertikutierer, Rasenwalze und Streuwagen zusammen.",
    pricePerDay = 30.0,
    deposit = 150.0,
    itemIds = listOf("walze", "vertikutierer"),
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

private fun profil(
    missing: List<String> = emptyList(),
    lenderStatus: LenderStatus = LenderStatus.NONE,
) = RentalProfile(
    name = "Erika Musterfrau",
    email = "erika@example.invalid",
    phone = "+49 5069 123456",
    addressStreet = "Hauptstraße 1",
    addressZip = "31171",
    addressCity = "Nordstemmen",
    lender = lenderStatus == LenderStatus.APPROVED,
    lenderStatus = lenderStatus,
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

    // --- Sets ---------------------------------------------------------------

    @Test
    fun `die Sets stehen neben den Geraeten`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            devices = listOf(geraet("maeher"))
            sets = listOf(satz("gartenset"))
        }
        val vm = vm(repo)

        vm.load()
        advanceUntilIdle()

        assertEquals(listOf("gartenset"), vm.state.value.sets.map { it.id })
    }

    /**
     * Eine Suche fragt den Server nach Geräten; über Sets sagt ihre Antwort
     * nichts. Sie werden deshalb nur zur ganzen Liste geholt.
     */
    @Test
    fun `eine Suche holt keine Sets`() = runTest(dispatcher) {
        val repo = FakeRental().apply { sets = listOf(satz("gartenset")) }
        val vm = vm(repo)
        vm.load()
        advanceUntilIdle()
        val vorher = repo.setsCalls

        vm.setQuery("rasen")
        advanceUntilIdle()

        assertEquals(vorher, repo.setsCalls)
    }

    /** Fehlen die Sets, stehen die Geräte trotzdem da — ohne Hinweis. */
    @Test
    fun `ein Ausfall der Sets laesst die Geraete stehen`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            devices = listOf(geraet("maeher"))
            setsFailure = IOException("kein Netz")
        }
        val vm = vm(repo)

        vm.load()
        advanceUntilIdle()

        assertEquals(1, vm.state.value.devices.size)
        assertTrue(vm.state.value.sets.isEmpty())
        assertFalse(vm.state.value.offline)
    }

    // --- Mein Profil im Maschinchenring -------------------------------------

    @Test
    fun `das Profil schickt nur, was ausgefuellt ist`() = runTest(dispatcher) {
        val repo = FakeRental()
        val vm = vm(repo, RentalSignIn.VALID)
        val ereignisse = mutableListOf<RentalEvent>()
        val lauscher = launch { ereignisse += vm.events.first() }
        vm.load()
        advanceUntilIdle()

        vm.saveProfile(name = "Erika Musterfrau", phone = "  ", addressStreet = "Hauptstraße 1", addressZip = "", addressCity = "Nordstemmen")
        advanceUntilIdle()
        lauscher.join()

        // Ein leeres Feld ist kein Weg, einen Wert zu löschen — es bleibt weg.
        assertEquals("Erika Musterfrau", repo.lastPatch?.name)
        assertNull(repo.lastPatch?.phone)
        assertNull(repo.lastPatch?.addressZip)
        assertEquals(listOf(RentalEvent.ProfileSaved), ereignisse)
        assertFalse(vm.state.value.savingProfile)
    }

    @Test
    fun `ein leeres Formular geht gar nicht erst hinaus`() = runTest(dispatcher) {
        val repo = FakeRental()
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.saveProfile("", " ", "", "", "")
        advanceUntilIdle()

        assertNull(repo.lastPatch)
    }

    @Test
    fun `misslingt das Speichern, sagt es der Server und nicht die App`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            updateFailure = RentalApiException(RentalErrorCode.BAD_REQUEST, "Leerer Wert")
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.saveProfile("Erika", "", "", "", "")
        advanceUntilIdle()

        assertEquals(RentalErrorCode.BAD_REQUEST, vm.state.value.notice?.code)
        assertEquals("Leerer Wert", vm.state.value.notice?.text)
        assertFalse(vm.state.value.savingProfile)
    }

    /**
     * „Ich möchte auch verleihen" ist eine Eingangsbestätigung, keine
     * Freischaltung — entschieden wird drüben, von Hand.
     */
    @Test
    fun `die Anfrage als Verleiher zeigt den Satz der Plattform`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            lenderReceipt = LenderRequest(LenderStatus.PENDING, "Deine Anfrage liegt vor.")
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.askToLend()
        advanceUntilIdle()

        assertEquals("Deine Anfrage liegt vor.", vm.state.value.lenderMessage)
        assertFalse(vm.state.value.askingToLend)
    }

    @Test
    fun `eine zu schnelle Verleiher-Anfrage bleibt eine Auskunft`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            lenderFailure = RentalApiException(RentalErrorCode.RATE_LIMITED, "Später noch einmal")
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.askToLend()
        advanceUntilIdle()

        assertEquals(RentalErrorCode.RATE_LIMITED, vm.state.value.notice?.code)
        assertNull(vm.state.value.lenderMessage)
    }

    // --- Meine Vermietung ---------------------------------------------------

    /**
     * Die Vermieteransicht hängt an genau einem Feld der Plattform. Die App
     * rechnet daran nichts nach — auch nicht an der Zahl eigener Geräte.
     */
    @Test
    fun `die Vermieteransicht erscheint nur bei approved`() = runTest(dispatcher) {
        val repo = FakeRental().apply { profile = profil() }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        assertFalse(vm.state.value.showsLenderArea)
        assertTrue(vm.state.value.canAskToLend)

        repo.profile = profil(lenderStatus = LenderStatus.APPROVED)
        vm.loadProfile()
        advanceUntilIdle()

        assertTrue(vm.state.value.showsLenderArea)
        // Wer freigeschaltet ist, wird nicht mehr gefragt, ob er möchte.
        assertFalse(vm.state.value.canAskToLend)
    }

    @Test
    fun `die Vermieterseite holt Anfragen, Geraete und Sperren`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            profile = profil(lenderStatus = LenderStatus.APPROVED)
            ownerBookings = listOf(anfrage("a-1"), anfrage("a-2", canDecide = false))
            ownerDevices = listOf(RentalOwnerDevice(geraet("maeher"), active = false))
            blocks = listOf(sperre("s-1"))
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.showOwner()
        advanceUntilIdle()

        assertEquals(RentalPage.OWNER, vm.state.value.page)
        assertEquals(2, vm.state.value.ownerBookings.size)
        // Offen ist, was der Server für entscheidbar hält.
        assertEquals(1, vm.state.value.waitingRequests)
        assertEquals(1, vm.state.value.ownerDevices.size)
        assertFalse(vm.state.value.ownerDevices.first().active)
        assertEquals(1, vm.state.value.blocks.size)
        assertFalse(vm.state.value.ownerOffline)
    }

    /** Ein missglückter Abruf leert keine Liste. */
    @Test
    fun `ohne Netz bleiben die Anfragen stehen`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            profile = profil(lenderStatus = LenderStatus.APPROVED)
            ownerBookings = listOf(anfrage("a-1"))
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.showOwner()
        advanceUntilIdle()

        repo.ownerFailure = IOException("kein Netz")
        vm.loadOwner()
        advanceUntilIdle()

        assertTrue(vm.state.value.ownerOffline)
        assertEquals(1, vm.state.value.ownerBookings.size)
    }

    @Test
    fun `zusagen holt danach die Liste des Servers`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            profile = profil(lenderStatus = LenderStatus.APPROVED)
            ownerBookings = listOf(anfrage("a-1"))
        }
        val vm = vm(repo, RentalSignIn.VALID)
        val ereignisse = mutableListOf<RentalEvent>()
        val lauscher = launch { ereignisse += vm.events.first() }
        vm.load()
        advanceUntilIdle()
        vm.showOwner()
        advanceUntilIdle()
        val vorher = repo.ownerCalls

        vm.approve("a-1")
        advanceUntilIdle()
        lauscher.join()

        assertEquals(listOf("a-1"), repo.approved)
        assertEquals(listOf(RentalEvent.Approved), ereignisse)
        // Was gerade geschehen ist, weiß die Plattform besser als der Schirm.
        assertEquals(vorher + 1, repo.ownerCalls)
        assertTrue(vm.state.value.deciding.isEmpty())
    }

    @Test
    fun `absagen meldet es und holt die Liste ebenso`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            profile = profil(lenderStatus = LenderStatus.APPROVED)
            ownerBookings = listOf(anfrage("a-1"))
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.reject("a-1")
        advanceUntilIdle()

        assertEquals(listOf("a-1"), repo.rejected)
        assertTrue(vm.state.value.deciding.isEmpty())
    }

    @Test
    fun `zweimal auf Zusagen tippen sagt nicht zweimal zu`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            profile = profil(lenderStatus = LenderStatus.APPROVED)
            ownerBookings = listOf(anfrage("a-1"))
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.approve("a-1")
        vm.approve("a-1")
        advanceUntilIdle()

        assertEquals(listOf("a-1"), repo.approved)
    }

    @Test
    fun `eine nicht mehr offene Anfrage meldet den Grund des Servers`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            profile = profil(lenderStatus = LenderStatus.APPROVED)
            ownerBookings = listOf(anfrage("a-1"))
            decideFailure = RentalApiException(RentalErrorCode.CONFLICT, "Nicht mehr offen")
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.approve("a-1")
        advanceUntilIdle()

        assertEquals(RentalErrorCode.CONFLICT, vm.state.value.notice?.code)
        assertEquals("Nicht mehr offen", vm.state.value.notice?.text)
        assertTrue(vm.state.value.deciding.isEmpty())
    }

    // --- Sperren ------------------------------------------------------------

    /**
     * Der Kalender gibt die Tage her, an denen jemand sein Gerät braucht; der
     * Vertrag will den Rückgabetag. Umgerechnet wird an einer Stelle.
     */
    @Test
    fun `eine Sperre geht mit dem Rueckgabetag hinaus`() = runTest(dispatcher) {
        val repo = FakeRental().apply { profile = profil(lenderStatus = LenderStatus.APPROVED) }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.openBlock("maeher")
        vm.addBlock(
            RentalPeriod.ofPickedDays(LocalDate.parse("2026-10-01"), LocalDate.parse("2026-10-07")),
            "Eigener Einsatz",
        )
        advanceUntilIdle()

        assertEquals("maeher", repo.lastBlock?.deviceId)
        assertEquals(LocalDate.parse("2026-10-08"), repo.lastBlock?.period?.end)
        assertEquals("Eigener Einsatz", repo.lastBlock?.reason)
        // Erst wenn die Plattform die Sperre genommen hat, schließt das Blatt.
        assertNull(vm.state.value.blockDeviceId)
        assertEquals(1, vm.state.value.blocks.size)
    }

    @Test
    fun `eine abgelehnte Sperre laesst das Blatt offen`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            profile = profil(lenderStatus = LenderStatus.APPROVED)
            blockFailure = RentalApiException(RentalErrorCode.OCCUPIED, "Da liegt schon etwas")
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.openBlock("maeher")
        vm.addBlock(zeitraum("2026-10-01", "2026-10-08"))
        advanceUntilIdle()

        assertEquals("maeher", vm.state.value.blockDeviceId)
        assertEquals(RentalErrorCode.OCCUPIED, vm.state.value.notice?.code)
        assertFalse(vm.state.value.blocking)
    }

    @Test
    fun `ohne geoeffnetes Blatt wird nichts gesperrt`() = runTest(dispatcher) {
        val repo = FakeRental().apply { profile = profil(lenderStatus = LenderStatus.APPROVED) }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.addBlock(zeitraum("2026-10-01", "2026-10-08"))
        advanceUntilIdle()

        assertNull(repo.lastBlock)
    }

    @Test
    fun `eine Sperre laesst sich wieder aufheben`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            profile = profil(lenderStatus = LenderStatus.APPROVED)
            blocks = listOf(sperre("s-1"))
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.showOwner()
        advanceUntilIdle()

        vm.removeBlock("s-1")
        advanceUntilIdle()

        assertEquals(listOf("s-1"), repo.removedBlocks)
        assertTrue(vm.state.value.blocks.isEmpty())
        assertTrue(vm.state.value.deciding.isEmpty())
    }

    // --- Die Seiten des Bereichs --------------------------------------------

    @Test
    fun `das Profil wird beim Aufschlagen frisch geholt`() = runTest(dispatcher) {
        val repo = FakeRental()
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        repo.profile = profil(lenderStatus = LenderStatus.APPROVED)
        vm.showProfile()
        advanceUntilIdle()

        assertEquals(RentalPage.PROFILE, vm.state.value.page)
        assertEquals(LenderStatus.APPROVED, vm.state.value.profile?.lenderStatus)
    }

    /**
     * Die Anfragen tragen Namen und Nummern anderer Menschen. Wer nicht
     * angemeldet ist, darf sie nicht weiter auf dem Schirm haben — und steht
     * wieder im Katalog.
     */
    @Test
    fun `eine verlorene Anmeldung raeumt die Vermieterseite ab`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            profile = profil(lenderStatus = LenderStatus.APPROVED)
            ownerBookings = listOf(anfrage("a-1"))
            blocks = listOf(sperre("s-1"))
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()
        vm.showOwner()
        advanceUntilIdle()
        assertEquals(1, vm.state.value.ownerBookings.size)

        repo.ownerFailure = RentalApiException(RentalErrorCode.UNAUTHORIZED, null)
        vm.loadOwner()
        advanceUntilIdle()

        assertTrue(vm.state.value.ownerBookings.isEmpty())
        assertTrue(vm.state.value.blocks.isEmpty())
        assertEquals(RentalPage.CATALOG, vm.state.value.page)
        assertFalse(vm.state.value.signedIn)
    }

    /** Ein Token von vor der Umstellung heißt „neu anmelden", nicht „leer". */
    @Test
    fun `ein veraltetes Token auf der Vermieterseite bittet um Anmeldung`() = runTest(dispatcher) {
        val repo = FakeRental().apply {
            profile = profil(lenderStatus = LenderStatus.APPROVED)
            ownerFailure = RentalApiException(RentalErrorCode.TOKEN_AUDIENCE, null)
        }
        val vm = vm(repo, RentalSignIn.VALID)
        vm.load()
        advanceUntilIdle()

        vm.showOwner()
        advanceUntilIdle()

        assertTrue(vm.state.value.staleSignIn)
        assertEquals(RentalPage.CATALOG, vm.state.value.page)
    }
}
