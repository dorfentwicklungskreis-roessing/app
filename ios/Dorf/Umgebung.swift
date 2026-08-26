import Observation
import SwiftUI

/// Alles, was die Bereiche der App brauchen, an einer Stelle — die
/// Handverdrahtung entspricht dem `AppContainer` der Android-App. Bewusst kein
/// DI-Framework: Die App hat eine Handvoll Abhängigkeiten, und ein Framework
/// müsste über Jahre mitgepflegt werden.
///
/// Zugriff aus einer View:
/// ```swift
/// @Environment(AppUmgebung.self) private var umgebung
/// ```
@Observable
final class AppUmgebung {
    let anmeldung: Anmeldung
    let api: DorfApi

    /// Wer gerade angemeldet ist. Wird nach der Anmeldung einmal geladen und
    /// danach von den Bereichen mitbenutzt — `isAdmin` entscheidet, ob der
    /// Bereich „Verwaltung" überhaupt auftaucht.
    private(set) var ich: Ich?
    private(set) var ichFehler: String?

    init(anmeldung: Anmeldung = Anmeldung()) {
        self.anmeldung = anmeldung
        self.api = DorfApi(tokenGeber: { [anmeldung] in
            await anmeldung.frischesToken()
        })
    }

    /// Für Vorschauen und Tests: eine Umgebung, die nichts abruft.
    init(anmeldung: Anmeldung, api: DorfApi, ich: Ich?) {
        self.anmeldung = anmeldung
        self.api = api
        self.ich = ich
    }

    var binAdmin: Bool { ich?.isAdmin ?? false }
    var meinSub: String? { ich?.sub }

    func ichLaden() async {
        guard case .angemeldet = anmeldung.sitzung else { return }
        do {
            ich = try await api.ich()
            ichFehler = nil
        } catch let fehler as DorfFehler {
            // Kein Grund, die App anzuhalten: Ohne Profil zeigt die Startseite
            // eben „Moin!" statt „Moin, Anna!".
            ichFehler = fehler.klartext
        } catch {
            ichFehler = "Unerwarteter Fehler."
        }
    }

    func profilUebernehmen(_ profil: Profil) {
        guard var stand = ich else { return }
        stand.profile = profil
        ich = stand
    }

    /// Der Name, mit dem die Startseite grüßt: Nickname vor Anzeigename vor
    /// dem Namen aus der Rössing-ID.
    var anrede: String? {
        let kandidaten = [ich?.profile?.nickname, ich?.profile?.displayName, ich?.name]
        return kandidaten.compactMap { $0 }.first { !$0.trimmingCharacters(in: .whitespaces).isEmpty }
    }
}
