package de.roessing.app.errors

import android.content.Context
import java.io.PrintWriter
import java.io.StringWriter

/**
 * Merkt sich einen Absturz und legt ihn beim nächsten Start vor.
 *
 * Der Anlass ist konkret: Ein Absturz fiel nur auf, weil zufällig jemand auf
 * eine Meldung getippt hat und das Protokoll von Hand geholt wurde. Play
 * sammelt zwar Abstürze — aber nur von Geräten, deren Besitzer das Google
 * gegenüber erlaubt haben, verzögert und ohne den Satz, den die Person
 * dazuschreiben würde. Also merkt es sich die App selbst.
 *
 * Gemerkt wird, **verschickt wird nicht**: Beim nächsten Start steht der
 * Hinweis da, und ob ein Bericht hinausgeht, entscheidet die Person.
 *
 * Anders als auf iOS reicht hier ein Handler: `Thread`s
 * `UncaughtExceptionHandler` bekommt praktisch jeden Kotlin- und Java-Fehler
 * samt Aufrufliste. Der bisherige Handler wird danach trotzdem aufgerufen —
 * ohne ihn stürbe der Prozess anders als sonst, und Play sähe den Absturz
 * nicht mehr.
 */
object CrashWatch {
    private const val SCHLUESSEL_TEXT = "letzterAbsturz"
    private const val SCHLUESSEL_ZEIT = "letzterAbsturzZeit"

    /** Höchstens so viel Aufrufliste — die ersten Zeilen sagen das Meiste. */
    private const val MAX_ZEICHEN = 4000

    /**
     * Hängt den Handler ein. Einmal beim Start der Anwendung.
     */
    fun install(context: Context, uhr: () -> Long = { System.currentTimeMillis() }) {
        val speicher = SharedPrefsCrashStore(context)
        val vorher = Thread.getDefaultUncaughtExceptionHandler()
        Thread.setDefaultUncaughtExceptionHandler { thread, fehler ->
            runCatching { record(speicher, fehler, uhr()) }
            // Der bisherige Handler bekommt seinen Lauf: Sonst beendete sich
            // der Prozess anders als üblich, und Play sähe den Absturz nicht.
            vorher?.uncaughtException(thread, fehler)
        }
    }

    /** Schreibt weg, was gerade passiert ist. Der Prozess stirbt gleich. */
    fun record(speicher: CrashStore, fehler: Throwable, jetzt: Long) {
        speicher.schreiben(SCHLUESSEL_TEXT, aufrufliste(fehler).take(MAX_ZEICHEN))
        speicher.schreiben(SCHLUESSEL_ZEIT, jetzt.toString())
    }

    /**
     * Liest, was der letzte Lauf hinterlassen hat — und räumt es weg, damit
     * derselbe Absturz nicht zweimal angeboten wird.
     */
    fun pendingCrash(speicher: CrashStore, jetzt: Long, meldung: String): ErrorIncident? {
        val text = speicher.lesen(SCHLUESSEL_TEXT)?.trim().orEmpty()
        val zeit = speicher.lesen(SCHLUESSEL_ZEIT)?.toLongOrNull()
        speicher.loeschen(SCHLUESSEL_TEXT)
        speicher.loeschen(SCHLUESSEL_ZEIT)
        if (text.isEmpty()) return null
        return ErrorIncident(
            kind = ErrorReportKind.CRASH,
            message = meldung,
            detail = text,
            area = "Absturz",
            // Nicht später als jetzt: Eine falsch gestellte Uhr soll den
            // Bericht nicht in die Zukunft schieben.
            occurredAt = minOf(zeit ?: jetzt, jetzt),
        )
    }

    private fun aufrufliste(fehler: Throwable): String {
        val schreiber = StringWriter()
        fehler.printStackTrace(PrintWriter(schreiber))
        return schreiber.toString()
    }
}

/**
 * Wo der Absturz bis zum nächsten Start liegt.
 *
 * Als Schnittstelle, damit sich das Verhalten ohne Android-Laufzeit prüfen
 * lässt — dieselbe Trennung wie beim `Anmeldespeicher` der Benachrichtigungen.
 */
interface CrashStore {
    fun lesen(schluessel: String): String?
    fun schreiben(schluessel: String, wert: String)
    fun loeschen(schluessel: String)
}

/**
 * Die echte Ablage. `commit()` statt `apply()` mit Absicht: Beim Schreiben
 * aus dem Absturz-Handler ist der Prozess gleich weg, und ein
 * asynchrones Schreiben käme nicht mehr an.
 */
class SharedPrefsCrashStore(context: Context) : CrashStore {
    private val prefs =
        context.applicationContext.getSharedPreferences("fehlerberichte", Context.MODE_PRIVATE)

    override fun lesen(schluessel: String): String? = prefs.getString(schluessel, null)

    override fun schreiben(schluessel: String, wert: String) {
        prefs.edit().putString(schluessel, wert).commit()
    }

    override fun loeschen(schluessel: String) {
        prefs.edit().remove(schluessel).commit()
    }
}
