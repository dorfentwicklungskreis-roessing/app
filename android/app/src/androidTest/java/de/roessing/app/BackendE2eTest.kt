package de.roessing.app

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import de.roessing.app.auth.TokenResult
import de.roessing.app.data.ApiPlacesRepository
import de.roessing.app.data.ApiProfileRepository
import de.roessing.app.data.ApiStatsRepository
import de.roessing.app.data.DorfApi
import de.roessing.app.data.ProfileInput
import de.roessing.app.data.ProfileValidationException
import de.roessing.app.data.ProfileVisibilityDto
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import org.junit.After
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

    private fun api() = DorfApi.create(BuildConfig.API_BASE_URL) {
        TokenResult.Token("e2e-user:E2E Tester:admin")
    }

    private fun repo() = ApiPlacesRepository(api())

    private fun stats() = ApiStatsRepository(api())

    private fun profile() = ApiProfileRepository(api())

    /**
     * Setzt das Profil auf den Auslieferungszustand zurück.
     *
     * Muss sein: Andere Tests dieser Klasse prüfen die Rangliste auf den
     * Namen aus dem Token („E2E Tester"). Bliebe hier ein Nickname stehen,
     * würden sie je nach Reihenfolge scheitern — die Tests teilen sich ein
     * Backend.
     */
    @After
    fun profilZuruecksetzen() {
        if (InstrumentationRegistry.getArguments().getString("e2e") != "true") return
        runBlocking {
            runCatching { profile().saveProfile(ProfileInput(displayName = "E2E Tester")) }
        }
    }

    /**
     * Legt einen eigenen Ort mit einer Gieß-Aufgabe an und liefert deren ID.
     *
     * Die Seed-Daten haben nur eine Handvoll unberührter Gieß-Aufgaben, und
     * jede Meldung verbraucht eine davon (Spielschutz). Wer eine braucht,
     * bringt seine eigene mit, statt den anderen Tests eine wegzunehmen.
     * Anlegen darf das nur die Verwaltung — der E2E-Nutzer hat die Rolle.
     */
    private fun eigeneGiessAufgabe(): Long {
        val basis = BuildConfig.API_BASE_URL.trimEnd('/')
        val client = OkHttpClient()
        val json = "application/json".toMediaType()
        fun post(pfad: String, koerper: String): JSONObject {
            val anfrage = Request.Builder()
                .url("$basis$pfad")
                .header("Authorization", "Bearer e2e-user:E2E Tester:admin")
                .post(koerper.toRequestBody(json))
                .build()
            client.newCall(anfrage).execute().use { antwort ->
                val text = antwort.body?.string().orEmpty()
                assertTrue("POST $pfad: HTTP ${antwort.code} $text", antwort.isSuccessful)
                return JSONObject(text)
            }
        }
        val ort = post(
            "/api/v1/places",
            """{"name":"Profil-E2E ${System.currentTimeMillis()}","kind":"blumenkasten","lat":52.2105,"lon":9.8695}""",
        )
        val aufgabe = post(
            "/api/v1/places/${ort.getLong("id")}/tasks",
            """{"kind":"giessen","liters":5,"intervalDays":7,"redAfterDays":14}""",
        )
        return aufgabe.getLong("id")
    }

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

    // --- Profilverwaltung gegen das echte Backend ---

    @Test
    fun profilKommtVorbelegtUndKontaktdatenSindNichtOeffentlich() = runBlocking {
        val p = profile().profile()
        assertEquals("E2E Tester", p.displayName)
        assertTrue("Anzeigename sollte sichtbar sein", p.visibility.displayNameIsPublic)
        assertTrue("Telefon darf nicht sichtbar sein", !p.visibility.phoneIsPublic)
        assertTrue("E-Mail darf nicht sichtbar sein", !p.visibility.emailIsPublic)
    }

    @Test
    fun profilSpeichernUndInDerRanglisteWiederfinden() = runBlocking {
        val repo = repo()
        val profil = profile()
        val nickname = "E2E-Gießmeister-" + System.currentTimeMillis()

        val gespeichert = profil.saveProfile(
            ProfileInput(
                displayName = "E2E Tester",
                nickname = nickname,
                phone = "05066 123456",
                email = "e2e@example.org",
                note = "erreichbar abends",
                visibility = ProfileVisibilityDto(
                    phone = ProfileVisibilityDto.SICHTBAR_DORF,
                    email = ProfileVisibilityDto.SICHTBAR_VERWALTUNG,
                    note = ProfileVisibilityDto.SICHTBAR_VERWALTUNG,
                ),
            ),
        )
        assertEquals(nickname, gespeichert.nickname)
        assertTrue("Telefon wurde nicht freigegeben", gespeichert.visibility.phoneIsPublic)

        // Melden und in der Rangliste unter dem Nickname wiederfinden.
        repo.complete(eigeneGiessAufgabe(), liters = 3.0)
        val liste = stats().leaderboard("gesamt")
        assertEquals("Rangliste nutzt den Nickname nicht", nickname, liste.me?.userName)
    }

    @Test
    fun dorfbewohnerListeLiefertNurFreigegebenes() = runBlocking {
        val profil = profile()
        profil.saveProfile(
            ProfileInput(
                displayName = "E2E Tester",
                phone = "05066 123456",
                email = "e2e@example.org",
                visibility = ProfileVisibilityDto(
                    phone = ProfileVisibilityDto.SICHTBAR_DORF,
                    email = ProfileVisibilityDto.SICHTBAR_VERWALTUNG,
                ),
            ),
        )
        // Der E2E-Nutzer ist Admin, sieht also alles — aber gekennzeichnet.
        val (liste, adminSicht) = profil.members()
        assertTrue("Verwaltungs-Sicht wird nicht gemeldet", adminSicht)
        val ich = liste.firstOrNull { it.phone == "05066 123456" }
        assertNotNull("Eigener Eintrag fehlt", ich)
        assertTrue(
            "Die gesperrte E-Mail ist nicht gekennzeichnet",
            ich!!.nurFuerVerwaltung("email"),
        )
        assertTrue(
            "Das freigegebene Telefon steht fälschlich in restricted",
            !ich.nurFuerVerwaltung("phone"),
        )
    }

    @Test
    fun backendWeistUnsinnigeEingabenAb() = runBlocking {
        val profil = profile()
        for (kaputt in listOf(
            ProfileInput(email = "keine-adresse"),
            ProfileInput(phone = "ruf mich an"),
            ProfileInput(note = "abends\nund nachts"),
        )) {
            var abgelehnt = false
            try {
                profil.saveProfile(kaputt)
            } catch (e: ProfileValidationException) {
                abgelehnt = true
                assertTrue("Begründung fehlt", e.grund.isNotBlank())
            }
            assertTrue("Eingabe wurde angenommen: $kaputt", abgelehnt)
        }
    }
}
