package de.roessing.app

import android.content.Intent
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.BySelector
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import org.junit.Assume.assumeFalse
import org.junit.Before
import org.junit.Test
import java.util.regex.Pattern

/**
 * Echter Ende-zu-Ende-Login gegen die Produktion (id.xn--rssing-wxa.de).
 *
 * Deckt genau den Weg ab, der in der Vergangenheit kaputt war: Browser-Login →
 * Rücksprung über AppAuths RedirectUriReceiverActivity → Token-Tausch →
 * angemeldete Ansicht. Ein Absturz beim Rücksprung (falsches Theme) oder ein
 * fehlgeschlagener Token-Tausch lässt diesen Test scheitern.
 *
 * Läuft nur, wenn die Zugangsdaten als Instrumentation-Argumente übergeben werden:
 *
 *     ./gradlew connectedDebugAndroidTest \
 *       -Pandroid.testInstrumentationRunnerArguments.realLoginUser=… \
 *       -Pandroid.testInstrumentationRunnerArguments.realLoginPassword=… \
 *       -Pandroid.testInstrumentationRunnerArguments.class=de.roessing.app.RealLoginE2eTest
 *
 * Ohne diese Argumente wird der Test übersprungen (Assume), damit lokale Läufe und
 * PRs ohne Secrets weiterhin grün sind.
 */
class RealLoginE2eTest {

    private lateinit var device: UiDevice
    private lateinit var user: String
    private lateinit var password: String

    /** „Weiter"-Buttons der Zitadel-Login-UI, sprachunabhängig. */
    private val weiterButton: BySelector = By.clazz("android.widget.Button")
        .text(Pattern.compile("continue|weiter|next|anmelden|submit|sign in", Pattern.CASE_INSENSITIVE))

    /** Einmalige Chrome-Dialoge, die den Login-Flow blockieren können. */
    private val chromeDialogTexte = Pattern.compile(
        "accept & continue|accept and continue|no thanks|nein danke|use without an account|" +
            "got it|not now|später|skip|überspringen|verstanden",
        Pattern.CASE_INSENSITIVE,
    )

    @Before
    fun setUp() {
        val args = InstrumentationRegistry.getArguments()
        user = args.getString("realLoginUser").orEmpty()
        password = args.getString("realLoginPassword").orEmpty()
        assumeFalse(
            "realLoginUser/realLoginPassword nicht gesetzt — echter Login-Test wird übersprungen",
            user.isBlank() || password.isBlank(),
        )
        device = UiDevice.getInstance(InstrumentationRegistry.getInstrumentation())
    }

    @Test
    fun echterLogin_fuehrtZurAngemeldetenAnsicht() {
        starteAppFrisch()

        // 1) Login-Screen der App → Browser öffnen.
        val loginButton = device.wait(Until.findObject(By.text("Mit Rössing-ID anmelden")), 30_000)
        requireNotNull(loginButton) { "Login-Screen der App nicht gefunden" }
        loginButton.click()

        // 2) Zitadel-Loginname. Chrome baut den Accessibility-Baum verzögert auf,
        //    deshalb großzügig warten und ggf. Erst-Start-Dialoge wegklicken.
        val nameFeld = wartAufEingabefeld(60_000)
            ?: error("Loginname-Feld der Zitadel-Anmeldung nicht gefunden")
        nameFeld.text = user
        klickeWeiter()

        // 3) Passwort.
        device.wait(Until.findObject(By.text(Pattern.compile("password|passwort", Pattern.CASE_INSENSITIVE))), 30_000)
        val pwFeld = wartAufEingabefeld(30_000)
            ?: error("Passwort-Feld der Zitadel-Anmeldung nicht gefunden")
        pwFeld.text = password
        klickeWeiter()

        // 4) Rücksprung in die App: Der Login gilt erst als bewiesen, wenn die
        //    angemeldete Ansicht (Karte/Liste) erscheint.
        val angemeldet = device.wait(Until.hasObject(By.text("Blumenkästen")), 90_000)

        // Aussagekräftige Diagnose, falls der Rücksprung wieder bricht.
        if (!angemeldet) {
            val fehler = device.findObject(By.textContains("Anmeldung fehlgeschlagen"))?.text
            error("Nach dem Login erschien nicht die angemeldete Ansicht. Angezeigter Fehler: $fehler")
        }

        // Die Orte-Liste beweist zusätzlich, dass das Access-Token vom Backend
        // akzeptiert wird (echter API-Aufruf, keine Mocks). Direkt nach dem Login
        // läuft der erste Abruf noch, deshalb großzügig warten.
        val listeTab = device.wait(Until.findObject(By.text("Liste")), 30_000)
        requireNotNull(listeTab) { "Tab „Liste\" nach dem Login nicht gefunden" }
        listeTab.click()
        // Bewusst kein konkreter Ortsname: geprüft wird der Status-Text, den jeder
        // geladene Ort trägt. So hängt der Test nicht am Inhalt der Produktionsdaten.
        // Auf Ortsebene sind die Texte neutral (dort können Gieß- und
        // Jätaufgaben zusammenkommen); die Gieß-Texte bleiben als Altbestand
        // in der Liste zulässig.
        val ortStatus = By.text(
            Pattern.compile(
                "alles gut|bitte erledigen|dringend!|bitte gießen|dringend gießen",
                Pattern.CASE_INSENSITIVE,
            ),
        )
        check(device.wait(Until.hasObject(ortStatus), 60_000)) {
            "Orte-Liste wurde nach dem Login nicht geladen"
        }
    }

    /** Startet die App mit geleertem Zustand, damit wirklich der Login-Screen kommt. */
    private fun starteAppFrisch() {
        val ctx = InstrumentationRegistry.getInstrumentation().targetContext
        device.pressHome()
        val intent = requireNotNull(ctx.packageManager.getLaunchIntentForPackage(ctx.packageName))
            .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TASK or Intent.FLAG_ACTIVITY_NEW_TASK)
        ctx.startActivity(intent)
        device.wait(Until.hasObject(By.pkg(ctx.packageName).depth(0)), 30_000)
    }

    /**
     * Wartet auf ein Texteingabefeld im Custom Tab und räumt dabei Chrome-Dialoge weg.
     * Chrome liefert den Accessibility-Baum der Webseite erst nach kurzer Verzögerung,
     * deshalb wird wiederholt gepollt statt einmalig gesucht.
     */
    private fun wartAufEingabefeld(timeoutMs: Long) = pollen(timeoutMs) {
        device.findObject(By.clazz("android.widget.EditText"))
            ?: run { schliesseChromeDialoge(); null }
    }

    private fun klickeWeiter() {
        val button = pollen(30_000) { device.findObject(weiterButton) }
            ?: error("„Weiter\"-Button der Zitadel-Anmeldung nicht gefunden")
        button.click()
        device.waitForIdle()
    }

    private fun schliesseChromeDialoge() {
        device.findObject(By.clazz("android.widget.Button").text(chromeDialogTexte))?.click()
            ?: device.findObject(By.text(chromeDialogTexte))?.click()
    }

    private fun <T> pollen(timeoutMs: Long, block: () -> T?): T? {
        val ende = System.currentTimeMillis() + timeoutMs
        while (System.currentTimeMillis() < ende) {
            block()?.let { return it }
            device.waitForIdle()
            Thread.sleep(500)
        }
        return null
    }
}
