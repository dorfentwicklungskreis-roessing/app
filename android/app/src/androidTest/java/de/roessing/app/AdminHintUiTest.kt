package de.roessing.app

import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.test.ext.junit.runners.AndroidJUnit4
import de.roessing.app.ui.AdminHintCard
import de.roessing.app.ui.AdminLink
import de.roessing.app.ui.StartScreen
import de.roessing.app.ui.theme.DorfAppTheme
import org.junit.Assert.assertEquals
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Administering runs on the MCP server and in the web administration. The
 * app no longer offers it — but whoever may administer must not run into an
 * empty spot: the start screen says where it happens and how to get there.
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
            DorfAppTheme {
                StartScreen(name = "Erna", faelligeOrte = 0, ladend = false, onBereich = {})
            }
        }

        compose.onNodeWithTag("admin-hint").assertDoesNotExist()
    }

    @Test
    fun bothAddressesCanBeTapped() {
        val opened = mutableListOf<AdminLink>()
        compose.setContent {
            DorfAppTheme { AdminHintCard(onOpen = { opened += it }) }
        }

        // No performScrollTo() here: the card is rendered on its own, without a
        // scrollable parent, so there is nothing to scroll — and asking for it
        // fails with "Semantic Node has no parent layout with a Scroll
        // SemanticsAction". The other tests place the card inside StartScreen,
        // where scrolling is both possible and necessary.
        compose.onNodeWithTag("admin-hint-mcp").performClick()
        compose.onNodeWithTag("admin-hint-web").performClick()

        assertEquals(listOf(AdminLink.MCP, AdminLink.WEB), opened)
    }
}
