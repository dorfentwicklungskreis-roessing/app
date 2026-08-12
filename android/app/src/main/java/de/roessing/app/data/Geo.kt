package de.roessing.app.data

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

/** Entscheidet, worauf die Karte beim Start zentriert. */
fun startCamera(
    user: LatLon?,
    village: LatLon = ROESSING,
    nearbyMeters: Double = NEARBY_METERS,
): CameraStart = CameraStart(village, VILLAGE_ZOOM, false)

/** Luftlinie in Metern (Haversine). */
fun distanceMeters(a: LatLon, b: LatLon): Double = 0.0

/** Entfernung für die Anzeige: „120 m", „1,3 km", „13 km". */
fun formatDistance(meters: Double): String = ""

/** Sortierung der Ortsliste. */
enum class PlaceSort { URGENCY, DISTANCE }
