package de.roessing.app.data

import java.util.Locale
import kotlin.math.PI
import kotlin.math.asin
import kotlin.math.atan
import kotlin.math.cos
import kotlin.math.ln
import kotlin.math.log2
import kotlin.math.max
import kotlin.math.min
import kotlin.math.pow
import kotlin.math.roundToInt
import kotlin.math.sin
import kotlin.math.sinh
import kotlin.math.sqrt
import kotlin.math.tan

/**
 * Standort-Rechnerei als reine Funktionen — bewusst ohne Android-Bezug,
 * damit sie in normalen Unit-Tests laufen.
 *
 * Datenschutz: Der Standort bleibt auf dem Gerät. Er wird nur für die
 * Kartenansicht und die Entfernungsanzeige benutzt und niemals ans Backend
 * geschickt.
 */
data class LatLon(val lat: Double, val lon: Double)

/**
 * Ortskern von Rössing. Die realen Blumenkästen „Unter den Eichen" stehen
 * bei 52.183159, 9.816763 — der Startausschnitt muss sie zeigen, sonst
 * startet die App auf freiem Feld.
 */
val ROESSING = LatLon(52.1843, 9.8162)

/**
 * Zoomstufe für „ich sehe meine Umgebung" — nur für den bewussten Druck auf
 * „Mein Standort". Der Startausschnitt wird dagegen aus den Orten berechnet
 * (siehe [startView]).
 */
const val USER_ZOOM = 16.0

/**
 * Obergrenze für den automatischen Startausschnitt. Gibt es nur einen
 * einzigen Ort (oder liegen alle dicht beieinander), soll die Karte nicht bis
 * auf Hausnummern-Ebene hineinzoomen, sondern die Umgebung mitzeigen.
 */
const val MAX_START_ZOOM = 16.5

/**
 * Rand um den Startausschnitt in dp. Deckt den Marker-Radius (11 dp Kreis +
 * 2,5 dp Rand) samt Luft ab, damit kein Marker am Bildrand klebt.
 */
const val START_PADDING_DP = 48.0

/** Kachelgröße der MapLibre-Zoomstufen: Bei Zoom z ist die Welt 512·2^z dp breit. */
private const val TILE_DP = 512.0

/** Rechteck in Geo-Koordinaten (Süd/West/Nord/Ost). */
data class GeoBounds(
    val south: Double,
    val west: Double,
    val north: Double,
    val east: Double,
)

/**
 * Rückfall-Ausschnitt: der bebaute Ortsbereich von Rössing.
 *
 * Ermittelt aus OpenStreetMap (Overpass, August 2026): umschließendes Rechteck
 * aller Flächen mit `landuse=residential|farmyard|commercial|industrial|retail`
 * im Umkreis von 2,5 km um den Ortskern, beschränkt auf die zusammenhängende
 * Bebauung (Flächenmittelpunkt höchstens 900 m vom Ortskern — weiter außen
 * liegen nur noch einzelne Höfe auf freiem Feld). Ergibt rund 1,5 km in
 * Nord-Süd- und 1,4 km in Ost-West-Richtung, also „ganz Rössing".
 */
val ROESSING_BOUNDS = GeoBounds(
    south = 52.1781,
    west = 9.8065,
    north = 52.1915,
    east = 9.8266,
)

/** Startausschnitt der Karte: umschließendes Rechteck samt passender Kamera. */
data class MapStart(val bounds: GeoBounds, val center: LatLon, val zoom: Double)

/** Umschließendes Rechteck aller Punkte — null, wenn es keine gibt. */
fun boundsOf(points: List<LatLon>): GeoBounds? {
    if (points.isEmpty()) return null
    return GeoBounds(
        south = points.minOf { it.lat },
        west = points.minOf { it.lon },
        north = points.maxOf { it.lat },
        east = points.maxOf { it.lon },
    )
}

/** Das Rechteck so vergrößern, dass der Punkt darin liegt. */
fun GeoBounds.extendedBy(p: LatLon) = GeoBounds(
    south = min(south, p.lat),
    west = min(west, p.lon),
    north = max(north, p.lat),
    east = max(east, p.lon),
)

/** Liegt der Punkt im Rechteck? */
fun GeoBounds.contains(p: LatLon) = p.lat in south..north && p.lon in west..east

/**
 * Mitte des Rechtecks. In Nord-Süd-Richtung in Mercator-Projektion gemittelt,
 * damit der Ausschnitt oben und unten wirklich gleich viel Luft lässt.
 */
val GeoBounds.center: LatLon
    get() = LatLon(
        lat = latFromMercatorY((mercatorY(south) + mercatorY(north)) / 2),
        lon = (west + east) / 2,
    )

/**
 * Ist der Standort nah genug am Dorf, um ihn überhaupt zu berücksichtigen?
 * Wer gerade in Hannover sitzt, will die Dorfkarte sehen und nicht seinen
 * Aufenthaltsort.
 */
fun isNearVillage(
    user: LatLon,
    village: LatLon = ROESSING,
    nearbyMeters: Double = NEARBY_METERS,
): Boolean = distanceMeters(user, village) <= nearbyMeters

/**
 * Größte Zoomstufe, bei der das Rechteck samt Rand noch komplett auf eine
 * Fläche von widthDp × heightDp passt — gedeckelt auf maxZoom.
 */
fun zoomForBounds(
    bounds: GeoBounds,
    widthDp: Double,
    heightDp: Double,
    paddingDp: Double = START_PADDING_DP,
    maxZoom: Double = MAX_START_ZOOM,
): Double {
    val nutzbareBreite = (widthDp - 2 * paddingDp).coerceAtLeast(1.0)
    val nutzbareHoehe = (heightDp - 2 * paddingDp).coerceAtLeast(1.0)
    // Anteil des Rechtecks an der ganzen Weltkarte (Web-Mercator, 0..1).
    val anteilX = ((bounds.east - bounds.west) / 360.0).coerceAtLeast(1e-12)
    val anteilY = (mercatorY(bounds.south) - mercatorY(bounds.north)).coerceAtLeast(1e-12)
    val zoomX = log2(nutzbareBreite / (TILE_DP * anteilX))
    val zoomY = log2(nutzbareHoehe / (TILE_DP * anteilY))
    return min(zoomX, zoomY).coerceIn(0.0, maxZoom)
}

/**
 * Startausschnitt der Karte: Beim Öffnen sollen möglichst **alle** Pflege-Orte
 * im Bild liegen, nicht ein zufälliger Ausschnitt.
 *
 * * Gibt es Orte, umschließt der Ausschnitt sie alle (mit Rand).
 * * Ist ein Standort in der Nähe des Dorfes bekannt, wird er in den Ausschnitt
 *   **aufgenommen** — die Kamera springt aber nicht auf ihn. Dafür gibt es den
 *   Knopf „Mein Standort".
 * * Ohne Orte zeigt die Karte den bebauten Ortsbereich (siehe ROESSING_BOUNDS).
 * * Nach oben deckelt maxZoom: ein einzelner Ort füllt nicht den Bildschirm.
 */
fun startView(
    places: List<LatLon>,
    user: LatLon? = null,
    widthDp: Double,
    heightDp: Double,
    paddingDp: Double = START_PADDING_DP,
    village: GeoBounds = ROESSING_BOUNDS,
    villageCenter: LatLon = ROESSING,
    nearbyMeters: Double = NEARBY_METERS,
    maxZoom: Double = MAX_START_ZOOM,
): MapStart {
    val orte = boundsOf(places) ?: village
    val ausschnitt = if (user != null && isNearVillage(user, villageCenter, nearbyMeters)) {
        orte.extendedBy(user)
    } else {
        orte
    }
    return MapStart(
        bounds = ausschnitt,
        center = ausschnitt.center,
        zoom = zoomForBounds(ausschnitt, widthDp, heightDp, paddingDp, maxZoom),
    )
}

/** Web-Mercator: Breitengrad → Anteil der Weltkarte von oben (0..1). */
private fun mercatorY(lat: Double): Double {
    val rad = Math.toRadians(lat.coerceIn(-85.05112878, 85.05112878))
    return (1 - ln(tan(rad) + 1 / cos(rad)) / PI) / 2
}

/** Umkehrung von [mercatorY]. */
private fun latFromMercatorY(y: Double): Double = Math.toDegrees(atan(sinh(PI * (1 - 2 * y))))

/**
 * Bis zu dieser Luftlinie gilt ein Standort als „in der Nähe des Dorfes".
 * Wer gerade in Hannover sitzt, will die Dorfkarte sehen und nicht seinen
 * Wohnort.
 */
const val NEARBY_METERS = 20_000.0

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
