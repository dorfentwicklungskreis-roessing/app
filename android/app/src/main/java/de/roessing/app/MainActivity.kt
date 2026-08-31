package de.roessing.app

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.lifecycle.viewmodel.compose.viewModel
import de.roessing.app.auth.LoginResult
import de.roessing.app.auth.SessionState
import de.roessing.app.ui.ChatViewModel
import de.roessing.app.ui.HomeScreen
import de.roessing.app.ui.IdeenViewModel
import de.roessing.app.ui.LeaderboardViewModel
import de.roessing.app.ui.LoginScreen
import de.roessing.app.ui.PlacesViewModel
import de.roessing.app.ui.ProfileViewModel
import de.roessing.app.ui.PublicRentalScreen
import de.roessing.app.ui.RentalViewModel
import de.roessing.app.ui.VeranstaltungenViewModel
import de.roessing.app.push.PushZiel
import de.roessing.app.ui.theme.DorfAppTheme
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    /**
     * Das Ziel eines angetippten Push-Hinweises. Steht als einfache Extras im
     * Intent — dieselben Felder, die auch von Firebase kamen. Als State,
     * damit ein Tipp bei laufender App (onNewIntent) sofort ankommt.
     */
    private val pushZiel = mutableStateOf<PushZiel?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        pushZiel.value = zielAus(intent)
        setContent {
            DorfAppTheme {
                Root(pushZiel.value) { pushZiel.value = null }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        zielAus(intent)?.let { pushZiel.value = it }
    }

    /** Liest das Sprungziel aus den Extras (alles Zeichenketten, siehe PushZiel). */
    private fun zielAus(intent: Intent?): PushZiel? {
        val extras = intent?.extras ?: return null
        val daten = extras.keySet().mapNotNull { schluessel ->
            (extras.getString(schluessel))?.let { schluessel to it }
        }.toMap()
        return PushZiel.ausDaten(daten)
    }
}

@Composable
private fun Root(pushZiel: PushZiel? = null, onPushZielVerbraucht: () -> Unit = {}) {
    val context = androidx.compose.ui.platform.LocalContext.current
    val container = context.appContainer
    val session by container.authManager.session.collectAsState()
    val scope = rememberCoroutineScope()
    // null = kein Fehler. Ein Abbruch (Zurück-Taste) ist bewusst kein Fehler.
    var loginError by remember { mutableStateOf<String?>(null) }
    // Der Maschinchenring ist der einzige Bereich, den man ohne Anmeldung
    // ansehen kann — seine Geräteliste ist auch im Web öffentlich.
    var verleihOhneAnmeldung by remember { mutableStateOf(false) }

    val authLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.StartActivityForResult(),
    ) { result ->
        scope.launch {
            loginError = when (val r = container.authManager.handleAuthResult(result.data)) {
                is LoginResult.Success, is LoginResult.Cancelled -> null
                is LoginResult.Failed -> r.code
            }
        }
    }

    // Die Anmeldung anstoßen. Sie steht hier oben, weil sie an zwei Stellen
    // gebraucht wird: auf dem Anmeldeschirm und im Maschinchenring, wo ein
    // Token von vor der Umstellung eine *erneute* Anmeldung verlangt, ohne
    // die bestehende vorher wegzuwerfen.
    val anmelden = {
        loginError = null
        scope.launch {
            runCatching { authLauncher.launch(container.authManager.buildLoginIntent()) }
                .onFailure { loginError = it::class.java.simpleName }
        }
        Unit
    }

    when (session) {
        is SessionState.Loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator()
        }

        is SessionState.LoggedOut -> {
            if (verleihOhneAnmeldung) {
                val verleihVm: RentalViewModel = viewModel(factory = viewModelFactory(container))
                val verleih by verleihVm.state.collectAsState()
                LaunchedEffect(Unit) { verleihVm.load() }
                PublicRentalScreen(
                    state = verleih,
                    onBack = { verleihOhneAnmeldung = false },
                    onSignIn = {
                        verleihOhneAnmeldung = false
                        anmelden()
                    },
                    onQuery = verleihVm::setQuery,
                    onRefresh = verleihVm::refresh,
                    onOpen = verleihVm::open,
                    onClose = verleihVm::close,
                    onPeriod = verleihVm::setPeriod,
                )
            } else {
                LoginScreen(
                    errorCode = loginError,
                    onLogin = anmelden,
                    onDevLogin = { asAdmin ->
                        scope.launch { container.authManager.devLogin(asAdmin) }
                    },
                    onBrowseRental = { verleihOhneAnmeldung = true },
                )
            }
        }

        is SessionState.LoggedIn -> {
            val factory = viewModelFactory(container)
            val vm: PlacesViewModel = viewModel(factory = factory)
            val rangVm: LeaderboardViewModel = viewModel(factory = factory)
            val profilVm: ProfileViewModel = viewModel(factory = factory)
            val ideenVm: IdeenViewModel = viewModel(factory = factory)
            val chatVm: ChatViewModel = viewModel(factory = factory)
            val termineVm: VeranstaltungenViewModel = viewModel(factory = factory)
            val verleihVm: RentalViewModel = viewModel(factory = factory)
            HomeScreen(
                viewModel = vm,
                leaderboardViewModel = rangVm,
                profileViewModel = profilVm,
                ideenViewModel = ideenVm,
                chatViewModel = chatVm,
                veranstaltungenViewModel = termineVm,
                rentalViewModel = verleihVm,
                pushZiel = pushZiel,
                onPushZielVerbraucht = onPushZielVerbraucht,
                onLogout = {
                    scope.launch {
                        // Erst das Gerät abmelden, dann die Sitzung beenden —
                        // danach fehlt das Token für den Aufruf.
                        de.roessing.app.push.Geraeteanmeldung.abmelden(context)
                        container.authManager.logout()
                    }
                },
                // Neu anmelden, ohne die bestehende Anmeldung vorher
                // wegzuwerfen: Bricht jemand im Browser ab, bleibt alles, wie
                // es war.
                onReauthenticate = anmelden,
            )
        }
    }
}

private fun viewModelFactory(container: AppContainer) =
    object : androidx.lifecycle.ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : androidx.lifecycle.ViewModel> create(modelClass: Class<T>): T = when {
            modelClass.isAssignableFrom(PlacesViewModel::class.java) ->
                PlacesViewModel(container.repository, container.vergabeRepository) as T

            modelClass.isAssignableFrom(LeaderboardViewModel::class.java) ->
                LeaderboardViewModel(container.statsRepository) as T

            modelClass.isAssignableFrom(ProfileViewModel::class.java) ->
                ProfileViewModel(container.profileRepository) as T

            modelClass.isAssignableFrom(IdeenViewModel::class.java) ->
                IdeenViewModel(container.ideenRepository) as T

            modelClass.isAssignableFrom(ChatViewModel::class.java) ->
                ChatViewModel(container.chatRepository) as T

            modelClass.isAssignableFrom(VeranstaltungenViewModel::class.java) ->
                VeranstaltungenViewModel(container.veranstaltungenRepository) as T

            modelClass.isAssignableFrom(RentalViewModel::class.java) ->
                RentalViewModel(
                    container.rentalRepository,
                    signIn = { container.rentalSignIn() },
                ) as T

            else -> error("Unbekanntes ViewModel: ${modelClass.name}")
        }
    }
