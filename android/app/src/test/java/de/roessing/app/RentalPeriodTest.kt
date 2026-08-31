package de.roessing.app

import de.roessing.app.data.RentalPeriod
import de.roessing.app.data.euroText
import de.roessing.app.data.markdownAsPlainText
import de.roessing.app.ui.periodOfPicked
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.LocalDate
import java.time.ZoneOffset

/**
 * Der halboffene Zeitraum ist die eine Stelle des Vertrags, an der ein
 * Missverständnis einen ganzen Tag kostet: `endDate` ist der **Rückgabetag**
 * und gehört nicht mehr zur Buchung. Wer vom 5. bis zum 7. bucht, hat das
 * Gerät am 5. und am 6.; am 7. kann jemand anders anfangen.
 *
 * Genau dazwischen liegt der Kalender: Er gibt die Tage her, an denen jemand
 * das Gerät haben will. Die Umrechnung passiert an einer Stelle, und die
 * steht hier unter Beobachtung.
 */
class RentalPeriodTest {
    @Test
    fun `der Rueckgabetag gehoert nicht mehr zur Buchung`() {
        val zeitraum = RentalPeriod(LocalDate.parse("2026-09-05"), LocalDate.parse("2026-09-07"))

        assertEquals(LocalDate.parse("2026-09-06"), zeitraum.lastDay)
        assertEquals(2, zeitraum.days)
        assertTrue(zeitraum.isValid)
    }

    @Test
    fun `ein einziger Tag ist ein Tag, nicht null`() {
        val zeitraum = RentalPeriod(LocalDate.parse("2026-09-05"), LocalDate.parse("2026-09-06"))

        assertEquals(1, zeitraum.days)
        assertEquals("5. September 2026", zeitraum.text)
    }

    @Test
    fun `ein Zeitraum ohne Dauer ist keiner`() {
        val gleich = RentalPeriod(LocalDate.parse("2026-09-05"), LocalDate.parse("2026-09-05"))
        val verdreht = RentalPeriod(LocalDate.parse("2026-09-05"), LocalDate.parse("2026-09-04"))

        assertFalse(gleich.isValid)
        assertFalse(verdreht.isValid)
    }

    @Test
    fun `der Text nennt die Tage, an denen das Geraet draussen ist`() {
        fun text(von: String, bis: String) =
            RentalPeriod(LocalDate.parse(von), LocalDate.parse(bis)).text

        assertEquals("5.–6. September 2026", text("2026-09-05", "2026-09-07"))
        assertEquals("30. September – 2. Oktober 2026", text("2026-09-30", "2026-10-03"))
        assertEquals("30. Dezember 2026 – 1. Januar 2027", text("2026-12-30", "2027-01-02"))
    }

    /**
     * Der Kalender gibt UTC-Mitternachten heraus. Wer sie in Ortszeit
     * zurückliest, landet in der Sommerzeit auf dem Vortag — und der
     * angetippte Tag wäre nicht der gebuchte.
     */
    @Test
    fun `aus zwei angetippten Tagen wird ein Zeitraum mit Rueckgabetag`() {
        val erster = tag("2026-09-05")
        val letzter = tag("2026-09-06")

        val zeitraum = periodOfPicked(erster, letzter)!!

        assertEquals(LocalDate.parse("2026-09-05"), zeitraum.start)
        assertEquals(LocalDate.parse("2026-09-07"), zeitraum.end)
        assertEquals(2, zeitraum.days)
    }

    @Test
    fun `ein einzeln angetippter Tag ergibt einen Miettag`() {
        val zeitraum = periodOfPicked(tag("2026-09-05"), null)!!

        assertEquals(LocalDate.parse("2026-09-05"), zeitraum.start)
        assertEquals(LocalDate.parse("2026-09-06"), zeitraum.end)
        assertEquals(1, zeitraum.days)
        assertTrue(zeitraum.isValid)
    }

    @Test
    fun `ohne Auswahl gibt es keinen Zeitraum`() {
        assertNull(periodOfPicked(null, null))
        assertNull(periodOfPicked(null, tag("2026-09-06")))
    }

    @Test
    fun `ein Preis wird deutsch geschrieben und nicht verrechnet`() {
        assertEquals("25 €", euroText(25.0))
        assertEquals("12,50 €", euroText(12.5))
        assertEquals("8 €", euroText(8.0))
        assertEquals("0,99 €", euroText(0.99))
    }

    /**
     * Beschreibungen kommen als Markdown. Roh mit Sternchen wäre keine
     * Lösung; die App zeigt sie deshalb bewusst als Klartext.
     */
    @Test
    fun `Markdown wird lesbarer Klartext`() {
        val roh = """
            ## Kreiselmäher

            Für **hohes** Gras und *Böschungen*.

            - Arbeitsbreite 85 cm
            - Benzin, [Herstellerseite](https://example.invalid/as-585)
        """.trimIndent()

        assertEquals(
            """
            Kreiselmäher

            Für hohes Gras und Böschungen.

            • Arbeitsbreite 85 cm
            • Benzin, Herstellerseite
            """.trimIndent(),
            markdownAsPlainText(roh),
        )
    }

    @Test
    fun `ein Unterstrich mitten im Wort bleibt stehen`() {
        // Sonst zerlegte die Übersetzung Namen wie `as_585_km`.
        assertEquals("Typ as_585_km", markdownAsPlainText("Typ as_585_km"))
    }

    private fun tag(datum: String): Long =
        LocalDate.parse(datum).atStartOfDay(ZoneOffset.UTC).toInstant().toEpochMilli()
}
