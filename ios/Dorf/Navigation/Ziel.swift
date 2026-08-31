import SwiftUI

/// Die Bereiche der App. Ein Wert je Seite, damit `NavigationStack` mit
/// Wertzielen arbeiten kann und jede Seite auch aus einer Benachrichtigung
/// heraus erreichbar bleibt.
enum Ziel: Hashable {
    case chat
    case mithelfen
    case rangliste
    case profil
    case dorfbewohner
    case veranstaltungen
    case ideen
    case anfragen
    case einstellungen
}

extension View {
    /// Verdrahtet alle Bereiche an genau einer Stelle. Jeder Bereich wohnt in
    /// einer eigenen Datei unter `Bereiche/` — hier steht nur, welcher Wert
    /// wohin führt.
    func dorfZiele() -> some View {
        navigationDestination(for: Ziel.self) { ziel in
            switch ziel {
            case .chat: ChatView()
            case .mithelfen: MithelfenView()
            case .rangliste: RanglisteView()
            case .profil: ProfilView()
            case .dorfbewohner: DorfbewohnerView()
            case .veranstaltungen: VeranstaltungenView()
            case .ideen: IdeenView()
            case .anfragen: VergabeView()
            case .einstellungen: EinstellungenView()
            }
        }
    }
}
