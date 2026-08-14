package de.roessing.app

import android.content.Intent
import android.util.Base64
import android.util.Log
import androidx.test.platform.app.InstrumentationRegistry
import androidx.test.uiautomator.By
import androidx.test.uiautomator.BySelector
import androidx.test.uiautomator.StaleObjectException
import androidx.test.uiautomator.UiDevice
import androidx.test.uiautomator.Until
import de.roessing.app.auth.LOGIN_SCOPES
import de.roessing.app.auth.ROLLEN_SCOPE
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assume.assumeFalse
import org.junit.Before
import org.junit.Test
import java.util.regex.Pattern

/** Stichwort, unter dem die Token-Claims ins Log gehen. */
private const val PROBE = "TOKENPROBE"

/** Stichwort für den Ablauf der Anmeldung — Diagnose, wenn sie hakt. */
private const val LOGIN_PROBE = "LOGINPROBE"

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

    /**
     * Der Zustimmungs-Bildschirm der Rössing-ID. Er erscheint, wenn eine App
     * zum ersten Mal etwas Neues anfragt — etwa nach dem Hinzufügen des
     * Rollen-Scopes. Wer ihn nicht wegklickt, bleibt im Browser stehen.
     */
    private val zustimmungButton: BySelector = By.clazz("android.widget.Button")
        .text(Pattern.compile("allow|zulassen|erlauben|zustimmen|accept|akzeptieren|continue|weiter", Pattern.CASE_INSENSITIVE))

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

        protokolliereBildschirm("nach dem Passwort")

        // 3b) Zustimmung, falls die Rössing-ID danach fragt. Das tut sie,
        //     wenn die App etwas Neues anfragt — etwa die Rollen. Ohne diesen
        //     Schritt bliebe der Login im Browser stehen.
        if (device.wait(Until.hasObject(By.textContains("Rössing")), 5_000) &&
            device.findObject(zustimmungButton) != null &&
            !device.hasObject(By.textStartsWith("Moin"))
        ) {
            protokolliereBildschirm("Zustimmung")
            runCatching { klickeRobust(zustimmungButton) { "Zustimmung nicht gefunden" } }
        }

        // 4) Rücksprung in die App: Der Login gilt erst als bewiesen, wenn die
        //    angemeldete Ansicht erscheint — das ist seit dem Umbau die
        //    Startseite mit den Bereichen, erkennbar an der Begrüßung.
        val angemeldet = device.wait(Until.hasObject(By.textStartsWith("Moin")), 90_000)

        // Aussagekräftige Diagnose, falls der Rücksprung wieder bricht.
        if (!angemeldet) {
            protokolliereBildschirm("nicht angemeldet")
            val fehler = device.findObject(By.textContains("Anmeldung fehlgeschlagen"))?.text
            error("Nach dem Login erschien nicht die angemeldete Ansicht. Angezeigter Fehler: $fehler")
        }

        // Die Orte-Liste beweist zusätzlich, dass das Access-Token vom Backend
        // akzeptiert wird (echter API-Aufruf, keine Mocks). Sie liegt jetzt im
        // Bereich „Mithelfen", also erst dorthin. Die Kachel trägt neben dem
        // Namen noch Untertitel und Statuszeile — deshalb textContains.
        val kachel = device.wait(Until.findObject(By.textContains("Mithelfen")), 30_000)
        requireNotNull(kachel) { "Bereichskachel „Mithelfen\" nach dem Login nicht gefunden" }
        kachel.click()

        // Direkt nach dem Login läuft der erste Abruf noch, deshalb großzügig warten.
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

        pruefeTokenClaims()
    }

    /**
     * Sieht im ECHTEN Token nach, was Zitadel tatsächlich hineingelegt hat.
     *
     * Der Anlass: Die App forderte anfangs keinen Rollen-Scope an. Zitadel
     * stellte daraufhin ein Token ohne jeden Rollen-Claim aus — in der
     * ausgelieferten App war damit niemand Verwaltung, und der ganze Bereich
     * „Verwaltung" antwortete nur mit 403. Kein Test hat das gesehen: Die
     * Rechteprüfungen erwarten das 403 ja, und die Test-Tokens der übrigen
     * E2E-Läufe fordern die Rollen ausdrücklich an.
     *
     * Deshalb wird hier am echten Aussteller nachgeschaut. Die Claims gehen
     * unter dem Stichwort TOKENPROBE ins Log; `android/ci-e2e.sh` holt sie in
     * die CI-Ausgabe, damit man sie ohne Debugger lesen kann.
     */
    private fun pruefeTokenClaims() {
        val ctx = InstrumentationRegistry.getInstrumentation().targetContext
        val token = runBlocking { ctx.appContainer.authManager.freshAccessToken() }
        requireNotNull(token) { "Nach dem Login liegt kein Access-Token vor" }

        val teile = token.split(".")
        check(teile.size == 3) { "Access-Token ist kein JWT (${teile.size} Teile)" }
        val nutzlast = String(
            Base64.decode(teile[1], Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING),
        )
        val claims = JSONObject(nutzlast)

        Log.i(PROBE, "angeforderte Scopes = ${LOGIN_SCOPES.joinToString(" ")}")
        Log.i(PROBE, "Access-Token: aud = ${claims.opt("aud")}")
        protokolliereRollen("Access-Token", claims)

        // Zitadel legt Rollen je nach Einstellung ins Access-Token, ins
        // ID-Token oder — ohne „Rollen zusichern" am Projekt — in keines von
        // beiden. Beide ansehen, sonst rät man.
        val idToken = ctx.appContainer.authManager.letztesIdToken
        if (idToken == null) {
            Log.i(PROBE, "ID-Token: keines vorhanden")
        } else {
            val idTeile = idToken.split(".")
            if (idTeile.size == 3) {
                val idClaims = JSONObject(
                    String(Base64.decode(idTeile[1], Base64.URL_SAFE or Base64.NO_WRAP or Base64.NO_PADDING)),
                )
                Log.i(PROBE, "ID-Token: aud = ${idClaims.opt("aud")}")
                protokolliereRollen("ID-Token", idClaims)
            }
        }

        // Das Token muss an diese App gerichtet sein — sonst würde eine
        // eingeschaltete Empfängerprüfung (AUTH_AUDIENCE) es abweisen.
        val aud = claims.opt("aud").toString()
        check(aud.contains(BuildConfig.OIDC_CLIENT_ID)) {
            "Das Token ist nicht an diese App gerichtet: aud = $aud"
        }

        // Die App muss die Rollen anfordern. Ob das Testkonto welche hat,
        // entscheidet Zitadel; dass wir danach fragen, entscheiden wir.
        check(LOGIN_SCOPES.contains(ROLLEN_SCOPE)) {
            "Die App fordert $ROLLEN_SCOPE nicht an — dann ist niemand Verwaltung"
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
        klickeRobust(weiterButton) { "„Weiter\"-Button der Zitadel-Anmeldung nicht gefunden" }
    }

    /**
     * Sucht ein Element und klickt es — und sucht neu, wenn es zwischen Fund
     * und Klick verschwindet. Die Zitadel-Oberfläche baut sich neu auf,
     * während wir zugreifen; ein einmal gefundenes Objekt ist dann veraltet
     * (StaleObjectException).
     */
    private fun klickeRobust(auswahl: BySelector, fehler: () -> String) {
        val ende = System.currentTimeMillis() + 30_000
        while (System.currentTimeMillis() < ende) {
            // Bewusst NUR suchen und klicken: Wer hier zwischendurch Dialoge
            // wegklickt, trifft schnell etwas anderes und leert das gerade
            // ausgefüllte Feld — die Anmeldung endet dann mit
            // „User not found in the system".
            val objekt = device.findObject(auswahl)
            if (objekt == null) {
                device.waitForIdle()
                Thread.sleep(500)
                continue
            }
            try {
                objekt.click()
                device.waitForIdle()
                return
            } catch (e: StaleObjectException) {
                // Die Seite hat sich unter uns erneuert — nächster Versuch.
                Thread.sleep(300)
            }
        }
        error(fehler())
    }

    /** Schreibt die Rollen-Claims eines Tokens ins Log. */
    private fun protokolliereRollen(welches: String, claims: JSONObject) {
        val rollen = claims.keys().asSequence().filter { it.contains(":roles") }.toList()
        Log.i(PROBE, "$welches: Rollen-Claims = $rollen")
        for (name in rollen) Log.i(PROBE, "  $name = ${claims.opt(name)}")
        if (rollen.isEmpty()) {
            Log.i(PROBE, "  (keine — mit diesem Token ist in der App niemand Verwaltung)")
        }
    }

    /** Schreibt den sichtbaren Bildschirm ins Log — Diagnose für die CI. */
    private fun protokolliereBildschirm(schritt: String) {
        val texte = device.findObjects(By.clazz("android.widget.TextView"))
            .mapNotNull { runCatching { it.text }.getOrNull() }
            .filter { it.isNotBlank() }
            .take(25)
        Log.i(LOGIN_PROBE, "$schritt: ${device.currentPackageName} | ${texte.joinToString(" · ")}")
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
