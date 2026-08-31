import Foundation

/// Adressen und Kennungen der App.
///
/// `nonisolated`, weil der API-Zugang außerhalb des Hauptthreads gebaut wird
/// und diese Werte dort als Vorgabewerte auftauchen: Ein `MainActor`-Wert als
/// Vorgabeargument in einem `nonisolated`-Initialisierer ist unter Swift 6
/// ein Fehler. Die Werte sind unveränderlich und aus der `Info.plist`
/// gelesen — an ihnen ist nichts zu schützen.
///
/// Sie stehen in den Build-Einstellungen (`ios/project.yml`) und landen über
/// `Info.plist` hier — dasselbe Verfahren wie `BuildConfig` auf Android. Der
/// Grund ist derselbe: CI und E2E müssen jede Adresse lokal übersteuern
/// können, ohne Quelltext anzufassen. Kein Test darf gegen die Produktion
/// laufen (siehe `.github/scripts/pruefe_lokale_tests.py`).
nonisolated enum Konfiguration {
    static let apiBasis = url("DorfApiBaseUrl", vorgabe: "https://app.xn--rssing-wxa.de")
    static let webseiteBasis = url("DorfWebsiteBaseUrl", vorgabe: "https://xn--rssing-wxa.de")
    static let oidcAussteller = url("DorfOidcIssuer", vorgabe: "https://id.xn--rssing-wxa.de")
    static let oidcClientId = text("DorfOidcClientId", vorgabe: "387943892076527811")
    static let oidcRuecksprung = text("DorfOidcRedirectUri", vorgabe: "de.roessing.app:/oauth2redirect")
    static let oidcAbmeldeRuecksprung = text("DorfOidcLogoutRedirectUri", vorgabe: "de.roessing.app:/logout")
    static let kartenstil = url("DorfMapStyleUrl", vorgabe: "https://tiles.openfreemap.org/styles/liberty")

    /// The rental platform („Maschinchenring"). A service of its own next to
    /// the backend: the app talks to it directly, the Go backend knows
    /// nothing about it (`docs/mietplattform-in-den-apps.md`, AP 4).
    static let rentalBaseUrl = url("DorfRentalBaseUrl", vorgabe: "https://mieten.xn--rssing-wxa.de")

    /// The rental platform's Zitadel project. It has to appear as an audience
    /// in our access token, otherwise every call is answered with 401 — see
    /// `rentalAudienceScope` in `Anmeldung.swift`.
    static let rentalProjectId = text("DorfRentalProjectId", vorgabe: "377276525071827047")

    /// Der Entwickler-Login (ohne Rössing-ID) — nur im Debug-Build und nur,
    /// wenn er ausdrücklich eingeschaltet wurde. In einer ausgelieferten App
    /// gibt es ihn nicht, auch nicht versehentlich.
    static var entwicklerLoginErlaubt: Bool {
        #if DEBUG
            return text("DorfDevAuth", vorgabe: "0") == "1"
        #else
            return false
        #endif
    }


    private static func text(_ schluessel: String, vorgabe: String) -> String {
        guard let roh = Bundle.main.object(forInfoDictionaryKey: schluessel) as? String,
              !roh.trimmingCharacters(in: .whitespaces).isEmpty
        else { return vorgabe }
        return roh.trimmingCharacters(in: .whitespaces)
    }

    private static func url(_ schluessel: String, vorgabe: String) -> URL {
        // Eine unbrauchbare Adresse darf die App nicht beim Start umbringen;
        // sie fällt auf die Vorgabe zurück und meldet sich beim ersten Abruf.
        URL(string: text(schluessel, vorgabe: vorgabe)) ?? URL(string: vorgabe)!
    }
}
