package de.roessing.app.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import de.roessing.app.R

/**
 * Kurze Begründung, bevor der System-Dialog nach dem Standort fragt.
 * Der Standort bleibt auf dem Gerät und geht nie ans Backend.
 */
@Composable
fun LocationHint(onRequest: () -> Unit, modifier: Modifier = Modifier) {
    Card(
        modifier
            .fillMaxWidth()
            .padding(horizontal = 16.dp, vertical = 8.dp)
            .testTag("location-hint"),
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.secondaryContainer,
        ),
    ) {
        Row(
            Modifier.padding(start = 16.dp, end = 8.dp, top = 8.dp, bottom = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f)) {
                Text(
                    stringResource(R.string.location_hint_title),
                    style = MaterialTheme.typography.titleSmall,
                )
                Text(
                    stringResource(R.string.location_hint_text),
                    style = MaterialTheme.typography.bodySmall,
                )
            }
            TextButton(onClick = onRequest, modifier = Modifier.testTag("location-hint-button")) {
                Text(stringResource(R.string.location_hint_button))
            }
        }
    }
}
