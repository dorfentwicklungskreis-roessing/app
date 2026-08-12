package de.roessing.app

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.test.ext.junit.runners.AndroidJUnit4
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.TaskDto
import de.roessing.app.ui.LoginScreen
import de.roessing.app.ui.PlaceDetail
import de.roessing.app.ui.PlaceListScreen
import de.roessing.app.ui.PlacesUiState
import de.roessing.app.ui.theme.DorfAppTheme
import org.junit.Assert.assertEquals
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
    fun detail_meldetErledigung() {
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
        compose.onNodeWithTag("complete-task-11").performClick()
        assertEquals(11L to 10.0, completed)
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
}
