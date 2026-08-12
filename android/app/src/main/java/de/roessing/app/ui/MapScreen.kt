package de.roessing.app.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import de.roessing.app.data.PlaceDto
import org.maplibre.android.MapLibre
import org.maplibre.android.camera.CameraPosition
import org.maplibre.android.geometry.LatLng
import org.maplibre.android.maps.MapLibreMap
import org.maplibre.android.maps.MapView
import org.maplibre.android.maps.Style
import org.maplibre.android.style.expressions.Expression.get
import org.maplibre.android.style.expressions.Expression.literal
import org.maplibre.android.style.expressions.Expression.match
import org.maplibre.android.style.expressions.Expression.stop
import org.maplibre.android.style.layers.CircleLayer
import org.maplibre.android.style.layers.PropertyFactory.circleColor
import org.maplibre.android.style.layers.PropertyFactory.circleOpacity
import org.maplibre.android.style.layers.PropertyFactory.circleRadius
import org.maplibre.android.style.layers.PropertyFactory.circleStrokeColor
import org.maplibre.android.style.layers.PropertyFactory.circleStrokeWidth
import org.maplibre.android.style.sources.GeoJsonSource
import org.maplibre.geojson.Feature
import org.maplibre.geojson.FeatureCollection
import org.maplibre.geojson.Point

// Rössing (Gemeinde Nordstemmen) — Startausschnitt der Karte.
private val ROESSING_CENTER = LatLng(52.2110, 9.8700)
private const val START_ZOOM = 15.2
// Freie Vektor-Kacheln ohne API-Key (OpenFreeMap, OSM-Daten).
private const val STYLE_URL = "https://tiles.openfreemap.org/styles/liberty"
private const val SOURCE_ID = "places"
private const val LAYER_ID = "places-layer"

/** Dorfkarte mit farbigen Status-Markern für alle Pflege-Orte. */
@Composable
fun MapScreen(
    places: List<PlaceDto>,
    modifier: Modifier = Modifier,
    onPlaceTap: (Long) -> Unit,
) {
    val context = LocalContext.current
    remember { MapLibre.getInstance(context) }
    val lifecycleOwner = LocalLifecycleOwner.current
    val currentPlaces = rememberUpdatedState(places)
    val currentOnTap = rememberUpdatedState(onPlaceTap)

    val mapView = remember {
        MapView(context).apply {
            getMapAsync { map ->
                map.cameraPosition = CameraPosition.Builder()
                    .target(ROESSING_CENTER).zoom(START_ZOOM).build()
                map.setStyle(Style.Builder().fromUri(STYLE_URL)) { style ->
                    style.addSource(GeoJsonSource(SOURCE_ID, toGeoJson(currentPlaces.value)))
                    style.addLayer(
                        CircleLayer(LAYER_ID, SOURCE_ID).withProperties(
                            circleRadius(11f),
                            circleColor(
                                match(
                                    get("status"), literal("#2E7D32"),
                                    stop("yellow", "#F9A825"),
                                    stop("red", "#C62828"),
                                ),
                            ),
                            circleStrokeWidth(2.5f),
                            circleStrokeColor("#FFFFFF"),
                            circleOpacity(0.95f),
                        ),
                    )
                }
                map.addOnMapClickListener { point ->
                    val screen = map.projection.toScreenLocation(point)
                    val features = map.queryRenderedFeatures(screen, LAYER_ID)
                    val id = features.firstOrNull()?.getNumberProperty("id")?.toLong()
                    if (id != null) currentOnTap.value(id)
                    id != null
                }
            }
        }
    }

    // Marker aktualisieren, wenn sich die Daten ändern.
    mapView.getMapAsync { map: MapLibreMap ->
        map.style?.getSourceAs<GeoJsonSource>(SOURCE_ID)?.setGeoJson(toGeoJson(places))
    }

    // MapView an den Activity-Lifecycle koppeln (Pflicht bei MapLibre).
    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            when (event) {
                Lifecycle.Event.ON_START -> mapView.onStart()
                Lifecycle.Event.ON_RESUME -> mapView.onResume()
                Lifecycle.Event.ON_PAUSE -> mapView.onPause()
                Lifecycle.Event.ON_STOP -> mapView.onStop()
                Lifecycle.Event.ON_DESTROY -> mapView.onDestroy()
                else -> {}
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose {
            lifecycleOwner.lifecycle.removeObserver(observer)
        }
    }

    AndroidView(factory = { mapView }, modifier = modifier)
}

private fun toGeoJson(places: List<PlaceDto>): FeatureCollection =
    FeatureCollection.fromFeatures(
        places.filter { it.active }.map { p ->
            Feature.fromGeometry(Point.fromLngLat(p.lon, p.lat)).apply {
                addNumberProperty("id", p.id)
                addStringProperty("status", p.status)
            }
        },
    )
