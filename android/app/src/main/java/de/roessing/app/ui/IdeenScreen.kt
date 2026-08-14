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
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Send
import androidx.compose.material.icons.outlined.Info
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import de.roessing.app.R

/**
 * Das Ideen-Formular in der App: ein mehrzeiliges Wunschfeld (Pflicht),
 * darunter Name und E-Mail — beides freiwillig und aus dem Profil
 * vorbelegt. Abgeschickt wird an denselben Eingang wie von der Website;
 * weil hier jemand angemeldet ist, hängt die Idee danach am Konto.
 *
 * Fehler kosten nie den getippten Text: Bei einer Ablehnung bleibt alles
 * stehen und die Begründung des Backends steht wörtlich darüber.
 */
@Composable
fun IdeenScreen(
    state: IdeenUiState,
    modifier: Modifier = Modifier,
    onWunsch: (String) -> Unit,
    onName: (String) -> Unit,
    onEmail: (String) -> Unit,
    onSenden: () -> Unit,
) {
    Column(
        modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp)
            .padding(top = 8.dp, bottom = 28.dp)
            .testTag("ideen-formular"),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                stringResource(R.string.ideas_intro),
                style = MaterialTheme.typography.bodyLarge,
            )
            Text(
                stringResource(R.string.ideas_encourage),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        if (state.fehler != null) {
            Card(
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.errorContainer,
                    contentColor = MaterialTheme.colorScheme.onErrorContainer,
                ),
                modifier = Modifier.fillMaxWidth().testTag("ideen-fehler"),
            ) {
                Text(state.fehler, Modifier.padding(16.dp), style = MaterialTheme.typography.bodyMedium)
            }
        }

        OutlinedTextField(
            value = state.wunsch,
            onValueChange = onWunsch,
            label = { Text(stringResource(R.string.ideas_wish_label)) },
            placeholder = { Text(stringResource(R.string.ideas_wish_placeholder)) },
            supportingText = {
                Text(
                    stringResource(
                        R.string.ideas_counter,
                        state.wunsch.length,
                        IdeenUiState.MAX_ZEICHEN,
                    ),
                )
            },
            minLines = 5,
            keyboardOptions = KeyboardOptions(
                capitalization = KeyboardCapitalization.Sentences,
                imeAction = ImeAction.Default,
            ),
            modifier = Modifier
                .fillMaxWidth()
                .heightIn(min = 140.dp)
                .testTag("feld-wunsch"),
        )

        Text(
            stringResource(R.string.ideas_prefilled_hint),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        OutlinedTextField(
            value = state.name,
            onValueChange = onName,
            singleLine = true,
            label = { Text(stringResource(R.string.ideas_name_label)) },
            keyboardOptions = KeyboardOptions(
                capitalization = KeyboardCapitalization.Words,
                imeAction = ImeAction.Next,
            ),
            modifier = Modifier.fillMaxWidth().testTag("feld-name"),
        )

        OutlinedTextField(
            value = state.email,
            onValueChange = onEmail,
            singleLine = true,
            label = { Text(stringResource(R.string.ideas_email_label)) },
            supportingText = { Text(stringResource(R.string.ideas_email_help)) },
            keyboardOptions = KeyboardOptions(
                keyboardType = KeyboardType.Email,
                imeAction = ImeAction.Done,
            ),
            modifier = Modifier.fillMaxWidth().testTag("feld-email"),
        )

        // Der Datenschutzhinweis steht direkt am Formular, nicht im
        // Kleingedruckten — hier wird gerade etwas über eine Person gespeichert.
        Card(
            colors = CardDefaults.cardColors(
                containerColor = MaterialTheme.colorScheme.surfaceContainer,
            ),
            modifier = Modifier.fillMaxWidth().testTag("ideen-datenschutz"),
        ) {
            Row(Modifier.padding(16.dp), verticalAlignment = Alignment.Top) {
                Icon(
                    Icons.Outlined.Info,
                    contentDescription = null,
                    tint = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.size(20.dp),
                )
                Spacer(Modifier.width(12.dp))
                Text(
                    stringResource(R.string.ideas_privacy),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }

        Button(
            onClick = onSenden,
            enabled = state.absendbar,
            modifier = Modifier.fillMaxWidth().testTag("idee-absenden"),
        ) {
            if (state.sendet) {
                CircularProgressIndicator(
                    Modifier.size(18.dp),
                    strokeWidth = 2.dp,
                    color = MaterialTheme.colorScheme.onPrimary,
                )
                Spacer(Modifier.width(12.dp))
                Text(stringResource(R.string.ideas_sending))
            } else {
                Icon(Icons.Filled.Send, contentDescription = null, modifier = Modifier.size(18.dp))
                Spacer(Modifier.width(12.dp))
                Text(stringResource(R.string.ideas_submit))
            }
        }
    }
}
