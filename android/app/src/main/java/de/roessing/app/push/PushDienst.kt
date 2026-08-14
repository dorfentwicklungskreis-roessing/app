package de.roessing.app.push

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.util.Log
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
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
        Log.i(TAG, "Neue Gerätekennung von Firebase")
        CoroutineScope(SupervisorJob() + Dispatchers.IO).launch {
            runCatching { applicationContext.appContainer.deviceRepository.register(token) }
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
        Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) ==
            PackageManager.PERMISSION_GRANTED
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
 * Angemeldet wird bei jedem Start der angemeldeten App: Das Backend legt
 * dieselbe Kennung nicht doppelt an, und so bleibt der Zeitstempel frisch.
 */
object Geraeteanmeldung {
    private const val TAG = "Push"

    suspend fun anmelden(context: Context) {
        val token = kennung() ?: return
        runCatching { context.appContainer.deviceRepository.register(token) }
            .onSuccess { Log.i(TAG, "Gerät für Benachrichtigungen angemeldet") }
            .onFailure { Log.w(TAG, "Anmelden der Kennung fehlgeschlagen", it) }
    }

    /**
     * Beim Abmelden aus der App: erst dem Backend Bescheid geben, dann die
     * Kennung wegwerfen. Andernfalls schickt das Backend Anfragen an ein
     * Gerät, an dem niemand mehr angemeldet ist.
     */
    suspend fun abmelden(context: Context) {
        val token = kennung() ?: return
        runCatching { context.appContainer.deviceRepository.unregister(token) }
            .onFailure { Log.w(TAG, "Abmelden der Kennung fehlgeschlagen", it) }
        runCatching { FirebaseMessaging.getInstance().deleteToken() }
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
