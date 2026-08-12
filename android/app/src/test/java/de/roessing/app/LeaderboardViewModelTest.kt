package de.roessing.app

import de.roessing.app.data.BadgeDto
import de.roessing.app.data.LeaderboardDto
import de.roessing.app.data.LeaderboardEntryDto
import de.roessing.app.data.LeaderboardTotalsDto
import de.roessing.app.data.StatsRepository
import de.roessing.app.ui.LeaderboardPeriod
import de.roessing.app.ui.LeaderboardViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
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

private fun eintrag(rang: Int, sub: String, name: String, anzahl: Int, liter: Double = 0.0) =
    LeaderboardEntryDto(
        rank = rang, userSub = sub, userName = name, completions = anzahl,
        byKind = mapOf("giessen" to anzahl, "jaeten" to 0, "sonstiges" to 0), liters = liter,
    )

/** Fake der Auswertungs-API: merkt sich die angefragten Zeiträume. */
private class FakeStatsRepo : StatsRepository {
    var fehler = false
    val angefragt = mutableListOf<String>()

    var antwort = LeaderboardDto(
        period = "saison",
        entries = listOf(
            eintrag(1, "erna", "Erna", 12, 120.0).copy(
                badges = listOf(BadgeDto("giesskanne", "Gießkanne des Monats", "Die meisten Gießungen.")),
            ),
            eintrag(2, "karl", "Karl", 8, 40.0),
            eintrag(3, "berta", "Berta", 5),
            eintrag(4, "udo", "Udo", 2),
        ),
        totals = LeaderboardTotalsDto(completions = 27, liters = 160.0, participants = 4),
        me = eintrag(4, "udo", "Udo", 2),
    )

    override suspend fun leaderboard(period: String): LeaderboardDto {
        angefragt += period
        if (fehler) throw RuntimeException("offline")
        return antwort
    }
}

@OptIn(ExperimentalCoroutinesApi::class)
class LeaderboardViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before
    fun setup() = Dispatchers.setMain(dispatcher)

    @After
    fun teardown() = Dispatchers.resetMain()

    @Test
    fun `laedt beim Start die Saison`() = runTest(dispatcher) {
        val repo = FakeStatsRepo()
        val vm = LeaderboardViewModel(repo)
        dispatcher.scheduler.advanceUntilIdle()

        assertEquals(listOf("saison"), repo.angefragt)
        val state = vm.state.value
        assertFalse(state.loading)
        assertEquals(LeaderboardPeriod.SAISON, state.period)
        assertEquals(listOf("Erna", "Karl", "Berta", "Udo"), state.entries.map { it.userName })
        assertEquals(27, state.totals.completions)
    }

    @Test
    fun `podest sind die ersten drei`() = runTest(dispatcher) {
        val vm = LeaderboardViewModel(FakeStatsRepo())
        dispatcher.scheduler.advanceUntilIdle()

        assertEquals(listOf("Erna", "Karl", "Berta"), vm.state.value.podest.map { it.userName })
    }

    @Test
    fun `eigener Rang bleibt auch ausserhalb der vorderen Plaetze erhalten`() = runTest(dispatcher) {
        val vm = LeaderboardViewModel(FakeStatsRepo())
        dispatcher.scheduler.advanceUntilIdle()

        val ich = vm.state.value.me
        assertEquals("Udo", ich?.userName)
        assertEquals(4, ich?.rank)
        assertTrue("Udo steht nicht auf dem Podest", vm.state.value.podest.none { it.userSub == "udo" })
    }

    @Test
    fun `zeitraumwechsel fragt neu an`() = runTest(dispatcher) {
        val repo = FakeStatsRepo()
        val vm = LeaderboardViewModel(repo)
        dispatcher.scheduler.advanceUntilIdle()

        vm.select(LeaderboardPeriod.MONAT)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(listOf("saison", "monat"), repo.angefragt)
        assertEquals(LeaderboardPeriod.MONAT, vm.state.value.period)

        // Derselbe Zeitraum noch einmal löst keine zweite Abfrage aus.
        vm.select(LeaderboardPeriod.MONAT)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(listOf("saison", "monat"), repo.angefragt)
    }

    @Test
    fun `leerer Zeitraum liefert leere Liste ohne eigenen Rang`() = runTest(dispatcher) {
        val repo = FakeStatsRepo().apply {
            antwort = LeaderboardDto(period = "woche", me = LeaderboardEntryDto(userSub = "udo", userName = "Udo"))
        }
        val vm = LeaderboardViewModel(repo)
        dispatcher.scheduler.advanceUntilIdle()

        assertTrue(vm.state.value.entries.isEmpty())
        assertTrue(vm.state.value.podest.isEmpty())
        assertEquals(0, vm.state.value.me?.rank)
        assertEquals(0, vm.state.value.totals.completions)
    }

    @Test
    fun `netzfehler setzt offline-Flag und behaelt die alten Daten`() = runTest(dispatcher) {
        val repo = FakeStatsRepo()
        val vm = LeaderboardViewModel(repo)
        dispatcher.scheduler.advanceUntilIdle()
        assertEquals(4, vm.state.value.entries.size)

        repo.fehler = true
        vm.select(LeaderboardPeriod.WOCHE)
        dispatcher.scheduler.advanceUntilIdle()

        assertTrue(vm.state.value.offline)
        assertFalse(vm.state.value.loading)
        assertEquals(4, vm.state.value.entries.size)

        // Nach einem erfolgreichen Versuch ist das Offline-Flag wieder weg.
        repo.fehler = false
        vm.refresh()
        dispatcher.scheduler.advanceUntilIdle()
        assertFalse(vm.state.value.offline)
    }

    @Test
    fun `ohne Daten ist kein eigener Eintrag gesetzt`() = runTest(dispatcher) {
        val repo = FakeStatsRepo().apply { antwort = LeaderboardDto() }
        val vm = LeaderboardViewModel(repo)
        dispatcher.scheduler.advanceUntilIdle()

        assertNull(vm.state.value.me)
    }
}
