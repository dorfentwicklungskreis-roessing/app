package de.roessing.app

import de.roessing.app.data.BeitrittDto
import de.roessing.app.data.MemberDto
import de.roessing.app.data.TraegerDto
import de.roessing.app.data.TraegerRefusedException
import de.roessing.app.data.TraegerRepository
import de.roessing.app.ui.TraegerViewModel
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
 * Fake des Verzeichnisses: merkt sich, was geschickt wurde, und kann sowohl
 * eine begründete Absage des Servers als auch einen Netzfehler nachstellen.
 */
private class FakeTraegerRepo(var liste: List<TraegerDto> = emptyList()) : TraegerRepository {
    var eigene: List<BeitrittDto> = emptyList()
    var antraege: List<BeitrittDto> = emptyList()
    var dorf: List<MemberDto> = emptyList()

    var ladeFehler: Throwable? = null
    var absage: String? = null
    var fehler: Throwable? = null

    val beitritte = mutableListOf<Pair<Long, String>>()
    val entscheidungen = mutableListOf<Pair<Long, String>>()
    val aufnahmen = mutableListOf<Pair<Long, String>>()

    override suspend fun list(): List<TraegerDto> {
        ladeFehler?.let { throw it }
        return liste
    }

    override suspend fun detail(id: Long): TraegerDto =
        liste.find { it.id == id } ?: throw IOException("unbekannt")

    override suspend fun join(id: Long, reason: String): BeitrittDto {
        beitritte += id to reason
        absage?.let { throw TraegerRefusedException(it) }
        fehler?.let { throw it }
        return BeitrittDto(id = 1, traegerId = id, status = "beantragt", begruendung = reason)
    }

    override suspend fun requests(traegerId: Long): List<BeitrittDto> = antraege

    override suspend fun myRequests(): List<BeitrittDto> = eigene

    override suspend fun decide(requestId: Long, status: String): BeitrittDto {
        entscheidungen += requestId to status
        absage?.let { throw TraegerRefusedException(it) }
        fehler?.let { throw it }
        return BeitrittDto(id = requestId, status = status)
    }

    override suspend fun addMember(traegerId: Long, userSub: String): BeitrittDto {
        aufnahmen += traegerId to userSub
        absage?.let { throw TraegerRefusedException(it) }
        fehler?.let { throw it }
        return BeitrittDto(traegerId = traegerId, userSub = userSub, status = "erteilt")
    }

    override suspend fun villagers(): List<MemberDto> = dorf
}

private fun traeger(
    id: Long,
    name: String,
    parent: Long = 0,
    mitglied: Boolean = false,
    verwaltet: Boolean = false,
    moeglich: Boolean = true,
    hindernis: String = "",
    antrag: String = "",
    offen: Int = 0,
    sichtbarkeit: String = "offen",
) = TraegerDto(
    id = id, name = name, status = "zugelassen", sichtbarkeit = sichtbarkeit,
    parentId = parent, istMitglied = mitglied, darfVerwalten = verwaltet,
    beitrittMoeglich = moeglich, beitrittHindernis = hindernis,
    beitrittStatus = antrag, offeneBeitritte = offen,
)

/**
 * Was das Verzeichnis der Vereine und Gruppen versprechen muss.
 *
 * Hier wird nirgends nachgerechnet, wer beitreten darf oder wer einen Träger
 * sehen darf — das steht in `model.Zugriff` im Backend, und die Antwort kommt
 * fertig mit. Geprüft wird, dass die App sie **übernimmt**, samt Wortlaut,
 * wenn etwas schiefgeht.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class TraegerViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before fun vorher() = Dispatchers.setMain(dispatcher)

    @After fun nachher() = Dispatchers.resetMain()

    @Test
    fun `Arbeitskreise stehen unter ihrem Verein`() = runTest(dispatcher) {
        val repo = FakeTraegerRepo(
            listOf(
                traeger(1, "Dorfpflege Rössing e.V."),
                traeger(2, "AK 2 Umwelt und Natur", parent = 1),
                traeger(3, "AK 1 Bauen", parent = 1),
            ),
        )
        val vm = TraegerViewModel(repo)
        vm.load()
        advanceUntilIdle()

        assertEquals(listOf(1L), vm.state.value.roots.map { it.id })
        assertEquals(
            listOf("AK 1 Bauen", "AK 2 Umwelt und Natur"),
            vm.state.value.children(of = 1).map { it.name },
        )
        assertTrue("Genau eine Ebene", vm.state.value.children(of = 2).isEmpty())
    }

    /**
     * Ein Arbeitskreis kann sichtbar sein, sein Verein aber nicht — dann
     * verschwände er aus dem Verzeichnis, hinge er nur unter seinem Dach.
     */
    @Test
    fun `ein Arbeitskreis ohne sichtbares Dach steht oben drin`() = runTest(dispatcher) {
        val repo = FakeTraegerRepo(listOf(traeger(2, "AK 2 Umwelt und Natur", parent = 99)))
        val vm = TraegerViewModel(repo)
        vm.load()
        advanceUntilIdle()

        assertEquals(listOf(2L), vm.state.value.roots.map { it.id })
    }

    /**
     * Das Verzeichnis des Servers ist die einzige Auskunft darüber, ob es
     * hinter einem Trägernamen etwas zu sehen gibt. Eine eigene Regel daneben
     * wäre die Sichtbarkeitsprüfung zum zweiten Mal.
     */
    @Test
    fun `ein Weg zum Traeger gibt es nur wenn er im Verzeichnis steht`() = runTest(dispatcher) {
        val repo = FakeTraegerRepo(listOf(traeger(1, "Dorfpflege Rössing e.V.")))
        val vm = TraegerViewModel(repo)
        vm.load()
        advanceUntilIdle()

        assertTrue(vm.state.value.inDirectory(1))
        assertFalse("Eine geschlossene Gruppe steht nicht drin", vm.state.value.inDirectory(5))
        assertFalse("Ein Ort ohne Träger führt nirgendwohin", vm.state.value.inDirectory(0))
    }

    @Test
    fun `Mitmachen schickt die Begruendung ohne Leerraum`() = runTest(dispatcher) {
        val repo = FakeTraegerRepo(listOf(traeger(1, "Dorfpflege Rössing e.V.")))
        val vm = TraegerViewModel(repo)
        vm.load()
        advanceUntilIdle()

        vm.join(1, "  Ich wohne nebenan.  ")
        advanceUntilIdle()

        assertEquals(listOf(1L to "Ich wohne nebenan."), repo.beitritte)
        assertNull(vm.state.value.error)
    }

    /**
     * Ein 409 ist keine Panne: Die Lage passt nicht, und der Server sagt in
     * welcher Weise. Genau dieser Satz gehört auf den Schirm.
     */
    @Test
    fun `eine Absage zeigt den Satz des Servers`() = runTest(dispatcher) {
        val repo = FakeTraegerRepo(listOf(traeger(1, "Dorfpflege Rössing e.V.")))
        repo.absage = "Du gehörst schon dazu."
        val vm = TraegerViewModel(repo)
        vm.load()
        advanceUntilIdle()

        vm.join(1, "")
        advanceUntilIdle()

        assertEquals("Du gehörst schon dazu.", vm.state.value.error)
    }

    /**
     * Der wichtigste Fall überhaupt: Die Freigabe schreibt zuerst in die
     * Rössing-ID. Klappt das nicht (502/503), bleibt der Antrag offen — und
     * die App darf auf keinen Fall Erfolg melden, während die Tür zu bleibt.
     */
    @Test
    fun `eine gescheiterte Freigabe meldet keinen Erfolg`() = runTest(dispatcher) {
        val satz = "Die Mitgliedschaft konnte in der Rössing-ID nicht eingetragen werden — " +
            "der Antrag bleibt deshalb offen. Bitte gleich noch einmal versuchen."
        val repo = FakeTraegerRepo(listOf(traeger(1, "Dorfpflege", verwaltet = true, offen = 1)))
        val antrag = BeitrittDto(
            id = 4, traegerId = 1, userName = "Anna Beispiel", status = "beantragt",
        )
        repo.antraege = listOf(antrag)
        repo.absage = satz
        val vm = TraegerViewModel(repo)
        vm.load()
        vm.loadRequests(1)
        advanceUntilIdle()

        vm.decide(antrag, "erteilt")
        advanceUntilIdle()

        assertEquals("Der Wortlaut des Servers, nicht ein eigener", satz, vm.state.value.error)
        assertEquals("Der Antrag bleibt stehen", 1, vm.state.value.openRequests(1).size)
    }

    @Test
    fun `direkt aufnehmen schickt die Kennung der Person`() = runTest(dispatcher) {
        val repo = FakeTraegerRepo(
            listOf(
                traeger(
                    1, "Der stille Kreis", verwaltet = true, moeglich = false,
                    sichtbarkeit = "geschlossen",
                ),
            ),
        )
        repo.dorf = listOf(MemberDto(userSub = "anna-sub", name = "Anna Beispiel"))
        val vm = TraegerViewModel(repo)
        vm.load()
        vm.loadVillagers()
        advanceUntilIdle()

        vm.addMember(1, vm.state.value.villagers.first())
        advanceUntilIdle()

        assertEquals(listOf(1L to "anna-sub"), repo.aufnahmen)
        assertNull(vm.state.value.error)
    }

    /** Eine leere Liste im Funkloch wäre eine Falschaussage über das Dorf. */
    @Test
    fun `ein Ausfall leert das Verzeichnis nicht`() = runTest(dispatcher) {
        val repo = FakeTraegerRepo(listOf(traeger(1, "Dorfpflege Rössing e.V.")))
        val vm = TraegerViewModel(repo)
        vm.load()
        advanceUntilIdle()
        assertEquals(1, vm.state.value.all.size)

        repo.ladeFehler = IOException("weg")
        vm.load()
        advanceUntilIdle()

        assertEquals(1, vm.state.value.all.size)
        assertTrue(vm.state.value.notice!!.isNotBlank())
    }

    @Test
    fun `offene Anfragen werden ueber alle Traeger gezaehlt`() = runTest(dispatcher) {
        val repo = FakeTraegerRepo(
            listOf(
                traeger(1, "Dorfpflege", verwaltet = true, offen = 2),
                traeger(2, "AK 2", parent = 1, verwaltet = true, offen = 1),
                traeger(3, "Feuerwehr"),
            ),
        )
        val vm = TraegerViewModel(repo)
        vm.load()
        advanceUntilIdle()

        assertEquals(3, vm.state.value.openRequestsForMe)
    }

    @Test
    fun `eigene offene Anfragen stehen fuer sich`() = runTest(dispatcher) {
        val repo = FakeTraegerRepo(listOf(traeger(1, "Dorfpflege")))
        repo.eigene = listOf(
            BeitrittDto(id = 1, traegerId = 1, traegerName = "Dorfpflege", status = "beantragt"),
            BeitrittDto(id = 2, traegerId = 2, traegerName = "Feuerwehr", status = "abgelehnt"),
        )
        val vm = TraegerViewModel(repo)
        vm.load()
        advanceUntilIdle()

        assertEquals(listOf(1L), vm.state.value.myPending.map { it.id })
    }

    /**
     * Ein Netzfehler ist keine Antwort des Servers — er darf nicht so
     * aussehen, als hätte jemand etwas entschieden.
     */
    @Test
    fun `ohne Verbindung steht ein eigener Satz da`() {
        assertEquals(
            "Das hat nicht geklappt. Besteht eine Verbindung?",
            TraegerViewModel.fehlertext(IOException("weg")),
        )
        assertEquals("Du gehörst schon dazu.", TraegerViewModel.fehlertext(TraegerRefusedException("Du gehörst schon dazu.")))
    }

    @Test
    fun `der Anzeigename faellt auf die Kennung zurueck`() {
        assertEquals("Anna", TraegerViewModel.anzeigename(MemberDto(userSub = "a", name = "Anna")))
        assertEquals("a", TraegerViewModel.anzeigename(MemberDto(userSub = "a")))
    }
}
