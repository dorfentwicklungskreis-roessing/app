package de.roessing.app

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import de.roessing.app.ui.LoginScreen
import de.roessing.app.ui.Rechtsdokument
import de.roessing.app.ui.RechtlichesLeiste
import de.roessing.app.ui.StartScreen
import de.roessing.app.ui.theme.DorfAppTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Impressum und Datenschutzerklärung sind Pflicht (§ 5 DDG) — und zwar
 * unmittelbar erreichbar. „Unmittelbar" heißt hier: schon vom
 * Anmeldebildschirm aus, ohne Konto und ohne Umweg.
 */
@RunWith(AndroidJUnit4::class)
class RechtlichesUiTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun anmeldebildschirm_fuehrtZuImpressumUndDatenschutz() {
        compose.setContent {
            DorfAppTheme { LoginScreen(onLogin = {}, onDevLogin = {}) }
        }

        compose.onNodeWithTag("rechtliches-impressum").performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("rechtliches-datenschutz").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun startseite_fuehrtZuImpressumUndDatenschutz() {
        compose.setContent {
            DorfAppTheme {
                StartScreen(name = "Erna", faelligeOrte = 0, ladend = false, onBereich = {})
            }
        }

        compose.onNodeWithTag("rechtliches-impressum").performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("rechtliches-datenschutz").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun antippen_meldetWelcheSeiteGemeintIst() {
        var gewaehlt: Rechtsdokument? = null
        compose.setContent {
            DorfAppTheme { RechtlichesLeiste(online = true, onOeffnen = { gewaehlt = it }) }
        }

        compose.onNodeWithTag("rechtliches-datenschutz").performClick()

        assertEquals(Rechtsdokument.DATENSCHUTZ, gewaehlt)
    }

    /**
     * Ohne Netz führt der Knopf ins Leere. Ein Hinweis mit der Adresse zum
     * Nachschlagen ist ehrlicher als ein Tipp, der nichts tut.
     */
    @Test
    fun ohneNetz_stehtEinHinweisStattEinesTotenKnopfes() {
        compose.setContent {
            DorfAppTheme { RechtlichesLeiste(online = false, onOeffnen = {}) }
        }

        compose.onNodeWithTag("rechtliches-offline").assertIsDisplayed()
        compose.onNodeWithTag("rechtliches-impressum").assertIsNotEnabled()
        compose.onNodeWithTag("rechtliches-datenschutz").assertIsNotEnabled()
    }

    @Test
    fun mitNetz_keinHinweisSondernEinKnopfDerGeht() {
        compose.setContent {
            DorfAppTheme { RechtlichesLeiste(online = true, onOeffnen = {}) }
        }

        compose.onNodeWithTag("rechtliches-offline").assertDoesNotExist()
        compose.onNodeWithTag("rechtliches-impressum").assertIsEnabled()
    }
}
