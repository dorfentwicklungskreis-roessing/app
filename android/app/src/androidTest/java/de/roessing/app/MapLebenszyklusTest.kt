package de.roessing.app

import android.view.View
import android.view.ViewGroup
import androidx.activity.ComponentActivity
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.test.junit4.createAndroidComposeRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import de.roessing.app.data.PlaceDto
import de.roessing.app.ui.MapScreen
import de.roessing.app.ui.theme.DorfAppTheme
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.maplibre.android.maps.MapView

/**
 * Die Karte muss ihren nativen Teil freigeben, sobald sie vom Bildschirm
 * verschwindet.
 *
 * Hintergrund (Issue #24): „Mithelfen" hat drei Reiter — Karte, Liste,
 * Rangliste. Jeder Wechsel weg von der Karte nimmt MapScreen aus der
 * Komposition, jeder Wechsel zurück legt eine neue MapView an. Solange beim
 * Verlassen nur der Lifecycle-Beobachter abgemeldet wurde, lief der native
 * Kartenkern der verlassenen Karte weiter. Wurde die verwaiste MapView danach
 * von der Speicherbereinigung eingesammelt, traf sein nächster Rückruf ins
 * Leere und die Laufzeit beendete den Prozess auf der Stelle
 * („JNI DETECTED ERROR IN APPLICATION: can't call void
 * NativeMapView.onSpriteRequested(…) on null object"). Im E2E-Lauf erschien
 * das als „Instrumentation run failed due to Process crashed" bei wechselnden,
 * unschuldigen Tests.
 *
 * MapLibre selbst beantwortet die Frage: MapView.isDestroyed() wird genau von
 * onDestroy() gesetzt. Der Test braucht deshalb keine Test-Hintertür im
 * Produktivcode.
 */
@RunWith(AndroidJUnit4::class)
class MapLebenszyklusTest {
    @get:Rule
    val compose = createAndroidComposeRule<ComponentActivity>()

    private val orte = listOf(
        PlaceDto(id = 1, name = "Unter den Eichen", lat = 52.183159, lon = 9.816763, status = "green"),
    )

    private fun findeMapView(view: View): MapView? = when {
        view is MapView -> view
        view is ViewGroup -> (0 until view.childCount)
            .asSequence()
            .mapNotNull { findeMapView(view.getChildAt(it)) }
            .firstOrNull()

        else -> null
    }

    private fun aktuelleMapView(): MapView? =
        compose.runOnUiThread { findeMapView(compose.activity.window.decorView) }

    @Test
    fun jedeVerlasseneKarteWirdFreigegeben() {
        val zeigeKarte = mutableStateOf(true)
        compose.setContent {
            DorfAppTheme {
                // Kein else-Zweig nötig: „Karte weg" heißt genau, dass
                // MapScreen die Komposition verlässt.
                if (zeigeKarte.value) {
                    MapScreen(places = orte, modifier = Modifier.fillMaxSize(), onPlaceTap = {})
                }
            }
        }

        // Drei Runden Karte → Liste → Karte, so wie ein Mensch zwischen den
        // Reitern springt. Keine der verlassenen Karten darf am Leben bleiben.
        val verlassene = mutableListOf<MapView>()
        repeat(3) { runde ->
            compose.runOnUiThread { zeigeKarte.value = true }
            compose.waitUntil(30_000) { aktuelleMapView() != null }
            val karte = aktuelleMapView()!!
            assertTrue("Runde $runde: frische Karte darf nicht freigegeben sein", !karte.isDestroyed)
            verlassene += karte

            compose.runOnUiThread { zeigeKarte.value = false }
            compose.waitForIdle()
            assertTrue(
                "Runde $runde: die verlassene Karte hält ihren nativen Teil weiter fest",
                karte.isDestroyed,
            )
        }

        assertTrue(
            "Alle verlassenen Karten müssen freigegeben sein",
            verlassene.all { it.isDestroyed },
        )
    }
}
