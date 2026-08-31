import SwiftUI

/// Das Verzeichnis: Welche Vereine und Gruppen es im Dorf gibt, welche
/// Arbeitskreise zu ihnen gehören — und ob ich selbst dabei bin.
///
/// Die Liste ist die des Servers. Eine geschlossene Gruppe steht nur für ihre
/// Mitglieder darin; hier wird nichts gefiltert, sonst gäbe es die
/// Sichtbarkeitsregel zweimal.
struct TraegerListView: View {
    @EnvironmentObject private var umgebung: AppUmgebung

    var body: some View {
        TraegerListInhalt(modell: umgebung.traeger)
            .navigationTitle("Vereine und Gruppen")
            .navigationBarTitleDisplayMode(.inline)
            .task { await umgebung.traeger.load() }
    }
}

/// Die Liste, sobald das Modell steht.
struct TraegerListInhalt: View {
    @ObservedObject var modell: TraegerModel

    var body: some View {
        List {
            if let hinweis = modell.notice {
                Hinweisstreifen(
                    text: hinweis, symbol: "wifi.slash", farbe: .orange,
                    kennung: "traeger-notice"
                )
                .listRowInsets(EdgeInsets())
                .onTapGesture { modell.dismissNotice() }
            }
            if let dank = modell.confirmation {
                Hinweisstreifen(
                    text: dank, symbol: "checkmark.circle.fill", farbe: Ampel.green.farbe,
                    kennung: "traeger-confirmation"
                )
                .listRowInsets(EdgeInsets())
                .onTapGesture { modell.dismissConfirmation() }
            }

            offeneAnfragen

            if modell.roots.isEmpty {
                leerzeile
            }

            ForEach(modell.roots) { verein in
                Section {
                    TraegerZeile(traeger: verein)
                    ForEach(modell.children(of: verein.id)) { arbeitskreis in
                        TraegerZeile(traeger: arbeitskreis, unterTraeger: true)
                    }
                } footer: {
                    if !modell.children(of: verein.id).isEmpty {
                        Text("Arbeitskreise arbeiten unter dem Verein, entscheiden aber selbst, "
                            + "wer bei ihnen mitmacht.")
                    }
                }
            }
        }
        .listStyle(.insetGrouped)
        .refreshable { await modell.load() }
        .accessibilityIdentifier("traeger-list")
        .alert("Das hat nicht geklappt", isPresented: fehlerOffen) {
            Button("Ok", role: .cancel) { modell.dismissError() }
                .accessibilityIdentifier("traeger-error-ok")
        } message: {
            Text(modell.error ?? "")
        }
    }

    /// Was ich selbst angefragt habe und noch nicht entschieden ist. Steht
    /// oben: Wer gefragt hat, sucht als Erstes danach.
    @ViewBuilder
    private var offeneAnfragen: some View {
        let offene = modell.mine.filter { $0.status == "beantragt" }
        if !offene.isEmpty {
            Section("Deine Anfragen") {
                ForEach(offene) { antrag in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(antrag.traegerName.isEmpty ? "Ein Träger" : antrag.traegerName)
                            .font(.subheadline.weight(.medium))
                        Text("Liegt beim Vorstand.")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                    .accessibilityIdentifier("my-request-\(antrag.id)")
                }
            }
        }
    }

    @ViewBuilder
    private var leerzeile: some View {
        if modell.loading && !modell.everLoaded {
            HStack {
                Spacer()
                ProgressView()
                Spacer()
            }
            .listRowSeparator(.hidden)
        } else {
            VStack(alignment: .leading, spacing: 6) {
                Label("Noch keine Vereine im Verzeichnis", systemImage: "person.2.slash")
                    .font(.headline)
                Text("Sobald ein Verein oder eine Gruppe zugelassen ist, steht sie hier — "
                    + "mit ihren Arbeitskreisen und dem Weg zum Mitmachen.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .padding(.vertical, 6)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("traeger-empty")
        }
    }

    private var fehlerOffen: Binding<Bool> {
        Binding(get: { modell.error != nil }, set: { if !$0 { modell.dismissError() } })
    }
}

/// Eine Zeile des Verzeichnisses.
struct TraegerZeile: View {
    let traeger: Traeger
    /// Arbeitskreise stehen eingerückt unter ihrem Verein.
    var unterTraeger = false

    var body: some View {
        NavigationLink(value: Ziel.traegerDetail(traeger.id)) {
            HStack(spacing: 12) {
                Image(systemName: unterTraeger ? "arrow.turn.down.right" : "person.2.fill")
                    .font(unterTraeger ? .footnote : .title3)
                    .frame(width: 24)
                    .foregroundStyle(unterTraeger ? AnyShapeStyle(.secondary) : AnyShapeStyle(.tint))
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 3) {
                    Text(traeger.name).font(unterTraeger ? .subheadline.weight(.medium) : .headline)
                    if !traeger.beschreibung.isEmpty {
                        Text(traeger.beschreibung)
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                            .lineLimit(2)
                    }
                    TraegerAbzeichen(traeger: traeger)
                }
            }
            .padding(.vertical, 2)
            .padding(.leading, unterTraeger ? 12 : 0)
        }
        .accessibilityIdentifier("traeger-\(traeger.id)")
    }
}

/// Die kurzen Marken unter dem Namen: dabei, offen, geschlossen, und wie
/// viele Anfragen auf eine Entscheidung warten.
struct TraegerAbzeichen: View {
    let traeger: Traeger

    var body: some View {
        HStack(spacing: 10) {
            if traeger.istMitglied {
                Label("Du bist dabei", systemImage: "checkmark.seal.fill")
                    .foregroundStyle(Ampel.green.farbe)
                    .accessibilityIdentifier("traeger-member-\(traeger.id)")
            } else if traeger.istGeschlossen {
                Label("Geschlossene Gruppe", systemImage: "lock.fill")
                    .foregroundStyle(.secondary)
            } else if traeger.beitrittStatus == "beantragt" {
                Label("Anfrage läuft", systemImage: "hourglass")
                    .foregroundStyle(.secondary)
            } else if traeger.beitrittMoeglich {
                Label("Mitmachen möglich", systemImage: "hand.raised.fill")
                    .foregroundStyle(.tint)
            }
            if traeger.offeneBeitritte > 0 {
                Label("\(traeger.offeneBeitritte) offen", systemImage: "tray.full.fill")
                    .foregroundStyle(Ampel.yellow.farbe)
                    .accessibilityLabel("\(traeger.offeneBeitritte) Anfragen warten auf deine Entscheidung")
                    .accessibilityIdentifier("traeger-open-\(traeger.id)")
            }
        }
        .font(.caption.weight(.medium))
    }
}
