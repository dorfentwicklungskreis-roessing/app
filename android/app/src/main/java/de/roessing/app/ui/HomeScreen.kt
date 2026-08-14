package de.roessing.app.ui

import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.SizeTransform
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
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
import androidx.compose.material3.Scaffold
import androidx.compose.material3.ShortNavigationBar
import androidx.compose.material3.ShortNavigationBarItem
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
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
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.data.DeviceLocation
import de.roessing.app.ui.theme.DorfMotion

/**
 * Das Gerüst der App: oben die Startseite mit den Bereichen, darunter die
 * Bereiche selbst. „Mithelfen" ist der erste davon — Karte, Liste und
 * Rangliste liegen deshalb *in* diesem Bereich und nicht mehr auf der
 * obersten Ebene.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(
    viewModel: PlacesViewModel,
    leaderboardViewModel: LeaderboardViewModel,
    profileViewModel: ProfileViewModel,
    onLogout: () -> Unit,
) {
    val state by viewModel.state.collectAsState()
    val leaderboard by leaderboardViewModel.state.collectAsState()
    val profil by profileViewModel.state.collectAsState()
    var bereich by rememberSaveable { mutableStateOf(Bereich.START) }
    val snackbar = remember { SnackbarHostState() }
    val context = LocalContext.current
    var tab by rememberSaveable { mutableStateOf(0) }
    var selectedPlaceId by rememberSaveable { mutableStateOf<Long?>(null) }
    var menuOpen by remember { mutableStateOf(false) }

    // System-Zurück führt aus dem Bereich auf die Startseite — und erst von
    // dort aus der App heraus.
    BackHandler(enabled = bereich != Bereich.START) { bereich = Bereich.START }

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
    LaunchedEffect(tab, bereich) {
        if (bereich == Bereich.MITHELFEN && tab == 2) leaderboardViewModel.refresh()
    }
    // Die Dorfbewohner-Liste wird beim Öffnen frisch geholt.
    LaunchedEffect(bereich) {
        if (bereich == Bereich.DORFBEWOHNER) profileViewModel.loadMembers()
    }
    val profilGespeichert = stringResource(R.string.profile_saved)
    LaunchedEffect(Unit) {
        profileViewModel.events.collect { event ->
            when (event) {
                ProfileEvent.Saved -> snackbar.showSnackbar(profilGespeichert)
            }
        }
    }

    Scaffold(
        topBar = {
            CenterAlignedTopAppBar(
                title = {
                    Text(
                        when (bereich) {
                            Bereich.PROFIL -> stringResource(R.string.profile_title)
                            Bereich.DORFBEWOHNER -> stringResource(R.string.members_title)
                            Bereich.MITHELFEN -> stringResource(R.string.area_care_title)
                            Bereich.START -> stringResource(R.string.home_title)
                        },
                    )
                },
                navigationIcon = {
                    if (bereich != Bereich.START) {
                        IconButton(
                            onClick = { bereich = Bereich.START },
                            modifier = Modifier.testTag("zurueck"),
                        ) {
                            Icon(
                                Icons.AutoMirrored.Filled.ArrowBack,
                                contentDescription = stringResource(R.string.back),
                            )
                        }
                    }
                },
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
                        // Runder Anfangsbuchstabe statt loser Letter — das ist
                        // der Knopf zum eigenen Konto, er darf danach aussehen.
                        Surface(
                            shape = CircleShape,
                            color = MaterialTheme.colorScheme.primaryContainer,
                            contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
                            modifier = Modifier.size(34.dp),
                        ) {
                            Box(contentAlignment = Alignment.Center) {
                                Text(
                                    state.me?.name?.take(1)?.uppercase() ?: "•",
                                    style = MaterialTheme.typography.labelLarge,
                                )
                            }
                        }
                    }
                    DropdownMenu(expanded = menuOpen, onDismissRequest = { menuOpen = false }) {
                        state.me?.let {
                            DropdownMenuItem(text = { Text(it.name) }, onClick = {}, enabled = false)
                        }
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.profile_title)) },
                            onClick = { menuOpen = false; bereich = Bereich.PROFIL },
                            modifier = Modifier.testTag("menu-profil"),
                        )
                        DropdownMenuItem(
                            text = { Text(stringResource(R.string.members_title)) },
                            onClick = { menuOpen = false; bereich = Bereich.DORFBEWOHNER },
                            modifier = Modifier.testTag("menu-dorfbewohner"),
                        )
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
            // Die Reiter gehören in den Bereich Mithelfen — überall sonst
            // wären sie schlicht falsch.
            if (bereich == Bereich.MITHELFEN) {
                // ShortNavigationBar ist der Expressive-Nachfolger der
                // NavigationBar: flacher, mit weicher Auswahl-Pille.
                ShortNavigationBar {
                    ShortNavigationBarItem(
                        selected = tab == 0, onClick = { tab = 0 },
                        icon = { Icon(Icons.Filled.Map, contentDescription = null) },
                        label = { Text(stringResource(R.string.tab_map)) },
                        modifier = Modifier.testTag("tab-map"),
                    )
                    ShortNavigationBarItem(
                        selected = tab == 1, onClick = { tab = 1 },
                        icon = { Icon(Icons.AutoMirrored.Filled.List, contentDescription = null) },
                        label = { Text(stringResource(R.string.list_title)) },
                        modifier = Modifier.testTag("tab-list"),
                    )
                    ShortNavigationBarItem(
                        selected = tab == 2, onClick = { tab = 2 },
                        icon = { Icon(Icons.Filled.EmojiEvents, contentDescription = null) },
                        label = { Text(stringResource(R.string.leaderboard_title)) },
                        modifier = Modifier.testTag("tab-leaderboard"),
                    )
                }
            }
        },
        snackbarHost = { SnackbarHost(snackbar) },
        floatingActionButton = {
            if (bereich == Bereich.MITHELFEN && tab == 0) {
                FloatingActionButton(
                    onClick = {
                        if (darfStandort) fokusZaehler++ else standortFrage.launch(DeviceLocation.PERMISSIONS)
                    },
                    shape = MaterialTheme.shapes.large,
                    modifier = Modifier.testTag("my-location"),
                ) {
                    Icon(Icons.Filled.MyLocation, contentDescription = stringResource(R.string.my_location))
                }
            }
        },
    ) { padding ->
        // Bewegung mit Richtung: in einen Bereich hinein wandert der Inhalt
        // von rechts herein, zurück zur Startseite von links.
        AnimatedContent(
            targetState = bereich,
            transitionSpec = {
                val hinein = initialState == Bereich.START
                val richtung = if (hinein) 1 else -1
                (
                    slideInHorizontally(DorfMotion.raeumlich()) { breite -> richtung * breite / 6 } +
                        fadeIn(DorfMotion.effekt())
                    ) togetherWith (
                    slideOutHorizontally(DorfMotion.raeumlich()) { breite -> -richtung * breite / 6 } +
                        fadeOut(DorfMotion.effekt())
                    ) using SizeTransform(clip = false)
            },
            label = "bereich",
            modifier = Modifier.fillMaxSize(),
        ) { aktuell ->
            when (aktuell) {
                Bereich.START -> StartScreen(
                    name = vorname(profil, state),
                    faelligeOrte = state.faelligeOrte,
                    ladend = state.loading && state.places.isEmpty(),
                    modifier = Modifier.padding(padding),
                    onBereich = { bereich = it },
                )

                Bereich.PROFIL -> ProfileScreen(
                    state = profil,
                    modifier = Modifier.padding(padding),
                    onDisplayName = profileViewModel::setDisplayName,
                    onNickname = profileViewModel::setNickname,
                    onPhone = profileViewModel::setPhone,
                    onEmail = profileViewModel::setEmail,
                    onNote = profileViewModel::setNote,
                    onDisplayNamePublic = profileViewModel::setDisplayNamePublic,
                    onNicknamePublic = profileViewModel::setNicknamePublic,
                    onPhonePublic = profileViewModel::setPhonePublic,
                    onEmailPublic = profileViewModel::setEmailPublic,
                    onNotePublic = profileViewModel::setNotePublic,
                    onSave = profileViewModel::save,
                )

                Bereich.DORFBEWOHNER -> MembersScreen(
                    state = profil,
                    modifier = Modifier.padding(padding),
                )

                Bereich.MITHELFEN -> Column(Modifier.padding(padding)) {
                    // Der Hinweis gehört zu Karte und Liste — die Rangliste
                    // kennt keine Entfernungen.
                    if (!darfStandort && tab != 2) {
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
        }
    }

    val selected = state.places.find { it.id == selectedPlaceId }
    if (selected != null && bereich == Bereich.MITHELFEN) {
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

/**
 * Der Name für die Begrüßung: Nickname vor Anzeigename vor dem Namen aus der
 * Rössing-ID — und davon nur der erste Teil, „Moin, Erna Beispiel!" klänge
 * nach Behördenpost.
 */
private fun vorname(profil: ProfileUiState, state: PlacesUiState): String {
    val voll = profil.nickname.ifBlank { profil.displayName }.ifBlank { state.me?.name.orEmpty() }
    return voll.trim().substringBefore(' ')
}
