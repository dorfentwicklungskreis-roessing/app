package de.roessing.app.data

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import retrofit2.HttpException

/**
 * Der Chat: in normalem Deutsch fragen, was im Dorf ansteht — und es tun.
 *
 * Der Schlüssel für Claude liegt im Backend und **nie** in der App. Ein
 * Schlüssel in einer ausgelieferten APK ist ein veröffentlichter Schlüssel;
 * wer sie entpackt, hat ihn. Die App spricht deshalb wie überall nur mit dem
 * Dorf-Backend, und von dort geht genau ein Weg nach draußen.
 *
 * Die App hält den Verlauf und schickt ihn bei jeder Frage mit — das Backend
 * führt keine Sitzung. Das hat einen angenehmen Nebeneffekt: Ein Neustart des
 * Servers unterbricht kein Gespräch.
 *
 * Was die Person sehen und tun darf, entscheidet ausschließlich das Backend.
 * Hier steht keine zweite Prüfung; eine Absage wird im Wortlaut angezeigt.
 */

/** Ein Zug des Gesprächs, wie ihn die Leitung trägt. */
@Serializable
data class ChatZugDto(
    /** [ROLLE_ICH] (die Person) oder [ROLLE_APP] (die Antwort des Chats). */
    val rolle: String,
    val text: String,
) {
    companion object {
        const val ROLLE_ICH = "ich"
        const val ROLLE_APP = "app"
    }
}

/** Eingabe von POST /api/v1/chat. */
@Serializable
data class ChatFrageInput(
    val frage: String,
    val verlauf: List<ChatZugDto> = emptyList(),
)

/** Antwort von POST /api/v1/chat. */
@Serializable
data class ChatAntwortDto(
    val antwort: String = "",
    /**
     * Die benutzten Werkzeuge in Aufrufreihenfolge. Sie stehen klein unter
     * der Antwort: Wer liest, dass „orte_liste" befragt wurde, weiß, dass die
     * Zahl aus dem Dorfserver kommt und nicht aus dem Gedächtnis eines
     * Modells.
     */
    val werkzeuge: List<String> = emptyList(),
    /** Die Rundengrenze war erreicht, bevor eine Antwort stand. */
    val abgebrochen: Boolean = false,
)

/** Antwort von GET /api/v1/chat. */
@Serializable
data class ChatStandDto(
    val verfuegbar: Boolean = false,
    /** Warum der Chat gerade nichts sagt. Für die Anzeige gedacht. */
    val hinweis: String = "",
)

/**
 * Das Backend hat die Frage abgewiesen und nennt den Grund im Klartext. Er
 * ist für die Person gedacht und wird wörtlich angezeigt — die App erfindet
 * dafür keinen eigenen Satz.
 *
 * [voruebergehend] trennt „gleich nochmal" (überlastet, zu langsam, zu viele
 * Fragen) von „so nicht" (zu lang, leer). Nur beim ersten lohnt ein zweiter
 * Versuch.
 */
class ChatAbsageException(
    val grund: String,
    val voruebergehend: Boolean,
) : RuntimeException(grund)

interface ChatRepository {
    /** Ob der Chat eingerichtet ist — und wenn nicht, warum. */
    suspend fun stand(): ChatStandDto

    /** Stellt eine Frage. Der Verlauf geht mit; das Backend hält keine Sitzung. */
    suspend fun fragen(frage: String, verlauf: List<ChatZugDto>): ChatAntwortDto
}

class ApiChatRepository(private val api: DorfApi) : ChatRepository {
    override suspend fun stand(): ChatStandDto = api.chatStand()

    override suspend fun fragen(frage: String, verlauf: List<ChatZugDto>): ChatAntwortDto =
        try {
            api.chatFragen(ChatFrageInput(frage = frage, verlauf = verlauf))
        } catch (e: HttpException) {
            when (e.code()) {
                400 -> throw ChatAbsageException(fehlertextAus(e), voruebergehend = false)
                // 429 zu viele Fragen, 503 nicht eingerichtet/überlastet/zu
                // langsam, 502 Störung der Gegenseite — alles drei geht von
                // selbst vorbei.
                429, 502, 503 -> throw ChatAbsageException(fehlertextAus(e), voruebergehend = true)
                else -> throw e
            }
        }

    private fun fehlertextAus(e: HttpException): String = runCatching {
        val roh = e.response()?.errorBody()?.string().orEmpty()
        Json { ignoreUnknownKeys = true }.decodeFromString<ApiErrorDto>(roh).error
    }.getOrNull()?.takeIf { it.isNotBlank() } ?: "Der Chat antwortet gerade nicht."
}

/**
 * Ein abgeschalteter Chat. Vorgabe für Oberflächen, die ohne ihn aufgebaut
 * werden (etwa Tests, die etwas ganz anderes prüfen) — er ruft nichts ab und
 * behauptet nichts, sondern sagt dasselbe wie ein Backend ohne Schlüssel.
 */
object KeinChat : ChatRepository {
    override suspend fun stand(): ChatStandDto =
        ChatStandDto(verfuegbar = false, hinweis = "Der Chat ist noch nicht eingerichtet.")

    override suspend fun fragen(frage: String, verlauf: List<ChatZugDto>): ChatAntwortDto =
        throw ChatAbsageException("Der Chat ist noch nicht eingerichtet.", voruebergehend = true)
}
