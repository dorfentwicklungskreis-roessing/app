package de.roessing.app

import androidx.activity.ComponentActivity
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.platform.UriHandler
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.test.ext.junit.runners.AndroidJUnit4
import de.roessing.app.auth.RentalSignIn
import de.roessing.app.data.BookingRequest
import de.roessing.app.data.BookingStatus
import de.roessing.app.data.CompletionDto
import de.roessing.app.data.LeaderboardDto
import de.roessing.app.data.LenderStatus
import de.roessing.app.data.MeDto
import de.roessing.app.data.MemberDto
import de.roessing.app.data.OccupancyStatus
import de.roessing.app.data.PlacesRepository
import de.roessing.app.data.PlacesResponse
import de.roessing.app.data.ProfileDto
import de.roessing.app.data.ProfileInput
import de.roessing.app.data.ProfileRepository
import de.roessing.app.data.RentalAvailability
import de.roessing.app.data.RentalBooking
import de.roessing.app.data.RentalDevice
import de.roessing.app.data.RentalDeviceDetail
import de.roessing.app.data.RentalOccupancy
import de.roessing.app.data.RentalPeriod
import de.roessing.app.data.RentalPickup
import de.roessing.app.data.RentalProfile
import de.roessing.app.data.RentalRepository
import de.roessing.app.data.StatsRepository
import de.roessing.app.ui.HomeScreen
import de.roessing.app.ui.IdeenViewModel
import de.roessing.app.ui.LeaderboardViewModel
import de.roessing.app.ui.PlacesViewModel
import de.roessing.app.ui.ProfileViewModel
import de.roessing.app.ui.RentalViewModel
import de.roessing.app.ui.theme.DorfAppTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import java.io.IOException
import java.time.LocalDate

/**
 * Der „Maschinchenring" in der App: Von der Startseite führt eine Kachel zu
 * den Geräten, die eigenen Buchungen stehen darüber, und wenn die
 * Mietplattform gerade nicht erreichbar ist, steht ein Hinweis über der
 * zuletzt geholten Liste statt einer leeren Seite.
 *
 * Der Bereich redet mit einem eigenen Dienst; hier steht an dessen Stelle ein
 * Doppel im selben Prozess. Kein Netz, kein entfernter Server.
 */
@RunWith(AndroidJUnit4::class)
class RentalUiTest {
    @get:Rule
    val compose = createAndroidComposeRule<ComponentActivity>()

    private class FakePlaces : PlacesRepository {
        override suspend fun me() = MeDto(sub = "erna", name = "Erna Beispiel")
        override suspend fun places() = PlacesResponse(places = emptyList())
        override suspend fun complete(taskId: Long, liters: Double?, note: String) = CompletionDto()
        override suspend fun completions(taskId: Long): List<CompletionDto> = emptyList()
    }

    private class FakeStats : StatsRepository {
        override suspend fun leaderboard(period: String) = LeaderboardDto(period = period)
    }

    private class FakeProfile : ProfileRepository {
        override suspend fun profile() = ProfileDto(userSub = "erna", displayName = "Erna Beispiel")
        override suspend fun saveProfile(input: ProfileInput) = profile()
        override suspend fun members(): Pair<List<MemberDto>, Boolean> = emptyList<MemberDto>() to false
    }

    /** Die Mietplattform als Doppel — sie antwortet, sie entscheidet. */
    private class FakeRental(
        private val devices: List<RentalDevice> = emptyList(),
        private val bookings: List<RentalBooking> = emptyList(),
        private val available: Boolean = true,
        private val fehler: Boolean = false,
    ) : RentalRepository {
        var letzteAnfrage: BookingRequest? = null

        override suspend fun devices(): List<RentalDevice> {
            if (fehler) throw IOException("kein Netz")
            return devices
        }

        override suspend fun device(id: String) = RentalDeviceDetail(
            device = devices.first { it.id == id },
            images = emptyList(),
        )

        override suspend fun search(query: String): List<RentalDevice> {
            if (fehler) throw IOException("kein Netz")
            return devices.filter { it.name.contains(query, ignoreCase = true) }
        }

        override suspend fun occupancy(deviceId: String) = listOf(
            RentalOccupancy(
                deviceId = deviceId,
                setId = null,
                period = RentalPeriod(
                    LocalDate.parse("2026-09-12"),
                    LocalDate.parse("2026-09-14"),
                ),
                status = OccupancyStatus.APPROVED,
            ),
        )

        override suspend fun availability(deviceId: String, period: RentalPeriod) =
            RentalAvailability(period, available, if (available) null else "occupied")

        override suspend fun profile() = RentalProfile(
            name = "Erna Beispiel",
            email = "erna@example.invalid",
            phone = "+49 5069 123456",
            addressStreet = "Kirchstraße 3",
            addressZip = "31171",
            addressCity = "Nordstemmen",
            lender = false,
            lenderStatus = LenderStatus.NONE,
            profileComplete = true,
            missingFields = emptyList(),
        )

        override suspend fun myBookings() = bookings

        override suspend fun book(request: BookingRequest): RentalBooking {
            letzteAnfrage = request
            return bookings.first()
        }

        override suspend fun cancel(bookingId: String) = Unit
    }

    private val maeher = RentalDevice(
        id = "as-585-km-kreiselmaeher",
        name = "AS 585 KM Kreiselmäher",
        description = "Für hohes Gras und Böschungen.\n\n- Arbeitsbreite 85 cm",
        pricePerDay = 25.0,
        pricePerWeekend = 40.0,
        pricePerWeek = null,
        deposit = 100.0,
        tags = listOf("garten"),
        thumbnailUrl = null,
        productUrl = null,
        webUrl = "https://mieten.example.invalid/geraete/as-585-km-kreiselmaeher/",
    )

    private val walze = RentalDevice(
        id = "rasenwalze",
        name = "Rasenwalze",
        description = null,
        pricePerDay = 8.0,
        pricePerWeekend = null,
        pricePerWeek = null,
        deposit = null,
        tags = listOf("garten"),
        thumbnailUrl = null,
        productUrl = null,
        webUrl = null,
    )

    private val buchung = RentalBooking(
        id = "b-4711",
        deviceId = maeher.id,
        setId = null,
        deviceName = maeher.name,
        period = RentalPeriod(LocalDate.parse("2026-09-05"), LocalDate.parse("2026-09-07")),
        status = BookingStatus.APPROVED,
        rawStatus = "approved",
        notes = null,
        canCancel = true,
        pickup = RentalPickup("Hauptstraße 1, 31171 Nordstemmen", "+49 5069 123456"),
    )

    /**
     * Wohin die App hinausgeführt hat. Der echte UriHandler würde den Browser
     * starten — die App wäre im Hintergrund und der Test fände seine
     * Oberfläche nicht mehr wieder. Hier wird nur mitgeschrieben.
     */
    private val hinausgefuehrt = mutableListOf<String>()

    private val notizbuch = object : UriHandler {
        override fun openUri(uri: String) {
            hinausgefuehrt += uri
        }
    }

    private fun zeigeApp(repo: RentalRepository, anmeldung: RentalSignIn = RentalSignIn.VALID) {
        compose.setContent {
            DorfAppTheme {
                CompositionLocalProvider(LocalUriHandler provides notizbuch) {
                    HomeScreen(
                        viewModel = PlacesViewModel(FakePlaces(), FakeVergabeRepo()),
                        leaderboardViewModel = LeaderboardViewModel(FakeStats()),
                        profileViewModel = ProfileViewModel(FakeProfile()),
                        ideenViewModel = IdeenViewModel(FakeIdeen()),
                        rentalViewModel = RentalViewModel(repo, signIn = { anmeldung }),
                        onLogout = {},
                        onReauthenticate = {},
                    )
                }
            }
        }
        compose.waitForIdle()
    }

    private fun zumVerleih() {
        compose.onNodeWithTag("bereich-verleih").performScrollTo().performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("rental").assertIsDisplayed()
    }

    /** Blättert zu einer Gerätekachel und stellt fest, dass sie da ist. */
    private fun geraetIstDa(id: String) {
        compose.onNodeWithTag("rental-list").performScrollToNode(hasTestTag("device-$id"))
        compose.onNodeWithTag("device-$id").assertIsDisplayed()
    }

    /**
     * Öffnet ein Gerät.
     *
     * Erst blättern, dann tippen: Über den Geräten stehen die eigenen
     * Buchungen, und eine LazyColumn baut nur, was zu sehen ist — eine Kachel
     * weiter unten gibt es also noch gar nicht, wenn man nach ihr greift.
     */
    private fun oeffneGeraet(id: String) {
        compose.onNodeWithTag("rental-list").performScrollToNode(hasTestTag("device-$id"))
        compose.onNodeWithTag("device-$id").performClick()
        compose.waitForIdle()
    }

    @Test
    fun kachelAufDerStartseiteFuehrtZumMaschinchenring() {
        zeigeApp(FakeRental(devices = listOf(maeher, walze)))

        compose.onNodeWithTag("bereich-verleih").performScrollTo().assertIsDisplayed()
        zumVerleih()

        geraetIstDa(maeher.id)
        geraetIstDa(walze.id)
    }

    /** Der Preis steht als Tarif da — die App rechnet keine Summe daraus. */
    @Test
    fun einGeraetZeigtSeinenTagespreis() {
        zeigeApp(FakeRental(devices = listOf(maeher)))
        zumVerleih()

        compose.onNodeWithText("25 € pro Tag").assertIsDisplayed()
    }

    @Test
    fun ohneNetzStehtEinHinweisStattEinerLeerenSeite() {
        zeigeApp(FakeRental(fehler = true))
        zumVerleih()

        compose.onNodeWithTag("rental-offline").assertIsDisplayed()
    }

    @Test
    fun eigeneBuchungenStehenUeberDenGeraeten() {
        zeigeApp(FakeRental(devices = listOf(maeher), bookings = listOf(buchung)))
        zumVerleih()

        compose.onNodeWithTag("booking-b-4711").performScrollTo().assertIsDisplayed()
        // Die Abholadresse steht erst nach der Zusage da — hier steht sie.
        compose.onNodeWithText("Hauptstraße 1, 31171 Nordstemmen").assertIsDisplayed()
        compose.onNodeWithTag("booking-cancel-b-4711").assertIsDisplayed()
    }

    /**
     * Der Sonderfall, der wie ein Defekt aussieht: Das Gerät ist angemeldet,
     * aber mit einem Token von vor der Umstellung. Statt einer leeren Liste
     * steht dort ein Satz, den jemand befolgen kann.
     */
    @Test
    fun einVeraltetesTokenBittetUmEineNeueAnmeldung() {
        zeigeApp(FakeRental(devices = listOf(maeher)), anmeldung = RentalSignIn.STALE)
        zumVerleih()

        compose.onNodeWithTag("rental-stale").assertIsDisplayed()
        compose.onNodeWithTag("rental-stale-signin").assertIsDisplayed()
        // Die Geräte sind öffentlich und stehen trotzdem da.
        geraetIstDa(maeher.id)
    }

    /**
     * Ohne Zeitraum ist nichts anzufragen — und das ist keine Regel der App,
     * sondern schlicht eine unvollständige Anfrage.
     */
    @Test
    fun ohneZeitraumBleibtDerAnfrageknopfAus() {
        zeigeApp(FakeRental(devices = listOf(maeher)))
        zumVerleih()

        oeffneGeraet(maeher.id)

        compose.onNodeWithTag("device-detail").assertIsDisplayed()
        compose.onNodeWithTag("rental-book").performScrollTo().assertIsNotEnabled()
    }

    /**
     * Die Beschreibung kommt als Markdown und wird als Klartext gezeigt.
     *
     * Geprüft wird am Blatt, nicht am Wortlaut: Die Kachel in der Liste zeigt
     * dieselbe Beschreibung (nur gekürzt), und ein Text kommt damit zweimal
     * in der Oberfläche vor.
     */
    @Test
    fun eineBeschreibungStehtOhneSternchenDa() {
        zeigeApp(FakeRental(devices = listOf(maeher)))
        zumVerleih()

        oeffneGeraet(maeher.id)

        compose.onNodeWithTag("device-description")
            .assertTextContains("• Arbeitsbreite 85 cm", substring = true)
    }

    /**
     * Der Weg nach draußen führt in den Browser — im Test in ein Notizbuch,
     * sonst wäre die App im Hintergrund und der Test ohne Oberfläche.
     *
     * Der Verweis steht im Blatt zum Gerät, und ein Blatt ist ein eigenes
     * Fenster: Dort drinnen belegt Compose `LocalUriHandler` selbst wieder mit
     * dem Browser des Systems. Dieser Fall prüft mit, dass der Bereich den Weg
     * nach draußen davor nachschlägt — sonst geht hier wirklich Chromium auf.
     */
    @Test
    fun derWegZurWebfassungFuehrtNachDraussen() {
        zeigeApp(FakeRental(devices = listOf(maeher)))
        zumVerleih()

        oeffneGeraet(maeher.id)
        compose.onNodeWithTag("rental-web").performScrollTo().performClick()
        compose.waitForIdle()

        assertEquals(listOf(maeher.webUrl), hinausgefuehrt)
    }

    @Test
    fun belegteZeitraeumeStehenAmGeraet() {
        zeigeApp(FakeRental(devices = listOf(maeher)))
        zumVerleih()

        oeffneGeraet(maeher.id)

        compose.onNodeWithText("12.–13. September 2026 (vergeben)")
            .performScrollTo()
            .assertIsDisplayed()
    }

    /** Ohne Anmeldung gibt es keine Buchungen und keinen Anfrageknopf. */
    @Test
    fun ohneAnmeldungBleibtDasBuchenZu() {
        zeigeApp(FakeRental(devices = listOf(maeher)), anmeldung = RentalSignIn.MISSING)
        zumVerleih()

        geraetIstDa(maeher.id)
        compose.onNodeWithTag("booking-b-4711").assertDoesNotExist()
    }

    @Test
    fun einAnfrageknopfWirdErstFreiWennDerServerFreiSagt() {
        zeigeApp(FakeRental(devices = listOf(maeher), bookings = listOf(buchung)))
        zumVerleih()
        oeffneGeraet(maeher.id)

        compose.onNodeWithTag("rental-pick-period").performScrollTo().assertIsEnabled()
        compose.onNodeWithTag("rental-book").performScrollTo().assertIsNotEnabled()
    }
}
