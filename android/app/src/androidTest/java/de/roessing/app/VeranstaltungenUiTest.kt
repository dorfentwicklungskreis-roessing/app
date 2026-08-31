package de.roessing.app

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import de.roessing.app.data.CompletionDto
import de.roessing.app.data.LeaderboardDto
import de.roessing.app.data.MeDto
import de.roessing.app.data.MemberDto
import de.roessing.app.data.OrtDto
import de.roessing.app.data.PlacesRepository
import de.roessing.app.data.PlacesResponse
import de.roessing.app.data.ProfileDto
import de.roessing.app.data.ProfileInput
import de.roessing.app.data.ProfileRepository
import de.roessing.app.data.StatsRepository
import de.roessing.app.data.VeranstalterDto
import de.roessing.app.data.VeranstaltungDto
import de.roessing.app.data.VeranstaltungenRepository
import de.roessing.app.ui.HomeScreen
import de.roessing.app.ui.IdeenViewModel
import de.roessing.app.ui.LeaderboardViewModel
import de.roessing.app.ui.PlacesViewModel
import de.roessing.app.ui.ProfileViewModel
import de.roessing.app.ui.VeranstaltungenViewModel
import de.roessing.app.ui.theme.DorfAppTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import java.time.Instant

/**
 * „Was ist los in Rössing" in der App: Von der Startseite führt eine Kachel
 * zu den Terminen, ganztägige Termine kommen ohne erfundene Uhrzeit aus, und
 * wenn nichts geladen werden kann, steht dort ein Hinweis statt einer leeren
 * Seite.
 */
@RunWith(AndroidJUnit4::class)
class VeranstaltungenUiTest {
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

    private class FakeTermine(
        private val termine: List<VeranstaltungDto>,
        private val fehler: Boolean = false,
    ) : VeranstaltungenRepository {
        override suspend fun kommende(): List<VeranstaltungDto> {
            if (fehler) throw RuntimeException("kein Netz")
            return termine
        }
    }

    private val blutspende = VeranstaltungDto(
        id = "2026-08-17-blutspende",
        name = "Blutspende im Dorfgemeinschaftshaus",
        description = "DRK-Blutspende in Rössing.",
        start = "2026-08-17",
        allDay = true,
        url = "https://xn--rssing-wxa.de/events/2026-08-17-blutspende",
        location = OrtDto(name = "Dorfgemeinschaftshaus Rössing"),
        organizer = VeranstalterDto(name = "DRK Rössing"),
    )

    /** Fremde Primärquelle: Der Tipp führt nach draußen, nicht in die App. */
    private val kreisfest = VeranstaltungDto(
        id = "2026-08-22-kreisfest",
        name = "Kreisfest in Nordstemmen",
        description = "Nicht unsere Veranstaltung.",
        start = "2026-08-22",
        allDay = true,
        url = "https://nordstemmen.example/kreisfest",
        external = true,
        organizer = VeranstalterDto(name = "Gemeinde Nordstemmen"),
    )

    private val grillen = VeranstaltungDto(
        id = "2026-08-20-grillen",
        name = "Grillen im Pfarrgarten",
        description = "Die Kirchenstiftung lädt ein.",
        start = "2026-08-20T18:00:00+02:00",
        url = "https://xn--rssing-wxa.de/events/2026-08-20-grillen",
    )

    private fun zeigeApp(repo: VeranstaltungenRepository) {
        compose.setContent {
            DorfAppTheme {
                HomeScreen(
                    viewModel = PlacesViewModel(FakePlaces(), FakeVergabeRepo()),
                    leaderboardViewModel = LeaderboardViewModel(FakeStats()),
                    profileViewModel = ProfileViewModel(FakeProfile()),
                    ideenViewModel = IdeenViewModel(FakeIdeen()),
                    veranstaltungenViewModel = VeranstaltungenViewModel(
                        repo,
                        uhr = { Instant.parse("2026-08-14T10:00:00Z") },
                    ),
                    onLogout = {},
                )
            }
        }
        compose.waitForIdle()
    }

    private fun zuDenTerminen() {
        compose.onNodeWithTag("bereich-veranstaltungen").performScrollTo().performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("veranstaltungen").assertIsDisplayed()
    }

    @Test
    fun kachelAufDerStartseiteFuehrtZuDenTerminen() {
        zeigeApp(FakeTermine(listOf(blutspende, grillen)))

        compose.onNodeWithTag("bereich-veranstaltungen").performScrollTo().assertIsDisplayed()
        zuDenTerminen()

        compose.onNodeWithTag("termin-2026-08-17-blutspende").assertIsDisplayed()
        compose.onNodeWithTag("termin-2026-08-20-grillen").assertIsDisplayed()
    }

    @Test
    fun ganztaegigerTerminZeigtKeineErfundeneUhrzeit() {
        zeigeApp(FakeTermine(listOf(blutspende)))
        zuDenTerminen()

        compose.onNodeWithText("Mo, 17.08.2026").assertIsDisplayed()
        compose.onNodeWithText("Ganztägig").assertIsDisplayed()
    }

    @Test
    fun terminMitUhrzeitZeigtDieOrtszeit() {
        zeigeApp(FakeTermine(listOf(grillen)))
        zuDenTerminen()

        compose.onNodeWithText("18:00 Uhr").assertIsDisplayed()
    }

    @Test
    fun einTerminDesDorfesZeigtSeineEinzelheitenInDerApp() {
        zeigeApp(FakeTermine(listOf(blutspende)))
        zuDenTerminen()

        compose.onNodeWithTag("termin-2026-08-17-blutspende").performClick()
        compose.waitForIdle()

        compose.onNodeWithTag("termin-detail").assertIsDisplayed()
        compose.onNodeWithTag("termin-beschreibung").assertIsDisplayed()
        compose.onNodeWithText("Dorfgemeinschaftshaus Rössing").assertIsDisplayed()
        compose.onNodeWithText("DRK Rössing").assertIsDisplayed()
        // Ganztägig heißt: keine erfundene Uhrzeit, auch hier nicht.
        compose.onNodeWithText("Ganztägig").assertIsDisplayed()
    }

    @Test
    fun ausDemTerminFuehrtZurueckInDieListe() {
        zeigeApp(FakeTermine(listOf(blutspende, grillen)))
        zuDenTerminen()

        compose.onNodeWithTag("termin-2026-08-17-blutspende").performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("termin-detail").assertIsDisplayed()

        compose.onNodeWithTag("zurueck").performClick()
        compose.waitForIdle()

        // Zurück in die Liste, nicht auf die Startseite: ein Schritt, nicht zwei.
        compose.onNodeWithTag("veranstaltungen").assertIsDisplayed()
        compose.onNodeWithTag("termin-2026-08-20-grillen").assertIsDisplayed()
    }

    @Test
    fun einFremderTerminBleibtBeiSeinerQuelle() {
        zeigeApp(FakeTermine(listOf(kreisfest)))
        zuDenTerminen()

        compose.onNodeWithTag("termin-2026-08-22-kreisfest").performClick()
        compose.waitForIdle()

        // Keine zweite Fassung in der App — der Tipp geht nach draußen.
        compose.onNodeWithTag("termin-detail").assertDoesNotExist()
        compose.onNodeWithTag("veranstaltungen").assertIsDisplayed()
    }

    @Test
    fun ohneNetzStehtEinHinweisStattEinerLeerenSeite() {
        zeigeApp(FakeTermine(emptyList(), fehler = true))
        zuDenTerminen()

        compose.onNodeWithTag("veranstaltungen-offline").assertIsDisplayed()
        compose.onNodeWithTag("veranstaltungen-erneut").assertIsDisplayed()
    }
}
