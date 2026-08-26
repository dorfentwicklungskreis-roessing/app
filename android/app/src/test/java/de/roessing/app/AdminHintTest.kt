package de.roessing.app

import de.roessing.app.ui.AdminAddress
import de.roessing.app.ui.needsCopyConfirmation
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Administration left the app; the hint on the start screen is all that is
 * left of it. Its two addresses are therefore the whole feature — a typo in
 * them is as good as no hint at all, because nobody would find the way.
 *
 * Both addresses are checked structurally, not by their literal text: the
 * production host must not appear in a test source (see
 * .github/scripts/pruefe_lokale_tests.py), and a test repeating the very
 * string it guards would prove nothing anyway.
 */
class AdminHintTest {
    @Test
    fun `both addresses are encrypted and absolute`() {
        AdminAddress.entries.forEach { address ->
            assertTrue(address.name, address.url.startsWith("https://"))
        }
    }

    @Test
    fun `the connector points at the MCP endpoint`() {
        // No trailing slash: claude.ai passes the address on unchanged, and
        // the MCP endpoint is served without one.
        assertTrue(AdminAddress.MCP.url, AdminAddress.MCP.url.endsWith("/mcp"))
    }

    @Test
    fun `the browser link points at the web administration`() {
        // With a trailing slash: the redirect URI registered in Zitadel ends
        // in one, and a redirect in the in-app browser looks like a failure.
        assertTrue(AdminAddress.WEB.url, AdminAddress.WEB.url.endsWith("/admin/"))
    }

    @Test
    fun `both ways lead to the same server`() {
        // One deployment serves REST, MCP and the web administration. Whoever
        // moves the app to another host must move both addresses.
        assertEquals(host(AdminAddress.MCP.url), host(AdminAddress.WEB.url))
    }

    @Test
    fun `the copied address is whole and not a fragment`() {
        // It is pasted into an empty field in claude.ai: scheme and host have
        // to travel with it.
        assertTrue(AdminAddress.MCP.url, AdminAddress.MCP.url.startsWith("https://"))
        assertTrue(AdminAddress.MCP.url, host(AdminAddress.MCP.url).isNotEmpty())
    }

    // MARK: Confirming the copy

    @Test
    fun `below Android 13 the app confirms the copy itself`() {
        // Nothing on screen changes when the clipboard is written, so an
        // unconfirmed copy is indistinguishable from a dead button.
        assertTrue(needsCopyConfirmation(26))
        assertTrue(needsCopyConfirmation(32))
    }

    @Test
    fun `from Android 13 on the system confirms it and the app keeps quiet`() {
        // The platform shows its own note for every copy; a Toast on top of
        // it would put the same sentence on screen twice.
        assertFalse(needsCopyConfirmation(33))
        assertFalse(needsCopyConfirmation(36))
    }

    private fun host(url: String): String = url.removePrefix("https://").substringBefore('/')
}
