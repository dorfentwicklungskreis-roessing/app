package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import de.roessing.app.data.LeaderboardEntryDto
import de.roessing.app.data.LeaderboardTotalsDto
import de.roessing.app.data.StatsRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/** Auswertungszeitraum der Rangliste. wert geht 1:1 an das Backend. */
enum class LeaderboardPeriod(val wert: String) {
    WOCHE("woche"),
    MONAT("monat"),
    SAISON("saison"),
    JAHR("jahr"),
    GESAMT("gesamt"),
}

data class LeaderboardUiState(
    val loading: Boolean = false,
    val period: LeaderboardPeriod = LeaderboardPeriod.SAISON,
    val entries: List<LeaderboardEntryDto> = emptyList(),
    val totals: LeaderboardTotalsDto = LeaderboardTotalsDto(),
    /** Der eigene Eintrag — auch außerhalb der sichtbaren Plätze. */
    val me: LeaderboardEntryDto? = null,
    /** true, wenn der letzte Abruf fehlschlug (alte Daten bleiben stehen). */
    val offline: Boolean = false,
) {
    /** Die ersten drei Plätze — sie kommen aufs Podest. */
    val podest: List<LeaderboardEntryDto> get() = entries.take(3)
}

/**
 * Rangliste des Mithelfens. Die Auswertung macht komplett das Backend
 * (Zeiträume, Sortierung, Auszeichnungen) — hier wird nur geladen und
 * der gewählte Zeitraum gehalten.
 */
class LeaderboardViewModel(private val repo: StatsRepository) : ViewModel() {
    private val _state = MutableStateFlow(LeaderboardUiState(loading = true))
    val state: StateFlow<LeaderboardUiState> = _state

    init {
        refresh()
    }

    /** Wechselt den Zeitraum. Derselbe Zeitraum löst keine neue Abfrage aus. */
    fun select(period: LeaderboardPeriod) {
        if (period == _state.value.period) return
        _state.update { it.copy(period = period) }
        refresh()
    }

    fun refresh() {
        val period = _state.value.period
        viewModelScope.launch {
            _state.update { it.copy(loading = true) }
            runCatching { repo.leaderboard(period.wert) }
                .onSuccess { antwort ->
                    _state.update {
                        it.copy(
                            loading = false, offline = false,
                            entries = antwort.entries,
                            totals = antwort.totals,
                            me = antwort.me,
                        )
                    }
                }
                // Bei Netzfehlern bleiben die zuletzt geladenen Daten stehen.
                .onFailure { _state.update { it.copy(loading = false, offline = true) } }
        }
    }
}
