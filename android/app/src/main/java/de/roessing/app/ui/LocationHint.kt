package de.roessing.app.ui

import androidx.compose.foundation.layout.Column
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier

/**
 * Kurze Begründung, bevor der System-Dialog nach dem Standort fragt.
 * Der Standort bleibt auf dem Gerät und geht nie ans Backend.
 */
@Composable
fun LocationHint(onRequest: () -> Unit, modifier: Modifier = Modifier) {
    Column(modifier) {
    }
}
