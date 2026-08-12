package de.roessing.app.ui

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.filled.EmojiEvents
import androidx.compose.material.icons.filled.Map
import androidx.compose.material.icons.filled.MyLocation
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.CenterAlignedTopAppBar
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import de.roessing.app.R
import de.roessing.app.data.DeviceLocation

/** Hauptbildschirm: Karte/Liste der Pflege-Orte mit Detail-Sheet. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    viewModel: PlacesViewModel,
    leaderboardViewModel: LeaderboardViewModel,
    onLogout: () -> Unit,
) {
    val state by viewModel.state.collectAsState()
    val leaderboard by leaderboardViewModel.state.collectAsState()
    val snackbar = remember { SnackbarHostState() }
    val context = LocalContext.current
    var tab by rememberSaveable { mutableStateOf(0) }
    var selectedPlaceId by rememberSaveable { mutableStateOf<Long?>(null) }
    var menuOpen by remember { mutableStateOf(false) }

    // Standort: nur im Vordergrund, nur auf Wunsch — und er bleibt auf dem
    // Gerät. Das Backend bekommt ihn nie zu sehen.
    val standort = remember { DeviceLocation(context) }
    var darfStandort by remember { mutableStateOf(standort.hasPermission()) }
    var abgelehnt by remember { mutableStateOf(false) }
    var fokusZaehler by rememberSaveable { mutableStateOf(0) }
    val standortFrage = rememberLauncherForActivityResult(
        ActivityResultContracts.RequestMultiplePermissions(),
    ) { ergebnis ->
        darfStandort = ergebnis.values.any { it }
        abgelehnt = !darfStandort
    }
    LaunchedEffect(darfStandort) {
        if (darfStandort) viewModel.setUserLocation(standort.current())
    }

    val savedMsg = stringResource(R.string.watered_success)
    val failMsg = stringResource(R.string.error_network)
    val deniedMsg = stringResource(R.string.location_denied)
    LaunchedEffect(Unit) {
        viewModel.events.collect { event ->
            when (event) {
                UiEvent.CompletionSaved -> snackbar.showSnackbar(savedMsg)
                UiEvent.CompletionFailed -> snackbar.showSnackbar(failMsg)
                is UiEvent.CompletionLocked -> snackbar.showSnackbar(
                    context.getString(
                        R.string.completion_locked,
                        event.until?.let { formatTime(it) } ?: "später",
                    ),
                )
            }
        }
    }
    LaunchedEffect(state.offline) {
        if (state.offline) snackbar.showSnackbar(failMsg)
    }
    LaunchedEffect(abgelehnt) {
        // Einmal freundlich erklären, dann nie wieder nörgeln.
        if (abgelehnt) snackbar.showSnackbar(deniedMsg)
    }
    // Beim Wechsel auf die Rangliste den aktuellen Stand holen.
    LaunchedEffect(tab) {
        if (tab == 2) leaderboardViewModel.refresh()
    }

    Scaffold(
        topBar = {
            CenterAlignedTopAppBar(
                title = { Text(stringResource(R.string.map_title)) },
                actions = {
                    IconButton(
                        onClick = {
                            viewModel.refresh()
                            leaderboardViewModel.refresh()
                        },
                        modifier = Modifier.testTag("refresh"),
                    ) {
                        Icon(Icons.Filled.Refresh, contentDescription = "Aktualisieren")
                    }
                    IconButton(onClick = { menuOpen = true }, modifier = Modifier.testTag("menu")) {
                        Text(state.me?.name?.take(1)?.uppercase() ?: "•")
                    }
                    DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                        state.me?.let {
                            DropdownMenuItem(text = { Text(it.name) }, onClick = {}, enabled = false)
                        }
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.logout)) },
                            onClick = { menuOpen = false; onLogout() },
                            modifier = Modifier.testTag("logout"),
                        )
                    }
                },
            )
        },
        bottomBar = {
            NavigationBar {
                NavigationBarItem(
                    selected = tab == 0, onClick = { tab = 0 },
                    icon = { Icon(Icons.Filled.Map, contentDescription = null) },
                    label = { Text(stringResource(R.string.map_title)) },
                    modifier = Modifier.testTag("tab-map"),
                )
                NavigationBarItem(
                    selected = tab == 1, onClick = { tab = 1 },
                    icon = { Icon(Icons.AutoMirrored.Filled.List, contentDescription = null) },
                    label = { Text(stringResource(R.string.list_title)) },
                    modifier = Modifier.testTag("tab-list"),
                )
                NavigationBarItem(
                    selected = tab == 2, onClick = { tab = 2 },
                    icon = { Icon(Icons.Filled.EmojiEvents, contentDescription = null) },
                    label = { Text(stringResource(R.string.leaderboard_title)) },
                    modifier = Modifier.testTag("tab-leaderboard"),
                )
            }
        },
        snackbarHost = { SnackbarHost(snackbar) },
        floatingActionButton = {
            if (tab == 0) {
                FloatingActionButton(
                    onClick = {
                        if (darfStandort) fokusZaehler++ else standortFrage.launch(DeviceLocation.PERMISSIONS)
                    },
                    modifier = Modifier.testTag("my-location"),
                ) {
                    Icon(Icons.Filled.MyLocation, contentDescription = stringResource(R.string.my_location))
                }
            }
        },
    ) { padding ->
        Column(Modifier.padding(padding)) {
            if (!darfStandort) {
                LocationHint(onRequest = { standortFrage.launch(DeviceLocation.PERMISSIONS) })
            }
            if (state.loading && state.places.isEmpty()) {
                LinearProgressIndicator(Modifier.fillMaxWidth())
            }
            when (tab) {
                0 -> MapScreen(
                    places = state.places,
                    modifier = Modifier.weight(1f).fillMaxWidth(),
                    userLocation = state.userLocation,
                    showUserLocation = darfStandort,
                    focusRequest = fokusZaehler,
                    onPlaceTap = { selectedPlaceId = it },
                )

                1 -> PlaceListScreen(
                    state = state,
                    modifier = Modifier.weight(1f).fillMaxWidth(),
                    onPlaceTap = { selectedPlaceId = it },
                    onSortChange = { viewModel.setSort(it) },
                )

                2 -> LeaderboardScreen(
                    state = leaderboard,
                    modifier = Modifier.weight(1f).fillMaxWidth(),
                    onSelectPeriod = { leaderboardViewModel.select(it) },
                )
            }
        }
    }

    val selected = state.places.find { it.id == selectedPlaceId }
    if (selected != null) {
        ModalBottomSheet(
            onDismissRequest = { selectedPlaceId = null },
            sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        ) {
            PlaceDetail(
                place = selected,
                pendingTasks = state.pendingTasks,
                history = viewModel.history.collectAsState().value,
                onComplete = { taskId, liters -> viewModel.complete(taskId, liters) },
                onLoadHistory = { viewModel.loadHistory(it) },
            )
        }
    }
}
