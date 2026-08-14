package de.roessing.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.unit.dp
import de.roessing.app.R

/**
 * Eigenes Profil: Angaben zur Person, jede mit einem Schalter „für alle
 * sichtbar".
 *
 * Der Hinweis, was andere zu sehen bekommen, steht ganz oben und in voller
 * Größe — nicht im Kleingedruckten. Darunter zeigt eine laufend
 * aktualisierte Zeile, was gerade tatsächlich freigegeben ist.
 */
@Composable
fun ProfileScreen(
    state: ProfileUiState,
    modifier: Modifier = Modifier,
    onDisplayName: (String) -> Unit,
    onNickname: (String) -> Unit,
    onPhone: (String) -> Unit,
    onEmail: (String) -> Unit,
    onNote: (String) -> Unit,
    onDisplayNamePublic: (Boolean) -> Unit,
    onNicknamePublic: (Boolean) -> Unit,
    onPhonePublic: (Boolean) -> Unit,
    onEmailPublic: (Boolean) -> Unit,
    onNotePublic: (Boolean) -> Unit,
    onSave: () -> Unit,
) {
    Column(
        modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState())
            .padding(16.dp)
            .testTag("profil"),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        if (state.loading) LinearProgressIndicator(Modifier.fillMaxWidth())

        Sichtbarkeitshinweis(state)

        state.error?.let { text ->
            Card(
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.errorContainer),
                modifier = Modifier.fillMaxWidth().testTag("profil-fehler"),
            ) {
                Text(
                    text,
                    Modifier.padding(16.dp),
                    color = MaterialTheme.colorScheme.onErrorContainer,
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
        }

        Feld(
            tag = "anzeigename",
            label = stringResource(R.string.profile_display_name),
            hint = stringResource(R.string.profile_display_name_hint),
            value = state.displayName, onValue = onDisplayName,
            public = state.displayNamePublic, onPublic = onDisplayNamePublic,
        )
        Feld(
            tag = "nickname",
            label = stringResource(R.string.profile_nickname),
            hint = stringResource(R.string.profile_nickname_hint),
            value = state.nickname, onValue = onNickname,
            public = state.nicknamePublic, onPublic = onNicknamePublic,
        )
        Feld(
            tag = "telefon",
            label = stringResource(R.string.profile_phone),
            hint = stringResource(R.string.profile_phone_hint),
            value = state.phone, onValue = onPhone,
            public = state.phonePublic, onPublic = onPhonePublic,
            keyboard = KeyboardType.Phone,
        )
        Feld(
            tag = "email",
            label = stringResource(R.string.profile_email),
            hint = stringResource(R.string.profile_email_hint),
            value = state.email, onValue = onEmail,
            public = state.emailPublic, onPublic = onEmailPublic,
            keyboard = KeyboardType.Email,
        )
        Feld(
            tag = "notiz",
            label = stringResource(R.string.profile_note),
            hint = stringResource(R.string.profile_note_hint),
            value = state.note, onValue = onNote,
            public = state.notePublic, onPublic = onNotePublic,
        )

        Button(
            onClick = onSave,
            enabled = !state.saving,
            modifier = Modifier.fillMaxWidth().testTag("profil-speichern"),
        ) {
            Text(stringResource(R.string.profile_save))
        }
    }
}

/** Der Hinweis, wer was sieht — bewusst prominent und in Warnfarbe. */
@Composable
private fun Sichtbarkeitshinweis(state: ProfileUiState) {
    Card(
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.tertiaryContainer),
        modifier = Modifier.fillMaxWidth().testTag("sichtbarkeitshinweis"),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(
                stringResource(R.string.profile_visibility_headline),
                style = MaterialTheme.typography.titleMedium,
                fontWeight = FontWeight.Bold,
                color = MaterialTheme.colorScheme.onTertiaryContainer,
            )
            Text(
                stringResource(R.string.profile_visibility_text),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onTertiaryContainer,
            )
            Text(
                stringResource(R.string.profile_visibility_name_hint),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onTertiaryContainer,
            )
            HorizontalDivider()
            val freigegeben = state.publicFields
            Text(
                if (freigegeben.isEmpty()) stringResource(R.string.profile_visibility_none)
                else stringResource(R.string.profile_visibility_now, freigegeben.joinToString(", ")),
                style = MaterialTheme.typography.bodyMedium,
                fontWeight = FontWeight.SemiBold,
                color = MaterialTheme.colorScheme.onTertiaryContainer,
                modifier = Modifier.testTag("sichtbar-jetzt"),
            )
        }
    }
}

/** Ein Eingabefeld mit seinem Sichtbarkeits-Schalter. */
@Composable
private fun Feld(
    tag: String,
    label: String,
    hint: String,
    value: String,
    onValue: (String) -> Unit,
    public: Boolean,
    onPublic: (Boolean) -> Unit,
    keyboard: KeyboardType = KeyboardType.Text,
) {
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            OutlinedTextField(
                value = value,
                onValueChange = onValue,
                label = { Text(label) },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = keyboard),
                modifier = Modifier.fillMaxWidth().testTag("feld-$tag"),
            )
            Text(hint, style = MaterialTheme.typography.bodySmall)
            Row(
                Modifier.fillMaxWidth().padding(top = 4.dp),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    stringResource(R.string.profile_public_switch),
                    style = MaterialTheme.typography.bodyMedium,
                )
                Switch(
                    checked = public,
                    onCheckedChange = onPublic,
                    modifier = Modifier.testTag("sicht-$tag"),
                )
            }
        }
    }
}
