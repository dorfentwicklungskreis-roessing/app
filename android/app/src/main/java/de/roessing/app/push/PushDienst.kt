package de.roessing.app.push

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.os.Build
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import com.google.firebase.FirebaseApp
import com.google.firebase.messaging.FirebaseMessaging
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import de.roessing.app.MainActivity
import de.roessing.app.R
import de.roessing.app.appContainer
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlin.coroutines.resume

/**
 * Push-Benachrichtigungen über Firebase Cloud Messaging.
 *
 * Der Weg ist eine Abkürzung, kein Ersatz: Jede Anfrage steht auch in der
 * Abrufliste des Backends und erscheint beim nächsten Öffnen der App. Wer die
 * Erlaubnis verweigert oder wessen Nachricht unterwegs verlorengeht, verpasst
 * deshalb nichts — es dauert nur länger.
 */
class DorfMessagingService : FirebaseMessagingService() {

    /**
     * Firebase tauscht die Kennung von Zeit zu Zeit aus (Neuinstallation,
     * Datenwiederherstellung, Ablauf). Dann muss das Backend die neue kennen,
     * sonst schickt es weiter ins Leere.
     */
    override fun onNewToken(token: String) {
        // Auch die Erneuerung braucht die Erlaubnis: Ohne sie hätte die neue
        // Kennung beim Backend nichts verloren.
        if (!Benachrichtigungserlaubnis.wirksam(applicationContext)) {
            Log.i(TAG, "Neue Gerätekennung verworfen — Benachrichtigungen sind nicht erlaubt")
            return
        }
        Log.i(TAG, "Neue Gerätekennung von Firebase")
        CoroutineScope(SupervisorJob() + Dispatchers.IO).launch {
            runCatching { applicationContext.appContainer.deviceRepository.register(token) }
                .onSuccess { Geraeteanmeldung.speicher(applicationContext).merken(true) }
                .onFailure { Log.w(TAG, "Kennung konnte nicht angemeldet werden", it) }
        }
    }

    /**
     * Eintreffende Nachricht, während die App im Vordergrund läuft. Steht die
     * App im Hintergrund, zeigt Android die Meldung selbst an (dafür trägt das
     * Backend den `notification`-Teil mit ein) und ruft das hier nicht auf.
     */
    override fun onMessageReceived(message: RemoteMessage) {
        val ziel = PushZiel.ausDaten(message.data)
        val titel = message.notification?.title ?: ziel?.titel.orEmpty()
        val text = message.notification?.body ?: ziel?.text.orEmpty()
        if (titel.isBlank() && text.isBlank()) return
        Systemmeldung.zeigen(this, ziel, titel, text)
    }

    companion object {
        private const val TAG = "Push"
    }
}

/** Die Systembenachrichtigung, die die App selbst anzeigt. */
object Systemmeldung {
    fun zeigen(context: Context, ziel: PushZiel?, titel: String, text: String) {
        Kanaele.anlegen(context)
        if (!darfAnzeigen(context)) return
        val kanal = ziel?.kanal ?: PushKanal.HINWEISE
        val absicht = Intent(context, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP
            // Das Ziel reist als einfache Zeichenketten mit — genau die, die
            // auch von Firebase kamen (siehe PushZiel).
            ziel?.alsDaten()?.forEach { (k, v) -> putExtra(k, v) }
        }
        val tippen = PendingIntent.getActivity(
            context,
            ziel?.assignmentId?.toInt() ?: 0,
            absicht,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val meldung = NotificationCompat.Builder(context, kanal)
            .setSmallIcon(R.drawable.ic_stat_dorf)
            .setContentTitle(titel)
            .setContentText(text)
            .setStyle(NotificationCompat.BigTextStyle().bigText(text))
            .setAutoCancel(true)
            .setPriority(
                if (kanal == PushKanal.ANFRAGEN) {
                    NotificationCompat.PRIORITY_HIGH
                } else {
                    NotificationCompat.PRIORITY_DEFAULT
                },
            )
            .setContentIntent(tippen)
            .build()
        // Alles zu einem Vorgang ersetzt sich gegenseitig statt sich zu
        // stapeln — wie im Backend (android.notification.tag).
        val kennung = ziel?.assignmentId?.toInt() ?: titel.hashCode()
        runCatching { NotificationManagerCompat.from(context).notify(kennung, meldung) }
            .onFailure { Log.w("Push", "Anzeige nicht möglich", it) }
    }

    private fun darfAnzeigen(context: Context): Boolean =
        Benachrichtigungserlaubnis.wirksam(context)
}

/**
 * Die beiden Kanäle. Getrennt, damit sich Hinweise leiser stellen lassen,
 * ohne die Anfragen mit stummzuschalten — bei einer Anfrage wartet jemand.
 */
object Kanaele {
    fun anlegen(context: Context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
        val manager = context.getSystemService(NotificationManager::class.java) ?: return
        manager.createNotificationChannel(
            NotificationChannel(
                PushKanal.ANFRAGEN,
                context.getString(R.string.push_channel_requests),
                NotificationManager.IMPORTANCE_HIGH,
            ).apply { description = context.getString(R.string.push_channel_requests_text) },
        )
        manager.createNotificationChannel(
            NotificationChannel(
                PushKanal.HINWEISE,
                context.getString(R.string.push_channel_hints),
                NotificationManager.IMPORTANCE_DEFAULT,
            ).apply { description = context.getString(R.string.push_channel_hints_text) },
        )
    }
}

/**
 * An- und Abmeldung des Geräts beim Backend.
 *
 * Die Kennung folgt der Erlaubnis: Sie entsteht erst, wenn Benachrichtigungen
 * wirklich erlaubt sind, und verschwindet wieder, sobald die Erlaubnis fehlt.
 * Die Entscheidung selbst steckt in [Geraeteabgleich] — hier hängen nur die
 * Android-Teile dran (Firebase, Einstellungen der App).
 */
object Geraeteanmeldung {
    private const val TAG = "Push"
    private const val DATEI = "push"
    private const val SCHLUESSEL = "kennung_angemeldet"

    /**
     * Der Abgleich bei jedem Start und bei jeder Rückkehr in den Vordergrund:
     * Erlaubnis erteilt → Kennung anlegen und hinterlegen; Erlaubnis fehlt →
     * eine vorhandene Kennung löschen (und ohne vorhandene gar nichts tun).
     */
    suspend fun abgleichen(context: Context) {
        abgleich(context).abgleichen(Benachrichtigungserlaubnis.wirksam(context))
    }

    /**
     * Beim Abmelden aus der App: erst dem Backend Bescheid geben, dann die
     * Kennung wegwerfen. Andernfalls schickt das Backend Anfragen an ein
     * Gerät, an dem niemand mehr angemeldet ist.
     */
    suspend fun abmelden(context: Context) {
        abgleich(context).abmelden()
    }

    private fun abgleich(context: Context) = Geraeteabgleich(
        speicher = speicher(context),
        geraete = context.appContainer.deviceRepository,
        kennung = { kennung() },
        kennungVerwerfen = { FirebaseMessaging.getInstance().deleteToken() },
        firebaseBereit = { bereit -> firebaseBereit(context, bereit) },
    )

    /**
     * Stellt das Firebase-SDK scharf oder still.
     *
     * Nötig, weil sich Firebase Cloud Messaging beim ersten Start sonst von
     * selbst bei Google anmeldet (Auto-Init) und dabei eine Kennung anlegt —
     * ohne Zutun der App und damit vor jeder Einwilligung. Im Manifest steht
     * beides deshalb auf `false`; hier wird es bei erteilter Erlaubnis
     * eingeschaltet und beim Entzug wieder aus. Firebase merkt sich die
     * Einstellung selbst, sie übersteht also den Neustart.
     */
    private fun firebaseBereit(context: Context, bereit: Boolean) {
        // Die Boolean?-Überladung ist die aktuelle; die auf Boolean gilt als
        // überholt. null hieße dort „zurück auf den Wert aus dem Manifest".
        val wert: Boolean? = bereit
        FirebaseApp.getInstance().setDataCollectionDefaultEnabled(wert)
        FirebaseMessaging.getInstance().isAutoInitEnabled = bereit
    }

    /**
     * Ob diese Installation angemeldet ist, steht in einer eigenen kleinen
     * Datei. Firebase danach zu fragen ginge nicht: Die Frage nach der
     * Kennung legt sie an — genau das, was ohne Erlaubnis nicht passieren soll.
     */
    fun speicher(context: Context): Anmeldespeicher = object : Anmeldespeicher {
        private val prefs =
            context.applicationContext.getSharedPreferences(DATEI, Context.MODE_PRIVATE)

        override suspend fun angemeldet(): Boolean = prefs.getBoolean(
            SCHLUESSEL,
            Anmeldevermutung.beiFehlenderMerkung(Anmeldevermutung.istAktualisierung(context)),
        )

        override suspend fun merken(wert: Boolean) {
            prefs.edit().putBoolean(SCHLUESSEL, wert).apply()
        }
    }

    private suspend fun kennung(): String? = suspendCancellableCoroutine { fortsetzung ->
        runCatching {
            FirebaseMessaging.getInstance().token
                .addOnSuccessListener { fortsetzung.resume(it) }
                .addOnFailureListener {
                    Log.w(TAG, "Keine Gerätekennung von Firebase", it)
                    fortsetzung.resume(null)
                }
        }.onFailure {
            Log.w(TAG, "Firebase steht nicht bereit", it)
            fortsetzung.resume(null)
        }
    }
}
