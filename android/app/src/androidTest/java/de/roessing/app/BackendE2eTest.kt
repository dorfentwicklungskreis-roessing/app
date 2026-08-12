package de.roessing.app

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import de.roessing.app.data.ApiPlacesRepository
import de.roessing.app.data.DorfApi
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
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

    private fun repo() = ApiPlacesRepository(
        DorfApi.create(BuildConfig.API_BASE_URL) { "e2e-user:E2E Tester:admin" },
    )

    @Test
    fun kompletterGiessFlow() = runBlocking {
        val repo = repo()

        // Seed-Daten sind da.
        val before = repo.places()
        assertTrue("Keine Orte im Backend", before.places.isNotEmpty())
        val task = before.places.flatMap { it.tasks }.firstOrNull { it.kind == "giessen" }
            ?: error("Keine Gieß-Aufgabe im Seed")

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
}
