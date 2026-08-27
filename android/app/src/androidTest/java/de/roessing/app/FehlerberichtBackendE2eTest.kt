package de.roessing.app

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import de.roessing.app.auth.TokenResult
import de.roessing.app.data.ApiErrorReportRepository
import de.roessing.app.data.DorfApi
import de.roessing.app.errors.DeviceFacts
import de.roessing.app.errors.ErrorIncident
import de.roessing.app.errors.ErrorReportKind
import de.roessing.app.errors.ErrorReportTexte
import de.roessing.app.errors.ErrorReporter
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Echter End-to-End-Test der Fehlerberichte gegen ein laufendes Backend
 * (AUTH_MODE=insecure-dev). Nichts wird gemockt.
 *
 * Zwei Dinge, die sich nur hier zeigen: dass ein Bericht **ohne Anmeldung**
 * durchkommt — genau darauf kommt es an, wenn das Anmelden selbst klemmt —,
 * und dass der Interceptor eine gescheiterte Anfrage tatsächlich beim Melder
 * abliefert. Beides hängt an echten HTTP-Antworten und ist mit einem
 * Doppelgänger nicht zu prüfen.
 *
 * Läuft nur im E2E-Modus (`-e e2e true`), wie BackendE2eTest.
 */
@RunWith(AndroidJUnit4::class)
class FehlerberichtBackendE2eTest {
    @Before
    fun onlyInE2eMode() {
        assumeTrue(
            "E2E-Modus nicht aktiv (Instrumentation-Arg e2e fehlt)",
            InstrumentationRegistry.getArguments().getString("e2e") == "true",
        )
    }

    private val texte = ErrorReportTexte(
        ohneVerbindung = "Keine Verbindung zum Server.",
        nichtGefunden = "Das gibt es nicht (mehr).",
        abschickenGescheitert = "Das Abschicken hat nicht geklappt.",
        zeileWas = "Was passiert ist",
        zeileBereich = "Bereich",
        zeileTechnisch = "Technisch",
        zeileDeinText = "Dein Text",
        zeileApp = "App",
        zeileGeraet = "Gerät",
        zeileWann = "Wann",
        serverfehlerVorlage = "Der Server antwortet gerade nicht (%1\$d).",
    )

    private val facts = DeviceFacts("0.1.11 (1)", "Android E2E", "E2E-Gerät")

    private fun melder(token: TokenResult): ErrorReporter {
        val melder = ErrorReporter(texte, facts)
        val api = DorfApi.create(BuildConfig.API_BASE_URL, beobachter = melder) { token }
        melder.wire(ApiErrorReportRepository(api))
        return melder
    }

    @Test
    fun einBerichtKommtOhneAnmeldungDurch() = runBlocking {
        val melder = melder(TokenResult.LoggedOut)
        melder.report(
            ErrorIncident(
                kind = ErrorReportKind.CRASH,
                message = "E2E: Die App hat sich beendet ${System.currentTimeMillis()}.",
                detail = "java.lang.IllegalStateException: E2E",
                area = "Absturz",
            ),
        )

        melder.send()

        assertTrue("Bericht nicht angekommen", melder.state.value.gesendet)
        assertNull(melder.state.value.sendefehler)
    }

    @Test
    fun einBerichtAusDerAngemeldetenAppHaengtAmKonto() = runBlocking {
        val melder = melder(TokenResult.Token("e2e-bericht:E2E Melder:member"))
        melder.report(
            ErrorIncident(
                kind = ErrorReportKind.SERVER,
                message = "E2E: Der Server antwortet gerade nicht (500).",
                area = "Mithelfen",
            ),
        )

        melder.send("E2E-Ergänzung: ich wollte gerade melden.")

        assertTrue("Bericht nicht angekommen", melder.state.value.gesendet)
    }

    @Test
    fun eineGescheiterteAnfrageLandetVonAlleinBeimMelder() = runBlocking {
        val melder = melder(TokenResult.Token("e2e-bericht:E2E Melder:member"))
        val api = DorfApi.create(BuildConfig.API_BASE_URL, beobachter = melder) {
            TokenResult.Token("e2e-bericht:E2E Melder:member")
        }

        // Eine Aufgabe, die es nicht gibt: Das Backend antwortet mit 404, und
        // der Interceptor liefert das beim Melder ab, ohne dass ein Bereich
        // daran denken musste.
        runCatching { api.completions(taskId = 999_999_999) }

        val vorfall = requireNotNull(melder.state.value.vorfall)
        assertEquals(ErrorReportKind.UNEXPECTED, vorfall.kind)
        assertEquals("Mithelfen", vorfall.area)
    }
}
