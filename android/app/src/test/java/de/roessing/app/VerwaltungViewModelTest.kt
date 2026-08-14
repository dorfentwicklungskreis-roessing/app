package de.roessing.app

import de.roessing.app.data.PlaceDto
import de.roessing.app.data.PlaceEingabe
import de.roessing.app.data.TaskDto
import de.roessing.app.data.TaskEingabe
import de.roessing.app.data.VerwaltungAbgelehntException
import de.roessing.app.data.VerwaltungRepository
import de.roessing.app.ui.AufgabeForm
import de.roessing.app.ui.OrtForm
import de.roessing.app.ui.VerwaltungViewModel
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
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Die Verwaltung in der App: Orte und Aufgaben anlegen, ändern, pausieren,
 * löschen. Der Sinn der Sache ist, am Blumenkasten zu stehen und dort die
 * Aufgabe einzutragen — Standort übernehmen inklusive.
 *
 * Die Regeln stehen im Backend; hier wird geprüft, dass die App das Richtige
 * hinschickt und mit einer Ablehnung vernünftig umgeht.
 */
private class FakeVerwaltungRepo : VerwaltungRepository {
    val orteAngelegt = mutableListOf<PlaceEingabe>()
    val orteGeaendert = mutableListOf<Pair<Long, PlaceEingabe>>()
    val orteGeloescht = mutableListOf<Long>()
    val aufgabenAngelegt = mutableListOf<Pair<Long, TaskEingabe>>()
    val aufgabenGeaendert = mutableListOf<Pair<Long, TaskEingabe>>()
    val aufgabenGeloescht = mutableListOf<Long>()
    var ablehnung: String? = null
    var fehler: Throwable? = null

    private fun pruefe() {
        ablehnung?.let { throw VerwaltungAbgelehntException(it) }
        fehler?.let { throw it }
    }

    override suspend fun ortAnlegen(eingabe: PlaceEingabe): PlaceDto {
        pruefe()
        orteAngelegt += eingabe
        return PlaceDto(id = 7, name = eingabe.name, lat = eingabe.lat, lon = eingabe.lon)
    }

    override suspend fun ortAendern(id: Long, eingabe: PlaceEingabe): PlaceDto {
        pruefe()
        orteGeaendert += id to eingabe
        return PlaceDto(id = id, name = eingabe.name, lat = eingabe.lat, lon = eingabe.lon)
    }

    override suspend fun ortLoeschen(id: Long) {
        pruefe()
        orteGeloescht += id
    }

    override suspend fun aufgabeAnlegen(placeId: Long, eingabe: TaskEingabe): TaskDto {
        pruefe()
        aufgabenAngelegt += placeId to eingabe
        return TaskDto(id = 11, placeId = placeId, kind = eingabe.kind)
    }

    override suspend fun aufgabeAendern(id: Long, eingabe: TaskEingabe): TaskDto {
        pruefe()
        aufgabenGeaendert += id to eingabe
        return TaskDto(id = id, kind = eingabe.kind)
    }

    override suspend fun aufgabeLoeschen(id: Long) {
        pruefe()
        aufgabenGeloescht += id
    }
}

@OptIn(ExperimentalCoroutinesApi::class)
class VerwaltungViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before fun vorher() = Dispatchers.setMain(dispatcher)

    @After fun nachher() = Dispatchers.resetMain()

    private fun vm(repo: FakeVerwaltungRepo = FakeVerwaltungRepo()) =
        VerwaltungViewModel(repo) to repo

    @Test
    fun `Ort anlegen schickt Name Art und Koordinaten`() = runTest(dispatcher) {
        val (vm, repo) = vm()
        vm.ortBearbeiten(null)
        vm.setOrtName("Unter den Eichen — Kasten 7")
        vm.setOrtArt("blumenkasten")
        vm.setOrtPosition(52.2110, 9.8697)
        vm.ortSpeichern()
        advanceUntilIdle()

        assertEquals(1, repo.orteAngelegt.size)
        val gesendet = repo.orteAngelegt.first()
        assertEquals("Unter den Eichen — Kasten 7", gesendet.name)
        assertEquals("blumenkasten", gesendet.kind)
        assertEquals(52.2110, gesendet.lat, 0.00001)
        assertEquals(9.8697, gesendet.lon, 0.00001)
    }

    @Test
    fun `Ohne Namen oder Ort laesst sich nichts speichern`() = runTest(dispatcher) {
        val (vm, repo) = vm()
        vm.ortBearbeiten(null)
        assertFalse("ohne alles speicherbar", vm.state.value.ortForm!!.speicherbar)

        vm.setOrtName("Kasten 7")
        assertFalse("ohne Standort speicherbar", vm.state.value.ortForm!!.speicherbar)

        vm.setOrtPosition(52.2110, 9.8697)
        assertTrue("mit Name und Standort nicht speicherbar", vm.state.value.ortForm!!.speicherbar)

        vm.ortSpeichern()
        advanceUntilIdle()
        assertEquals(1, repo.orteAngelegt.size)
    }

    @Test
    fun `Eigener Standort laesst sich uebernehmen`() = runTest(dispatcher) {
        val (vm, _) = vm()
        vm.ortBearbeiten(null)
        vm.standortUebernehmen(52.1843, 9.8162)

        val form = vm.state.value.ortForm!!
        assertEquals(52.1843, form.lat!!, 0.00001)
        assertEquals(9.8162, form.lon!!, 0.00001)
    }

    @Test
    fun `Einmalige Aufgabe schickt Termin statt Intervall`() = runTest(dispatcher) {
        val (vm, repo) = vm()
        vm.aufgabeBearbeiten(placeId = 3, aufgabe = null)
        vm.setAufgabeArt("sonstiges")
        vm.setAufgabeTitel("Zum Bahnhof fahren")
        vm.setAufgabeEinmalig(true)
        vm.setAufgabeTermin("2026-08-20")
        vm.setAufgabeEntfernenNachErledigung(true)
        vm.aufgabeSpeichern()
        advanceUntilIdle()

        assertEquals(1, repo.aufgabenAngelegt.size)
        val (placeId, gesendet) = repo.aufgabenAngelegt.first()
        assertEquals(3L, placeId)
        assertTrue("nicht als einmalig geschickt", gesendet.oneOff)
        assertEquals("2026-08-20", gesendet.dueDate)
        assertTrue("Schalter fehlt", gesendet.removeWhenDone)
    }

    @Test
    fun `Regelmaessige Aufgabe schickt Intervall und keinen Termin`() = runTest(dispatcher) {
        val (vm, repo) = vm()
        vm.aufgabeBearbeiten(placeId = 3, aufgabe = null)
        vm.setAufgabeArt("giessen")
        vm.setAufgabeLiter("10")
        vm.setAufgabeIntervall("7")
        vm.setAufgabeRot("14")
        vm.aufgabeSpeichern()
        advanceUntilIdle()

        val (_, gesendet) = repo.aufgabenAngelegt.first()
        assertFalse(gesendet.oneOff)
        assertEquals("", gesendet.dueDate)
        assertEquals(7.0, gesendet.intervalDays, 0.001)
        assertEquals(14.0, gesendet.redAfterDays, 0.001)
        assertEquals(10.0, gesendet.liters!!, 0.001)
    }

    @Test
    fun `Einmalige Aufgabe ohne Termin ist nicht speicherbar`() = runTest(dispatcher) {
        val (vm, repo) = vm()
        vm.aufgabeBearbeiten(placeId = 3, aufgabe = null)
        vm.setAufgabeArt("sonstiges")
        vm.setAufgabeEinmalig(true)
        assertFalse(vm.state.value.aufgabeForm!!.speicherbar)

        vm.aufgabeSpeichern()
        advanceUntilIdle()
        assertTrue("trotzdem geschickt", repo.aufgabenAngelegt.isEmpty())
    }

    @Test
    fun `Bestehende Aufgabe wird zum Bearbeiten vorbelegt`() = runTest(dispatcher) {
        val (vm, repo) = vm()
        vm.aufgabeBearbeiten(
            placeId = 3,
            aufgabe = TaskDto(
                id = 42, placeId = 3, kind = "sonstiges", title = "Bank streichen",
                oneOff = true, dueDate = "2026-08-20T21:59:59Z", removeWhenDone = true,
            ),
        )
        val form = vm.state.value.aufgabeForm!!
        assertEquals("Bank streichen", form.titel)
        assertTrue(form.einmalig)
        assertEquals("2026-08-20", form.termin)
        assertTrue(form.entfernenNachErledigung)

        vm.aufgabeSpeichern()
        advanceUntilIdle()
        assertEquals(42L, repo.aufgabenGeaendert.first().first)
        assertTrue("als neu angelegt statt geändert", repo.aufgabenAngelegt.isEmpty())
    }

    @Test
    fun `Pausieren schickt aktiv false`() = runTest(dispatcher) {
        val (vm, repo) = vm()
        val aufgabe = TaskDto(id = 42, placeId = 3, kind = "giessen", intervalDays = 7.0,
            redAfterDays = 14.0, active = true)
        vm.aufgabePausieren(aufgabe, pausiert = true)
        advanceUntilIdle()

        val (id, gesendet) = repo.aufgabenGeaendert.first()
        assertEquals(42L, id)
        assertFalse("aktiv-Schalter nicht umgelegt", gesendet.active)
    }

    @Test
    fun `Loeschen einer Aufgabe geht ans Backend`() = runTest(dispatcher) {
        val (vm, repo) = vm()
        vm.aufgabeLoeschen(42)
        advanceUntilIdle()
        assertEquals(listOf(42L), repo.aufgabenGeloescht)
    }

    @Test
    fun `Eine Ablehnung des Backends steht im Klartext im Formular`() = runTest(dispatcher) {
        val (vm, repo) = vm()
        repo.ablehnung = "dueDate fehlt: eine einmalige Aufgabe braucht ein Fälligkeitsdatum"
        vm.aufgabeBearbeiten(placeId = 3, aufgabe = null)
        vm.setAufgabeArt("sonstiges")
        vm.setAufgabeEinmalig(true)
        vm.setAufgabeTermin("2026-08-20")
        vm.aufgabeSpeichern()
        advanceUntilIdle()

        val form = vm.state.value.aufgabeForm
        assertNotNull("Formular wurde trotz Fehler geschlossen", form)
        assertEquals(repo.ablehnung, form!!.fehler)
        // Der getippte Text bleibt stehen.
        assertEquals("2026-08-20", form.termin)
    }

    @Test
    fun `Nach dem Speichern ist das Formular zu`() = runTest(dispatcher) {
        val (vm, _) = vm()
        vm.ortBearbeiten(null)
        vm.setOrtName("Kasten 7")
        vm.setOrtPosition(52.2110, 9.8697)
        vm.ortSpeichern()
        advanceUntilIdle()
        assertNull(vm.state.value.ortForm)
    }
}
