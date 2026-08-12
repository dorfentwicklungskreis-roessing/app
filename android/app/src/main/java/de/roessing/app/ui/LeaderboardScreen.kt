package de.roessing.app.ui

import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.data.LeaderboardEntryDto

/** Anzeigename eines Zeitraums. */
@Composable
private fun periodLabel(period: LeaderboardPeriod): String = stringResource(
    when (period) {
        LeaderboardPeriod.WOCHE -> R.string.period_week
        LeaderboardPeriod.MONAT -> R.string.period_month
        LeaderboardPeriod.SAISON -> R.string.period_season
        LeaderboardPeriod.JAHR -> R.string.period_year
        LeaderboardPeriod.GESAMT -> R.string.period_all
    },
)

/**
 * Rangliste: Podest für die ersten drei, darunter alle Beteiligten mit ihren
 * Auszeichnungen. Die eigene Zeile ist hervorgehoben — auch, wenn sie weit
 * hinten steht. Bewusst ohne Punkte und ohne negative Abzeichen.
 */
@Composable
fun LeaderboardScreen(
    state: LeaderboardUiState,
    modifier: Modifier = Modifier,
    onSelectPeriod: (LeaderboardPeriod) -> Unit,
) {
    Column(
        modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState())
            .padding(16.dp)
            .testTag("leaderboard"),
    ) {
        Row(
            Modifier
                .fillMaxWidth()
                .horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            LeaderboardPeriod.entries.forEach { period ->
                FilterChip(
                    selected = period == state.period,
                    onClick = { onSelectPeriod(period) },
                    label = { Text(periodLabel(period)) },
                    modifier = Modifier.testTag("period-${period.wert}"),
                )
            }
        }

        Spacer(Modifier.height(16.dp))
        Card(Modifier.fillMaxWidth().testTag("leaderboard-totals")) {
            Column(Modifier.padding(16.dp)) {
                Text(
                    stringResource(R.string.leaderboard_totals_title),
                    style = MaterialTheme.typography.titleMedium,
                )
                Text(
                    stringResource(
                        R.string.leaderboard_totals,
                        state.totals.completions,
                        state.totals.liters.trimmed(),
                        state.totals.participants,
                    ),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }

        if (state.entries.isEmpty()) {
            Spacer(Modifier.height(24.dp))
            Text(
                stringResource(R.string.leaderboard_empty),
                style = MaterialTheme.typography.bodyLarge,
                textAlign = TextAlign.Center,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier
                    .fillMaxWidth()
                    .testTag("leaderboard-empty"),
            )
        } else {
            Spacer(Modifier.height(16.dp))
            Row(
                Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.Bottom,
            ) {
                state.podest.forEach { eintrag ->
                    PodiumCard(eintrag, Modifier.weight(1f))
                }
            }

            Spacer(Modifier.height(16.dp))
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                state.entries.forEach { eintrag ->
                    LeaderboardRow(eintrag, eigen = eintrag.userSub == state.me?.userSub)
                }
            }
        }

        state.me?.let { ich ->
            Spacer(Modifier.height(16.dp))
            Card(
                Modifier.fillMaxWidth().testTag("leaderboard-me"),
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.primaryContainer,
                ),
            ) {
                Text(
                    if (ich.rank > 0) {
                        stringResource(R.string.leaderboard_my_rank, ich.rank)
                    } else {
                        stringResource(R.string.leaderboard_my_none)
                    },
                    style = MaterialTheme.typography.titleMedium,
                    modifier = Modifier.padding(16.dp),
                )
            }
        }
        Spacer(Modifier.height(24.dp))
    }
}

/** Eine der ersten drei Karten — mit Medaille. */
@Composable
private fun PodiumCard(eintrag: LeaderboardEntryDto, modifier: Modifier = Modifier) {
    OutlinedCard(modifier.testTag("podium-${eintrag.rank}")) {
        Column(
            Modifier
                .fillMaxWidth()
                .padding(12.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Text(
                when (eintrag.rank) {
                    1 -> "🥇"
                    2 -> "🥈"
                    else -> "🥉"
                },
                style = MaterialTheme.typography.headlineMedium,
            )
            Text(
                eintrag.userName,
                style = MaterialTheme.typography.titleSmall,
                textAlign = TextAlign.Center,
            )
            Text(
                stringResource(R.string.leaderboard_completions, eintrag.completions),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** Eine Zeile der vollständigen Liste. */
@Composable
private fun LeaderboardRow(eintrag: LeaderboardEntryDto, eigen: Boolean) {
    Card(
        Modifier
            .fillMaxWidth()
            .testTag("leaderboard-row-${eintrag.userSub}"),
        colors = if (eigen) {
            CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.secondaryContainer)
        } else {
            CardDefaults.cardColors()
        },
    ) {
        Column(Modifier.padding(12.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                // Platz und Name in einem Text — sonst stünde derselbe Name
                // zweimal wortgleich auf der Seite (Podest und Liste).
                Text(
                    "${eintrag.rank}. ${eintrag.userName}",
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.weight(1f),
                )
                if (eigen) {
                    Text(
                        stringResource(R.string.leaderboard_you),
                        style = MaterialTheme.typography.labelLarge,
                        color = MaterialTheme.colorScheme.primary,
                    )
                    Spacer(Modifier.width(8.dp))
                }
                Text(
                    stringResource(R.string.leaderboard_completions, eintrag.completions),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            val details = buildString {
                eintrag.byKind["giessen"]?.takeIf { it > 0 }?.let { append("$it× gegossen") }
                eintrag.byKind["jaeten"]?.takeIf { it > 0 }?.let {
                    if (isNotEmpty()) append(" · ")
                    append("$it× gejätet")
                }
                if (eintrag.liters > 0) {
                    if (isNotEmpty()) append(" · ")
                    append("${eintrag.liters.trimmed()} Liter")
                }
            }
            if (details.isNotEmpty()) {
                Text(
                    details,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (eintrag.badges.isNotEmpty()) {
                Row(
                    Modifier
                        .fillMaxWidth()
                        .horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(6.dp),
                ) {
                    eintrag.badges.forEach { badge ->
                        AssistChip(onClick = {}, label = { Text(badge.label) })
                    }
                }
            }
        }
    }
}

/** 10.0 → "10", 7.5 → "7.5" */
private fun Double.trimmed(): String =
    if (this == toLong().toDouble()) toLong().toString() else toString()
