package de.roessing.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material.icons.filled.CalendarMonth
import androidx.compose.material.icons.filled.Group
import androidx.compose.material.icons.filled.Place
import androidx.compose.material.icons.filled.Schedule
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.data.Termin

/**
 * Ein Termin des Dorfes, ausführlich — ohne die App zu verlassen.
 *
 * Bewusst nur für **eigene** Termine: Verweist ein Eintrag auf eine fremde
 * Primärquelle, führt der Tipp dorthin. Denselben Inhalt an zwei Stellen zu
 * erzählen heißt, dass eine der beiden irgendwann falsch ist — dieselbe
 * Regel, nach der die Termine überhaupt nur auf rössing.de gepflegt werden.
 *
 * Dieselbe Gliederung wie auf iOS (`TerminDetailView`): Wann, Wo und von wem,
 * Worum es geht, zuletzt der Weg zur Quelle. Wer zwei Telefone nebeneinander
 * hält, soll dasselbe sehen.
 */
@Composable
fun TerminDetail(
    termin: Termin,
    modifier: Modifier = Modifier,
    onWebsite: () -> Unit = {},
) {
    Column(
        modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp, vertical = 12.dp)
            .testTag("termin-detail"),
        verticalArrangement = Arrangement.spacedBy(20.dp),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                termin.datumText,
                style = MaterialTheme.typography.labelLarge,
                color = MaterialTheme.colorScheme.primary,
            )
            Text(termin.name, style = MaterialTheme.typography.headlineSmall)
        }

        Abschnitt(stringResource(R.string.events_when)) {
            Angabe(Icons.Filled.CalendarMonth, stringResource(R.string.events_day), termin.datumText)
            HorizontalDivider()
            // Ganztägig heißt: Es gibt keine Uhrzeit — und es wird auch keine
            // erfunden.
            Angabe(
                Icons.Filled.Schedule,
                stringResource(R.string.events_time),
                termin.zeitText ?: stringResource(R.string.events_all_day),
            )
        }

        if (termin.ortName != null || termin.veranstalter != null) {
            Abschnitt(stringResource(R.string.events_where)) {
                termin.ortName?.let {
                    Angabe(Icons.Filled.Place, stringResource(R.string.events_place), it)
                }
                termin.ortAdresse?.let {
                    HorizontalDivider()
                    Angabe(null, stringResource(R.string.events_address), it)
                }
                termin.veranstalter?.let {
                    if (termin.ortName != null) HorizontalDivider()
                    Angabe(Icons.Filled.Group, stringResource(R.string.events_organizer), it)
                }
            }
        }

        if (termin.beschreibung.isNotBlank()) {
            Abschnitt(stringResource(R.string.events_about)) {
                Text(
                    termin.beschreibung,
                    style = MaterialTheme.typography.bodyLarge,
                    modifier = Modifier
                        .padding(16.dp)
                        .testTag("termin-beschreibung"),
                )
            }
        }

        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            TextButton(
                onClick = onWebsite,
                modifier = Modifier.testTag("termin-quelle"),
            ) {
                Icon(
                    Icons.AutoMirrored.Filled.OpenInNew,
                    contentDescription = null,
                    modifier = Modifier.size(18.dp),
                )
                Spacer(Modifier.width(8.dp))
                Text(stringResource(R.string.events_source_link))
            }
            Text(
                stringResource(R.string.events_source_note),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/** Überschrift und Karte — dieselbe Gliederung für jeden Block. */
@Composable
private fun Abschnitt(titel: String, inhalt: @Composable () -> Unit) {
    Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text(
            titel,
            style = MaterialTheme.typography.titleSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Card(
            shape = MaterialTheme.shapes.large,
            colors = CardDefaults.cardColors(
                containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
            ),
            modifier = Modifier.fillMaxWidth(),
        ) { inhalt() }
    }
}

/**
 * Eine Zeile „Titel — Wert" mit Symbol, damit Wann und Wo gleich aussehen.
 * Ohne Symbol bleibt die Spalte trotzdem stehen: Die Adresse gehört unter den
 * Ortsnamen, nicht an den linken Rand.
 */
@Composable
private fun Angabe(symbol: ImageVector?, titel: String, wert: String) {
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.Top,
    ) {
        if (symbol != null) {
            Icon(
                symbol,
                contentDescription = null,
                modifier = Modifier.size(18.dp),
                tint = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        } else {
            Spacer(Modifier.size(18.dp))
        }
        Spacer(Modifier.width(12.dp))
        Text(
            titel,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.width(12.dp))
        Text(
            wert,
            style = MaterialTheme.typography.bodyMedium,
            textAlign = TextAlign.End,
            modifier = Modifier.weight(1f),
        )
    }
}
