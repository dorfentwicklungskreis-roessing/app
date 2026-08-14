package de.roessing.app

import de.roessing.app.data.IdeeDto
import de.roessing.app.data.IdeeInput
import de.roessing.app.data.IdeenAblehnungException
import de.roessing.app.data.IdeenRepository
import de.roessing.app.ui.IdeenEvent
import de.roessing.app.ui.IdeenViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.launch
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

/** Fake des Ideen-Eingangs: merkt sich, was geschickt wurde, und kann eine
 *  Ablehnung des Backends (HTTP 400) sowie einen Netzfehler nachstellen. */
private class FakeIdeenRepo : IdeenRepository {
    val geschickt = mutableListOf<IdeeInput>()
    var ablehnung: String? = null
    var fehler: Throwable? = null

    override suspend fun einreichen(input: IdeeInput): IdeeDto {
        ablehnung?.let { throw IdeenAblehnungException(it) }
        fehler?.let { throw it }
        geschickt += input
        return IdeeDto(id = 1, wunsch = input.wunsch, name = input.name, email = input.email)
    }
}

@OptIn(ExperimentalCoroutinesApi::class)
class IdeenViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before fun vorher() = Dispatchers.setMain(dispatcher)

    @After fun nachher() = Dispatchers.resetMain()

    @Test
    fun `Name und E-Mail kommen aus dem Profil`() = runTest(dispatcher) {
        val vm = IdeenViewModel(FakeIdeenRepo())
        vm.vorbelegen(name = "Erna Beispiel", email = "erna@example.org")
        advanceUntilIdle()

        assertEquals("Erna Beispiel", vm.state.value.name)
        assertEquals("erna@example.org", vm.state.value.email)
    }

    @Test
    fun `Vorbelegung ueberschreibt nichts, was schon getippt wurde`() = runTest(dispatcher) {
        val vm = IdeenViewModel(FakeIdeenRepo())
        vm.setName("Ich selbst")
        vm.vorbelegen(name = "Erna Beispiel", email = "erna@example.org")
        advanceUntilIdle()

        assertEquals("Ich selbst", vm.state.value.name)
        // Was leer geblieben ist, wird aber ergänzt.
        assertEquals("erna@example.org", vm.state.value.email)
    }

    @Test
    fun `Absenden schickt den Wunsch und meldet Erfolg`() = runTest(dispatcher) {
        val repo = FakeIdeenRepo()
        val vm = IdeenViewModel(repo)
        val ereignisse = mutableListOf<IdeenEvent>()
        val job = launch { vm.events.collect { ereignisse += it } }

        vm.vorbelegen(name = "Erna Beispiel", email = "erna@example.org")
        vm.setWunsch("  Ein Mitfahrbrett für Fahrten nach Hildesheim.  ")
        vm.absenden()
        advanceUntilIdle()

        assertEquals(1, repo.geschickt.size)
        val geschickt = repo.geschickt.single()
        // Der Text wird von Leerraum befreit, aber nicht sonst angefasst.
        assertEquals("Ein Mitfahrbrett für Fahrten nach Hildesheim.", geschickt.wunsch)
        assertEquals("Erna Beispiel", geschickt.name)
        assertEquals("erna@example.org", geschickt.email)
        assertTrue(ereignisse.contains(IdeenEvent.Gesendet))
        assertFalse(vm.state.value.sendet)
        // Nach dem Absenden ist das Feld frei für die nächste Idee — Name und
        // E-Mail bleiben stehen, damit niemand sie erneut tippen muss.
        assertEquals("", vm.state.value.wunsch)
        assertEquals("Erna Beispiel", vm.state.value.name)
        job.cancel()
    }

    @Test
    fun `Zu kurzer Wunsch wird gar nicht erst geschickt`() = runTest(dispatcher) {
        val repo = FakeIdeenRepo()
        val vm = IdeenViewModel(repo)
        vm.setWunsch("hm")
        assertFalse(vm.state.value.absendbar)
        vm.absenden()
        advanceUntilIdle()

        assertTrue(repo.geschickt.isEmpty())
        // Der Text bleibt stehen — es wurde ja nichts abgeschickt.
        assertEquals("hm", vm.state.value.wunsch)
    }

    @Test
    fun `Ein langer genug Wunsch macht den Knopf verfuegbar`() = runTest(dispatcher) {
        val vm = IdeenViewModel(FakeIdeenRepo())
        assertFalse(vm.state.value.absendbar)
        vm.setWunsch("Radweg")
        assertTrue(vm.state.value.absendbar)
        vm.setWunsch("   ")
        assertFalse(vm.state.value.absendbar)
    }

    @Test
    fun `Ablehnung des Backends steht woertlich da und der Text bleibt erhalten`() = runTest(dispatcher) {
        val repo = FakeIdeenRepo()
        repo.ablehnung = "Die E-Mail-Adresse sieht nicht richtig aus."
        val vm = IdeenViewModel(repo)
        vm.setWunsch("Ein Mitfahrbrett für Fahrten nach Hildesheim.")
        vm.setEmail("keine-mail")
        vm.absenden()
        advanceUntilIdle()

        assertEquals("Die E-Mail-Adresse sieht nicht richtig aus.", vm.state.value.fehler)
        assertEquals("Ein Mitfahrbrett für Fahrten nach Hildesheim.", vm.state.value.wunsch)
        assertFalse(vm.state.value.sendet)
    }

    @Test
    fun `Netzfehler verliert den getippten Text nicht`() = runTest(dispatcher) {
        val repo = FakeIdeenRepo()
        repo.fehler = RuntimeException("kein Netz")
        val vm = IdeenViewModel(repo)
        vm.setWunsch("Ein Mitfahrbrett für Fahrten nach Hildesheim.")
        vm.absenden()
        advanceUntilIdle()

        assertTrue(vm.state.value.fehler!!.isNotBlank())
        assertEquals("Ein Mitfahrbrett für Fahrten nach Hildesheim.", vm.state.value.wunsch)
    }

    @Test
    fun `Tippen raeumt eine alte Fehlermeldung weg`() = runTest(dispatcher) {
        val repo = FakeIdeenRepo()
        repo.ablehnung = "Der Wunsch ist zu kurz."
        val vm = IdeenViewModel(repo)
        vm.setWunsch("Radweg")
        vm.absenden()
        advanceUntilIdle()
        assertTrue(vm.state.value.fehler != null)

        vm.setWunsch("Ein Radweg nach Nordstemmen wäre großartig.")
        assertNull(vm.state.value.fehler)
    }
}
