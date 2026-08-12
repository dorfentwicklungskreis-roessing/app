package de.roessing.app.data

import android.Manifest
import android.annotation.SuppressLint
import android.content.Context
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.os.Looper
import androidx.core.content.ContextCompat
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull
import kotlin.coroutines.resume

/**
 * Standort des Geräts — sparsam abgefragt: zuerst die zuletzt bekannte
 * Position, und nur wenn es keine gibt, ein einziger frischer Fix. Kein
 * Dauer-Tracking, kein Hintergrundstandort.
 *
 * Bewusst über den `LocationManager` der Plattform statt über die Play
 * Services: keine zusätzliche Abhängigkeit, und es funktioniert auch auf
 * Emulator-Abbildern ohne Google-Dienste (wichtig für die CI, die den
 * Standort mit `adb emu geo fix` setzt).
 *
 * Datenschutz: Die Position bleibt auf dem Gerät. Sie wird ausschließlich
 * für Kartenausschnitt und Entfernungsanzeige benutzt und nie gesendet.
 */
class DeviceLocation(private val context: Context) {

    fun hasPermission(): Boolean = PERMISSIONS.any {
        ContextCompat.checkSelfPermission(context, it) == PackageManager.PERMISSION_GRANTED
    }

    @SuppressLint("MissingPermission")
    suspend fun current(timeoutMs: Long = 8_000): LatLon? {
        if (!hasPermission()) return null
        val manager = context.getSystemService(Context.LOCATION_SERVICE) as? LocationManager ?: return null
        letzteBekannte(manager)?.let { return it.toLatLon() }
        return withTimeoutOrNull(timeoutMs) { einzelnerFix(manager) }
    }

    @SuppressLint("MissingPermission")
    private fun letzteBekannte(manager: LocationManager): Location? =
        runCatching {
            manager.getProviders(true)
                .mapNotNull { manager.getLastKnownLocation(it) }
                .maxByOrNull { it.time }
        }.getOrNull()

    @SuppressLint("MissingPermission")
    private suspend fun einzelnerFix(manager: LocationManager): LatLon? =
        suspendCancellableCoroutine { fortsetzung ->
            val provider = when {
                manager.isProviderEnabled(LocationManager.GPS_PROVIDER) -> LocationManager.GPS_PROVIDER
                manager.isProviderEnabled(LocationManager.NETWORK_PROVIDER) -> LocationManager.NETWORK_PROVIDER
                else -> null
            }
            if (provider == null) {
                fortsetzung.resume(null)
                return@suspendCancellableCoroutine
            }
            val listener = object : LocationListener {
                override fun onLocationChanged(location: Location) {
                    manager.removeUpdates(this)
                    if (fortsetzung.isActive) fortsetzung.resume(location.toLatLon())
                }

                // Auf älteren Geräten sind diese Rückrufe noch abstrakt.
                override fun onProviderEnabled(provider: String) {}
                override fun onProviderDisabled(provider: String) {}

                @Deprecated("Seit API 29 ohne Funktion, für ältere Geräte nötig")
                override fun onStatusChanged(provider: String?, status: Int, extras: android.os.Bundle?) {
                }
            }
            runCatching { manager.requestLocationUpdates(provider, 0L, 0f, listener, Looper.getMainLooper()) }
                .onFailure { if (fortsetzung.isActive) fortsetzung.resume(null) }
            fortsetzung.invokeOnCancellation { runCatching { manager.removeUpdates(listener) } }
        }

    companion object {
        val PERMISSIONS = arrayOf(
            Manifest.permission.ACCESS_FINE_LOCATION,
            Manifest.permission.ACCESS_COARSE_LOCATION,
        )
    }
}

private fun Location.toLatLon() = LatLon(latitude, longitude)
