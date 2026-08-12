package de.roessing.app

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
import de.roessing.app.ui.HomeScreen
import de.roessing.app.ui.LoginScreen
import de.roessing.app.ui.PlacesViewModel
import de.roessing.app.ui.theme.DorfAppTheme
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        setContent {
            DorfAppTheme {
                Root()
            }
        }
    }
}

@Composable
private fun Root() {
    val context = androidx.compose.ui.platform.LocalContext.current
    val container = context.appContainer
    val session by container.authManager.session.collectAsState()
    val scope = rememberCoroutineScope()
    // null = kein Fehler. Ein Abbruch (Zurück-Taste) ist bewusst kein Fehler.
    var loginError by remember { mutableStateOf<String?>(null) }

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

    when (session) {
        is SessionState.Loading -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator()
        }

        is SessionState.LoggedOut -> LoginScreen(
            errorCode = loginError,
            onLogin = {
                loginError = null
                scope.launch {
                    runCatching { authLauncher.launch(container.authManager.buildLoginIntent()) }
                        .onFailure { loginError = it::class.java.simpleName }
                }
            },
            onDevLogin = { asAdmin ->
                scope.launch { container.authManager.devLogin(asAdmin) }
            },
        )

        is SessionState.LoggedIn -> {
            val vm: PlacesViewModel = viewModel(factory = viewModelFactory(container))
            HomeScreen(
                viewModel = vm,
                onLogout = { scope.launch { container.authManager.logout() } },
            )
        }
    }
}

private fun viewModelFactory(container: AppContainer) =
    object : androidx.lifecycle.ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : androidx.lifecycle.ViewModel> create(modelClass: Class<T>): T =
            PlacesViewModel(container.repository) as T
    }
