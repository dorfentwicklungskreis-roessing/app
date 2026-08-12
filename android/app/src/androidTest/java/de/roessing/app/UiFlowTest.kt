package de.roessing.app

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import de.roessing.app.data.BadgeDto
import de.roessing.app.data.LeaderboardEntryDto
import de.roessing.app.data.LeaderboardTotalsDto
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.TaskDto
import de.roessing.app.ui.LeaderboardPeriod
import de.roessing.app.ui.LeaderboardScreen
import de.roessing.app.ui.LeaderboardUiState
import de.roessing.app.ui.LoginScreen
import de.roessing.app.ui.PlaceDetail
import de.roessing.app.ui.PlaceListScreen
import de.roessing.app.ui.PlacesUiState
import de.roessing.app.ui.theme.DorfAppTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** UI-Tests der Kern-Flows mit Fake-Daten (ohne Netzwerk). */
@RunWith(AndroidJUnit4::class)
class UiFlowTest {
    @get:Rule
    val compose = createComposeRule()

    private fun task(id: Long, kind: String, status: String) = TaskDto(
        id = id, placeId = 1, kind = kind, liters = if (kind == "giessen") 10.0 else null,
        intervalDays = 7.0, redAfterDays = 14.0, status = status,
    )

    private val places = listOf(
        PlaceDto(
            id = 1, name = "Unter den Eichen — Kasten 1", lat = 52.211, lon = 9.87,
            status = "red", tasks = listOf(task(11, "giessen", "red")),
        ),
        PlaceDto(
            id = 2, name = "Dorfbeet", kind = "beet", lat = 52.211, lon = 9.87,
            status = "green", tasks = listOf(task(21, "jaeten", "green")),
        ),
    )

    @Test
    fun loginScreen_zeigtAnmeldeButton() {
        compose.setContent {
            DorfAppTheme { LoginScreen(errorCode = null, onLogin = {}, onDevLogin = { }) }
        }
        compose.onNodeWithTag("login-button").assertIsDisplayed()
        compose.onNodeWithText("Mit Rössing-ID anmelden").assertIsDisplayed()
        compose.onNodeWithTag("login-error").assertDoesNotExist()
    }

    @Test
    fun loginScreen_zeigtFehlercodeAn() {
        compose.setContent {
            DorfAppTheme { LoginScreen(errorCode = "invalid_grant", onLogin = {}, onDevLogin = { }) }
        }
        compose.onNodeWithTag("login-error").assertIsDisplayed()
        compose.onNodeWithText("Anmeldung fehlgeschlagen (invalid_grant). Bitte erneut versuchen.")
            .assertIsDisplayed()
    }

    @Test
    fun liste_zeigtOrteUndOeffnetDetail() {
        var tapped: Long? = null
        compose.setContent {
            DorfAppTheme {
                PlaceListScreen(
                    state = PlacesUiState(places = places),
                    onPlaceTap = { tapped = it },
                )
            }
        }
        compose.onNodeWithText("Unter den Eichen — Kasten 1").assertIsDisplayed()
        compose.onNodeWithText("Dringend gießen!").assertIsDisplayed()
        compose.onNodeWithTag("place-card-1").performClick()
        assertEquals(1L, tapped)
    }

    @Test
    fun detail_meldetErledigungErstNachBestaetigung() {
        var completed: Pair<Long, Double?>? = null
        compose.setContent {
            DorfAppTheme {
                PlaceDetail(
                    place = places[0],
                    pendingTasks = emptySet(),
                    history = emptyMap(),
                    onComplete = { id, liters -> completed = id to liters },
                    onLoadHistory = {},
                )
            }
        }
        compose.onNodeWithText("10 Liter, alle 7 Tage").assertIsDisplayed()

        // Ein Klick fragt erst nach — gemeldet wird noch nichts.
        compose.onNodeWithTag("complete-task-11").performClick()
        compose.onNodeWithTag("confirm-completion").assertIsDisplayed()
        compose.onNodeWithText("Hast du wirklich gegossen?").assertIsDisplayed()
        compose.onNodeWithText("Ort: Unter den Eichen — Kasten 1").assertIsDisplayed()
        compose.onNodeWithText("Menge: 10 Liter").assertIsDisplayed()
        assertNull("Ohne Bestätigung darf nichts gemeldet werden", completed)

        // Abbrechen schließt den Dialog, ohne zu melden.
        compose.onNodeWithTag("confirm-completion-no").performClick()
        compose.onNodeWithTag("confirm-completion").assertDoesNotExist()
        assertNull("Nach Abbrechen darf nichts gemeldet sein", completed)

        // Erst die Bestätigung meldet.
        compose.onNodeWithTag("complete-task-11").performClick()
        compose.onNodeWithTag("confirm-completion-yes").performClick()
        assertEquals(11L to 10.0, completed)
        compose.onNodeWithTag("confirm-completion").assertDoesNotExist()
    }

    @Test
    fun detail_bestaetigungFragtBeimJaetenNachDemJaeten() {
        compose.setContent {
            DorfAppTheme {
                PlaceDetail(
                    place = places[1],
                    pendingTasks = emptySet(),
                    history = emptyMap(),
                    onComplete = { _, _ -> },
                    onLoadHistory = {},
                )
            }
        }
        compose.onNodeWithTag("complete-task-21").performClick()
        compose.onNodeWithText("Hast du wirklich gejätet?").assertIsDisplayed()
        compose.onNodeWithText("Ort: Dorfbeet").assertIsDisplayed()
    }

    @Test
    fun detail_jaeten_zeigtRichtigenButton() {
        compose.setContent {
            DorfAppTheme {
                PlaceDetail(
                    place = places[1],
                    pendingTasks = emptySet(),
                    history = emptyMap(),
                    onComplete = { _, _ -> },
                    onLoadHistory = {},
                )
            }
        }
        compose.onNodeWithText("Ich habe gejätet 🌿").assertIsDisplayed()
    }

    // --- Rangliste ---------------------------------------------------------

    private fun rangEintrag(rang: Int, sub: String, name: String, anzahl: Int, liter: Double = 0.0) =
        LeaderboardEntryDto(
            rank = rang, userSub = sub, userName = name, completions = anzahl,
            byKind = mapOf("giessen" to anzahl, "jaeten" to 0, "sonstiges" to 0), liters = liter,
        )

    private val rangliste = LeaderboardUiState(
        period = LeaderboardPeriod.SAISON,
        entries = listOf(
            rangEintrag(1, "erna", "Erna", 12, 120.0).copy(
                badges = listOf(BadgeDto("giesskanne", "Gießkanne des Monats", "Die meisten Gießungen.")),
            ),
            rangEintrag(2, "karl", "Karl", 8, 40.0),
            rangEintrag(3, "berta", "Berta", 5),
            rangEintrag(4, "udo", "Udo", 2),
        ),
        totals = LeaderboardTotalsDto(completions = 27, liters = 160.0, participants = 4),
        me = rangEintrag(4, "udo", "Udo", 2),
    )

    @Test
    fun rangliste_zeigtPodestListeUndAuszeichnungen() {
        compose.setContent {
            DorfAppTheme { LeaderboardScreen(state = rangliste, onSelectPeriod = {}) }
        }
        compose.onNodeWithTag("podium-1").assertIsDisplayed()
        compose.onNodeWithTag("podium-3").assertIsDisplayed()
        compose.onNodeWithText("Erna").assertIsDisplayed()
        // Alles unterhalb des Podests liegt je nach Bildschirmhöhe unter dem
        // Rand — deshalb erst hinscrollen, dann prüfen.
        compose.onNodeWithText("Gießkanne des Monats").performScrollTo().assertIsDisplayed()

        // Der eigene Platz ist hervorgehoben — auch außerhalb des Podests.
        compose.onNodeWithTag("leaderboard-row-udo").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Du").performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("leaderboard-me").performScrollTo().assertIsDisplayed()
        compose.onNodeWithText("Dein Platz: 4").assertIsDisplayed()
    }

    @Test
    fun rangliste_schaltetDenZeitraumUm() {
        var gewaehlt: LeaderboardPeriod? = null
        compose.setContent {
            DorfAppTheme { LeaderboardScreen(state = rangliste, onSelectPeriod = { gewaehlt = it }) }
        }
        compose.onNodeWithTag("period-woche").performClick()
        assertEquals(LeaderboardPeriod.WOCHE, gewaehlt)
    }

    @Test
    fun rangliste_zeigtLeereListeFreundlich() {
        compose.setContent {
            DorfAppTheme {
                LeaderboardScreen(
                    state = LeaderboardUiState(period = LeaderboardPeriod.WOCHE),
                    onSelectPeriod = {},
                )
            }
        }
        compose.onNodeWithTag("leaderboard-empty").assertIsDisplayed()
        compose.onNodeWithTag("podium-1").assertDoesNotExist()
    }
}
