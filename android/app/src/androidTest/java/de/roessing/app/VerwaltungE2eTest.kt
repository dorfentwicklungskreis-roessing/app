package de.roessing.app

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import de.roessing.app.data.ApiPlacesRepository
import de.roessing.app.data.ApiStatsRepository
import de.roessing.app.data.ApiVerwaltungRepository
import de.roessing.app.data.DorfApi
import de.roessing.app.data.PlaceEingabe
import de.roessing.app.data.TaskEingabe
import de.roessing.app.data.VerwaltungAbgelehntException
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith
import java.time.LocalDate
import java.time.format.DateTimeFormatter

/**
 * Die Verwaltung gegen ein echtes Backend (AUTH_MODE=insecure-dev), ohne
 * Attrappen. Geprüft wird der ganze Weg, um den es in #5 bis #7 geht:
 *
 *   anlegen → erscheint auf der Karte → wird erledigt → verschwindet
 *   (bzw. bleibt, wenn der Schalter aus ist)
 *
 * Dazu die Rolle: Wer kein Admin ist, wird vom Backend abgewiesen — nicht
 * bloß in der Oberfläche ausgeblendet.
 */
@RunWith(AndroidJUnit4::class)
class VerwaltungE2eTest {
    @Before
    fun nurImE2eModus() {
        assumeTrue(
            "E2E-Modus nicht aktiv (Instrumentation-Arg e2e fehlt)",
            InstrumentationRegistry.getArguments().getString("e2e") == "true",
        )
    }

    private val adminToken = "verwaltung-e2e:Verwaltung E2E:admin"
    private val mitgliedToken = "mitglied-e2e:Mitglied E2E:"

    private fun api(token: String) = DorfApi.create(BuildConfig.API_BASE_URL) { token }

    private fun verwaltung(token: String = adminToken) = ApiVerwaltungRepository(api(token))

    private fun orte(token: String = adminToken) = ApiPlacesRepository(api(token))

    private fun stats(token: String = adminToken) = ApiStatsRepository(api(token))

    /** Ein eigener Ort je Testlauf — die Tests teilen sich ein Backend. */
    private suspend fun eigenerOrt(name: String) = verwaltung().ortAnlegen(
        PlaceEingabe(
            name = "$name ${System.currentTimeMillis()}",
            kind = "sonstiges", lat = 52.2108, lon = 9.8692,
        ),
    )

    private fun inTagen(tage: Long): String =
        LocalDate.now().plusDays(tage).format(DateTimeFormatter.ISO_LOCAL_DATE)

    @Test
    fun einmaligeAufgabeAnlegenErledigenUndVerschwinden() = runBlocking {
        val ort = eigenerOrt("Bahnhof")

        val aufgabe = verwaltung().aufgabeAnlegen(
            ort.id,
            TaskEingabe(
                kind = "sonstiges", title = "Zum Bahnhof fahren",
                oneOff = true, dueDate = inTagen(10), removeWhenDone = true,
            ),
        )
        assertTrue("nicht als einmalig gespeichert", aufgabe.oneOff)
        assertNotNull("kein Termin gespeichert", aufgabe.dueDate)

        // Sie steht auf der Karte: der Ort ist da, die Aufgabe hängt dran.
        val vorher = orte().places().places.first { it.id == ort.id }
        assertEquals(1, vorher.tasks.size)
        assertEquals("green", vorher.tasks.first().status)

        val ranglisteVorher = stats().leaderboard("gesamt").totals.completions

        // Ein gewöhnliches Mitglied erledigt sie — das darf jeder.
        ApiPlacesRepository(api(mitgliedToken)).complete(aufgabe.id, liters = null)

        // Und weg von der Karte.
        val nachher = orte().places().places.first { it.id == ort.id }
        assertTrue(
            "Die erledigte einmalige Aufgabe steht noch auf der Karte: ${nachher.tasks}",
            nachher.tasks.isEmpty(),
        )

        // Die Erledigung zählt weiter — sonst verschwänden Verdienste.
        assertEquals(
            "Die Erledigung zählt nicht mehr für die Rangliste",
            ranglisteVorher + 1, stats().leaderboard("gesamt").totals.completions,
        )
    }

    @Test
    fun einmaligeAufgabeOhneSchalterBleibtStehen() = runBlocking {
        val ort = eigenerOrt("Bank")
        val aufgabe = verwaltung().aufgabeAnlegen(
            ort.id,
            TaskEingabe(kind = "sonstiges", title = "Bank streichen", oneOff = true, dueDate = inTagen(5)),
        )
        ApiPlacesRepository(api(mitgliedToken)).complete(aufgabe.id, liters = null)

        val nachher = orte().places().places.first { it.id == ort.id }
        assertEquals("Aufgabe verschwunden, obwohl der Schalter aus ist", 1, nachher.tasks.size)
        assertEquals("green", nachher.tasks.first().status)
    }

    /** Der Termin macht die Ampel: heute fällig heißt rot. */
    @Test
    fun ueberfaelligeEinmaligeAufgabeIstRot() = runBlocking {
        val ort = eigenerOrt("Ueberfaellig")
        verwaltung().aufgabeAnlegen(
            ort.id,
            TaskEingabe(kind = "sonstiges", title = "Längst fällig", oneOff = true, dueDate = inTagen(-2)),
        )
        val geladen = orte().places().places.first { it.id == ort.id }
        assertEquals("red", geladen.tasks.first().status)
        assertEquals("red", geladen.status)
    }

    /** Und knapp vor dem Termin gelb. */
    @Test
    fun einmaligeAufgabeKurzVorDemTerminIstGelb() = runBlocking {
        val ort = eigenerOrt("Bald")
        verwaltung().aufgabeAnlegen(
            ort.id,
            TaskEingabe(kind = "sonstiges", title = "Übermorgen", oneOff = true, dueDate = inTagen(1)),
        )
        val geladen = orte().places().places.first { it.id == ort.id }
        assertEquals("yellow", geladen.tasks.first().status)
    }

    @Test
    fun pausierenUndFortsetzenEinerAufgabe() = runBlocking {
        val ort = eigenerOrt("Pause")
        val aufgabe = verwaltung().aufgabeAnlegen(
            ort.id,
            TaskEingabe(kind = "giessen", liters = 10.0, intervalDays = 7.0, redAfterDays = 14.0),
        )

        val pausiert = verwaltung().aufgabeAendern(
            aufgabe.id,
            TaskEingabe(kind = "giessen", liters = 10.0, intervalDays = 7.0, redAfterDays = 14.0, active = false),
        )
        assertFalse("Pausieren hat nicht gegriffen", pausiert.active)

        val wieder = verwaltung().aufgabeAendern(
            aufgabe.id,
            TaskEingabe(kind = "giessen", liters = 10.0, intervalDays = 7.0, redAfterDays = 14.0, active = true),
        )
        assertTrue("Fortsetzen hat nicht gegriffen", wieder.active)
    }

    @Test
    fun loeschenNimmtOrtUndAufgabeVonDerKarte() = runBlocking {
        val ort = eigenerOrt("Weg")
        val aufgabe = verwaltung().aufgabeAnlegen(
            ort.id,
            TaskEingabe(kind = "giessen", liters = 5.0, intervalDays = 7.0, redAfterDays = 14.0),
        )
        verwaltung().aufgabeLoeschen(aufgabe.id)
        assertTrue(
            "Aufgabe noch da",
            orte().places().places.first { it.id == ort.id }.tasks.isEmpty(),
        )

        verwaltung().ortLoeschen(ort.id)
        assertNull(
            "Ort noch da",
            orte().places().places.firstOrNull { it.id == ort.id },
        )
    }

    /**
     * Der wichtigste Teil: Ohne die Rolle „admin" weist der Server ab. Das
     * ist keine Frage der Oberfläche — hier geht der Aufruf direkt an die
     * Schnittstelle, wie es auch eine eigene App täte.
     */
    @Test
    fun ohneAdminRolleWeistDasBackendAb() = runBlocking {
        val ort = eigenerOrt("Verboten")
        val aufgabe = verwaltung().aufgabeAnlegen(
            ort.id,
            TaskEingabe(kind = "giessen", liters = 5.0, intervalDays = 7.0, redAfterDays = 14.0),
        )
        val alsMitglied = ApiVerwaltungRepository(api(mitgliedToken))

        val versuche: List<Pair<String, suspend () -> Unit>> = listOf(
            "Ort anlegen" to {
                alsMitglied.ortAnlegen(PlaceEingabe(name = "Heimlich", lat = 52.2, lon = 9.8))
                Unit
            },
            "Ort ändern" to {
                alsMitglied.ortAendern(ort.id, PlaceEingabe(name = "Umbenannt", lat = 52.2, lon = 9.8))
                Unit
            },
            "Ort löschen" to { alsMitglied.ortLoeschen(ort.id) },
            "Aufgabe anlegen" to {
                alsMitglied.aufgabeAnlegen(
                    ort.id,
                    TaskEingabe(kind = "sonstiges", title = "X", oneOff = true, dueDate = inTagen(3)),
                )
                Unit
            },
            "Aufgabe ändern" to {
                alsMitglied.aufgabeAendern(
                    aufgabe.id,
                    TaskEingabe(kind = "giessen", intervalDays = 1.0, redAfterDays = 2.0),
                )
                Unit
            },
            "Aufgabe löschen" to { alsMitglied.aufgabeLoeschen(aufgabe.id) },
        )
        for ((name, versuch) in versuche) {
            var abgewiesen = false
            try {
                versuch()
            } catch (e: VerwaltungAbgelehntException) {
                abgewiesen = true
                assertTrue("Begründung fehlt bei $name", e.grund.isNotBlank())
            }
            assertTrue("$name wurde einem Mitglied erlaubt", abgewiesen)
        }

        // Und der Bestand steht unverändert da.
        val geladen = orte().places().places.first { it.id == ort.id }
        assertEquals(1, geladen.tasks.size)
    }
}
