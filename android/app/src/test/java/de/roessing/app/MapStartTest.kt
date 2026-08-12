package de.roessing.app

import de.roessing.app.data.GeoBounds
import de.roessing.app.data.LatLon
import de.roessing.app.data.MAX_START_ZOOM
import de.roessing.app.data.MapStart
import de.roessing.app.data.ROESSING
import de.roessing.app.data.ROESSING_BOUNDS
import de.roessing.app.data.START_PADDING_DP
import de.roessing.app.data.startView
import kotlin.math.PI
import kotlin.math.atan
import kotlin.math.cos
import kotlin.math.ln
import kotlin.math.pow
import kotlin.math.sinh
import kotlin.math.tan
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Startausschnitt der Karte: Beim Öffnen der App sollen alle Pflege-Orte im
 * Bild liegen — nicht ein zufälliger Ausschnitt des Ortskerns.
 *
 * Die Prüfung rechnet den sichtbaren Bereich aus Mitte und Zoom bewusst mit
 * eigener Mercator-Mathematik nach, damit ein Fehler in der Produktivformel
 * nicht durch dieselbe Formel im Test verdeckt wird.
 */
class MapStartTest {
    // Typisches Hochkant-Telefon in dp.
    private val breite = 400.0
    private val hoehe = 700.0

    private fun mercY(lat: Double): Double {
        val rad = Math.toRadians(lat.coerceIn(-85.05112878, 85.05112878))
        return (1 - ln(tan(rad) + 1 / cos(rad)) / PI) / 2
    }

    private fun latVonY(y: Double): Double = Math.toDegrees(atan(sinh(PI * (1 - 2 * y))))

    /**
     * Der Bereich, der bei diesem Start garantiert im Bild liegt — also der
     * sichtbare Ausschnitt abzüglich des Randes.
     */
    private fun sichtbarOhneRand(
        start: MapStart,
        breiteDp: Double = breite,
        hoeheDp: Double = hoehe,
        randDp: Double = START_PADDING_DP,
    ): GeoBounds {
        val welt = TILE * 2.0.pow(start.zoom)
        val mitteX = (start.center.lon + 180) / 360 * welt
        val mitteY = mercY(start.center.lat) * welt
        val halbeBreite = breiteDp / 2 - randDp
        val halbeHoehe = hoeheDp / 2 - randDp
        return GeoBounds(
            south = latVonY((mitteY + halbeHoehe) / welt),
            west = (mitteX - halbeBreite) / welt * 360 - 180,
            north = latVonY((mitteY - halbeHoehe) / welt),
            east = (mitteX + halbeBreite) / welt * 360 - 180,
        )
    }

    /**
     * Enthalten — mit einer Toleranz von 1e-9 Grad (rund 0,1 mm). Ein Punkt
     * genau auf der Randlinie darf nicht an Rundungsresten scheitern.
     */
    private fun GeoBounds.enthaelt(p: LatLon) =
        p.lat in (south - EPS)..(north + EPS) && p.lon in (west - EPS)..(east + EPS)

    private fun pruefeSichtbar(start: MapStart, vararg punkte: LatLon) {
        val sicht = sichtbarOhneRand(start)
        punkte.forEach {
            assertTrue("$it liegt nicht im Startausschnitt $sicht (Zoom ${start.zoom})", sicht.enthaelt(it))
        }
    }

    // Drei Orte quer durchs Dorf.
    private val kasten = LatLon(52.183159, 9.816763)
    private val nordrand = LatLon(52.1908, 9.8210)
    private val westrand = LatLon(52.1795, 9.8080)

    @Test
    fun `alle Orte liegen beim Start im Bild`() {
        val start = startView(listOf(kasten, nordrand, westrand), null, breite, hoehe)
        pruefeSichtbar(start, kasten, nordrand, westrand)
        assertTrue("So ein kleines Dorf braucht keinen Weltausschnitt", start.zoom > 13.0)
    }

    @Test
    fun `ein einziger Ort zoomt nicht bis auf die Hausnummer`() {
        val start = startView(listOf(kasten), null, breite, hoehe)
        assertEquals(MAX_START_ZOOM, start.zoom, 0.001)
        assertEquals(kasten.lat, start.center.lat, 0.0005)
        assertEquals(kasten.lon, start.center.lon, 0.0005)
        pruefeSichtbar(start, kasten)
    }

    @Test
    fun `ohne Orte zeigt die Karte ganz Rössing`() {
        val start = startView(emptyList(), null, breite, hoehe)
        assertEquals(ROESSING_BOUNDS, start.bounds)
        // Alle vier Ecken des bebauten Ortsbereichs müssen ins Bild passen.
        pruefeSichtbar(
            start,
            LatLon(ROESSING_BOUNDS.south, ROESSING_BOUNDS.west),
            LatLon(ROESSING_BOUNDS.north, ROESSING_BOUNDS.east),
            LatLon(ROESSING_BOUNDS.south, ROESSING_BOUNDS.east),
            LatLon(ROESSING_BOUNDS.north, ROESSING_BOUNDS.west),
            // und die realen Blumenkästen „Unter den Eichen" sowieso.
            kasten,
        )
        assertTrue("Ganz Rössing passt nicht in Zoom ${start.zoom}", start.zoom < MAX_START_ZOOM)
    }

    @Test
    fun `ein Standort im Dorf wird mit ins Bild genommen statt angesprungen`() {
        val ich = LatLon(52.1875, 9.8250)
        val start = startView(listOf(kasten), ich, breite, hoehe)
        pruefeSichtbar(start, kasten, ich)
        // Nicht auf den Nutzer zentriert — die Orte bleiben gleichberechtigt.
        assertTrue(
            "Die Kamera darf nicht auf dem Nutzer kleben",
            kotlin.math.abs(start.center.lat - ich.lat) > 0.0005,
        )
    }

    @Test
    fun `ein weit entfernter Standort bleibt außen vor`() {
        val hamburg = LatLon(53.5511, 9.9937)
        val ohne = startView(listOf(kasten, nordrand), null, breite, hoehe)
        val mit = startView(listOf(kasten, nordrand), hamburg, breite, hoehe)
        assertEquals("Hamburg darf den Dorfausschnitt nicht sprengen", ohne, mit)
    }

    @Test
    fun `ohne Orte, aber mit Standort im Dorf, bleibt das Dorf im Bild`() {
        val ich = LatLon(52.1890, 9.8290)
        val start = startView(emptyList(), ich, breite, hoehe)
        pruefeSichtbar(
            start,
            ich,
            LatLon(ROESSING_BOUNDS.south, ROESSING_BOUNDS.west),
            LatLon(ROESSING_BOUNDS.north, ROESSING_BOUNDS.east),
        )
    }

    @Test
    fun `weit auseinander liegende Orte passen trotzdem alle ins Bild`() {
        val hildesheim = LatLon(52.1548, 9.9511)
        val hannover = LatLon(52.3759, 9.7320)
        val start = startView(listOf(kasten, hildesheim, hannover), null, breite, hoehe)
        pruefeSichtbar(start, kasten, hildesheim, hannover)
        assertTrue("Weite Streuung braucht kleinen Zoom", start.zoom < 12.0)
    }

    @Test
    fun `auch im Querformat bleibt alles im Bild`() {
        val start = startView(listOf(kasten, nordrand, westrand), null, hoehe, breite)
        val sicht = sichtbarOhneRand(start, hoehe, breite)
        listOf(kasten, nordrand, westrand).forEach {
            assertTrue("$it fehlt im Querformat-Ausschnitt $sicht", sicht.enthaelt(it))
        }
    }

    @Test
    fun `die Bounding Box umschließt genau die aktiven Orte`() {
        val start = startView(listOf(kasten, nordrand, westrand), null, breite, hoehe)
        assertEquals(westrand.lat, start.bounds.south, 1e-9)
        assertEquals(nordrand.lat, start.bounds.north, 1e-9)
        assertEquals(westrand.lon, start.bounds.west, 1e-9)
        assertEquals(nordrand.lon, start.bounds.east, 1e-9)
    }

    @Test
    fun `der Ortskern liegt im Rückfall-Ausschnitt`() {
        assertTrue(ROESSING.lat in ROESSING_BOUNDS.south..ROESSING_BOUNDS.north)
        assertTrue(ROESSING.lon in ROESSING_BOUNDS.west..ROESSING_BOUNDS.east)
    }

    private companion object {
        const val TILE = 512.0
        const val EPS = 1e-9
    }
}
