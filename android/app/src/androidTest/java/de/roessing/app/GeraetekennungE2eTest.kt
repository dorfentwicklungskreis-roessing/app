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
 * Die Gerätekennung darf nur bei erlaubten Benachrichtigungen beim Backend
 * liegen — hier am echten Android geprüft, mit dem echten Erlaubniszustand
 * des Systems und gegen das echte Backend.
 *
 * Der Nachweis nutzt einen Umstand des Backends: `POST /api/v1/me/devices`
 * antwortet mit **201**, wenn die Kennung dort neu ist, und mit **200**, wenn
 * sie schon lag (siehe internal/api/geraete.go). Danach lässt sich also
 * ablesen, ob die App die Kennung zuvor hinterlegt hat — ohne dass es dafür
 * einen Auskunfts-Endpunkt bräuchte, den es aus gutem Grund nicht gibt.
 */
@RunWith(AndroidJUnit4::class)
class GeraetekennungE2eTest {
    private val instrumentierung = InstrumentationRegistry.getInstrumentation()
    private val kontext = instrumentierung.targetContext
    private val paket = "de.roessing.app"
    private val token = "kennung-e2e:Kennung Tester:member"
    private val basis = BuildConfig.API_BASE_URL.trimEnd('/')
    private val client = OkHttpClient()
    private val json = "application/json".toMediaType()

    /** Eindeutig je Lauf, damit ein früherer Lauf das Ergebnis nicht färbt. */
    private val kennung = "e2e-kennung-${System.nanoTime()}"

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
        // Andere Tests (und der echte Push-Test) brauchen wieder erlaubte
        // Benachrichtigungen — den Emulator so hinterlassen, wie er war.
        benachrichtigungen(erlaubt = true)
        runCatching { abmeldenAmBackend() }
    }

    // --- Testfälle -----------------------------------------------------------

    @Test
    fun ohneErlaubnisKommtKeineKennungAmBackendAn() {
        benachrichtigungen(erlaubt = false)
        assertFalse(
            "Der Emulator meldet trotz abgeschalteter Benachrichtigungen eine Erlaubnis",
            Benachrichtigungserlaubnis.wirksam(kontext),
        )

        val speicher = Speicher()
        runBlocking { abgleich(speicher).abgleichen(Benachrichtigungserlaubnis.wirksam(kontext)) }

        assertEquals(
            "Die Kennung ist trotz abgelehnter Benachrichtigungen im Backend gelandet",
            201,
            statusBeimAnmelden(),
        )
        assertFalse(speicher.wert)
    }

    @Test
    fun mitErlaubnisLiegtDieKennungAmBackend() {
        benachrichtigungen(erlaubt = true)
        assumeTrue(
            "Benachrichtigungen ließen sich auf diesem Abbild nicht erlauben",
            Benachrichtigungserlaubnis.wirksam(kontext),
        )

        val speicher = Speicher()
        runBlocking { abgleich(speicher).abgleichen(Benachrichtigungserlaubnis.wirksam(kontext)) }

        assertEquals(
            "Bei erlaubten Benachrichtigungen muss die Kennung beim Backend liegen",
            200,
            statusBeimAnmelden(),
        )
        assertTrue(speicher.wert)
    }

    @Test
    fun entzogeneErlaubnisRaeumtDieKennungWiederWeg() {
        benachrichtigungen(erlaubt = true)
        assumeTrue(
            "Benachrichtigungen ließen sich auf diesem Abbild nicht erlauben",
            Benachrichtigungserlaubnis.wirksam(kontext),
        )
        val speicher = Speicher()
        runBlocking { abgleich(speicher).abgleichen(erlaubt = true) }
        assertTrue("Vorbedingung: die Kennung liegt beim Backend", speicher.wert)

        // Jetzt dreht die Person die Benachrichtigungen in den Einstellungen ab.
        benachrichtigungen(erlaubt = false)
        runBlocking { abgleich(speicher).abgleichen(Benachrichtigungserlaubnis.wirksam(kontext)) }

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
     * Der echte Abgleich mit dem echten Backend — nur die Kennung ist fest
     * gesetzt statt von Firebase geholt: Das Systemabbild von API 28 hat
     * keine Google-Play-Dienste und liefert dort gar keine.
     */
    private fun abgleich(speicher: Anmeldespeicher) = Geraeteabgleich(
        speicher = speicher,
        geraete = ApiDeviceRepository(DorfApi.create(BuildConfig.API_BASE_URL) { token }),
        kennung = { kennung },
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

    /**
     * Legt den Schalter um, den sonst die Person in den Android-Einstellungen
     * umlegt. `appops POST_NOTIFICATION` steuert `areNotificationsEnabled()`
     * auf allen API-Ständen; ab Android 13 kommt die Laufzeitberechtigung
     * hinzu, die getrennt erteilt und entzogen wird.
     */
    private fun benachrichtigungen(erlaubt: Boolean) {
        schale("cmd appops set $paket POST_NOTIFICATION ${if (erlaubt) "allow" else "ignore"}")
        // Nur erteilen, nie entziehen: `pm revoke` schießt den eigenen Prozess
        // ab — und damit den laufenden Test. Zum Abschalten genügt der
        // AppOp; er ist genau der Schalter, den Android in den Einstellungen
        // zeigt, und `areNotificationsEnabled()` richtet sich danach.
        if (erlaubt && Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            schale("pm grant $paket android.permission.POST_NOTIFICATIONS")
        }
        // Der Wechsel braucht einen Wimpernschlag, bis ihn der eigene Prozess
        // sieht.
        Thread.sleep(500)
    }

    private fun schale(befehl: String) {
        instrumentierung.uiAutomation.executeShellCommand(befehl).use { deskriptor ->
            java.io.FileInputStream(deskriptor.fileDescriptor).use { it.readBytes() }
        }
    }
}
