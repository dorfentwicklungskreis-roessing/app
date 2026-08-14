package de.roessing.app

import android.os.SystemClock
import androidx.activity.ComponentActivity
import androidx.compose.ui.test.hasTestTag
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performScrollToNode
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.UiDevice
import de.roessing.app.data.ApiPlacesRepository
import de.roessing.app.data.ApiIdeenRepository
import de.roessing.app.data.ApiProfileRepository
import de.roessing.app.data.ApiStatsRepository
import de.roessing.app.data.ApiVergabeRepository
import de.roessing.app.data.AssignmentTakenException
import de.roessing.app.data.DorfApi
import de.roessing.app.data.NotificationDto
import de.roessing.app.ui.HomeScreen
import de.roessing.app.ui.LeaderboardViewModel
import de.roessing.app.ui.PlacesViewModel
import de.roessing.app.ui.IdeenViewModel
import de.roessing.app.ui.ProfileViewModel
import java.io.File
import kotlinx.coroutines.runBlocking
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import de.roessing.app.ui.theme.DorfAppTheme

/**
 * Die Vergabe gegen ein echtes Backend (AUTH_MODE=insecure-dev, VERGABE_TAKT
 * kurz gestellt): eintragen, gefragt werden, zusagen — einmal über die
 * Schnittstelle und einmal durch die Oberfläche der App.
 *
 * Läuft nur mit `-e e2e true`; ohne laufendes Backend hat der Test nichts zu
 * prüfen.
 */
@RunWith(AndroidJUnit4::class)
class VergabeE2eTest {
    @get:Rule
    val compose = createAndroidComposeRule<ComponentActivity>()

    @Before
    fun nurImE2eModus() {
        assumeTrue(
            "E2E-Modus nicht aktiv (Instrumentation-Arg e2e fehlt)",
            InstrumentationRegistry.getArguments().getString("e2e") == "true",
        )
    }

    private val basis = BuildConfig.API_BASE_URL.trimEnd('/')
    private val client = OkHttpClient()
    private val json = "application/json".toMediaType()

    private fun api(token: String) = DorfApi.create(BuildConfig.API_BASE_URL) { token }

    private val annaToken = "anna-e2e:Anna E2E:admin"
    private val berndToken = "bernd-e2e:Bernd E2E:"

    private fun post(pfad: String, koerper: String, token: String = annaToken): JSONObject {
        val anfrage = Request.Builder()
            .url("$basis$pfad")
            .header("Authorization", "Bearer $token")
            .post(koerper.toRequestBody(json))
            .build()
        client.newCall(anfrage).execute().use { antwort ->
            val text = antwort.body?.string().orEmpty()
            assertTrue("POST $pfad: HTTP ${antwort.code} $text", antwort.isSuccessful)
            return if (text.isBlank()) JSONObject() else JSONObject(text)
        }
    }

    /**
     * Ein eigener Ort mit überfälliger Gieß-Aufgabe. Eigener deshalb, weil die
     * Vergabe an ihm sofort losläuft und die Tests sich sonst gegenseitig die
     * Aufgaben wegnehmen.
     *
     * Überfällig wird die Aufgabe über eine nachgetragene Erledigung von vor
     * zehn Tagen: Eine frisch angelegte Aufgabe ist zunächst grün (sie rechnet
     * ab ihrer Anlage), und die Vergabe rührt nur an, was wirklich ansteht.
     * Nachtragen darf das die Verwaltung — der E2E-Nutzer hat die Rolle.
     */
    private fun eigenerFaelligerOrt(name: String): Pair<Long, Long> {
        val ort = post(
            "/api/v1/places",
            """{"name":"$name ${System.currentTimeMillis()}","kind":"blumenkasten","lat":52.2105,"lon":9.8695}""",
        )
        val aufgabe = post(
            "/api/v1/places/${ort.getLong("id")}/tasks",
            """{"kind":"giessen","liters":5,"intervalDays":7,"redAfterDays":14}""",
        )
        val vorZehnTagen = java.time.Instant.now().minus(java.time.Duration.ofDays(10))
        post(
            "/api/v1/tasks/${aufgabe.getLong("id")}/completions",
            """{"liters":5,"doneAt":"${java.time.format.DateTimeFormatter.ISO_INSTANT.format(vorZehnTagen)}","force":true}""",
        )
        return ort.getLong("id") to aufgabe.getLong("id")
    }

    /**
     * Wartet, bis für die Aufgabe eine Anfrage vorliegt.
     *
     * Großzügig bemessen: Der Takt der Vergabe steht in der Vorgabe auf einer
     * Minute (`VERGABE_TAKT`), und getaktet wird erst nach der Anmeldung.
     */
    private fun warteAufAnfrage(token: String, taskId: Long, sekunden: Int = 150): NotificationDto {
        val vergabe = ApiVergabeRepository(api(token))
        repeat(sekunden * 2) {
            val treffer = runBlocking { vergabe.notifications() }
                .firstOrNull { it.taskId == taskId && it.istAnfrage }
            if (treffer != null) return treffer
            SystemClock.sleep(500)
        }
        error("Nach $sekunden s kam keine Anfrage für Aufgabe $taskId")
    }

    // --- Über die Schnittstelle ------------------------------------------------

    @Test
    fun eintragenAnfrageZusageUndRueckgabe() = runBlocking {
        val (placeId, taskId) = eigenerFaelligerOrt("Vergabe-E2E")
        val anna = ApiVergabeRepository(api(annaToken))
        val orte = ApiPlacesRepository(api(annaToken))

        anna.signup(placeId, "giessen")
        val meine = orte.places().places.first { it.id == placeId }.tasks.first()
        assertTrue("signedUp fehlt nach dem Eintragen", meine.signedUp)
        assertEquals(1, meine.signupCount)

        val anfrage = warteAufAnfrage(annaToken, taskId)
        assertTrue("Die Anfrage hat keinen Text", anfrage.title.isNotBlank() && anfrage.text.isNotBlank())
        assertNotNull("Einer Anfrage fehlt die Frist", anfrage.expiresAt)

        val vorgang = anna.claim(anfrage.assignmentId)
        assertEquals("uebernommen", vorgang.state)
        assertEquals("anna-e2e", vorgang.claimedBy)

        val nachZusage = orte.places().places.first { it.id == placeId }.tasks.first()
        assertEquals("anna-e2e", nachZusage.assignment?.claimedBy)
        assertNotNull("Die Zusage hat keine Frist", nachZusage.assignment?.claimedUntil)

        // Zurückgeben gibt den Vorgang wieder frei.
        val frei = anna.release(anfrage.assignmentId)
        assertEquals("offen", frei.state)
        assertEquals("", frei.claimedBy)
    }

    /**
     * Wartet auf den Vergabe-Vorgang der Aufgabe. Wer zuerst gefragt wird,
     * entscheidet die faire Reihenfolge — für diesen Test ist nur wichtig,
     * dass der Vorgang läuft. Zusagen darf ohnehin jede:r Eingetragene.
     */
    private fun warteAufVorgang(taskId: Long, sekunden: Int = 150): Long {
        val orte = ApiPlacesRepository(api(annaToken))
        repeat(sekunden * 2) {
            val vorgang = runBlocking { orte.places() }.places
                .flatMap { it.tasks }.firstOrNull { it.id == taskId }?.assignment
            if (vorgang != null) return vorgang.id
            SystemClock.sleep(500)
        }
        error("Nach $sekunden s lief kein Vorgang für Aufgabe $taskId")
    }

    // Zwei Leute, ein Vorgang: Der zweite bekommt 409 mit deutschem Text.
    @Test
    fun wennJemandAnderesSchnellerWar() = runBlocking {
        val (placeId, taskId) = eigenerFaelligerOrt("Wettlauf-E2E")
        val anna = ApiVergabeRepository(api(annaToken))
        val bernd = ApiVergabeRepository(api(berndToken))
        anna.signup(placeId, null)
        bernd.signup(placeId, null)

        val vorgangId = warteAufVorgang(taskId)
        anna.claim(vorgangId)

        var abgewiesen = false
        try {
            bernd.claim(vorgangId)
        } catch (e: AssignmentTakenException) {
            abgewiesen = true
            assertTrue("Der Grund nennt niemanden: ${e.grund}", e.grund.contains("übernommen"))
        }
        assertTrue("Der zweite Zugriff wurde angenommen", abgewiesen)
    }

    // Das Gerät meldet sich an und wieder ab — ohne diese Kennung gäbe es
    // keinen Push.
    @Test
    fun geraetAnUndAbmelden() = runBlocking {
        val geraete = de.roessing.app.data.ApiDeviceRepository(api(annaToken))
        val kennung = "e2e-testkennung-${System.currentTimeMillis()}"
        geraete.register(kennung)
        // Zweimal anmelden ist ein Auffrischen und kein Fehler.
        geraete.register(kennung)
        geraete.unregister(kennung)
    }

    // --- Durch die Oberfläche --------------------------------------------------

    private val ordner: File
        get() = File(
            InstrumentationRegistry.getInstrumentation().targetContext.filesDir,
            "push-neu",
        ).apply { mkdirs() }

    private fun foto(name: String, wartenMs: Long = 800) {
        compose.waitForIdle()
        SystemClock.sleep(wartenMs)
        UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
            .takeScreenshot(File(ordner, "$name.png"))
    }

    @Test
    fun durchDieApp() {
        val (placeId, taskId) = eigenerFaelligerOrt("App-E2E")
        val api = api(annaToken)
        compose.setContent {
            DorfAppTheme {
                HomeScreen(
                    viewModel = PlacesViewModel(ApiPlacesRepository(api), ApiVergabeRepository(api)),
                    leaderboardViewModel = LeaderboardViewModel(ApiStatsRepository(api)),
                    profileViewModel = ProfileViewModel(ApiProfileRepository(api)),
                    ideenViewModel = IdeenViewModel(ApiIdeenRepository(api)),
                    onLogout = {},
                )
            }
        }
        foto("e2e-01-startseite")

        // 1) Ortsdetail öffnen und sich als Helfer:in eintragen.
        compose.onNodeWithTag("bereich-mithelfen").performScrollTo().performClick()
        compose.onNodeWithTag("tab-list").performClick()
        // Die Liste lädt aus dem Netz; danach zum eigenen Ort scrollen (in
        // einer langen Liste ist er zunächst gar nicht gebaut).
        compose.waitUntil(20_000) { compose.onAllNodesWithTagSicher("place-list") }
        SystemClock.sleep(1_500)
        compose.onNodeWithTag("place-list").performScrollToNode(hasTestTag("place-card-$placeId"))
        compose.onNodeWithTag("place-card-$placeId").performClick()
        foto("e2e-02-ortsdetail")

        compose.onNodeWithTag("signup-switch").performScrollTo().performClick()
        compose.waitForIdle()
        foto("e2e-03-eingetragen")

        // 2) Auf die Anfrage warten — der Takt der Vergabe erledigt das.
        val anfrage = warteAufAnfrage(annaToken, taskId)

        // 3) Zurück auf die Startseite: dort steht die Anfrage.
        //    Das Blatt schließt die Zurück-Taste, den Bereich der Pfeil oben.
        UiDevice.getInstance(InstrumentationRegistry.getInstrumentation()).pressBack()
        compose.waitForIdle()
        // Nach dem Eintragen fragt die App nach der Erlaubnis für
        // Benachrichtigungen. Hier interessiert der Abrufweg — „Später".
        if (compose.onAllNodesWithTagSicher("push-spaeter")) {
            compose.onNodeWithTag("push-spaeter").performClick()
            compose.waitForIdle()
        }
        compose.onNodeWithTag("zurueck").performClick()
        compose.waitForIdle()
        // Der Knopf oben holt den Stand frisch — die Anfrage kam gerade eben.
        // Notfalls mehrmals: Zwischen Abholen und Anzeigen liegt ein Netzweg.
        var sichtbar = false
        repeat(12) {
            compose.onNodeWithTag("refresh").performClick()
            compose.waitForIdle()
            SystemClock.sleep(2_000)
            if (compose.onAllNodesWithTagSicher("claim-${anfrage.assignmentId}")) {
                sichtbar = true
                return@repeat
            }
        }
        assertTrue("Die Anfrage steht nicht auf der Startseite", sichtbar)
        foto("e2e-04-anfrage")

        // 4) Zusagen — im Ortsdetail, wo der Knopf eindeutig zu dieser Aufgabe
        //    gehört (auf der Startseite können mehrere Anfragen stehen).
        compose.onNodeWithTag("bereich-mithelfen").performScrollTo().performClick()
        compose.onNodeWithTag("tab-list").performClick()
        SystemClock.sleep(1_000)
        compose.onNodeWithTag("place-list").performScrollToNode(hasTestTag("place-card-$placeId"))
        compose.onNodeWithTag("place-card-$placeId").performClick()
        compose.waitUntil(60_000) { compose.onAllNodesWithTagSicher("claim-task-$taskId") }
        compose.onNodeWithTag("claim-task-$taskId").performScrollTo().performClick()
        compose.waitForIdle()
        SystemClock.sleep(2_000)
        foto("e2e-05-zugesagt")

        // 5) Der Vergabestand steht auch in der Liste.
        UiDevice.getInstance(InstrumentationRegistry.getInstrumentation()).pressBack()
        compose.waitForIdle()
        SystemClock.sleep(1_000)
        compose.onNodeWithTag("place-list").performScrollToNode(hasTestTag("place-card-$placeId"))
        // Die anklickbare Karte verschmilzt ihre Texte zu einem Knoten.
        compose.onNodeWithTag("vergabe-$placeId", useUnmergedTree = true).assertExists()
        foto("e2e-06-vergabestand")

        // Die Zusage steht auch im Backend — die Oberfläche hat sie wirklich
        // abgeschickt und nicht nur angezeigt.
        var claimedBy = ""
        repeat(20) {
            claimedBy = runBlocking { ApiPlacesRepository(api).places() }
                .places.first { it.id == placeId }.tasks.first().assignment?.claimedBy.orEmpty()
            if (claimedBy.isNotEmpty()) return@repeat
            SystemClock.sleep(500)
        }
        assertEquals("anna-e2e", claimedBy)
    }
}

/** Hilfe für waitUntil: gibt es den Knoten schon? */
private fun androidx.compose.ui.test.junit4.ComposeTestRule.onAllNodesWithTagSicher(tag: String): Boolean =
    onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()
