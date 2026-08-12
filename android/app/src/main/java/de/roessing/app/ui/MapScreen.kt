package de.roessing.app.ui

import android.annotation.SuppressLint
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.layout.onSizeChanged
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.platform.LocalLifecycleOwner
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import de.roessing.app.data.LatLon
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.ROESSING_BOUNDS
import de.roessing.app.data.USER_ZOOM
import de.roessing.app.data.center
import de.roessing.app.data.startView
import de.roessing.app.data.zoomForBounds
import org.maplibre.android.MapLibre
import org.maplibre.android.camera.CameraPosition
import org.maplibre.android.camera.CameraUpdateFactory
import org.maplibre.android.location.LocationComponentActivationOptions
import org.maplibre.android.location.modes.CameraMode
import org.maplibre.android.location.modes.RenderMode
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

// Erster Ausschnitt, bevor die Kartengröße bekannt ist: ganz Rössing auf einem
// gedachten kleinen Telefon (360 × 640 dp). Sobald die echte Größe und die Orte
// da sind, rechnet startView() den richtigen Ausschnitt (siehe Geo.kt).
private val ROESSING_CENTER = LatLng(ROESSING_BOUNDS.center.lat, ROESSING_BOUNDS.center.lon)
private val START_ZOOM = zoomForBounds(ROESSING_BOUNDS, widthDp = 360.0, heightDp = 640.0)
// Freie Vektor-Kacheln ohne API-Key (OpenFreeMap, OSM-Daten).
private const val STYLE_URL = "https://tiles.openfreemap.org/styles/liberty"
private const val SOURCE_ID = "places"
private const val LAYER_ID = "places-layer"

/**
 * Dorfkarte mit farbigen Status-Markern für alle Pflege-Orte.
 *
 * userLocation kommt vom Gerät und bleibt dort — die Karte nutzt ihn nur für
 * den Startausschnitt und den eigenen Standortpunkt.
 * focusRequest steigt bei jedem Druck auf „Mein Standort".
 */
@Composable
fun MapScreen(
    places: List<PlaceDto>,
    modifier: Modifier = Modifier,
    userLocation: LatLon? = null,
    showUserLocation: Boolean = false,
    focusRequest: Int = 0,
    onPlaceTap: (Long) -> Unit,
) {
    val context = LocalContext.current
    remember { MapLibre.getInstance(context) }
    val lifecycleOwner = LocalLifecycleOwner.current
    val dichte = LocalDensity.current
    val currentPlaces = rememberUpdatedState(places)
    val currentOnTap = rememberUpdatedState(onPlaceTap)

    // Kartengröße in dp — MapLibre rechnet Zoomstufen in genau dieser Einheit.
    var breiteDp by remember { mutableStateOf(0.0) }
    var hoeheDp by remember { mutableStateOf(0.0) }
    // Der Startausschnitt wird genau einmal gesetzt: sobald Orte da sind.
    var startGesetzt by remember { mutableStateOf(false) }
    // Sobald der Nutzer selbst schiebt oder zoomt, mischt sich die App nicht mehr ein.
    var vomNutzerBewegt by remember { mutableStateOf(false) }

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
                map.addOnCameraMoveStartedListener { grund ->
                    if (grund == MapLibreMap.OnCameraMoveStartedListener.REASON_API_GESTURE) {
                        vomNutzerBewegt = true
                    }
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

    // Startausschnitt: möglichst alle Orte ins Bild, ein naher Standort kommt
    // dazu (statt ihn anzuspringen). Solange noch keine Orte geladen sind,
    // zeigt die Karte ganz Rössing und rückt nach, sobald die Orte eintreffen.
    // Danach — oder sobald der Nutzer die Karte selbst angefasst hat — bleibt
    // die Kamera in Ruhe.
    LaunchedEffect(breiteDp, hoeheDp, places, userLocation) {
        if (startGesetzt || vomNutzerBewegt) return@LaunchedEffect
        if (breiteDp <= 0.0 || hoeheDp <= 0.0) return@LaunchedEffect
        val orte = places.filter { it.active }.map { LatLon(it.lat, it.lon) }
        val start = startView(orte, userLocation, breiteDp, hoeheDp)
        mapView.getMapAsync { map ->
            map.moveCamera(
                CameraUpdateFactory.newLatLngZoom(
                    LatLng(start.center.lat, start.center.lon), start.zoom,
                ),
            )
        }
        if (orte.isNotEmpty()) startGesetzt = true
    }

    // „Mein Standort": jeder Druck zentriert erneut und zoomt hinein — das ist
    // die bewusste Nutzeraktion und schlägt den automatischen Startausschnitt.
    LaunchedEffect(focusRequest) {
        val ich = userLocation ?: return@LaunchedEffect
        if (focusRequest <= 0) return@LaunchedEffect
        vomNutzerBewegt = true
        mapView.getMapAsync { map ->
            map.animateCamera(CameraUpdateFactory.newLatLngZoom(LatLng(ich.lat, ich.lon), USER_ZOOM))
        }
    }

    // Eigener Standortpunkt, sobald die Berechtigung da ist.
    LaunchedEffect(showUserLocation) {
        if (!showUserLocation) return@LaunchedEffect
        mapView.getMapAsync { map ->
            map.style?.let { style -> zeigeStandortpunkt(context, map, style) }
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

    AndroidView(
        factory = { mapView },
        modifier = modifier.onSizeChanged { groesse ->
            with(dichte) {
                breiteDp = groesse.width.toDp().value.toDouble()
                hoeheDp = groesse.height.toDp().value.toDouble()
            }
        },
    )
}

/**
 * Schaltet den blauen Standortpunkt (inkl. Blickrichtung) ein. Die
 * Berechtigung ist an dieser Stelle bereits erteilt — sonst wird nicht
 * aufgerufen; die Kamera bleibt in der Hand des Nutzers.
 */
@SuppressLint("MissingPermission")
private fun zeigeStandortpunkt(
    context: android.content.Context,
    map: MapLibreMap,
    style: Style,
) {
    runCatching {
        val komponente = map.locationComponent
        if (!komponente.isLocationComponentActivated) {
            komponente.activateLocationComponent(
                LocationComponentActivationOptions.builder(context, style).build(),
            )
        }
        komponente.isLocationComponentEnabled = true
        komponente.cameraMode = CameraMode.NONE
        komponente.renderMode = RenderMode.COMPASS
    }
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
