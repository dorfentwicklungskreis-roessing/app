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
import androidx.compose.material3.FilterChip
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
import de.roessing.app.data.LatLon
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.PlaceSort
import de.roessing.app.data.distanceMeters
import de.roessing.app.data.formatDistance
import de.roessing.app.ui.theme.StatusGreen
import de.roessing.app.ui.theme.StatusRed
import de.roessing.app.ui.theme.StatusYellow

fun statusColor(status: CareStatus) = when (status) {
    CareStatus.green -> StatusGreen
    CareStatus.yellow -> StatusYellow
    CareStatus.red -> StatusRed
}

/**
 * Statustext eines Ortes. Bewusst neutral: an einem Ort können Gieß- und
 * Jätaufgaben zusammenkommen, „Bitte gießen" wäre dort schlicht falsch.
 */
@Composable
fun statusLabel(status: CareStatus): String = when (status) {
    CareStatus.green -> stringResource(R.string.status_green)
    CareStatus.yellow -> stringResource(R.string.status_yellow)
    CareStatus.red -> stringResource(R.string.status_red)
}

/** Statustext einer einzelnen Aufgabe — hier passt die Aufgabenart dazu. */
@Composable
fun taskStatusLabel(kind: String, status: CareStatus): String = when (status) {
    CareStatus.green -> stringResource(R.string.status_green)
    CareStatus.yellow -> stringResource(
        when (kind) {
            "giessen" -> R.string.status_yellow_watering
            "jaeten" -> R.string.status_yellow_weeding
            else -> R.string.status_yellow
        },
    )
    CareStatus.red -> stringResource(
        when (kind) {
            "giessen" -> R.string.status_red_watering
            "jaeten" -> R.string.status_red_weeding
            else -> R.string.status_red
        },
    )
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
    onSortChange: (PlaceSort) -> Unit = {},
) {
    Column(modifier) {
        Row(
            Modifier
                .fillMaxWidth()
                .padding(horizontal = 16.dp, vertical = 8.dp),
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            FilterChip(
                selected = state.sort == PlaceSort.URGENCY,
                onClick = { onSortChange(PlaceSort.URGENCY) },
                label = { Text(stringResource(R.string.sort_urgency)) },
                modifier = Modifier.testTag("sort-urgency"),
            )
            // Ohne Standort gibt es nichts zu sortieren.
            FilterChip(
                selected = state.sort == PlaceSort.DISTANCE,
                onClick = { onSortChange(PlaceSort.DISTANCE) },
                enabled = state.userLocation != null,
                label = { Text(stringResource(R.string.sort_distance)) },
                modifier = Modifier.testTag("sort-distance"),
            )
        }
        LazyColumn(
            modifier = Modifier.testTag("place-list"),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(
                start = 16.dp, end = 16.dp, bottom = 16.dp,
            ),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            items(state.places, key = { it.id }) { place ->
                PlaceCard(
                    place = place,
                    entfernung = state.userLocation?.let { ich ->
                        formatDistance(distanceMeters(ich, LatLon(place.lat, place.lon)))
                    },
                    onTap = { onPlaceTap(place.id) },
                )
            }
        }
    }
}

@Composable
private fun PlaceCard(place: PlaceDto, entfernung: String?, onTap: () -> Unit) {
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
            Column(horizontalAlignment = androidx.compose.ui.Alignment.End) {
                Text(
                    statusLabel(place.careStatus),
                    style = MaterialTheme.typography.labelLarge,
                    color = statusColor(place.careStatus),
                )
                if (entfernung != null) {
                    Text(
                        entfernung,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.testTag("distance-${place.id}"),
                    )
                }
            }
        }
    }
}
