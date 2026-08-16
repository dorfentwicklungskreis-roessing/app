package de.roessing.app.auth

import android.net.Uri
import de.roessing.app.BuildConfig
import net.openid.appauth.connectivity.ConnectionBuilder
import net.openid.appauth.connectivity.DefaultConnectionBuilder
import java.net.HttpURLConnection
import java.net.URL
import java.util.concurrent.TimeUnit

/**
 * Verbindungsaufbau für AppAuth — https, außer der Aussteller steht bewusst
 * lokal.
 *
 * AppAuth lässt von sich aus **ausschließlich https** zu und wirft sonst
 * `IllegalArgumentException: only https connections are permitted`. Das ist
 * richtig so: Discovery und Token-Tausch über Klartext lägen in der Hand jedes
 * Netzes dazwischen.
 *
 * Die E2E-Tests brauchen aber einen Aussteller auf demselben Rechner
 * (`http://10.0.2.2:8123`, von der CI im docker compose mitgestartet), damit
 * sich kein Test an der echten Rössing-ID anmelden muss — ein solcher Test
 * wird rot, sobald die Produktion hustet, und kann dort Daten verändern.
 *
 * Die Ausnahme hat deshalb zwei Riegel, die **beide** greifen müssen:
 *
 *  1. Debug-Build (`BuildConfig.DEBUG`) — im ausgelieferten Release-Build gibt
 *     es sie nicht.
 *  2. Ein ausdrücklich auf `http://` gestellter Aussteller. Die Vorbelegung in
 *     `build.gradle.kts` ist `https://id.xn--rssing-wxa.de`; wer nichts
 *     übersteuert, bekommt auch im Debug-Build den strengen Aufbau.
 *
 * Die Entscheidung selbst steht als reine Funktion in
 * [klartextAusstellerErlaubt] — sie ist in `KlartextAusstellerTest` festgenagelt.
 */

/**
 * Darf für [issuer] Klartext gesprochen werden?
 *
 * Bewusst ohne Android-Abhängigkeiten, damit der JVM-Unit-Test alle Fälle
 * abdecken kann. Geprüft wird das Schema, nicht der Wortanfang: „httpfoo://"
 * ist kein http.
 */
fun klartextAusstellerErlaubt(debug: Boolean, issuer: String): Boolean =
    debug && issuer.startsWith("http://")

/**
 * Wie [DefaultConnectionBuilder], nur ohne die https-Pflicht.
 *
 * Die Zeitgrenzen sind bewusst dieselben wie dort — hier soll sich allein das
 * erlaubte Schema unterscheiden.
 */
private object KlartextVerbindungsaufbau : ConnectionBuilder {
    private val VERBINDEN_MS = TimeUnit.SECONDS.toMillis(15).toInt()
    private val LESEN_MS = TimeUnit.SECONDS.toMillis(10).toInt()

    override fun openConnection(uri: Uri): HttpURLConnection {
        val schema = uri.scheme
        require(schema == "http" || schema == "https") { "nur http oder https: $uri" }
        return (URL(uri.toString()).openConnection() as HttpURLConnection).apply {
            connectTimeout = VERBINDEN_MS
            readTimeout = LESEN_MS
            instanceFollowRedirects = false
        }
    }
}

/**
 * Der Verbindungsaufbau, den AuthManager verwendet: streng, es sei denn beide
 * Riegel oben sind offen.
 */
val oidcVerbindungsaufbau: ConnectionBuilder =
    if (klartextAusstellerErlaubt(BuildConfig.DEBUG, BuildConfig.OIDC_ISSUER)) {
        KlartextVerbindungsaufbau
    } else {
        DefaultConnectionBuilder.INSTANCE
    }
