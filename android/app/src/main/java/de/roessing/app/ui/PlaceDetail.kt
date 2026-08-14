package de.roessing.app.ui

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
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.OutlinedButton
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
private val dayFormat = DateTimeFormatter.ofPattern("d.M.yyyy").withZone(ZoneId.of("Europe/Berlin"))

internal fun formatTime(iso: String): String =
    runCatching { dateFormat.format(Instant.parse(iso)) }.getOrDefault(iso)

/**
 * Nur das Datum, in der Ortszeit des Dorfes. Ein Termin „bis zum 20." steht
 * als 20. um 23:59 Ortszeit in der Datenbank — in einer anderen Zeitzone
 * gelesen wäre daraus schnell der 21.
 */
internal fun formatDate(iso: String): String =
    runCatching { dayFormat.format(Instant.parse(iso)) }.getOrDefault(iso.take(10))

/** Detailansicht eines Ortes im BottomSheet: Aufgaben, Pläne, Historie, Melden. */
@Composable
fun PlaceDetail(
    place: PlaceDto,
    pendingTasks: Set<Long>,
    history: Map<Long, List<CompletionDto>>,
    onComplete: (taskId: Long, liters: Double?) -> Unit,
    onLoadHistory: (taskId: Long) -> Unit,
    meinSub: String? = null,
    pendingSignups: Set<Long> = emptySet(),
    pendingAssignments: Set<Long> = emptySet(),
    onSignup: (placeId: Long, taskKind: String?, an: Boolean) -> Unit = { _, _, _ -> },
    onClaim: (assignmentId: Long) -> Unit = {},
    onRelease: (assignmentId: Long) -> Unit = {},
) {
    LaunchedEffect(place.id) {
        place.tasks.forEach { onLoadHistory(it.id) }
    }
    // Vor dem Melden wird nachgefragt: der Knopf ist schnell versehentlich
    // getroffen, und eine Meldung, die es nie gab, verdirbt Ampel und Rangliste.
    var nachfrage by remember { mutableStateOf<TaskDto?>(null) }
    // Scrollbar: Mit Aufgaben, Historie und dem Eintrag als Helfer:in wird
    // das Blatt länger als ein kleiner Bildschirm.
    Column(
        Modifier
            .verticalScroll(rememberScrollState())
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
        // Der Eintrag als Helfer:in gilt für den ganzen Ort — gemeint ist
        // immer „ich kümmere mich um den Kasten vor meiner Tür".
        if (place.tasks.any { it.active }) {
            HelferKarte(
                place = place,
                ausstehend = place.id in pendingSignups,
                onSignup = { art, an -> onSignup(place.id, art, an) },
            )
            Spacer(Modifier.height(12.dp))
        }
        place.tasks.filter { it.active }.forEach { task ->
            TaskCard(
                task = task,
                pending = task.id in pendingTasks,
                history = history[task.id].orEmpty(),
                onComplete = { nachfrage = task },
                meinSub = meinSub,
                pendingAssignments = pendingAssignments,
                onClaim = onClaim,
                onRelease = onRelease,
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
    meinSub: String? = null,
    pendingAssignments: Set<Long> = emptySet(),
    onClaim: (Long) -> Unit = {},
    onRelease: (Long) -> Unit = {},
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
            // Regelmäßig steht hier der Plan, einmalig der Termin.
            Text(planText(task), style = MaterialTheme.typography.bodyMedium)
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
            // Vergabestand: wer hat zugesagt, wie viele helfen hier mit.
            vergabeText(task, meinSub)?.let { stand ->
                Spacer(Modifier.height(6.dp))
                Text(
                    stand,
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.primary,
                    modifier = Modifier.testTag("vergabe-stand-${task.id}"),
                )
            }
            val vorgang = task.assignment
            if (vorgang != null && !gesperrt) {
                Spacer(Modifier.height(8.dp))
                val meiner = vorgang.vonMir(meinSub)
                val laeuft = vorgang.id in pendingAssignments
                if (meiner) {
                    OutlinedButton(
                        onClick = { onRelease(vorgang.id) },
                        enabled = !laeuft,
                        modifier = Modifier
                            .fillMaxWidth()
                            .testTag("release-task-${task.id}"),
                    ) {
                        Text(stringResource(R.string.notification_release))
                    }
                } else if (!vorgang.uebernommen) {
                    Button(
                        onClick = { onClaim(vorgang.id) },
                        enabled = !laeuft,
                        modifier = Modifier
                            .fillMaxWidth()
                            .testTag("claim-task-${task.id}"),
                    ) {
                        Text(stringResource(R.string.notification_claim))
                    }
                }
            }
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
