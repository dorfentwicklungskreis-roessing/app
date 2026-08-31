package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import de.roessing.app.auth.RentalSignIn
import de.roessing.app.data.BookingRequest
import de.roessing.app.data.RentalApiException
import de.roessing.app.data.RentalAvailability
import de.roessing.app.data.RentalBooking
import de.roessing.app.data.RentalDevice
import de.roessing.app.data.RentalErrorCode
import de.roessing.app.data.RentalImage
import de.roessing.app.data.RentalOccupancy
import de.roessing.app.data.RentalPeriod
import de.roessing.app.data.RentalProfile
import de.roessing.app.data.RentalRepository
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * A refusal of the rental platform, on its way to the screen.
 *
 * [text] is the German sentence the server wrote and may be shown as it
 * stands. [code] is what the app decides on; the screen falls back to its own
 * wording only when the server sent none.
 */
data class RentalNotice(val code: RentalErrorCode, val text: String?)

/**
 * State of the area "Maschinchenring".
 *
 * [offline] does not mean "nothing there", it means "not reachable right
 * now": if an older list is on screen it stays, and the hint appears above
 * it. An empty page without an explanation would be the worst outcome.
 *
 * [staleSignIn] is the one case that looks like a defect and is not one: the
 * phone is signed in, but with a token issued before the rental platform
 * joined the Rössing-ID. It cannot be repaired from here — only a fresh
 * sign-in produces a token with the new audience — so the app says so and
 * offers the button.
 */
data class RentalUiState(
    val query: String = "",
    val devices: List<RentalDevice> = emptyList(),
    val loading: Boolean = false,
    val offline: Boolean = false,
    /** Somebody is signed in and the token counts for the rental platform. */
    val signedIn: Boolean = false,
    /** Signed in, but with a token from before the changeover. */
    val staleSignIn: Boolean = false,
    val profile: RentalProfile? = null,
    val bookings: List<RentalBooking> = emptyList(),
    val bookingsOffline: Boolean = false,
    /** The device whose detail sheet is open, or null. */
    val selected: RentalDevice? = null,
    val images: List<RentalImage> = emptyList(),
    /** Taken periods of the open device, as the server drew them. */
    val occupancy: List<RentalOccupancy> = emptyList(),
    val period: RentalPeriod? = null,
    val availability: RentalAvailability? = null,
    val checking: Boolean = false,
    val booking: Boolean = false,
    val cancelling: Set<String> = emptySet(),
    /** The last refusal, worded by the server. */
    val notice: RentalNotice? = null,
    /**
     * What the server misses before it will take a booking. Empty is the
     * normal case — it takes name and telephone from the profile itself.
     */
    val missingFields: List<String> = emptyList(),
) {
    /** Nothing there, nothing on its way, nothing broken — then it is empty. */
    val empty: Boolean get() = devices.isEmpty() && !loading && !offline

    /**
     * Whether the booking button may be pressed.
     *
     * Every part of this comes from the server: it said the period is free.
     * The app adds only that a request is not already on its way.
     */
    val canBook: Boolean
        get() = signedIn &&
            period?.isValid == true &&
            availability?.available == true &&
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
 * Nothing here decides anything about renting. Whether a period is free,
 * whether a booking may be cancelled — all of that arrives from the server,
 * and a button is greyed out because the server said so, never because this
 * class worked it out.
 *
 * @param repo the rental platform
 * @param signIn how this device's sign-in relates to it
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
     * Searching happens on the server: its hybrid ranking knows the devices
     * and their descriptions, and a local `contains` would quietly disagree
     * with the website. Only the waiting is ours, so that a five-letter word
     * is not five requests.
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
                    // The old list stays; the hint goes above it. Whether the
                    // platform answered 500 or the connection broke makes no
                    // difference here — either way it is not reachable.
                    _state.update { it.copy(loading = false, offline = true) }
                }
        }
    }

    /** Looks at the sign-in and, if it counts, fetches profile and bookings. */
    private suspend fun refreshSignIn() {
        val signedIn = runCatching { signIn() }.getOrDefault(RentalSignIn.UNREACHABLE)
        _state.update {
            it.copy(
                signedIn = signedIn == RentalSignIn.VALID,
                staleSignIn = signedIn == RentalSignIn.STALE,
                // Nobody signed in means no bookings to show — and none to
                // keep on screen either.
                bookings = if (signedIn == RentalSignIn.VALID) it.bookings else emptyList(),
                profile = if (signedIn == RentalSignIn.VALID) it.profile else null,
            )
        }
        if (signedIn == RentalSignIn.VALID) {
            loadProfile()
            loadBookings()
        }
    }

    /**
     * The own account over there.
     *
     * Worth its own call even though the booking works without it: the first
     * request creates the account in the rental platform, and its answer is
     * the cheapest place to notice a token that does not count over there.
     */
    fun loadProfile() {
        viewModelScope.launch {
            runCatching { repo.profile() }
                .onSuccess { profile -> _state.update { it.copy(profile = profile) } }
                .onFailure { failure ->
                    _state.update { state ->
                        when (failure.rentalCode()) {
                            RentalErrorCode.TOKEN_AUDIENCE -> state.asStale()
                            RentalErrorCode.UNAUTHORIZED -> state.asSignedOut()
                            // Not being able to read the profile is no reason
                            // to hide the area — booking may still work.
                            else -> state
                        }
                    }
                }
        }
    }

    /** My bookings. Only makes sense signed in; [load] takes care of that. */
    fun loadBookings() {
        viewModelScope.launch {
            runCatching { repo.myBookings() }
                .onSuccess { list ->
                    _state.update { it.copy(bookings = list, bookingsOffline = false) }
                }
                .onFailure { failure ->
                    _state.update { state ->
                        when (failure.rentalCode()) {
                            RentalErrorCode.TOKEN_AUDIENCE -> state.asStale()
                            RentalErrorCode.UNAUTHORIZED -> state.asSignedOut()
                            else -> state.copy(bookingsOffline = true)
                        }
                    }
                }
        }
    }

    // --- Detail sheet --------------------------------------------------------

    /**
     * Opens a device.
     *
     * The pictures and the taken periods are fetched afterwards; the sheet is
     * on screen immediately with what the list already knows. Neither of the
     * two is allowed to keep it shut.
     */
    fun open(device: RentalDevice) {
        _state.update {
            it.copy(
                selected = device,
                images = emptyList(),
                occupancy = emptyList(),
                availability = null,
                period = null,
                notice = null,
                missingFields = emptyList(),
            )
        }
        viewModelScope.launch {
            runCatching { repo.device(device.id) }.onSuccess { detail ->
                // Only if the same device is still open — somebody may have
                // closed the sheet and opened the next one in the meantime.
                _state.update { state ->
                    if (state.selected?.id == device.id) {
                        state.copy(selected = detail.device, images = detail.images)
                    } else {
                        state
                    }
                }
            }
        }
        loadOccupancy(device.id)
    }

    /** The taken periods of one device — public, so it needs no sign-in. */
    fun loadOccupancy(deviceId: String) {
        viewModelScope.launch {
            runCatching { repo.occupancy(deviceId) }.onSuccess { periods ->
                _state.update { state ->
                    if (state.selected?.id == deviceId) state.copy(occupancy = periods) else state
                }
            }
        }
    }

    fun close() = _state.update {
        it.copy(
            selected = null,
            images = emptyList(),
            occupancy = emptyList(),
            availability = null,
            period = null,
            notice = null,
            missingFields = emptyList(),
        )
    }

    /**
     * A chosen period. Whether it is free is asked at once, because that is
     * the question people came with — and it is the server's answer, not one
     * the app reads out of the occupancy it has just drawn.
     */
    fun setPeriod(period: RentalPeriod) {
        _state.update { it.copy(period = period, availability = null, notice = null) }
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
                        it.copy(checking = false, availability = null).after(failure)
                    }
                }
        }
    }

    /**
     * Sends the request. The owner decides afterwards.
     *
     * The three personal fields stay empty in the ordinary case: the server
     * takes name and telephone from the profile. They are passed only after
     * the server has said it misses them.
     */
    fun book(
        notes: String? = null,
        firstName: String? = null,
        lastName: String? = null,
        phone: String? = null,
    ) {
        val current = _state.value
        val device = current.selected ?: return
        val period = current.period ?: return
        if (!current.canBook) return
        // Der Vermerk „läuft" muss stehen, bevor der Ablauf zurückkehrt —
        // sonst kommt ein zweiter Tipp am selben Prüfpunkt vorbei.
        _state.update { it.copy(booking = true, notice = null) }
        viewModelScope.launch {
            runCatching {
                repo.book(
                    BookingRequest(
                        deviceId = device.id,
                        period = period,
                        notes = notes,
                        firstName = firstName,
                        lastName = lastName,
                        phone = phone,
                    ),
                )
            }
                .onSuccess {
                    _state.update {
                        it.copy(
                            booking = false,
                            selected = null,
                            images = emptyList(),
                            occupancy = emptyList(),
                            availability = null,
                            period = null,
                            missingFields = emptyList(),
                        )
                    }
                    _events.emit(RentalEvent.Booked)
                    loadBookings()
                }
                .onFailure { failure ->
                    _state.update { it.copy(booking = false).after(failure) }
                    // A period taken in the meantime is the ordinary race
                    // between drawing the calendar and tapping the button:
                    // fetch it again instead of leaving a stale picture.
                    if (failure.rentalCode() == RentalErrorCode.OCCUPIED) {
                        loadOccupancy(device.id)
                    }
                }
        }
    }

    /** Withdraws one of my bookings — if the server allows it. */
    fun cancel(bookingId: String) {
        if (bookingId in _state.value.cancelling) return
        // Wie beim Anfragen: erst vermerken, dann losschicken.
        _state.update { it.copy(cancelling = it.cancelling + bookingId, notice = null) }
        viewModelScope.launch {
            runCatching { repo.cancel(bookingId) }
                .onSuccess {
                    _state.update { it.copy(cancelling = it.cancelling - bookingId) }
                    _events.emit(RentalEvent.Cancelled)
                    loadBookings()
                }
                .onFailure { failure ->
                    _state.update {
                        it.copy(cancelling = it.cancelling - bookingId).after(failure)
                    }
                    // "Already cancelled" and friends: the list over there
                    // knows better than the one on screen.
                    loadBookings()
                }
        }
    }

    /** Clears a refusal once it has been read. */
    fun clearNotice() = _state.update { it.copy(notice = null) }

    /**
     * One failure, three kinds of consequence.
     *
     * A refusal of the platform keeps its wording and is shown. A token that
     * does not count over there turns into the request to sign in again.
     * Everything else is the network, and for that the area has its hint.
     */
    private fun RentalUiState.after(failure: Throwable): RentalUiState {
        val error = failure as? RentalApiException
            ?: return copy(offline = true)
        return when (error.code) {
            RentalErrorCode.TOKEN_AUDIENCE -> asStale()
            RentalErrorCode.UNAUTHORIZED -> asSignedOut()
            RentalErrorCode.PROFILE_INCOMPLETE -> copy(
                notice = RentalNotice(error.code, error.message),
                // What the server misses is what the sheet asks for. The app
                // has no list of its own.
                missingFields = error.missingFields,
            )

            else -> copy(notice = RentalNotice(error.code, error.message))
        }
    }
}

/** Signed in, but with a token from before the changeover. */
private fun RentalUiState.asStale() =
    copy(staleSignIn = true, signedIn = false, bookings = emptyList(), profile = null)

/** The sign-in is gone — the ordinary way back is the login screen. */
private fun RentalUiState.asSignedOut() =
    copy(signedIn = false, staleSignIn = false, bookings = emptyList(), profile = null)

/** The platform's code, or null when the failure was not one of its answers. */
private fun Throwable.rentalCode(): RentalErrorCode? = (this as? RentalApiException)?.code

/** Long enough that a typed word is one request, short enough to feel live. */
const val SEARCH_DELAY = 300L
