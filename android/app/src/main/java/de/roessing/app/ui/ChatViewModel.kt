package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import de.roessing.app.data.ChatAbsageException
import de.roessing.app.data.ChatRepository
import de.roessing.app.data.ChatZugDto
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Wer gerade spricht. */
enum class ChatRolle { ICH, APP }

/**
 * Ein Zug des Gesprächs, wie ihn die Oberfläche zeigt.
 *
 * [werkzeuge] steht klein unter einer Antwort der App. Das ist kein Beiwerk:
 * Wer liest, dass „orte_liste" befragt wurde, weiß, dass die Zahl aus dem
 * Dorfserver kommt und nicht aus dem Gedächtnis eines Sprachmodells.
 */
data class ChatZug(
    val rolle: ChatRolle,
    val text: String,
    val werkzeuge: List<String> = emptyList(),
    /** Die Antwort blieb unfertig, weil die Rundengrenze erreicht war. */
    val abgebrochen: Boolean = false,
)

/**
 * Zustand des Chats.
 *
 * [verfuegbar] wird beim Öffnen einmal beim Backend erfragt. Ist der Chat
 * nicht eingerichtet (der Betreiber hat noch keinen Schlüssel hinterlegt),
 * sagt [hinweis], was los ist — und es lässt sich gar nicht erst tippen.
 * Jemanden eine Frage schreiben zu lassen, die sicher ins Leere geht, wäre
 * die unfreundlichere Fassung derselben Nachricht.
 */
data class ChatUiState(
    val zuege: List<ChatZug> = emptyList(),
    val eingabe: String = "",
    /** Die Antwort ist unterwegs. Sie darf spürbar lange dauern. */
    val wartet: Boolean = false,
    /** Klartext des Backends, wenn eine Frage nicht durchkam. */
    val fehler: String? = null,
    /** Der Stand wurde noch nicht abgefragt — es ist noch nichts zu sagen. */
    val laedtStand: Boolean = true,
    val verfuegbar: Boolean = false,
    val hinweis: String = "",
) {
    val absendbar: Boolean
        get() = verfuegbar && !wartet && eingabe.trim().isNotEmpty()

    companion object {
        /** Dieselbe Grenze wie im Backend — hier nur, um früher Bescheid zu sagen. */
        const val MAX_ZEICHEN = 2000
    }
}

/**
 * Der Chat-Bereich.
 *
 * Der Verlauf lebt hier und wird bei jeder Frage mitgeschickt: Das Backend
 * führt keine Sitzung, damit ein Neustart des Servers kein Gespräch
 * unterbricht. Weil das ViewModel das Drehen des Geräts überlebt, überlebt es
 * der Verlauf mit.
 */
class ChatViewModel(private val repo: ChatRepository) : ViewModel() {
    private val _state = MutableStateFlow(ChatUiState())
    val state: StateFlow<ChatUiState> = _state

    /**
     * Fragt einmal nach, ob der Chat überhaupt eingerichtet ist.
     *
     * Scheitert schon diese Frage (kein Netz), gilt der Chat als verfügbar:
     * Dann sagt der erste Versuch die Wahrheit, statt dass ein Ausfall der
     * Leitung wie eine dauerhafte Abschaltung aussieht.
     */
    fun standPruefen() {
        if (!_state.value.laedtStand) return
        viewModelScope.launch {
            runCatching { repo.stand() }
                .onSuccess { stand ->
                    _state.update {
                        it.copy(
                            laedtStand = false,
                            verfuegbar = stand.verfuegbar,
                            hinweis = stand.hinweis,
                        )
                    }
                }
                .onFailure {
                    _state.update { it.copy(laedtStand = false, verfuegbar = true, hinweis = "") }
                }
        }
    }

    fun setEingabe(v: String) = _state.update {
        it.copy(eingabe = v.take(ChatUiState.MAX_ZEICHEN), fehler = null)
    }

    fun absenden() {
        val vorher = _state.value
        if (!vorher.absendbar) return
        val frage = vorher.eingabe.trim()
        // Der Verlauf sind die abgeschlossenen Züge; die neue Frage geht
        // getrennt hinaus.
        val verlauf = vorher.zuege.map {
            ChatZugDto(
                rolle = if (it.rolle == ChatRolle.ICH) ChatZugDto.ROLLE_ICH else ChatZugDto.ROLLE_APP,
                text = it.text,
            )
        }
        // Die Frage steht sofort im Gespräch — sie soll nicht erst nach einer
        // halben Minute auftauchen.
        _state.update {
            it.copy(
                zuege = it.zuege + ChatZug(ChatRolle.ICH, frage),
                eingabe = "",
                wartet = true,
                fehler = null,
            )
        }
        viewModelScope.launch {
            runCatching { repo.fragen(frage, verlauf) }
                .onSuccess { antwort ->
                    _state.update {
                        it.copy(
                            zuege = it.zuege + ChatZug(
                                rolle = ChatRolle.APP,
                                text = antwort.antwort,
                                werkzeuge = antwort.werkzeuge,
                                abgebrochen = antwort.abgebrochen,
                            ),
                            wartet = false,
                            fehler = null,
                        )
                    }
                }
                .onFailure { fehler ->
                    // Die Frage wandert zurück ins Eingabefeld. Ein zweiter
                    // Versuch ist dann ein Tipp und kein Abtippen — und im
                    // Gespräch bleibt keine Frage stehen, auf die nie eine
                    // Antwort kam.
                    _state.update {
                        it.copy(
                            zuege = it.zuege.dropLast(1),
                            eingabe = frage,
                            wartet = false,
                            fehler = fehlertext(fehler),
                        )
                    }
                }
        }
    }

    /** Das Gespräch von vorn — der bisherige Verlauf kostet bei jeder Frage mit. */
    fun neuBeginnen() = _state.update {
        it.copy(zuege = emptyList(), eingabe = "", fehler = null)
    }
}

private fun fehlertext(fehler: Throwable): String = when (fehler) {
    // Der Satz des Backends ist genauer als alles, was die App raten könnte —
    // und bei einer Absage wegen fehlender Berechtigung ist er der
    // eigentliche Inhalt. Er wird wörtlich übernommen.
    is ChatAbsageException -> fehler.grund
    else -> "Das hat nicht geklappt. Besteht eine Verbindung?"
}
