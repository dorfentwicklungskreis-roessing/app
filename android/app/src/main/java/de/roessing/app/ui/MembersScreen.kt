package de.roessing.app.ui

import android.content.Intent
import android.net.Uri
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AssistChip
import androidx.compose.material3.AssistChipDefaults
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.data.MemberDto

/**
 * Wer macht mit: alle, die ein Profil haben, mit genau den Angaben, die sie
 * freigegeben haben. Telefonnummer und E-Mail sind antippbar.
 *
 * Was hier fehlt, hat die Person nicht freigegeben — das Backend schickt es
 * gar nicht erst mit.
 */
@Composable
fun MembersScreen(
    state: ProfileUiState,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    Column(
        modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState())
            .padding(16.dp)
            .testTag("dorfbewohner"),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        if (state.membersLoading) LinearProgressIndicator(Modifier.fillMaxWidth())
        Text(stringResource(R.string.members_intro), style = MaterialTheme.typography.bodyMedium)

        if (state.adminView) {
            Card(
                shape = MaterialTheme.shapes.large,
                colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.tertiaryContainer),
                modifier = Modifier.fillMaxWidth().testTag("verwaltungs-hinweis"),
            ) {
                Text(
                    stringResource(R.string.members_admin_hint),
                    Modifier.padding(16.dp),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onTertiaryContainer,
                )
            }
        }

        if (!state.membersLoading && state.members.isEmpty()) {
            Text(
                stringResource(R.string.members_empty),
                Modifier.testTag("dorfbewohner-leer"),
                style = MaterialTheme.typography.bodyMedium,
            )
        }

        state.members.forEach { m ->
            Card(
                Modifier.fillMaxWidth().testTag("bewohner-${m.userSub}"),
                shape = MaterialTheme.shapes.large,
                colors = CardDefaults.cardColors(
                    containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
                ),
            ) {
                Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                    Text(
                        m.name,
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.SemiBold,
                    )
                    if (m.nickname.isNotBlank() && m.displayName.isNotBlank() && m.nickname != m.displayName) {
                        Zeile(m.displayName, m.nurFuerVerwaltung("displayName"))
                    }
                    if (m.phone.isNotBlank()) {
                        Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
                            AssistChip(
                                onClick = { context.oeffne(Intent.ACTION_DIAL, "tel:${m.phone}") },
                                label = { Text(m.phone) },
                                colors = AssistChipDefaults.assistChipColors(),
                                modifier = Modifier.testTag("anrufen-${m.userSub}"),
                            )
                            if (m.nurFuerVerwaltung("phone")) NurVerwaltung()
                        }
                    }
                    if (m.email.isNotBlank()) {
                        Row(verticalAlignment = androidx.compose.ui.Alignment.CenterVertically) {
                            AssistChip(
                                onClick = { context.oeffne(Intent.ACTION_SENDTO, "mailto:${m.email}") },
                                label = { Text(m.email) },
                                modifier = Modifier.testTag("mailen-${m.userSub}"),
                            )
                            if (m.nurFuerVerwaltung("email")) NurVerwaltung()
                        }
                    }
                    if (m.note.isNotBlank()) Zeile(m.note, m.nurFuerVerwaltung("note"))
                }
            }
        }
    }
}

@Composable
private fun Zeile(text: String, nurVerwaltung: Boolean) {
    Row(
        Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        verticalAlignment = androidx.compose.ui.Alignment.CenterVertically,
    ) {
        Text(text, style = MaterialTheme.typography.bodyMedium)
        if (nurVerwaltung) NurVerwaltung()
    }
}

/** Kennzeichnet Angaben, die die Person nicht für alle freigegeben hat. */
@Composable
private fun NurVerwaltung() {
    Text(
        stringResource(R.string.members_only_admins),
        style = MaterialTheme.typography.labelSmall,
        color = MaterialTheme.colorScheme.error,
        modifier = Modifier.padding(start = 8.dp).testTag("nur-verwaltung"),
    )
}

/**
 * Öffnet Telefon- oder E-Mail-App. Fehlt eine (Emulator ohne Telefon-App),
 * passiert nichts — ein Absturz wäre hier die schlechteste Antwort.
 */
private fun android.content.Context.oeffne(aktion: String, uri: String) {
    runCatching {
        startActivity(Intent(aktion, Uri.parse(uri)).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
    }
}
