package de.roessing.app

import de.roessing.app.data.AssignmentDto
import de.roessing.app.data.AssignmentTakenException
import de.roessing.app.data.CompletionDto
import de.roessing.app.data.MeDto
import de.roessing.app.data.NotificationDto
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.PlacesRepository
import de.roessing.app.data.PlacesResponse
import de.roessing.app.data.TaskDto
import de.roessing.app.data.VergabeRepository
import de.roessing.app.ui.PlacesViewModel
import de.roessing.app.ui.UiEvent
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Tests der Vergabe in der App: sich als Helfer:in eintragen, Anfragen sehen,
 * zusagen — und der Fall, in dem jemand anderes schneller war.
 */

/** Fake der Vergabe-Endpunkte mit steuerbaren Antworten. */
class FakeVergabe : VergabeRepository {
    var anfragen = mutableListOf<NotificationDto>()
    val eingetragen = mutableListOf<Pair<Long, String?>>()
    val ausgetragen = mutableListOf<Pair<Long, String?>>()
    val bestaetigt = mutableListOf<Long>()
    val zugesagt = mutableListOf<Long>()
    val zurueckgegeben = mutableListOf<Long>()

    /** Wenn gesetzt, meldet das Backend „jemand anderes war schneller" (409). */
    var schonVergeben: String? = null
    var fehler = false
    var abrufe = 0

    override suspend fun notifications(): List<NotificationDto> {
        abrufe++
        if (fehler) throw RuntimeException("offline")
        return anfragen
    }

    override suspend fun ack(id: Long) {
        if (fehler) throw RuntimeException("offline")
        bestaetigt += id
        anfragen = anfragen.filterNot { it.id == id && !it.istAnfrage }.toMutableList()
    }

    override suspend fun signup(placeId: Long, taskKind: String?) {
        if (fehler) throw RuntimeException("offline")
        eingetragen += placeId to taskKind
    }

    override suspend fun signoff(placeId: Long, taskKind: String?) {
        if (fehler) throw RuntimeException("offline")
        ausgetragen += placeId to taskKind
    }

    override suspend fun claim(assignmentId: Long): AssignmentDto {
        schonVergeben?.let { throw AssignmentTakenException(it) }
        if (fehler) throw RuntimeException("offline")
        zugesagt += assignmentId
        return AssignmentDto(id = assignmentId, state = "uebernommen", claimedBy = "u1", claimedByName = "Erna")
    }

    override suspend fun release(assignmentId: Long): AssignmentDto {
        if (fehler) throw RuntimeException("offline")
        zurueckgegeben += assignmentId
        return AssignmentDto(id = assignmentId, state = "offen")
    }
}

private class VergabeRepo : PlacesRepository {
    var signedUp = false
    val completions = mutableListOf<Long>()

    fun response() = PlacesResponse(
        places = listOf(
            PlaceDto(
                id = 1, name = "Unter den Eichen", lat = 52.2110, lon = 9.8697, status = "yellow",
                tasks = listOf(
                    TaskDto(
                        id = 11, placeId = 1, kind = "giessen", liters = 10.0, status = "yellow",
                        signupCount = 3, signedUp = signedUp,
                        assignment = AssignmentDto(
                            id = 5, taskId = 11, state = "offen", askedCount = 1,
                        ),
                    ),
                ),
            ),
        ),
    )

    override suspend fun me() = MeDto(sub = "u1", name = "Erna")
    override suspend fun places() = response()
    override suspend fun complete(taskId: Long, liters: Double?, note: String): CompletionDto {
        completions += taskId
        return CompletionDto(id = 1, taskId = taskId)
    }

    override suspend fun completions(taskId: Long): List<CompletionDto> = emptyList()
}

private fun anfrage(id: Long = 1, kind: String = "anfrage") = NotificationDto(
    id = id, assignmentId = 5, taskId = 11, placeId = 1, kind = kind,
    taskName = "Gießen", placeName = "Unter den Eichen",
    title = "Gießen an „Unter den Eichen“ ist dran",
    text = "Du bist als Nächste(r) an der Reihe.",
    expiresAt = "2026-08-14T12:00:00Z",
)

@OptIn(ExperimentalCoroutinesApi::class)
class VergabeViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before
    fun setup() = Dispatchers.setMain(dispatcher)

    @After
    fun teardown() = Dispatchers.resetMain()

    @Test
    fun `Vergabestand kommt mit der Orts-Liste`() = runTest(dispatcher) {
        val vm = PlacesViewModel(VergabeRepo(), FakeVergabe())
        dispatcher.scheduler.advanceUntilIdle()
        val task = vm.state.value.places.first().tasks.first()
        assertEquals(3, task.signupCount)
        assertFalse(task.signedUp)
        assertEquals(5L, task.assignment?.id)
    }

    @Test
    fun `Benachrichtigungen werden beim Start geladen`() = runTest(dispatcher) {
        val vergabe = FakeVergabe().apply { anfragen += anfrage() }
        val vm = PlacesViewModel(VergabeRepo(), vergabe)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(1, vm.state.value.notifications.size)
        assertEquals("Gießen an „Unter den Eichen“ ist dran", vm.state.value.notifications.first().title)
    }

    @Test
    fun `Eintragen und wieder austragen`() = runTest(dispatcher) {
        val repo = VergabeRepo()
        val vergabe = FakeVergabe()
        val vm = PlacesViewModel(repo, vergabe)
        dispatcher.scheduler.advanceUntilIdle()

        vm.setSignup(placeId = 1, taskKind = "giessen", an = true)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(listOf(1L to "giessen"), vergabe.eingetragen)
        assertTrue(vm.state.value.pendingSignups.isEmpty())

        vm.setSignup(placeId = 1, taskKind = "giessen", an = false)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(listOf(1L to "giessen"), vergabe.ausgetragen)
    }

    @Test
    fun `Zusage meldet Erfolg und laedt neu`() = runTest(dispatcher) {
        val vergabe = FakeVergabe().apply { anfragen += anfrage() }
        val vm = PlacesViewModel(VergabeRepo(), vergabe)
        dispatcher.scheduler.advanceUntilIdle()
        val vorher = vergabe.abrufe

        var event: UiEvent? = null
        val job = launch { event = vm.events.first() }
        vm.claim(5)
        dispatcher.scheduler.advanceUntilIdle()
        job.cancel()

        assertEquals(listOf(5L), vergabe.zugesagt)
        assertEquals(UiEvent.AssignmentClaimed, event)
        assertTrue("nach der Zusage muss neu geladen werden", vergabe.abrufe > vorher)
    }

    @Test
    fun `jemand anderes war schneller`() = runTest(dispatcher) {
        val vergabe = FakeVergabe().apply {
            anfragen += anfrage()
            schonVergeben = "Diese Aufgabe wurde gerade schon von Bernd übernommen."
        }
        val vm = PlacesViewModel(VergabeRepo(), vergabe)
        dispatcher.scheduler.advanceUntilIdle()

        var event: UiEvent? = null
        val job = launch { event = vm.events.first() }
        vm.claim(5)
        dispatcher.scheduler.advanceUntilIdle()
        job.cancel()

        assertTrue(vergabe.zugesagt.isEmpty())
        assertTrue("erwartet AssignmentTaken, bekommen $event", event is UiEvent.AssignmentTaken)
        assertEquals(
            "Diese Aufgabe wurde gerade schon von Bernd übernommen.",
            (event as UiEvent.AssignmentTaken).grund,
        )
        assertTrue(vm.state.value.pendingAssignments.isEmpty())
    }

    @Test
    fun `Zurueckgeben gibt den Vorgang frei`() = runTest(dispatcher) {
        val vergabe = FakeVergabe()
        val vm = PlacesViewModel(VergabeRepo(), vergabe)
        dispatcher.scheduler.advanceUntilIdle()

        vm.release(5)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(listOf(5L), vergabe.zurueckgegeben)
    }

    @Test
    fun `Empfang bestaetigen laesst Hinweise verschwinden`() = runTest(dispatcher) {
        val vergabe = FakeVergabe().apply {
            anfragen += anfrage(id = 1, kind = "vorgang_beendet")
            anfragen += anfrage(id = 2, kind = "anfrage")
        }
        val vm = PlacesViewModel(VergabeRepo(), vergabe)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(2, vm.state.value.notifications.size)

        vm.acknowledge(1)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(listOf(1L), vergabe.bestaetigt)
        assertEquals(listOf(2L), vm.state.value.notifications.map { it.id })
    }

    // Wer meldet, dass er gegossen hat, beendet den Vorgang — die Anfrage
    // darf danach nicht weiter in der App stehen.
    @Test
    fun `nach dem Melden werden die Benachrichtigungen neu geholt`() = runTest(dispatcher) {
        val vergabe = FakeVergabe().apply { anfragen += anfrage() }
        val vm = PlacesViewModel(VergabeRepo(), vergabe)
        dispatcher.scheduler.advanceUntilIdle()
        val vorher = vergabe.abrufe

        vergabe.anfragen.clear()
        vm.complete(11, liters = 10.0)
        dispatcher.scheduler.advanceUntilIdle()

        assertTrue("nach dem Melden muss neu geladen werden", vergabe.abrufe > vorher)
        assertTrue(vm.state.value.notifications.isEmpty())
    }

    // Ein Netzfehler beim Abholen darf die Startseite nicht leeren.
    @Test
    fun `Netzfehler behaelt die bekannten Benachrichtigungen`() = runTest(dispatcher) {
        val vergabe = FakeVergabe().apply { anfragen += anfrage() }
        val vm = PlacesViewModel(VergabeRepo(), vergabe)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(1, vm.state.value.notifications.size)

        vergabe.fehler = true
        vm.loadNotifications()
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(1, vm.state.value.notifications.size)
    }

    @Test
    fun `offene Anfragen sind fuer die Startseite zaehlbar`() = runTest(dispatcher) {
        val vergabe = FakeVergabe().apply {
            anfragen += anfrage(id = 1, kind = "anfrage")
            anfragen += anfrage(id = 2, kind = "vorgang_beendet")
        }
        val vm = PlacesViewModel(VergabeRepo(), vergabe)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(2, vm.state.value.notifications.size)
        assertEquals(1, vm.state.value.offeneAnfragen)
    }
}
