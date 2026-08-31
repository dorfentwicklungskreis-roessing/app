package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import de.roessing.app.auth.RentalSignIn
import de.roessing.app.data.Availability
import de.roessing.app.data.Booking
import de.roessing.app.data.BookingRequest
import de.roessing.app.data.Device
import de.roessing.app.data.NotSignedInException
import de.roessing.app.data.Period
import de.roessing.app.data.RentalRejectedException
import de.roessing.app.data.RentalRepository
import de.roessing.app.data.StaleAudienceException
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * State of the area "Maschinchenring".
 *
 * [offline] does not mean "nothing there", it means "not reachable right
 * now": if an older list is on screen it stays, and the hint appears above
 * it. An empty page without an explanation would be the worst outcome.
 *
 * [staleSignIn] is the one case that looks like a defect and is not one: the
 * phone is signed in, but with a token that was issued before the rental
 * platform joined the Rössing-ID. It cannot be repaired from here — only a
 * fresh sign-in produces a token with the new audience — so the app says so
 * and offers the button.
 */
data class RentalUiState(
    val query: String = "",
    val devices: List<Device> = emptyList(),
    val loading: Boolean = false,
    val offline: Boolean = false,
    /** Somebody is signed in and the token counts for the rental platform. */
    val signedIn: Boolean = false,
    /** Signed in, but with a token from before the changeover. */
    val staleSignIn: Boolean = false,
    val bookings: List<Booking> = emptyList(),
    val bookingsOffline: Boolean = false,
    /** The device whose detail sheet is open, or null. */
    val selected: Device? = null,
    val period: Period? = null,
    val availability: Availability? = null,
    val checking: Boolean = false,
    val booking: Boolean = false,
    val cancelling: Set<String> = emptySet(),
    /** Plain German refusal from the server, shown as it was worded. */
    val message: String? = null,
) {
    /** Nothing there, nothing on its way, nothing broken — then it is empty. */
    val empty: Boolean get() = devices.isEmpty() && !loading && !offline

    /** Whether the booking button may be pressed at all. */
    val canBook: Boolean
        get() = selected?.bookable == true &&
            period?.isSane == true &&
            availability?.free == true &&
            !booking
}

/** One-off events of the area. */
sealed interface RentalEvent {
    /** The request went out; the owner decides. */
    data object Booked : RentalEvent

    data object Cancelled : RentalEvent
}

/**
 * Talks to `mieten.xn--rssing-wxa.de` through [RentalRepository].
 *
 * Nothing here decides anything about renting. Whether a device can be
 * booked, whether a weekend is free, whether a booking may be cancelled — all
 * of that arrives from the server, and the button is greyed out because the
 * server said so, never because this class worked it out.
 *
 * @param signIn how this device's sign-in relates to the rental platform
 * @param searchDelay how long typing settles before the server is asked
 */
class RentalViewModel(
    private val repo: RentalRepository,
    private val signIn: suspend () -> RentalSignIn = { RentalSignIn.MISSING },
    private val searchDelay: Long = SEARCH_DELAY,
) : ViewModel() {
    private val _state = MutableStateFlow(RentalUiState())
    val state: StateFlow<RentalUiState> = _state

    private val _events = MutableSharedFlow<RentalEvent>(extraBufferCapacity = 4)
    val events: SharedFlow<RentalEvent> = _events

    private var loaded = false
    private var searchJob: Job? = null
    private var checkJob: Job? = null

    /**
     * On opening the area. What is already there is not fetched again; the
     * sign-in, on the other hand, is looked at every time — somebody may have
     * signed in on the login screen in between.
     */
    fun load() {
        viewModelScope.launch { refreshSignIn() }
        if (loaded) return
        fetch(_state.value.query)
    }

    /** Deliberate refresh by the user. */
    fun refresh() {
        viewModelScope.launch { refreshSignIn() }
        fetch(_state.value.query)
    }

    /**
     * Typing in the search field.
     *
     * Searching happens on the server — the ranking over there knows the
     * devices, their sets and their descriptions; a local `contains` would
     * quietly disagree with the website. Only the waiting is ours, so that a
     * five-letter word is not five requests.
     */
    fun setQuery(text: String) {
        _state.update { it.copy(query = text) }
        searchJob?.cancel()
        searchJob = viewModelScope.launch {
            delay(searchDelay)
            fetch(text)
        }
    }

    private fun fetch(query: String) {
        viewModelScope.launch {
            _state.update { it.copy(loading = true) }
            runCatching {
                if (query.isBlank()) repo.devices() else repo.search(query.trim())
            }
                .onSuccess { list ->
                    loaded = true
                    _state.update { it.copy(loading = false, offline = false, devices = list) }
                }
                .onFailure {
                    // The old list stays; the hint goes above it.
                    _state.update { it.copy(loading = false, offline = true) }
                }
        }
    }

    /** Looks at the sign-in and, if it counts, fetches the own bookings. */
    private suspend fun refreshSignIn() {
        val state = runCatching { signIn() }.getOrDefault(RentalSignIn.UNREACHABLE)
        _state.update {
            it.copy(
                signedIn = state == RentalSignIn.VALID,
                staleSignIn = state == RentalSignIn.STALE,
                // Nobody signed in means no bookings to show — and none to
                // keep on screen either.
                bookings = if (state == RentalSignIn.VALID) it.bookings else emptyList(),
            )
        }
        if (state == RentalSignIn.VALID) loadBookings()
    }

    /** My bookings. Only makes sense signed in; [load] takes care of that. */
    fun loadBookings() {
        viewModelScope.launch {
            runCatching { repo.myBookings() }
                .onSuccess { list ->
                    _state.update { it.copy(bookings = list, bookingsOffline = false) }
                }
                .onFailure { failure ->
                    _state.update {
                        when (failure) {
                            is StaleAudienceException ->
                                it.copy(staleSignIn = true, signedIn = false, bookings = emptyList())

                            is NotSignedInException ->
                                it.copy(signedIn = false, bookings = emptyList())

                            else -> it.copy(bookingsOffline = true)
                        }
                    }
                }
        }
    }

    // --- Detail sheet --------------------------------------------------------

    fun open(device: Device) = _state.update {
        it.copy(selected = device, availability = null, message = null)
    }

    fun close() = _state.update {
        it.copy(selected = null, availability = null, message = null, period = null)
    }

    /**
     * A chosen period. The answer whether it is free comes from the server —
     * asked right away, because that is the question people came with.
     */
    fun setPeriod(period: Period) {
        _state.update { it.copy(period = period, availability = null, message = null) }
        val device = _state.value.selected ?: return
        checkJob?.cancel()
        checkJob = viewModelScope.launch {
            _state.update { it.copy(checking = true) }
            runCatching { repo.availability(device.id, period) }
                .onSuccess { answer ->
                    _state.update { it.copy(checking = false, availability = answer) }
                }
                .onFailure { failure ->
                    _state.update {
                        it.copy(
                            checking = false,
                            availability = null,
                            message = (failure as? RentalRejectedException)?.reason,
                            offline = failure !is RentalRejectedException,
                        )
                    }
                }
        }
    }

    /** Sends the request. The owner decides — that is what the wording says. */
    fun book(message: String = "") {
        val current = _state.value
        val device = current.selected ?: return
        val period = current.period ?: return
        if (!current.canBook) return
        viewModelScope.launch {
            _state.update { it.copy(booking = true, message = null) }
            runCatching { repo.book(BookingRequest(device.id, period, message)) }
                .onSuccess {
                    _state.update {
                        it.copy(booking = false, selected = null, availability = null, period = null)
                    }
                    _events.emit(RentalEvent.Booked)
                    loadBookings()
                }
                .onFailure { failure -> _state.update { it.afterFailure(failure) } }
        }
    }

    /** Cancels one of my bookings — if the server allows it. */
    fun cancel(bookingId: String) {
        if (bookingId in _state.value.cancelling) return
        viewModelScope.launch {
            _state.update { it.copy(cancelling = it.cancelling + bookingId, message = null) }
            runCatching { repo.cancel(bookingId) }
                .onSuccess {
                    _state.update { it.copy(cancelling = it.cancelling - bookingId) }
                    _events.emit(RentalEvent.Cancelled)
                    loadBookings()
                }
                .onFailure { failure ->
                    _state.update {
                        it.afterFailure(failure).copy(cancelling = it.cancelling - bookingId)
                    }
                }
        }
    }

    /** Clears a refusal once it has been read. */
    fun clearMessage() = _state.update { it.copy(message = null) }

    /**
     * One failure, three different sentences.
     *
     * A refusal is the server's wording and is shown verbatim. A stale token
     * is a request to sign in again. Everything else is the network, and for
     * that the area already has its hint.
     */
    private fun RentalUiState.afterFailure(failure: Throwable): RentalUiState = when (failure) {
        is RentalRejectedException ->
            copy(booking = false, message = failure.reason)

        is StaleAudienceException ->
            copy(booking = false, staleSignIn = true, signedIn = false, bookings = emptyList())

        is NotSignedInException ->
            copy(booking = false, signedIn = false, bookings = emptyList())

        else -> copy(booking = false, offline = true)
    }

    companion object {
        /** Long enough that a typed word is one request, short enough to feel live. */
        const val SEARCH_DELAY = 300L
    }
}
