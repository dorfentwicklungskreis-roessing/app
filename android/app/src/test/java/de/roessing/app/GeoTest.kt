package de.roessing.app

import de.roessing.app.data.LatLon
import de.roessing.app.data.NEARBY_METERS
import de.roessing.app.data.ROESSING
import de.roessing.app.data.USER_ZOOM
import de.roessing.app.data.VILLAGE_ZOOM
import de.roessing.app.data.distanceMeters
import de.roessing.app.data.formatDistance
import de.roessing.app.data.startCamera
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
    fun `ohne Standort zeigt die Karte das Dorf`() {
        val start = startCamera(null)
        assertEquals(ROESSING, start.target)
        assertEquals(VILLAGE_ZOOM, start.zoom, 0.001)
        assertFalse(start.followsUser)
    }

    @Test
    fun `Standort im Dorf zentriert auf den Nutzer`() {
        val ich = noerdlichVon(ROESSING, 300.0)
        val start = startCamera(ich)
        assertEquals(ich, start.target)
        assertEquals(USER_ZOOM, start.zoom, 0.001)
        assertTrue(start.followsUser)
    }

    @Test
    fun `weit entfernter Standort zeigt trotzdem das Dorf`() {
        val hamburg = LatLon(53.5511, 9.9937)
        val start = startCamera(hamburg)
        assertEquals(ROESSING, start.target)
        assertEquals(VILLAGE_ZOOM, start.zoom, 0.001)
        assertFalse(start.followsUser)
    }

    @Test
    fun `Grenzfall genau an der Umkreis-Schwelle zaehlt noch als nah`() {
        val genau = noerdlichVon(ROESSING, NEARBY_METERS)
        assertTrue("genau auf der Schwelle muss noch zählen", startCamera(genau).followsUser)

        val knappDrueber = noerdlichVon(ROESSING, NEARBY_METERS + 100)
        assertFalse("knapp außerhalb gehört dem Dorf", startCamera(knappDrueber).followsUser)
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
