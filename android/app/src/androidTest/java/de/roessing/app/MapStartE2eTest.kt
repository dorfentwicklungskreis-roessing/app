package de.roessing.app

import android.view.View
import android.view.ViewGroup
import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import de.roessing.app.data.LatLon
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.ROESSING
import de.roessing.app.ui.MapScreen
import de.roessing.app.ui.theme.DorfAppTheme
import java.util.concurrent.atomic.AtomicReference
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.maplibre.android.geometry.LatLng
import org.maplibre.android.maps.MapLibreMap
import org.maplibre.android.maps.MapView

/**
 * Der Startausschnitt auf einer echten Karte: Nach dem Öffnen müssen alle
 * Marker im sichtbaren Bereich liegen — genau das war vorher nicht so.
 *
 * Der Test greift sich die MapView aus dem Fensterbaum, damit der
 * Produktivcode keine Test-Hintertür braucht.
 */
@RunWith(AndroidJUnit4::class)
class MapStartE2eTest {
    @get:Rule
    val compose = createAndroidComposeRule<ComponentActivity>()

    private fun place(id: Long, lat: Double, lon: Double, active: Boolean = true) = PlaceDto(
        id = id, name = "Ort $id", lat = lat, lon = lon, active = active, status = "green",
    )

    /** Die Orte liegen quer über das Dorf verteilt — echte Rössing-Koordinaten. */
    private val orte = listOf(
        place(1, 52.183159, 9.816763), // Unter den Eichen
        place(2, 52.1908, 9.8210), // Nordrand
        place(3, 52.1795, 9.8080), // Westrand
        place(4, 52.1990, 9.9000, active = false), // inaktiv: darf den Ausschnitt nicht aufblähen
    )

    private fun findeMapView(view: View): MapView? = when {
        view is MapView -> view
        view is ViewGroup -> (0 until view.childCount)
            .asSequence()
            .mapNotNull { findeMapView(view.getChildAt(it)) }
            .firstOrNull()

        else -> null
    }

    private fun karteMitAusschnitt(standort: LatLon? = null): MapLibreMap {
        compose.setContent {
            DorfAppTheme {
                MapScreen(
                    places = orte,
                    modifier = Modifier.fillMaxSize(),
                    userLocation = standort,
                    onPlaceTap = {},
                )
            }
        }
        compose.waitForIdle()

        compose.waitUntil(30_000) {
            compose.runOnUiThread { findeMapView(compose.activity.window.decorView) } != null
        }
        val karte = AtomicReference<MapLibreMap?>(null)
        compose.runOnUiThread {
            findeMapView(compose.activity.window.decorView)!!.getMapAsync { karte.set(it) }
        }
        // Zoom 0 hieße: die Kamera steht noch auf der Weltkarte.
        compose.waitUntil(30_000) {
            compose.runOnUiThread { karte.get()?.cameraPosition?.zoom ?: 0.0 } > 1.0
        }
        return karte.get()!!
    }

    private fun pruefeSichtbar(map: MapLibreMap, vararg punkte: LatLon) {
        val sicht = compose.runOnUiThread { map.projection.visibleRegion.latLngBounds }
        punkte.forEach {
            assertTrue(
                "$it liegt außerhalb des Startausschnitts $sicht (Zoom ${map.cameraPosition.zoom})",
                sicht.contains(LatLng(it.lat, it.lon)),
            )
        }
    }

    @Test
    fun beimStartLiegenAlleMarkerImBild() {
        val map = karteMitAusschnitt()
        pruefeSichtbar(map, *orte.filter { it.active }.map { LatLon(it.lat, it.lon) }.toTypedArray())
        assertTrue(
            "Der Startausschnitt darf nicht auf Hausnummern-Ebene stehen",
            map.cameraPosition.zoom <= 16.5,
        )
    }

    @Test
    fun einNaherStandortKommtMitInsBild() {
        val ich = LatLon(ROESSING.lat + 0.004, ROESSING.lon + 0.008)
        val map = karteMitAusschnitt(ich)
        pruefeSichtbar(
            map,
            *orte.filter { it.active }.map { LatLon(it.lat, it.lon) }.toTypedArray(),
            ich,
        )
    }
}
