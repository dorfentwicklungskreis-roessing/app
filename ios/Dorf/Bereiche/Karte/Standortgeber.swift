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

    /// Der zuletzt gemeldete eigene Standort — nur gefüllt, solange jemand
    /// ihn ausdrücklich angefordert hat (`beobachten()`). Die Karte selbst
    /// braucht ihn nicht: Den eigenen Punkt zeichnet MapLibre.
    private(set) var letzterPunkt: Kartenpunkt?

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

    /// Beginnt, den eigenen Standort zu verfolgen — für „Meinen Standort
    /// übernehmen" beim Anlegen eines Ortes.
    ///
    /// Bewusst nicht dauerhaft: Wer das Formular schließt, ruft `ruhen()`.
    /// Genauigkeit „nächste zehn Meter" reicht für einen Blumenkasten und
    /// kostet weniger Strom als die feinste Stufe.
    func beobachten() {
        guard freigabe == .erlaubt else { return }
        verwalter.desiredAccuracy = kCLLocationAccuracyNearestTenMeters
        verwalter.startUpdatingLocation()
    }

    func ruhen() {
        verwalter.stopUpdatingLocation()
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

    nonisolated func locationManager(
        _ manager: CLLocationManager,
        didUpdateLocations orte: [CLLocation]
    ) {
        guard let ort = orte.last else { return }
        // Nur die beiden Zahlen wandern über die Grenze — ein CLLocation ist
        // kein Sendable-Wert.
        let breite = ort.coordinate.latitude
        let laenge = ort.coordinate.longitude
        Task { @MainActor [weak self] in
            let punkt = Kartenpunkt(breite: breite, laenge: laenge)
            self?.letzterPunkt = punkt.gueltig ? punkt : nil
        }
    }

    /// Eine gescheiterte Ortung ist kein Fehler der App: Im Haus zwischen
    /// Wänden kommt eben nichts. Der letzte bekannte Punkt bleibt stehen.
    nonisolated func locationManager(_ manager: CLLocationManager, didFailWithError fehler: Error) {}
}
