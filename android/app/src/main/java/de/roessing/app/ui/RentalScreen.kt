package de.roessing.app.ui

import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.EventBusy
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.filled.Phone
import androidx.compose.material.icons.filled.Place
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CenterAlignedTopAppBar
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.DateRangePicker
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberDateRangePickerState
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalUriHandler
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.data.BookingStatus
import de.roessing.app.data.LenderStatus
import de.roessing.app.data.OccupancyStatus
import de.roessing.app.data.RentalBlock
import de.roessing.app.data.RentalBooking
import de.roessing.app.data.RentalDevice
import de.roessing.app.data.RentalErrorCode
import de.roessing.app.data.RentalOccupancy
import de.roessing.app.data.RentalOwnerBooking
import de.roessing.app.data.RentalOwnerDevice
import de.roessing.app.data.RentalPeriod
import de.roessing.app.data.RentalSet
import de.roessing.app.data.euroText
import de.roessing.app.data.markdownAsPlainText
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset

/**
 * The area "Maschinchenring": what the village lends, and to whom.
 *
 * One screen with two rooms behind it. The catalogue is what people come for;
 * „Mein Profil im Maschinchenring" is reached from it, and „Meine Vermietung"
 * from the profile — the latter only where the platform said `approved`.
 *
 * Not a single decision about renting is made here. "Free", "taken", "may be
 * cancelled", "may be decided" — every one of those comes from the rental
 * platform; this file puts it on screen.
 */
@Composable
fun RentalScreen(
    state: RentalUiState,
    modifier: Modifier = Modifier,
    onQuery: (String) -> Unit = {},
    onRefresh: () -> Unit = {},
    onOpen: (RentalDevice) -> Unit = {},
    onClose: () -> Unit = {},
    onPeriod: (RentalPeriod) -> Unit = {},
    onBook: (notes: String, firstName: String, lastName: String, phone: String) -> Unit =
        { _, _, _, _ -> },
    onCancelBooking: (String) -> Unit = {},
    /** Null when there is no way to sign in from here. */
    onSignIn: (() -> Unit)? = null,
    /** Null when a fresh sign-in cannot be started from here. */
    onSignInAgain: (() -> Unit)? = null,
    onShowCatalog: () -> Unit = {},
    onShowProfile: () -> Unit = {},
    onShowOwner: () -> Unit = {},
    onSaveProfile: (
        name: String,
        phone: String,
        addressStreet: String,
        addressZip: String,
        addressCity: String,
    ) -> Unit = { _, _, _, _, _ -> },
    onAskToLend: () -> Unit = {},
    onRefreshOwner: () -> Unit = {},
    onApprove: (String) -> Unit = {},
    onReject: (String) -> Unit = {},
    onCancelOwnerBooking: (String) -> Unit = {},
    onOpenBlock: (String) -> Unit = {},
    onCloseBlock: () -> Unit = {},
    onAddBlock: (RentalPeriod, String) -> Unit = { _, _ -> },
    onRemoveBlock: (String) -> Unit = {},
) {
    // Zurück führt zuerst in den Katalog und erst dann aus dem Bereich: Wer im
    // Profil steht, will von dort zurück und nicht gleich auf die Startseite.
    BackHandler(enabled = state.page != RentalPage.CATALOG, onBack = onShowCatalog)

    when (state.page) {
        RentalPage.CATALOG -> RentalCatalogPage(
            state = state,
            modifier = modifier,
            onQuery = onQuery,
            onRefresh = onRefresh,
            onOpen = onOpen,
            onClose = onClose,
            onPeriod = onPeriod,
            onBook = onBook,
            onCancelBooking = onCancelBooking,
            onSignIn = onSignIn,
            onSignInAgain = onSignInAgain,
            onShowProfile = onShowProfile,
        )

        RentalPage.PROFILE -> RentalProfilePage(
            state = state,
            modifier = modifier,
            onBack = onShowCatalog,
            onSave = onSaveProfile,
            onAskToLend = onAskToLend,
            onShowOwner = onShowOwner,
        )

        RentalPage.OWNER -> RentalOwnerPage(
            state = state,
            modifier = modifier,
            onBack = onShowCatalog,
            onRefresh = onRefreshOwner,
            onApprove = onApprove,
            onReject = onReject,
            onCancelBooking = onCancelOwnerBooking,
            onOpenBlock = onOpenBlock,
            onCloseBlock = onCloseBlock,
            onAddBlock = onAddBlock,
            onRemoveBlock = onRemoveBlock,
        )
    }
}

/**
 * The catalogue: what the village lends.
 *
 * The order of the page follows what people come for. Searching sits at the
 * top, because whoever opens this area is usually after one particular thing.
 * Then whatever needs saying — no connection, sign in again, sign in at all.
 * Then my own bookings, because that is the second reason to look. Only then
 * the devices, and the sets after them.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun RentalCatalogPage(
    state: RentalUiState,
    modifier: Modifier = Modifier,
    onQuery: (String) -> Unit = {},
    onRefresh: () -> Unit = {},
    onOpen: (RentalDevice) -> Unit = {},
    onClose: () -> Unit = {},
    onPeriod: (RentalPeriod) -> Unit = {},
    onBook: (notes: String, firstName: String, lastName: String, phone: String) -> Unit =
        { _, _, _, _ -> },
    onCancelBooking: (String) -> Unit = {},
    onSignIn: (() -> Unit)? = null,
    onSignInAgain: (() -> Unit)? = null,
    onShowProfile: () -> Unit = {},
) {
    // Der Weg nach draußen wird **hier** nachgeschlagen, im Fenster der App —
    // und nicht unten im Blatt. Ein ModalBottomSheet ist ein eigenes Fenster
    // mit eigenem Owner, und der belegt `LocalUriHandler` selbst neu
    // (`ProvideCommonCompositionLocals`). Wer den Verweis erst dort drinnen
    // liest, bekommt darum immer den Browser des Systems — auch dann, wenn
    // die App außen etwas anderes untergeschoben bekommen hat. Im Test hieß
    // das: Chromium ging auf, die App verschwand im Hintergrund, und die drei
    // folgenden Prüfungen fanden ihre Oberfläche nicht wieder.
    val uriHandler = LocalUriHandler.current
    val browser: (String) -> Unit = { uriHandler.openUri(it) }

    Column(modifier.fillMaxWidth().testTag("rental")) {
        SearchField(state.query, onQuery)

        if (state.loading && state.devices.isEmpty()) {
            LinearProgressIndicator(Modifier.fillMaxWidth())
        }

        // First the hint, then the (possibly older) list — an empty page
        // without an explanation would be the worst outcome.
        if (state.offline) {
            Notice(
                text = if (state.devices.isEmpty()) {
                    stringResource(R.string.rental_offline)
                } else {
                    stringResource(R.string.rental_offline_stale)
                },
                actionText = stringResource(R.string.rental_retry),
                testTag = "rental-offline",
                actionTestTag = "rental-retry",
                onAction = onRefresh,
            )
        }

        if (state.staleSignIn && onSignInAgain != null) {
            Notice(
                title = stringResource(R.string.rental_stale_title),
                text = stringResource(R.string.rental_stale_body),
                actionText = stringResource(R.string.rental_stale_button),
                testTag = "rental-stale",
                actionTestTag = "rental-stale-signin",
                onAction = onSignInAgain,
            )
        }

        LazyColumn(
            // Die Kennung trägt die Liste selbst: Eine LazyColumn baut nur,
            // was zu sehen ist, und ein Test muss sie zum Blättern greifen
            // können, statt eine Kachel zu suchen, die es noch nicht gibt.
            Modifier.fillMaxWidth().testTag("rental-list"),
            contentPadding = PaddingValues(start = 20.dp, end = 20.dp, top = 12.dp, bottom = 28.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            item {
                Text(
                    stringResource(R.string.rental_intro),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }

            if (!state.signedIn && !state.staleSignIn && onSignIn != null) {
                item { SignInCard(onSignIn) }
            }

            // Der Weg zum eigenen Profil drüben — und von dort weiter zur
            // Vermieterseite, wenn die Plattform sie freigegeben hat.
            if (state.signedIn) {
                item { ProfileEntryCard(onShowProfile) }
            }

            if (state.signedIn) {
                item {
                    Text(
                        stringResource(R.string.rental_bookings_title),
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.padding(top = 8.dp),
                    )
                }
                if (state.bookingsOffline) {
                    item {
                        Text(
                            stringResource(R.string.rental_bookings_offline),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.error,
                            modifier = Modifier.testTag("rental-bookings-offline"),
                        )
                    }
                } else if (state.bookings.isEmpty()) {
                    item {
                        Text(
                            stringResource(R.string.rental_bookings_empty),
                            style = MaterialTheme.typography.bodyMedium,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                            modifier = Modifier.testTag("rental-bookings-empty"),
                        )
                    }
                }
                items(state.bookings, key = { "booking-${it.id}" }) { booking ->
                    BookingCard(
                        booking = booking,
                        busy = booking.id in state.cancelling,
                        onCancel = { onCancelBooking(booking.id) },
                    )
                }
            }

            items(state.devices, key = { it.id }) { device ->
                DeviceCard(device) { onOpen(device) }
            }

            if (state.empty) {
                item {
                    Text(
                        if (state.query.isBlank()) {
                            stringResource(R.string.rental_empty)
                        } else {
                            stringResource(R.string.rental_empty_search)
                        },
                        style = MaterialTheme.typography.bodyLarge,
                        modifier = Modifier.padding(top = 24.dp).testTag("rental-empty"),
                    )
                }
            }

            // Sets stehen unter den Geräten und nur in der ganzen Liste: Eine
            // Suche fragt den Server nach Geräten, und ihre Antwort sagt über
            // Sets nichts. Gebucht werden sie in der App nicht — Zusagen und
            // Stornieren kann der Server dafür noch nicht.
            if (state.sets.isNotEmpty() && state.query.isBlank()) {
                item {
                    Text(
                        stringResource(R.string.rental_sets_title),
                        style = MaterialTheme.typography.titleMedium,
                        modifier = Modifier.padding(top = 8.dp),
                    )
                }
                items(state.sets, key = { "set-${it.id}" }) { SetCard(it) }
                item {
                    Text(
                        stringResource(R.string.rental_sets_note),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }

            // Wer ein Gerät einstellen will, macht das drüben — die App
            // kennt dafür bewusst keinen Weg.
            item {
                Text(
                    stringResource(R.string.rental_manage_hint),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(top = 16.dp),
                )
            }
        }
    }

    val selected = state.selected
    if (selected != null) {
        ModalBottomSheet(
            onDismissRequest = onClose,
            sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        ) {
            DeviceDetail(
                device = selected,
                state = state,
                onPeriod = onPeriod,
                onBook = onBook,
                onOpenLink = browser,
            )
        }
    }
}

@Composable
private fun SearchField(query: String, onQuery: (String) -> Unit) {
    OutlinedTextField(
        value = query,
        onValueChange = onQuery,
        singleLine = true,
        label = { Text(stringResource(R.string.rental_search_label)) },
        placeholder = { Text(stringResource(R.string.rental_search_placeholder)) },
        leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
        trailingIcon = {
            if (query.isNotEmpty()) {
                IconButton(
                    onClick = { onQuery("") },
                    modifier = Modifier.testTag("rental-search-clear"),
                ) {
                    Icon(
                        Icons.Filled.Clear,
                        contentDescription = stringResource(R.string.rental_search_clear),
                    )
                }
            }
        },
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 20.dp, vertical = 8.dp)
            .testTag("rental-search"),
    )
}

/** The banner above the list: no connection, or sign in again. */
@Composable
private fun Notice(
    text: String,
    actionText: String,
    testTag: String,
    actionTestTag: String,
    onAction: () -> Unit,
    title: String? = null,
) {
    Surface(
        color = MaterialTheme.colorScheme.errorContainer,
        contentColor = MaterialTheme.colorScheme.onErrorContainer,
        shape = MaterialTheme.shapes.large,
        modifier = Modifier
            .fillMaxWidth()
            .padding(horizontal = 20.dp, vertical = 8.dp)
            .testTag(testTag),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            title?.let { Text(it, style = MaterialTheme.typography.titleSmall) }
            Text(text, style = MaterialTheme.typography.bodyMedium)
            TextButton(onClick = onAction, modifier = Modifier.testTag(actionTestTag)) {
                Text(actionText)
            }
        }
    }
}

/** Browsing works without an account; booking does not. Said once, calmly. */
@Composable
private fun SignInCard(onSignIn: () -> Unit) {
    Card(
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.secondaryContainer,
            contentColor = MaterialTheme.colorScheme.onSecondaryContainer,
        ),
        modifier = Modifier.fillMaxWidth().testTag("rental-signin"),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                stringResource(R.string.rental_signin_title),
                style = MaterialTheme.typography.titleSmall,
            )
            Text(
                stringResource(R.string.rental_signin_body),
                style = MaterialTheme.typography.bodyMedium,
            )
            Button(
                onClick = onSignIn,
                shape = MaterialTheme.shapes.large,
                modifier = Modifier.testTag("rental-signin-button"),
            ) { Text(stringResource(R.string.rental_signin_button)) }
        }
    }
}

@Composable
private fun DeviceCard(device: RentalDevice, onClick: () -> Unit) {
    Card(
        onClick = onClick,
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
        ),
        modifier = Modifier.fillMaxWidth().testTag("device-${device.id}"),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(device.name, style = MaterialTheme.typography.titleMedium)
            device.pricePerDay?.let {
                Text(
                    stringResource(R.string.rental_price_day, euroText(it)),
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
            device.description?.let {
                Text(
                    markdownAsPlainText(it),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 3,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

/** The German wording of a booking state. The four values are the contract's. */
@Composable
private fun statusText(booking: RentalBooking): String = when (booking.status) {
    BookingStatus.PENDING -> stringResource(R.string.rental_status_pending)
    BookingStatus.APPROVED -> stringResource(R.string.rental_status_approved)
    BookingStatus.REJECTED -> stringResource(R.string.rental_status_rejected)
    BookingStatus.CANCELLED -> stringResource(R.string.rental_status_cancelled)
    // A state a later version of the platform invented: show it as it came
    // rather than claim something.
    BookingStatus.UNKNOWN -> booking.rawStatus
}

@Composable
private fun BookingCard(booking: RentalBooking, busy: Boolean, onCancel: () -> Unit) {
    Card(
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.tertiaryContainer,
            contentColor = MaterialTheme.colorScheme.onTertiaryContainer,
        ),
        modifier = Modifier.fillMaxWidth().testTag("booking-${booking.id}"),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(booking.deviceName, style = MaterialTheme.typography.titleMedium)
            Text(booking.period.text, style = MaterialTheme.typography.bodyMedium)
            Text(
                stringResource(R.string.rental_return_day, dayText(booking.period.end)),
                style = MaterialTheme.typography.bodySmall,
            )
            Text(statusText(booking), style = MaterialTheme.typography.labelLarge)
            booking.notes?.let {
                Text(
                    stringResource(R.string.rental_note, it),
                    style = MaterialTheme.typography.bodySmall,
                )
            }
            // Adresse und Telefonnummer eines anderen Menschen — sie stehen
            // hier erst, nachdem er die Buchung bestätigt hat, und nirgends
            // sonst in der App.
            booking.pickup?.let { pickup ->
                Text(
                    stringResource(R.string.rental_pickup_title),
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.padding(top = 4.dp),
                )
                pickup.address?.let { Line(Icons.Filled.Place, it) }
                pickup.phone?.let { Text(stringResource(R.string.rental_pickup_phone, it)) }
            }
            // Der Knopf richtet sich nach canCancel — die App prüft den
            // Zustand nicht selbst nach.
            if (booking.canCancel) {
                OutlinedButton(
                    onClick = onCancel,
                    enabled = !busy,
                    shape = MaterialTheme.shapes.large,
                    modifier = Modifier.testTag("booking-cancel-${booking.id}"),
                ) { Text(stringResource(R.string.rental_cancel_booking)) }
            }
        }
    }
}

/**
 * The detail sheet: everything about one device, and the way to a booking.
 *
 * The button is enabled when the server has said the period is free — not
 * when this screen has worked it out from the taken periods next to it.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DeviceDetail(
    device: RentalDevice,
    state: RentalUiState,
    onPeriod: (RentalPeriod) -> Unit,
    onBook: (notes: String, firstName: String, lastName: String, phone: String) -> Unit,
    /** Führt eine Adresse nach draußen — gereicht, nicht hier nachgeschlagen. */
    onOpenLink: (String) -> Unit,
) {
    var picking by remember { mutableStateOf(false) }
    var notes by rememberSaveable(device.id) { mutableStateOf("") }
    var firstName by rememberSaveable(device.id) { mutableStateOf("") }
    var lastName by rememberSaveable(device.id) { mutableStateOf("") }
    var phone by rememberSaveable(device.id) { mutableStateOf("") }

    Column(
        Modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp)
            .padding(bottom = 32.dp)
            .testTag("device-detail"),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        Text(device.name, style = MaterialTheme.typography.headlineSmall)

        // Die Tarife, wie sie drüben stehen — einzeln, unverrechnet.
        val tariffs = listOfNotNull(
            device.pricePerDay?.let { stringResource(R.string.rental_price_day, euroText(it)) },
            device.pricePerWeekend?.let {
                stringResource(R.string.rental_price_weekend, euroText(it))
            },
            device.pricePerWeek?.let { stringResource(R.string.rental_price_week, euroText(it)) },
        )
        if (tariffs.isEmpty()) {
            Text(stringResource(R.string.rental_no_price), style = MaterialTheme.typography.bodyMedium)
        } else {
            tariffs.forEach {
                Text(
                    it,
                    style = MaterialTheme.typography.titleSmall,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
        }
        device.deposit?.let { Text(stringResource(R.string.rental_deposit, euroText(it))) }
        Text(
            stringResource(R.string.rental_price_note),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )

        device.description?.let {
            Text(
                markdownAsPlainText(it),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.testTag("device-description"),
            )
        }

        if (state.images.size > 1) {
            Text(
                stringResource(R.string.rental_images, state.images.size),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        // Belegte Zeiträume, gezeichnet wie der Server sie liefert. Sie sind
        // Anschauung, nicht die Antwort — die gibt die Verfügbarkeit.
        Text(
            stringResource(R.string.rental_occupied_title),
            style = MaterialTheme.typography.titleSmall,
            modifier = Modifier.padding(top = 8.dp),
        )
        if (state.occupancy.isEmpty()) {
            Text(
                stringResource(R.string.rental_occupied_none),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.testTag("rental-occupancy-empty"),
            )
        } else {
            state.occupancy.forEach { Line(Icons.Filled.EventBusy, occupancyText(it)) }
        }

        Spacer(Modifier.height(4.dp))
        OutlinedButton(
            onClick = { picking = true },
            shape = MaterialTheme.shapes.large,
            modifier = Modifier.fillMaxWidth().testTag("rental-pick-period"),
        ) {
            Text(
                if (state.period == null) {
                    stringResource(R.string.rental_choose_period)
                } else {
                    stringResource(R.string.rental_change_period)
                },
            )
        }

        state.period?.let { period ->
            Text(period.text, style = MaterialTheme.typography.titleSmall)
            Text(
                pluralStringResource(R.plurals.rental_days, period.days, period.days),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            Text(
                stringResource(R.string.rental_return_day, dayText(period.end)),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        when {
            state.checking -> Row(verticalAlignment = Alignment.CenterVertically) {
                CircularProgressIndicator(Modifier.size(16.dp))
                Spacer(Modifier.width(8.dp))
                Text(stringResource(R.string.rental_checking))
            }

            state.availability?.available == true -> Text(
                stringResource(R.string.rental_free),
                color = MaterialTheme.colorScheme.primary,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("rental-free"),
            )

            state.availability != null -> Text(
                stringResource(R.string.rental_taken),
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("rental-taken"),
            )
        }

        OutlinedTextField(
            value = notes,
            onValueChange = { notes = it },
            label = { Text(stringResource(R.string.rental_notes_label)) },
            minLines = 2,
            modifier = Modifier.fillMaxWidth().testTag("rental-notes"),
        )

        // Was fehlt, sagt der Server. Die App fragt genau das ab — und nur
        // dann, wenn er es gesagt hat.
        if (state.missingFields.isNotEmpty()) {
            Text(
                stringResource(R.string.rental_missing_title),
                style = MaterialTheme.typography.titleSmall,
                modifier = Modifier.testTag("rental-missing"),
            )
            if ("firstName" in state.missingFields || "name" in state.missingFields) {
                OutlinedTextField(
                    value = firstName,
                    onValueChange = { firstName = it },
                    singleLine = true,
                    label = { Text(stringResource(R.string.rental_missing_first_name)) },
                    modifier = Modifier.fillMaxWidth().testTag("rental-first-name"),
                )
            }
            if ("lastName" in state.missingFields || "name" in state.missingFields) {
                OutlinedTextField(
                    value = lastName,
                    onValueChange = { lastName = it },
                    singleLine = true,
                    label = { Text(stringResource(R.string.rental_missing_last_name)) },
                    modifier = Modifier.fillMaxWidth().testTag("rental-last-name"),
                )
            }
            if ("phone" in state.missingFields) {
                OutlinedTextField(
                    value = phone,
                    onValueChange = { phone = it },
                    singleLine = true,
                    label = { Text(stringResource(R.string.rental_missing_phone)) },
                    modifier = Modifier.fillMaxWidth().testTag("rental-phone"),
                )
            }
            // Adressfelder kann eine Buchung nicht mitschicken — dafür führt
            // der Weg auf die Webseite des Maschinchenrings.
            if (state.missingFields.any { it.startsWith("address") }) {
                Text(
                    stringResource(R.string.rental_missing_web),
                    style = MaterialTheme.typography.bodySmall,
                )
                device.webUrl?.let { url ->
                    TextButton(
                        onClick = { onOpenLink(url) },
                        modifier = Modifier.testTag("rental-missing-web"),
                    ) { Text(stringResource(R.string.rental_web_link)) }
                }
            }
        }

        state.notice?.let { notice ->
            Text(
                notice.text ?: stringResource(fallbackFor(notice.code)),
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("rental-refusal"),
            )
        }

        Button(
            onClick = { onBook(notes, firstName, lastName, phone) },
            enabled = state.canBook,
            shape = MaterialTheme.shapes.large,
            contentPadding = PaddingValues(vertical = 14.dp),
            modifier = Modifier.fillMaxWidth().testTag("rental-book"),
        ) {
            Text(
                if (state.booking) {
                    stringResource(R.string.rental_booking)
                } else {
                    stringResource(R.string.rental_book)
                },
            )
        }

        device.webUrl?.let { url ->
            TextButton(
                onClick = { onOpenLink(url) },
                modifier = Modifier.testTag("rental-web"),
            ) { Text(stringResource(R.string.rental_web_link)) }
        }
        device.productUrl?.let { url ->
            TextButton(
                onClick = { onOpenLink(url) },
                modifier = Modifier.testTag("rental-product"),
            ) { Text(stringResource(R.string.rental_product_link)) }
        }
    }

    if (picking) {
        val picker = rememberDateRangePickerState()
        DatePickerDialog(
            onDismissRequest = { picking = false },
            confirmButton = {
                TextButton(
                    onClick = {
                        val period = periodOfPicked(
                            picker.selectedStartDateMillis,
                            picker.selectedEndDateMillis,
                        )
                        picking = false
                        period?.let(onPeriod)
                    },
                    modifier = Modifier.testTag("rental-period-ok"),
                ) { Text(stringResource(R.string.rental_period_ok)) }
            },
            dismissButton = {
                TextButton(onClick = { picking = false }) {
                    Text(stringResource(R.string.rental_period_abort))
                }
            },
        ) {
            DateRangePicker(picker, modifier = Modifier.testTag("rental-period-picker"))
        }
    }
}

/** German wording for a taken period, with the reason it is taken. */
@Composable
private fun occupancyText(occupancy: RentalOccupancy): String {
    val what = when (occupancy.status) {
        OccupancyStatus.PENDING -> stringResource(R.string.rental_occupied_pending)
        OccupancyStatus.APPROVED -> stringResource(R.string.rental_occupied_approved)
        OccupancyStatus.BLOCKED -> stringResource(R.string.rental_occupied_blocked)
        // Whatever it is, it means taken — and that is all the calendar needs.
        OccupancyStatus.UNKNOWN -> stringResource(R.string.rental_occupied_approved)
    }
    return "${occupancy.period.text} ($what)"
}

/** Our own wording, used only where the server sent none. */
private fun fallbackFor(code: RentalErrorCode): Int = when (code) {
    RentalErrorCode.OCCUPIED -> R.string.rental_error_occupied
    RentalErrorCode.INVALID_PERIOD -> R.string.rental_error_invalid_period
    RentalErrorCode.BAD_REQUEST -> R.string.rental_error_bad_request
    RentalErrorCode.PROFILE_INCOMPLETE -> R.string.rental_error_profile_incomplete
    RentalErrorCode.FORBIDDEN, RentalErrorCode.NOT_A_LENDER -> R.string.rental_error_forbidden
    RentalErrorCode.NOT_FOUND -> R.string.rental_error_not_found
    RentalErrorCode.CONFLICT -> R.string.rental_error_conflict
    RentalErrorCode.RATE_LIMITED -> R.string.rental_error_rate_limited
    RentalErrorCode.INTERNAL -> R.string.rental_error_internal
    else -> R.string.rental_error_unknown
}

/** "7. September 2026" — used for the return day. */
private fun dayText(date: LocalDate): String =
    "${date.dayOfMonth}. ${MONTH_NAMES[date.monthValue - 1]} ${date.year}"

private val MONTH_NAMES = arrayOf(
    "Januar", "Februar", "März", "April", "Mai", "Juni",
    "Juli", "August", "September", "Oktober", "November", "Dezember",
)

/**
 * Turns the picker's two instants into a period.
 *
 * Two things happen here, and both are easy to get wrong. Material hands out
 * UTC midnights, so the days are read back in UTC — otherwise a tap in
 * Central European Summer Time lands on the day before. And the calendar
 * hands out the **days somebody wants the device**, while the contract wants
 * the **day it comes back**: the last picked day plus one. Picking a single
 * day means one day of renting.
 */
internal fun periodOfPicked(startMillis: Long?, endMillis: Long?): RentalPeriod? {
    val first = startMillis?.let { day(it) } ?: return null
    val last = endMillis?.let { day(it) } ?: first
    if (last.isBefore(first)) return null
    return RentalPeriod.ofPickedDays(first, last)
}

private fun day(millis: Long): LocalDate =
    Instant.ofEpochMilli(millis).atZone(ZoneOffset.UTC).toLocalDate()

@Composable
private fun Line(symbol: ImageVector, text: String) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Icon(
            symbol,
            contentDescription = null,
            modifier = Modifier.size(16.dp),
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.width(8.dp))
        Text(
            text,
            style = MaterialTheme.typography.bodyMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

// --- Der Weg zum eigenen Profil und zur Vermieterseite -----------------------

/** The way into „Mein Profil im Maschinchenring". Only shown when signed in. */
@Composable
private fun ProfileEntryCard(onClick: () -> Unit) {
    Card(
        onClick = onClick,
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
        ),
        modifier = Modifier.fillMaxWidth().testTag("rental-profile-entry"),
    ) {
        Row(
            Modifier.padding(16.dp).fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Icon(Icons.Filled.Person, contentDescription = null, Modifier.size(20.dp))
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(
                    stringResource(R.string.rental_profile_entry),
                    style = MaterialTheme.typography.titleSmall,
                )
                Text(
                    stringResource(R.string.rental_profile_entry_body),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Icon(Icons.AutoMirrored.Filled.KeyboardArrowRight, contentDescription = null)
        }
    }
}

/** A set, as it stands over there: shown with its tariff, not bookable here. */
@Composable
private fun SetCard(set: RentalSet) {
    Card(
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
        ),
        modifier = Modifier.fillMaxWidth().testTag("set-${set.id}"),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(set.name, style = MaterialTheme.typography.titleMedium)
            // Bei Sets ist die Beschreibung Klartext, kein Markdown — sie geht
            // also unverändert auf den Schirm.
            set.description?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            set.pricePerDay?.let {
                Text(
                    stringResource(R.string.rental_price_day, euroText(it)),
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
            set.deposit?.let { Text(stringResource(R.string.rental_deposit, euroText(it))) }
            if (set.itemIds.isNotEmpty()) {
                Text(
                    pluralStringResource(
                        R.plurals.rental_set_items,
                        set.itemIds.size,
                        set.itemIds.size,
                    ),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

/** The head of a page behind the catalogue: a way back and a title. */
@Composable
private fun PageHeader(title: String, onBack: () -> Unit) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        IconButton(onClick = onBack, modifier = Modifier.testTag("rental-to-catalog")) {
            Icon(
                Icons.AutoMirrored.Filled.ArrowBack,
                contentDescription = stringResource(R.string.back),
            )
        }
        Text(title, style = MaterialTheme.typography.titleLarge)
    }
}

// --- Mein Profil im Maschinchenring -----------------------------------------

/**
 * „Mein Profil im Maschinchenring" — telephone and address, so a handover can
 * be arranged, plus the way to ask for permission to lend things out.
 *
 * The profile lives over there, not in the village backend: it is the one the
 * lender sees. Whether it is complete enough is the platform's answer
 * (`profileComplete`, `missingFields`), not a check of ours — and the e-mail
 * address comes from the Rössing-ID and cannot be changed here at all.
 */
@Composable
private fun RentalProfilePage(
    state: RentalUiState,
    modifier: Modifier = Modifier,
    onBack: () -> Unit,
    onSave: (
        name: String,
        phone: String,
        addressStreet: String,
        addressZip: String,
        addressCity: String,
    ) -> Unit,
    onAskToLend: () -> Unit,
    onShowOwner: () -> Unit,
) {
    val profile = state.profile
    // Die Felder werden aus dem Profil vorbelegt und gehören danach dem, der
    // tippt. Kommt drüben ein anderer Wert an, wechselt der Schlüssel und das
    // Feld zeigt ihn — sonst bliebe ein alter Stand stehen.
    var name by rememberSaveable(profile?.name) { mutableStateOf(profile?.name.orEmpty()) }
    var phone by rememberSaveable(profile?.phone) { mutableStateOf(profile?.phone.orEmpty()) }
    var street by rememberSaveable(profile?.addressStreet) {
        mutableStateOf(profile?.addressStreet.orEmpty())
    }
    var zip by rememberSaveable(profile?.addressZip) {
        mutableStateOf(profile?.addressZip.orEmpty())
    }
    var city by rememberSaveable(profile?.addressCity) {
        mutableStateOf(profile?.addressCity.orEmpty())
    }

    Column(
        modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp)
            .padding(bottom = 32.dp)
            .testTag("rental-profile"),
        verticalArrangement = Arrangement.spacedBy(10.dp),
    ) {
        PageHeader(stringResource(R.string.rental_profile_title), onBack)

        if (profile == null) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                CircularProgressIndicator(Modifier.size(16.dp))
                Spacer(Modifier.width(8.dp))
                Text(stringResource(R.string.rental_profile_loading))
            }
        }

        state.notice?.let { notice ->
            Text(
                notice.text ?: stringResource(fallbackFor(notice.code)),
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("rental-profile-refusal"),
            )
        }

        if (profile != null) {
            profile.email?.let {
                Text(
                    stringResource(R.string.rental_profile_email, it),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            Text(
                stringResource(R.string.rental_profile_section),
                style = MaterialTheme.typography.titleSmall,
            )
            OutlinedTextField(
                value = name,
                onValueChange = { name = it },
                singleLine = true,
                label = { Text(stringResource(R.string.rental_field_name)) },
                modifier = Modifier.fillMaxWidth().testTag("rental-profile-name"),
            )
            OutlinedTextField(
                value = phone,
                onValueChange = { phone = it },
                singleLine = true,
                label = { Text(stringResource(R.string.rental_field_phone)) },
                modifier = Modifier.fillMaxWidth().testTag("rental-profile-phone"),
            )
            OutlinedTextField(
                value = street,
                onValueChange = { street = it },
                singleLine = true,
                label = { Text(stringResource(R.string.rental_field_street)) },
                modifier = Modifier.fillMaxWidth().testTag("rental-profile-street"),
            )
            OutlinedTextField(
                value = zip,
                onValueChange = { zip = it },
                singleLine = true,
                label = { Text(stringResource(R.string.rental_field_zip)) },
                modifier = Modifier.fillMaxWidth().testTag("rental-profile-zip"),
            )
            OutlinedTextField(
                value = city,
                onValueChange = { city = it },
                singleLine = true,
                label = { Text(stringResource(R.string.rental_field_city)) },
                modifier = Modifier.fillMaxWidth().testTag("rental-profile-city"),
            )

            // Was fehlt, sagt der Server — die App führt keine eigene Liste.
            if (profile.missingFields.isNotEmpty()) {
                val missing = profile.missingFields.map { fieldLabel(it) }.joinToString(", ")
                Text(
                    stringResource(R.string.rental_profile_missing, missing),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.testTag("rental-profile-missing"),
                )
            }

            Button(
                onClick = { onSave(name, phone, street, zip, city) },
                enabled = !state.savingProfile,
                shape = MaterialTheme.shapes.large,
                contentPadding = PaddingValues(vertical = 14.dp),
                modifier = Modifier.fillMaxWidth().testTag("rental-profile-save"),
            ) {
                Text(
                    if (state.savingProfile) {
                        stringResource(R.string.rental_profile_saving)
                    } else {
                        stringResource(R.string.rental_profile_save)
                    },
                )
            }
            Text(
                stringResource(R.string.rental_profile_note),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )

            HorizontalDivider(Modifier.padding(vertical = 8.dp))

            Text(
                stringResource(R.string.rental_lender_title),
                style = MaterialTheme.typography.titleSmall,
            )
            Text(
                lenderStatusText(profile.lenderStatus),
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("rental-lender-status"),
            )
            // Der Satz der Plattform, so wie er kam.
            state.lenderMessage?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag("rental-lender-message"),
                )
            }
            if (state.canAskToLend) {
                OutlinedButton(
                    onClick = onAskToLend,
                    enabled = !state.askingToLend,
                    shape = MaterialTheme.shapes.large,
                    modifier = Modifier.fillMaxWidth().testTag("rental-ask-to-lend"),
                ) {
                    Text(
                        if (state.askingToLend) {
                            stringResource(R.string.rental_lender_asking)
                        } else {
                            stringResource(R.string.rental_lender_ask)
                        },
                    )
                }
            }
            // Die Vermieterseite steht da, weil die Plattform „approved"
            // gesagt hat — nie, weil die App etwas ausgerechnet hätte.
            if (state.showsLenderArea) {
                Button(
                    onClick = onShowOwner,
                    shape = MaterialTheme.shapes.large,
                    modifier = Modifier.fillMaxWidth().testTag("rental-owner-entry"),
                ) { Text(stringResource(R.string.rental_owner_entry)) }
            }
            Text(
                stringResource(R.string.rental_lender_note),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/**
 * The platform names its fields in English (`addressZip`); a person reads
 * German. A name this version does not know is passed through rather than
 * dropped — whoever reads „addressCountry" at least knows something is
 * missing.
 */
@Composable
private fun fieldLabel(field: String): String = when (field) {
    "name" -> stringResource(R.string.rental_field_name)
    "phone" -> stringResource(R.string.rental_field_phone)
    "addressStreet" -> stringResource(R.string.rental_field_street)
    "addressZip" -> stringResource(R.string.rental_field_zip)
    "addressCity" -> stringResource(R.string.rental_field_city)
    else -> field
}

/** The German wording of the lender's state. The three values are the contract's. */
@Composable
private fun lenderStatusText(status: LenderStatus): String = when (status) {
    LenderStatus.NONE -> stringResource(R.string.rental_lender_none)
    LenderStatus.PENDING -> stringResource(R.string.rental_lender_pending)
    LenderStatus.APPROVED -> stringResource(R.string.rental_lender_approved)
    LenderStatus.UNKNOWN -> stringResource(R.string.rental_lender_unknown)
}

// --- Meine Vermietung --------------------------------------------------------

/**
 * „Meine Vermietung" — who is asking for my devices, which devices are mine,
 * and which stretches I keep for myself.
 *
 * Reachable only from the profile, and only where the platform has approved
 * somebody as a lender. Adding or changing a device is **not** here: that
 * happens in the chat and the web version of the Maschinchenring, and it
 * stays there.
 */
@Composable
private fun RentalOwnerPage(
    state: RentalUiState,
    modifier: Modifier = Modifier,
    onBack: () -> Unit,
    onRefresh: () -> Unit,
    onApprove: (String) -> Unit,
    onReject: (String) -> Unit,
    onCancelBooking: (String) -> Unit,
    onOpenBlock: (String) -> Unit,
    onCloseBlock: () -> Unit,
    onAddBlock: (RentalPeriod, String) -> Unit,
    onRemoveBlock: (String) -> Unit,
) {
    LazyColumn(
        modifier.fillMaxWidth().testTag("rental-owner"),
        contentPadding = PaddingValues(start = 20.dp, end = 20.dp, bottom = 28.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        item { PageHeader(stringResource(R.string.rental_owner_title), onBack) }

        // Ein missglückter Abruf leert keine Liste — der Hinweis steht über
        // dem, was zuletzt da war.
        if (state.ownerOffline) {
            item {
                Notice(
                    text = stringResource(R.string.rental_owner_offline),
                    actionText = stringResource(R.string.rental_retry),
                    testTag = "rental-owner-offline",
                    actionTestTag = "rental-owner-retry",
                    onAction = onRefresh,
                )
            }
        }

        state.notice?.let { notice ->
            item {
                Text(
                    notice.text ?: stringResource(fallbackFor(notice.code)),
                    color = MaterialTheme.colorScheme.error,
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.testTag("rental-owner-refusal"),
                )
            }
        }

        item {
            Text(
                if (state.waitingRequests > 0) {
                    stringResource(R.string.rental_owner_requests_open, state.waitingRequests)
                } else {
                    stringResource(R.string.rental_owner_requests)
                },
                style = MaterialTheme.typography.titleMedium,
            )
        }
        if (state.ownerLoading && state.ownerBookings.isEmpty()) {
            item {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    CircularProgressIndicator(Modifier.size(16.dp))
                    Spacer(Modifier.width(8.dp))
                    Text(stringResource(R.string.rental_owner_loading))
                }
            }
        }
        items(state.ownerBookings, key = { "owner-${it.id}" }) { booking ->
            OwnerBookingCard(
                booking = booking,
                busy = booking.id in state.deciding,
                onApprove = { onApprove(booking.id) },
                onReject = { onReject(booking.id) },
                onCancel = { onCancelBooking(booking.id) },
            )
        }
        if (state.ownerBookings.isEmpty() && !state.ownerLoading) {
            item {
                Text(
                    stringResource(R.string.rental_owner_no_bookings),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag("rental-owner-no-bookings"),
                )
            }
        }
        item {
            Text(
                stringResource(R.string.rental_owner_requests_note),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        item {
            Text(
                stringResource(R.string.rental_owner_devices),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.padding(top = 8.dp),
            )
        }
        items(state.ownerDevices, key = { "owner-device-${it.device.id}" }) { own ->
            OwnerDeviceCard(own) { onOpenBlock(own.device.id) }
        }
        if (state.ownerDevices.isEmpty() && !state.ownerLoading) {
            item {
                Text(
                    stringResource(R.string.rental_owner_no_devices),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag("rental-owner-no-devices"),
                )
            }
        }
        item {
            Text(
                stringResource(R.string.rental_owner_devices_note),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        item {
            Text(
                stringResource(R.string.rental_owner_blocks),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.padding(top = 8.dp),
            )
        }
        items(state.blocks, key = { "block-${it.id}" }) { block ->
            BlockCard(
                block = block,
                busy = block.id in state.deciding,
                onRemove = { onRemoveBlock(block.id) },
            )
        }
        if (state.blocks.isEmpty() && !state.ownerLoading) {
            item {
                Text(
                    stringResource(R.string.rental_owner_no_blocks),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag("rental-owner-no-blocks"),
                )
            }
        }
    }

    state.blockDeviceId?.let { deviceId ->
        BlockDialog(
            deviceName = state.ownerDevices
                .firstOrNull { it.device.id == deviceId }?.device?.name.orEmpty(),
            state = state,
            onDismiss = onCloseBlock,
            onSave = onAddBlock,
        )
    }
}

/**
 * One request on one of my devices — with the decisions the platform says are
 * possible right now.
 *
 * Name and telephone stand here because the handover has to be arranged, and
 * they stand nowhere else in the app.
 */
@Composable
private fun OwnerBookingCard(
    booking: RentalOwnerBooking,
    busy: Boolean,
    onApprove: () -> Unit,
    onReject: () -> Unit,
    onCancel: () -> Unit,
) {
    Card(
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.tertiaryContainer,
            contentColor = MaterialTheme.colorScheme.onTertiaryContainer,
        ),
        modifier = Modifier.fillMaxWidth().testTag("owner-booking-${booking.id}"),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(booking.deviceName, style = MaterialTheme.typography.titleMedium)
            Text(booking.period.text, style = MaterialTheme.typography.bodyMedium)
            Text(
                stringResource(R.string.rental_return_day, dayText(booking.period.end)),
                style = MaterialTheme.typography.bodySmall,
            )
            Text(ownerStatusText(booking), style = MaterialTheme.typography.labelLarge)
            booking.renterName?.let {
                Line(Icons.Filled.Person, stringResource(R.string.rental_owner_renter, it))
            }
            booking.renterPhone?.let {
                Line(Icons.Filled.Phone, stringResource(R.string.rental_owner_phone, it))
            }
            booking.notes?.let {
                Text(
                    stringResource(R.string.rental_owner_note, it),
                    style = MaterialTheme.typography.bodySmall,
                )
            }
            // Die Knöpfe richten sich nach canDecide und canCancel. Ob eine
            // Entscheidung noch offen ist, sagt die Plattform.
            Row(
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                if (booking.canDecide) {
                    Button(
                        onClick = onApprove,
                        enabled = !busy,
                        shape = MaterialTheme.shapes.large,
                        modifier = Modifier.testTag("rental-approve-${booking.id}"),
                    ) { Text(stringResource(R.string.rental_approve)) }
                    OutlinedButton(
                        onClick = onReject,
                        enabled = !busy,
                        shape = MaterialTheme.shapes.large,
                        modifier = Modifier.testTag("rental-reject-${booking.id}"),
                    ) { Text(stringResource(R.string.rental_reject)) }
                } else if (booking.canCancel) {
                    OutlinedButton(
                        onClick = onCancel,
                        enabled = !busy,
                        shape = MaterialTheme.shapes.large,
                        modifier = Modifier.testTag("owner-cancel-${booking.id}"),
                    ) { Text(stringResource(R.string.rental_owner_cancel)) }
                }
            }
        }
    }
}

/** The German wording of a request's state, seen from the lender's side. */
@Composable
private fun ownerStatusText(booking: RentalOwnerBooking): String = when (booking.status) {
    BookingStatus.PENDING -> stringResource(R.string.rental_status_pending)
    BookingStatus.APPROVED -> stringResource(R.string.rental_status_approved)
    BookingStatus.REJECTED -> stringResource(R.string.rental_status_rejected)
    BookingStatus.CANCELLED -> stringResource(R.string.rental_status_cancelled)
    BookingStatus.UNKNOWN -> booking.rawStatus
}

/** One of my devices, with the way to keep a stretch for myself. */
@Composable
private fun OwnerDeviceCard(own: RentalOwnerDevice, onBlock: () -> Unit) {
    Card(
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
        ),
        modifier = Modifier.fillMaxWidth().testTag("owner-device-${own.device.id}"),
    ) {
        Row(
            Modifier.padding(16.dp).fillMaxWidth(),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                Text(own.device.name, style = MaterialTheme.typography.titleSmall)
                // Abgeschaltete Geräte stehen nur hier — in der öffentlichen
                // Liste tauchen sie nicht auf.
                if (!own.active) {
                    Text(
                        stringResource(R.string.rental_owner_inactive),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
            }
            OutlinedButton(
                onClick = onBlock,
                shape = MaterialTheme.shapes.large,
                modifier = Modifier.testTag("rental-block-${own.device.id}"),
            ) { Text(stringResource(R.string.rental_block)) }
        }
    }
}

/** One of my blocks, with the way to lift it. */
@Composable
private fun BlockCard(block: RentalBlock, busy: Boolean, onRemove: () -> Unit) {
    Card(
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
        ),
        modifier = Modifier.fillMaxWidth().testTag("block-${block.id}"),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(block.deviceName, style = MaterialTheme.typography.titleSmall)
            Line(Icons.Filled.EventBusy, block.period.text)
            block.reason?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            OutlinedButton(
                onClick = onRemove,
                enabled = !busy,
                shape = MaterialTheme.shapes.large,
                modifier = Modifier.testTag("rental-unblock-${block.id}"),
            ) { Text(stringResource(R.string.rental_unblock)) }
        }
    }
}

/**
 * „Zeitraum sperren" — the lender keeps a stretch for themselves.
 *
 * The same conversion as everywhere in this area: the calendar hands out the
 * days somebody needs the device, the contract wants the day it comes back
 * ([periodOfPicked]). A refusal stays in the sheet, because closing it would
 * throw away what was just typed.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun BlockDialog(
    deviceName: String,
    state: RentalUiState,
    onDismiss: () -> Unit,
    onSave: (RentalPeriod, String) -> Unit,
) {
    var period by remember { mutableStateOf<RentalPeriod?>(null) }
    var reason by rememberSaveable { mutableStateOf("") }
    var picking by remember { mutableStateOf(false) }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(R.string.rental_block_title)) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(deviceName, style = MaterialTheme.typography.titleSmall)
                OutlinedButton(
                    onClick = { picking = true },
                    shape = MaterialTheme.shapes.large,
                    modifier = Modifier.fillMaxWidth().testTag("rental-block-period"),
                ) {
                    Text(
                        if (period == null) {
                            stringResource(R.string.rental_choose_period)
                        } else {
                            stringResource(R.string.rental_change_period)
                        },
                    )
                }
                period?.let {
                    Text(it.text, style = MaterialTheme.typography.titleSmall)
                    Text(
                        stringResource(R.string.rental_return_day, dayText(it.end)),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                OutlinedTextField(
                    value = reason,
                    onValueChange = { reason = it },
                    singleLine = true,
                    label = { Text(stringResource(R.string.rental_block_reason)) },
                    modifier = Modifier.fillMaxWidth().testTag("rental-block-reason"),
                )
                Text(
                    stringResource(R.string.rental_block_note),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                state.notice?.let { notice ->
                    Text(
                        notice.text ?: stringResource(fallbackFor(notice.code)),
                        color = MaterialTheme.colorScheme.error,
                        style = MaterialTheme.typography.bodyMedium,
                        modifier = Modifier.testTag("rental-block-refusal"),
                    )
                }
            }
        },
        confirmButton = {
            TextButton(
                onClick = { period?.let { onSave(it, reason) } },
                enabled = period != null && !state.blocking,
                modifier = Modifier.testTag("rental-block-save"),
            ) {
                Text(
                    if (state.blocking) {
                        stringResource(R.string.rental_block_saving)
                    } else {
                        stringResource(R.string.rental_block)
                    },
                )
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss, modifier = Modifier.testTag("rental-block-abort")) {
                Text(stringResource(R.string.rental_period_abort))
            }
        },
    )

    if (picking) {
        val picker = rememberDateRangePickerState()
        DatePickerDialog(
            onDismissRequest = { picking = false },
            confirmButton = {
                TextButton(
                    onClick = {
                        val chosen = periodOfPicked(
                            picker.selectedStartDateMillis,
                            picker.selectedEndDateMillis,
                        )
                        picking = false
                        chosen?.let { period = it }
                    },
                    modifier = Modifier.testTag("rental-block-period-ok"),
                ) { Text(stringResource(R.string.rental_period_ok)) }
            },
            dismissButton = {
                TextButton(onClick = { picking = false }) {
                    Text(stringResource(R.string.rental_period_abort))
                }
            },
        ) {
            DateRangePicker(picker, modifier = Modifier.testTag("rental-block-period-picker"))
        }
    }
}

/**
 * The rental area on its own, before anybody has signed in.
 *
 * Reading is public over there, so the area does not have to wait behind the
 * sign-in screen. Whoever opens the app can look through the devices and
 * search them; the sentence about the Rössing-ID appears where it becomes
 * relevant — at the booking — and not as a gate in front of everything.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PublicRentalScreen(
    state: RentalUiState,
    onBack: () -> Unit,
    onSignIn: () -> Unit,
    onQuery: (String) -> Unit = {},
    onRefresh: () -> Unit = {},
    onOpen: (RentalDevice) -> Unit = {},
    onClose: () -> Unit = {},
    onPeriod: (RentalPeriod) -> Unit = {},
) {
    BackHandler(onBack = onBack)
    Scaffold(
        topBar = {
            CenterAlignedTopAppBar(
                title = { Text(stringResource(R.string.rental_title)) },
                navigationIcon = {
                    IconButton(onClick = onBack, modifier = Modifier.testTag("rental-back")) {
                        Icon(
                            Icons.AutoMirrored.Filled.ArrowBack,
                            contentDescription = stringResource(R.string.rental_back_to_login),
                        )
                    }
                },
            )
        },
    ) { padding ->
        RentalScreen(
            state = state,
            modifier = Modifier.padding(padding),
            onQuery = onQuery,
            onRefresh = onRefresh,
            onOpen = onOpen,
            onClose = onClose,
            onPeriod = onPeriod,
            onSignIn = onSignIn,
            // Nobody is signed in here, so there is no stale token to fix.
            onSignInAgain = null,
        )
    }
}
