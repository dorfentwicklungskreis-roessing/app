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

    /// Schreibt den Satz — ohne den alten vorher wegzuwerfen.
    ///
    /// Vorher stand hier `SecItemDelete` und danach `SecItemAdd`. Zwischen
    /// beiden Zeilen gibt es einen Augenblick ohne Eintrag, und scheitert das
    /// Hinzufügen (das Gerät ist noch nie entsperrt worden, der Schlüsselbund
    /// ist gerade nicht ansprechbar), bleibt es dabei: Die laufende Sitzung
    /// merkt nichts, aber der nächste Start findet nichts mehr vor und steht
    /// vor dem Anmeldeknopf. Ein Schreibfehler darf keine gültige Anmeldung
    /// kosten — deshalb erst aktualisieren und nur anlegen, wenn wirklich
    /// nichts da ist.
    @discardableResult
    static func sichern(_ satz: Tokensatz) -> Bool {
        guard let daten = try? JSONEncoder().encode(satz) else { return false }
        let suche: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: dienst,
            kSecAttrAccount as String: konto,
        ]
        let stand = SecItemUpdate(suche as CFDictionary,
                                  [kSecValueData as String: daten] as CFDictionary)
        if stand == errSecSuccess { return true }
        guard stand == errSecItemNotFound else { return false }

        var eintrag = suche
        eintrag[kSecValueData as String] = daten
        eintrag[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        return SecItemAdd(eintrag as CFDictionary, nil) == errSecSuccess
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
        sichern: { _ = Schluesselbund.sichern($0) },
        loeschen: { Schluesselbund.loeschen() }
    )
}
