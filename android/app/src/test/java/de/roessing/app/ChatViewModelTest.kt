package de.roessing.app

import de.roessing.app.data.ChatAbsageException
import de.roessing.app.data.ChatAntwortDto
import de.roessing.app.data.ChatRepository
import de.roessing.app.data.ChatStandDto
import de.roessing.app.data.ChatZugDto
import de.roessing.app.ui.ChatRolle
import de.roessing.app.ui.ChatUiState
import de.roessing.app.ui.ChatViewModel
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
import java.io.IOException

/**
 * Der Chat spricht ausschließlich mit dem Dorf-Backend, und alle Regeln
 * stehen dort. Hier wird deshalb geprüft, was die App daraus macht: dass der
 * Verlauf vollständig mitgeht, dass eine Absage im Wortlaut ankommt, und dass
 * ein Fehler nie den getippten Text kostet.
 *
 * Das Repository ist ein handgeschriebenes Doppel — kein Netz, kein Server.
 */
private class FakeChatRepo : ChatRepository {
    var stand = ChatStandDto(verfuegbar = true)
    var standFehler: Throwable? = null

    var antwort = ChatAntwortDto(antwort = "Am Kirchplatz muss gegossen werden.")
    var fehler: Throwable? = null

    /** Was das Backend zu sehen bekam — Frage und Verlauf, in Reihenfolge. */
    val fragen = mutableListOf<Pair<String, List<ChatZugDto>>>()

    override suspend fun stand(): ChatStandDto {
        standFehler?.let { throw it }
        return stand
    }

    override suspend fun fragen(frage: String, verlauf: List<ChatZugDto>): ChatAntwortDto {
        fragen += frage to verlauf
        fehler?.let { throw it }
        return antwort
    }
}

@OptIn(ExperimentalCoroutinesApi::class)
class ChatViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before fun vorher() = Dispatchers.setMain(dispatcher)

    @After fun nachher() = Dispatchers.resetMain()

    @Test
    fun `Eine Frage landet im Gespraech und die Antwort dahinter`() = runTest(dispatcher) {
        val repo = FakeChatRepo()
        repo.antwort = ChatAntwortDto(
            antwort = "Am Kirchplatz muss gegossen werden.",
            werkzeuge = listOf("orte_liste"),
        )
        val vm = ChatViewModel(repo)
        vm.standPruefen()
        advanceUntilIdle()

        vm.setEingabe("Was steht gerade an?")
        vm.absenden()
        advanceUntilIdle()

        val zuege = vm.state.value.zuege
        assertEquals(2, zuege.size)
        assertEquals(ChatRolle.ICH, zuege[0].rolle)
        assertEquals("Was steht gerade an?", zuege[0].text)
        assertEquals(ChatRolle.APP, zuege[1].rolle)
        assertEquals("Am Kirchplatz muss gegossen werden.", zuege[1].text)
        // Die befragten Werkzeuge stehen unter der Antwort: Sie belegen, dass
        // die Zahl aus dem Dorfserver kommt.
        assertEquals(listOf("orte_liste"), zuege[1].werkzeuge)
        assertEquals("", vm.state.value.eingabe)
        assertFalse(vm.state.value.wartet)
    }

    @Test
    fun `Der Verlauf geht mit - das Backend haelt keine Sitzung`() = runTest(dispatcher) {
        val repo = FakeChatRepo()
        val vm = ChatViewModel(repo)
        vm.standPruefen()
        advanceUntilIdle()

        vm.setEingabe("Moin")
        vm.absenden()
        advanceUntilIdle()
        vm.setEingabe("Und was steht an?")
        vm.absenden()
        advanceUntilIdle()

        assertEquals(2, repo.fragen.size)
        // Die erste Frage kennt noch keinen Verlauf.
        assertEquals(emptyList<ChatZugDto>(), repo.fragen[0].second)
        // Die zweite bringt Frage und Antwort der ersten mit.
        val verlauf = repo.fragen[1].second
        assertEquals(2, verlauf.size)
        assertEquals(ChatZugDto.ROLLE_ICH, verlauf[0].rolle)
        assertEquals("Moin", verlauf[0].text)
        assertEquals(ChatZugDto.ROLLE_APP, verlauf[1].rolle)
        assertEquals("Am Kirchplatz muss gegossen werden.", verlauf[1].text)
        assertEquals("Und was steht an?", repo.fragen[1].first)
    }

    @Test
    fun `Die Absage des Backends kommt im Wortlaut an`() = runTest(dispatcher) {
        val repo = FakeChatRepo()
        // So sagt das Backend „du darfst das nicht" — der Satz ist der
        // eigentliche Inhalt und darf nicht umformuliert werden.
        repo.fehler = ChatAbsageException(
            "Das dürfen nur die Verwaltenden von „Dorfpflege“.",
            voruebergehend = false,
        )
        val vm = ChatViewModel(repo)
        vm.standPruefen()
        advanceUntilIdle()

        vm.setEingabe("Leg einen Blumenkasten an")
        vm.absenden()
        advanceUntilIdle()

        assertEquals("Das dürfen nur die Verwaltenden von „Dorfpflege“.", vm.state.value.fehler)
    }

    @Test
    fun `Ein Fehler kostet nie den getippten Text`() = runTest(dispatcher) {
        val repo = FakeChatRepo()
        repo.fehler = IOException("kein Netz")
        val vm = ChatViewModel(repo)
        vm.standPruefen()
        advanceUntilIdle()

        vm.setEingabe("Was steht gerade an?")
        vm.absenden()
        advanceUntilIdle()

        // Die Frage steht wieder im Eingabefeld: Ein zweiter Versuch ist ein
        // Tipp und kein Abtippen.
        assertEquals("Was steht gerade an?", vm.state.value.eingabe)
        // Und im Gespräch bleibt keine Frage stehen, auf die nie eine
        // Antwort kam.
        assertTrue(vm.state.value.zuege.isEmpty())
        assertFalse(vm.state.value.wartet)
        assertEquals("Das hat nicht geklappt. Besteht eine Verbindung?", vm.state.value.fehler)
    }

    @Test
    fun `Ohne eingerichteten Chat laesst sich nichts abschicken`() = runTest(dispatcher) {
        val repo = FakeChatRepo()
        repo.stand = ChatStandDto(verfuegbar = false, hinweis = "Der Chat ist noch nicht eingerichtet.")
        val vm = ChatViewModel(repo)
        vm.standPruefen()
        advanceUntilIdle()

        assertFalse(vm.state.value.verfuegbar)
        assertEquals("Der Chat ist noch nicht eingerichtet.", vm.state.value.hinweis)

        vm.setEingabe("Was steht gerade an?")
        assertFalse(vm.state.value.absendbar)
        vm.absenden()
        advanceUntilIdle()
        assertTrue("ohne Schlüssel darf gar nicht erst gefragt werden", repo.fragen.isEmpty())
    }

    @Test
    fun `Faellt die Standabfrage aus gilt der Chat als verfuegbar`() = runTest(dispatcher) {
        val repo = FakeChatRepo()
        repo.standFehler = IOException("kein Netz")
        val vm = ChatViewModel(repo)
        vm.standPruefen()
        advanceUntilIdle()

        // Ein Ausfall der Leitung darf nicht aussehen wie eine dauerhafte
        // Abschaltung — der erste Versuch sagt dann die Wahrheit.
        assertTrue(vm.state.value.verfuegbar)
        assertFalse(vm.state.value.laedtStand)
        assertEquals("", vm.state.value.hinweis)
    }

    @Test
    fun `Eine leere Frage geht gar nicht erst hinaus`() = runTest(dispatcher) {
        val repo = FakeChatRepo()
        val vm = ChatViewModel(repo)
        vm.standPruefen()
        advanceUntilIdle()

        vm.setEingabe("   ")
        assertFalse(vm.state.value.absendbar)
        vm.absenden()
        advanceUntilIdle()
        assertTrue(repo.fragen.isEmpty())
    }

    @Test
    fun `Zu lange Eingaben werden gekuerzt statt abgewiesen`() = runTest(dispatcher) {
        val repo = FakeChatRepo()
        val vm = ChatViewModel(repo)
        vm.setEingabe("ä".repeat(ChatUiState.MAX_ZEICHEN + 500))
        assertEquals(ChatUiState.MAX_ZEICHEN, vm.state.value.eingabe.length)
    }

    @Test
    fun `Waehrend die Antwort unterwegs ist wird gewartet`() = runTest(dispatcher) {
        val repo = FakeChatRepo()
        val vm = ChatViewModel(repo)
        vm.standPruefen()
        advanceUntilIdle()

        vm.setEingabe("Was steht gerade an?")
        vm.absenden()
        // Noch nicht abgearbeitet: Die Frage steht schon im Gespräch, die
        // Antwort ist unterwegs, und ein zweites Absenden ist gesperrt.
        assertTrue(vm.state.value.wartet)
        assertEquals(1, vm.state.value.zuege.size)
        assertFalse(vm.state.value.absendbar)
        advanceUntilIdle()
        assertFalse(vm.state.value.wartet)
    }

    @Test
    fun `Eine abgebrochene Antwort wird als solche gemerkt`() = runTest(dispatcher) {
        val repo = FakeChatRepo()
        repo.antwort = ChatAntwortDto(antwort = "Das hat länger gedauert …", abgebrochen = true)
        val vm = ChatViewModel(repo)
        vm.standPruefen()
        advanceUntilIdle()

        vm.setEingabe("Rechne mir das ganze Dorf durch")
        vm.absenden()
        advanceUntilIdle()

        assertTrue(vm.state.value.zuege.last().abgebrochen)
    }

    @Test
    fun `Neu beginnen raeumt das Gespraech weg`() = runTest(dispatcher) {
        val repo = FakeChatRepo()
        val vm = ChatViewModel(repo)
        vm.standPruefen()
        advanceUntilIdle()
        vm.setEingabe("Moin")
        vm.absenden()
        advanceUntilIdle()

        vm.neuBeginnen()
        assertTrue(vm.state.value.zuege.isEmpty())
        assertNull(vm.state.value.fehler)
        // Der Stand bleibt: Ob der Chat eingerichtet ist, ändert sich davon nicht.
        assertTrue(vm.state.value.verfuegbar)
    }
}
