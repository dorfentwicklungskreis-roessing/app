package de.roessing.app

import android.app.Application
import android.content.Context
import android.os.Build
import de.roessing.app.auth.AuthManager
import de.roessing.app.data.ApiDeviceRepository
import de.roessing.app.data.ApiErrorReportRepository
import de.roessing.app.data.ApiIdeenRepository
import de.roessing.app.data.ApiPlacesRepository
import de.roessing.app.data.ApiProfileRepository
import de.roessing.app.data.ApiStatsRepository
import de.roessing.app.data.ApiVergabeRepository
import de.roessing.app.data.DeviceRepository
import de.roessing.app.data.ErrorReportRepository
import de.roessing.app.data.IdeenRepository
import de.roessing.app.data.DorfApi
import de.roessing.app.data.PlacesRepository
import de.roessing.app.data.ProfileRepository
import de.roessing.app.data.StatsRepository
import de.roessing.app.data.VeranstaltungenRepository
import de.roessing.app.data.VergabeRepository
import de.roessing.app.data.WebsiteApi
import de.roessing.app.data.WebsiteVeranstaltungenRepository
import de.roessing.app.errors.CrashWatch
import de.roessing.app.errors.DeviceFacts
import de.roessing.app.errors.ErrorReportTexte
import de.roessing.app.errors.ErrorReporter
import de.roessing.app.errors.SharedPrefsCrashStore
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
        // Muss vor allem anderen stehen: Was jetzt noch schiefgeht, soll
        // schon gefangen werden.
        CrashWatch.install(this)
        container = AppContainer(this)
        // Was der letzte Lauf hinterlassen hat, kommt beim Start auf den
        // Schirm — angezeigt, nicht verschickt. Abschicken tut die Person.
        CrashWatch.pendingCrash(
            speicher = SharedPrefsCrashStore(this),
            jetzt = System.currentTimeMillis(),
            meldung = getString(R.string.error_report_crash),
        )?.let { container.errorReporter.report(it) }
        // Die Kanäle müssen stehen, bevor die erste Nachricht eintrifft —
        // Android verwirft sonst Meldungen mit unbekanntem Kanal.
        Kanaele.anlegen(this)
    }
}

class AppContainer(context: Context) {
    val authManager = AuthManager(context.applicationContext)

    /**
     * Der Melder für Fehlerberichte. Einer für die ganze App — jede Stelle
     * muss melden können, und angezeigt wird es an genau einer. Er hängt als
     * Beobachter an der API und bekommt so jede gescheiterte Anfrage zu
     * sehen, ohne dass ein Bereich daran denken muss.
     */
    val errorReporter = ErrorReporter(
        texte = ErrorReportTexte(
            ohneVerbindung = context.getString(R.string.error_report_offline),
            nichtGefunden = context.getString(R.string.error_report_not_found),
            abschickenGescheitert = context.getString(R.string.error_report_send_failed),
            zeileWas = context.getString(R.string.error_report_line_what),
            zeileBereich = context.getString(R.string.error_report_line_area),
            zeileTechnisch = context.getString(R.string.error_report_line_technical),
            zeileDeinText = context.getString(R.string.error_report_line_yours),
            zeileApp = context.getString(R.string.error_report_line_app),
            zeileGeraet = context.getString(R.string.error_report_line_device),
            zeileWann = context.getString(R.string.error_report_line_when),
            serverfehlerVorlage = context.getString(R.string.error_report_server),
        ),
        facts = DeviceFacts.of(
            versionName = BuildConfig.VERSION_NAME,
            versionCode = BuildConfig.VERSION_CODE,
            sdk = Build.VERSION.SDK_INT,
            release = Build.VERSION.RELEASE.orEmpty(),
            manufacturer = Build.MANUFACTURER.orEmpty(),
            model = Build.MODEL.orEmpty(),
        ),
    )

    private val api = DorfApi.create(
        BuildConfig.API_BASE_URL,
        beobachter = errorReporter,
    ) { authManager.freshToken() }
    val repository: PlacesRepository = ApiPlacesRepository(api)
    val statsRepository: StatsRepository = ApiStatsRepository(api)
    val profileRepository: ProfileRepository = ApiProfileRepository(api)
    val vergabeRepository: VergabeRepository = ApiVergabeRepository(api)
    val deviceRepository: DeviceRepository = ApiDeviceRepository(api)
    val ideenRepository: IdeenRepository = ApiIdeenRepository(api)
    val errorReportRepository: ErrorReportRepository = ApiErrorReportRepository(api)

    init {
        // Der Weg zum Backend, einmal verdrahtet — wie bei den
        // Benachrichtigungen. Der Eingang kommt auch ohne Anmeldung durch;
        // genau darauf kommt es an, wenn die Anmeldung selbst klemmt.
        errorReporter.wire(errorReportRepository)
    }

    // Die Veranstaltungen kommen nicht aus dem Dorf-Backend, sondern von der
    // Website — dort werden sie gepflegt. Eigener Client, ohne Token.
    val veranstaltungenRepository: VeranstaltungenRepository =
        WebsiteVeranstaltungenRepository(WebsiteApi.create(BuildConfig.WEBSITE_BASE_URL))
}

val Context.appContainer: AppContainer
    get() = (applicationContext as DorfApp).container
