package de.roessing.app

import android.content.ClipboardManager
import android.content.Context
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import de.roessing.app.ui.AdminAddress
import de.roessing.app.ui.AdminHintCard
import de.roessing.app.ui.StartScreen
import de.roessing.app.ui.copyAdminAddress
import de.roessing.app.ui.theme.DorfAppTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Administering runs on the MCP server and in the web administration. The
 * app no longer offers it — but whoever may administer must not run into an
 * empty spot: the start screen says where it happens and how to get there.
 *
 * The two addresses look alike and are used in opposite ways: the connector
 * address is copied — `/mcp` is a protocol endpoint, a browser only shows an
 * error there — while the web administration is opened. Mixing the two up is
 * exactly the mistake this file guards against.
 */
@RunWith(AndroidJUnit4::class)
class AdminHintUiTest {
    @get:Rule
    val compose = createComposeRule()

    @Test
    fun startScreen_showsTheHintToAdmins() {
        compose.setContent {
            DorfAppTheme {
                StartScreen(
                    name = "Erna",
                    faelligeOrte = 0,
                    ladend = false,
                    isAdmin = true,
                    onBereich = {},
                )
            }
        }

        compose.onNodeWithTag("admin-hint").performScrollTo().assertIsDisplayed()
        // The advantage over the old screen is the reason for the change —
        // it must be readable, not implied.
        compose.onNodeWithTag("admin-hint-benefit").performScrollTo().assertIsDisplayed()
    }

    /** An ordinary member has no business there and is not told about it. */
    @Test
    fun startScreen_staysQuietForMembers() {
        compose.setContent {
            DorfAppTheme { StartScreen(name = "Erna", faelligeOrte = 0, ladend = false, onBereich = {}) }
        }

        compose.onNodeWithTag("admin-hint").assertDoesNotExist()
    }

    @Test
    fun theConnectorAddressIsCopiedAndTheWebAddressIsOpened() {
        val copied = mutableListOf<AdminAddress>()
        val opened = mutableListOf<AdminAddress>()
        compose.setContent {
            DorfAppTheme {
                Scrollbar { AdminHintCard(onCopy = { copied += it }, onOpen = { opened += it }) }
            }
        }

        compose.onNodeWithTag("admin-hint-mcp").performScrollTo().performClick()
        compose.onNodeWithTag("admin-hint-web").performScrollTo().performClick()

        assertEquals(listOf(AdminAddress.MCP), copied)
        assertEquals(listOf(AdminAddress.WEB), opened)
        // The connector address is never opened in a browser — that is the
        // whole point of the change.
        assertEquals(emptyList<AdminAddress>(), opened.filter { it == AdminAddress.MCP })
    }

    /** Both addresses stay legible for whoever types them off a second screen. */
    @Test
    fun bothAddressesAreSpelledOut() {
        compose.setContent {
            DorfAppTheme { Scrollbar { AdminHintCard(onCopy = {}, onOpen = {}) } }
        }

        compose.onNodeWithTag("admin-hint-mcp-url").performScrollTo().assertIsDisplayed()
        compose.onNodeWithTag("admin-hint-web-url").performScrollTo().assertIsDisplayed()
    }

    @Test
    fun copyingPutsTheConnectorAddressOnTheClipboard() {
        val context = ApplicationProvider.getApplicationContext<Context>()

        // On the main thread: below Android 13 the copy also raises a Toast,
        // and a Toast from a background thread throws.
        InstrumentationRegistry.getInstrumentation().runOnMainSync {
            context.copyAdminAddress(AdminAddress.MCP)
        }

        val clipboard = context.getSystemService(ClipboardManager::class.java)
        val clip = requireNotNull(clipboard.primaryClip) { "Nothing on the clipboard" }
        assertEquals(AdminAddress.MCP.url, clip.getItemAt(0).text.toString())
    }
}

/**
 * The card on its own is taller than a small emulator screen, so the web row
 * ends up below the fold: not displayed, not clickable. In the app the card
 * sits inside a scrollable column — the test has to give it the same, or it
 * tests a situation that never occurs.
 */
@androidx.compose.runtime.Composable
private fun Scrollbar(content: @androidx.compose.runtime.Composable () -> Unit) {
    Column(Modifier.verticalScroll(rememberScrollState())) { content() }
}
