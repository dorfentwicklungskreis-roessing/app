package de.roessing.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.data.LatLon
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.TaskDto

/**
 * Der Bereich „Verwaltung": Orte und Aufgaben pflegen — aus der App heraus,
 * am Blumenkasten stehend. Sichtbar nur für Verwaltende; das Backend weist
 * jede Änderung ohne die Rolle „admin" ohnehin ab.
 */
@Composable
fun VerwaltungScreen(
    places: List<PlaceDto>,
    state: VerwaltungUiState,
    modifier: Modifier = Modifier,
    onOrtBearbeiten: (PlaceDto?) -> Unit,
    onOrtLoeschen: (Long) -> Unit,
    onAufgabeBearbeiten: (placeId: Long, aufgabe: TaskDto?) -> Unit,
    onAufgabePausieren: (TaskDto, Boolean) -> Unit,
    onAufgabeLoeschen: (Long) -> Unit,
) {
    // Vor dem Löschen wird gefragt — hier verschwindet die Historie mit.
    var loeschfrage by remember { mutableStateOf<Loeschfrage?>(null) }

    Column(
        modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp)
            .padding(top = 8.dp, bottom = 32.dp)
            .testTag("verwaltung"),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        Text(
            stringResource(R.string.admin_intro),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Button(
            onClick = { onOrtBearbeiten(null) },
            modifier = Modifier
                .fillMaxWidth()
                .testTag("verwaltung-ort-neu"),
        ) {
            Text(stringResource(R.string.admin_new_place))
        }

        if (places.isEmpty()) {
            Text(stringResource(R.string.admin_no_places), style = MaterialTheme.typography.bodyMedium)
        }
        places.forEach { ort ->
            OrtKarte(
                ort = ort,
                laufend = state.laufend,
                onBearbeiten = { onOrtBearbeiten(ort) },
                onLoeschen = { loeschfrage = Loeschfrage.Ort(ort) },
                onAufgabeNeu = { onAufgabeBearbeiten(ort.id, null) },
                onAufgabeBearbeiten = { onAufgabeBearbeiten(ort.id, it) },
                onAufgabePausieren = onAufgabePausieren,
                onAufgabeLoeschen = { loeschfrage = Loeschfrage.Aufgabe(it) },
            )
        }
    }

    loeschfrage?.let { frage ->
        AlertDialog(
            onDismissRequest = { loeschfrage = null },
            modifier = Modifier.testTag("verwaltung-loeschfrage"),
            title = { Text(stringResource(R.string.admin_delete_title)) },
            text = {
                Text(
                    when (frage) {
                        is Loeschfrage.Ort -> stringResource(R.string.admin_delete_place_text, frage.ort.name)
                        is Loeschfrage.Aufgabe ->
                            stringResource(R.string.admin_delete_task_text, frage.aufgabe.displayName)
                    },
                )
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        when (frage) {
                            is Loeschfrage.Ort -> onOrtLoeschen(frage.ort.id)
                            is Loeschfrage.Aufgabe -> onAufgabeLoeschen(frage.aufgabe.id)
                        }
                        loeschfrage = null
                    },
                    modifier = Modifier.testTag("verwaltung-loeschen-ja"),
                ) { Text(stringResource(R.string.admin_delete_yes)) }
            },
            dismissButton = {
                TextButton(onClick = { loeschfrage = null }) { Text(stringResource(R.string.cancel)) }
            },
        )
    }
}

private sealed interface Loeschfrage {
    data class Ort(val ort: PlaceDto) : Loeschfrage
    data class Aufgabe(val aufgabe: TaskDto) : Loeschfrage
}

@Composable
private fun OrtKarte(
    ort: PlaceDto,
    laufend: Set<Long>,
    onBearbeiten: () -> Unit,
    onLoeschen: () -> Unit,
    onAufgabeNeu: () -> Unit,
    onAufgabeBearbeiten: (TaskDto) -> Unit,
    onAufgabePausieren: (TaskDto, Boolean) -> Unit,
    onAufgabeLoeschen: (TaskDto) -> Unit,
) {
    Card(
        Modifier
            .fillMaxWidth()
            .testTag("verwaltung-ort-${ort.id}"),
        shape = MaterialTheme.shapes.large,
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(ort.name, style = MaterialTheme.typography.titleMedium)
                    Text(
                        if (ort.active) {
                            stringResource(R.string.admin_place_active)
                        } else {
                            stringResource(R.string.admin_place_paused)
                        },
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                TextButton(onClick = onBearbeiten, modifier = Modifier.testTag("ort-bearbeiten-${ort.id}")) {
                    Text(stringResource(R.string.admin_edit))
                }
                TextButton(onClick = onLoeschen, modifier = Modifier.testTag("ort-loeschen-${ort.id}")) {
                    Text(stringResource(R.string.admin_delete))
                }
            }
            HorizontalDivider()
            ort.tasks.forEach { aufgabe ->
                AufgabeZeile(
                    aufgabe = aufgabe,
                    laeuft = aufgabe.id in laufend,
                    onBearbeiten = { onAufgabeBearbeiten(aufgabe) },
                    onPausieren = { onAufgabePausieren(aufgabe, it) },
                    onLoeschen = { onAufgabeLoeschen(aufgabe) },
                )
            }
            OutlinedButton(
                onClick = onAufgabeNeu,
                modifier = Modifier
                    .fillMaxWidth()
                    .testTag("aufgabe-neu-${ort.id}"),
            ) {
                Text(stringResource(R.string.admin_new_task))
            }
        }
    }
}

@Composable
private fun AufgabeZeile(
    aufgabe: TaskDto,
    laeuft: Boolean,
    onBearbeiten: () -> Unit,
    onPausieren: (Boolean) -> Unit,
    onLoeschen: () -> Unit,
) {
    OutlinedCard(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(aufgabe.displayName, style = MaterialTheme.typography.titleSmall)
            Text(
                planText(aufgabe),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.testTag("aufgabe-plan-${aufgabe.id}"),
            )
            Row(verticalAlignment = Alignment.CenterVertically) {
                Switch(
                    checked = aufgabe.active,
                    enabled = !laeuft,
                    onCheckedChange = { an -> onPausieren(!an) },
                    modifier = Modifier.testTag("aufgabe-pause-${aufgabe.id}"),
                )
                Spacer(Modifier.height(0.dp))
                Text(
                    if (aufgabe.active) {
                        stringResource(R.string.admin_task_active)
                    } else {
                        stringResource(R.string.admin_task_paused)
                    },
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.padding(start = 8.dp),
                )
                Spacer(Modifier.weight(1f))
                TextButton(
                    onClick = onBearbeiten,
                    modifier = Modifier.testTag("aufgabe-bearbeiten-${aufgabe.id}"),
                ) { Text(stringResource(R.string.admin_edit)) }
                TextButton(
                    onClick = onLoeschen,
                    modifier = Modifier.testTag("aufgabe-loeschen-${aufgabe.id}"),
                ) { Text(stringResource(R.string.admin_delete)) }
            }
        }
    }
}

/** „10 Liter, alle 7 Tage" bzw. „Einmalig — fällig am 20.08.2026". */
@Composable
internal fun planText(aufgabe: TaskDto): String = if (aufgabe.oneOff) {
    val termin = aufgabe.dueDate?.let { formatDate(it) }.orEmpty()
    if (termin.isBlank()) {
        stringResource(R.string.task_once)
    } else {
        stringResource(R.string.task_once_due, termin)
    }
} else {
    buildString {
        aufgabe.liters?.let { append("${zahl(it)} Liter, ") }
        append(stringResource(R.string.task_every_days, zahl(aufgabe.intervalDays)))
    }
}

/**
 * Das Formular für einen Ort. Der Standort kommt entweder vom eigenen Gerät
 * („Ich stehe davor") oder aus einem Tipp auf die Karte.
 */
@Composable
fun OrtFormularDialog(
    form: OrtForm,
    places: List<PlaceDto>,
    userLocation: LatLon?,
    onName: (String) -> Unit,
    onBeschreibung: (String) -> Unit,
    onArt: (String) -> Unit,
    onAktiv: (Boolean) -> Unit,
    onPosition: (Double, Double) -> Unit,
    onStandort: () -> Unit,
    onSpeichern: () -> Unit,
    onAbbrechen: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onAbbrechen,
        modifier = Modifier.testTag("ort-formular"),
        title = {
            Text(
                if (form.neu) {
                    stringResource(R.string.admin_new_place)
                } else {
                    stringResource(R.string.admin_edit_place)
                },
            )
        },
        text = {
            Column(
                Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                form.fehler?.let { Fehlerzeile(it, "ort-formular-fehler") }
                OutlinedTextField(
                    value = form.name,
                    onValueChange = onName,
                    label = { Text(stringResource(R.string.admin_place_name)) },
                    singleLine = true,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("ort-name"),
                )
                OutlinedTextField(
                    value = form.beschreibung,
                    onValueChange = onBeschreibung,
                    label = { Text(stringResource(R.string.admin_place_description)) },
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("ort-beschreibung"),
                )
                Text(stringResource(R.string.admin_place_kind), style = MaterialTheme.typography.labelLarge)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    listOf(
                        "blumenkasten" to R.string.place_kind_box,
                        "beet" to R.string.place_kind_bed,
                        "sonstiges" to R.string.place_kind_other,
                    ).forEach { (wert, label) ->
                        FilterChip(
                            selected = form.art == wert,
                            onClick = { onArt(wert) },
                            label = { Text(stringResource(label)) },
                            modifier = Modifier.testTag("ort-art-$wert"),
                        )
                    }
                }

                Text(stringResource(R.string.admin_place_position), style = MaterialTheme.typography.labelLarge)
                Text(
                    if (form.lat != null && form.lon != null) {
                        stringResource(R.string.admin_position_set, form.lat, form.lon)
                    } else {
                        stringResource(R.string.admin_position_none)
                    },
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag("ort-position"),
                )
                OutlinedButton(
                    onClick = onStandort,
                    enabled = userLocation != null,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("ort-standort-uebernehmen"),
                ) { Text(stringResource(R.string.admin_use_my_location)) }
                if (userLocation == null) {
                    Text(
                        stringResource(R.string.admin_no_location),
                        style = MaterialTheme.typography.bodySmall,
                        fontStyle = FontStyle.Italic,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Text(
                    stringResource(R.string.admin_tap_map),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                MapScreen(
                    places = places,
                    modifier = Modifier
                        .fillMaxWidth()
                        .height(220.dp)
                        .testTag("ort-karte"),
                    userLocation = userLocation,
                    onMapTap = { onPosition(it.lat, it.lon) },
                    auswahl = if (form.lat != null && form.lon != null) {
                        LatLon(form.lat, form.lon)
                    } else {
                        null
                    },
                    onPlaceTap = {},
                )

                Row(verticalAlignment = Alignment.CenterVertically) {
                    Switch(
                        checked = form.aktiv,
                        onCheckedChange = onAktiv,
                        modifier = Modifier.testTag("ort-aktiv"),
                    )
                    Text(
                        stringResource(R.string.admin_place_is_active),
                        modifier = Modifier.padding(start = 8.dp),
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = onSpeichern,
                enabled = form.speicherbar,
                modifier = Modifier.testTag("ort-speichern"),
            ) { Text(stringResource(R.string.admin_save)) }
        },
        dismissButton = {
            TextButton(onClick = onAbbrechen) { Text(stringResource(R.string.cancel)) }
        },
    )
}

/**
 * Das Formular für eine Aufgabe. Die Wahl zwischen regelmäßig und einmalig
 * entscheidet, welche Felder gelten — Intervall oder Termin.
 */
@Composable
fun AufgabeFormularDialog(
    form: AufgabeForm,
    onArt: (String) -> Unit,
    onTitel: (String) -> Unit,
    onLiter: (String) -> Unit,
    onEinmalig: (Boolean) -> Unit,
    onTermin: (String) -> Unit,
    onIntervall: (String) -> Unit,
    onRot: (String) -> Unit,
    onEntfernen: (Boolean) -> Unit,
    onAktiv: (Boolean) -> Unit,
    onSpeichern: () -> Unit,
    onAbbrechen: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onAbbrechen,
        modifier = Modifier.testTag("aufgabe-formular"),
        title = {
            Text(
                if (form.neu) {
                    stringResource(R.string.admin_new_task)
                } else {
                    stringResource(R.string.admin_edit_task)
                },
            )
        },
        text = {
            Column(
                Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(10.dp),
            ) {
                form.fehler?.let { Fehlerzeile(it, "aufgabe-formular-fehler") }
                Text(stringResource(R.string.admin_task_kind), style = MaterialTheme.typography.labelLarge)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    listOf(
                        "giessen" to R.string.task_kind_watering,
                        "jaeten" to R.string.task_kind_weeding,
                        "sonstiges" to R.string.task_kind_other,
                    ).forEach { (wert, label) ->
                        FilterChip(
                            selected = form.art == wert,
                            onClick = { onArt(wert) },
                            label = { Text(stringResource(label)) },
                            modifier = Modifier.testTag("aufgabe-art-$wert"),
                        )
                    }
                }
                OutlinedTextField(
                    value = form.titel,
                    onValueChange = onTitel,
                    label = { Text(stringResource(R.string.admin_task_title)) },
                    singleLine = true,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("aufgabe-titel"),
                )
                if (form.art == "giessen") {
                    OutlinedTextField(
                        value = form.liter,
                        onValueChange = onLiter,
                        label = { Text(stringResource(R.string.admin_task_liters)) },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                        modifier = Modifier
                            .fillMaxWidth()
                            .testTag("aufgabe-liter"),
                    )
                }

                Text(stringResource(R.string.admin_task_repeat), style = MaterialTheme.typography.labelLarge)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    FilterChip(
                        selected = !form.einmalig,
                        onClick = { onEinmalig(false) },
                        label = { Text(stringResource(R.string.admin_task_regular)) },
                        modifier = Modifier.testTag("aufgabe-regelmaessig"),
                    )
                    FilterChip(
                        selected = form.einmalig,
                        onClick = { onEinmalig(true) },
                        label = { Text(stringResource(R.string.admin_task_once)) },
                        modifier = Modifier.testTag("aufgabe-einmalig"),
                    )
                }

                if (form.einmalig) {
                    OutlinedTextField(
                        value = form.termin,
                        onValueChange = onTermin,
                        label = { Text(stringResource(R.string.admin_task_due)) },
                        placeholder = { Text("2026-08-20") },
                        singleLine = true,
                        modifier = Modifier
                            .fillMaxWidth()
                            .testTag("aufgabe-termin"),
                    )
                    Text(
                        stringResource(R.string.admin_task_due_hint),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                } else {
                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        OutlinedTextField(
                            value = form.intervall,
                            onValueChange = onIntervall,
                            label = { Text(stringResource(R.string.admin_task_interval)) },
                            singleLine = true,
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                            modifier = Modifier
                                .weight(1f)
                                .testTag("aufgabe-intervall"),
                        )
                        OutlinedTextField(
                            value = form.rot,
                            onValueChange = onRot,
                            label = { Text(stringResource(R.string.admin_task_red)) },
                            singleLine = true,
                            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Decimal),
                            modifier = Modifier
                                .weight(1f)
                                .testTag("aufgabe-rot"),
                        )
                    }
                }

                Row(verticalAlignment = Alignment.CenterVertically) {
                    Switch(
                        checked = form.entfernenNachErledigung,
                        onCheckedChange = onEntfernen,
                        modifier = Modifier.testTag("aufgabe-entfernen"),
                    )
                    Text(
                        stringResource(R.string.admin_task_remove_when_done),
                        modifier = Modifier.padding(start = 8.dp),
                    )
                }
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Switch(
                        checked = form.aktiv,
                        onCheckedChange = onAktiv,
                        modifier = Modifier.testTag("aufgabe-aktiv"),
                    )
                    Text(
                        stringResource(R.string.admin_task_is_active),
                        modifier = Modifier.padding(start = 8.dp),
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = onSpeichern,
                enabled = form.speicherbar,
                modifier = Modifier.testTag("aufgabe-speichern"),
            ) { Text(stringResource(R.string.admin_save)) }
        },
        dismissButton = {
            TextButton(onClick = onAbbrechen) { Text(stringResource(R.string.cancel)) }
        },
    )
}

@Composable
private fun Fehlerzeile(text: String, tag: String) {
    Text(
        text,
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.error,
        modifier = Modifier.testTag(tag),
    )
}

/** 10.0 → „10", 7.5 → „7,5" */
private fun zahl(v: Double): String =
    if (v == v.toLong().toDouble()) v.toLong().toString() else v.toString().replace('.', ',')
