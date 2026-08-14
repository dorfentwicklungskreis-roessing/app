package de.roessing.app

import de.roessing.app.data.DeviceRepository
import de.roessing.app.push.Anmeldespeicher
import de.roessing.app.push.Benachrichtigungserlaubnis
import de.roessing.app.push.Geraeteabgleich
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Die Gerätekennung von Firebase steht für genau dieses Handy und erlaubt es
 * dem Dorfserver, Nachrichten dorthin zu schicken. Sie ist damit ein
 * personenbezogenes Datum, und die Datenschutzerklärung stützt sich auf die
 * Einwilligung. Also: Ohne erlaubte Benachrichtigungen darf gar keine Kennung
 * entstehen — und wird die Erlaubnis entzogen, muss sie wieder verschwinden.
 */
class GeraeteabgleichTest {

    /** Merkt sich im Speicher, was sonst in den Einstellungen der App läge. */
    private class FakeSpeicher(var wert: Boolean = false) : Anmeldespeicher {
        override suspend fun angemeldet() = wert
        override suspend fun merken(wert: Boolean) { this.wert = wert }
    }

    private class FakeGeraete : DeviceRepository {
        val angemeldet = mutableListOf<String>()
        val abgemeldet = mutableListOf<String>()
        override suspend fun register(token: String) { angemeldet += token }
        override suspend fun unregister(token: String) { abgemeldet += token }
    }

    private val speicher = FakeSpeicher()
    private val geraete = FakeGeraete()
    private var kennungAbgefragt = 0
    private var kennungVerworfen = 0

    private fun abgleich(kennung: String? = "kennung-123") = Geraeteabgleich(
        speicher = speicher,
        geraete = geraete,
        kennung = { kennungAbgefragt++; kennung },
        kennungVerwerfen = { kennungVerworfen++ },
    )

    @Test
    fun `ohne Erlaubnis entsteht keine Kennung`() = runTest {
        abgleich().abgleichen(erlaubt = false)

        assertEquals(
            "Ohne erlaubte Benachrichtigungen darf keine Kennung beim Backend landen",
            emptyList<String>(),
            geraete.angemeldet,
        )
        assertEquals(
            "Firebase darf gar nicht erst nach einer Kennung gefragt werden — " +
                "das Fragen legt sie an",
            0,
            kennungAbgefragt,
        )
        assertFalse(speicher.wert)
    }

    @Test
    fun `mit Erlaubnis wird die Kennung hinterlegt`() = runTest {
        abgleich().abgleichen(erlaubt = true)

        assertEquals(listOf("kennung-123"), geraete.angemeldet)
        assertTrue("Die Anmeldung muss gemerkt werden", speicher.wert)
    }

    @Test
    fun `entzogene Erlaubnis löscht die Kennung wieder`() = runTest {
        speicher.wert = true

        abgleich().abgleichen(erlaubt = false)

        assertEquals(
            "Wer die Erlaubnis entzieht, dessen Kennung muss das Backend loswerden",
            listOf("kennung-123"),
            geraete.abgemeldet,
        )
        assertEquals("Die Kennung muss auch bei Firebase weg", 1, kennungVerworfen)
        assertFalse(speicher.wert)
    }

    @Test
    fun `bleibt die Erlaubnis erteilt, wird nichts gelöscht`() = runTest {
        speicher.wert = true

        abgleich().abgleichen(erlaubt = true)

        assertEquals(emptyList<String>(), geraete.abgemeldet)
        assertEquals(listOf("kennung-123"), geraete.angemeldet)
    }

    @Test
    fun `Abmelden ohne je angemeldet gewesen zu sein legt keine Kennung an`() = runTest {
        abgleich().abmelden()

        assertEquals(0, kennungAbgefragt)
        assertEquals(emptyList<String>(), geraete.abgemeldet)
    }

    @Test
    fun `beim Abmelden verschwindet die Kennung`() = runTest {
        speicher.wert = true

        abgleich().abmelden()

        assertEquals(listOf("kennung-123"), geraete.abgemeldet)
        assertEquals(1, kennungVerworfen)
        assertFalse(speicher.wert)
    }

    @Test
    fun `ohne Netz bleibt die Merkung stehen, damit es der nächste Start erneut versucht`() =
        runTest {
            speicher.wert = true
            val kaputt = object : DeviceRepository {
                override suspend fun register(token: String) = error("kein Netz")
                override suspend fun unregister(token: String) = error("kein Netz")
            }

            Geraeteabgleich(
                speicher = speicher,
                geraete = kaputt,
                kennung = { "kennung-123" },
                kennungVerwerfen = { kennungVerworfen++ },
            ).abgleichen(erlaubt = false)

            assertTrue("Sonst bliebe die Kennung für immer im Backend", speicher.wert)
            assertEquals("Ohne bestätigte Löschung bleibt die Kennung nutzbar", 0, kennungVerworfen)
        }

    // --- Wann gilt die Erlaubnis als erteilt? --------------------------------

    @Test
    fun `vor Android 13 zählt allein der Schalter in den Einstellungen`() {
        // API 28: POST_NOTIFICATIONS gibt es dort noch gar nicht.
        assertTrue(
            Benachrichtigungserlaubnis.wirksam(
                sdk = 28,
                berechtigungErteilt = false,
                systemErlaubt = true,
            ),
        )
        assertFalse(
            Benachrichtigungserlaubnis.wirksam(
                sdk = 28,
                berechtigungErteilt = true,
                systemErlaubt = false,
            ),
        )
    }

    @Test
    fun `ab Android 13 müssen Berechtigung und Schalter zusammenkommen`() {
        assertTrue(
            Benachrichtigungserlaubnis.wirksam(
                sdk = 33,
                berechtigungErteilt = true,
                systemErlaubt = true,
            ),
        )
        assertFalse(
            "Ohne erteilte Berechtigung gibt es keine Einwilligung",
            Benachrichtigungserlaubnis.wirksam(
                sdk = 33,
                berechtigungErteilt = false,
                systemErlaubt = true,
            ),
        )
        assertFalse(
            "In den Einstellungen abgedreht heißt abgedreht",
            Benachrichtigungserlaubnis.wirksam(
                sdk = 35,
                berechtigungErteilt = true,
                systemErlaubt = false,
            ),
        )
    }
}
