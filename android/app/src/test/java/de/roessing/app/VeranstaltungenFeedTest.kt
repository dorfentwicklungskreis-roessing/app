package de.roessing.app

import de.roessing.app.data.WebsiteApi
import de.roessing.app.data.WebsiteVeranstaltungenRepository
import de.roessing.app.data.alsTermine
import kotlinx.coroutines.test.runTest
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Instant

/**
 * Der Vertrag mit der Website: So sieht `/events.json` aus, und so wird es
 * gelesen. Der Ausschnitt stammt aus der tatsächlich gebauten Datei von
 * rössing.de (ergänzt um einen Termin mit externer Primärquelle).
 *
 * Ändert sich das Format dort, muss dieser Test hier auffallen — und nicht
 * erst eine leere Liste auf dem Telefon.
 */
class VeranstaltungenFeedTest {
    private lateinit var server: MockWebServer

    private val feed = """
        {
          "version": 1,
          "generatedAt": "2026-08-14T13:37:58.000Z",
          "events": [
            {
              "id": "2026-08-17-blutspende-drk",
              "name": "Blutspende",
              "description": "DRK-Blutspende im Dorfgemeinschaftshaus Rössing.",
              "start": "2026-08-17",
              "allDay": true,
              "url": "https://xn--rssing-wxa.de/events/2026-08-17-blutspende-drk",
              "external": false,
              "location": {
                "name": "Dorfgemeinschaftshaus Rössing",
                "address": "Kirchstraße 3, 31171 Nordstemmen"
              },
              "organizer": { "name": "DRK-Blutspendedienst" }
            },
            {
              "id": "2026-08-20-grillen-kirchenstiftung",
              "name": "Grillen im Pfarrgarten",
              "description": "Die Kirchenstiftung Rössing lädt zum Grillen im Pfarrgarten ein.",
              "start": "2026-08-20T18:00:00+02:00",
              "allDay": false,
              "url": "https://xn--rssing-wxa.de/events/2026-08-20-grillen-kirchenstiftung",
              "external": false,
              "location": {
                "name": "Pfarrgarten Rössing",
                "address": "Pfarrstr. 1, 31171 Nordstemmen",
                "lat": 52.1843,
                "lon": 9.8162
              },
              "organizer": { "name": "Kirchenstiftung Rössing" }
            },
            {
              "id": "2026-09-05-konzert",
              "name": "Jahreskonzert",
              "description": "Das Jahreskonzert des Musikzugs.",
              "start": "2026-09-05T19:00:00+02:00",
              "end": "2026-09-05T21:30:00+02:00",
              "allDay": false,
              "url": "https://musikzug-roessing.de/jahreskonzert",
              "external": true
            }
          ]
        }
    """.trimIndent()

    @Before
    fun vorher() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun nachher() = server.shutdown()

    private fun repo() =
        WebsiteVeranstaltungenRepository(WebsiteApi.create(server.url("/").toString()))

    @Test
    fun `Die Datei der Website wird vollstaendig gelesen`() = runTest {
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "application/json; charset=utf-8")
                .setBody(feed),
        )

        val termine = repo().kommende().alsTermine(Instant.parse("2026-08-14T10:00:00Z"))

        assertEquals(
            listOf(
                "2026-08-17-blutspende-drk",
                "2026-08-20-grillen-kirchenstiftung",
                "2026-09-05-konzert",
            ),
            termine.map { it.id },
        )

        val blutspende = termine.first()
        assertTrue(blutspende.ganztaegig)
        assertNull(blutspende.zeitText)
        assertEquals("Mo, 17.08.2026", blutspende.datumText)
        assertEquals("Dorfgemeinschaftshaus Rössing", blutspende.ortName)
        assertEquals("DRK-Blutspendedienst", blutspende.veranstalter)

        val grillen = termine[1]
        assertEquals("18:00 Uhr", grillen.zeitText)
        assertEquals(52.1843, grillen.koordinate?.lat ?: 0.0, 0.00001)

        // Externe Primärquelle: Der Tipp führt dorthin, nicht auf rössing.de.
        val konzert = termine.last()
        assertTrue(konzert.extern)
        assertEquals("https://musikzug-roessing.de/jahreskonzert", konzert.url)
    }

    @Test
    fun `An die Website geht kein Zugangstoken`() = runTest {
        server.enqueue(MockResponse().setBody(feed))

        repo().kommende()

        val anfrage = server.takeRequest()
        assertEquals("/events.json", anfrage.path)
        // Die Website ist öffentlich und hat mit unserer Anmeldung nichts zu
        // tun. Ein Token dorthin wäre eine unnötige Preisgabe.
        assertNull(anfrage.getHeader("Authorization"))
    }

    @Test
    fun `Eine kaputte Antwort wirft, statt heimlich nichts zu zeigen`() = runTest {
        server.enqueue(MockResponse().setResponseCode(500))

        val fehler = runCatching { repo().kommende() }.exceptionOrNull()

        // Das ViewModel macht daraus den Hinweis „gerade nicht erreichbar" —
        // still verschlucken wäre das Schlimmste.
        assertTrue(fehler != null)
    }

    @Test
    fun `Eine Antwort, die kein JSON ist, reisst die App nicht mit`() = runTest {
        // Kommt statt der Datei eine Fehlerseite (Zwischenspeicher, Portal,
        // umgezogene Adresse), darf das nicht abstürzen.
        server.enqueue(
            MockResponse()
                .setHeader("Content-Type", "text/html")
                .setBody("<!doctype html><title>Nicht gefunden</title>"),
        )

        val fehler = runCatching { repo().kommende() }.exceptionOrNull()

        assertTrue(fehler != null)
    }

    @Test
    fun `Ein leerer Kalender ist kein Fehler`() = runTest {
        server.enqueue(
            MockResponse().setBody("""{"version":1,"generatedAt":"","events":[]}"""),
        )

        assertTrue(repo().kommende().isEmpty())
    }
}
