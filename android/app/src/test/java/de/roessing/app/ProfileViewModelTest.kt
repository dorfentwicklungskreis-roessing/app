package de.roessing.app

import de.roessing.app.data.MemberDto
import de.roessing.app.data.ProfileDto
import de.roessing.app.data.ProfileInput
import de.roessing.app.data.ProfileRepository
import de.roessing.app.data.ProfileValidationException
import de.roessing.app.data.ProfileVisibilityDto
import de.roessing.app.ui.ProfileEvent
import de.roessing.app.ui.ProfileViewModel
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.resetMain
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.test.setMain
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/**
 * Fake des Profil-Zugriffs. Verhält sich wie das Backend: Er merkt sich, was
 * gespeichert wurde, und kann eine Ablehnung (HTTP 400) nachstellen.
 */
private class FakeProfileRepo : ProfileRepository {
    var profil = ProfileDto(
        userSub = "erna-sub",
        displayName = "Erna Beispiel",
        email = "erna@example.org",
        visibility = ProfileVisibilityDto(),
    )
    var mitglieder = listOf(
        MemberDto(userSub = "karl-sub", name = "Karl", phone = "05066 1234", email = "karl@example.org"),
        MemberDto(userSub = "erna-sub", name = "Erna Beispiel"),
    )
    var adminSicht = false
    var fehler: Throwable? = null
    var ablehnung: String? = null
    val gespeichert = mutableListOf<ProfileInput>()

    override suspend fun profile(): ProfileDto {
        fehler?.let { throw it }
        return profil
    }

    override suspend fun saveProfile(input: ProfileInput): ProfileDto {
        ablehnung?.let { throw ProfileValidationException(it) }
        fehler?.let { throw it }
        gespeichert += input
        profil = profil.copy(
            displayName = input.displayName, nickname = input.nickname,
            phone = input.phone, email = input.email, note = input.note,
            visibility = input.visibility,
        )
        return profil
    }

    override suspend fun members(): Pair<List<MemberDto>, Boolean> {
        fehler?.let { throw it }
        return mitglieder to adminSicht
    }
}

@OptIn(ExperimentalCoroutinesApi::class)
class ProfileViewModelTest {
    private val dispatcher = StandardTestDispatcher()

    @Before
    fun setup() = Dispatchers.setMain(dispatcher)

    @After
    fun teardown() = Dispatchers.resetMain()

    @Test
    fun `laedt das eigene Profil beim Start`() = runTest(dispatcher) {
        val vm = ProfileViewModel(FakeProfileRepo())
        dispatcher.scheduler.advanceUntilIdle()

        val state = vm.state.value
        assertFalse(state.loading)
        assertEquals("Erna Beispiel", state.displayName)
        assertEquals("erna@example.org", state.email)
    }

    // Die Vorbelegung ist die entscheidende Zusage: Kontaktdaten sind nicht
    // veröffentlicht, bevor jemand den Schalter bewusst umlegt.
    @Test
    fun `Kontaktdaten sind in der Vorbelegung nicht oeffentlich`() = runTest(dispatcher) {
        val vm = ProfileViewModel(FakeProfileRepo())
        dispatcher.scheduler.advanceUntilIdle()

        val state = vm.state.value
        assertTrue("Anzeigename sollte sichtbar sein", state.displayNamePublic)
        assertTrue("Nickname sollte sichtbar sein", state.nicknamePublic)
        assertFalse("Telefon darf nicht sichtbar sein", state.phonePublic)
        assertFalse("E-Mail darf nicht sichtbar sein", state.emailPublic)
        assertFalse("Notiz darf nicht sichtbar sein", state.notePublic)
    }

    @Test
    fun `speichert Aenderungen samt Sichtbarkeit`() = runTest(dispatcher) {
        val repo = FakeProfileRepo()
        val vm = ProfileViewModel(repo)
        dispatcher.scheduler.advanceUntilIdle()

        vm.setNickname("Gießmeisterin")
        vm.setPhone("05066 123456")
        vm.setPhonePublic(true)
        vm.save()
        dispatcher.scheduler.advanceUntilIdle()

        assertEquals(1, repo.gespeichert.size)
        val gesendet = repo.gespeichert.single()
        assertEquals("Gießmeisterin", gesendet.nickname)
        assertEquals("05066 123456", gesendet.phone)
        assertEquals("dorf", gesendet.visibility.phone)
        // Nicht umgelegte Schalter bleiben bei der Verwaltung.
        assertEquals("verwaltung", gesendet.visibility.email)
        assertFalse(vm.state.value.saving)
    }

    @Test
    fun `meldet Erfolg als einmaliges Ereignis`() = runTest(dispatcher) {
        val vm = ProfileViewModel(FakeProfileRepo())
        dispatcher.scheduler.advanceUntilIdle()

        val ereignisse = mutableListOf<ProfileEvent>()
        val job = launch { vm.events.collect { ereignisse += it } }
        vm.setNickname("Neu")
        vm.save()
        dispatcher.scheduler.advanceUntilIdle()

        assertEquals(listOf(ProfileEvent.Saved), ereignisse)
        job.cancel()
    }

    // Das Backend prüft, nicht die App: Seine Begründung muss die Person
    // wörtlich zu sehen bekommen.
    @Test
    fun `zeigt die Begruendung des Backends bei abgelehnten Eingaben`() = runTest(dispatcher) {
        val repo = FakeProfileRepo().apply { ablehnung = "email ist keine gültige E-Mail-Adresse" }
        val vm = ProfileViewModel(repo)
        dispatcher.scheduler.advanceUntilIdle()

        vm.setEmail("keine-adresse")
        vm.save()
        dispatcher.scheduler.advanceUntilIdle()

        assertEquals("email ist keine gültige E-Mail-Adresse", vm.state.value.error)
        assertFalse(vm.state.value.saving)
        assertTrue("nichts darf gespeichert worden sein", repo.gespeichert.isEmpty())
    }

    @Test
    fun `netzfehler beim Speichern setzt eine Fehlermeldung`() = runTest(dispatcher) {
        val repo = FakeProfileRepo()
        val vm = ProfileViewModel(repo)
        dispatcher.scheduler.advanceUntilIdle()
        repo.fehler = RuntimeException("offline")

        vm.save()
        dispatcher.scheduler.advanceUntilIdle()

        assertTrue(vm.state.value.error?.isNotEmpty() == true)
    }

    @Test
    fun `laedt die Dorfbewohner`() = runTest(dispatcher) {
        val repo = FakeProfileRepo()
        val vm = ProfileViewModel(repo)
        vm.loadMembers()
        dispatcher.scheduler.advanceUntilIdle()

        assertEquals(listOf("Karl", "Erna Beispiel"), vm.state.value.members.map { it.name })
        assertFalse(vm.state.value.adminView)
    }

    @Test
    fun `merkt sich die Verwaltungs-Sicht`() = runTest(dispatcher) {
        val repo = FakeProfileRepo().apply { adminSicht = true }
        val vm = ProfileViewModel(repo)
        vm.loadMembers()
        dispatcher.scheduler.advanceUntilIdle()

        assertTrue(vm.state.value.adminView)
    }
}

/** Kleine Probe der DTO-Umwandlung — sie trägt die Sichtbarkeitsregel. */
class ProfileVisibilityTest {
    @Test
    fun `Vorbelegung stellt Kontaktdaten auf verwaltung`() {
        val v = ProfileVisibilityDto()
        assertEquals("dorf", v.displayName)
        assertEquals("dorf", v.nickname)
        assertEquals("verwaltung", v.phone)
        assertEquals("verwaltung", v.email)
        assertEquals("verwaltung", v.note)
    }

    @Test
    fun `unbekannte Werte gelten als nicht oeffentlich`() {
        // Ein Wert, den diese App-Version nicht kennt, darf nie als
        // „öffentlich" durchgehen.
        assertFalse(ProfileVisibilityDto(phone = "irgendwas").phoneIsPublic)
        assertTrue(ProfileVisibilityDto(phone = "dorf").phoneIsPublic)
    }
}
