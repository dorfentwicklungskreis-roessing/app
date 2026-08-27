package de.roessing.app.errors

import java.util.Locale

/**
 * Was für eine Störung es war. Dieselben vier Werte, die das Backend kennt
 * (`model.ErrorReportKind`) — die App erfindet keinen fünften.
 */
enum class ErrorReportKind(val wire: String) {
    CRASH("crash"),
    NETWORK("network"),
    SERVER("server"),
    UNEXPECTED("unexpected"),
}

/**
 * Eine Sache, die schiefgegangen ist — so, wie die Person sie erlebt hat.
 *
 * [message] ist der Satz, der auf dem Schirm stand, in verständlichem
 * Deutsch. Er kommt aus den Ressourcen bzw. — wo das Backend etwas gesagt
 * hat — im Wortlaut von dort. Die App denkt sich für den Bericht keinen
 * zweiten aus.
 */
data class ErrorIncident(
    val kind: ErrorReportKind,
    val message: String,
    val detail: String = "",
    val area: String = "",
    val occurredAt: Long = System.currentTimeMillis(),
)

/**
 * Übersetzt einen Anfragepfad in den Teil der App, den jemand gerade vor
 * sich hatte. „api/v1/places" sagt dem Dorfentwicklungskreis nichts,
 * „Mithelfen" sagt, wo er nachsehen muss.
 *
 * Wortgleich mit `Bereichsnamen` der iOS-App: Ein Bericht von einem iPhone
 * und einer vom Android-Telefon sollen in derselben Liste dasselbe heißen.
 */
object AreaNames {
    /** Der längste passende Anfang gewinnt, damit `me/devices` nicht `me` ist. */
    private val zuordnung = listOf(
        "api/v1/me/notifications" to "Anfragen und Hinweise",
        "api/v1/me/devices" to "Benachrichtigungen",
        "api/v1/me/profile" to "Mein Profil",
        "api/v1/me" to "Konto",
        "api/v1/members" to "Dorfbewohner",
        "api/v1/places" to "Mithelfen",
        "api/v1/tasks" to "Mithelfen",
        "api/v1/completions" to "Mithelfen",
        "api/v1/assignments" to "Anfragen und Hinweise",
        "api/v1/stats/leaderboard" to "Rangliste",
        "api/v1/ideen" to "Idee vorschlagen",
        "api/v1/traeger" to "Träger",
        "api/v1/settings" to "Einstellungen",
    )

    fun of(pfad: String): String {
        val sauber = pfad.removePrefix("/")
        return zuordnung
            .filter { sauber.startsWith(it.first) }
            .maxByOrNull { it.first.length }
            ?.second
            ?: "App"
    }
}

/**
 * Was die App über sich selbst weiß: Version, System und Gerätetyp.
 *
 * Bewusst der Gerätetyp und keine Gerätekennung: Das Modell hilft beim
 * Nachstellen, eine Kennung verfolgte nur eine Person.
 */
data class DeviceFacts(
    val appVersion: String,
    val osVersion: String,
    val deviceModel: String,
) {
    companion object {
        fun of(versionName: String, versionCode: Int, sdk: Int, release: String,
               manufacturer: String, model: String): DeviceFacts = DeviceFacts(
            appVersion = "$versionName ($versionCode)",
            osVersion = "Android $release (API $sdk)",
            deviceModel = geraetename(manufacturer, model),
        )

        /**
         * „Google Pixel 6" statt „google Pixel 6" — und ohne den Hersteller
         * doppelt, wenn er schon im Modellnamen steht.
         */
        private fun geraetename(hersteller: String, modell: String): String {
            val h = hersteller.trim()
            val m = modell.trim()
            if (h.isEmpty()) return m
            if (m.lowercase(Locale.ROOT).startsWith(h.lowercase(Locale.ROOT))) return m
            return "${h.replaceFirstChar { it.uppercase(Locale.ROOT) }} $m".trim()
        }
    }
}
