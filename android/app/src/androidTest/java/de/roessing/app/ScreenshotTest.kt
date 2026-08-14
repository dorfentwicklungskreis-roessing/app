package de.roessing.app

import android.os.SystemClock
import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.UiDevice
import de.roessing.app.data.CompletionDto
import de.roessing.app.data.LeaderboardDto
import de.roessing.app.data.LeaderboardEntryDto
import de.roessing.app.data.LeaderboardTotalsDto
import de.roessing.app.data.MeDto
import de.roessing.app.data.MemberDto
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.PlacesRepository
import de.roessing.app.data.PlacesResponse
import de.roessing.app.data.ProfileDto
import de.roessing.app.data.ProfileInput
import de.roessing.app.data.ProfileRepository
import de.roessing.app.data.StatsRepository
import de.roessing.app.data.TaskDto
import de.roessing.app.ui.HomeScreen
import de.roessing.app.ui.LeaderboardViewModel
import de.roessing.app.ui.PlacesViewModel
import de.roessing.app.ui.IdeenViewModel
import de.roessing.app.ui.VerwaltungViewModel
import de.roessing.app.ui.ProfileViewModel
import de.roessing.app.ui.theme.DorfAppTheme
import java.io.File
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Macht Bildschirmfotos der neuen Oberfläche — echte App auf dem Emulator,
 * nur die Daten kommen aus Attrappen (ein Backend hat der Testrechner nicht).
 *
 * Läuft ausschließlich auf Zuruf:
 *
 *     ./gradlew connectedDebugAndroidTest \
 *       -Pandroid.testInstrumentationRunnerArguments.class=de.roessing.app.ScreenshotTest \
 *       -Pandroid.testInstrumentationRunnerArguments.screenshots=true
 *
 * Die Bilder landen unter /sdcard/Android/data/de.roessing.app/files/ui-neu/.
 */
@RunWith(AndroidJUnit4::class)
class ScreenshotTest {
    @get:Rule
    val compose = createAndroidComposeRule<ComponentActivity>()

    @Before
    fun nurAufZuruf() {
        assumeTrue(
            "Bildschirmfotos nur mit -e screenshots true",
            InstrumentationRegistry.getArguments().getString("screenshots") == "true",
        )
    }

    private fun task(id: Long, kind: String, status: String, liter: Double? = 10.0) = TaskDto(
        id = id, placeId = id / 10, kind = kind, liters = liter,
        intervalDays = 7.0, redAfterDays = 14.0, status = status,
        lastCompletion = CompletionDto(userName = "Karl", doneAt = "2026-08-09T17:20:00Z", liters = liter),
    )

    private val orte = listOf(
        PlaceDto(
            id = 1, name = "Unter den Eichen — Kasten 1", lat = 52.183159, lon = 9.816763,
            status = "red", tasks = listOf(task(11, "giessen", "red")),
        ),
        PlaceDto(
            id = 2, name = "Brunnen am Anger", lat = 52.1846, lon = 9.8185,
            status = "yellow", tasks = listOf(task(21, "giessen", "yellow", 8.0)),
        ),
        PlaceDto(
            id = 3, name = "Dorfbeet an der Kirche", kind = "beet", lat = 52.1838, lon = 9.8142,
            status = "green", tasks = listOf(task(31, "jaeten", "green", null)),
        ),
        PlaceDto(
            id = 4, name = "Bushaltestelle Hauptstraße", lat = 52.1855, lon = 9.8203,
            status = "green", tasks = listOf(task(41, "giessen", "green", 12.0)),
        ),
    )

    private inner class FakePlaces : PlacesRepository {
        override suspend fun me() = MeDto(sub = "erna", name = "Erna Beispiel")
        override suspend fun places() = PlacesResponse(places = orte)
        override suspend fun complete(taskId: Long, liters: Double?, note: String) = CompletionDto()
        override suspend fun completions(taskId: Long): List<CompletionDto> = emptyList()
    }

    private fun eintrag(rang: Int, sub: String, name: String, anzahl: Int, liter: Double) =
        LeaderboardEntryDto(
            rank = rang, userSub = sub, userName = name, completions = anzahl,
            byKind = mapOf("giessen" to anzahl, "jaeten" to 0), liters = liter,
        )

    private inner class FakeStats : StatsRepository {
        override suspend fun leaderboard(period: String) = LeaderboardDto(
            period = period,
            entries = listOf(
                eintrag(1, "karl", "Karl", 14, 140.0),
                eintrag(2, "erna", "Erna", 9, 95.0),
                eintrag(3, "berta", "Berta", 6, 55.0),
                eintrag(4, "udo", "Udo", 2, 20.0),
            ),
            totals = LeaderboardTotalsDto(completions = 31, liters = 310.0, participants = 4),
            me = eintrag(2, "erna", "Erna", 9, 95.0),
        )
    }

    private inner class FakeProfile : ProfileRepository {
        override suspend fun profile() = ProfileDto(
            userSub = "erna", displayName = "Erna Beispiel", nickname = "Erna",
            email = "erna@example.org",
        )

        override suspend fun saveProfile(input: ProfileInput) = profile()
        override suspend fun members(): Pair<List<MemberDto>, Boolean> = listOf(
            MemberDto(userSub = "karl", name = "Karl", phone = "05066 123456"),
            MemberDto(userSub = "berta", name = "Berta", email = "berta@example.org"),
        ) to false
    }

    // Bewusst der interne Speicher: Auf /sdcard/Android/data kommt die
    // adb-Shell seit Android 11 nicht mehr heran, an den App-Ordner eines
    // Debug-Builds dagegen schon (`adb exec-out run-as …`).
    private val ordner: File
        get() = File(
            InstrumentationRegistry.getInstrumentation().targetContext.filesDir,
            "ui-neu",
        ).apply { mkdirs() }

    private fun foto(name: String, wartenMs: Long = 600) {
        compose.waitForIdle()
        SystemClock.sleep(wartenMs)
        val geraet = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
        geraet.takeScreenshot(File(ordner, "$name.png"))
    }

    @Test
    fun bildschirmfotos() {
        compose.setContent {
            DorfAppTheme {
                HomeScreen(
                    viewModel = PlacesViewModel(FakePlaces(), FakeVergabeRepo()),
                    leaderboardViewModel = LeaderboardViewModel(FakeStats()),
                    profileViewModel = ProfileViewModel(FakeProfile()),
                    ideenViewModel = IdeenViewModel(FakeIdeen()),
                    verwaltungViewModel = VerwaltungViewModel(FakeVerwaltung()),
                    onLogout = {},
                )
            }
        }
        foto("01-startseite")

        compose.onNodeWithTag("bereich-mithelfen").performClick()
        // Die Karte lädt ihre Kacheln aus dem Netz — ihr etwas Zeit lassen.
        foto("02-mithelfen-karte", wartenMs = 6_000)

        compose.onNodeWithTag("tab-list").performClick()
        foto("03-mithelfen-liste")

        compose.onNodeWithTag("tab-leaderboard").performClick()
        foto("04-rangliste", wartenMs = 1_200)

        zurueck()
        compose.onNodeWithTag("bereich-profil").performClick()
        foto("05-profil")

        zurueck()
        compose.onNodeWithTag("bereich-dorfbewohner").performClick()
        foto("06-dorfbewohner")

        // Zum Schluss das Detail-Blatt — es liegt über allem und müsste
        // sonst erst wieder weggeräumt werden.
        zurueck()
        compose.onNodeWithTag("bereich-mithelfen").performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("tab-list").performClick()
        compose.onNodeWithTag("place-card-1").performClick()
        foto("07-ort-detail", wartenMs = 1_500)
    }

    private fun zurueck() {
        compose.runOnUiThread { compose.activity.onBackPressedDispatcher.onBackPressed() }
        compose.waitForIdle()
    }
}
