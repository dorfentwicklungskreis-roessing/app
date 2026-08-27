package de.roessing.app

import de.roessing.app.data.ErrorReportDto
import de.roessing.app.data.ErrorReportInput
import de.roessing.app.data.ErrorReportRepository
import de.roessing.app.errors.AreaNames
import de.roessing.app.errors.CrashStore
import de.roessing.app.errors.CrashWatch
import de.roessing.app.errors.DeviceFacts
import de.roessing.app.errors.ErrorIncident
import de.roessing.app.errors.ErrorReportKind
import de.roessing.app.errors.ErrorReportTexte
import de.roessing.app.errors.ErrorReporter
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Fehlerberichte: Wenn etwas nicht klappt, sieht die Person das — und kann
 * mit einem Fingertipp einen Bericht schicken, ohne etwas zu beschreiben.
 *
 * Geprüft wird das, woran es hängt: dass eine Regel, die greift, **kein**
 * Bericht wird (sonst ersäuft die echte Störung im Rauschen), dass ein
 * Absturz beim nächsten Start auftaucht, und dass wirklich nur das hinausgeht,
 * was im Blatt steht. Dieselben Fälle wie in `ios/DorfTests/ErrorReportTests`.
 */
class ErrorReportTest {

    private val texte = ErrorReportTexte(
        ohneVerbindung = "Keine Verbindung zum Server. Es werden ggf. alte Daten angezeigt.",
        nichtGefunden = "Das gibt es nicht (mehr).",
        abschickenGescheitert = "Das Abschicken hat nicht geklappt. Besteht eine Verbindung?",
        zeileWas = "Was passiert ist",
        zeileBereich = "Bereich",
        zeileTechnisch = "Technisch",
        zeileDeinText = "Dein Text",
        zeileApp = "App",
        zeileGeraet = "Gerät",
        zeileWann = "Wann",
        serverfehlerVorlage = "Der Server antwortet gerade nicht (%1\$d).",
    )

    private val facts = DeviceFacts(
        appVersion = "0.1.10 (1000110)",
        osVersion = "Android 15 (API 35)",
        deviceModel = "Google Pixel 6",
    )

    /** Ein Ziel, das mitschreibt und auf Wunsch scheitert. */
    private class Empfang(var scheitern: Boolean = false) : ErrorReportRepository {
        var letzte: ErrorReportInput? = null
        var versuche = 0

        override suspend fun send(input: ErrorReportInput): ErrorReportDto {
            versuche++
            letzte = input
            if (scheitern) error("kein Netz")
            return ErrorReportDto(id = 7)
        }
    }

    /** Ablage im Arbeitsspeicher statt SharedPreferences. */
    private class Zettel : CrashStore {
        val werte = mutableMapOf<String, String>()
        override fun lesen(schluessel: String) = werte[schluessel]
        override fun schreiben(schluessel: String, wert: String) { werte[schluessel] = wert }
        override fun loeschen(schluessel: String) { werte.remove(schluessel) }
    }

    private fun melder(empfang: Empfang = Empfang(), jetzt: Long = 1_756_000_000_000) =
        ErrorReporter(texte, facts, uhr = { jetzt }).also { it.wire(empfang) }

    // --- Was gemeldet wird, und was nicht ------------------------------------

    @Test
    fun `eine Regel die greift ist kein Fehler`() {
        val melder = melder()
        // Diese Abweisungen sind das Backend bei der Arbeit: zu kurzer Text,
        // fehlende Rolle, jemand war schneller, zu viel auf einmal. Sie stehen
        // dort, wo sie hingehören — ein Bericht darüber wäre Rauschen.
        for (code in listOf(400, 401, 403, 409, 429)) {
            melder.antwort("POST", "/api/v1/places/1/signup", code)
            assertNull("HTTP $code darf keinen Bericht auslösen", melder.state.value.vorfall)
        }
    }

    @Test
    fun `eine Stoerung wird gemeldet und behaelt den Wortlaut`() {
        val melder = melder()
        melder.antwort("GET", "/api/v1/places", 500)

        val vorfall = requireNotNull(melder.state.value.vorfall)
        assertEquals(ErrorReportKind.SERVER, vorfall.kind)
        // Der Satz ist der, den die Person gelesen hat — für den Bericht wird
        // kein zweiter erfunden.
        assertEquals("Der Server antwortet gerade nicht (500).", vorfall.message)
        assertTrue(vorfall.detail.contains("HTTP 500"))
        assertTrue(vorfall.detail.contains("GET /api/v1/places"))
        assertEquals("Mithelfen", vorfall.area)
    }

    @Test
    fun `ohne Verbindung wird gemeldet`() {
        val melder = melder()
        melder.fehlschlag("GET", "/api/v1/me", "timeout")

        val vorfall = requireNotNull(melder.state.value.vorfall)
        assertEquals(ErrorReportKind.NETWORK, vorfall.kind)
        assertEquals("Konto", vorfall.area)
        assertTrue(vorfall.detail.contains("timeout"))
    }

    @Test
    fun `ein gescheiterter Bericht meldet sich nicht selbst`() {
        // Sonst drehte sich das im Kreis: Jeder Fehlversuch erzeugte einen
        // neuen Bericht, der wieder scheitert.
        val melder = melder()
        melder.antwort("POST", "/api/v1/error-reports", 500)
        assertNull(melder.state.value.vorfall)
        melder.fehlschlag("POST", "/api/v1/error-reports", "timeout")
        assertNull(melder.state.value.vorfall)
    }

    @Test
    fun `der Bereich kommt in Alltagssprache an`() {
        // „api/v1/places" sagt dem Dorfentwicklungskreis nichts.
        assertEquals("Mithelfen", AreaNames.of("api/v1/places"))
        assertEquals("Mithelfen", AreaNames.of("/api/v1/tasks/3/completions"))
        assertEquals("Rangliste", AreaNames.of("api/v1/stats/leaderboard"))
        assertEquals("Dorfbewohner", AreaNames.of("api/v1/members"))
        // Der längere Treffer gewinnt: `me/devices` ist nicht `me`.
        assertEquals("Benachrichtigungen", AreaNames.of("api/v1/me/devices"))
        assertEquals("Mein Profil", AreaNames.of("api/v1/me/profile"))
        assertEquals("Konto", AreaNames.of("api/v1/me"))
        assertEquals("App", AreaNames.of("api/v1/voellig/neu"))
    }

    // --- Ein Fingertipp -------------------------------------------------------

    @Test
    fun `ein Fingertipp schickt den Bericht`() = runTest {
        val empfang = Empfang()
        val melder = melder(empfang)
        melder.antwort("GET", "/api/v1/places", 500)

        // Genau ein Knopfdruck, kein getippter Text.
        melder.send()

        assertTrue(melder.state.value.gesendet)
        assertNull(melder.state.value.sendefehler)
        val gesendet = requireNotNull(empfang.letzte)
        assertEquals("server", gesendet.kind)
        assertEquals("android", gesendet.platform)
        assertEquals("0.1.10 (1000110)", gesendet.appVersion)
        assertEquals("Google Pixel 6", gesendet.deviceModel)
        assertEquals("Mithelfen", gesendet.area)
        assertEquals("", gesendet.comment)
        assertTrue(gesendet.occurredAt.isNotBlank())
    }

    @Test
    fun `wer etwas dazuschreibt wird gehoert`() = runTest {
        val empfang = Empfang()
        val melder = melder(empfang)
        melder.report(ErrorIncident(ErrorReportKind.CRASH, "Die App hat sich beendet."))

        melder.send("  Ich wollte gerade das Gießen melden.  ")

        assertEquals("Ich wollte gerade das Gießen melden.", empfang.letzte?.comment)
        assertEquals("crash", empfang.letzte?.kind)
    }

    @Test
    fun `es geht nur hinaus was im Blatt steht`() {
        val melder = melder()
        val vorfall = ErrorIncident(
            kind = ErrorReportKind.SERVER,
            message = "Der Server antwortet gerade nicht (500).",
            detail = "HTTP 500 · GET /api/v1/places",
            area = "Mithelfen",
        )
        val eingabe = melder.inputFor(vorfall, "Nur ein Satz.")
        val gezeigt = melder.contentLines(eingabe, vorfall.occurredAt)
            .joinToString("\n") { it.second }

        // Was im Blatt steht, steht auch in der Anfrage.
        for (wert in listOf(
            eingabe.message, eingabe.detail, eingabe.comment,
            eingabe.area, eingabe.appVersion, eingabe.deviceModel,
        )) {
            assertTrue("$wert fehlt in der Aufstellung", gezeigt.contains(wert))
        }
    }

    @Test
    fun `ein gescheitertes Abschicken wird gesagt nicht verschluckt`() = runTest {
        val empfang = Empfang(scheitern = true)
        val melder = melder(empfang)
        melder.report(ErrorIncident(ErrorReportKind.CRASH, "Die App hat sich beendet."))

        melder.send()

        assertFalse(melder.state.value.gesendet)
        assertNotNull(melder.state.value.sendefehler)
        // Der Vorfall bleibt stehen — sonst wäre der Bericht weg, ohne
        // angekommen zu sein.
        assertNotNull(melder.state.value.vorfall)
    }

    @Test
    fun `schliessen ist eine gueltige Antwort`() {
        val melder = melder()
        melder.report(ErrorIncident(ErrorReportKind.NETWORK, "Keine Verbindung."))
        melder.dismiss()
        assertNull(melder.state.value.vorfall)
    }

    @Test
    fun `ohne Vorfall geht nichts hinaus`() = runTest {
        val empfang = Empfang()
        melder(empfang).send("etwas")
        assertEquals(0, empfang.versuche)
    }

    // --- Abstürze -------------------------------------------------------------

    @Test
    fun `ein Absturz taucht beim naechsten Start auf`() {
        val zettel = Zettel()
        CrashWatch.record(zettel, IllegalStateException("kaputt"), jetzt = 1_000)

        val vorfall = requireNotNull(
            CrashWatch.pendingCrash(zettel, jetzt = 2_000, meldung = "Die App hat sich beendet."),
        )
        assertEquals(ErrorReportKind.CRASH, vorfall.kind)
        assertEquals("Absturz", vorfall.area)
        assertEquals(1_000L, vorfall.occurredAt)
        // Die Aufrufliste ist das, was beim Nachstellen hilft.
        assertTrue(vorfall.detail.contains("IllegalStateException"))
        assertTrue(vorfall.detail.contains("kaputt"))

        // Und nur einmal: Derselbe Absturz wird nicht zweimal angeboten.
        assertNull(CrashWatch.pendingCrash(zettel, jetzt = 3_000, meldung = "egal"))
    }

    @Test
    fun `ein sauberer erster Start meldet nichts`() {
        assertNull(CrashWatch.pendingCrash(Zettel(), jetzt = 1_000, meldung = "egal"))
    }

    @Test
    fun `ein Absturzzeitpunkt liegt nie in der Zukunft`() {
        val zettel = Zettel()
        CrashWatch.record(zettel, RuntimeException("kaputt"), jetzt = 9_000)

        val vorfall = requireNotNull(
            CrashWatch.pendingCrash(zettel, jetzt = 5_000, meldung = "egal"),
        )
        assertTrue(vorfall.occurredAt <= 5_000)
    }

    // --- Angaben über das Gerät ------------------------------------------------

    @Test
    fun `die Angaben sind Version und Geraetetyp und sonst nichts`() {
        val angaben = DeviceFacts.of(
            versionName = "0.1.10", versionCode = 1000110, sdk = 35,
            release = "15", manufacturer = "google", model = "Pixel 6",
        )
        assertEquals("0.1.10 (1000110)", angaben.appVersion)
        assertEquals("Android 15 (API 35)", angaben.osVersion)
        // Kleingeschrieben kommt der Hersteller herein, lesbar geht er heraus.
        assertEquals("Google Pixel 6", angaben.deviceModel)

        // Steht der Hersteller schon im Modellnamen, wird er nicht doppelt.
        val samsung = DeviceFacts.of(
            versionName = "0.1.10", versionCode = 1, sdk = 33,
            release = "13", manufacturer = "samsung", model = "Samsung Galaxy A54",
        )
        assertEquals("Samsung Galaxy A54", samsung.deviceModel)
    }
}
