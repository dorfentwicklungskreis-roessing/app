package de.roessing.app.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import de.roessing.app.R

/**
 * Where administration happens now that it has left the app: the MCP server
 * and the web administration. Both are addresses someone has to reach, so
 * both are spelled out and both can be tapped.
 */
enum class AdminLink(val url: String) {
    /** Entered in claude.ai as a custom connector. */
    MCP("https://app.xn--rssing-wxa.de/mcp"),

    /** The web administration — the same things, to click. */
    WEB("https://app.xn--rssing-wxa.de/admin/"),
}

/**
 * The section that took the place of the old "Verwaltung" tile.
 *
 * Administration runs entirely on the MCP server and in the web
 * administration; the app no longer edits places and tasks. Whoever is
 * allowed to administer should not hit an empty spot but learn where it
 * happens and how to get there — hence a hint that advertises the two ways
 * instead of merely announcing a removal.
 *
 * Shown only to accounts holding the role "admin". Everyone else has no
 * business here and would only be confused.
 */
@Composable
fun AdminHint(modifier: Modifier = Modifier) {
    val context = LocalContext.current
    AdminHintCard(modifier = modifier, onOpen = { link -> context.openInCustomTab(link.url) })
}

/** The display on its own — without a context, so it can be tested. */
@Composable
fun AdminHintCard(
    onOpen: (AdminLink) -> Unit,
    modifier: Modifier = Modifier,
) {
    Card(
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
        ),
        modifier = modifier
            .fillMaxWidth()
            .testTag("admin-hint"),
    ) {
        Column(
            Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Text(
                stringResource(R.string.admin_hint_title),
                style = MaterialTheme.typography.titleMedium,
            )
            Text(
                stringResource(R.string.admin_hint_intro),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            // The way through Claude comes first: it is the one that also
            // works while standing in front of the flower box.
            Text(
                stringResource(R.string.admin_hint_claude_title),
                style = MaterialTheme.typography.titleSmall,
            )
            Text(
                stringResource(R.string.admin_hint_claude_setup),
                style = MaterialTheme.typography.bodyMedium,
            )
            AddressButton(AdminLink.MCP, "admin-hint-mcp", onOpen)
            Text(
                stringResource(R.string.admin_hint_claude_can),
                style = MaterialTheme.typography.bodyMedium,
            )
            Text(
                stringResource(R.string.admin_hint_claude_benefit),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.primary,
                modifier = Modifier.testTag("admin-hint-benefit"),
            )

            Text(
                stringResource(R.string.admin_hint_web_title),
                style = MaterialTheme.typography.titleSmall,
            )
            Text(
                stringResource(R.string.admin_hint_web_body),
                style = MaterialTheme.typography.bodyMedium,
            )
            AddressButton(AdminLink.WEB, "admin-hint-web", onOpen)
        }
    }
}

/**
 * The address as it is: readable, so it can be typed off a second device,
 * and tappable, so it does not have to be.
 */
@Composable
private fun AddressButton(link: AdminLink, testTag: String, onOpen: (AdminLink) -> Unit) {
    TextButton(
        onClick = { onOpen(link) },
        modifier = Modifier.testTag(testTag),
    ) {
        Text(link.url, style = MaterialTheme.typography.bodyMedium)
    }
}
