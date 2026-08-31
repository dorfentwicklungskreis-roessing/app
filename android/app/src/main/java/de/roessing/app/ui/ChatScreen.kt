package de.roessing.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilledIconButton
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.unit.dp
import de.roessing.app.R

/**
 * Der Chat: in normalem Deutsch fragen, was im Dorf ansteht — und es tun.
 *
 * Die Ansicht hält sich bewusst zurück. Was hier steht, kommt aus dem
 * Dorfserver, und was jemand sehen und tun darf, entscheidet ausschließlich
 * das Backend. Deshalb wird eine Absage im Wortlaut gezeigt, statt sie in
 * einen eigenen Satz zu übersetzen — der wäre entweder ungenauer oder
 * schlicht erfunden.
 *
 * Unter jeder Antwort stehen die befragten Werkzeuge. Wer liest, dass
 * „orte_liste" befragt wurde, weiß, dass die Zahl aus dem Dorf kommt.
 */
@Composable
fun ChatScreen(
    state: ChatUiState,
    modifier: Modifier = Modifier,
    onEingabe: (String) -> Unit,
    onSenden: () -> Unit,
) {
    val liste = rememberLazyListState()
    // Nach jedem neuen Zug — und wenn das Warten beginnt — nach unten rollen.
    // Sonst schreibt die App ins Unsichtbare. Gezaehlt wird ueber die
    // Einleitung (Eintrag 0) hinweg; Warte- und Fehlerzeile haengen hinten
    // dran und sollen genauso zu sehen sein.
    val letzterEintrag = state.zuege.size +
        (if (state.wartet) 1 else 0) +
        (if (state.fehler != null) 1 else 0)
    LaunchedEffect(letzterEintrag) {
        if (letzterEintrag > 0) liste.animateScrollToItem(letzterEintrag)
    }

    Column(modifier.fillMaxSize().imePadding()) {
        when {
            state.laedtStand -> Box(
                Modifier.fillMaxSize(),
                contentAlignment = Alignment.Center,
            ) { CircularProgressIndicator() }

            // Kein Schlüssel hinterlegt: sagen, was los ist, und niemanden
            // ins Leere tippen lassen.
            !state.verfuegbar -> NichtEingerichtet(
                hinweis = state.hinweis.ifBlank { stringResource(R.string.chat_unavailable) },
                modifier = Modifier.fillMaxSize(),
            )

            else -> {
                LazyColumn(
                    state = liste,
                    modifier = Modifier
                        .weight(1f)
                        .fillMaxWidth()
                        .testTag("chat-liste"),
                    contentPadding = PaddingValues(horizontal = 16.dp, vertical = 12.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp),
                ) {
                    item {
                        Einleitung(leer = state.zuege.isEmpty())
                    }
                    items(state.zuege) { zug -> Sprechblase(zug) }
                    if (state.wartet) {
                        item { Denkt() }
                    }
                    if (state.fehler != null) {
                        item { Fehlerzeile(state.fehler) }
                    }
                }
                Eingabezeile(
                    text = state.eingabe,
                    absendbar = state.absendbar,
                    onEingabe = onEingabe,
                    onSenden = onSenden,
                )
            }
        }
    }
}

/**
 * Die Einleitung steht über dem Gespräch und bleibt dort. Sie sagt, was hier
 * geht — ein leeres Feld mit blinkendem Strich sagt das nicht.
 */
@Composable
private fun Einleitung(leer: Boolean) {
    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Text(
            stringResource(R.string.chat_intro),
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        if (leer) {
            Text(
                stringResource(R.string.chat_examples),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.testTag("chat-leer"),
            )
        }
    }
}

/** Ein Zug des Gesprächs — meine Fragen rechts, die Antworten links. */
@Composable
private fun Sprechblase(zug: ChatZug) {
    val meins = zug.rolle == ChatRolle.ICH
    val hintergrund =
        if (meins) MaterialTheme.colorScheme.primaryContainer
        else MaterialTheme.colorScheme.surfaceContainerHigh
    val vordergrund =
        if (meins) MaterialTheme.colorScheme.onPrimaryContainer
        else MaterialTheme.colorScheme.onSurface

    Row(
        Modifier.fillMaxWidth(),
        horizontalArrangement = if (meins) Arrangement.End else Arrangement.Start,
    ) {
        Column(
            Modifier
                .widthIn(max = 320.dp)
                .background(hintergrund, RoundedCornerShape(18.dp))
                .padding(horizontal = 14.dp, vertical = 10.dp)
                .testTag(if (meins) "chat-zug-ich" else "chat-zug-app"),
            verticalArrangement = Arrangement.spacedBy(4.dp),
        ) {
            Text(zug.text, style = MaterialTheme.typography.bodyMedium, color = vordergrund)
            if (zug.abgebrochen) {
                Text(
                    stringResource(R.string.chat_incomplete),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            if (zug.werkzeuge.isNotEmpty()) {
                Text(
                    stringResource(R.string.chat_sources, zug.werkzeuge.joinToString(", ")),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag("chat-werkzeuge"),
                )
            }
        }
    }
}

/**
 * Die Antwort ist unterwegs. Sie darf eine halbe Minute dauern — deshalb
 * steht hier nicht nur ein Rädchen, sondern auch, worauf gewartet wird.
 */
@Composable
private fun Denkt() {
    Row(
        Modifier.fillMaxWidth().testTag("chat-wartet"),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp)
        Spacer(Modifier.width(10.dp))
        Text(
            stringResource(R.string.chat_thinking),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** Der Klartext des Backends, unverändert. */
@Composable
private fun Fehlerzeile(text: String) {
    Card(
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.errorContainer,
            contentColor = MaterialTheme.colorScheme.onErrorContainer,
        ),
        modifier = Modifier.fillMaxWidth().testTag("chat-fehler"),
    ) {
        Text(text, Modifier.padding(14.dp), style = MaterialTheme.typography.bodyMedium)
    }
}

@Composable
private fun NichtEingerichtet(hinweis: String, modifier: Modifier = Modifier) {
    Box(modifier.padding(24.dp), contentAlignment = Alignment.Center) {
        Card(
            colors = CardDefaults.cardColors(
                containerColor = MaterialTheme.colorScheme.surfaceContainer,
            ),
            modifier = Modifier.fillMaxWidth().testTag("chat-hinweis"),
        ) {
            Row(Modifier.padding(20.dp), verticalAlignment = Alignment.Top) {
                Icon(
                    Icons.Outlined.Info,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.size(20.dp),
                )
                Spacer(Modifier.width(12.dp))
                Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(hinweis, style = MaterialTheme.typography.bodyMedium)
                    Text(
                        stringResource(R.string.chat_unavailable_hint),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
        }
    }
}

@Composable
private fun Eingabezeile(
    text: String,
    absendbar: Boolean,
    onEingabe: (String) -> Unit,
    onSenden: () -> Unit,
) {
    Row(
        Modifier
            .fillMaxWidth()
            .padding(horizontal = 12.dp, vertical = 8.dp),
        verticalAlignment = Alignment.Bottom,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        OutlinedTextField(
            value = text,
            onValueChange = onEingabe,
            placeholder = { Text(stringResource(R.string.chat_input_placeholder)) },
            maxLines = 5,
            keyboardOptions = KeyboardOptions(
                capitalization = KeyboardCapitalization.Sentences,
                imeAction = ImeAction.Default,
            ),
            modifier = Modifier.weight(1f).testTag("chat-eingabe"),
        )
        FilledIconButton(
            onClick = onSenden,
            enabled = absendbar,
            modifier = Modifier.testTag("chat-senden"),
        ) {
            Icon(
                Icons.AutoMirrored.Filled.Send,
                contentDescription = stringResource(R.string.chat_send),
            )
        }
    }
}
