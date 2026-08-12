package de.roessing.app

import android.content.Intent
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import de.roessing.app.auth.AuthManager
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Reproduziert den Absturz auf dem Rückweg aus dem Browser.
 *
 * Kommt aus dem Login ein leeres Ergebnis zurück (Abbruch, verworfener Task,
 * Prozess-Neustart), enthält der Intent WEDER eine AuthorizationResponse NOCH
 * eine AuthorizationException. Der frühere Code baute daraus trotzdem sofort ein
 * `AuthState(resp, ex)` — dessen Konstruktor verlangt aber genau eines von beiden
 * und wirft sonst eine IllegalArgumentException. Ergebnis: unbehandelte Exception,
 * die App wurde „wiederholt beendet".
 *
 * Der Test prüft bewusst nur, dass KEINE Exception fliegt (nicht den Rückgabewert),
 * damit er auch gegen den alten Code kompiliert — dort schlägt er fehl, hier ist er
 * grün.
 */
@RunWith(AndroidJUnit4::class)
class AuthResultRobustnessTest {

    private lateinit var authManager: AuthManager

    @Before
    fun setUp() {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        authManager = AuthManager(context)
    }

    @Test
    fun leeresErgebnis_stuerztNichtAb() {
        val ergebnis = runCatching { runBlocking { authManager.handleAuthResult(Intent()) } }
        assertTrue(
            "Leeres Login-Ergebnis darf keine Exception werfen, warf aber: ${ergebnis.exceptionOrNull()}",
            ergebnis.isSuccess,
        )
    }

    @Test
    fun fehlendesErgebnis_stuerztNichtAb() {
        val ergebnis = runCatching { runBlocking { authManager.handleAuthResult(null) } }
        assertTrue(
            "Fehlendes Login-Ergebnis darf keine Exception werfen, warf aber: ${ergebnis.exceptionOrNull()}",
            ergebnis.isSuccess,
        )
    }

    @Test
    fun ergebnisMitUnbekanntenExtras_stuerztNichtAb() {
        // Ein Intent, der zwar Daten trägt, aber keine AppAuth-Extras enthält —
        // genau der Fall, in dem resp und ex beide null bleiben.
        val intent = Intent().putExtra("irgendwas", "egal")
        val ergebnis = runCatching { runBlocking { authManager.handleAuthResult(intent) } }
        assertTrue(
            "Unbekanntes Login-Ergebnis darf keine Exception werfen, warf aber: ${ergebnis.exceptionOrNull()}",
            ergebnis.isSuccess,
        )
    }
}
