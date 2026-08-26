import SwiftUI

/// Entscheidet nur eins: angemeldet oder nicht. Alles Weitere hängt an
/// [StartView].
struct WurzelView: View {
    @EnvironmentObject private var umgebung: AppUmgebung

    var body: some View {
        switch umgebung.anmeldung.sitzung {
        case .laedt:
            ProgressView().controlSize(.large)
        case .abgemeldet:
            AnmeldungView()
        case .angemeldet:
            StartView()
                .task { await umgebung.ichLaden() }
        }
    }
}
