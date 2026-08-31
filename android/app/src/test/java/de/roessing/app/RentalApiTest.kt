package de.roessing.app

import de.roessing.app.auth.RentalSignIn
import de.roessing.app.auth.TokenResult
import de.roessing.app.data.BlockRequest
import de.roessing.app.data.BookingRequest
import de.roessing.app.data.BookingStatus
import de.roessing.app.data.LenderStatus
import de.roessing.app.data.MietenApi
import de.roessing.app.data.MietenRentalRepository
import de.roessing.app.data.OccupancyStatus
import de.roessing.app.data.ProfilePatch
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

    // --- Sets (Route 4) ------------------------------------------------------

    /**
     * Sets stehen im Vertrag als eigene Hülle. Sie werden angezeigt und in der
     * App nicht gebucht — Zusagen und Stornieren kann der Server dafür noch
     * nicht, und was er nicht kann, baut die App nicht nach.
     */
    @Test
    fun `die Sets kommen ohne Anmeldung und mit ihren Geraeten`() = runTest {
        server.enqueue(antwort("sets.json"))

        val sets = repo(token = TokenResult.Token("streng-geheim")).sets()

        assertEquals(listOf("gartenset", "malerset"), sets.map { it.id })
        assertEquals("Gartenset", sets.first().name)
        assertEquals(30.0, sets.first().pricePerDay!!, 0.001)
        assertEquals(3, sets.first().itemIds.size)
        // Kein Preis für ein Wochenende, keine Kaution — das bleibt leer.
        assertNull(sets[1].description)
        assertNull(sets[1].deposit)

        val anfrage = server.takeRequest()
        assertEquals("/api/v1/sets", anfrage.path)
        assertNull(anfrage.getHeader("Authorization"))
    }

    // --- Profil (Routen 8 und 9) ---------------------------------------------

    /**
     * Der Server ändert genau, was er bekommt. Ein leeres Feld ist deshalb
     * kein Weg, einen Wert zu löschen — es bleibt weg.
     */
    @Test
    fun `das Profil aendert nur, was mitgeschickt wird`() = runTest {
        server.enqueue(antwort("me-updated.json"))

        val profil = repo().updateProfile(ProfilePatch(phone = "+49 5069 987654", addressCity = "  "))

        assertEquals("+49 5069 987654", profil.phone)
        val anfrage = server.takeRequest()
        assertEquals("PATCH", anfrage.method)
        assertEquals("/api/v1/me", anfrage.path)
        val koerper = anfrage.body.readUtf8()
        assertTrue(koerper, koerper.contains("\"phone\":\"+49 5069 987654\""))
        assertFalse(koerper, koerper.contains("addressCity"))
        // Die E-Mail-Adresse kommt aus der Rössing-ID und geht nie hinaus.
        assertFalse(koerper, koerper.contains("email"))
    }

    @Test
    fun `die Anfrage als Verleiher ist eine Eingangsbestaetigung`() = runTest {
        server.enqueue(antwort("lender-request.json"))

        val antwort = repo().requestLender()

        assertEquals(LenderStatus.PENDING, antwort.lenderStatus)
        assertTrue(antwort.message!!.startsWith("Deine Anfrage wurde weitergeleitet"))
        val anfrage = server.takeRequest()
        assertEquals("POST", anfrage.method)
        assertEquals("/api/v1/me/lender-request", anfrage.path)
    }

    /** Zu schnell hintereinander gefragt: eine Auskunft, kein Absturz. */
    @Test
    fun `eine zu schnelle Verleiher-Anfrage kommt als Kennung zurueck`() = runTest {
        server.enqueue(
            MockResponse()
                .setResponseCode(429)
                .setHeader("Content-Type", "application/json; charset=utf-8")
                .setBody("""{"error":{"code":"rate_limited","message":"Bitte später noch einmal"}}"""),
        )

        val fehler = fehlerVon { repo().requestLender() }

        assertEquals(RentalErrorCode.RATE_LIMITED, fehler.code)
    }

    @Test
    fun `ein freigeschaltetes Profil sagt es selbst`() = runTest {
        server.enqueue(antwort("me-lender.json"))

        val profil = repo().profile()

        // Daran — und an nichts anderem — hängt die Vermieteransicht.
        assertEquals(LenderStatus.APPROVED, profil.lenderStatus)
        assertTrue(profil.lender)
    }

    // --- Die Vermieterseite (Routen 13 bis 19) -------------------------------

    /**
     * Name und Nummer des Mieters stehen hier, weil die Übergabe verabredet
     * werden muss. Sie stehen in keiner anderen Antwort des Vertrags.
     */
    @Test
    fun `die Anfragen auf meinen Geraeten tragen Name und Nummer`() = runTest {
        server.enqueue(antwort("owner-bookings.json"))

        val anfragen = repo(token = TokenResult.Token("abc123")).ownerBookings()

        assertEquals(2, anfragen.size)
        val offen = anfragen.first()
        assertEquals("Erika Musterfrau", offen.renterName)
        assertEquals("+49 5069 123456", offen.renterPhone)
        assertEquals(BookingStatus.PENDING, offen.status)
        // Ob entschieden werden kann, sagt der Server — nicht die App.
        assertTrue(offen.canDecide)
        assertFalse(anfragen[1].canDecide)
        assertTrue(anfragen[1].canCancel)
        assertNull(anfragen[1].renterPhone)

        val anfrage = server.takeRequest()
        assertEquals("/api/v1/owner/bookings", anfrage.path)
        assertEquals("Bearer abc123", anfrage.getHeader("Authorization"))
    }

    /** Wer keine Geräte hat, bekommt eine leere Liste — kein Fehler. */
    @Test
    fun `wer nicht freigeschaltet ist, bekommt eine Kennung dafuer`() = runTest {
        server.enqueue(antwort("error-not-a-lender.json", code = 403))

        val fehler = fehlerVon { repo().ownerBookings() }

        assertEquals(RentalErrorCode.NOT_A_LENDER, fehler.code)
    }

    @Test
    fun `zusagen und absagen gehen an die Buchung`() = runTest {
        server.enqueue(antwort("status-approved.json"))
        server.enqueue(
            MockResponse()
                .setResponseCode(200)
                .setHeader("Content-Type", "application/json; charset=utf-8")
                .setBody("""{"status":"rejected"}"""),
        )

        repo().approve("8f14c2b0")
        repo().reject("1c9e77a3")

        val zusage = server.takeRequest()
        assertEquals("POST", zusage.method)
        assertEquals("/api/v1/bookings/8f14c2b0/approve", zusage.path)
        assertEquals("/api/v1/bookings/1c9e77a3/reject", server.takeRequest().path)
    }

    /**
     * Eine Buchung, die nicht mehr offen ist, ist kein Absturz: Der Server
     * sagt `conflict`, und die Ansicht holt danach seine Liste.
     */
    @Test
    fun `eine schon entschiedene Buchung meldet conflict`() = runTest {
        server.enqueue(antwort("error-conflict.json", code = 409))

        val fehler = fehlerVon { repo().approve("8f14c2b0") }

        assertEquals(RentalErrorCode.CONFLICT, fehler.code)
    }

    /** Stornieren liefert einen Körper — der wird gelesen und nicht geraten. */
    @Test
    fun `das Stornieren liest die Antwort des Servers`() = runTest {
        server.enqueue(antwort("status-cancelled.json"))

        repo().cancel("8f14c2b0")

        assertEquals("/api/v1/bookings/8f14c2b0/cancel", server.takeRequest().path)
    }

    @Test
    fun `meine Geraete stehen mit den abgeschalteten da`() = runTest {
        server.enqueue(antwort("owner-items.json"))

        val meine = repo().ownerDevices()

        assertEquals(2, meine.size)
        assertTrue(meine.first().active)
        // Das eine Feld, das die öffentliche Liste nicht hat.
        assertFalse(meine[1].active)
        assertEquals("Rasenwalze", meine[1].device.name)
        assertEquals("/api/v1/owner/items", server.takeRequest().path)
    }

    @Test
    fun `eine Sperre wird angelegt und wieder aufgehoben`() = runTest {
        server.enqueue(antwort("block-created.json", code = 201))
        server.enqueue(antwort("block-deleted.json"))

        val sperre = repo().addBlock(
            BlockRequest(
                deviceId = "as-585-km-kreiselmaeher",
                period = RentalPeriod(
                    LocalDate.parse("2026-10-01"),
                    LocalDate.parse("2026-10-08"),
                ),
                reason = "Eigener Einsatz",
            ),
        )

        assertEquals("b1f0c9a2-33aa-4d10-8e77-5c2b1a9f0e33", sperre.id)
        // Der 7. ist der letzte Tag, der 8. der Rückgabetag.
        assertEquals(LocalDate.parse("2026-10-07"), sperre.period.lastDay)

        val anlegen = server.takeRequest()
        assertEquals("/api/v1/owner/blocks", anlegen.path)
        val koerper = anlegen.body.readUtf8()
        assertTrue(koerper, koerper.contains("\"startDate\":\"2026-10-01\""))
        assertTrue(koerper, koerper.contains("\"endDate\":\"2026-10-08\""))
        assertTrue(koerper, koerper.contains("\"reason\":\"Eigener Einsatz\""))

        repo().removeBlock(sperre.id)
        val aufheben = server.takeRequest()
        assertEquals("DELETE", aufheben.method)
        assertEquals("/api/v1/owner/blocks/${sperre.id}", aufheben.path)
    }

    @Test
    fun `meine Sperren kommen mit Geraet und Grund`() = runTest {
        server.enqueue(antwort("blocks.json"))

        val sperren = repo().blocks()

        assertEquals(1, sperren.size)
        assertEquals("AS 585 KM Kreiselmäher", sperren.first().deviceName)
        assertEquals("Eigener Einsatz", sperren.first().reason)
        assertEquals("/api/v1/owner/blocks", server.takeRequest().path)
    }

    /**
     * Eine bestehende Buchung wird von einer Sperre nicht verdrängt. Der
     * Server sagt `occupied`; wer die Tage trotzdem will, storniert zuerst.
     */
    @Test
    fun `eine Sperre verdraengt keine Buchung`() = runTest {
        server.enqueue(antwort("error-block-occupied.json", code = 409))

        val fehler = fehlerVon {
            repo().addBlock(
                BlockRequest(
                    deviceId = "as-585-km-kreiselmaeher",
                    period = RentalPeriod(
                        LocalDate.parse("2026-10-01"),
                        LocalDate.parse("2026-10-08"),
                    ),
                ),
            )
        }

        assertEquals(RentalErrorCode.OCCUPIED, fehler.code)
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
