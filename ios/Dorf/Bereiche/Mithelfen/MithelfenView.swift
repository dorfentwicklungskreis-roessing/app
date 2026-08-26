import SwiftUI

/// Der Bereich „Mithelfen": Was gerade im Dorf ansteht — als Karte oder als
/// Liste, und dahinter je Ort die Aufgaben mit dem Knopf zum Melden.
struct MithelfenView: View {
    @Environment(AppUmgebung.self) private var umgebung

    var body: some View {
        // Das Modell gehört der **Umgebung**, nicht dieser Seite: Die
        // Startseite zählt daraus die wartenden Orte und liest den
        // Hitzefaktor. Ein eigenes Modell hier hieße ein zweiter Abruf und —
        // schlimmer — zwei Stände: Die Kachel zählte dann etwas anderes, als
        // die Liste dahinter zeigt.
        MithelfenInhalt(modell: umgebung.orte, meinSub: umgebung.meinSub)
            .navigationTitle("Mithelfen")
            .navigationBarTitleDisplayMode(.inline)
            // Beim zweiten Erscheinen (Zurück aus dem Detail) wird nur neu
            // geladen; der letzte Stand bleibt so lange stehen.
            .task { await umgebung.orte.laden() }
    }
}

/// Karte oder Liste. Zwei Blicke auf dieselben Daten — geladen wird nur einmal.
enum Mithelfenansicht: String, CaseIterable, Identifiable {
    case karte, liste

    var id: String { rawValue }

    var titel: String {
        switch self {
        case .karte: return "Karte"
        case .liste: return "Liste"
        }
    }
}

/// Der eigentliche Bereich, sobald das Modell steht.
struct MithelfenInhalt: View {
    let modell: OrteModell
    let meinSub: String?

    @State private var ansicht: Mithelfenansicht = .liste
    @State private var gewaehlterOrt: Int64?

    var body: some View {
        VStack(spacing: 0) {
            Picker("Ansicht", selection: $ansicht) {
                ForEach(Mithelfenansicht.allCases) { Text($0.titel).tag($0) }
            }
            .pickerStyle(.segmented)
            .padding(.horizontal, 16)
            .padding(.vertical, 8)
            .accessibilityIdentifier("mithelfen-ansicht")

            Streifen(modell: modell)

            switch ansicht {
            case .karte:
                // Feste Schnittstelle: Die Karte lädt nichts, sie bekommt die
                // Orte gereicht und meldet den Tipp zurück.
                KarteView(orte: modell.orte) { ort in gewaehlterOrt = ort.id }
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            case .liste:
                OrteListe(modell: modell) { gewaehlterOrt = $0 }
            }
        }
        .navigationDestination(item: $gewaehlterOrt) { id in
            OrtDetailView(modell: modell, ortId: id, meinSub: meinSub)
        }
        // Der Fehler hängt am ganzen Bereich, damit er auch über der
        // Detailseite erscheint — dort wird gemeldet.
        .alert("Das hat nicht geklappt", isPresented: fehlerOffen) {
            Button("Ok", role: .cancel) { modell.fehlerVerwerfen() }
                .accessibilityIdentifier("mithelfen-fehler-ok")
        } message: {
            Text(modell.fehler ?? "")
        }
    }

    private var fehlerOffen: Binding<Bool> {
        Binding(get: { modell.fehler != nil }, set: { if !$0 { modell.fehlerVerwerfen() } })
    }
}

/// Hinweis und Dank über der Liste — beide sagen es im Text, nicht nur in der
/// Farbe.
struct Streifen: View {
    let modell: OrteModell

    var body: some View {
        VStack(spacing: 0) {
            if let hinweis = modell.hinweis {
                Hinweisstreifen(
                    text: hinweis, symbol: "wifi.slash", farbe: .orange,
                    kennung: "mithelfen-netzhinweis"
                )
            }
            if let dank = modell.bestaetigung {
                Hinweisstreifen(
                    text: dank, symbol: "checkmark.circle.fill", farbe: Ampel.green.farbe,
                    kennung: "mithelfen-bestaetigung"
                )
                .onTapGesture { modell.bestaetigungVerwerfen() }
            }
        }
    }
}

/// Ein farbiger Streifen mit Symbol und Text. Die Farbe ist Beiwerk — die
/// Aussage steht im Text.
struct Hinweisstreifen: View {
    let text: String
    var symbol: String
    var farbe: Color
    var kennung: String

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Image(systemName: symbol)
                .foregroundStyle(farbe)
                .accessibilityHidden(true)
            Text(text)
                .font(.footnote)
                .fixedSize(horizontal: false, vertical: true)
            Spacer(minLength: 0)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(farbe.opacity(0.14))
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier(kennung)
    }
}

/// Die Orte, dringendste zuerst.
struct OrteListe: View {
    let modell: OrteModell
    var auswahl: (Int64) -> Void

    var body: some View {
        List {
            if modell.orte.isEmpty {
                if modell.laeuft && !modell.jeGeladen {
                    HStack {
                        Spacer()
                        ProgressView()
                        Spacer()
                    }
                    .listRowSeparator(.hidden)
                } else {
                    Text("Gerade steht nichts an. Zieh die Liste nach unten, um neu zu laden.")
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("orte-leer")
                }
            }
            ForEach(modell.nachDringlichkeit) { ort in
                Button { auswahl(ort.id) } label: { OrtZeile(ort: ort) }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("ort-\(ort.id)")
            }
        }
        .listStyle(.insetGrouped)
        .refreshable { await modell.laden() }
        .accessibilityIdentifier("orte-liste")
    }
}

/// Eine Zeile: Name, Ampel mit Text, die drängendste offene Aufgabe.
struct OrtZeile: View {
    let ort: Ort

    var body: some View {
        HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 6) {
                Text(ort.name)
                    .font(.headline)
                    .foregroundStyle(.primary)
                Ampelpunkt(ampel: ort.ampel)
                Text(ort.kuerzesteOffeneAufgabe?.kurztext ?? "Nichts offen")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
            Spacer(minLength: 8)
            Image(systemName: "chevron.right")
                .font(.footnote.weight(.semibold))
                .foregroundStyle(.tertiary)
                .accessibilityHidden(true)
        }
        .padding(.vertical, 4)
        .contentShape(Rectangle())
    }
}
