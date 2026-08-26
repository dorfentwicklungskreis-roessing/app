package de.roessing.app.ui

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.produceState
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import de.roessing.app.R

/**
 * Die Pflichtseiten der App. Sie stehen auf der Website und werden nur dort
 * gepflegt — eine zweite Fassung in der App würde über kurz oder lang von der
 * ersten abweichen, und dann wäre keine mehr verbindlich.
 */
enum class Rechtsdokument(val url: String) {
    IMPRESSUM("https://xn--rssing-wxa.de/impressum/"),
    DATENSCHUTZ("https://xn--rssing-wxa.de/app/datenschutz/"),
}

/**
 * Impressum und Datenschutzerklärung — leicht erkennbar und unmittelbar
 * erreichbar, wie es § 5 DDG verlangt. Steht deshalb auf der Startseite
 * *und* auf dem Anmeldebildschirm: Wer noch kein Konto hat, muss trotzdem
 * nachlesen können, was mit seinen Daten geschieht.
 */
@Composable
fun Rechtliches(modifier: Modifier = Modifier) {
    val context = LocalContext.current
    RechtlichesLeiste(
        online = netzVerfuegbar(),
        modifier = modifier,
        onOeffnen = { dokument -> context.openInCustomTab(dokument.url) },
    )
}

/** Die Anzeige für sich — ohne Kontext und ohne Netz, damit sie prüfbar ist. */
@Composable
fun RechtlichesLeiste(
    online: Boolean,
    onOeffnen: (Rechtsdokument) -> Unit,
    modifier: Modifier = Modifier,
) {
    Column(
        modifier.fillMaxWidth().testTag("rechtliches"),
        horizontalAlignment = Alignment.CenterHorizontally,
    ) {
        Text(
            stringResource(R.string.legal_title),
            style = MaterialTheme.typography.labelMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Row(horizontalArrangement = Arrangement.Center) {
            TextButton(
                onClick = { onOeffnen(Rechtsdokument.IMPRESSUM) },
                enabled = online,
                modifier = Modifier.testTag("rechtliches-impressum"),
            ) {
                Text(stringResource(R.string.legal_imprint))
            }
            TextButton(
                onClick = { onOeffnen(Rechtsdokument.DATENSCHUTZ) },
                enabled = online,
                modifier = Modifier.testTag("rechtliches-datenschutz"),
            ) {
                Text(stringResource(R.string.legal_privacy))
            }
        }
        // Ohne Netz bleibt der Knopf grau — dafür steht die Adresse da, unter
        // der die Seiten später (oder auf einem anderen Gerät) zu finden sind.
        if (!online) {
            Text(
                stringResource(R.string.legal_offline),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                textAlign = TextAlign.Center,
                modifier = Modifier.testTag("rechtliches-offline"),
            )
        }
    }
}

/**
 * Ob gerade eine Internetverbindung besteht — und zwar laufend: Wer die App
 * im Funkloch öffnet und danach ins WLAN kommt, soll die Seiten öffnen
 * können, ohne den Bildschirm zu verlassen.
 *
 * Im Zweifel „ja": Eine falsch erkannte Trennung würde den Weg zu Pflicht-
 * angaben verstellen, ein vergeblicher Versuch kostet nur einen Fingertipp.
 */
@Composable
private fun netzVerfuegbar(): Boolean {
    val context = LocalContext.current
    return produceState(initialValue = istOnline(context), context) {
        val dienst = context.getSystemService(ConnectivityManager::class.java)
            ?: return@produceState
        val horcher = object : ConnectivityManager.NetworkCallback() {
            override fun onAvailable(network: Network) {
                value = true
            }

            override fun onLost(network: Network) {
                value = istOnline(context)
            }
        }
        val angemeldet = runCatching { dienst.registerDefaultNetworkCallback(horcher) }.isSuccess
        awaitDispose { if (angemeldet) runCatching { dienst.unregisterNetworkCallback(horcher) } }
    }.value
}

/** Einmalige Nachfrage beim System (siehe [netzVerfuegbar]). */
private fun istOnline(context: Context): Boolean = runCatching {
    val dienst = context.getSystemService(ConnectivityManager::class.java)
        ?: return@runCatching true
    val netz = dienst.activeNetwork ?: return@runCatching false
    val koennen = dienst.getNetworkCapabilities(netz) ?: return@runCatching false
    koennen.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
}.getOrDefault(true)
