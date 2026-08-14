package de.roessing.app

import android.os.Build
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import de.roessing.app.data.ApiDeviceRepository
import de.roessing.app.data.DorfApi
import de.roessing.app.push.Anmeldespeicher
import de.roessing.app.push.Benachrichtigungserlaubnis
import de.roessing.app.push.Geraeteabgleich
import kotlinx.coroutines.runBlocking
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Die Gerätekennung darf nur bei erlaubten Mitteilungen beim Backend liegen —
 * hier am echten Android geprüft, mit dem echten Erlaubniszustand des Systems
 * und gegen ein echtes Backend.
 *
 * Der Nachweis nutzt einen Umstand des Backends: `POST /api/v1/me/devices`
 * antwortet mit **201**, wenn die Kennung dort neu ist, und mit **200**, wenn
 * sie schon lag (siehe internal/api/geraete.go). Danach lässt sich also
 * ablesen, ob die App die Kennung zuvor hinterlegt hat — ohne dass es dafür
 * einen Auskunfts-Endpunkt bräuchte, den es aus gutem Grund nicht gibt.
 *
 * Der Erlaubniszustand wird **nicht** aus dem Test heraus umgestellt: Er lässt
 * sich weder über `appops` zuverlässig setzen (`areNotificationsEnabled()`
 * richtet sich nicht danach) noch über `pm revoke` — das schösse den eigenen
 * Testprozess ab. Stattdessen läuft dieselbe Klasse in `ci-e2e.sh` **zweimal**:
 * einmal im Gradle-Lauf (der die APKs mit `-g` installiert, alle Rechte
 * erteilt) und einmal nach einer Installation ohne `-g`. Jeder Testfall nimmt
 * über `assumeTrue` den Durchgang, der zu ihm passt.
 */
@RunWith(AndroidJUnit4::class)
class GeraetekennungE2eTest {
    private val kontext = InstrumentationRegistry.getInstrumentation().targetContext
    private val token = "kennung-e2e:Kennung Tester:member"
    private val basis = BuildConfig.API_BASE_URL.trimEnd('/')
    private val client = OkHttpClient()
    private val json = "application/json".toMediaType()

    /** Eindeutig je Lauf, damit ein früherer Lauf das Ergebnis nicht färbt. */
    private val kennung = "e2e-kennung-${System.nanoTime()}"

    private val speicher = Speicher()
    private var kennungAbgefragt = 0

    @Before
    fun nurImE2eModus() {
        assumeTrue(
            "E2E-Modus nicht aktiv (Instrumentation-Arg e2e fehlt)",
            InstrumentationRegistry.getArguments().getString("e2e") == "true",
        )
    }

    @After
    fun aufraeumen() {
        if (InstrumentationRegistry.getArguments().getString("e2e") != "true") return
        runCatching { abmeldenAmBackend() }
    }

    // --- Testfälle -----------------------------------------------------------

    /**
     * Der eigentliche Nachweis. Läuft im Durchgang ohne erteilte Berechtigung,
     * den `ci-e2e.sh` eigens dafür anstößt.
     */
    @Test
    fun ohneErlaubnisKommtKeineKennungAmBackendAn() {
        // Im eigens dafür angestoßenen Durchgang muss dieser Fall auch
        // wirklich laufen — ein stilles Überspringen wäre kein Nachweis.
        // Vor Android 13 lässt sich die Erlaubnis nicht wegnehmen: Es gibt
        // dort keine Berechtigung POST_NOTIFICATIONS, und Mitteilungen sind
        // ab Werk an. Dort bleibt es beim Überspringen.
        val erlaubnisfrei =
            InstrumentationRegistry.getArguments().getString("erlaubnisfrei") == "true"
        if (erlaubnisfrei && Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            assertFalse(
                "Der Durchgang ohne erteilte Berechtigung meldet trotzdem eine Erlaubnis — " +
                    "dann prüft dieser Test nichts",
                Benachrichtigungserlaubnis.wirksam(kontext),
            )
        }
        assumeTrue(
            "Dieses Gerät erlaubt Mitteilungen — der Fall gehört in den " +
                "Durchgang ohne erteilte Berechtigung (siehe ci-e2e.sh)",
            !Benachrichtigungserlaubnis.wirksam(kontext),
        )

        runBlocking { abgleich().abgleichen(Benachrichtigungserlaubnis.wirksam(kontext)) }

        assertEquals(
            "Die Kennung ist trotz abgelehnter Mitteilungen im Backend gelandet",
            201,
            statusBeimAnmelden(),
        )
        assertEquals("Firebase darf gar nicht erst gefragt worden sein", 0, kennungAbgefragt)
        assertFalse(speicher.wert)
    }

    /** Die Gegenprobe: Mit Erlaubnis muss die Kennung dort ankommen. */
    @Test
    fun mitErlaubnisLiegtDieKennungAmBackend() {
        assumeTrue(
            "Dieses Gerät erlaubt keine Mitteilungen — der Fall gehört in den " +
                "gewöhnlichen Gradle-Durchgang",
            Benachrichtigungserlaubnis.wirksam(kontext),
        )

        runBlocking { abgleich().abgleichen(Benachrichtigungserlaubnis.wirksam(kontext)) }

        assertEquals(
            "Bei erlaubten Mitteilungen muss die Kennung beim Backend liegen",
            200,
            statusBeimAnmelden(),
        )
        assertTrue(speicher.wert)
    }

    /**
     * Der Entzug. Läuft in beiden Durchgängen: Der Erlaubniszustand wird hier
     * ausdrücklich übergeben, weil er sich am System nicht umstellen lässt —
     * der Weg zum Backend ist deshalb nicht weniger echt.
     */
    @Test
    fun entzogeneErlaubnisRaeumtDieKennungWiederWeg() {
        runBlocking { abgleich().abgleichen(erlaubt = true) }
        assertTrue("Vorbedingung: die Kennung liegt beim Backend", speicher.wert)

        // Jetzt dreht die Person die Mitteilungen in den Einstellungen ab.
        runBlocking { abgleich().abgleichen(erlaubt = false) }

        assertEquals(
            "Nach dem Entzug darf die Kennung nicht mehr im Backend stehen",
            201,
            statusBeimAnmelden(),
        )
        assertFalse(speicher.wert)
    }

    // --- Hilfen --------------------------------------------------------------

    private class Speicher(var wert: Boolean = false) : Anmeldespeicher {
        override suspend fun angemeldet() = wert
        override suspend fun merken(wert: Boolean) { this.wert = wert }
    }

    /**
     * Der echte Abgleich gegen das echte Backend — nur die Kennung ist fest
     * gesetzt statt von Firebase geholt: Das Systemabbild von API 28 hat keine
     * Google-Play-Dienste und liefert dort gar keine. Firebase selbst wird
     * bewusst nicht angefasst (`firebaseBereit` bleibt leer), damit der Test
     * den Zustand der Installation nicht verändert.
     */
    private fun abgleich() = Geraeteabgleich(
        speicher = speicher,
        geraete = ApiDeviceRepository(DorfApi.create(BuildConfig.API_BASE_URL) { token }),
        kennung = { kennungAbgefragt++; kennung },
        kennungVerwerfen = {},
    )

    /**
     * Meldet die Kennung selbst an und liefert den Statuscode: 201 = das
     * Backend kannte sie nicht, 200 = sie lag schon dort.
     */
    private fun statusBeimAnmelden(): Int {
        val anfrage = Request.Builder()
            .url("$basis/api/v1/me/devices")
            .header("Authorization", "Bearer $token")
            .post("""{"token":"$kennung"}""".toRequestBody(json))
            .build()
        return client.newCall(anfrage).execute().use { it.code }
    }

    private fun abmeldenAmBackend() {
        val anfrage = Request.Builder()
            .url("$basis/api/v1/me/devices?token=$kennung")
            .header("Authorization", "Bearer $token")
            .delete()
            .build()
        client.newCall(anfrage).execute().close()
    }
}
