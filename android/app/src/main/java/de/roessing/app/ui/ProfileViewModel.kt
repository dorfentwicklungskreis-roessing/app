package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import de.roessing.app.data.MemberDto
import de.roessing.app.data.ProfileDto
import de.roessing.app.data.ProfileInput
import de.roessing.app.data.ProfileRepository
import de.roessing.app.data.ProfileValidationException
import de.roessing.app.data.ProfileVisibilityDto
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * Zustand der Profilseite. Die Sichtbarkeits-Schalter stehen als Boolean
 * daneben: In der Oberfläche ist „an" gleichbedeutend mit „für alle
 * Dorfbewohner sichtbar".
 */
data class ProfileUiState(
    val loading: Boolean = true,
    val saving: Boolean = false,
    val displayName: String = "",
    val nickname: String = "",
    val phone: String = "",
    val email: String = "",
    val note: String = "",
    val displayNamePublic: Boolean = true,
    val nicknamePublic: Boolean = true,
    val phonePublic: Boolean = false,
    val emailPublic: Boolean = false,
    val notePublic: Boolean = false,
    /** Klartext-Begründung des Backends, wenn eine Eingabe abgelehnt wurde. */
    val error: String? = null,
    val members: List<MemberDto> = emptyList(),
    val membersLoading: Boolean = false,
    /** true, wenn die Mitgliederliste in der Verwaltungs-Sicht kam. */
    val adminView: Boolean = false,
) {
    /** Aufzählung dessen, was gerade für andere sichtbar ist — für den Hinweis. */
    val publicFields: List<String>
        get() = buildList {
            if (displayNamePublic && displayName.isNotBlank()) add("Anzeigename")
            if (nicknamePublic && nickname.isNotBlank()) add("Nickname")
            if (phonePublic && phone.isNotBlank()) add("Telefonnummer")
            if (emailPublic && email.isNotBlank()) add("E-Mail-Adresse")
            if (notePublic && note.isNotBlank()) add("Notiz")
        }
}

/** Einmalige Ereignisse der Profilseite. */
sealed interface ProfileEvent {
    data object Saved : ProfileEvent
}

class ProfileViewModel(private val repo: ProfileRepository) : ViewModel() {
    private val _state = MutableStateFlow(ProfileUiState())
    val state: StateFlow<ProfileUiState> = _state

    private val _events = MutableSharedFlow<ProfileEvent>(extraBufferCapacity = 4)
    val events: SharedFlow<ProfileEvent> = _events

    init {
        refresh()
    }

    fun refresh() {
        viewModelScope.launch {
            _state.update { it.copy(loading = true) }
            runCatching { repo.profile() }
                .onSuccess { p -> _state.update { it.uebernimm(p) } }
                .onFailure { _state.update { it.copy(loading = false, error = "Das Profil konnte nicht geladen werden.") } }
        }
    }

    fun loadMembers() {
        viewModelScope.launch {
            _state.update { it.copy(membersLoading = true) }
            runCatching { repo.members() }
                .onSuccess { (liste, adminSicht) ->
                    _state.update { it.copy(membersLoading = false, members = liste, adminView = adminSicht) }
                }
                .onFailure { _state.update { it.copy(membersLoading = false) } }
        }
    }

    fun setDisplayName(v: String) = _state.update { it.copy(displayName = v, error = null) }
    fun setNickname(v: String) = _state.update { it.copy(nickname = v, error = null) }
    fun setPhone(v: String) = _state.update { it.copy(phone = v, error = null) }
    fun setEmail(v: String) = _state.update { it.copy(email = v, error = null) }
    fun setNote(v: String) = _state.update { it.copy(note = v, error = null) }

    fun setDisplayNamePublic(v: Boolean) = _state.update { it.copy(displayNamePublic = v) }
    fun setNicknamePublic(v: Boolean) = _state.update { it.copy(nicknamePublic = v) }
    fun setPhonePublic(v: Boolean) = _state.update { it.copy(phonePublic = v) }
    fun setEmailPublic(v: Boolean) = _state.update { it.copy(emailPublic = v) }
    fun setNotePublic(v: Boolean) = _state.update { it.copy(notePublic = v) }

    fun save() {
        val s = _state.value
        viewModelScope.launch {
            _state.update { it.copy(saving = true, error = null) }
            runCatching { repo.saveProfile(s.alsEingabe()) }
                .onSuccess { p ->
                    _state.update { it.uebernimm(p).copy(saving = false) }
                    _events.emit(ProfileEvent.Saved)
                }
                .onFailure { fehler ->
                    // Die Begründung des Backends wird wörtlich übernommen —
                    // sie ist genauer als alles, was die App raten könnte.
                    val text = if (fehler is ProfileValidationException) fehler.grund
                    else "Speichern hat nicht geklappt. Besteht eine Verbindung?"
                    _state.update { it.copy(saving = false, error = text) }
                }
        }
    }
}

private fun ProfileUiState.uebernimm(p: ProfileDto) = copy(
    loading = false,
    error = null,
    displayName = p.displayName,
    nickname = p.nickname,
    phone = p.phone,
    email = p.email,
    note = p.note,
    displayNamePublic = p.visibility.displayNameIsPublic,
    nicknamePublic = p.visibility.nicknameIsPublic,
    phonePublic = p.visibility.phoneIsPublic,
    emailPublic = p.visibility.emailIsPublic,
    notePublic = p.visibility.noteIsPublic,
)

private fun ProfileUiState.alsEingabe() = ProfileInput(
    displayName = displayName.trim(),
    nickname = nickname.trim(),
    phone = phone.trim(),
    email = email.trim(),
    note = note.trim(),
    visibility = ProfileVisibilityDto(
        displayName = ProfileVisibilityDto.wert(displayNamePublic),
        nickname = ProfileVisibilityDto.wert(nicknamePublic),
        phone = ProfileVisibilityDto.wert(phonePublic),
        email = ProfileVisibilityDto.wert(emailPublic),
        note = ProfileVisibilityDto.wert(notePublic),
    ),
)
