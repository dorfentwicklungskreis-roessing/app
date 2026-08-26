package de.roessing.app.ui

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.os.Build
import android.widget.Toast
import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.OpenInNew
import androidx.compose.material.icons.filled.ContentCopy
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import de.roessing.app.R

/**
 * Where administration happens now that it has left the app: the MCP server
 * and the web administration. Both are addresses someone has to reach, so
 * both are spelled out — but they are reached in entirely different ways.
 */
enum class AdminAddress(val url: String) {
    /**
     * The MCP endpoint, entered in claude.ai as a custom connector.
     *
     * Copied, never opened: `/mcp` speaks MCP over Streamable HTTP and is no
     * web page — a browser shows nothing there but an error message. The
     * address is needed somewhere else entirely, in the settings of
     * claude.ai and often on another device, so it belongs on the clipboard.
     */
    MCP("https://app.xn--rssing-wxa.de/mcp"),

    /** The web administration — a real page, so this one is opened. */
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
    AdminHintCard(
        modifier = modifier,
        onCopy = { address -> context.copyAdminAddress(address) },
        onOpen = { address -> context.openInCustomTab(address.url) },
    )
}

/** The display on its own — without a context, so it can be tested. */
@Composable
fun AdminHintCard(
    onCopy: (AdminAddress) -> Unit,
    onOpen: (AdminAddress) -> Unit,
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
            AddressRow(
                address = AdminAddress.MCP,
                testTag = "admin-hint-mcp",
                icon = Icons.Filled.ContentCopy,
                action = R.string.admin_hint_copy,
                onClick = onCopy,
            )
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
            AddressRow(
                address = AdminAddress.WEB,
                testTag = "admin-hint-web",
                icon = Icons.AutoMirrored.Filled.OpenInNew,
                action = R.string.admin_hint_open,
                onClick = onOpen,
            )
        }
    }
}

/**
 * The address as it is — readable, so it can be typed off a second device —
 * and underneath it the gesture that belongs to it. Icon and wording differ
 * per row, because the two gestures differ: one copies, the other leaves the
 * app.
 */
@Composable
private fun AddressRow(
    address: AdminAddress,
    testTag: String,
    icon: ImageVector,
    @StringRes action: Int,
    onClick: (AdminAddress) -> Unit,
) {
    Column {
        // Outside the button on purpose: whoever cannot tap — because
        // claude.ai is open on another machine — still has to be able to read
        // the address off the screen.
        Text(
            address.url,
            style = MaterialTheme.typography.bodyMedium,
            fontFamily = FontFamily.Monospace,
            color = MaterialTheme.colorScheme.primary,
            modifier = Modifier.testTag("$testTag-url"),
        )
        TextButton(onClick = { onClick(address) }, modifier = Modifier.testTag(testTag)) {
            Icon(icon, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(8.dp))
            Text(stringResource(action))
        }
    }
}

/**
 * Puts an address on the clipboard and says so — but only where the system
 * does not say it already.
 *
 * Without any feedback a tap is indistinguishable from a dead button: the
 * clipboard is invisible. Two confirmations are just as bad, and that is the
 * case from Android 13 on, where the platform shows its own note for every
 * copy.
 */
internal fun Context.copyAdminAddress(address: AdminAddress) {
    val clipboard = getSystemService(ClipboardManager::class.java) ?: return
    val label = getString(R.string.admin_hint_clip_label)
    clipboard.setPrimaryClip(ClipData.newPlainText(label, address.url))
    if (needsCopyConfirmation(Build.VERSION.SDK_INT)) {
        Toast.makeText(this, R.string.admin_hint_copied, Toast.LENGTH_SHORT).show()
    }
}

/**
 * Whether the app has to confirm a copy itself.
 *
 * Android 13 (API 33) shows a confirmation of its own for every write to the
 * clipboard; a Toast on top of it would put the same sentence on screen
 * twice. Below that, nothing happens visibly at all.
 *
 * The API level is a parameter so a plain unit test can ask both sides of the
 * line — `Build.VERSION.SDK_INT` has no value outside a device.
 */
internal fun needsCopyConfirmation(sdkInt: Int): Boolean = sdkInt < Build.VERSION_CODES.TIRAMISU
