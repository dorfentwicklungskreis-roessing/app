package de.roessing.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.data.CompletionDto
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.TaskDto
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

private val dateFormat = DateTimeFormatter.ofPattern("d.M.yyyy HH:mm").withZone(ZoneId.systemDefault())

internal fun formatTime(iso: String): String =
    runCatching { dateFormat.format(Instant.parse(iso)) }.getOrDefault(iso)

/** Detailansicht eines Ortes im BottomSheet: Aufgaben, Pläne, Historie, Melden. */
@Composable
fun PlaceDetail(
    place: PlaceDto,
    pendingTasks: Set<Long>,
    history: Map<Long, List<CompletionDto>>,
    onComplete: (taskId: Long, liters: Double?) -> Unit,
    onLoadHistory: (taskId: Long) -> Unit,
) {
    LaunchedEffect(place.id) {
        place.tasks.forEach { onLoadHistory(it.id) }
    }
    // Vor dem Melden wird nachgefragt: der Knopf ist schnell versehentlich
    // getroffen, und eine Meldung, die es nie gab, verdirbt Ampel und Rangliste.
    var nachfrage by remember { mutableStateOf<TaskDto?>(null) }
    Column(
        Modifier
            .padding(horizontal = 20.dp)
            .padding(bottom = 32.dp)
            .testTag("place-detail"),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text(place.name, style = MaterialTheme.typography.headlineSmall)
            StatusPill(place.careStatus, statusLabel(place.careStatus))
        }
        if (place.description.isNotEmpty()) {
            Spacer(Modifier.height(4.dp))
            Text(
                place.description,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        Spacer(Modifier.height(16.dp))
        place.tasks.filter { it.active }.forEach { task ->
            TaskCard(
                task = task,
                pending = task.id in pendingTasks,
                history = history[task.id].orEmpty(),
                onComplete = { nachfrage = task },
            )
            Spacer(Modifier.height(12.dp))
        }
    }

    nachfrage?.let { task ->
        CompletionConfirmDialog(
            place = place,
            task = task,
            onDismiss = { nachfrage = null },
            onConfirm = {
                nachfrage = null
                onComplete(task.id, task.liters)
            },
        )
    }
}

/** Rückfrage vor dem Melden — mit Ort und vorgesehener Menge. */
@Composable
private fun CompletionConfirmDialog(
    place: PlaceDto,
    task: TaskDto,
    onDismiss: () -> Unit,
    onConfirm: () -> Unit,
) {
    AlertDialog(
        onDismissRequest = onDismiss,
        modifier = Modifier.testTag("confirm-completion"),
        title = {
            Text(
                stringResource(
                    when (task.kind) {
                        "giessen" -> R.string.confirm_completion_watering
                        "jaeten" -> R.string.confirm_completion_weeding
                        else -> R.string.confirm_completion_other
                    },
                ),
            )
        },
        text = {
            Column {
                Text(stringResource(R.string.confirm_completion_place, place.name))
                task.liters?.let {
                    Text(stringResource(R.string.confirm_completion_amount, it.trimmed()))
                }
                Spacer(Modifier.height(8.dp))
                Text(
                    stringResource(R.string.confirm_completion_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        },
        confirmButton = {
            TextButton(onClick = onConfirm, modifier = Modifier.testTag("confirm-completion-yes")) {
                Text(stringResource(R.string.confirm_completion_yes))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, modifier = Modifier.testTag("confirm-completion-no")) {
                Text(stringResource(R.string.cancel))
            }
        },
    )
}

@Composable
private fun TaskCard(
    task: TaskDto,
    pending: Boolean,
    history: List<CompletionDto>,
    onComplete: () -> Unit,
) {
    // Spielschutz: nach einer Erledigung bleibt die Aufgabe eine Weile
    // gesperrt. Der Knopf sagt das, statt erst in einen Fehler zu laufen.
    val gesperrtBis = task.lockedUntilInstant
    val gesperrt = gesperrtBis != null && gesperrtBis.isAfter(Instant.now())
    OutlinedCard(Modifier.fillMaxWidth(), shape = MaterialTheme.shapes.large) {
        Column(Modifier.padding(18.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(task.displayName, style = MaterialTheme.typography.titleMedium)
                Spacer(Modifier.weight(1f))
                Spacer(Modifier.width(8.dp))
                StatusPill(task.careStatus, taskStatusLabel(task.kind, task.careStatus))
            }
            Spacer(Modifier.height(6.dp))
            val plan = buildString {
                task.liters?.let { append("${it.trimmed()} Liter, ") }
                append("alle ${task.intervalDays.trimmed()} Tage")
            }
            Text(plan, style = MaterialTheme.typography.bodyMedium)
            val last = task.lastCompletion
            Text(
                if (last != null) {
                    "Zuletzt: ${formatTime(last.doneAt)} von ${last.userName}"
                } else {
                    "Noch nie erledigt"
                },
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(12.dp))
            if (gesperrt) {
                Text(
                    stringResource(R.string.task_locked, formatTime(task.lockedUntil!!)),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.primary,
                )
                Spacer(Modifier.height(6.dp))
            }
            Button(
                onClick = onComplete,
                enabled = !pending && !gesperrt,
                modifier = Modifier
                    .fillMaxWidth()
                    .testTag("complete-task-${task.id}"),
            ) {
                Text(
                    when (task.kind) {
                        "giessen" -> "Ich habe gegossen 💧"
                        "jaeten" -> "Ich habe gejätet 🌿"
                        else -> "Erledigt ✓"
                    },
                )
            }
            if (history.size > 1) {
                Spacer(Modifier.height(10.dp))
                HorizontalDivider()
                Spacer(Modifier.height(6.dp))
                Text("Historie", style = MaterialTheme.typography.labelMedium)
                Column(verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    history.take(5).forEach { c ->
                        Text(
                            "${formatTime(c.doneAt)} — ${c.userName}" +
                                (c.liters?.let { " (${it.trimmed()} l)" } ?: ""),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
    }
}

/** 10.0 → "10", 7.5 → "7.5" */
private fun Double.trimmed(): String =
    if (this == toLong().toDouble()) toLong().toString() else toString()
