package de.roessing.app

import android.os.SystemClock
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import com.google.firebase.messaging.FirebaseMessaging
import de.roessing.app.data.ApiDeviceRepository
import de.roessing.app.data.ApiVergabeRepository
import de.roessing.app.data.DorfApi
import de.roessing.app.push.Kanaele
import kotlinx.coroutines.runBlocking
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import org.json.JSONObject
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Echter Push im Emulator — nur auf Zuruf (`-e push true`) und nur gegen ein
 * Backend mit hinterlegtem Dienstkonto-Schlüssel:
 *
 *     FCM_CREDENTIALS_FILE=… go run ./cmd/server
 *     adb shell am instrument -e class de.roessing.app.PushEchtTest \
 *       -e push true -e e2e true …
 *
 * Ohne Google-Play-Dienste im Systemabbild gibt es keine Gerätekennung; der
 * Test überspringt sich dann mit klarer Ansage, statt rot zu werden.
 */
@RunWith(AndroidJUnit4::class)
class PushEchtTest {
    private val basis = BuildConfig.API_BASE_URL.trimEnd('/')
    private val client = OkHttpClient()
    private val json = "application/json".toMediaType()
    private val token = "push-e2e:Push Tester:admin"

    @Before
    fun nurAufZuruf() {
        assumeTrue(
            "Echter Push nur mit -e push true",
            InstrumentationRegistry.getArguments().getString("push") == "true",
        )
    }

    private fun post(pfad: String, koerper: String): JSONObject {
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

    /** Holt die Gerätekennung von Firebase — oder null ohne Play-Dienste. */
    private fun geraeteKennung(): String? {
        val ergebnis = arrayOfNulls<String>(1)
        val fertig = java.util.concurrent.CountDownLatch(1)
        // Firebase steht im Manifest bewusst still (siehe Geraeteabgleich):
        // ohne erlaubte Mitteilungen soll gar keine Kennung entstehen. Für
        // diesen Test wird es ausdrücklich scharf gestellt.
        runCatching {
            com.google.firebase.FirebaseApp.getInstance()
                .setDataCollectionDefaultEnabled(true as Boolean?)
            FirebaseMessaging.getInstance().isAutoInitEnabled = true
        }
        runCatching {
            FirebaseMessaging.getInstance().token
                .addOnSuccessListener { ergebnis[0] = it; fertig.countDown() }
                .addOnFailureListener { fertig.countDown() }
        }.onFailure { fertig.countDown() }
        fertig.await(60, java.util.concurrent.TimeUnit.SECONDS)
        return ergebnis[0]
    }

    @Test
    fun anfrageKommtAlsSystemmeldung() {
        val geraet = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
        val kontext = InstrumentationRegistry.getInstrumentation().targetContext
        Kanaele.anlegen(kontext)

        val kennung = geraeteKennung()
        assumeTrue(
            "Keine Gerätekennung — dieses Systemabbild hat keine Google-Play-Dienste",
            kennung != null,
        )
        val api = DorfApi.create(BuildConfig.API_BASE_URL) { token }
        runBlocking { ApiDeviceRepository(api).register(kennung!!) }

        // Ein eigener, überfälliger Ort — daran läuft die Vergabe sofort los.
        val ort = post(
            "/api/v1/places",
            """{"name":"Push-E2E ${System.currentTimeMillis()}","kind":"blumenkasten","lat":52.2105,"lon":9.8695}""",
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
        runBlocking { ApiVergabeRepository(api).signup(ort.getLong("id"), null) }

        // Auf die Systemmeldung warten — sie kommt über Google, nicht über uns.
        geraet.openNotification()
        val gefunden = geraet.wait(Until.hasObject(By.textContains("ist dran")), 90_000)
        SystemClock.sleep(500)
        geraet.takeScreenshot(
            java.io.File(
                java.io.File(kontext.filesDir, "push-neu").apply { mkdirs() },
                "push-01-systemmeldung.png",
            ),
        )
        assertTrue("Keine Push-Benachrichtigung eingetroffen", gefunden)

        // Antippen bringt die App nach vorn — mit dem Ziel in den Extras
        // (siehe MainActivity.zielAus und PushZiel).
        geraet.findObject(By.textContains("ist dran")).click()
        val vorn = geraet.wait(Until.hasObject(By.pkg("de.roessing.app").depth(0)), 20_000)
        SystemClock.sleep(1_500)
        geraet.takeScreenshot(
            java.io.File(
                java.io.File(kontext.filesDir, "push-neu").apply { mkdirs() },
                "push-02-angetippt.png",
            ),
        )
        assertTrue("Der Fingertipp hat die App nicht geöffnet", vorn)
    }
}
