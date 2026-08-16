package de.roessing.app

import de.roessing.app.auth.appAuthKonfiguration
import de.roessing.app.auth.klartextAusstellerErlaubt
import net.openid.appauth.connectivity.DefaultConnectionBuilder
import org.junit.Assert.assertFalse
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * AppAuth lässt von sich aus **nur https** zu — zu Recht: Ein OIDC-Fluss über
 * Klartext läge in der Hand jedes Netzes dazwischen.
 *
 * Für den E2E-Test läuft der Aussteller aber auf demselben Rechner
 * (`http://10.0.2.2:8123`), damit sich kein Test an der Produktion anmelden
 * muss. Deshalb gibt es einen eigenen Verbindungsaufbau, der Klartext zulässt.
 *
 * Diese Ausnahme ist die gefährlichste Zeile des ganzen Umbaus: Griffe sie im
 * ausgelieferten Build, wäre die Anmeldung der Dorf-App mitlesbar. Sie hat
 * deshalb zwei Riegel — Debug-Build **und** ein ausdrücklich auf http
 * gestellter Aussteller — und hier stehen die Fälle, die das festnageln.
 */
class KlartextAusstellerTest {

    @Test
    fun `im Release-Build niemals, auch nicht bei http`() {
        assertFalse(klartextAusstellerErlaubt(debug = false, issuer = "http://10.0.2.2:8123"))
        assertFalse(klartextAusstellerErlaubt(debug = false, issuer = "http://localhost:8123"))
    }

    @Test
    fun `die Produktion bleibt auch im Debug-Build auf https`() {
        // ci-extern-ok: reine Zeichenkette in einer Behauptung — hier wird
        // gerade nachgewiesen, dass die Produktion NICHT im Klartext läuft.
        assertFalse(klartextAusstellerErlaubt(debug = true, issuer = "https://id.xn--rssing-wxa.de")) // ci-extern-ok: nur Zeichenkette
    }

    @Test
    fun `im Debug-Build mit ausdruecklichem http-Aussteller erlaubt`() {
        assertTrue(klartextAusstellerErlaubt(debug = true, issuer = "http://10.0.2.2:8123"))
    }

    /**
     * „httpsx://…" oder ein Aussteller, der nur zufällig mit „http" beginnt,
     * darf die Ausnahme nicht auslösen — geprüft wird das Schema, nicht der
     * Wortanfang.
     */
    @Test
    fun `nur das echte http-Schema zaehlt`() {
        assertFalse(klartextAusstellerErlaubt(debug = true, issuer = "httpsx://id.example"))
        assertFalse(klartextAusstellerErlaubt(debug = true, issuer = "httpfoo://id.example"))
        assertFalse(klartextAusstellerErlaubt(debug = true, issuer = ""))
    }

    /**
     * Und der Beweis am gebauten Ergebnis: Dieser Testlauf hat keine
     * Aussteller-Übersteuerung, also gilt die Vorbelegung aus
     * `build.gradle.kts` — die Produktion über https. Dann muss AppAuth
     * unverändert streng eingestellt sein, obwohl es ein Debug-Build ist.
     *
     * Damit hängt nicht nur die Entscheidung an einem Test, sondern auch das,
     * was tatsächlich gebaut wird.
     */
    @Test
    fun `ohne Uebersteuerung bleibt AppAuth streng`() {
        assertFalse(
            "Der ausgelieferte Aussteller darf die https-Pflicht des ID-Tokens nicht lockern",
            appAuthKonfiguration.skipIssuerHttpsCheck,
        )
        assertSame(
            "Ohne lokalen Aussteller muss der Standard-Verbindungsaufbau greifen",
            DefaultConnectionBuilder.INSTANCE,
            appAuthKonfiguration.connectionBuilder,
        )
    }
}
