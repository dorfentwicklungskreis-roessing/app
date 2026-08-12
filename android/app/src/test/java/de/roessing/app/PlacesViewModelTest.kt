package de.roessing.app

import de.roessing.app.data.CompletionDto
import de.roessing.app.data.CompletionLockedException
import de.roessing.app.data.LatLon
import de.roessing.app.data.MeDto
import de.roessing.app.data.PlaceSort
import de.roessing.app.data.ROESSING
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.PlacesRepository
import de.roessing.app.data.PlacesResponse
import de.roessing.app.data.TaskDto
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

/** Fake-Repository mit steuerbaren Antworten. */
private class FakeRepo : PlacesRepository {
    var failPlaces = false
    var failComplete = false
    /** Wenn gesetzt, antwortet das Backend mit der Sperrfrist (HTTP 409). */
    var gesperrtBis: String? = null
    val completions = mutableListOf<Long>()

    private fun task(id: Long, status: String) = TaskDto(
        id = id, placeId = 1, kind = "giessen", liters = 10.0,
        intervalDays = 7.0, redAfterDays = 14.0, status = status,
    )

    var response = PlacesResponse(
        places = listOf(
            // Koordinaten so gewählt, dass die Entfernung eine andere
            // Reihenfolge ergibt als die Dringlichkeit.
            PlaceDto(id = 1, name = "Kasten Grün", lat = 52.2120, lon = 9.8700, status = "green", tasks = listOf(task(11, "green"))),
            PlaceDto(id = 2, name = "Kasten Rot", lat = 52.3000, lon = 9.8700, status = "red", tasks = listOf(task(22, "red"))),
            PlaceDto(id = 3, name = "Kasten Gelb", lat = 52.2110, lon = 9.8700, status = "yellow", tasks = listOf(task(33, "yellow"))),
        ),
        wateringFactor = 1.0,
    )

    override suspend fun me() = MeDto(sub = "u1", name = "Erna", isAdmin = false)

    override suspend fun places(): PlacesResponse {
        if (failPlaces) throw RuntimeException("offline")
        return response
    }

    override suspend fun complete(taskId: Long, liters: Double?, note: String): CompletionDto {
        gesperrtBis?.let { throw CompletionLockedException(it) }
        if (failComplete) throw RuntimeException("offline")
        completions += taskId
        return CompletionDto(id = 99, taskId = taskId, userName = "Erna")
    }

    override suspend fun completions(taskId: Long): List<CompletionDto> = emptyList()
}

@OptIn(ExperimentalCoroutinesApi::class)
class PlacesViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before
    fun setup() {
        Dispatchers.setMain(dispatcher)
    }

    @After
    fun teardown() {
        Dispatchers.resetMain()
    }

    @Test
    fun `laedt Orte und sortiert dringendste zuerst`() = runTest(dispatcher) {
        val vm = PlacesViewModel(FakeRepo())
        dispatcher.scheduler.advanceUntilIdle()
        val state = vm.state.value
        assertFalse(state.loading)
        assertEquals(listOf("Kasten Rot", "Kasten Gelb", "Kasten Grün"), state.places.map { it.name })
        assertEquals("Erna", state.me?.name)
    }

    @Test
    fun `netzfehler setzt offline-Flag und behaelt alte Daten`() = runTest(dispatcher) {
        val repo = FakeRepo()
        val vm = PlacesViewModel(repo)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(3, vm.state.value.places.size)

        repo.failPlaces = true
        vm.refresh()
        dispatcher.scheduler.advanceUntilIdle()
        assertTrue(vm.state.value.offline)
        assertEquals(3, vm.state.value.places.size)
    }

    @Test
    fun `erledigung melden ruft api und feuert erfolgs-event`() = runTest(dispatcher) {
        val repo = FakeRepo()
        val vm = PlacesViewModel(repo)
        dispatcher.scheduler.advanceUntilIdle()

        var event: UiEvent? = null
        val job = launch { event = vm.events.first() }
        vm.complete(22, liters = 10.0)
        dispatcher.scheduler.advanceUntilIdle()
        job.cancel()

        assertEquals(listOf(22L), repo.completions)
        assertEquals(UiEvent.CompletionSaved, event)
        assertTrue(vm.state.value.pendingTasks.isEmpty())
    }

    @Test
    fun `fehlgeschlagene meldung feuert fehler-event`() = runTest(dispatcher) {
        val repo = FakeRepo().apply { failComplete = true }
        val vm = PlacesViewModel(repo)
        dispatcher.scheduler.advanceUntilIdle()

        var event: UiEvent? = null
        val job = launch { event = vm.events.first() }
        vm.complete(22, liters = null)
        dispatcher.scheduler.advanceUntilIdle()
        job.cancel()

        assertEquals(UiEvent.CompletionFailed, event)
        assertTrue(repo.completions.isEmpty())
    }

    @Test
    fun `gesperrte Aufgabe meldet die Sperrfrist statt eines Netzfehlers`() = runTest(dispatcher) {
        val repo = FakeRepo().apply { gesperrtBis = "2026-08-15T09:00:00Z" }
        val vm = PlacesViewModel(repo)
        dispatcher.scheduler.advanceUntilIdle()

        var event: UiEvent? = null
        val job = launch { event = vm.events.first() }
        vm.complete(22, liters = null)
        dispatcher.scheduler.advanceUntilIdle()
        job.cancel()

        assertEquals(UiEvent.CompletionLocked("2026-08-15T09:00:00Z"), event)
        assertTrue(repo.completions.isEmpty())
    }

    @Test
    fun `Liste laesst sich nach Entfernung sortieren`() = runTest(dispatcher) {
        val vm = PlacesViewModel(FakeRepo())
        dispatcher.scheduler.advanceUntilIdle()
        // Ohne Standort bleibt es bei der Dringlichkeit.
        vm.setSort(PlaceSort.DISTANCE)
        assertEquals(
            listOf("Kasten Rot", "Kasten Gelb", "Kasten Grün"),
            vm.state.value.places.map { it.name },
        )

        vm.setUserLocation(ROESSING)
        assertEquals(PlaceSort.DISTANCE, vm.state.value.sort)
        assertEquals(
            listOf("Kasten Gelb", "Kasten Grün", "Kasten Rot"),
            vm.state.value.places.map { it.name },
        )

        // Zurück auf Dringlichkeit — der Standort bleibt bekannt.
        vm.setSort(PlaceSort.URGENCY)
        assertEquals(
            listOf("Kasten Rot", "Kasten Gelb", "Kasten Grün"),
            vm.state.value.places.map { it.name },
        )
        assertEquals(ROESSING, vm.state.value.userLocation)
    }

    @Test
    fun `Standort ueberlebt einen Refresh`() = runTest(dispatcher) {
        val vm = PlacesViewModel(FakeRepo())
        dispatcher.scheduler.advanceUntilIdle()
        vm.setUserLocation(LatLon(52.2115, 9.8700))
        vm.setSort(PlaceSort.DISTANCE)

        vm.refresh()
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(PlaceSort.DISTANCE, vm.state.value.sort)
        assertEquals("Kasten Grün", vm.state.value.places.first().name)
    }
}
