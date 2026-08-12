package de.roessing.app

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import de.roessing.app.data.ApiPlacesRepository
import de.roessing.app.data.ApiStatsRepository
import de.roessing.app.data.DorfApi
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Echter End-to-End-Test gegen ein laufendes Backend (AUTH_MODE=insecure-dev).
 *
 * Läuft nur, wenn die Instrumentation mit `-e e2e true` gestartet wird —
 * in CI startet der Workflow vorher das Go-Backend und baut die App mit
 * `-PapiBaseUrl=http://10.0.2.2:8099 -PdevAuth=true`.
 */
@RunWith(AndroidJUnit4::class)
class BackendE2eTest {
    @Before
    fun onlyInE2eMode() {
        assumeTrue(
            "E2E-Modus nicht aktiv (Instrumentation-Arg e2e fehlt)",
            InstrumentationRegistry.getArguments().getString("e2e") == "true",
        )
    }

    private fun api() = DorfApi.create(BuildConfig.API_BASE_URL) { "e2e-user:E2E Tester:admin" }

    private fun repo() = ApiPlacesRepository(api())

    private fun stats() = ApiStatsRepository(api())

    /**
     * Eine Gieß-Aufgabe, die noch nie erledigt wurde. Der Spielschutz des
     * Backends sperrt eine Aufgabe nach jeder Meldung — die Tests dürfen sich
     * deshalb nicht dieselbe teilen.
     */
    private suspend fun freieGiessAufgabe() =
        repo().places().places.flatMap { it.tasks }
            .firstOrNull { it.kind == "giessen" && it.lastCompletion == null && it.lockedUntil == null }
            ?: error("Keine freie Gieß-Aufgabe im Seed")

    @Test
    fun kompletterGiessFlow() = runBlocking {
        val repo = repo()

        // Seed-Daten sind da.
        val before = repo.places()
        assertTrue("Keine Orte im Backend", before.places.isNotEmpty())
        val task = freieGiessAufgabe()

        // Gießen melden.
        val completion = repo.complete(task.id, liters = 10.0)
        assertEquals("E2E Tester", completion.userName)

        // Aufgabe ist danach grün und die Historie enthält die Meldung.
        val after = repo.places()
        val updated = after.places.flatMap { it.tasks }.first { it.id == task.id }
        assertEquals("green", updated.status)
        assertEquals("E2E Tester", updated.lastCompletion?.userName)
        assertTrue(repo.completions(task.id).isNotEmpty())
    }

    @Test
    fun meLiefertNutzer() = runBlocking {
        val me = repo().me()
        assertEquals("E2E Tester", me.name)
        assertTrue(me.isAdmin)
    }

    @Test
    fun ranglisteZaehltDieEigeneMeldung() = runBlocking {
        val repo = repo()
        val stats = stats()
        val task = freieGiessAufgabe()

        // Zeitraum „gesamt", damit der Test unabhängig vom Kalender läuft.
        val vorher = stats.leaderboard("gesamt")
        repo.complete(task.id, liters = 7.0)
        val nachher = stats.leaderboard("gesamt")

        assertEquals(
            "Die Meldung fehlt in der Gesamtsumme",
            vorher.totals.completions + 1, nachher.totals.completions,
        )
        val ich = nachher.me
        assertNotNull("Eigener Rang fehlt", ich)
        assertTrue("Eigener Rang ist nicht gesetzt", ich!!.rank >= 1)
        assertEquals("E2E Tester", ich.userName)
        assertTrue(
            "Eigene Meldung nicht gezählt",
            ich.completions > (vorher.me?.completions ?: 0) - 1,
        )
        assertTrue("Liter nicht summiert", ich.liters >= 7.0)
        assertTrue(
            "Eigener Eintrag fehlt in der Liste",
            nachher.entries.any { it.userSub == ich.userSub },
        )
        assertTrue("Aufgabenarten fehlen", ich.byKind.containsKey("giessen"))
    }

    @Test
    fun ranglisteKenntAlleZeitraeume() = runBlocking {
        val stats = stats()
        listOf("woche", "monat", "saison", "jahr", "gesamt").forEach { zeitraum ->
            val liste = stats.leaderboard(zeitraum)
            assertEquals(zeitraum, liste.period)
            assertNotNull("me fehlt für $zeitraum", liste.me)
        }
    }
}
