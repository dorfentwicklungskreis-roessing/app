package de.roessing.app

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextReplacement
import androidx.test.ext.junit.runners.AndroidJUnit4
import de.roessing.app.data.MemberDto
import de.roessing.app.ui.MembersScreen
import de.roessing.app.ui.ProfileScreen
import de.roessing.app.ui.ProfileUiState
import de.roessing.app.ui.theme.DorfAppTheme
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Compose-Tests der Profilseite und der Dorfbewohner-Liste. */
@RunWith(AndroidJUnit4::class)
class ProfileUiTest {
    @get:Rule
    val compose = createComposeRule()

    private fun zeige(state: ProfileUiState, onSave: () -> Unit = {}, onPhonePublic: (Boolean) -> Unit = {}) {
        compose.setContent {
            DorfAppTheme {
                ProfileScreen(
                    state = state,
                    onDisplayName = {}, onNickname = {}, onPhone = {}, onEmail = {}, onNote = {},
                    onDisplayNamePublic = {}, onNicknamePublic = {}, onPhonePublic = onPhonePublic,
                    onEmailPublic = {}, onNotePublic = {},
                    onSave = onSave,
                )
            }
        }
    }

    // Der Hinweis darf nicht im Kleingedruckten stehen: Er ist das Erste auf
    // der Seite und im ersten Bild sichtbar, ohne zu scrollen.
    @Test
    fun profil_zeigtDenHinweisSofort() {
        zeige(ProfileUiState(loading = false, displayName = "Erna"))

        compose.onNodeWithTag("sichtbarkeitshinweis").assertIsDisplayed()
        compose.onNodeWithText("Das sehen andere").assertIsDisplayed()
        compose.onNodeWithTag("sichtbar-jetzt").assertIsDisplayed()
    }

    @Test
    fun profil_zaehltAufWasGeradeSichtbarIst() {
        zeige(
            ProfileUiState(
                loading = false, displayName = "Erna", phone = "05066 1234",
                displayNamePublic = true, phonePublic = true,
            ),
        )
        compose.onNodeWithText("Für alle sichtbar: Anzeigename, Telefonnummer").assertIsDisplayed()
    }

    @Test
    fun profil_ohneFreigabeSagtDasDeutlich() {
        zeige(
            ProfileUiState(
                loading = false, displayName = "Erna",
                displayNamePublic = false, nicknamePublic = false,
            ),
        )
        compose.onNodeWithText(
            "Für alle sichtbar: nichts. Nur die Verwaltenden sehen deine Angaben.",
        ).assertIsDisplayed()
    }

    // Die Vorbelegung der Schalter ist die eigentliche Zusage.
    @Test
    fun profil_kontaktdatenSindNichtVorgehakt() {
        zeige(ProfileUiState(loading = false))

        compose.onNodeWithTag("sicht-anzeigename").performScrollTo().assertIsOn()
        compose.onNodeWithTag("sicht-nickname").performScrollTo().assertIsOn()
        compose.onNodeWithTag("sicht-telefon").performScrollTo().assertIsOff()
        compose.onNodeWithTag("sicht-email").performScrollTo().assertIsOff()
        compose.onNodeWithTag("sicht-notiz").performScrollTo().assertIsOff()
    }

    @Test
    fun profil_schalterMeldetUmlegen() {
        var gemeldet: Boolean? = null
        zeige(ProfileUiState(loading = false), onPhonePublic = { gemeldet = it })

        compose.onNodeWithTag("sicht-telefon").performScrollTo().performClick()
        assertEquals(true, gemeldet)
    }

    @Test
    fun profil_speichernMeldetSich() {
        var gespeichert = false
        zeige(ProfileUiState(loading = false), onSave = { gespeichert = true })

        compose.onNodeWithTag("profil-speichern").performScrollTo().performClick()
        assertTrue(gespeichert)
    }

    @Test
    fun profil_zeigtDieBegruendungDesBackends() {
        zeige(ProfileUiState(loading = false, error = "email ist keine gültige E-Mail-Adresse"))

        compose.onNodeWithTag("profil-fehler").assertIsDisplayed()
        compose.onNodeWithText("email ist keine gültige E-Mail-Adresse").assertIsDisplayed()
    }

    @Test
    fun profil_feldeingabeGehtDurch() {
        var eingetippt = ""
        compose.setContent {
            DorfAppTheme {
                ProfileScreen(
                    state = ProfileUiState(loading = false),
                    onDisplayName = {}, onNickname = { eingetippt = it }, onPhone = {},
                    onEmail = {}, onNote = {},
                    onDisplayNamePublic = {}, onNicknamePublic = {}, onPhonePublic = {},
                    onEmailPublic = {}, onNotePublic = {}, onSave = {},
                )
            }
        }
        compose.onNodeWithTag("feld-nickname").performScrollTo().performTextReplacement("Gießmeisterin")
        assertEquals("Gießmeisterin", eingetippt)
    }

    // --- Dorfbewohner-Liste ---

    private val bewohner = listOf(
        MemberDto(
            userSub = "erna", name = "Gießmeisterin", displayName = "Erna Beispiel",
            nickname = "Gießmeisterin", phone = "05066 123456", email = "erna@example.org",
        ),
        MemberDto(userSub = "karl", name = "Karl"),
    )

    @Test
    fun dorfbewohner_zeigtFreigegebeneKontaktdaten() {
        compose.setContent {
            DorfAppTheme { MembersScreen(state = ProfileUiState(loading = false, members = bewohner)) }
        }
        compose.onNodeWithText("Gießmeisterin").assertIsDisplayed()
        compose.onNodeWithTag("anrufen-erna").assertIsDisplayed()
        compose.onNodeWithTag("mailen-erna").assertIsDisplayed()
        // Karl hat nichts freigegeben — also gibt es auch nichts anzutippen.
        compose.onNodeWithTag("anrufen-karl").assertDoesNotExist()
        compose.onNodeWithTag("mailen-karl").assertDoesNotExist()
    }

    @Test
    fun dorfbewohner_kennzeichnetWasNurVerwaltendeSehen() {
        val gesperrt = listOf(
            MemberDto(
                userSub = "erna", name = "Erna", phone = "05066 123456",
                restricted = listOf("phone"),
            ),
        )
        compose.setContent {
            DorfAppTheme {
                MembersScreen(state = ProfileUiState(loading = false, members = gesperrt, adminView = true))
            }
        }
        compose.onNodeWithTag("verwaltungs-hinweis").assertIsDisplayed()
        compose.onNodeWithTag("nur-verwaltung").assertIsDisplayed()
    }

    @Test
    fun dorfbewohner_leereListeSagtBescheid() {
        compose.setContent {
            DorfAppTheme { MembersScreen(state = ProfileUiState(loading = false)) }
        }
        compose.onNodeWithTag("dorfbewohner-leer").assertIsDisplayed()
    }
}
