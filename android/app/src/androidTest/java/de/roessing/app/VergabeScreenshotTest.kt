package de.roessing.app

import android.os.SystemClock
import androidx.activity.ComponentActivity
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.UiDevice
import de.roessing.app.data.AssignmentDto
import de.roessing.app.data.CompletionDto
import de.roessing.app.data.LeaderboardDto
import de.roessing.app.data.MeDto
import de.roessing.app.data.MemberDto
import de.roessing.app.data.NotificationDto
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
import de.roessing.app.ui.ProfileViewModel
import de.roessing.app.ui.theme.DorfAppTheme
import java.io.File
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Bildschirmfotos der Vergabe auf dem Emulator: Anfrage auf der Startseite,
 * Eintrag als Helfer:in im Ortsdetail, Vergabestand in der Liste.
 *
 * Auf Zuruf:
 *
 *     ./gradlew connectedDebugAndroidTest \
 *       -Pandroid.testInstrumentationRunnerArguments.class=de.roessing.app.VergabeScreenshotTest \
 *       -Pandroid.testInstrumentationRunnerArguments.screenshots=true
 */
@RunWith(AndroidJUnit4::class)
class VergabeScreenshotTest {
    @get:Rule
    val compose = createAndroidComposeRule<ComponentActivity>()

    @Before
    fun nurAufZuruf() {
        assumeTrue(
            "Bildschirmfotos nur mit -e screenshots true",
            InstrumentationRegistry.getArguments().getString("screenshots") == "true",
        )
    }

    private val vorgang = AssignmentDto(
        id = 5, taskId = 11, state = "offen", askedCount = 1,
    )

    private val fremd = AssignmentDto(
        id = 6, taskId = 21, state = "uebernommen", claimedBy = "karl",
        claimedByName = "Karl", claimedUntil = "2026-08-15T09:00:00Z",
    )

    private val orte = listOf(
        PlaceDto(
            id = 1, name = "Unter den Eichen — Kasten 1", lat = 52.183159, lon = 9.816763,
            status = "red",
            tasks = listOf(
                TaskDto(
                    id = 11, placeId = 1, kind = "giessen", liters = 10.0, status = "red",
                    intervalDays = 7.0, redAfterDays = 14.0, signedUp = true, signupCount = 3,
                    assignment = vorgang,
                    lastCompletion = CompletionDto(userName = "Karl", doneAt = "2026-08-01T17:20:00Z", liters = 10.0),
                ),
                TaskDto(
                    id = 12, placeId = 1, kind = "jaeten", status = "green",
                    intervalDays = 30.0, redAfterDays = 60.0, signupCount = 1,
                ),
            ),
        ),
        PlaceDto(
            id = 2, name = "Brunnen am Anger", lat = 52.1846, lon = 9.8185,
            status = "yellow",
            tasks = listOf(
                TaskDto(
                    id = 21, placeId = 2, kind = "giessen", liters = 8.0, status = "yellow",
                    intervalDays = 7.0, redAfterDays = 14.0, signupCount = 2, assignment = fremd,
                ),
            ),
        ),
        PlaceDto(
            id = 3, name = "Dorfbeet an der Kirche", kind = "beet", lat = 52.1838, lon = 9.8142,
            status = "green",
            tasks = listOf(TaskDto(id = 31, placeId = 3, kind = "jaeten", status = "green")),
        ),
    )

    private val benachrichtigungen = listOf(
        NotificationDto(
            id = 1, assignmentId = 5, taskId = 11, placeId = 1, kind = "anfrage",
            taskName = "Gießen", placeName = "Unter den Eichen — Kasten 1",
            title = "Gießen an „Unter den Eichen — Kasten 1“ ist dran",
            text = "Du bist als Nächste(r) an der Reihe: Gießen an „Unter den Eichen — Kasten 1“. " +
                "Wenn du zusagst, hast du 24 Stunden Zeit.",
            expiresAt = "2026-08-14T12:00:00Z",
            acknowledgedAt = "2026-08-14T11:00:00Z",
        ),
        NotificationDto(
            id = 2, assignmentId = 7, taskId = 31, placeId = 3, kind = "vorgang_beendet",
            taskName = "Jäten", placeName = "Dorfbeet an der Kirche",
            title = "Schon erledigt",
            text = "Jäten an „Dorfbeet an der Kirche“ wurde bereits erledigt — " +
                "du musst nichts mehr tun. Danke trotzdem!",
            acknowledgedAt = "2026-08-14T11:00:00Z",
        ),
    )

    private inner class FakePlaces : PlacesRepository {
        override suspend fun me() = MeDto(sub = "erna", name = "Erna Beispiel")
        override suspend fun places() = PlacesResponse(places = orte)
        override suspend fun complete(taskId: Long, liters: Double?, note: String) = CompletionDto()
        override suspend fun completions(taskId: Long): List<CompletionDto> = emptyList()
    }

    private inner class FakeStats : StatsRepository {
        override suspend fun leaderboard(period: String) = LeaderboardDto(period = period)
    }

    private inner class FakeProfile : ProfileRepository {
        override suspend fun profile() = ProfileDto(userSub = "erna", displayName = "Erna Beispiel", nickname = "Erna")
        override suspend fun saveProfile(input: ProfileInput) = profile()
        override suspend fun members(): Pair<List<MemberDto>, Boolean> = emptyList<MemberDto>() to false
    }

    private val ordner: File
        get() = File(
            InstrumentationRegistry.getInstrumentation().targetContext.filesDir,
            "push-neu",
        ).apply { mkdirs() }

    private fun foto(name: String, wartenMs: Long = 700) {
        compose.waitForIdle()
        SystemClock.sleep(wartenMs)
        UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
            .takeScreenshot(File(ordner, "$name.png"))
    }

    @Test
    fun bildschirmfotos() {
        compose.setContent {
            DorfAppTheme {
                HomeScreen(
                    viewModel = PlacesViewModel(FakePlaces(), FakeVergabeRepo(benachrichtigungen)),
                    leaderboardViewModel = LeaderboardViewModel(FakeStats()),
                    profileViewModel = ProfileViewModel(FakeProfile()),
                    onLogout = {},
                )
            }
        }
        // 1) Startseite mit Anfrage und Hinweis.
        foto("01-startseite-benachrichtigungen")

        // 2) Liste mit Vergabestand („übernommen von …", „3 helfen hier mit").
        compose.onNodeWithTag("bereich-mithelfen").performClick()
        compose.onNodeWithTag("tab-list").performClick()
        foto("02-liste-vergabestand")

        // 3) Ortsdetail mit Schalter „Du hilfst hier mit" und Zusage-Knopf.
        compose.onNodeWithTag("place-card-1").performClick()
        foto("03-ortsdetail-helfer")

        // 4) Ein Ort ohne eigenen Eintrag: „Hier als Helfer:in eintragen".
        compose.onNodeWithTag("place-detail").performScrollTo()
        UiDevice.getInstance(InstrumentationRegistry.getInstrumentation()).pressBack()
        compose.waitForIdle()
        compose.onNodeWithTag("place-card-2").performClick()
        foto("04-ortsdetail-eintragen")
    }
}
