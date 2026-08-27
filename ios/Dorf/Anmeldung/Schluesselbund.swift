import Foundation
import Security

/// Der Tokensatz der Anmeldung, wie er zwischen zwei Starts überlebt.
struct Tokensatz: Codable, Sendable {
    var zugangstoken: String
    var erneuerungstoken: String?
    var idToken: String?
    /// Zeitpunkt, ab dem das Zugangstoken als abgelaufen gilt.
    var laeuftAbAm: Date

    /// Eine Minute Sicherheitsabstand: Ein Token, das während der Anfrage
    /// abläuft, wäre schlimmer als eine Erneuerung zu früh.
    func gueltig(jetzt: Date = Date()) -> Bool {
        jetzt.addingTimeInterval(60) < laeuftAbAm
    }
}

/// Ablage der Anmeldedaten im Schlüsselbund.
///
/// Bewusst nicht in `UserDefaults`: Dort läge ein gültiges Zugangstoken im
/// Klartext im Dateisystem und in jedem iTunes-/iCloud-Backup. Der Eintrag
/// bekommt `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly` — nach dem
/// ersten Entsperren lesbar (sonst könnte die App nach einem Neustart nichts
/// nachladen), aber er wandert nicht auf ein anderes Gerät.
enum Schluesselbund {
    private static let dienst = "de.roessing.app.anmeldung"
    private static let konto = "tokensatz"

    static func sichern(_ satz: Tokensatz) {
        guard let daten = try? JSONEncoder().encode(satz) else { return }
        let suche: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: dienst,
            kSecAttrAccount as String: konto,
        ]
        SecItemDelete(suche as CFDictionary)
        var eintrag = suche
        eintrag[kSecValueData as String] = daten
        eintrag[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        SecItemAdd(eintrag as CFDictionary, nil)
    }

    static func lesen() -> Tokensatz? {
        let suche: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: dienst,
            kSecAttrAccount as String: konto,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var ergebnis: CFTypeRef?
        guard SecItemCopyMatching(suche as CFDictionary, &ergebnis) == errSecSuccess,
              let daten = ergebnis as? Data
        else { return nil }
        return try? JSONDecoder().decode(Tokensatz.self, from: daten)
    }

    static func loeschen() {
        let suche: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: dienst,
            kSecAttrAccount as String: konto,
        ]
        SecItemDelete(suche as CFDictionary)
    }
}

/// Wo der Tokensatz zwischen zwei Starts liegt.
///
/// A small bundle of closures instead of a protocol: `Anmeldung` needs
/// exactly three calls, and a test hands in its own three — otherwise every
/// run would write into the real keychain of the machine it runs on.
struct Tokenablage {
    var lesen: () -> Tokensatz?
    var sichern: (Tokensatz) -> Void
    var loeschen: () -> Void

    /// Der echte Schlüsselbund — die Vorbelegung der App.
    static let schluesselbund = Tokenablage(
        lesen: { Schluesselbund.lesen() },
        sichern: { Schluesselbund.sichern($0) },
        loeschen: { Schluesselbund.loeschen() }
    )
}
