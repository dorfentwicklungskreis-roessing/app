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
import de.roessing.app.ui.ProfileViewModel
import de.roessing.app.ui.theme.DorfAppTheme
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Die App ist die Dorf-App — nicht die Gieß-App.
 *
 * Diese Tests halten die Struktur fest: Ganz oben steht eine Startseite mit
 * Bereichen, „Mithelfen" ist einer davon (der erste), und der Weg dorthin
 * und zurück muss mit der System-Zurück-Taste funktionieren.
 */
@RunWith(AndroidJUnit4::class)
class StartNavigationTest {
    @get:Rule
    val compose = createAndroidComposeRule<ComponentActivity>()

    private fun ort(id: Long, status: String) = PlaceDto(
        id = id, name = "Ort $id", lat = 52.183, lon = 9.816, status = status,
        tasks = listOf(TaskDto(id = id * 10, placeId = id, kind = "giessen", status = status)),
    )

    private class FakePlaces(private val orte: List<PlaceDto>) : PlacesRepository {
        override suspend fun me() = MeDto(sub = "erna", name = "Erna Beispiel")
        override suspend fun places() = PlacesResponse(places = orte)
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

    private fun zeigeApp(orte: List<PlaceDto>) {
        compose.setContent {
            DorfAppTheme {
                HomeScreen(
                    viewModel = PlacesViewModel(FakePlaces(orte), FakeVergabeRepo()),
                    leaderboardViewModel = LeaderboardViewModel(FakeStats()),
                    profileViewModel = ProfileViewModel(FakeProfile()),
                    ideenViewModel = IdeenViewModel(FakeIdeen()),
                    onLogout = {},
                )
            }
        }
        compose.waitForIdle()
    }

    private fun zurueckDruecken() {
        compose.runOnUiThread { compose.activity.onBackPressedDispatcher.onBackPressed() }
        compose.waitForIdle()
    }

    @Test
    fun startseite_zeigtDieBereicheStattDerKarte() {
        zeigeApp(listOf(ort(1, "green")))

        // Von oben nach unten gelesen. Die Startseite wächst mit jedem
        // Bereich, und was gestern noch über der Falz stand, liegt heute
        // darunter — deshalb steht zuerst, was oben ist, und alles Weitere
        // wird herangeblättert. Ohne das meldet ein neuer Bereich seinen
        // Einzug als Fehler in einem Fall, der mit ihm nichts zu tun hat.
        compose.onNodeWithText("Moin, Erna!").assertIsDisplayed()
        compose.onNodeWithTag("bereich-mithelfen").assertIsDisplayed()
        // Der Reiter-Balken gehört in den Bereich, nicht auf die oberste Ebene.
        compose.onNodeWithTag("tab-map").assertDoesNotExist()
        compose.onNodeWithTag("bereich-profil").performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("bereich-dorfbewohner").performScrollTo().assertIsDisplayed()
        // Kein Versprechen, aber die Einladung, Wünsche zu äußern: Die
        // Ausblick-Kachel führt inzwischen ins Ideen-Formular.
        compose.onNodeWithTag("bereich-ideen").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun startseite_zaehltDieFaelligenOrte() {
        zeigeApp(listOf(ort(1, "red"), ort(2, "yellow"), ort(3, "green")))

        // Die Statuszeile liegt in der anklickbaren Kachel und wird für
        // Sprachausgaben mit ihr verschmolzen — deshalb der ungemergte Baum.
        compose.onNodeWithTag("mithelfen-status", useUnmergedTree = true).assertIsDisplayed()
        compose.onNodeWithText("2 Orte warten auf dich").assertIsDisplayed()
    }

    @Test
    fun startseite_sagtWennAllesErledigtIst() {
        zeigeApp(listOf(ort(1, "green"), ort(2, "green")))

        compose.onNodeWithText("Alles erledigt — danke!").assertIsDisplayed()
    }

    @Test
    fun mithelfen_fuehrtZuKarteListeUndRangliste() {
        zeigeApp(listOf(ort(1, "red")))

        compose.onNodeWithTag("bereich-mithelfen").performClick()
        compose.waitForIdle()

        compose.onNodeWithTag("tab-map").assertIsDisplayed()
        compose.onNodeWithTag("tab-list").assertIsDisplayed()
        compose.onNodeWithTag("tab-leaderboard").assertIsDisplayed()
        compose.onNodeWithTag("bereich-mithelfen").assertDoesNotExist()
    }

    @Test
    fun systemZurueck_fuehrtAusDemBereichAufDieStartseite() {
        zeigeApp(listOf(ort(1, "red")))

        compose.onNodeWithTag("bereich-mithelfen").performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("tab-list").assertIsDisplayed()

        zurueckDruecken()

        compose.onNodeWithTag("bereich-mithelfen").assertIsDisplayed()
        compose.onNodeWithTag("tab-map").assertDoesNotExist()
    }

    @Test
    fun profil_istEinBereichUndZurueckFuehrtHeim() {
        zeigeApp(listOf(ort(1, "green")))

        compose.onNodeWithTag("bereich-profil").performScrollTo().performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("sichtbarkeitshinweis").assertIsDisplayed()

        zurueckDruecken()

        compose.onNodeWithTag("bereich-mithelfen").assertIsDisplayed()
    }

    @Test
    fun dorfbewohner_istEinBereichUndZurueckFuehrtHeim() {
        zeigeApp(listOf(ort(1, "green")))

        compose.onNodeWithTag("bereich-dorfbewohner").performScrollTo().performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("dorfbewohner").assertIsDisplayed()

        zurueckDruecken()

        // Die Startseite steht danach wieder oben — ihre Blätterstellung
        // überlebt den Ausflug in den Bereich nicht.
        compose.onNodeWithTag("bereich-dorfbewohner").performScrollTo().assertIsDisplayed()
    }
}
