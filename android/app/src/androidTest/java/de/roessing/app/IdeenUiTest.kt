package de.roessing.app

import androidx.activity.ComponentActivity
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertTextContains
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextClearance
import androidx.compose.ui.test.performTextInput
import androidx.test.ext.junit.runners.AndroidJUnit4
import de.roessing.app.data.CompletionDto
import de.roessing.app.data.LeaderboardDto
import de.roessing.app.data.MeDto
import de.roessing.app.data.MemberDto
import de.roessing.app.data.PlacesRepository
import de.roessing.app.data.PlacesResponse
import de.roessing.app.data.ProfileDto
import de.roessing.app.data.ProfileInput
import de.roessing.app.data.ProfileRepository
import de.roessing.app.data.StatsRepository
import de.roessing.app.ui.HomeScreen
import de.roessing.app.ui.IdeenViewModel
import de.roessing.app.ui.VerwaltungViewModel
import de.roessing.app.ui.LeaderboardViewModel
import de.roessing.app.ui.PlacesViewModel
import de.roessing.app.ui.ProfileViewModel
import de.roessing.app.ui.theme.DorfAppTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Die Ideen-Sammlung in der App: Von der Startseite führt eine sichtbare
 * Kachel zum Formular, Name und E-Mail sind aus dem Profil vorbelegt, und
 * ein abgelehnter Versuch kostet den getippten Text nicht.
 */
@RunWith(AndroidJUnit4::class)
class IdeenUiTest {
    @get:Rule
    val compose = createAndroidComposeRule<ComponentActivity>()

    private class FakePlaces : PlacesRepository {
        override suspend fun me() = MeDto(sub = "erna", name = "Erna Beispiel", email = "erna@example.org")
        override suspend fun places() = PlacesResponse(places = emptyList())
        override suspend fun complete(taskId: Long, liters: Double?, note: String) = CompletionDto()
        override suspend fun completions(taskId: Long): List<CompletionDto> = emptyList()
    }

    private class FakeStats : StatsRepository {
        override suspend fun leaderboard(period: String) = LeaderboardDto(period = period)
    }

    private class FakeProfile : ProfileRepository {
        override suspend fun profile() = ProfileDto(
            userSub = "erna", displayName = "Erna Beispiel", email = "erna@example.org",
        )

        override suspend fun saveProfile(input: ProfileInput) = profile()
        override suspend fun members(): Pair<List<MemberDto>, Boolean> = emptyList<MemberDto>() to false
    }

    private fun zeigeApp(ideen: FakeIdeen): FakeIdeen {
        compose.setContent {
            DorfAppTheme {
                HomeScreen(
                    viewModel = PlacesViewModel(FakePlaces(), FakeVergabeRepo()),
                    leaderboardViewModel = LeaderboardViewModel(FakeStats()),
                    profileViewModel = ProfileViewModel(FakeProfile()),
                    ideenViewModel = IdeenViewModel(ideen),
                    verwaltungViewModel = VerwaltungViewModel(FakeVerwaltung()),
                    onLogout = {},
                )
            }
        }
        compose.waitForIdle()
        return ideen
    }

    private fun insFormular() {
        compose.onNodeWithTag("bereich-ideen").performScrollTo().performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("ideen-formular").assertIsDisplayed()
    }

    @Test
    fun kachelAufDerStartseiteFuehrtInsFormular() {
        zeigeApp(FakeIdeen())
        compose.onNodeWithTag("bereich-ideen").performScrollTo().assertIsDisplayed()
        insFormular()
        // Der Datenschutzhinweis steht am Formular, nicht im Kleingedruckten.
        compose.onNodeWithTag("ideen-datenschutz").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun nameUndEmailKommenAusDemProfil() {
        zeigeApp(FakeIdeen())
        insFormular()
        // Die Felder stehen unter dem Wunschfeld — auf kleinen Bildschirmen
        // also erst nach einem Scroll im Bild.
        compose.onNodeWithTag("feld-name").performScrollTo().assertTextContains("Erna Beispiel")
        compose.onNodeWithTag("feld-email").performScrollTo().assertTextContains("erna@example.org")
    }

    @Test
    fun absendenBrauchtEinenWunsch() {
        val ideen = zeigeApp(FakeIdeen())
        insFormular()
        compose.onNodeWithTag("idee-absenden").performScrollTo().assertIsNotEnabled()

        compose.onNodeWithTag("feld-wunsch").performScrollTo()
            .performTextInput("Ein Mitfahrbrett für Fahrten nach Hildesheim.")
        compose.waitForIdle()
        compose.onNodeWithTag("idee-absenden").performScrollTo().assertIsEnabled().performClick()
        compose.waitForIdle()

        assertEquals(1, ideen.geschickt.size)
        val geschickt = ideen.geschickt.single()
        assertEquals("Ein Mitfahrbrett für Fahrten nach Hildesheim.", geschickt.wunsch)
        assertEquals("Erna Beispiel", geschickt.name)
        assertEquals("erna@example.org", geschickt.email)
    }

    @Test
    fun eineAblehnungKostetDenTextNicht() {
        zeigeApp(FakeIdeen(ablehnung = "Die E-Mail-Adresse sieht nicht richtig aus."))
        insFormular()
        compose.onNodeWithTag("feld-email").performScrollTo().performTextClearance()
        compose.onNodeWithTag("feld-email").performTextInput("keine-mail")
        compose.onNodeWithTag("feld-wunsch").performScrollTo()
            .performTextInput("Ein Mitfahrbrett für Fahrten nach Hildesheim.")
        compose.waitForIdle()
        compose.onNodeWithTag("idee-absenden").performScrollTo().performClick()
        compose.waitForIdle()

        compose.onNodeWithTag("ideen-fehler").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Die E-Mail-Adresse sieht nicht richtig aus.").assertIsDisplayed()
        // Der Wunsch steht noch da.
        compose.onNodeWithTag("feld-wunsch").performScrollTo()
            .assertTextContains("Ein Mitfahrbrett für Fahrten nach Hildesheim.")
    }

    @Test
    fun zurueckFuehrtAufDieStartseite() {
        zeigeApp(FakeIdeen())
        insFormular()
        compose.onNodeWithTag("zurueck").performClick()
        compose.waitForIdle()
        compose.onNodeWithTag("startseite").assertIsDisplayed()
        assertTrue(true)
    }
}
