package de.roessing.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Group
import androidx.compose.material.icons.filled.HourglassEmpty
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.PersonAdd
import androidx.compose.material.icons.filled.Public
import androidx.compose.material.icons.filled.Verified
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.data.BeitrittDto
import de.roessing.app.data.MemberDto
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.TraegerDto
import de.roessing.app.ui.theme.statusFarben

/**
 * Der Bereich „Vereine und Gruppen": das Verzeichnis, und dahinter je Träger
 * die Seite mit Arbeitskreisen, Orten, dem Weg zum Mitmachen — und, für den
 * Vorstand, den Anfragen.
 *
 * Was hier zu sehen und zu tun ist, entscheidet der Server: Die Liste enthält
 * nur, was diese Person sehen darf, und jeder Eintrag bringt mit, ob sie
 * Mitglied ist, verwalten darf und beitreten kann. Hier steht nur, wie das
 * aussieht und welche Knöpfe es gibt.
 */
@Composable
fun TraegerScreen(
    state: TraegerUiState,
    modifier: Modifier = Modifier,
    places: List<PlaceDto> = emptyList(),
    onOpen: (Long) -> Unit = {},
    onJoin: (Long, String) -> Unit = { _, _ -> },
    onDecide: (BeitrittDto, String) -> Unit = { _, _ -> },
    onAddMember: (Long, MemberDto) -> Unit = { _, _ -> },
    onLoadVillagers: () -> Unit = {},
    onPlace: (Long) -> Unit = {},
    onDismissError: () -> Unit = {},
) {
    val offen = state.openTraeger
    Column(
        modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp)
            .padding(top = 8.dp, bottom = 28.dp)
            .testTag(if (offen == null) "traeger-list" else "traeger-detail"),
        verticalArrangement = Arrangement.spacedBy(14.dp),
    ) {
        state.notice?.let { hinweis ->
            Hinweiszeile(hinweis, testTag = "traeger-notice")
        }
        if (offen == null) {
            Verzeichnis(state = state, onOpen = onOpen)
        } else {
            Detail(
                state = state,
                traeger = offen,
                places = places.filter { it.traegerId == offen.id },
                onOpen = onOpen,
                onJoin = onJoin,
                onDecide = onDecide,
                onAddMember = onAddMember,
                onLoadVillagers = onLoadVillagers,
                onPlace = onPlace,
            )
        }
    }

    // Der Wortlaut kommt vom Server. Er weiß, warum es nicht ging — die App
    // wüsste es nur ungefähr.
    state.error?.let { fehler ->
        AlertDialog(
            onDismissRequest = onDismissError,
            modifier = Modifier.testTag("traeger-error"),
            title = { Text(stringResource(R.string.traeger_failed_title)) },
            text = { Text(fehler) },
            confirmButton = {
                TextButton(onClick = onDismissError, modifier = Modifier.testTag("traeger-error-ok")) {
                    Text("Ok")
                }
            },
        )
    }
}

// --- Verzeichnis ------------------------------------------------------------

@Composable
private fun Verzeichnis(state: TraegerUiState, onOpen: (Long) -> Unit) {
    if (state.myPending.isNotEmpty()) {
        Text(
            stringResource(R.string.traeger_my_requests),
            style = MaterialTheme.typography.titleMedium,
        )
        state.myPending.forEach { antrag ->
            Card(
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
                ),
                modifier = Modifier
                    .fillMaxWidth()
                    .testTag("my-request-${antrag.id}"),
            ) {
                Column(Modifier.padding(16.dp)) {
                    Text(antrag.traegerName, style = MaterialTheme.typography.titleSmall)
                    Text(
                        stringResource(R.string.traeger_my_request_pending),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }

    if (state.roots.isEmpty()) {
        if (state.loading && !state.everLoaded) {
            CircularProgressIndicator()
        } else {
            Card(
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
                ),
                modifier = Modifier
                    .fillMaxWidth()
                    .testTag("traeger-empty"),
            ) {
                Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(
                        stringResource(R.string.traeger_empty_title),
                        style = MaterialTheme.typography.titleMedium,
                    )
                    Text(
                        stringResource(R.string.traeger_empty_text),
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
        return
    }

    state.roots.forEach { verein ->
        TraegerKarte(traeger = verein, onClick = { onOpen(verein.id) })
        state.children(of = verein.id).forEach { arbeitskreis ->
            TraegerKarte(
                traeger = arbeitskreis,
                unterTraeger = true,
                onClick = { onOpen(arbeitskreis.id) },
            )
        }
    }
}

/** Eine Zeile des Verzeichnisses. Arbeitskreise stehen eingerückt darunter. */
@Composable
private fun TraegerKarte(
    traeger: TraegerDto,
    unterTraeger: Boolean = false,
    onClick: () -> Unit,
) {
    Card(
        onClick = onClick,
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = if (unterTraeger) {
                MaterialTheme.colorScheme.surfaceContainer
            } else {
                MaterialTheme.colorScheme.surfaceContainerHigh
            },
        ),
        modifier = Modifier
            .fillMaxWidth()
            .padding(start = if (unterTraeger) 20.dp else 0.dp)
            .testTag("traeger-${traeger.id}"),
    ) {
        Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    traeger.name,
                    style = if (unterTraeger) {
                        MaterialTheme.typography.titleSmall
                    } else {
                        MaterialTheme.typography.titleMedium
                    },
                )
                if (traeger.beschreibung.isNotBlank()) {
                    Text(
                        traeger.beschreibung,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Abzeichen(traeger)
            }
            Icon(
                Icons.AutoMirrored.Filled.ArrowForward,
                contentDescription = null,
                modifier = Modifier.size(20.dp),
            )
        }
    }
}

/** Die kurzen Marken: dabei, offen, geschlossen — und wie viele Anfragen auf
 *  eine Entscheidung warten. */
@Composable
private fun Abzeichen(traeger: TraegerDto) {
    val farben = statusFarben
    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        when {
            traeger.istMitglied -> Marke(
                stringResource(R.string.traeger_member_badge),
                Icons.Filled.Verified,
                farben.gruen,
                "traeger-member-${traeger.id}",
            )

            traeger.istGeschlossen -> Marke(
                stringResource(R.string.traeger_closed_badge),
                Icons.Filled.Lock,
                MaterialTheme.colorScheme.onSurfaceVariant,
                "traeger-closed-${traeger.id}",
            )

            traeger.beitrittStatus == "beantragt" -> Marke(
                stringResource(R.string.traeger_pending_badge),
                Icons.Filled.HourglassEmpty,
                MaterialTheme.colorScheme.onSurfaceVariant,
                "traeger-pending-${traeger.id}",
            )

            traeger.beitrittMoeglich -> Marke(
                stringResource(R.string.traeger_joinable_badge),
                Icons.Filled.PersonAdd,
                MaterialTheme.colorScheme.primary,
                "traeger-joinable-${traeger.id}",
            )
        }
        if (traeger.offeneBeitritte > 0) {
            Marke(
                stringResource(R.string.traeger_open_badge, traeger.offeneBeitritte),
                Icons.Filled.HourglassEmpty,
                farben.gelb,
                "traeger-open-${traeger.id}",
            )
        }
    }
}

@Composable
private fun Marke(text: String, symbol: ImageVector, farbe: Color, tag: String) {
    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.testTag(tag)) {
        Icon(symbol, contentDescription = null, tint = farbe, modifier = Modifier.size(14.dp))
        Spacer(Modifier.width(4.dp))
        Text(text, style = MaterialTheme.typography.labelMedium, color = farbe)
    }
}

@Composable
private fun Hinweiszeile(text: String, testTag: String) {
    Surface(
        shape = MaterialTheme.shapes.large,
        color = MaterialTheme.colorScheme.errorContainer,
        contentColor = MaterialTheme.colorScheme.onErrorContainer,
        modifier = Modifier
            .fillMaxWidth()
            .testTag(testTag),
    ) {
        Text(text, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(14.dp))
    }
}

// --- Ein einzelner Träger ---------------------------------------------------

@Composable
private fun Detail(
    state: TraegerUiState,
    traeger: TraegerDto,
    places: List<PlaceDto>,
    onOpen: (Long) -> Unit,
    onJoin: (Long, String) -> Unit,
    onDecide: (BeitrittDto, String) -> Unit,
    onAddMember: (Long, MemberDto) -> Unit,
    onLoadVillagers: () -> Unit,
    onPlace: (Long) -> Unit,
) {
    var mitmachenOffen by remember { mutableStateOf(false) }
    var aufnahmeOffen by remember { mutableStateOf(false) }

    Text(traeger.name, style = MaterialTheme.typography.headlineSmall)
    if (traeger.beschreibung.isNotBlank()) {
        Text(traeger.beschreibung, style = MaterialTheme.typography.bodyMedium)
    }
    Abzeichen(traeger)

    state.traeger(traeger.parentId)?.let { dach ->
        TextButton(
            onClick = { onOpen(dach.id) },
            modifier = Modifier.testTag("traeger-parent"),
        ) {
            Text(stringResource(R.string.traeger_parent, dach.name))
        }
    }

    HorizontalDivider()
    Text(
        stringResource(R.string.traeger_state_heading),
        style = MaterialTheme.typography.titleSmall,
    )
    Zeile(
        text = when (traeger.status) {
            "zugelassen" -> stringResource(R.string.traeger_status_approved)
            "beantragt" -> stringResource(R.string.traeger_status_pending)
            "gesperrt" -> stringResource(R.string.traeger_status_blocked)
            else -> traeger.status
        },
        symbol = Icons.Filled.Verified,
    )
    Zeile(
        text = if (traeger.istGeschlossen) {
            stringResource(R.string.traeger_visibility_closed)
        } else {
            stringResource(R.string.traeger_visibility_open)
        },
        symbol = if (traeger.istGeschlossen) Icons.Filled.Lock else Icons.Filled.Public,
    )

    // Mitmachen
    HorizontalDivider()
    Text(stringResource(R.string.traeger_join_heading), style = MaterialTheme.typography.titleSmall)
    when {
        traeger.istMitglied -> Zeile(
            text = stringResource(R.string.traeger_i_am_member),
            symbol = Icons.Filled.CheckCircle,
            testTag = "traeger-i-am-member",
        )

        traeger.beitrittStatus == "beantragt" -> Zeile(
            text = stringResource(R.string.traeger_my_request_state),
            symbol = Icons.Filled.HourglassEmpty,
            testTag = "traeger-my-request",
        )

        traeger.beitrittMoeglich -> Button(
            onClick = { mitmachenOffen = true },
            enabled = traeger.id !in state.busy,
            modifier = Modifier
                .fillMaxWidth()
                .testTag("traeger-join"),
        ) {
            Text(stringResource(R.string.traeger_join))
        }

        // Der Satz kommt vom Server: „Diese Gruppe ist geschlossen …",
        // „Dieser Träger hat in der Rössing-ID noch kein Projekt …". Ihn hier
        // nachzubauen hieße, die Regeln ein zweites Mal zu haben.
        traeger.beitrittHindernis.isNotBlank() -> Zeile(
            text = traeger.beitrittHindernis,
            symbol = Icons.Filled.Lock,
            testTag = "traeger-obstacle",
        )
    }

    // Arbeitskreise
    val kinder = state.children(of = traeger.id)
    if (kinder.isNotEmpty()) {
        HorizontalDivider()
        Text(
            stringResource(R.string.traeger_working_groups),
            style = MaterialTheme.typography.titleSmall,
        )
        kinder.forEach { kind ->
            TraegerKarte(traeger = kind, unterTraeger = true, onClick = { onOpen(kind.id) })
        }
        Text(
            stringResource(R.string.traeger_working_groups_hint),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }

    // Die Orte, die dieser Träger betreut — der Weg zurück zu dem, worum es
    // eigentlich geht. Dieselbe Liste, die auch „Mithelfen" zeigt, und damit
    // dieselbe Sichtbarkeit.
    if (places.isNotEmpty()) {
        HorizontalDivider()
        Text(stringResource(R.string.traeger_places), style = MaterialTheme.typography.titleSmall)
        places.forEach { ort ->
            Card(
                onClick = { onPlace(ort.id) },
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
                ),
                modifier = Modifier
                    .fillMaxWidth()
                    .testTag("traeger-place-${ort.id}"),
            ) {
                Row(
                    Modifier.padding(14.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(12.dp),
                ) {
                    StatusPill(ort.careStatus, statusLabel(ort.careStatus))
                    Text(ort.name, style = MaterialTheme.typography.bodyMedium)
                }
            }
        }
        Text(
            stringResource(R.string.traeger_places_hint),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }

    // Anfragen entscheiden — nur, wer verwalten darf, sieht das überhaupt.
    // Ob er darf, sagt der Server.
    if (traeger.darfVerwalten) {
        HorizontalDivider()
        Text(stringResource(R.string.traeger_requests), style = MaterialTheme.typography.titleSmall)
        val offene = state.openRequests(traeger.id)
        if (offene.isEmpty()) {
            Text(
                stringResource(R.string.traeger_requests_empty),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.testTag("traeger-requests-empty"),
            )
        }
        offene.forEach { antrag ->
            AntragKarte(
                antrag = antrag,
                laeuft = antrag.id in state.busyRequests,
                onDecide = onDecide,
            )
        }
        OutlinedButton(
            onClick = {
                onLoadVillagers()
                aufnahmeOffen = true
            },
            modifier = Modifier
                .fillMaxWidth()
                .testTag("traeger-add-member"),
        ) {
            Text(stringResource(R.string.traeger_add_member))
        }
        Text(
            stringResource(R.string.traeger_requests_hint),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }

    if (mitmachenOffen) {
        MitmachenDialog(
            traeger = traeger,
            laeuft = traeger.id in state.busy,
            onDismiss = { mitmachenOffen = false },
            onSend = { grund ->
                mitmachenOffen = false
                onJoin(traeger.id, grund)
            },
        )
    }

    if (aufnahmeOffen) {
        AufnahmeDialog(
            villagers = state.villagers,
            onDismiss = { aufnahmeOffen = false },
            onPick = { person ->
                aufnahmeOffen = false
                onAddMember(traeger.id, person)
            },
        )
    }
}

@Composable
private fun Zeile(text: String, symbol: ImageVector, testTag: String? = null) {
    Row(
        verticalAlignment = Alignment.Top,
        modifier = if (testTag == null) Modifier else Modifier.testTag(testTag),
    ) {
        Icon(
            symbol,
            contentDescription = null,
            modifier = Modifier.size(18.dp),
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.width(10.dp))
        Text(text, style = MaterialTheme.typography.bodyMedium)
    }
}

@Composable
private fun AntragKarte(
    antrag: BeitrittDto,
    laeuft: Boolean,
    onDecide: (BeitrittDto, String) -> Unit,
) {
    Card(
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
        ),
        modifier = Modifier
            .fillMaxWidth()
            .testTag("request-${antrag.id}"),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Text(antrag.anzeigename, style = MaterialTheme.typography.titleSmall)
            if (antrag.begruendung.isNotBlank()) {
                Text(
                    antrag.begruendung,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                Button(
                    onClick = { onDecide(antrag, "erteilt") },
                    enabled = !laeuft,
                    modifier = Modifier.testTag("request-grant-${antrag.id}"),
                ) {
                    Text(stringResource(R.string.traeger_grant))
                }
                OutlinedButton(
                    onClick = { onDecide(antrag, "abgelehnt") },
                    enabled = !laeuft,
                    modifier = Modifier.testTag("request-reject-${antrag.id}"),
                ) {
                    Text(stringResource(R.string.traeger_reject))
                }
            }
        }
    }
}

/** „Ich will mitmachen" — mit einem Satz dazu, warum. */
@Composable
private fun MitmachenDialog(
    traeger: TraegerDto,
    laeuft: Boolean,
    onDismiss: () -> Unit,
    onSend: (String) -> Unit,
) {
    var grund by remember { mutableStateOf("") }
    AlertDialog(
        onDismissRequest = onDismiss,
        modifier = Modifier.testTag("traeger-join-dialog"),
        title = { Text(stringResource(R.string.traeger_join_heading)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(stringResource(R.string.traeger_join_intro, traeger.name))
                OutlinedTextField(
                    value = grund,
                    onValueChange = { grund = it },
                    placeholder = { Text(stringResource(R.string.traeger_join_placeholder)) },
                    minLines = 2,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("join-reason"),
                )
            }
        },
        confirmButton = {
            Button(
                onClick = { onSend(grund) },
                enabled = !laeuft,
                modifier = Modifier.testTag("join-send"),
            ) {
                Text(stringResource(R.string.traeger_join_send))
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) }
        },
    )
}

/** Jemanden ohne Antrag aufnehmen — bei einer geschlossenen Gruppe der
 *  einzige Weg hinein. */
@Composable
private fun AufnahmeDialog(
    villagers: List<MemberDto>,
    onDismiss: () -> Unit,
    onPick: (MemberDto) -> Unit,
) {
    var suche by remember { mutableStateOf("") }
    val treffer = remember(suche, villagers) {
        val begriff = suche.trim().lowercase()
        if (begriff.isBlank()) {
            villagers
        } else {
            villagers.filter { TraegerViewModel.anzeigename(it).lowercase().contains(begriff) }
        }
    }
    AlertDialog(
        onDismissRequest = onDismiss,
        modifier = Modifier.testTag("traeger-add-member-dialog"),
        title = { Text(stringResource(R.string.traeger_add_member)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(
                    stringResource(R.string.traeger_add_member_hint),
                    style = MaterialTheme.typography.bodySmall,
                )
                OutlinedTextField(
                    value = suche,
                    onValueChange = { suche = it },
                    label = { Text(stringResource(R.string.traeger_add_member_search)) },
                    singleLine = true,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("add-member-search"),
                )
                if (villagers.isEmpty()) {
                    Text(
                        stringResource(R.string.traeger_add_member_loading),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                LazyColumn(
                    modifier = Modifier.heightIn(max = 260.dp),
                    verticalArrangement = Arrangement.spacedBy(4.dp),
                ) {
                    items(treffer, key = { it.userSub }) { person ->
                        TextButton(
                            onClick = { onPick(person) },
                            modifier = Modifier
                                .fillMaxWidth()
                                .testTag("add-member-${person.userSub}"),
                        ) {
                            Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically) {
                                Icon(Icons.Filled.Group, contentDescription = null, modifier = Modifier.size(18.dp))
                                Spacer(Modifier.width(10.dp))
                                Text(TraegerViewModel.anzeigename(person))
                            }
                        }
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = onDismiss) { Text(stringResource(R.string.cancel)) }
        },
    )
}
