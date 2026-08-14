package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import de.roessing.app.data.Termin
import de.roessing.app.data.VeranstaltungenRepository
import de.roessing.app.data.alsTermine
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.Instant

/**
 * Zustand der Ansicht „Was ist los in Rössing".
 *
 * `fehler` heißt nicht „nichts da", sondern „gerade nicht erreichbar": Steht
 * noch eine ältere Liste, bleibt sie stehen und bekommt einen Hinweis
 * darüber. Eine leere Seite ohne Erklärung wäre das schlechteste Ergebnis.
 */
data class VeranstaltungenUiState(
    val termine: List<Termin> = emptyList(),
    val laedt: Boolean = false,
    val fehler: Boolean = false,
) {
    /** Nur Termine mit einer echten Stelle taugen für die Dorfkarte. */
    val kartenpunkte: List<Termin> get() = termine.filter { it.koordinate != null }

    /** Nichts da, nichts unterwegs, nichts kaputt — dann ist wirklich nichts los. */
    val leer: Boolean get() = termine.isEmpty() && !laedt && !fehler
}

/**
 * Holt die Termine von der Website (`/events.json`). Es wird nichts
 * geschrieben und nichts gemeldet — die App zeigt hier nur an.
 *
 * Kein Push für Termine: Erinnerungen an Veranstaltungen wären ein eigenes
 * Thema mit eigener Einwilligung.
 */
class VeranstaltungenViewModel(
    private val repo: VeranstaltungenRepository,
    /** Die Uhr als Parameter — sonst altern die Tests mit dem Kalender. */
    private val uhr: () -> Instant = Instant::now,
) : ViewModel() {
    private val _state = MutableStateFlow(VeranstaltungenUiState())
    val state: StateFlow<VeranstaltungenUiState> = _state

    private var geholt = false

    /**
     * Beim Öffnen des Bereichs. Was schon da ist, wird nicht noch einmal
     * geholt — nur neu gesiebt, damit ein Termin, der während der laufenden
     * Sitzung vorbeigegangen ist, verschwindet.
     */
    fun laden() {
        if (geholt) {
            val jetzt = uhr()
            _state.update { zustand ->
                zustand.copy(termine = zustand.termine.filterNot { it.istVorbei(jetzt) })
            }
            return
        }
        holen()
    }

    /** Bewusstes Aktualisieren durch die Nutzerin. */
    fun aktualisieren() = holen()

    private fun holen() {
        if (_state.value.laedt) return
        viewModelScope.launch {
            _state.update { it.copy(laedt = true) }
            runCatching { repo.kommende() }
                .onSuccess { liste ->
                    geholt = true
                    _state.update {
                        it.copy(
                            laedt = false,
                            fehler = false,
                            termine = liste.alsTermine(uhr()),
                        )
                    }
                }
                .onFailure {
                    // Die alte Liste bleibt stehen; der Hinweis kommt darüber.
                    _state.update { it.copy(laedt = false, fehler = true) }
                }
        }
    }
}
