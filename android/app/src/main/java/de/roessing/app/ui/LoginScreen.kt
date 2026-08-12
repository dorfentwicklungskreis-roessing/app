package de.roessing.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.auth.AuthManager

/** Startbildschirm: Anmeldung mit der Rössing-ID (OAuth im Browser). */
@Composable
fun LoginScreen(
    /** Technisches Fehlerkürzel oder null, wenn kein Fehler vorliegt. */
    errorCode: String? = null,
    onLogin: () -> Unit,
    onDevLogin: (asAdmin: Boolean) -> Unit,
) {
    Surface(Modifier.fillMaxSize()) {
        Column(
            Modifier
                .fillMaxSize()
                .padding(32.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.Center,
        ) {
            Text("🌻", style = MaterialTheme.typography.displayLarge)
            Spacer(Modifier.height(16.dp))
            Text(
                stringResource(R.string.login_title),
                style = MaterialTheme.typography.headlineMedium,
                textAlign = TextAlign.Center,
            )
            Spacer(Modifier.height(8.dp))
            Text(
                stringResource(R.string.login_subtitle),
                style = MaterialTheme.typography.bodyLarge,
                textAlign = TextAlign.Center,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Spacer(Modifier.height(32.dp))
            Button(
                onClick = onLogin,
                modifier = Modifier
                    .fillMaxWidth()
                    .testTag("login-button"),
            ) {
                Text(stringResource(R.string.login_button))
            }
            if (errorCode != null) {
                Spacer(Modifier.height(12.dp))
                Text(
                    stringResource(R.string.login_error, errorCode),
                    color = MaterialTheme.colorScheme.error,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.testTag("login-error"),
                )
            }
            if (AuthManager.isDevAuthAllowed()) {
                Spacer(Modifier.height(24.dp))
                TextButton(
                    onClick = { onDevLogin(true) },
                    modifier = Modifier.testTag("dev-login-button"),
                ) {
                    Text(stringResource(R.string.login_dev_button))
                }
            }
        }
    }
}
