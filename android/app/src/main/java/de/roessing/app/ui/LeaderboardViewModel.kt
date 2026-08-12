package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import de.roessing.app.data.LeaderboardEntryDto
import de.roessing.app.data.LeaderboardTotalsDto
import de.roessing.app.data.StatsRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

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
    val offline: Boolean = false,
) {
    /** Die ersten drei Plätze — sie kommen aufs Podest. */
    val podest: List<LeaderboardEntryDto> get() = entries.take(3)
}

class LeaderboardViewModel(private val repo: StatsRepository) : ViewModel() {
    private val _state = MutableStateFlow(LeaderboardUiState())
    val state: StateFlow<LeaderboardUiState> = _state

    fun select(period: LeaderboardPeriod) {
    }

    fun refresh() {
    }
}
