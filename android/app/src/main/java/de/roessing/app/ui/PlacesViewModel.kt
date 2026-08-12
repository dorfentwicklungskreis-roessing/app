package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import de.roessing.app.data.CompletionDto
import de.roessing.app.data.MeDto
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.PlacesRepository
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

data class PlacesUiState(
    val loading: Boolean = false,
    val places: List<PlaceDto> = emptyList(),
    val wateringFactor: Double = 1.0,
    val me: MeDto? = null,
    /** true, wenn der letzte Refresh fehlschlug (alte Daten werden weiter angezeigt). */
    val offline: Boolean = false,
    /** Task-IDs, für die gerade eine Meldung läuft (Buttons deaktivieren). */
    val pendingTasks: Set<Long> = emptySet(),
)

/** Einmalige UI-Ereignisse (Snackbars). */
sealed interface UiEvent {
    data object CompletionSaved : UiEvent
    data object CompletionFailed : UiEvent
}

class PlacesViewModel(private val repo: PlacesRepository) : ViewModel() {
    private val _state = MutableStateFlow(PlacesUiState(loading = true))
    val state: StateFlow<PlacesUiState> = _state

    private val _events = MutableSharedFlow<UiEvent>(extraBufferCapacity = 8)
    val events: SharedFlow<UiEvent> = _events

    /** Detail-Historie je Aufgabe, on demand geladen. */
    private val _history = MutableStateFlow<Map<Long, List<CompletionDto>>>(emptyMap())
    val history: StateFlow<Map<Long, List<CompletionDto>>> = _history

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(loading = true) }
            val me = runCatching { repo.me() }.getOrNull()
            runCatching { repo.places() }
                .onSuccess { resp ->
                    _state.update {
                        it.copy(
                            loading = false, offline = false,
                            places = resp.places.sortedWith(
                                compareByDescending<PlaceDto> { p -> p.careStatus.ordinal }
                                    .thenBy { p -> p.name },
                            ),
                            wateringFactor = resp.wateringFactor,
                            me = me ?: it.me,
                        )
                    }
                }
                .onFailure {
                    _state.update { it.copy(loading = false, offline = true) }
                }
        }
    }

    /** Meldet eine Aufgabe als erledigt und lädt danach neu. */
    fun complete(taskId: Long, liters: Double?) {
        viewModelScope.launch {
            _state.update { it.copy(pendingTasks = it.pendingTasks + taskId) }
            runCatching { repo.complete(taskId, liters) }
                .onSuccess {
                    _events.emit(UiEvent.CompletionSaved)
                    loadHistory(taskId)
                    refresh()
                }
                .onFailure { _events.emit(UiEvent.CompletionFailed) }
            _state.update { it.copy(pendingTasks = it.pendingTasks - taskId) }
        }
    }

    fun loadHistory(taskId: Long) {
        viewModelScope.launch {
            runCatching { repo.completions(taskId) }.onSuccess { list ->
                _history.update { it + (taskId to list) }
            }
        }
    }
}
