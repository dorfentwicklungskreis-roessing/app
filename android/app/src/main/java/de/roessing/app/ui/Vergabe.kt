package de.roessing.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Campaign
import androidx.compose.material.icons.filled.Handshake
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
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
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.data.NotificationDto
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.TaskDto

/**
 * Die Oberfläche der Vergabe: Wer sich als Helfer:in einträgt, wird gefragt,
 * sobald an „seinem" Ort etwas ansteht — der Reihe nach, nicht alle auf
 * einmal. Die Regeln dazu stehen alle im Backend; hier steht nur, wie das
 * aussieht und welche Knöpfe es gibt.
 */

/** Aufgabenart, für die jemand mithelfen möchte (null = alles am Ort). */
private const val ALLES = "alles"

/**
 * Der Eintrag als Helfer:in für einen Ort. Hat der Ort sowohl Gieß- als auch
 * Jätaufgaben, lässt sich das einschränken: Gießen ist eine kurze Sache, die
 * jede Woche ansteht, Jäten dauert und kommt selten.
 */
@Composable
fun HelferKarte(
    place: PlaceDto,
    ausstehend: Boolean,
    onSignup: (taskKind: String?, an: Boolean) -> Unit,
) {
    val aufgaben = place.tasks.filter { it.active }
    val arten = aufgaben.map { it.kind }.distinct()
    val eingetragen = aufgaben.any { it.signedUp }
    // Vorauswahl: die Art, für die ich schon eingetragen bin — oder alles.
    val vorgemerkt = when {
        !eingetragen -> ALLES
        aufgaben.all { it.signedUp } -> ALLES
        else -> aufgaben.first { it.signedUp }.kind
    }
    var auswahl by remember(place.id, vorgemerkt) { mutableStateOf(vorgemerkt) }

    Card(
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = if (eingetragen) {
                MaterialTheme.colorScheme.secondaryContainer
            } else {
                MaterialTheme.colorScheme.surfaceContainerHigh
            },
        ),
        modifier = Modifier
            .fillMaxWidth()
            .testTag("helfer-karte"),
    ) {
        Column(Modifier.padding(18.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Icon(Icons.Filled.Handshake, contentDescription = null)
                Spacer(Modifier.width(12.dp))
                Column(Modifier.weight(1f)) {
                    Text(
                        stringResource(
                            if (eingetragen) R.string.signup_active else R.string.signup_title,
                        ),
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Text(
                        stringResource(
                            if (eingetragen) R.string.signup_hint_active else R.string.signup_hint,
                        ),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Spacer(Modifier.width(12.dp))
                Switch(
                    checked = eingetragen,
                    enabled = !ausstehend,
                    onCheckedChange = { an ->
                        onSignup(auswahl.takeIf { it != ALLES }, an)
                    },
                    modifier = Modifier.testTag("signup-switch"),
                )
            }
            // Die Auswahl lohnt nur, wenn es hier wirklich Verschiedenes zu
            // tun gibt. Wer schon eingetragen ist, sieht sie unveränderlich —
            // zum Wechseln erst austragen.
            if (arten.size > 1) {
                Spacer(Modifier.height(12.dp))
                Text(
                    stringResource(R.string.signup_kind_question),
                    style = MaterialTheme.typography.labelMedium,
                )
                Spacer(Modifier.height(6.dp))
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    ArtChip(ALLES, R.string.signup_kind_all, auswahl, eingetragen) { auswahl = it }
                    if ("giessen" in arten) {
                        ArtChip("giessen", R.string.signup_kind_watering, auswahl, eingetragen) { auswahl = it }
                    }
                    if ("jaeten" in arten) {
                        ArtChip("jaeten", R.string.signup_kind_weeding, auswahl, eingetragen) { auswahl = it }
                    }
                }
            }
        }
    }
}

@Composable
private fun ArtChip(
    wert: String,
    label: Int,
    auswahl: String,
    gesperrt: Boolean,
    onSelect: (String) -> Unit,
) {
    FilterChip(
        selected = auswahl == wert,
        enabled = !gesperrt,
        onClick = { onSelect(wert) },
        label = { Text(stringResource(label)) },
        modifier = Modifier.testTag("signup-kind-$wert"),
    )
}

/**
 * Der Vergabestand einer Aufgabe in einem Satz: „Übernommen von … bis …",
 * „3 helfen hier mit". Liefert null, wenn es nichts zu sagen gibt.
 */
@Composable
fun vergabeText(task: TaskDto, meinSub: String?): String? {
    val vorgang = task.assignment
    return when {
        vorgang != null && vorgang.vonMir(meinSub) ->
            stringResource(R.string.assignment_claimed_by_me, frist(vorgang.claimedUntil))

        vorgang != null && vorgang.uebernommen -> stringResource(
            R.string.assignment_claimed,
            vorgang.claimedByName.ifBlank { "jemandem" },
            frist(vorgang.claimedUntil),
        )

        vorgang != null && vorgang.state == "rundruf" -> stringResource(R.string.assignment_broadcast)
        task.signupCount == 1 -> stringResource(R.string.signup_count_one)
        task.signupCount > 1 -> stringResource(R.string.signup_count, task.signupCount)
        else -> null
    }
}

private fun frist(iso: String?): String = iso?.let { formatTime(it) } ?: "später"

/**
 * Die offenen Benachrichtigungen: Anfragen mit Knöpfen, Hinweise zum
 * Wegklicken. Steht auf der Startseite, weil sie der Grund ist, die App
 * überhaupt zu öffnen.
 */
@Composable
fun BenachrichtigungenAbschnitt(
    notifications: List<NotificationDto>,
    pendingAssignments: Set<Long>,
    meineVorgaenge: Set<Long>,
    onClaim: (Long) -> Unit,
    onRelease: (Long) -> Unit,
    onAck: (Long) -> Unit,
) {
    if (notifications.isEmpty()) return
    Column(
        Modifier
            .fillMaxWidth()
            .testTag("benachrichtigungen"),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Filled.Campaign, contentDescription = null)
            Spacer(Modifier.width(10.dp))
            Text(
                stringResource(R.string.notifications_title),
                style = MaterialTheme.typography.titleMedium,
            )
        }
        notifications.forEach { n ->
            BenachrichtigungKarte(
                n = n,
                ausstehend = n.assignmentId in pendingAssignments,
                meiner = n.assignmentId in meineVorgaenge,
                onClaim = onClaim,
                onRelease = onRelease,
                onAck = onAck,
            )
        }
    }
}

@Composable
private fun BenachrichtigungKarte(
    n: NotificationDto,
    ausstehend: Boolean,
    meiner: Boolean,
    onClaim: (Long) -> Unit,
    onRelease: (Long) -> Unit,
    onAck: (Long) -> Unit,
) {
    Card(
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = if (n.istAnfrage) {
                MaterialTheme.colorScheme.primaryContainer
            } else {
                MaterialTheme.colorScheme.surfaceContainerHigh
            },
        ),
        modifier = Modifier
            .fillMaxWidth()
            .testTag("benachrichtigung-${n.id}"),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(n.title, style = MaterialTheme.typography.titleSmall)
            Text(n.text, style = MaterialTheme.typography.bodyMedium)
            n.expiresAt?.let {
                Text(
                    stringResource(R.string.notification_until, formatTime(it)),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Spacer(Modifier.height(2.dp))
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                if (n.istAnfrage && !meiner) {
                    Button(
                        onClick = { onClaim(n.assignmentId) },
                        enabled = !ausstehend,
                        modifier = Modifier.testTag("claim-${n.assignmentId}"),
                    ) {
                        Text(stringResource(R.string.notification_claim))
                    }
                }
                if (meiner) {
                    OutlinedButton(
                        onClick = { onRelease(n.assignmentId) },
                        enabled = !ausstehend,
                        modifier = Modifier.testTag("release-${n.assignmentId}"),
                    ) {
                        Text(stringResource(R.string.notification_release))
                    }
                }
                if (!n.istAnfrage) {
                    TextButton(
                        onClick = { onAck(n.id) },
                        modifier = Modifier.testTag("ack-${n.id}"),
                    ) {
                        Text(stringResource(R.string.notification_ack))
                    }
                }
            }
        }
    }
}

/** „Eine Anfrage wartet auf dich" für die Startseite. */
@Composable
fun anfragenZeile(offeneAnfragen: Int): String? =
    if (offeneAnfragen <= 0) null else pluralStringResource(R.plurals.home_requests, offeneAnfragen, offeneAnfragen)
