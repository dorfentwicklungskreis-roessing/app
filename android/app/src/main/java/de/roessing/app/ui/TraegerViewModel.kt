package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import de.roessing.app.data.BeitrittDto
import de.roessing.app.data.MemberDto
import de.roessing.app.data.TraegerDto
import de.roessing.app.data.TraegerRefusedException
import de.roessing.app.data.TraegerRepository
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * The state of the Traeger area: which associations and working groups there
 * are, whether I belong to them, and what is currently running.
 *
 * Nothing here is re-decided. Whether joining is possible, whether the
 * deciding buttons appear at all, whether a Traeger shows up — all of that
 * arrives answered from the server (`model.Zugriff`). What this class does is
 * remember the answer and say what is currently being written.
 *
 * As everywhere in this app: **the last state stays put.** If the network
 * drops, the list is not emptied but gets a notice — "there are no
 * associations" would be a false statement in a dead spot.
 */
data class TraegerUiState(
    val all: List<TraegerDto> = emptyList(),
    /** My own requests across all Traeger. */
    val mine: List<BeitrittDto> = emptyList(),
    /** Requests per Traeger, for those who administer it. */
    val requests: Map<Long, List<BeitrittDto>> = emptyMap(),
    /** The village directory — the only place a person's identifier can come
     *  from when taking somebody in without a request of their own. */
    val villagers: List<MemberDto> = emptyList(),
    val loading: Boolean = false,
    /** Whether a fetch ever completed — before that the list shows a spinner
     *  instead of "no associations yet". */
    val everLoaded: Boolean = false,
    /** The last fetch failed, in the backend's own words. */
    val notice: String? = null,
    /** A genuinely refused write — shown as a dialog, in the server's words. */
    val error: String? = null,
    /** Traeger currently being written to. */
    val busy: Set<Long> = emptySet(),
    /** Requests currently being decided. */
    val busyRequests: Set<Long> = emptySet(),
    /** The Traeger whose detail page is open; null = the directory. */
    val open: Long? = null,
) {
    fun traeger(id: Long): TraegerDto? = all.find { it.id == id }

    val openTraeger: TraegerDto? get() = open?.let { traeger(it) }

    /**
     * Whether the server offered this Traeger to this person at all. The
     * place detail asks before it offers a way there — not as a second
     * visibility rule, but because the server's own directory is the only
     * honest answer to "is there anything to see behind this?".
     */
    fun inDirectory(id: Long): Boolean = id != 0L && traeger(id) != null

    /**
     * Associations: everything without a roof — plus anything whose roof this
     * person cannot see, because otherwise it would drop out of the directory
     * altogether.
     */
    val roots: List<TraegerDto>
        get() {
            val sichtbar = all.map { it.id }.toSet()
            return all
                .filter { it.parentId == 0L || it.parentId !in sichtbar }
                .sortedBy { it.name.lowercase() }
        }

    /** The working groups under an association. Exactly one level — deeper
     *  nesting does not exist (see `model.Traeger.ParentID`). */
    fun children(of: Long): List<TraegerDto> =
        all.filter { it.parentId == of }.sortedBy { it.name.lowercase() }

    /** How many undecided requests wait for my decision, across everything I
     *  administer. The start page shows this — somebody is waiting. */
    val openRequestsForMe: Int get() = all.sumOf { it.offeneBeitritte }

    /** My own requests that nobody has decided yet. */
    val myPending: List<BeitrittDto> get() = mine.filter { it.status == "beantragt" }

    fun openRequests(traegerId: Long): List<BeitrittDto> =
        requests[traegerId].orEmpty().filter { it.status == "beantragt" }
}

/** One-off events of the Traeger area. */
sealed interface TraegerEvent {
    data class Joined(val name: String) : TraegerEvent
    data class Granted(val name: String) : TraegerEvent
    data class Rejected(val name: String) : TraegerEvent
}

class TraegerViewModel(private val repo: TraegerRepository) : ViewModel() {
    private val _state = MutableStateFlow(TraegerUiState())
    val state: StateFlow<TraegerUiState> = _state

    private val _events = MutableSharedFlow<TraegerEvent>(extraBufferCapacity = 4)
    val events: SharedFlow<TraegerEvent> = _events

    fun load() {
        viewModelScope.launch {
            _state.update { it.copy(loading = true) }
            runCatching { repo.list() }
                .onSuccess { liste ->
                    _state.update {
                        it.copy(all = liste, loading = false, everLoaded = true, notice = null)
                    }
                }
                .onFailure { fehler ->
                    // The list stays. A dead spot is not an empty village.
                    _state.update {
                        it.copy(loading = false, everLoaded = true, notice = hinweis(fehler))
                    }
                }
            // The own requests are a second call and must not cost the list:
            // a directory without them is still useful.
            runCatching { repo.myRequests() }
                .onSuccess { eigene -> _state.update { it.copy(mine = eigene) } }
        }
    }

    /** Opens a Traeger's detail page and fetches what belongs on it. */
    fun open(id: Long) {
        _state.update { it.copy(open = id) }
        viewModelScope.launch {
            // The directory may be older than what is shown — the way here
            // can come from a place. A failure stays silent: the entry that
            // is there is not wrong, only possibly a minute old.
            runCatching { repo.detail(id) }.onSuccess { frisch ->
                _state.update { stand ->
                    val ohne = stand.all.filterNot { it.id == id }
                    stand.copy(all = ohne + frisch)
                }
            }
            if (_state.value.traeger(id)?.darfVerwalten == true) loadRequests(id)
        }
    }

    fun close() = _state.update { it.copy(open = null) }

    /** Fetches the requests of one Traeger. Only for those who administer it
     *  — the server answers everyone else with 403. */
    fun loadRequests(traegerId: Long) {
        viewModelScope.launch { fetchRequests(traegerId) }
    }

    private suspend fun fetchRequests(traegerId: Long) {
        runCatching { repo.requests(traegerId) }
            .onSuccess { liste ->
                _state.update { it.copy(requests = it.requests + (traegerId to liste)) }
            }
            .onFailure { fehler -> _state.update { it.copy(notice = hinweis(fehler)) } }
    }

    /** The village directory — needed to take somebody in directly. */
    fun loadVillagers() {
        if (_state.value.villagers.isNotEmpty()) return
        viewModelScope.launch {
            runCatching { repo.villagers() }
                .onSuccess { liste -> _state.update { it.copy(villagers = liste) } }
        }
    }

    /** "I want to join." */
    fun join(traegerId: Long, reason: String) {
        if (traegerId in _state.value.busy) return
        val name = _state.value.traeger(traegerId)?.name.orEmpty()
        viewModelScope.launch {
            _state.update { it.copy(busy = it.busy + traegerId, error = null) }
            val ergebnis = runCatching { repo.join(traegerId, reason.trim()) }
            _state.update { it.copy(busy = it.busy - traegerId) }
            ergebnis
                .onSuccess {
                    reload()
                    _events.emit(TraegerEvent.Joined(name))
                }
                // A 409 is not a mishap: the situation does not fit, and the
                // server says in which way. Either way the list is stale now.
                .onFailure { fehler ->
                    reload()
                    _state.update { it.copy(error = fehlertext(fehler)) }
                }
        }
    }

    /** Grants or rejects one request. */
    fun decide(request: BeitrittDto, status: String) {
        if (request.id in _state.value.busyRequests) return
        viewModelScope.launch {
            _state.update { it.copy(busyRequests = it.busyRequests + request.id, error = null) }
            val ergebnis = runCatching { repo.decide(request.id, status) }
            _state.update { it.copy(busyRequests = it.busyRequests - request.id) }
            fetchRequests(request.traegerId)
            ergebnis
                .onSuccess {
                    reload()
                    _events.emit(
                        if (status == "erteilt") {
                            TraegerEvent.Granted(request.anzeigename)
                        } else {
                            TraegerEvent.Rejected(request.anzeigename)
                        },
                    )
                }
                // Granting writes into the Rössing-ID first. Fails that, the
                // request stays open — and saying so is the whole point.
                .onFailure { fehler -> _state.update { it.copy(error = fehlertext(fehler)) } }
        }
    }

    /** Takes somebody in without a request of their own — the only way into a
     *  closed group. */
    fun addMember(traegerId: Long, person: MemberDto) {
        if (traegerId in _state.value.busy) return
        viewModelScope.launch {
            _state.update { it.copy(busy = it.busy + traegerId, error = null) }
            val ergebnis = runCatching { repo.addMember(traegerId, person.userSub) }
            _state.update { it.copy(busy = it.busy - traegerId) }
            fetchRequests(traegerId)
            ergebnis
                .onSuccess {
                    reload()
                    _events.emit(TraegerEvent.Granted(anzeigename(person)))
                }
                .onFailure { fehler -> _state.update { it.copy(error = fehlertext(fehler)) } }
        }
    }

    fun dismissError() = _state.update { it.copy(error = null) }

    fun dismissNotice() = _state.update { it.copy(notice = null) }

    private suspend fun reload() {
        runCatching { repo.list() }
            .onSuccess { liste -> _state.update { it.copy(all = liste, notice = null) } }
        runCatching { repo.myRequests() }
            .onSuccess { eigene -> _state.update { it.copy(mine = eigene) } }
    }

    companion object {
        /** The name a villager goes by — the resolved one before the bare
         *  identifier. */
        fun anzeigename(person: MemberDto): String =
            listOf(person.name, person.nickname, person.displayName)
                .firstOrNull { it.isNotBlank() } ?: person.userSub

        /**
         * The backend's reason is more precise than anything the app could
         * guess — it is taken over word for word. Only a connection that
         * never reached the server gets a sentence of our own.
         */
        fun fehlertext(fehler: Throwable): String = when (fehler) {
            is TraegerRefusedException -> fehler.grund
            else -> "Das hat nicht geklappt. Besteht eine Verbindung?"
        }

        fun hinweis(fehler: Throwable): String = when (fehler) {
            is TraegerRefusedException -> fehler.grund
            else -> "Keine Verbindung zum Server. Es werden ggf. alte Daten angezeigt."
        }
    }
}
