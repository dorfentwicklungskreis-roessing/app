package de.roessing.app

import de.roessing.app.ui.AdminLink
import org.junit.Assert.assertEquals
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
        AdminLink.entries.forEach { link ->
            assertTrue(link.name, link.url.startsWith("https://"))
        }
    }

    @Test
    fun `the connector points at the MCP endpoint`() {
        // No trailing slash: claude.ai passes the address on unchanged, and
        // the MCP endpoint is served without one.
        assertTrue(AdminLink.MCP.url, AdminLink.MCP.url.endsWith("/mcp"))
    }

    @Test
    fun `the browser link points at the web administration`() {
        // With a trailing slash: the redirect URI registered in Zitadel ends
        // in one, and a redirect in the in-app browser looks like a failure.
        assertTrue(AdminLink.WEB.url, AdminLink.WEB.url.endsWith("/admin/"))
    }

    @Test
    fun `both ways lead to the same server`() {
        // One deployment serves REST, MCP and the web administration. Whoever
        // moves the app to another host must move both addresses.
        assertEquals(host(AdminLink.MCP.url), host(AdminLink.WEB.url))
    }

    private fun host(url: String): String = url.removePrefix("https://").substringBefore('/')
}
