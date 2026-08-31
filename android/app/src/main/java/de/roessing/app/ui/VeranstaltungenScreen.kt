package de.roessing.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material.icons.filled.Group
import androidx.compose.material.icons.filled.Place
import androidx.compose.material.icons.filled.Schedule
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.data.Termin

/**
 * „Was ist los in Rössing" — die kommenden Termine, der nächste zuoberst.
 *
 * Die Liste kommt von rössing.de und wird dort gepflegt. Ein Tipp führt
 * dorthin, wo der Termin zu Hause ist: bei einer externen Primärquelle zum
 * Veranstalter, sonst auf die Seite des Dorfes. Doppelt erzählt wird nichts.
 */
@Composable
fun VeranstaltungenScreen(
    state: VeranstaltungenUiState,
    modifier: Modifier = Modifier,
    onAktualisieren: () -> Unit = {},
    onTermin: (Termin) -> Unit = {},
) {
    val browser = LocalUriHandler.current
    Column(modifier.fillMaxWidth().testTag("veranstaltungen")) {
        if (state.laedt && state.termine.isEmpty()) {
            LinearProgressIndicator(Modifier.fillMaxWidth())
        }

        // Erst der Hinweis, dann die (womöglich ältere) Liste — eine leere
        // Seite ohne Erklärung wäre das schlechteste Ergebnis.
        if (state.fehler) {
            Offlinehinweis(
                text = if (state.termine.isEmpty()) {
                    stringResource(R.string.events_offline)
                } else {
                    stringResource(R.string.events_offline_stale)
                },
                onAktualisieren = onAktualisieren,
            )
        }

        LazyColumn(
            Modifier.fillMaxWidth(),
            contentPadding = androidx.compose.foundation.layout.PaddingValues(
                start = 20.dp,
                end = 20.dp,
                top = 12.dp,
                bottom = 28.dp,
            ),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item {
                Text(
                    stringResource(R.string.events_intro),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            items(state.termine, key = { it.id }) { termin ->
                // Ein Termin des Dorfes trägt alles, was wir über ihn wissen,
                // schon in sich — dafür muss niemand die App verlassen. Nur
                // eine fremde Primärquelle führt hinaus; sie hier
                // nachzuerzählen hieße, eine zweite Fassung in die Welt zu
                // setzen, die irgendwann von der ersten abweicht.
                TerminKarte(termin) {
                    if (termin.extern) browser.openUri(termin.url) else onTermin(termin)
                }
            }

            if (state.leer) {
                item {
                    Text(
                        stringResource(R.string.events_empty),
                        style = MaterialTheme.typography.bodyLarge,
                        modifier = Modifier
                            .padding(top = 24.dp)
                            .testTag("veranstaltungen-leer"),
                    )
                }
            }
        }
    }
}

@Composable
private fun Offlinehinweis(text: String, onAktualisieren: () -> Unit) {
    Surface(
        color = MaterialTheme.colorScheme.errorContainer,
        contentColor = MaterialTheme.colorScheme.onErrorContainer,
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 20.dp, vertical = 8.dp)
            .testTag("veranstaltungen-offline"),
        shape = MaterialTheme.shapes.large,
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(text, style = MaterialTheme.typography.bodyMedium)
            TextButton(
                onClick = onAktualisieren,
                modifier = Modifier.testTag("veranstaltungen-erneut"),
            ) { Text(stringResource(R.string.events_retry)) }
        }
    }
}

@Composable
private fun TerminKarte(termin: Termin, onClick: () -> Unit) {
    Card(
        onClick = onClick,
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
        ),
        modifier = Modifier
            .fillMaxWidth()
            .testTag("termin-${termin.id}"),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(
                    termin.datumText,
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.primary,
                )
                if (termin.extern) {
                    Spacer(Modifier.width(8.dp))
                    Icon(
                        Icons.AutoMirrored.Filled.OpenInNew,
                        contentDescription = stringResource(R.string.events_external),
                        modifier = Modifier.size(16.dp),
                        tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            Text(termin.name, style = MaterialTheme.typography.titleMedium)

            // Ganztägig heißt: Es gibt keine Uhrzeit. Dann wird auch keine
            // erfunden, sondern schlicht „Ganztägig" gesagt.
            Zeile(
                Icons.Filled.Schedule,
                termin.zeitText ?: stringResource(R.string.events_all_day),
            )
            termin.ortName?.let { Zeile(Icons.Filled.Place, it) }
            // Die Adresse gehört unter den Ortsnamen, nicht an den linken
            // Rand — deshalb bleibt die Symbolspalte leer stehen.
            termin.ortAdresse?.let { Zeile(null, it) }
            termin.veranstalter?.let { Zeile(Icons.Filled.Group, it) }

            if (termin.beschreibung.isNotBlank()) {
                Text(
                    termin.beschreibung,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 3,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            if (termin.extern) {
                Text(
                    stringResource(R.string.events_external),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun Zeile(symbol: ImageVector?, text: String) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        if (symbol != null) {
            Icon(
                symbol,
                contentDescription = null,
                modifier = Modifier.size(16.dp),
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        } else {
            Spacer(Modifier.size(16.dp))
        }
        Spacer(Modifier.width(8.dp))
        Text(
            text,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}
