package de.roessing.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Card
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.data.CareStatus
import de.roessing.app.data.PlaceDto
import de.roessing.app.ui.theme.StatusGreen
import de.roessing.app.ui.theme.StatusRed
import de.roessing.app.ui.theme.StatusYellow

fun statusColor(status: CareStatus) = when (status) {
    CareStatus.green -> StatusGreen
    CareStatus.yellow -> StatusYellow
    CareStatus.red -> StatusRed
}

@Composable
fun statusLabel(status: CareStatus): String = when (status) {
    CareStatus.green -> stringResource(R.string.status_green)
    CareStatus.yellow -> stringResource(R.string.status_yellow)
    CareStatus.red -> stringResource(R.string.status_red)
}

/** Punkt in Statusfarbe. */
@Composable
fun StatusDot(status: CareStatus, size: Int = 14) {
    Box(
        Modifier
            .size(size.dp)
            .clip(CircleShape)
            .background(statusColor(status)),
    )
}

/** Liste aller Orte, dringendste zuerst (Sortierung macht das ViewModel). */
@Composable
fun PlaceListScreen(
    state: PlacesUiState,
    modifier: Modifier = Modifier,
    onPlaceTap: (Long) -> Unit,
) {
    LazyColumn(
        modifier = modifier.testTag("place-list"),
        contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        items(state.places, key = { it.id }) { place ->
            PlaceCard(place, onTap = { onPlaceTap(place.id) })
        }
    }
}

@Composable
private fun PlaceCard(place: PlaceDto, onTap: () -> Unit) {
    Card(
        onClick = onTap,
        modifier = Modifier
            .fillMaxWidth()
            .testTag("place-card-${place.id}"),
    ) {
        Row(
            Modifier.padding(16.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            StatusDot(place.careStatus, size = 18)
            Spacer(Modifier.width(14.dp))
            Column(Modifier.weight(1f)) {
                Text(place.name, style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.height(2.dp))
                val tasks = place.tasks.filter { it.active }
                    .joinToString(" · ") { it.displayName }
                Text(
                    if (tasks.isEmpty()) statusLabel(place.careStatus) else tasks,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                statusLabel(place.careStatus),
                style = MaterialTheme.typography.labelLarge,
                color = statusColor(place.careStatus),
            )
        }
    }
}
