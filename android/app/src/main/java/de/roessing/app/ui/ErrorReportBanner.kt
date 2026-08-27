package de.roessing.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.errors.ErrorIncident
import de.roessing.app.errors.ErrorReportKind
import de.roessing.app.errors.ErrorReportUiState
import de.roessing.app.errors.ErrorReporter
import kotlinx.coroutines.launch

/**
 * Die eine Stelle, an der die App sagt „das hat nicht geklappt" — und
 * anbietet, etwas dagegen zu tun.
 *
 * Sitzt an der Wurzel, nicht in einem einzelnen Bereich: Ein Fehler kann
 * überall passieren, auch schon auf dem Anmeldeschirm, und niemand soll erst
 * den richtigen Schirm suchen müssen, um davon zu erfahren.
 *
 * Zwei Knöpfe, und der wichtige ist ein einziger Tipp:
 *
 *  - **Bericht schicken** geht sofort hinaus. Niemand muss etwas beschreiben —
 *    genau darum ging es.
 *  - **Dazuschreiben** öffnet das Blatt für alle, die sagen wollen, was sie
 *    gerade gemacht haben — und zeigt Zeile für Zeile, was das Telefon
 *    verlässt.
 *
 * Wortgleich mit dem `Fehlerbanner` der iOS-App: Ein Bericht vom iPhone und
 * einer vom Android-Telefon sollen in derselben Liste dasselbe heißen.
 */
@Composable
fun ErrorReportBanner(
    state: ErrorReportUiState,
    onSend: (String) -> Unit,
    onDismiss: () -> Unit,
    contentLines: (ErrorIncident, String) -> List<Pair<String, String>>,
    modifier: Modifier = Modifier,
    /** Noch einmal versuchen — nur gesetzt, wenn ein neuer Abruf etwas bringt. */
    onRetry: (() -> Unit)? = null,
) {
    val vorfall = state.vorfall ?: return
    var blattOffen by rememberSaveable { mutableStateOf(false) }

    Card(
        modifier = modifier
            .fillMaxWidth()
            .padding(12.dp)
            .testTag("fehler-banner"),
        colors = CardDefaults.elevatedCardColors(),
        elevation = CardDefaults.elevatedCardElevation(),
    ) {
        Column(
            Modifier.padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            if (state.gesendet) {
                Text(
                    stringResource(R.string.error_report_thanks),
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.testTag("fehler-banner-dank"),
                )
                OutlinedButton(
                    onClick = onDismiss,
                    modifier = Modifier.testTag("fehler-banner-schliessen"),
                ) { Text(stringResource(R.string.error_report_close)) }
                return@Column
            }

            Row(verticalAlignment = Alignment.Top) {
                // Farbe allein ist nie die Information: Das Zeichen steht
                // neben dem Text, nicht an seiner Stelle.
                Icon(
                    Icons.Filled.Warning,
                    contentDescription = null,
                    modifier = Modifier.size(20.dp),
                )
                Text(
                    vorfall.message,
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier
                        .weight(1f)
                        .padding(start = 8.dp)
                        .testTag("fehler-banner-meldung"),
                )
                IconButton(
                    onClick = onDismiss,
                    modifier = Modifier.testTag("fehler-banner-schliessen"),
                ) {
                    Icon(
                        Icons.Filled.Close,
                        contentDescription = stringResource(R.string.error_report_dismiss),
                    )
                }
            }

            state.sendefehler?.let { fehler ->
                Text(
                    fehler,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.testTag("fehler-banner-sendefehler"),
                )
            }

            if (onRetry != null && vorfall.kind == ErrorReportKind.NETWORK) {
                // Wer eben noch angemeldet war, sucht den Fehler sonst bei
                // sich. Die Anmeldung gilt weiter — das gehört dazu.
                Text(
                    stringResource(R.string.error_offline_signed_in),
                    style = MaterialTheme.typography.bodySmall,
                    modifier = Modifier.testTag("fehler-banner-bleibt-angemeldet"),
                )
            }

            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                if (onRetry != null) {
                    TextButton(
                        onClick = {
                            onRetry()
                            onDismiss()
                        },
                        enabled = !state.sendet,
                        modifier = Modifier.testTag("fehler-erneut-versuchen"),
                    ) { Text(stringResource(R.string.error_report_retry)) }
                }
                TextButton(
                    onClick = { onSend("") },
                    enabled = !state.sendet,
                    modifier = Modifier.testTag("fehler-bericht-schicken"),
                ) {
                    if (state.sendet) {
                        CircularProgressIndicator(Modifier.size(16.dp))
                        Text(
                            stringResource(R.string.error_report_sending),
                            Modifier.padding(start = 8.dp),
                        )
                    } else {
                        Text(stringResource(R.string.error_report_send))
                    }
                }
                TextButton(
                    onClick = { blattOffen = true },
                    enabled = !state.sendet,
                    modifier = Modifier.testTag("fehler-bericht-dazuschreiben"),
                ) { Text(stringResource(R.string.error_report_add)) }
            }
        }
    }

    if (blattOffen) {
        ErrorReportDialog(
            vorfall = vorfall,
            state = state,
            contentLines = contentLines,
            onSend = { kommentar ->
                blattOffen = false
                onSend(kommentar)
            },
            onCancel = { blattOffen = false },
        )
    }
}

/**
 * „Dazuschreiben": ein freiwilliger Satz — und vor allem die vollständige
 * Aufstellung dessen, was das Telefon verlässt. Kein Versprechen darüber:
 * Die Zeilen entstehen aus genau den Werten, aus denen die Anfrage gebaut wird.
 */
@Composable
private fun ErrorReportDialog(
    vorfall: ErrorIncident,
    state: ErrorReportUiState,
    contentLines: (ErrorIncident, String) -> List<Pair<String, String>>,
    onSend: (String) -> Unit,
    onCancel: () -> Unit,
) {
    var kommentar by rememberSaveable { mutableStateOf("") }

    AlertDialog(
        onDismissRequest = onCancel,
        title = { Text(stringResource(R.string.error_report_dialog_title)) },
        text = {
            Column(
                Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp),
            ) {
                Text(
                    stringResource(R.string.error_report_what_happened),
                    style = MaterialTheme.typography.labelLarge,
                )
                Text(vorfall.message, style = MaterialTheme.typography.bodyMedium)

                OutlinedTextField(
                    value = kommentar,
                    onValueChange = { kommentar = it },
                    label = { Text(stringResource(R.string.error_report_comment_label)) },
                    placeholder = { Text(stringResource(R.string.error_report_comment_hint)) },
                    minLines = 3,
                    modifier = Modifier
                        .fillMaxWidth()
                        .testTag("feld-kommentar"),
                )
                Text(
                    stringResource(R.string.error_report_comment_help),
                    style = MaterialTheme.typography.bodySmall,
                )

                Text(
                    stringResource(R.string.error_report_contents),
                    style = MaterialTheme.typography.labelLarge,
                )
                Column(Modifier.testTag("bericht-inhalt")) {
                    contentLines(vorfall, kommentar).forEach { (titel, wert) ->
                        Row(Modifier.fillMaxWidth().padding(vertical = 2.dp)) {
                            Text(
                                titel,
                                style = MaterialTheme.typography.bodySmall,
                                modifier = Modifier.weight(0.4f),
                            )
                            Text(
                                wert,
                                style = MaterialTheme.typography.bodySmall,
                                textAlign = TextAlign.End,
                                modifier = Modifier.weight(0.6f),
                            )
                        }
                    }
                }
                Text(
                    stringResource(R.string.error_report_privacy),
                    style = MaterialTheme.typography.bodySmall,
                )
            }
        },
        confirmButton = {
            TextButton(
                onClick = { onSend(kommentar) },
                enabled = !state.sendet,
                modifier = Modifier.testTag("bericht-abschicken"),
            ) { Text(stringResource(R.string.error_report_submit)) }
        },
        dismissButton = {
            TextButton(
                onClick = onCancel,
                modifier = Modifier.testTag("bericht-abbrechen"),
            ) { Text(stringResource(R.string.error_report_cancel)) }
        },
    )
}

/**
 * Hängt den Hinweis an die Wurzel der App. Nimmt dem Aufrufer das
 * Einsammeln des Zustands und den Nebenläufigkeitsbereich ab, damit an der
 * Wurzel eine Zeile genügt.
 */
@Composable
fun ErrorReportBannerHost(
    melder: ErrorReporter,
    modifier: Modifier = Modifier,
    onRetry: (() -> Unit)? = null,
) {
    val state by melder.state.collectAsState()
    val bereich = rememberCoroutineScope()
    ErrorReportBanner(
        state = state,
        onSend = { kommentar -> bereich.launch { melder.send(kommentar) } },
        onDismiss = melder::dismiss,
        contentLines = { vorfall, kommentar ->
            melder.contentLines(melder.inputFor(vorfall, kommentar), vorfall.occurredAt)
        },
        modifier = modifier,
        onRetry = onRetry,
    )
}
