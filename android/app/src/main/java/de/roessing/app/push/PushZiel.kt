package de.roessing.app.push

/**
 * Die Kanäle der Systembenachrichtigungen. Getrennt, weil sie
 * Verschiedenes wollen: Eine **Anfrage** möchte eine Antwort und darf
 * deshalb auffallen; ein **Hinweis** berichtet nur („schon erledigt",
 * „Zusage abgelaufen"). Wer die Hinweise leiser stellt, soll trotzdem
 * gefragt werden können — deshalb zwei Kanäle statt einem.
 */
object PushKanal {
    const val ANFRAGEN = "anfragen"
    const val HINWEISE = "hinweise"
}

/**
 * Wohin ein Fingertipp auf eine Push-Nachricht führt.
 *
 * Der Datenteil kommt aus dem Backend (internal/push/fcm.go) und besteht
 * ausschließlich aus Zeichenketten — mehr lässt Firebase Cloud Messaging im
 * `data`-Teil nicht zu. Dieselbe Form reist auch durch den Intent zur
 * MainActivity, deshalb gibt es den Weg zurück (alsDaten) gleich mit.
 */
data class PushZiel(
    val placeId: Long,
    val taskId: Long,
    val assignmentId: Long,
    val notificationId: Long,
    val kind: String,
    val taskKind: String = "",
    val ortsname: String = "",
    val aufgabe: String = "",
    val titel: String = "",
    val text: String = "",
) {
    /** Anfragen wollen eine Antwort; alles andere ist ein Hinweis. */
    val istAnfrage: Boolean get() = kind == "anfrage" || kind == "rundruf"

    val kanal: String get() = if (istAnfrage) PushKanal.ANFRAGEN else PushKanal.HINWEISE

    fun alsDaten(): Map<String, String> = mapOf(
        "placeId" to placeId.toString(),
        "taskId" to taskId.toString(),
        "assignmentId" to assignmentId.toString(),
        "notificationId" to notificationId.toString(),
        "kind" to kind,
        "taskKind" to taskKind,
        "placeName" to ortsname,
        "taskName" to aufgabe,
        "title" to titel,
        "body" to text,
    )

    companion object {
        /**
         * Liest das Ziel aus dem Datenteil. Ohne brauchbare Orts-Kennung gibt
         * es nichts anzuspringen — dann bleibt es bei der bloßen Anzeige.
         */
        fun ausDaten(daten: Map<String, String>): PushZiel? {
            val placeId = daten["placeId"]?.toLongOrNull() ?: return null
            if (placeId <= 0) return null
            return PushZiel(
                placeId = placeId,
                taskId = daten["taskId"]?.toLongOrNull() ?: 0,
                assignmentId = daten["assignmentId"]?.toLongOrNull() ?: 0,
                notificationId = daten["notificationId"]?.toLongOrNull() ?: 0,
                kind = daten["kind"].orEmpty(),
                taskKind = daten["taskKind"].orEmpty(),
                ortsname = daten["placeName"].orEmpty(),
                aufgabe = daten["taskName"].orEmpty(),
                titel = daten["title"].orEmpty(),
                text = daten["body"].orEmpty(),
            )
        }
    }
}
