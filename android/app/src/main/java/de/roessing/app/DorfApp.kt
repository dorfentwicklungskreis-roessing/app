package de.roessing.app

import android.app.Application
import android.content.Context
import de.roessing.app.auth.AuthManager
import de.roessing.app.data.ApiPlacesRepository
import de.roessing.app.data.ApiProfileRepository
import de.roessing.app.data.ApiStatsRepository
import de.roessing.app.data.DorfApi
import de.roessing.app.data.PlacesRepository
import de.roessing.app.data.ProfileRepository
import de.roessing.app.data.StatsRepository

/**
 * Application-Klasse mit manueller Dependency Injection.
 * Bewusst ohne DI-Framework — die App ist klein genug.
 */
class DorfApp : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)
    }
}

class AppContainer(context: Context) {
    val authManager = AuthManager(context.applicationContext)
    private val api = DorfApi.create(BuildConfig.API_BASE_URL) { authManager.freshAccessToken() }
    val repository: PlacesRepository = ApiPlacesRepository(api)
    val statsRepository: StatsRepository = ApiStatsRepository(api)
    val profileRepository: ProfileRepository = ApiProfileRepository(api)
}

val Context.appContainer: AppContainer
    get() = (applicationContext as DorfApp).container
