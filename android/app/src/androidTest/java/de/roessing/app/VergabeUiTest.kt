package de.roessing.app

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import de.roessing.app.data.AssignmentDto
import de.roessing.app.data.NotificationDto
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.TaskDto
import de.roessing.app.ui.Bereich
import de.roessing.app.ui.PlaceDetail
import de.roessing.app.ui.PlaceListScreen
import de.roessing.app.ui.PlacesUiState
import de.roessing.app.ui.StartScreen
import de.roessing.app.ui.theme.DorfAppTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Oberfläche der Vergabe: sich eintragen, eine Anfrage sehen, zusagen — und
 * der Fall, in dem jemand anderes schneller war.
 */
@RunWith(AndroidJUnit4::class)
class VergabeUiTest {
    @get:Rule
    val compose = createComposeRule()

    private fun task(
        id: Long = 11,
        kind: String = "giessen",
        signedUp: Boolean = false,
        signupCount: Int = 0,
        assignment: AssignmentDto? = null,
    ) = TaskDto(
        id = id, placeId = 1, kind = kind, liters = if (kind == "giessen") 10.0 else null,
        intervalDays = 7.0, redAfterDays = 14.0, status = "yellow",
        signedUp = signedUp, signupCount = signupCount, assignment = assignment,
    )

    private fun ort(vararg tasks: TaskDto) = PlaceDto(
        id = 1, name = "Unter den Eichen — Kasten 1", lat = 52.211, lon = 9.87,
        status = "yellow", tasks = tasks.toList(),
    )

    private fun anfrage(id: Long = 1, kind: String = "anfrage") = NotificationDto(
        id = id, assignmentId = 5, taskId = 11, placeId = 1, kind = kind,
        taskName = "Gießen", placeName = "Unter den Eichen — Kasten 1",
        title = if (kind == "anfrage") "Gießen ist dran" else "Schon erledigt",
        text = "Du bist als Nächste(r) an der Reihe.",
        expiresAt = "2026-08-14T12:00:00Z",
    )

    // --- Eintragen als Helfer:in ---------------------------------------------

    @Test
    fun ortsdetail_zeigtDenSchalterZumEintragen() {
        var gemeldet: Triple<Long, String?, Boolean>? = null
        compose.setContent {
            DorfAppTheme {
                PlaceDetail(
                    place = ort(task()),
                    pendingTasks = emptySet(),
                    history = emptyMap(),
                    onComplete = { _, _ -> },
                    onLoadHistory = {},
                    onSignup = { p, art, an -> gemeldet = Triple(p, art, an) },
                )
            }
        }
        compose.onNodeWithText("Hier als Helfer:in eintragen").assertIsDisplayed()
        compose.onNodeWithTag("signup-switch").assertIsOff().performClick()
        assertEquals(Triple(1L, null, true), gemeldet)
    }

    @Test
    fun ortsdetail_zeigtDassIchSchonMithelfe() {
        compose.setContent {
            DorfAppTheme {
                PlaceDetail(
                    place = ort(task(signedUp = true, signupCount = 3)),
                    pendingTasks = emptySet(),
                    history = emptyMap(),
                    onComplete = { _, _ -> },
                    onLoadHistory = {},
                )
            }
        }
        compose.onNodeWithText("Du hilfst hier mit").assertIsDisplayed()
        compose.onNodeWithTag("signup-switch").assertIsOn()
        compose.onNodeWithText("3 helfen hier mit").assertIsDisplayed()
    }

    // Gibt es an einem Ort Gießen und Jäten, lässt sich das trennen.
    @Test
    fun ortsdetail_bietetDieAuswahlDerAufgabenart() {
        var gemeldet: Triple<Long, String?, Boolean>? = null
        compose.setContent {
            DorfAppTheme {
                PlaceDetail(
                    place = ort(task(id = 11, kind = "giessen"), task(id = 12, kind = "jaeten")),
                    pendingTasks = emptySet(),
                    history = emptyMap(),
                    onComplete = { _, _ -> },
                    onLoadHistory = {},
                    onSignup = { p, art, an -> gemeldet = Triple(p, art, an) },
                )
            }
        }
        compose.onNodeWithTag("signup-kind-giessen").performScrollTo().performClick()
        compose.onNodeWithTag("signup-switch").performClick()
        assertEquals(Triple(1L, "giessen", true), gemeldet)
    }

    @Test
    fun ortsdetail_ohneZweiteAufgabenartKeineAuswahl() {
        compose.setContent {
            DorfAppTheme {
                PlaceDetail(
                    place = ort(task()),
                    pendingTasks = emptySet(),
                    history = emptyMap(),
                    onComplete = { _, _ -> },
                    onLoadHistory = {},
                )
            }
        }
        compose.onNodeWithTag("signup-kind-giessen").assertDoesNotExist()
    }

    // --- Vergabestand ---------------------------------------------------------

    @Test
    fun ortsdetail_zeigtWerUebernommenHat() {
        val vorgang = AssignmentDto(
            id = 5, taskId = 11, state = "uebernommen", claimedBy = "bernd",
            claimedByName = "Bernd", claimedUntil = "2026-08-15T09:00:00Z",
        )
        compose.setContent {
            DorfAppTheme {
                PlaceDetail(
                    place = ort(task(assignment = vorgang, signupCount = 2)),
                    pendingTasks = emptySet(),
                    history = emptyMap(),
                    onComplete = { _, _ -> },
                    onLoadHistory = {},
                    meinSub = "anna",
                )
            }
        }
        compose.onNodeWithTag("vergabe-stand-11").assertIsDisplayed()
        // Wer schon vergeben ist, lässt sich nicht noch einmal übernehmen.
        compose.onNodeWithTag("claim-task-11").assertDoesNotExist()
    }

    @Test
    fun ortsdetail_erlaubtZurueckgebenDerEigenenZusage() {
        var zurueck: Long? = null
        val vorgang = AssignmentDto(
            id = 5, taskId = 11, state = "uebernommen", claimedBy = "anna",
            claimedByName = "Anna", claimedUntil = "2026-08-15T09:00:00Z",
        )
        compose.setContent {
            DorfAppTheme {
                PlaceDetail(
                    place = ort(task(assignment = vorgang)),
                    pendingTasks = emptySet(),
                    history = emptyMap(),
                    onComplete = { _, _ -> },
                    onLoadHistory = {},
                    meinSub = "anna",
                    onRelease = { zurueck = it },
                )
            }
        }
        compose.onNodeWithTag("release-task-11").performScrollTo().performClick()
        assertEquals(5L, zurueck)
    }

    @Test
    fun ortsdetail_erlaubtZusagenBeiOffenemVorgang() {
        var zugesagt: Long? = null
        val vorgang = AssignmentDto(id = 5, taskId = 11, state = "offen")
        compose.setContent {
            DorfAppTheme {
                PlaceDetail(
                    place = ort(task(assignment = vorgang, signedUp = true)),
                    pendingTasks = emptySet(),
                    history = emptyMap(),
                    onComplete = { _, _ -> },
                    onLoadHistory = {},
                    meinSub = "anna",
                    onClaim = { zugesagt = it },
                )
            }
        }
        compose.onNodeWithTag("claim-task-11").performScrollTo().performClick()
        assertEquals(5L, zugesagt)
    }

    // Der Vergabestand steht auch in der Liste — ohne die Karte zu öffnen.
    @Test
    fun liste_zeigtDenVergabestand() {
        val vorgang = AssignmentDto(
            id = 5, taskId = 11, state = "uebernommen", claimedBy = "anna",
            claimedByName = "Anna", claimedUntil = "2026-08-15T09:00:00Z",
        )
        compose.setContent {
            DorfAppTheme {
                PlaceListScreen(
                    state = PlacesUiState(
                        places = listOf(ort(task(assignment = vorgang, signupCount = 2))),
                        me = de.roessing.app.data.MeDto(sub = "anna", name = "Anna"),
                    ),
                    onPlaceTap = {},
                )
            }
        }
        // Die Karte fasst ihre Beschriftungen zu einem Knoten zusammen (sie
        // ist anklickbar) — deshalb der Blick in den unverschmolzenen Baum.
        compose.onNodeWithTag("vergabe-1", useUnmergedTree = true).assertExists()
        // Ohne die Uhrzeit: Die hängt an der Zeitzone des Geräts.
        compose.onNodeWithText("Du hast zugesagt", substring = true, useUnmergedTree = true)
            .assertExists()
    }

    // --- Benachrichtigungen auf der Startseite --------------------------------

    @Test
    fun startseite_zeigtAnfrageMitFristUndKnopf() {
        var zugesagt: Long? = null
        compose.setContent {
            DorfAppTheme {
                StartScreen(
                    name = "Anna", faelligeOrte = 1, ladend = false,
                    notifications = listOf(anfrage()),
                    onClaim = { zugesagt = it },
                    onBereich = {},
                )
            }
        }
        compose.onNodeWithTag("benachrichtigungen").assertIsDisplayed()
        compose.onNodeWithText("Gießen ist dran").assertIsDisplayed()
        compose.onNodeWithText("Du bist als Nächste(r) an der Reihe.").assertIsDisplayed()
        compose.onNodeWithTag("claim-5").performScrollTo().performClick()
        assertEquals(5L, zugesagt)
        // Die Statuszeile der Kachel nennt die Anfrage.
        compose.onNodeWithText("Eine Anfrage wartet auf dich").assertIsDisplayed()
    }

    @Test
    fun startseite_zeigtBeiEigenerZusageDasZurueckgeben() {
        var zurueck: Long? = null
        compose.setContent {
            DorfAppTheme {
                StartScreen(
                    name = "Anna", faelligeOrte = 1, ladend = false,
                    notifications = listOf(anfrage()),
                    meineVorgaenge = setOf(5L),
                    onRelease = { zurueck = it },
                    onBereich = {},
                )
            }
        }
        compose.onNodeWithTag("claim-5").assertDoesNotExist()
        compose.onNodeWithTag("release-5").performScrollTo().performClick()
        assertEquals(5L, zurueck)
    }

    // Hinweise verschwinden auf „Verstanden"; Anfragen haben den Knopf nicht.
    @Test
    fun startseite_hinweisLaesstSichBestaetigen() {
        var bestaetigt: Long? = null
        compose.setContent {
            DorfAppTheme {
                StartScreen(
                    name = "Anna", faelligeOrte = 0, ladend = false,
                    notifications = listOf(anfrage(id = 9, kind = "vorgang_beendet")),
                    onAck = { bestaetigt = it },
                    onBereich = {},
                )
            }
        }
        compose.onNodeWithTag("claim-5").assertDoesNotExist()
        compose.onNodeWithTag("ack-9").performScrollTo().performClick()
        assertEquals(9L, bestaetigt)
    }

    @Test
    fun startseite_ohneBenachrichtigungenKeinAbschnitt() {
        compose.setContent {
            DorfAppTheme {
                StartScreen(name = "Anna", faelligeOrte = 0, ladend = false, onBereich = {})
            }
        }
        compose.onNodeWithTag("benachrichtigungen").assertDoesNotExist()
        compose.onNodeWithText("Alles erledigt — danke!").assertIsDisplayed()
    }

    // Während ein Aufruf läuft, sind die Knöpfe gesperrt — sonst sagt ein
    // Doppeltipp zweimal zu.
    @Test
    fun startseite_sperrtDenKnopfWaehrendDesAufrufs() {
        compose.setContent {
            DorfAppTheme {
                StartScreen(
                    name = "Anna", faelligeOrte = 1, ladend = false,
                    notifications = listOf(anfrage()),
                    pendingAssignments = setOf(5L),
                    onBereich = {},
                )
            }
        }
        compose.onNodeWithTag("claim-5").performScrollTo().assertIsNotEnabled()
    }

    @Test
    fun bereiche_sindUnveraendertErreichbar() {
        var gewaehlt: Bereich? = null
        compose.setContent {
            DorfAppTheme {
                StartScreen(
                    name = "Anna", faelligeOrte = 0, ladend = false,
                    notifications = listOf(anfrage()),
                    onBereich = { gewaehlt = it },
                )
            }
        }
        assertNull(gewaehlt)
        compose.onNodeWithTag("bereich-mithelfen").performScrollTo().performClick()
        assertTrue(gewaehlt == Bereich.MITHELFEN)
    }
}
