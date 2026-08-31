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
    case rental
    case ideen
    case anfragen
    case einstellungen
    /// Das Verzeichnis der Vereine und Gruppen.
    case traeger
    /// Ein einzelner Träger.
    case traegerDetail(Int64)
    /// Ein einzelner Ort — der Weg vom Träger zurück zu dem, was er betreut.
    case ort(Int64)
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
            case .rental: RentalCatalogView()
            case .ideen: IdeenView()
            case .anfragen: VergabeView()
            case .einstellungen: EinstellungenView()
            case .traeger: TraegerListView()
            case .traegerDetail(let id): TraegerDetailView(traegerId: id)
            case .ort(let id): OrtZiel(ortId: id)
            }
        }
    }
}

/// Ein Ort als Wertziel.
///
/// „Mithelfen" öffnet seine Orte selbst — es hat die Liste ohnehin in der
/// Hand. Vom Träger aus gibt es die aber nicht: Dort steht nur eine Kennung.
/// Diese Hülle holt das gemeinsame Ortsmodell aus der Umgebung und reicht es
/// weiter, damit der Weg vom Träger zu seinen Orten ohne einen zweiten Abruf
/// und ohne einen zweiten Stand auskommt.
struct OrtZiel: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    let ortId: Int64

    var body: some View {
        OrtDetailView(modell: umgebung.orte, ortId: ortId, meinSub: umgebung.meinSub)
            .task { await umgebung.orte.laden() }
    }
}
