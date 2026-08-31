package de.roessing.app

import android.app.Application
import android.content.Context
import de.roessing.app.auth.AuthManager
import de.roessing.app.auth.RentalAudience
import de.roessing.app.data.ApiChatRepository
import de.roessing.app.data.ApiDeviceRepository
import de.roessing.app.data.ApiIdeenRepository
import de.roessing.app.data.ApiPlacesRepository
import de.roessing.app.data.ApiProfileRepository
import de.roessing.app.data.ApiStatsRepository
import de.roessing.app.data.ApiVergabeRepository
import de.roessing.app.data.ChatRepository
import de.roessing.app.data.DeviceRepository
import de.roessing.app.data.IdeenRepository
import de.roessing.app.data.DorfApi
import de.roessing.app.data.MietenApi
import de.roessing.app.data.MietenRentalRepository
import de.roessing.app.data.RentalRepository
import de.roessing.app.data.PlacesRepository
import de.roessing.app.data.ProfileRepository
import de.roessing.app.data.ApiTraegerRepository
import de.roessing.app.data.StatsRepository
import de.roessing.app.data.TraegerRepository
import de.roessing.app.data.VeranstaltungenRepository
import de.roessing.app.data.VergabeRepository
import de.roessing.app.data.WebsiteApi
import de.roessing.app.data.WebsiteVeranstaltungenRepository
import de.roessing.app.push.Kanaele

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
        // Die Kanäle müssen stehen, bevor die erste Nachricht eintrifft —
        // Android verwirft sonst Meldungen mit unbekanntem Kanal.
        Kanaele.anlegen(this)
    }
}

class AppContainer(context: Context) {
    val authManager = AuthManager(context.applicationContext)
    private val api = DorfApi.create(BuildConfig.API_BASE_URL) { authManager.freshToken() }
    val repository: PlacesRepository = ApiPlacesRepository(api)
    val statsRepository: StatsRepository = ApiStatsRepository(api)
    val profileRepository: ProfileRepository = ApiProfileRepository(api)
    val vergabeRepository: VergabeRepository = ApiVergabeRepository(api)
    val deviceRepository: DeviceRepository = ApiDeviceRepository(api)
    val ideenRepository: IdeenRepository = ApiIdeenRepository(api)
    val chatRepository: ChatRepository = ApiChatRepository(api)
    val traegerRepository: TraegerRepository = ApiTraegerRepository(api)

    // Die Veranstaltungen kommen nicht aus dem Dorf-Backend, sondern von der
    // Website — dort werden sie gepflegt. Eigener Client, ohne Token.
    val veranstaltungenRepository: VeranstaltungenRepository =
        WebsiteVeranstaltungenRepository(WebsiteApi.create(BuildConfig.WEBSITE_BASE_URL))

    // Der Maschinchenring läuft auf einem eigenen Server. Das Dorf-Backend ist
    // kein Weiterleiter und weiß von ihm nichts — deshalb ein eigener Client,
    // wie bei den Veranstaltungen, nur dass hier ein Token mitgeht.
    val rentalRepository: RentalRepository = MietenRentalRepository(
        MietenApi.create(BuildConfig.MIETEN_BASE_URL) { authManager.freshToken() },
        signIn = {
            RentalAudience.state(authManager.freshToken(), BuildConfig.MIETEN_PROJECT_ID)
        },
    )

    /** Wie die Anmeldung dieses Geräts zur Mietplattform steht. */
    suspend fun rentalSignIn() =
        RentalAudience.state(authManager.freshToken(), BuildConfig.MIETEN_PROJECT_ID)
}

val Context.appContainer: AppContainer
    get() = (applicationContext as DorfApp).container
