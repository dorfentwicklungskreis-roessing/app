package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import de.roessing.app.data.CompletionDto
import de.roessing.app.data.LatLon
import de.roessing.app.data.MeDto
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.PlaceSort
import de.roessing.app.data.PlacesRepository
import de.roessing.app.data.CompletionLockedException
import de.roessing.app.data.distanceMeters
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
    /**
     * Zuletzt bekannter eigener Standort. Bleibt auf dem Gerät und wird
     * nie ans Backend geschickt — er dient nur Karte und Entfernungen.
     */
    val userLocation: LatLon? = null,
    val sort: PlaceSort = PlaceSort.URGENCY,
)

/** Einmalige UI-Ereignisse (Snackbars). */
sealed interface UiEvent {
    data object CompletionSaved : UiEvent
    data object CompletionFailed : UiEvent

    /** Der Spielschutz hat die Meldung abgelehnt (HTTP 409). */
    data class CompletionLocked(val until: String?) : UiEvent
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
                            places = sortiert(resp.places, it.sort, it.userLocation),
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
                .onFailure { fehler ->
                    if (fehler is CompletionLockedException) {
                        _events.emit(UiEvent.CompletionLocked(fehler.retryAfter))
                        refresh()
                    } else {
                        _events.emit(UiEvent.CompletionFailed)
                    }
                }
            _state.update { it.copy(pendingTasks = it.pendingTasks - taskId) }
        }
    }

    /** Sortierung der Liste umschalten (Dringlichkeit oder Entfernung). */
    fun setSort(sort: PlaceSort) {
        _state.update { it.copy(sort = sort, places = sortiert(it.places, sort, it.userLocation)) }
    }

    /** Neuen Standort übernehmen (oder null, wenn keiner bekannt ist). */
    fun setUserLocation(location: LatLon?) {
        _state.update {
            it.copy(userLocation = location, places = sortiert(it.places, it.sort, location))
        }
    }

    /**
     * Nach Entfernung nur, wenn ein Standort bekannt ist — sonst bleibt es
     * bei der Dringlichkeit (rot vor gelb vor grün).
     */
    private fun sortiert(places: List<PlaceDto>, sort: PlaceSort, user: LatLon?): List<PlaceDto> =
        if (sort == PlaceSort.DISTANCE && user != null) {
            places.sortedWith(
                compareBy<PlaceDto> { p -> distanceMeters(user, LatLon(p.lat, p.lon)) }
                    .thenBy { p -> p.name },
            )
        } else {
            places.sortedWith(
                compareByDescending<PlaceDto> { p -> p.careStatus.ordinal }.thenBy { p -> p.name },
            )
        }

    fun loadHistory(taskId: Long) {
        viewModelScope.launch {
            runCatching { repo.completions(taskId) }.onSuccess { list ->
                _history.update { it + (taskId to list) }
            }
        }
    }
}
