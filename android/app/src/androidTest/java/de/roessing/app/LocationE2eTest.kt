package de.roessing.app

import android.Manifest
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import de.roessing.app.data.DeviceLocation
import de.roessing.app.data.LatLon
import de.roessing.app.data.ROESSING
import de.roessing.app.data.contains
import de.roessing.app.data.distanceMeters
import de.roessing.app.data.isNearVillage
import de.roessing.app.data.startView
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Before
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Echter Standort-Test auf dem Emulator. Die Position setzt die CI vorher
 * mit `adb emu geo fix` auf Rössing (siehe android/ci-e2e.sh).
 */
@RunWith(AndroidJUnit4::class)
class LocationE2eTest {
    @Before
    fun onlyInE2eMode() {
        assumeTrue(
            "E2E-Modus nicht aktiv (Instrumentation-Arg e2e fehlt)",
            InstrumentationRegistry.getArguments().getString("e2e") == "true",
        )
    }

    @Test
    fun standortWirdGelesenUndLiegtImStartausschnitt() = runBlocking {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val context = instrumentation.targetContext
        // Berechtigung wie im echten Betrieb erteilen — nur eben ohne Dialog.
        instrumentation.uiAutomation.grantRuntimePermission(
            context.packageName, Manifest.permission.ACCESS_FINE_LOCATION,
        )

        val standort = DeviceLocation(context)
        assertTrue("Berechtigung wurde nicht erteilt", standort.hasPermission())

        val ich = standort.current(20_000)
        assumeTrue("Emulator liefert keinen Standort", ich != null)

        // Die CI setzt den Emulator nach Rössing.
        val entfernung = distanceMeters(ich!!, ROESSING)
        assertTrue("Standort liegt $entfernung m von Rössing entfernt", entfernung < 2_000)

        assertTrue("Standort müsste als nah am Dorf gelten", isNearVillage(ich))

        // Der Startausschnitt springt nicht auf den Nutzer, nimmt ihn aber mit
        // ins Bild — zusammen mit den Orten.
        val orte = listOf(LatLon(52.183159, 9.816763), LatLon(52.1908, 9.8210))
        val start = startView(orte, ich, widthDp = 400.0, heightDp = 700.0)
        assertTrue("Standort fehlt im Startausschnitt", start.bounds.contains(ich))
        orte.forEach { assertTrue("Ort $it fehlt im Startausschnitt", start.bounds.contains(it)) }
    }
}
