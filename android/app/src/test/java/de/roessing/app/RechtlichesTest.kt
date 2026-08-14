package de.roessing.app

import de.roessing.app.ui.Rechtsdokument
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Die beiden Adressen sind die eigentliche Pflichterfüllung — ein Tippfehler
 * darin ist rechtlich dasselbe wie gar kein Impressum. Sie zeigen bewusst auf
 * die Website: gepflegt wird an genau einer Stelle.
 */
class RechtlichesTest {
    @Test
    fun `zeigt auf die Seiten der Website`() {
        assertEquals("https://xn--rssing-wxa.de/impressum/", Rechtsdokument.IMPRESSUM.url)
        assertEquals("https://xn--rssing-wxa.de/app/datenschutz/", Rechtsdokument.DATENSCHUTZ.url)
    }

    @Test
    fun `beide Adressen sind verschluesselt und vollstaendig`() {
        Rechtsdokument.entries.forEach { dokument ->
            assertTrue(dokument.name, dokument.url.startsWith("https://"))
            // Mit Schrägstrich am Ende: die Website leitet sonst um, und eine
            // Umleitung im In-App-Browser sieht wie ein Fehler aus.
            assertTrue(dokument.name, dokument.url.endsWith("/"))
        }
    }
}
