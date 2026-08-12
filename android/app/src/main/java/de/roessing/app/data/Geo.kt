package de.roessing.app.data

import java.util.Locale
import kotlin.math.asin
import kotlin.math.cos
import kotlin.math.min
import kotlin.math.pow
import kotlin.math.roundToInt
import kotlin.math.sin
import kotlin.math.sqrt

/**
 * Standort-Rechnerei als reine Funktionen — bewusst ohne Android-Bezug,
 * damit sie in normalen Unit-Tests laufen.
 *
 * Datenschutz: Der Standort bleibt auf dem Gerät. Er wird nur für die
 * Kartenansicht und die Entfernungsanzeige benutzt und niemals ans Backend
 * geschickt.
 */
data class LatLon(val lat: Double, val lon: Double)

/** Dorfmitte Rössing (Unter den Eichen). */
val ROESSING = LatLon(52.2110, 9.8700)

/** Zoomstufen: Dorfüberblick bzw. „ich sehe meine Umgebung". */
const val VILLAGE_ZOOM = 14.0
const val USER_ZOOM = 16.0

/**
 * Bis zu dieser Luftlinie gilt ein Standort als „in der Nähe des Dorfes".
 * Wer gerade in Hannover sitzt, will die Dorfkarte sehen und nicht seinen
 * Wohnort.
 */
const val NEARBY_METERS = 20_000.0

/** Startausschnitt der Karte. */
data class CameraStart(val target: LatLon, val zoom: Double, val followsUser: Boolean)

/**
 * Entscheidet, worauf die Karte beim Start zentriert: auf den Nutzer, wenn
 * er in der Nähe des Dorfes ist — sonst aufs Dorf. Wer gerade in Hannover
 * sitzt, will die Dorfkarte sehen und nicht seinen Wohnort.
 */
fun startCamera(
    user: LatLon?,
    village: LatLon = ROESSING,
    nearbyMeters: Double = NEARBY_METERS,
): CameraStart =
    if (user != null && distanceMeters(user, village) <= nearbyMeters) {
        CameraStart(user, USER_ZOOM, true)
    } else {
        CameraStart(village, VILLAGE_ZOOM, false)
    }

/** Luftlinie in Metern (Haversine, Erdradius 6371 km). */
fun distanceMeters(a: LatLon, b: LatLon): Double {
    val erdradius = 6_371_000.0
    val dLat = Math.toRadians(b.lat - a.lat)
    val dLon = Math.toRadians(b.lon - a.lon)
    val h = sin(dLat / 2).pow(2) +
        cos(Math.toRadians(a.lat)) * cos(Math.toRadians(b.lat)) * sin(dLon / 2).pow(2)
    return 2 * erdradius * asin(min(1.0, sqrt(h)))
}

/** Entfernung für die Anzeige: „120 m", „1,3 km", „13 km". */
fun formatDistance(meters: Double): String = when {
    meters < 950 -> "${meters.roundToInt()} m"
    meters < 9_950 -> "%.1f km".format(Locale.GERMANY, meters / 1000)
    else -> "${(meters / 1000).roundToInt()} km"
}

/** Sortierung der Ortsliste. */
enum class PlaceSort { URGENCY, DISTANCE }
