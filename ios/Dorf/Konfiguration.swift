import Foundation

/// Adressen und Kennungen der App.
///
/// Sie stehen in den Build-Einstellungen (`ios/project.yml`) und landen über
/// `Info.plist` hier — dasselbe Verfahren wie `BuildConfig` auf Android. Der
/// Grund ist derselbe: CI und E2E müssen jede Adresse lokal übersteuern
/// können, ohne Quelltext anzufassen. Kein Test darf gegen die Produktion
/// laufen (siehe `.github/scripts/pruefe_lokale_tests.py`).
enum Konfiguration {
    static let apiBasis = url("DorfApiBaseUrl", vorgabe: "https://app.xn--rssing-wxa.de")
    static let webseiteBasis = url("DorfWebsiteBaseUrl", vorgabe: "https://xn--rssing-wxa.de")
    static let oidcAussteller = url("DorfOidcIssuer", vorgabe: "https://id.xn--rssing-wxa.de")
    static let oidcClientId = text("DorfOidcClientId", vorgabe: "387943892076527811")
    static let oidcRuecksprung = text("DorfOidcRedirectUri", vorgabe: "de.roessing.app:/oauth2redirect")
    static let oidcAbmeldeRuecksprung = text("DorfOidcLogoutRedirectUri", vorgabe: "de.roessing.app:/logout")
    static let kartenstil = url("DorfMapStyleUrl", vorgabe: "https://tiles.openfreemap.org/styles/liberty")

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

    /// Das Schema, unter dem der Browser zurück in die App springt
    /// (`de.roessing.app` aus `de.roessing.app:/oauth2redirect`).
    static var ruecksprungSchema: String {
        String(oidcRuecksprung.prefix(while: { $0 != ":" }))
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
