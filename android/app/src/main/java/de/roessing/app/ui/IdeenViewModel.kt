package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import de.roessing.app.data.IdeeInput
import de.roessing.app.data.IdeenAblehnungException
import de.roessing.app.data.IdeenRepository
import de.roessing.app.data.IdeenZuVieleException
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * Zustand des Ideen-Formulars. Der Wunsch ist Pflicht, Name und E-Mail sind
 * freiwillig und aus dem Profil vorbelegt.
 */
data class IdeenUiState(
    val wunsch: String = "",
    val name: String = "",
    val email: String = "",
    val sendet: Boolean = false,
    /** Klartext-Begründung, wenn die Einreichung abgelehnt wurde. */
    val fehler: String? = null,
) {
    /**
     * Der Knopf ist erst brauchbar, wenn wirklich etwas dasteht. Die
     * verbindliche Prüfung macht das Backend (5–2000 Zeichen) — hier geht es
     * nur darum, niemanden mit einer vermeidbaren Fehlermeldung zu ärgern.
     */
    val absendbar: Boolean get() = !sendet && wunsch.trim().length >= MIN_ZEICHEN

    companion object {
        const val MIN_ZEICHEN = 5
        const val MAX_ZEICHEN = 2000
    }
}

/** Einmalige Ereignisse des Ideen-Formulars. */
sealed interface IdeenEvent {
    data object Gesendet : IdeenEvent
}

class IdeenViewModel(private val repo: IdeenRepository) : ViewModel() {
    private val _state = MutableStateFlow(IdeenUiState())
    val state: StateFlow<IdeenUiState> = _state

    private val _events = MutableSharedFlow<IdeenEvent>(extraBufferCapacity = 4)
    val events: SharedFlow<IdeenEvent> = _events

    /**
     * Belegt Name und E-Mail aus dem Profil vor — aber nur, was noch leer
     * ist. Wer schon getippt hat, soll seine Eingabe nicht verlieren, bloß
     * weil das Profil eine Sekunde später geladen wurde.
     */
    fun vorbelegen(name: String, email: String) = _state.update {
        it.copy(
            name = it.name.ifBlank { name },
            email = it.email.ifBlank { email },
        )
    }

    fun setWunsch(v: String) = _state.update { it.copy(wunsch = v.take(IdeenUiState.MAX_ZEICHEN), fehler = null) }
    fun setName(v: String) = _state.update { it.copy(name = v, fehler = null) }
    fun setEmail(v: String) = _state.update { it.copy(email = v, fehler = null) }

    fun absenden() {
        val s = _state.value
        if (!s.absendbar) return
        viewModelScope.launch {
            _state.update { it.copy(sendet = true, fehler = null) }
            val eingabe = IdeeInput(
                wunsch = s.wunsch.trim(),
                name = s.name.trim(),
                email = s.email.trim(),
            )
            runCatching { repo.einreichen(eingabe) }
                .onSuccess {
                    // Das Wunschfeld wird frei für die nächste Idee; Name und
                    // E-Mail bleiben stehen, damit niemand sie erneut tippt.
                    _state.update { it.copy(sendet = false, fehler = null, wunsch = "") }
                    _events.emit(IdeenEvent.Gesendet)
                }
                .onFailure { fehler ->
                    // Der getippte Text bleibt in jedem Fall stehen.
                    _state.update { it.copy(sendet = false, fehler = fehlertext(fehler)) }
                }
        }
    }
}

private fun fehlertext(fehler: Throwable): String = when (fehler) {
    // Die Begründung des Backends ist genauer als alles, was die App raten
    // könnte — sie wird wörtlich übernommen.
    is IdeenAblehnungException -> fehler.grund
    is IdeenZuVieleException ->
        "Das waren gerade viele Ideen auf einmal. Bitte in einer Stunde noch einmal versuchen."
    else -> "Das Abschicken hat nicht geklappt. Besteht eine Verbindung?"
}
