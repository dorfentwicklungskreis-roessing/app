package de.roessing.app.push

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import android.util.Log
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import de.roessing.app.data.DeviceRepository

/**
 * Darf die App überhaupt benachrichtigen?
 *
 * Maßgeblich ist der *wirksame* Zustand, nicht bloß die Laufzeitberechtigung:
 * Vor Android 13 gibt es `POST_NOTIFICATIONS` gar nicht — dort zählt allein,
 * ob die Benachrichtigungen in den Einstellungen anstehen. Ab Android 13 muss
 * beides zusammenkommen: die Berechtigung *und* der Schalter im System. Wer
 * die Erlaubnis erteilt und die Meldungen später in den Einstellungen
 * abdreht, hat sie eben doch nicht erlaubt.
 */
object Benachrichtigungserlaubnis {

    /** Die Entscheidung für sich genommen — ohne Android, damit prüfbar. */
    fun wirksam(sdk: Int, berechtigungErteilt: Boolean, systemErlaubt: Boolean): Boolean =
        if (sdk >= Build.VERSION_CODES.TIRAMISU) {
            berechtigungErteilt && systemErlaubt
        } else {
            systemErlaubt
        }

    /** Dieselbe Frage, am echten Gerät gestellt. */
    fun wirksam(context: Context): Boolean = wirksam(
        sdk = Build.VERSION.SDK_INT,
        berechtigungErteilt = ContextCompat.checkSelfPermission(
            context,
            Manifest.permission.POST_NOTIFICATIONS,
        ) == PackageManager.PERMISSION_GRANTED,
        systemErlaubt = NotificationManagerCompat.from(context).areNotificationsEnabled(),
    )
}

/**
 * Merkt sich, ob diese Installation ihre Kennung beim Backend hinterlegt hat.
 *
 * Nötig, weil sich die Frage sonst nicht ohne Nebenwirkung beantworten ließe:
 * Firebase nach der Kennung zu fragen *erzeugt* sie. Wer nie eingewilligt
 * hat, dessen Gerät soll aber gar keine Kennung bekommen — also merken wir
 * uns lokal, ob es je eine gab, statt bei Firebase nachzuschlagen.
 */
interface Anmeldespeicher {
    suspend fun angemeldet(): Boolean
    suspend fun merken(wert: Boolean)
}

/**
 * Bringt die Gerätekennung mit der erteilten Erlaubnis in Einklang.
 *
 * Die Kennung ist ein personenbezogenes Datum: Sie steht für genau dieses
 * Handy, und das Backend kann darüber Nachrichten schicken. Sie darf deshalb
 * nur entstehen und nur beim Backend liegen, solange Benachrichtigungen
 * wirklich erlaubt sind. Wird die Erlaubnis in den Android-Einstellungen
 * wieder entzogen, merkt die App das beim nächsten Start (bzw. bei der
 * Rückkehr in den Vordergrund) und räumt die Kennung weg.
 */
class Geraeteabgleich(
    private val speicher: Anmeldespeicher,
    private val geraete: DeviceRepository,
    /** Fragt Firebase nach der Kennung — und legt sie dabei an, falls nötig. */
    private val kennung: suspend () -> String?,
    /** Wirft die Kennung bei Firebase weg. */
    private val kennungVerwerfen: suspend () -> Unit,
) {

    /** Der Abgleich bei jedem Start und bei jeder Rückkehr in den Vordergrund. */
    suspend fun abgleichen(erlaubt: Boolean) {
        anmelden()
    }

    /**
     * Kennung anlegen und beim Backend hinterlegen. Auch wenn sie dort schon
     * liegt: Das Backend legt dieselbe Kennung nicht doppelt an, und der
     * Zeitstempel bleibt so frisch.
     */
    suspend fun anmelden() {
        val token = kennung() ?: return
        runCatching { geraete.register(token) }
            .onSuccess {
                speicher.merken(true)
                Log.i(TAG, "Gerät für Benachrichtigungen angemeldet")
            }
            .onFailure { Log.w(TAG, "Anmelden der Kennung fehlgeschlagen", it) }
    }

    /**
     * Kennung beim Backend löschen und danach wegwerfen. Wer nie angemeldet
     * war, hat auch keine Kennung — dann bleibt es dabei, und es wird keine
     * angelegt, nur um sie gleich wieder zu löschen.
     *
     * Scheitert der Aufruf (kein Netz), bleibt die Merkung stehen: Der
     * nächste Start versucht es erneut. Erst wenn das Backend die Kennung
     * los ist, wird sie auch bei Firebase weggeworfen — sonst hätten wir eine
     * Kennung im Backend, die niemand mehr abmelden kann.
     */
    suspend fun abmelden() {
        if (!speicher.angemeldet()) return
        val token = kennung() ?: return
        val abgemeldet = runCatching { geraete.unregister(token) }
            .onFailure { Log.w(TAG, "Abmelden der Kennung fehlgeschlagen", it) }
            .isSuccess
        if (!abgemeldet) return
        runCatching { kennungVerwerfen() }
        speicher.merken(false)
        Log.i(TAG, "Gerätekennung gelöscht")
    }

    private companion object {
        const val TAG = "Push"
    }
}
