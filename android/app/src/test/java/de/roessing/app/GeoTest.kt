package de.roessing.app

import de.roessing.app.data.LatLon
import de.roessing.app.data.NEARBY_METERS
import de.roessing.app.data.ROESSING
import de.roessing.app.data.distanceMeters
import de.roessing.app.data.formatDistance
import de.roessing.app.data.isNearVillage
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class GeoTest {
    /** Ein Punkt in genau nord-südlicher Richtung, entfernung Meter entfernt. */
    private fun noerdlichVon(p: LatLon, entfernung: Double) =
        LatLon(p.lat + entfernung / 111_320.0, p.lon)

    @Test
    fun `Entfernung stimmt in der Größenordnung`() {
        // Ein Grad Breite sind rund 111 km.
        val einKilometer = distanceMeters(ROESSING, noerdlichVon(ROESSING, 1000.0))
        assertEquals(1000.0, einKilometer, 5.0)
        assertEquals(0.0, distanceMeters(ROESSING, ROESSING), 0.001)

        // Hildesheim liegt rund 15 km entfernt, Hannover rund 25 km.
        val hildesheim = LatLon(52.1548, 9.9511)
        assertTrue(distanceMeters(ROESSING, hildesheim) in 8_000.0..15_000.0)
        val hannover = LatLon(52.3759, 9.7320)
        assertTrue(distanceMeters(ROESSING, hannover) > 18_000.0)
    }

    @Test
    fun `die Dorfmitte liegt beim Ortskern und nicht auf freiem Feld`() {
        // Die realen Blumenkästen „Unter den Eichen".
        val kaesten = LatLon(52.183159, 9.816763)
        val entfernung = distanceMeters(ROESSING, kaesten)
        assertTrue("Kartenmitte liegt $entfernung m von den Kästen entfernt", entfernung < 500)
    }

    @Test
    fun `ein Standort im Dorf gilt als nah`() {
        assertTrue(isNearVillage(noerdlichVon(ROESSING, 300.0)))
    }

    @Test
    fun `ein weit entfernter Standort gilt nicht als nah`() {
        val hamburg = LatLon(53.5511, 9.9937)
        assertFalse(isNearVillage(hamburg))
    }

    @Test
    fun `Grenzfall genau an der Umkreis-Schwelle zaehlt noch als nah`() {
        val genau = noerdlichVon(ROESSING, NEARBY_METERS)
        assertTrue("genau auf der Schwelle muss noch zählen", isNearVillage(genau))

        val knappDrueber = noerdlichVon(ROESSING, NEARBY_METERS + 100)
        assertFalse("knapp außerhalb gehört dem Dorf", isNearVillage(knappDrueber))
    }

    @Test
    fun `Entfernungen werden lesbar formatiert`() {
        assertEquals("0 m", formatDistance(0.0))
        assertEquals("42 m", formatDistance(42.4))
        assertEquals("120 m", formatDistance(120.0))
        assertEquals("1,0 km", formatDistance(999.0))
        assertEquals("1,3 km", formatDistance(1250.0))
        assertEquals("13 km", formatDistance(13_400.0))
    }
}
