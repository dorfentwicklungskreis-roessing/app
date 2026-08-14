package de.roessing.app

import de.roessing.app.data.OrtDto
import de.roessing.app.data.VeranstalterDto
import de.roessing.app.data.VeranstaltungDto
import de.roessing.app.data.VeranstaltungenRepository
import de.roessing.app.ui.VeranstaltungenViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Instant

/**
 * „Was ist los in Rössing" — die Termine kommen von der Website
 * (`/events.json`), gepflegt werden sie dort und nur dort.
 *
 * Geprüft wird vor allem, was leicht schiefgeht: die Regel für Termine mit
 * externer Primärquelle, ganztägige Termine ohne Uhrzeit, die Zeitzone und
 * der Fall „kein Netz".
 */
private class FakeVeranstaltungen(
    var termine: List<VeranstaltungDto> = emptyList(),
) : VeranstaltungenRepository {
    var fehler: Throwable? = null
    var abrufe = 0

    override suspend fun kommende(): List<VeranstaltungDto> {
        abrufe++
        fehler?.let { throw it }
        return termine
    }
}

private fun dto(
    id: String,
    start: String,
    name: String = "Offenes Dorfarchiv",
    end: String? = null,
    allDay: Boolean = false,
    url: String = "https://xn--rssing-wxa.de/events/$id",
    external: Boolean = false,
    location: OrtDto? = null,
    organizer: VeranstalterDto? = null,
) = VeranstaltungDto(
    id = id,
    name = name,
    description = "Kurz gesagt, worum es geht.",
    start = start,
    end = end,
    allDay = allDay,
    url = url,
    external = external,
    location = location,
    organizer = organizer,
)

@OptIn(ExperimentalCoroutinesApi::class)
class VeranstaltungenViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    /** Fester „Jetzt"-Zeitpunkt: 14.8.2026, 12 Uhr Ortszeit (Sommerzeit). */
    private val jetzt = Instant.parse("2026-08-14T10:00:00Z")

    private fun vm(repo: VeranstaltungenRepository) =
        VeranstaltungenViewModel(repo, uhr = { jetzt })

    @Before fun vorher() = Dispatchers.setMain(dispatcher)

    @After fun nachher() = Dispatchers.resetMain()

    @Test
    fun `Kommende Termine stehen vorn, vergangene fallen heraus`() = runTest(dispatcher) {
        val repo = FakeVeranstaltungen(
            listOf(
                dto("spaet", "2026-09-01T19:00:00+02:00"),
                dto("vorbei", "2026-08-01T19:00:00+02:00"),
                dto("bald", "2026-08-20T18:00:00+02:00"),
            ),
        )
        val vm = vm(repo)
        vm.laden()
        advanceUntilIdle()

        assertEquals(listOf("bald", "spaet"), vm.state.value.termine.map { it.id })
        assertFalse(vm.state.value.laedt)
        assertFalse(vm.state.value.fehler)
    }

    @Test
    fun `Ein ganztaegiger Termin hat keine Uhrzeit`() = runTest(dispatcher) {
        val repo = FakeVeranstaltungen(listOf(dto("blutspende", "2026-08-17", allDay = true)))
        val vm = vm(repo)
        vm.laden()
        advanceUntilIdle()

        val termin = vm.state.value.termine.single()
        assertTrue(termin.ganztaegig)
        assertNull(termin.zeitText)
        assertEquals("Mo, 17.08.2026", termin.datumText)
    }

    @Test
    fun `Ein Termin mit Uhrzeit behaelt die Ortszeit aus dem Offset`() = runTest(dispatcher) {
        val repo = FakeVeranstaltungen(
            listOf(
                // Sommerzeit (+02:00) und Winterzeit (+01:00) — beides muss in
                // Ortszeit dastehen, nicht in UTC.
                dto("sommer", "2026-08-20T18:00:00+02:00"),
                dto("winter", "2026-12-04T19:30:00+01:00"),
            ),
        )
        val vm = vm(repo)
        vm.laden()
        advanceUntilIdle()

        val (sommer, winter) = vm.state.value.termine
        assertEquals("18:00 Uhr", sommer.zeitText)
        assertEquals("Do, 20.08.2026", sommer.datumText)
        assertEquals("19:30 Uhr", winter.zeitText)
        assertEquals("Fr, 04.12.2026", winter.datumText)
    }

    @Test
    fun `Ein Termin mit externer Primaerquelle fuehrt nach draussen`() = runTest(dispatcher) {
        val repo = FakeVeranstaltungen(
            listOf(
                dto(
                    "konzert",
                    "2026-09-05T19:00:00+02:00",
                    url = "https://kulturkreis-roessing.de/konzert",
                    external = true,
                ),
            ),
        )
        val vm = vm(repo)
        vm.laden()
        advanceUntilIdle()

        val termin = vm.state.value.termine.single()
        assertTrue(termin.extern)
        assertEquals("https://kulturkreis-roessing.de/konzert", termin.url)
        // Die interne Detailseite darf nirgends auftauchen: Die externe Seite
        // ist die Primärquelle, doppelte Inhalte sind ausdrücklich unerwünscht.
        assertFalse(termin.url.contains("/events/konzert"))
    }

    @Test
    fun `Ohne externe Quelle fuehrt der Termin auf die Seite des Dorfes`() = runTest(dispatcher) {
        val repo = FakeVeranstaltungen(listOf(dto("dorfarchiv", "2026-09-01T17:00:00+02:00")))
        val vm = vm(repo)
        vm.laden()
        advanceUntilIdle()

        val termin = vm.state.value.termine.single()
        assertFalse(termin.extern)
        assertEquals("https://xn--rssing-wxa.de/events/dorfarchiv", termin.url)
    }

    @Test
    fun `Ein Termin von heute frueh bleibt bis Mitternacht stehen`() = runTest(dispatcher) {
        val repo = FakeVeranstaltungen(
            listOf(
                dto("heute-frueh", "2026-08-14T09:30:00+02:00"),
                dto("heute-ganztags", "2026-08-14", allDay = true),
                dto("gestern", "2026-08-13T20:00:00+02:00"),
            ),
        )
        val vm = vm(repo)
        vm.laden()
        advanceUntilIdle()

        assertEquals(
            listOf("heute-ganztags", "heute-frueh"),
            vm.state.value.termine.map { it.id },
        )
    }

    @Test
    fun `Ein mehrtaegiger Termin bleibt bis zum letzten Tag stehen`() = runTest(dispatcher) {
        val repo = FakeVeranstaltungen(
            listOf(
                dto(
                    "schuetzenfest",
                    "2026-08-12",
                    end = "2026-08-16",
                    allDay = true,
                ),
            ),
        )
        val vm = vm(repo)
        vm.laden()
        advanceUntilIdle()

        assertEquals(listOf("schuetzenfest"), vm.state.value.termine.map { it.id })
    }

    @Test
    fun `Eine kaputte Datumsangabe wirft nicht die ganze Liste weg`() = runTest(dispatcher) {
        val repo = FakeVeranstaltungen(
            listOf(
                dto("kaputt", "irgendwann"),
                dto("heil", "2026-09-01T17:00:00+02:00"),
            ),
        )
        val vm = vm(repo)
        vm.laden()
        advanceUntilIdle()

        assertEquals(listOf("heil"), vm.state.value.termine.map { it.id })
        assertFalse(vm.state.value.fehler)
    }

    @Test
    fun `Ohne Netz gibt es einen Hinweis statt einer leeren Seite`() = runTest(dispatcher) {
        val repo = FakeVeranstaltungen()
        repo.fehler = RuntimeException("kein Netz")
        val vm = vm(repo)
        vm.laden()
        advanceUntilIdle()

        assertTrue(vm.state.value.fehler)
        assertFalse(vm.state.value.laedt)
        assertTrue(vm.state.value.termine.isEmpty())
    }

    @Test
    fun `Nach einem Fehler bleiben die zuletzt geholten Termine stehen`() = runTest(dispatcher) {
        val repo = FakeVeranstaltungen(listOf(dto("bald", "2026-08-20T18:00:00+02:00")))
        val vm = vm(repo)
        vm.laden()
        advanceUntilIdle()
        assertEquals(1, vm.state.value.termine.size)

        repo.fehler = RuntimeException("kein Netz")
        vm.aktualisieren()
        advanceUntilIdle()

        // Lieber ein Hinweis über einer alten Liste als eine leere Seite.
        assertTrue(vm.state.value.fehler)
        assertEquals(listOf("bald"), vm.state.value.termine.map { it.id })
    }

    @Test
    fun `Ein zweites Oeffnen holt die Liste nicht noch einmal`() = runTest(dispatcher) {
        val repo = FakeVeranstaltungen(listOf(dto("bald", "2026-08-20T18:00:00+02:00")))
        val vm = vm(repo)
        vm.laden()
        advanceUntilIdle()
        vm.laden()
        advanceUntilIdle()

        assertEquals(1, repo.abrufe)

        // Wer selbst aktualisiert, bekommt aber frische Daten.
        vm.aktualisieren()
        advanceUntilIdle()
        assertEquals(2, repo.abrufe)
    }

    @Test
    fun `Ort und Veranstalter stehen am Termin, Koordinaten nur wenn vorhanden`() = runTest(dispatcher) {
        val repo = FakeVeranstaltungen(
            listOf(
                dto(
                    "dorfarchiv",
                    "2026-09-01T17:00:00+02:00",
                    location = OrtDto(
                        name = "Dorfgemeinschaftshaus Rössing",
                        address = "Kirchstraße 3, 31171 Nordstemmen",
                        lat = 52.1843,
                        lon = 9.8162,
                    ),
                    organizer = VeranstalterDto(name = "Dorfpflege Rössing"),
                ),
                dto("ohne-ort", "2026-09-02T17:00:00+02:00"),
            ),
        )
        val vm = vm(repo)
        vm.laden()
        advanceUntilIdle()

        val (mitOrt, ohneOrt) = vm.state.value.termine
        assertEquals("Dorfgemeinschaftshaus Rössing", mitOrt.ortName)
        assertEquals("Dorfpflege Rössing", mitOrt.veranstalter)
        assertEquals(52.1843, mitOrt.koordinate?.lat ?: 0.0, 0.00001)
        assertNull(ohneOrt.ortName)
        assertNull(ohneOrt.koordinate)

        // Für die Dorfkarte zählt nur, was wirklich eine Stelle hat.
        assertEquals(listOf("dorfarchiv"), vm.state.value.kartenpunkte.map { it.id })
    }
}
