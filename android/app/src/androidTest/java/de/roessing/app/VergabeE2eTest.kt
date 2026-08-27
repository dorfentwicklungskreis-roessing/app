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
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import de.roessing.app.ui.theme.DorfAppTheme

/**
 * Die Vergabe gegen ein echtes Backend (AUTH_MODE=insecure-dev): eintragen,
 * gefragt werden, zusagen — einmal über die Schnittstelle und einmal durch
 * die Oberfläche der App.
 *
 * Der Test wartet auf nichts. Statt zu hoffen, dass der Hintergrund-Takt
 * vorbeikommt, stellt er die Uhr des Backends auf den Tag, an dem die
 * Aufgabe fällig ist, und stößt genau einen Vergabe-Durchlauf an (siehe
 * [DevBackend]). Danach liegt die Anfrage vor — oder sie liegt nicht vor,
 * und dann ist wirklich etwas kaputt. Vorher stand hier eine Schleife, die
 * bis zu 150 Sekunden schlief; grün oder rot entschied damit, wie beschäftigt
 * der Emulator gerade war.
 *
 * Läuft nur mit `-e e2e true`; ohne laufendes Backend hat der Test nichts zu
 * prüfen.
 */
@RunWith(AndroidJUnit4::class)
class VergabeE2eTest {
    @get:Rule
    val compose = createAndroidComposeRule<ComponentActivity>()

    private val dev = DevBackend()

    @Before
    fun nurImE2eModus() {
        assumeTrue(
            "E2E-Modus nicht aktiv (Instrumentation-Arg e2e fehlt)",
            InstrumentationRegistry.getArguments().getString("e2e") == "true",
        )
    }

    /**
     * Die Uhr gehört dem ganzen Backend — wer sie verstellt, stellt sie
     * zurück. Im @After, damit das auch nach einem gescheiterten Test
     * passiert und der nächste nicht in einem Dorf in der Zukunft aufwacht.
     */
    @After
    fun uhrZurueck() {
        if (InstrumentationRegistry.getArguments().getString("e2e") == "true") {
            dev.resetClock()
        }
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
     * Ein eigener Ort mit Gieß-Aufgabe. Eigener deshalb, weil die Vergabe an
     * ihm losläuft, sobald er fällig wird, und die Tests sich sonst
     * gegenseitig die Aufgaben wegnehmen.
     *
     * Fällig ist die Aufgabe hier noch nicht: Eine frisch angelegte rechnet ab
     * ihrer Anlage und ist zunächst grün. Fällig wird sie später durch die
     * Zeitreise ([makeDueAndAssign]) — nicht mehr, wie früher, durch eine zehn
     * Tage zurückdatierte Erledigung. Rückwirkend melden darf inzwischen nur
     * noch die Verwaltung und nur drei Tage weit (siehe model.MaxBackdate);
     * die Ausgangslage eines Tests über eine Ausnahme für Verwaltende zu
     * bauen, wäre ohnehin der falsche Weg gewesen.
     */
    private fun eigenerOrtMitAufgabe(name: String): Pair<Long, Long> {
        val ort = post(
            "/api/v1/places",
            """{"name":"$name ${System.currentTimeMillis()}","kind":"blumenkasten","lat":52.2105,"lon":9.8695}""",
        )
        val aufgabe = post(
            "/api/v1/places/${ort.getLong("id")}/tasks",
            """{"kind":"giessen","liters":5,"intervalDays":7,"redAfterDays":14}""",
        )
        return ort.getLong("id") to aufgabe.getLong("id")
    }

    /**
     * Macht die Aufgabe fällig und lässt die Vergabe genau einmal laufen.
     *
     * Zehn Tage weiter als das Soll-Intervall von sieben Tagen — die Aufgabe
     * ist dann überfällig, die Ampel steht auf Gelb. Der Durchlauf kehrt erst
     * zurück, wenn die Benachrichtigungen geschrieben sind; danach ist nichts
     * mehr abzuwarten.
     *
     * Muss NACH dem Eintragen kommen: Die Vergabe fragt nur, wer schon
     * eingetragen ist.
     */
    private fun makeDueAndAssign() {
        dev.travelForward(days = 10)
        dev.runAssignment()
    }

    /** Die Anfrage zu dieser Aufgabe — sie liegt jetzt vor oder gar nicht. */
    private fun requestFor(token: String, taskId: Long): NotificationDto =
        requestOrNull(token, taskId)
            ?: error("Nach dem Vergabe-Durchlauf liegt keine Anfrage für Aufgabe $taskId vor")

    private fun requestOrNull(token: String, taskId: Long): NotificationDto? {
        val vergabe = ApiVergabeRepository(api(token))
        return runBlocking { vergabe.notifications() }
            .firstOrNull { it.taskId == taskId && it.istAnfrage }
    }

    // --- Über die Schnittstelle ------------------------------------------------

    @Test
    fun eintragenAnfrageZusageUndRueckgabe() = runBlocking {
        val (placeId, taskId) = eigenerOrtMitAufgabe("Vergabe-E2E")
        val anna = ApiVergabeRepository(api(annaToken))
        val orte = ApiPlacesRepository(api(annaToken))

        anna.signup(placeId, "giessen")
        val meine = orte.places().places.first { it.id == placeId }.tasks.first()
        assertTrue("signedUp fehlt nach dem Eintragen", meine.signedUp)
        assertEquals(1, meine.signupCount)

        // Solange nichts fällig ist, wird auch niemand gefragt. Bewusst nur
        // auf DIESE Aufgabe geschaut: Was andere Tests im selben Backend
        // hinterlassen haben, geht diesen Test nichts an.
        dev.runAssignment()
        assertNull("Es wurde gefragt, obwohl die Aufgabe noch grün ist", requestOrNull(annaToken, taskId))

        makeDueAndAssign()
        val anfrage = requestFor(annaToken, taskId)
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
     * Der Vergabe-Vorgang der Aufgabe. Wer zuerst gefragt wird, entscheidet
     * die faire Reihenfolge — für diesen Test ist nur wichtig, dass der
     * Vorgang läuft. Zusagen darf ohnehin jede:r Eingetragene.
     */
    private fun assignmentFor(taskId: Long): Long {
        val orte = ApiPlacesRepository(api(annaToken))
        return runBlocking { orte.places() }.places
            .flatMap { it.tasks }.firstOrNull { it.id == taskId }?.assignment?.id
            ?: error("Nach dem Vergabe-Durchlauf läuft kein Vorgang für Aufgabe $taskId")
    }

    // Zwei Leute, ein Vorgang: Der zweite bekommt 409 mit deutschem Text.
    @Test
    fun wennJemandAnderesSchnellerWar() = runBlocking {
        val (placeId, taskId) = eigenerOrtMitAufgabe("Wettlauf-E2E")
        val anna = ApiVergabeRepository(api(annaToken))
        val bernd = ApiVergabeRepository(api(berndToken))
        anna.signup(placeId, null)
        bernd.signup(placeId, null)

        makeDueAndAssign()
        val vorgangId = assignmentFor(taskId)
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

    /**
     * Bildschirmfoto. Der kurze Schlaf bleibt bewusst stehen: Er wartet keine
     * Fachlichkeit ab, sondern die Übergangs-Animation und das Zeichnen —
     * `waitForIdle` ist damit fertig, bevor das Bild fertig ist. Ein Foto,
     * das eine halb eingeblendete Seite zeigt, ist kein Nachweis.
     */
    private fun foto(name: String, wartenMs: Long = 800) {
        compose.waitForIdle()
        SystemClock.sleep(wartenMs)
        UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
            .takeScreenshot(File(ordner, "$name.png"))
    }

    @Test
    fun durchDieApp() {
        val (placeId, taskId) = eigenerOrtMitAufgabe("App-E2E")
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
        // einer langen Liste ist er zunächst gar nicht gebaut). Der Schlaf
        // wartet auf das Füllen und Zeichnen der Liste, nicht auf das
        // Backend — die Karte trägt ihren testTag erst, wenn sie gebaut ist,
        // und vorher lässt sich auf nichts warten.
        compose.waitUntil(20_000) { compose.onAllNodesWithTagSicher("place-list") }
        SystemClock.sleep(1_500)
        compose.onNodeWithTag("place-list").performScrollToNode(hasTestTag("place-card-$placeId"))
        compose.onNodeWithTag("place-card-$placeId").performClick()
        foto("e2e-02-ortsdetail")

        compose.onNodeWithTag("signup-switch").performScrollTo().performClick()
        compose.waitForIdle()
        foto("e2e-03-eingetragen")

        // 2) Die Aufgabe fällig machen und einmal vergeben lassen. Danach
        //    liegt die Anfrage im Backend — abzuwarten ist nichts.
        makeDueAndAssign()
        val anfrage = requestFor(annaToken, taskId)

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
        // Der Knopf oben holt den Stand frisch. Die Anfrage steht im Backend
        // fest, gewartet wird also nur noch auf den Netzweg und das Zeichnen
        // — und zwar so lange, wie es dauert, statt fester Sekunden. Notfalls
        // noch einmal tippen: Ein Tipp kann ins Leere gehen, wenn die Liste
        // gerade neu aufgebaut wird.
        var sichtbar = false
        for (versuch in 1..6) {
            compose.onNodeWithTag("refresh").performClick()
            runCatching {
                compose.waitUntil(5_000) { compose.onAllNodesWithTagSicher("claim-${anfrage.assignmentId}") }
            }
            if (compose.onAllNodesWithTagSicher("claim-${anfrage.assignmentId}")) {
                sichtbar = true
                break
            }
        }
        assertTrue("Die Anfrage steht nicht auf der Startseite", sichtbar)
        foto("e2e-04-anfrage")

        // 4) Zusagen — im Ortsdetail, wo der Knopf eindeutig zu dieser Aufgabe
        //    gehört (auf der Startseite können mehrere Anfragen stehen).
        compose.onNodeWithTag("bereich-mithelfen").performScrollTo().performClick()
        compose.onNodeWithTag("tab-list").performClick()
        // Wie oben: Zeit fürs Bauen der Liste, nicht fürs Backend.
        SystemClock.sleep(1_000)
        compose.onNodeWithTag("place-list").performScrollToNode(hasTestTag("place-card-$placeId"))
        compose.onNodeWithTag("place-card-$placeId").performClick()
        compose.waitUntil(60_000) { compose.onAllNodesWithTagSicher("claim-task-$taskId") }
        compose.onNodeWithTag("claim-task-$taskId").performScrollTo().performClick()
        compose.waitForIdle()
        // Die Zusage geht übers Netz und die Seite baut sich neu — das Foto
        // soll den Zustand danach zeigen.
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
        val claimedBy = runBlocking { ApiPlacesRepository(api).places() }
            .places.first { it.id == placeId }.tasks.first().assignment?.claimedBy.orEmpty()
        assertEquals("anna-e2e", claimedBy)
    }
}

/** Hilfe für waitUntil: gibt es den Knoten schon? */
private fun androidx.compose.ui.test.junit4.ComposeTestRule.onAllNodesWithTagSicher(tag: String): Boolean =
    onAllNodes(hasTestTag(tag)).fetchSemanticsNodes().isNotEmpty()
