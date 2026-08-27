package de.roessing.app

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import de.roessing.app.auth.TokenResult
import de.roessing.app.data.ApiIdeenRepository
import de.roessing.app.data.DorfApi
import de.roessing.app.data.IdeeInput
import de.roessing.app.data.IdeenAblehnungException
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Echter End-to-End-Test der Ideen-Sammlung gegen ein laufendes Backend
 * (AUTH_MODE=insecure-dev). Nichts wird gemockt: Die App schickt ihre
 * Einreichung an denselben Eingang wie das Formular auf der Website.
 *
 * Läuft nur im E2E-Modus (`-e e2e true`), wie BackendE2eTest.
 */
@RunWith(AndroidJUnit4::class)
class IdeenBackendE2eTest {
    @Before
    fun onlyInE2eMode() {
        assumeTrue(
            "E2E-Modus nicht aktiv (Instrumentation-Arg e2e fehlt)",
            InstrumentationRegistry.getArguments().getString("e2e") == "true",
        )
    }

    private fun repo() = ApiIdeenRepository(
        DorfApi.create(BuildConfig.API_BASE_URL) { TokenResult.Token("e2e-idee:E2E Ideengeber:member") },
    )

    @Test
    fun eineIdeeAusDerAppLandetBeimKonto() = runBlocking {
        val wunsch = "E2E-App-Wunsch ${System.currentTimeMillis()}: ein Mitfahrbrett nach Hildesheim."
        val idee = repo().einreichen(
            IdeeInput(wunsch = wunsch, name = "E2E Ideengeber", email = "e2e@example.org"),
        )

        assertTrue("Die Idee hat keine ID bekommen", idee.id > 0)
        assertEquals(wunsch, idee.wunsch)
        // Angemeldet eingereicht → das Backend vermerkt den Weg „app".
        assertEquals("app", idee.quelle)
        assertEquals("neu", idee.status)
    }

    @Test
    fun einZuKurzerWunschWirdMitKlartextAbgelehnt() = runBlocking {
        try {
            repo().einreichen(IdeeInput(wunsch = "hm"))
            fail("Ein zu kurzer Wunsch wurde angenommen")
        } catch (e: IdeenAblehnungException) {
            // Die Begründung kommt im Klartext und ist für Menschen gedacht.
            assertTrue("Begründung ist leer: ${e.grund}", e.grund.isNotBlank())
        }
    }

    @Test
    fun eineKaputteEmailWirdAbgelehnt() = runBlocking {
        try {
            repo().einreichen(
                IdeeInput(
                    wunsch = "Ein Radweg nach Nordstemmen wäre großartig.",
                    email = "keine-mail",
                ),
            )
            fail("Eine kaputte E-Mail wurde angenommen")
        } catch (e: IdeenAblehnungException) {
            assertTrue("Begründung ist leer: ${e.grund}", e.grund.isNotBlank())
        }
    }
}
