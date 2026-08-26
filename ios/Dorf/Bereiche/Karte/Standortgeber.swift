import CoreLocation
import Observation

/// Die Ortungsfreigabe für die Karte — und sonst nichts.
///
/// Gefragt wird erst, wenn jemand „Mein Standort" drückt. Eine Karte, die
/// beim ersten Öffnen einen Systemdialog wirft, bekommt ein „Nicht erlauben"
/// und danach nie wieder eine Chance. Der Standort bleibt auf dem Gerät: Die
/// App zeigt ihn nur an und schickt ihn nirgendwohin.
@Observable
final class Standortgeber: NSObject, CLLocationManagerDelegate {
    enum Freigabe: Sendable {
        /// Noch nie gefragt — der Knopf darf den Systemdialog auslösen.
        case ungefragt
        /// Erteilt (beim Benutzen der App).
        case erlaubt
        /// Abgelehnt oder gesperrt. Nur die Einstellungen können das ändern.
        case verweigert
    }

    private(set) var freigabe: Freigabe = .ungefragt

    private let verwalter = CLLocationManager()

    override init() {
        super.init()
        verwalter.delegate = self
        freigabe = Self.freigabe(aus: verwalter.authorizationStatus)
    }

    /// Fragt genau einmal; ist schon entschieden, passiert nichts. Die Antwort
    /// kommt über den Delegaten und ändert `freigabe`.
    func anfragen() {
        guard freigabe == .ungefragt else { return }
        verwalter.requestWhenInUseAuthorization()
    }

    static func freigabe(aus stand: CLAuthorizationStatus) -> Freigabe {
        switch stand {
        case .notDetermined: return .ungefragt
        case .authorizedWhenInUse, .authorizedAlways: return .erlaubt
        case .denied, .restricted: return .verweigert
        @unknown default: return .verweigert
        }
    }

    // CoreLocation ruft den Delegaten ohne Actor-Bezug; die Antwort wird
    // deshalb ausdrücklich auf den Hauptthread gereicht, wo die Oberfläche
    // sie lesen darf.
    nonisolated func locationManagerDidChangeAuthorization(_ manager: CLLocationManager) {
        let stand = manager.authorizationStatus
        Task { @MainActor [weak self] in
            self?.freigabe = Self.freigabe(aus: stand)
        }
    }
}
