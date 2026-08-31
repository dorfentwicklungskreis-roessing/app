package de.roessing.app

import de.roessing.app.auth.RentalSignIn
import de.roessing.app.auth.TokenResult
import de.roessing.app.data.BookingRequest
import de.roessing.app.data.BookingStatus
import de.roessing.app.data.LenderStatus
import de.roessing.app.data.MietenApi
import de.roessing.app.data.MietenRentalRepository
import de.roessing.app.data.OccupancyStatus
import de.roessing.app.data.RentalApiException
import de.roessing.app.data.RentalErrorCode
import de.roessing.app.data.RentalPeriod
import kotlinx.coroutines.test.runTest
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.io.File
import java.time.LocalDate

/**
 * Der Vertrag mit der Mietplattform: So sehen ihre Antworten aus, und so
 * werden sie gelesen. Die Beispiele stammen Wort für Wort aus
 * `docs/mieten-api.md` und liegen unter `android/e2e/fixtures/mieten/` —
 * ändert sich die Form drüben, muss es hier auffallen und nicht erst als
 * leere Liste auf dem Telefon.
 *
 * Der Server ist ein MockWebServer im selben Prozess. Kein Test dieser Datei
 * fasst einen entfernten Dienst an.
 */
class RentalApiTest {
    private lateinit var server: MockWebServer

    @Before
    fun vorher() {
        server = MockWebServer()
        server.start()
    }

    @After
    fun nachher() = server.shutdown()

    /** Die Beispieldatei zu einer Route. */
    private fun beispiel(name: String): String {
        val kandidaten = listOf(
            File("../e2e/fixtures/mieten/$name"),
            File("android/e2e/fixtures/mieten/$name"),
        )
        val datei = kandidaten.firstOrNull { it.isFile }
            ?: error("Beispieldatei nicht gefunden: $name (${File(".").absolutePath})")
        return datei.readText()
    }

    private fun antwort(name: String, code: Int = 200) = MockResponse()
        .setResponseCode(code)
        .setHeader("Content-Type", "application/json; charset=utf-8")
        .setBody(beispiel(name))

    private fun repo(
        signIn: RentalSignIn = RentalSignIn.VALID,
        token: TokenResult = TokenResult.Token("wert"),
    ) = MietenRentalRepository(
        MietenApi.create(server.url("/").toString()) { token },
        signIn = { signIn },
    )

    @Test
    fun `die Geraeteliste kommt in ihrer Huelle und ohne Eigentuemer`() = runTest {
        server.enqueue(antwort("items.json"))

        val geraete = repo().devices()

        assertEquals(3, geraete.size)
        val maeher = geraete.first()
        assertEquals("as-585-km-kreiselmaeher", maeher.id)
        assertEquals("AS 585 KM Kreiselmäher", maeher.name)
        assertEquals(25.0, maeher.pricePerDay!!, 0.001)
        assertEquals(40.0, maeher.pricePerWeekend!!, 0.001)
        assertEquals(120.0, maeher.pricePerWeek!!, 0.001)
        assertEquals(100.0, maeher.deposit!!, 0.001)
        assertEquals(listOf("garten", "motorgeraet"), maeher.tags)

        // Fehlende Tarife bleiben fehlend — sie sind nicht null-gleich-null.
        val walze = geraete[1]
        assertNull(walze.pricePerWeekend)
        assertNull(walze.deposit)
        assertNull(walze.description)
        assertNull(walze.thumbnailUrl)

        assertEquals("/api/v1/items", server.takeRequest().path)
    }

    @Test
    fun `die oeffentlichen Routen schicken kein Token mit`() = runTest {
        server.enqueue(antwort("items.json"))

        repo(token = TokenResult.Token("streng-geheim")).devices()

        // Wer nur schaut, gibt nichts von sich preis — und der Bereich
        // funktioniert vor jeder Anmeldung.
        assertNull(server.takeRequest().getHeader("Authorization"))
    }

    @Test
    fun `das Geraet im Detail bringt seine Bilder mit`() = runTest {
        server.enqueue(antwort("item-detail.json"))

        val detail = repo().device("as-585-km-kreiselmaeher")

        assertEquals("AS 585 KM Kreiselmäher", detail.device.name)
        assertEquals(2, detail.images.size)
        assertTrue(detail.images.first().isThumbnail)
        assertEquals("/api/v1/items/as-585-km-kreiselmaeher", server.takeRequest().path)
    }

    @Test
    fun `die Suche fragt den Server und sortiert nicht um`() = runTest {
        server.enqueue(antwort("search.json"))

        val treffer = repo().search("rasen")

        // Nach Passung, nicht nach Namen — die Reihenfolge ist die des Servers.
        assertEquals(
            listOf("as-585-km-kreiselmaeher", "018f2c1a-7b3d-4e91-9a0c-2f5d8e6b4a11"),
            treffer.map { it.id },
        )
        val pfad = server.takeRequest().path!!
        assertTrue(pfad, pfad.startsWith("/api/v1/search?"))
        assertTrue(pfad, pfad.contains("q=rasen"))
        assertTrue(pfad, pfad.contains("limit=20"))
    }

    @Test
    fun `die Verfuegbarkeit beantwortet der Server, nicht die App`() = runTest {
        server.enqueue(antwort("availability-taken.json"))

        val zeitraum = RentalPeriod(LocalDate.parse("2026-09-05"), LocalDate.parse("2026-09-07"))
        val antwort = repo().availability("as-585-km-kreiselmaeher", zeitraum)

        assertEquals(false, antwort.available)
        // „occupied" ist eine Kennung, kein Satz für die Anzeige.
        assertEquals("occupied", antwort.reason)

        val pfad = server.takeRequest().path!!
        assertTrue(pfad, pfad.contains("deviceId=as-585-km-kreiselmaeher"))
        assertTrue(pfad, pfad.contains("startDate=2026-09-05"))
        // Der Rückgabetag geht so hinaus, wie der Vertrag ihn versteht.
        assertTrue(pfad, pfad.contains("endDate=2026-09-07"))
    }

    @Test
    fun `belegt ist belegt, gleich aus welchem Grund`() = runTest {
        server.enqueue(antwort("occupancy.json"))

        val belegt = repo().occupancy("as-585-km-kreiselmaeher")

        assertEquals(
            listOf(OccupancyStatus.APPROVED, OccupancyStatus.PENDING, OccupancyStatus.BLOCKED),
            belegt.map { it.status },
        )
        // Der erste Eintrag belegt den 5. und den 6. — nicht den 7.
        assertEquals(LocalDate.parse("2026-09-06"), belegt.first().period.lastDay)
    }

    @Test
    fun `die angemeldeten Routen tragen das Token`() = runTest {
        server.enqueue(antwort("my-bookings.json"))

        val buchungen = repo(token = TokenResult.Token("abc123")).myBookings()

        assertEquals(3, buchungen.size)
        assertEquals("Bearer abc123", server.takeRequest().getHeader("Authorization"))

        val bestaetigt = buchungen.first()
        assertEquals(BookingStatus.APPROVED, bestaetigt.status)
        assertTrue(bestaetigt.canCancel)
        // Die Abholadresse steht erst nach der Zusage da.
        assertEquals("Hauptstraße 1, 31171 Nordstemmen", bestaetigt.pickup?.address)
        assertNull(buchungen[1].pickup)
        assertEquals(BookingStatus.REJECTED, buchungen[2].status)
        assertEquals(false, buchungen[2].canCancel)
    }

    @Test
    fun `das Profil sagt selbst, was ihm fehlt`() = runTest {
        server.enqueue(antwort("me.json"))

        val profil = repo().profile()

        assertEquals("Erika Musterfrau", profil.name)
        assertEquals(LenderStatus.NONE, profil.lenderStatus)
        assertTrue(profil.profileComplete)
        assertTrue(profil.missingFields.isEmpty())
    }

    /**
     * Der Normalfall der Buchung: Name und Telefon nimmt der Server aus dem
     * Profil. Wer sie trotzdem mitschickt, macht aus einer Auslassung eine
     * zweite Pflegestelle.
     */
    @Test
    fun `eine Buchung laesst die Personenfelder weg`() = runTest {
        server.enqueue(antwort("booking-created.json", code = 201))

        val buchung = repo().book(
            BookingRequest(
                deviceId = "as-585-km-kreiselmaeher",
                period = RentalPeriod(
                    LocalDate.parse("2026-09-05"),
                    LocalDate.parse("2026-09-07"),
                ),
                notes = "Hole ich Samstag früh ab.",
            ),
        )

        assertEquals(BookingStatus.PENDING, buchung.status)
        val koerper = server.takeRequest().body.readUtf8()
        assertTrue(koerper, koerper.contains("\"deviceId\":\"as-585-km-kreiselmaeher\""))
        assertTrue(koerper, koerper.contains("\"startDate\":\"2026-09-05\""))
        assertTrue(koerper, koerper.contains("\"endDate\":\"2026-09-07\""))
        assertFalse(koerper, koerper.contains("firstName"))
        assertFalse(koerper, koerper.contains("phone"))
    }

    @Test
    fun `ein belegter Zeitraum kommt als Kennung, nicht als Absturz`() = runTest {
        server.enqueue(antwort("error-occupied.json", code = 409))

        val fehler = fehlerVon {
            repo().book(
                BookingRequest(
                    deviceId = "as-585-km-kreiselmaeher",
                    period = RentalPeriod(
                        LocalDate.parse("2026-09-05"),
                        LocalDate.parse("2026-09-07"),
                    ),
                ),
            )
        }

        assertEquals(RentalErrorCode.OCCUPIED, fehler.code)
        // Die Meldung ist deutsch und darf so gezeigt werden.
        assertEquals("In diesem Zeitraum ist das Gerät schon vergeben", fehler.message)
    }

    @Test
    fun `ein unvollstaendiges Profil sagt, welche Felder fehlen`() = runTest {
        server.enqueue(antwort("error-profile-incomplete.json", code = 400))

        val fehler = fehlerVon {
            repo().book(
                BookingRequest(
                    deviceId = "as-585-km-kreiselmaeher",
                    period = RentalPeriod(
                        LocalDate.parse("2026-09-05"),
                        LocalDate.parse("2026-09-07"),
                    ),
                ),
            )
        }

        assertEquals(RentalErrorCode.PROFILE_INCOMPLETE, fehler.code)
        assertEquals(
            listOf("phone", "addressStreet", "addressZip", "addressCity"),
            fehler.missingFields,
        )
    }

    /**
     * Der Stolperstein dieses Projekts: Ein Gerät, das schon angemeldet war,
     * behält seinen Token-Satz über die Aktualisierung hinweg. Die
     * Mietplattform sagt dazu `token_audience` — und das heißt „neu
     * anmelden", nicht „abgemeldet".
     */
    @Test
    fun `ein Token ohne Empfaenger wird als solches erkannt`() = runTest {
        server.enqueue(antwort("error-token-audience.json", code = 401))

        val fehler = fehlerVon { repo().myBookings() }

        assertEquals(RentalErrorCode.TOKEN_AUDIENCE, fehler.code)
    }

    /**
     * Steht schon vor dem Absenden fest, dass das Token drüben nicht gilt,
     * geht die Anfrage gar nicht erst hinaus.
     */
    @Test
    fun `ein veraltetes Token kostet keinen vergeblichen Abruf`() = runTest {
        val fehler = fehlerVon { repo(signIn = RentalSignIn.STALE).myBookings() }

        assertEquals(RentalErrorCode.TOKEN_AUDIENCE, fehler.code)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun `ohne Anmeldung geht keine persoenliche Anfrage hinaus`() = runTest {
        val fehler = fehlerVon { repo(signIn = RentalSignIn.MISSING).myBookings() }

        assertEquals(RentalErrorCode.UNAUTHORIZED, fehler.code)
        assertEquals(0, server.requestCount)
    }

    /**
     * Nicht fragen zu können ist kein Nein: Läuft die Erneuerung gerade ins
     * Leere, geht die Anfrage nicht ohne Kopfzeile hinaus — sie käme als 401
     * zurück, und die App hielte eine gültige Anmeldung für beendet.
     */
    @Test
    fun `ohne erneuerbares Token geht die Anfrage nicht nackt hinaus`() {
        val anfrage = Request.Builder()
            .url("https://mieten.example.invalid/api/v1/me")
            .header(de.roessing.app.data.AUTH_MARKER, "required")
            .build()

        org.junit.Assert.assertThrows(java.io.IOException::class.java) {
            MietenApi.authorized(anfrage, TokenResult.Unreachable)
        }
    }

    @Test
    fun `eine unmarkierte Anfrage bleibt unangetastet`() {
        val anfrage = Request.Builder()
            .url("https://mieten.example.invalid/api/v1/items")
            .build()

        val hinaus = MietenApi.authorized(anfrage, TokenResult.Token("abc"))

        assertNull(hinaus.header("Authorization"))
    }

    /**
     * Der erwartete Fehler der Mietplattform. Ohne Umweg über runBlocking:
     * ein zweiter Ablaufplan mitten in einem Test ist eine Quelle von
     * Hängern, die mit der Sache nichts zu tun haben.
     */
    private suspend fun fehlerVon(block: suspend () -> Unit): RentalApiException = try {
        block()
        throw AssertionError("Es kam kein Fehler der Mietplattform zurück.")
    } catch (erwartet: RentalApiException) {
        erwartet
    }
}
