package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import de.roessing.app.data.AssignmentTakenException
import de.roessing.app.data.CareStatus
import de.roessing.app.data.CompletionDto
import de.roessing.app.data.CompletionLockedException
import de.roessing.app.data.LatLon
import de.roessing.app.data.MeDto
import de.roessing.app.data.NotificationDto
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.PlaceSort
import de.roessing.app.data.PlacesRepository
import de.roessing.app.data.VergabeRepository
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
    /**
     * Offene Benachrichtigungen der Vergabe: Anfragen („du bist dran") und
     * Hinweise. Sie kommen aus der Abrufliste des Backends — auch dann, wenn
     * die Push-Nachricht nie ankam oder abgelehnt wurde.
     */
    val notifications: List<NotificationDto> = emptyList(),
    /** Orts-IDs, für die gerade ein Ein- oder Austragen läuft. */
    val pendingSignups: Set<Long> = emptySet(),
    /** Vorgangs-IDs, für die gerade eine Zusage oder Rückgabe läuft. */
    val pendingAssignments: Set<Long> = emptySet(),
) {
    /**
     * Zahl der Orte, die gerade Aufmerksamkeit brauchen (gelb oder rot).
     * Die Startseite macht daraus ihre Statuszeile; abgeschaltete Orte
     * zählen nicht, sie sind niemandes Aufgabe.
     */
    val faelligeOrte: Int
        get() = places.count { it.active && it.careStatus != CareStatus.green }

    /** Anfragen, auf die man zusagen kann — ohne die reinen Hinweise. */
    val offeneAnfragen: Int
        get() = notifications.count { it.istAnfrage }
}

/** Einmalige UI-Ereignisse (Snackbars). */
sealed interface UiEvent {
    data object CompletionSaved : UiEvent
    data object CompletionFailed : UiEvent

    /** Der Spielschutz hat die Meldung abgelehnt (HTTP 409). */
    data class CompletionLocked(val until: String?) : UiEvent

    /** Die Zusage hat geklappt — die Aufgabe gehört jetzt mir. */
    data object AssignmentClaimed : UiEvent

    /** Jemand anderes war schneller (HTTP 409); grund kommt vom Backend. */
    data class AssignmentTaken(val grund: String) : UiEvent

    data object AssignmentReleased : UiEvent

    /** Ein- oder ausgetragen als Helfer:in. */
    data class SignupChanged(val an: Boolean) : UiEvent

    /** Ein Aufruf der Vergabe ist am Netz gescheitert. */
    data object VergabeFailed : UiEvent
}

class PlacesViewModel(
    private val repo: PlacesRepository,
    private val vergabe: VergabeRepository,
) : ViewModel() {
    private val _state = MutableStateFlow(PlacesUiState(loading = true))
    val state: StateFlow<PlacesUiState> = _state

    private val _events = MutableSharedFlow<UiEvent>(extraBufferCapacity = 8)
    val events: SharedFlow<UiEvent> = _events

    /** Detail-Historie je Aufgabe, on demand geladen. */
    private val _history = MutableStateFlow<Map<Long, List<CompletionDto>>>(emptyMap())
    val history: StateFlow<Map<Long, List<CompletionDto>>> = _history

    init {
        refresh()
        loadNotifications()
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

    /**
     * Holt die offenen Benachrichtigungen. Das ist die Rückfallebene, die
     * immer funktioniert: Wer keine Push-Erlaubnis gegeben hat oder dessen
     * Nachricht unterwegs verlorenging, sieht seine Anfragen trotzdem hier.
     *
     * Ein Netzfehler lässt den bekannten Stand stehen, statt die Liste zu
     * leeren — sonst verschwände eine Anfrage bei jedem Funkloch.
     */
    fun loadNotifications() {
        viewModelScope.launch {
            runCatching { vergabe.notifications() }.onSuccess { liste ->
                _state.update { it.copy(notifications = liste) }
            }
        }
    }

    /**
     * Bestätigt den Empfang. Hinweise verschwinden damit; Anfragen bleiben
     * stehen, bis der Vorgang sie schließt — sonst wäre die Aufgabe aus der
     * App verschwunden, bevor jemand zugesagt hat.
     */
    fun acknowledge(id: Long) {
        viewModelScope.launch {
            runCatching { vergabe.ack(id) }
                .onSuccess { loadNotifications() }
                .onFailure { _events.emit(UiEvent.VergabeFailed) }
        }
    }

    /**
     * Trägt mich als Helfer:in für einen Ort ein oder wieder aus.
     * taskKind schränkt auf eine Aufgabenart ein (null = alle Aufgaben).
     */
    fun setSignup(placeId: Long, taskKind: String?, an: Boolean) {
        viewModelScope.launch {
            _state.update { it.copy(pendingSignups = it.pendingSignups + placeId) }
            runCatching {
                if (an) vergabe.signup(placeId, taskKind) else vergabe.signoff(placeId, taskKind)
            }
                .onSuccess {
                    _events.emit(UiEvent.SignupChanged(an))
                    refresh()
                }
                .onFailure { _events.emit(UiEvent.VergabeFailed) }
            _state.update { it.copy(pendingSignups = it.pendingSignups - placeId) }
        }
    }

    /** Sagt zu. 409 heißt: jemand anderes war schneller. */
    fun claim(assignmentId: Long) {
        viewModelScope.launch {
            _state.update { it.copy(pendingAssignments = it.pendingAssignments + assignmentId) }
            runCatching { vergabe.claim(assignmentId) }
                .onSuccess {
                    _events.emit(UiEvent.AssignmentClaimed)
                    refresh()
                    loadNotifications()
                }
                .onFailure { fehler ->
                    if (fehler is AssignmentTakenException) {
                        _events.emit(UiEvent.AssignmentTaken(fehler.grund))
                        // Der Stand ist überholt — neu laden, damit die
                        // Oberfläche zeigt, wer die Aufgabe jetzt hat.
                        refresh()
                        loadNotifications()
                    } else {
                        _events.emit(UiEvent.VergabeFailed)
                    }
                }
            _state.update { it.copy(pendingAssignments = it.pendingAssignments - assignmentId) }
        }
    }

    /** Gibt die eigene Zusage zurück; die Warteschlange läuft dann weiter. */
    fun release(assignmentId: Long) {
        viewModelScope.launch {
            _state.update { it.copy(pendingAssignments = it.pendingAssignments + assignmentId) }
            runCatching { vergabe.release(assignmentId) }
                .onSuccess {
                    _events.emit(UiEvent.AssignmentReleased)
                    refresh()
                    loadNotifications()
                }
                .onFailure { _events.emit(UiEvent.VergabeFailed) }
            _state.update { it.copy(pendingAssignments = it.pendingAssignments - assignmentId) }
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
                    // Jede Erledigung beendet den Vorgang — die Anfrage dazu
                    // ist damit hinfällig und darf nicht stehen bleiben.
                    loadNotifications()
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
