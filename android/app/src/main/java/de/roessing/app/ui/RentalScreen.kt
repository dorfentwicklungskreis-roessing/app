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
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material.icons.filled.EventBusy
import androidx.compose.material.icons.filled.Handyman
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.filled.Place
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.CenterAlignedTopAppBar
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.DateRangePicker
import androidx.compose.material3.ExperimentalMaterial3Api
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
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.data.Booking
import de.roessing.app.data.Device
import de.roessing.app.data.Period
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneOffset

/**
 * The area "Maschinchenring": what the village lends, and to whom.
 *
 * The order of the page follows what people come for. Searching sits at the
 * top, because whoever opens this area is usually after one particular thing.
 * Then whatever needs saying — no connection, sign in again, sign in at all.
 * Then my own bookings, because that is the second reason to look. Only then
 * the devices.
 *
 * Not a single decision is made here. "Free", "occupied", "cannot be
 * cancelled" — every one of those sentences comes from the rental platform;
 * this file only puts them on screen.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RentalScreen(
    state: RentalUiState,
    modifier: Modifier = Modifier,
    onQuery: (String) -> Unit = {},
    onRefresh: () -> Unit = {},
    onOpen: (Device) -> Unit = {},
    onClose: () -> Unit = {},
    onPeriod: (Period) -> Unit = {},
    onBook: (String) -> Unit = {},
    onCancelBooking: (String) -> Unit = {},
    /** Null when there is no way to sign in from here. */
    onSignIn: (() -> Unit)? = null,
    /** Null when a fresh sign-in cannot be started from here. */
    onSignInAgain: (() -> Unit)? = null,
) {
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
            Modifier.fillMaxWidth(),
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
private fun DeviceCard(device: Device, onClick: () -> Unit) {
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
            device.price?.let {
                Text(
                    it,
                    style = MaterialTheme.typography.labelLarge,
                    color = MaterialTheme.colorScheme.primary,
                )
            }
            device.owner?.let { Line(Icons.Filled.Person, stringResource(R.string.rental_owner, it)) }
            device.location?.let { Line(Icons.Filled.Place, it) }
            if (device.description.isNotBlank()) {
                Text(
                    device.description,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 3,
                    overflow = TextOverflow.Ellipsis,
                )
            }
            // Why something cannot be booked is the server's sentence, not ours.
            if (!device.bookable) {
                Text(
                    device.notBookableReason ?: stringResource(R.string.rental_not_bookable),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.testTag("device-blocked-${device.id}"),
                )
            }
        }
    }
}

@Composable
private fun BookingCard(booking: Booking, busy: Boolean, onCancel: () -> Unit) {
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
            // The wording of the state comes from the rental platform; the app
            // has no table that would translate `pending` into German.
            Text(booking.statusText, style = MaterialTheme.typography.labelLarge)
            booking.pickupAddress?.let {
                Text(
                    stringResource(R.string.rental_pickup, it),
                    style = MaterialTheme.typography.bodyMedium,
                )
            }
            if (booking.cancellable) {
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
 * when this screen has worked it out from the occupied stretches next to it.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun DeviceDetail(
    device: Device,
    state: RentalUiState,
    onPeriod: (Period) -> Unit,
    onBook: (String) -> Unit,
) {
    var picking by remember { mutableStateOf(false) }
    var message by rememberSaveable { mutableStateOf("") }

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
        device.price?.let {
            Text(it, style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.primary)
        }
        device.deposit?.let { Text(stringResource(R.string.rental_deposit, it)) }
        device.owner?.let { Line(Icons.Filled.Person, stringResource(R.string.rental_owner, it)) }
        device.location?.let { Line(Icons.Filled.Place, it) }
        if (device.description.isNotBlank()) {
            Text(
                device.description,
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
        if (!device.bookable) {
            Text(
                device.notBookableReason ?: stringResource(R.string.rental_not_bookable),
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.testTag("detail-blocked"),
            )
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
        }

        when {
            state.checking -> Row(verticalAlignment = Alignment.CenterVertically) {
                CircularProgressIndicator(Modifier.size(16.dp))
                Spacer(Modifier.width(8.dp))
                Text(stringResource(R.string.rental_checking))
            }

            state.availability?.free == true -> Text(
                state.availability.hint ?: stringResource(R.string.rental_free),
                color = MaterialTheme.colorScheme.primary,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("rental-free"),
            )

            state.availability != null -> Text(
                // "Occupied" is the server's answer, with its own wording
                // whenever it gave one.
                state.availability.hint ?: stringResource(R.string.rental_taken),
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("rental-taken"),
            )
        }

        state.availability?.occupied?.takeIf { it.isNotEmpty() }?.let { occupied ->
            Text(
                stringResource(R.string.rental_occupied_title),
                style = MaterialTheme.typography.titleSmall,
                modifier = Modifier.padding(top = 8.dp),
            )
            occupied.forEach { Line(Icons.Filled.EventBusy, it.text) }
        }

        OutlinedTextField(
            value = message,
            onValueChange = { message = it },
            label = { Text(stringResource(R.string.rental_message_label)) },
            minLines = 2,
            modifier = Modifier.fillMaxWidth().testTag("rental-message"),
        )

        state.message?.let {
            Text(
                it,
                color = MaterialTheme.colorScheme.error,
                style = MaterialTheme.typography.bodyMedium,
                modifier = Modifier.testTag("rental-refusal"),
            )
        }

        Button(
            onClick = { onBook(message) },
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
    }

    if (picking) {
        val picker = rememberDateRangePickerState()
        DatePickerDialog(
            onDismissRequest = { picking = false },
            confirmButton = {
                TextButton(
                    onClick = {
                        val period = periodOf(picker.selectedStartDateMillis, picker.selectedEndDateMillis)
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

/**
 * Turns the picker's two instants into a period.
 *
 * Material hands out UTC midnights; reading them back in UTC keeps the day
 * that was tapped. Picking a single day leaves the end empty — then start and
 * end are the same day, which is what people mean.
 */
internal fun periodOf(startMillis: Long?, endMillis: Long?): Period? {
    val from = startMillis?.let { day(it) } ?: return null
    return Period(from, endMillis?.let { day(it) } ?: from)
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

/** The icon of the area — used by the tile on the start page. */
internal val rentalIcon: ImageVector get() = Icons.Filled.Handyman

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
    onOpen: (Device) -> Unit = {},
    onClose: () -> Unit = {},
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
            onSignIn = onSignIn,
            // Nobody is signed in here, so there is no stale token to fix.
            onSignInAgain = null,
        )
    }
}
