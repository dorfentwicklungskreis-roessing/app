import SwiftUI

/// Der Bereich „Anfragen": Wer sich zum Mithelfen angemeldet hat, wird
/// gefragt, sobald an „seinem" Ort etwas ansteht — der Reihe nach, nicht
/// alle auf einmal.
///
/// Die Regeln dazu (Reihenfolge, Fristen, Ruhezeiten) stehen im Backend.
/// Hier steht nur, wie das aussieht und welche Knöpfe es gibt.
struct VergabeView: View {
    @Environment(AppUmgebung.self) private var umgebung
    @State private var modell: VergabeModell?

    var body: some View {
        Group {
            if let modell {
                VergabeInhalt(modell: modell)
            } else {
                ProgressView().controlSize(.large)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle("Anfragen")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            let vorhanden = modell ?? VergabeModell(api: umgebung.api, meinSub: umgebung.meinSub)
            modell = vorhanden
            await vorhanden.laden()
        }
    }
}

/// Die Liste, sobald das Modell steht.
struct VergabeInhalt: View {
    let modell: VergabeModell

    /// Der Vorgang, zu dem gerade nach der Rückgabe gefragt wird. Eine Zusage
    /// gibt niemand aus Versehen zurück.
    @State private var rueckgabe: Zusagestand?

    var body: some View {
        List {
            streifen

            if !modell.zusagen.isEmpty {
                Section("Deine Zusagen") {
                    ForEach(modell.zusagen) { stand in
                        ZusageZeile(modell: modell, stand: stand) { rueckgabe = stand }
                    }
                }
            }

            Section {
                if modell.leer {
                    leerzeile
                }
                ForEach(modell.geordnet) { eintrag in
                    BenachrichtigungZeile(modell: modell, eintrag: eintrag)
                }
            }
        }
        .listStyle(.insetGrouped)
        .refreshable { await modell.laden() }
        .accessibilityIdentifier("anfragen-liste")
        .alert("Das hat nicht geklappt", isPresented: fehlerOffen) {
            Button("Ok", role: .cancel) { modell.fehlerVerwerfen() }
                .accessibilityIdentifier("anfragen-fehler-ok")
        } message: {
            Text(modell.fehler ?? "")
        }
        .alert("Zusage zurückgeben?", isPresented: rueckgabeOffen, presenting: rueckgabe) { stand in
            Button("Abbrechen", role: .cancel) { rueckgabe = nil }
            Button("Zurückgeben", role: .destructive) {
                rueckgabe = nil
                Task { await modell.zurueckgeben(vorgang: stand.id) }
            }
            .accessibilityIdentifier("zurueckgeben-bestaetigen")
        } message: { stand in
            Text(stand.was.isEmpty
                ? "Die Aufgabe wird wieder freigegeben und den anderen angeboten."
                : "\(stand.was) wird wieder freigegeben und den anderen angeboten.")
        }
    }

    @ViewBuilder
    private var streifen: some View {
        if let hinweis = modell.hinweis {
            Hinweisstreifen(
                text: hinweis, symbol: "person.2.fill", farbe: .orange,
                kennung: "anfragen-hinweis"
            )
            .listRowInsets(EdgeInsets())
            .onTapGesture { modell.hinweisVerwerfen() }
        }
        if let dank = modell.bestaetigung {
            Hinweisstreifen(
                text: dank, symbol: "checkmark.circle.fill", farbe: Ampel.green.farbe,
                kennung: "anfragen-bestaetigung"
            )
            .listRowInsets(EdgeInsets())
            .onTapGesture { modell.bestaetigungVerwerfen() }
        }
    }

    @ViewBuilder
    private var leerzeile: some View {
        if modell.laeuft && !modell.jeGeladen {
            HStack {
                Spacer()
                ProgressView()
                Spacer()
            }
            .listRowSeparator(.hidden)
        } else {
            VStack(alignment: .leading, spacing: 6) {
                Label("Gerade ist nichts offen", systemImage: "checkmark.circle")
                    .font(.headline)
                Text("Steht an einem Ort etwas an, für den du mithilfst, fragen wir dich hier. "
                    + "Zieh die Liste nach unten, um neu zu laden.")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            .padding(.vertical, 6)
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("anfragen-leer")
        }
    }

    private var fehlerOffen: Binding<Bool> {
        Binding(get: { modell.fehler != nil }, set: { if !$0 { modell.fehlerVerwerfen() } })
    }

    private var rueckgabeOffen: Binding<Bool> {
        Binding(get: { rueckgabe != nil }, set: { if !$0 { rueckgabe = nil } })
    }
}

/// Eine Anfrage („du bist dran") oder ein Hinweis.
struct BenachrichtigungZeile: View {
    let modell: VergabeModell
    let eintrag: Benachrichtigung

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label {
                Text(eintrag.anzeigetitel).font(.headline)
            } icon: {
                Image(systemName: eintrag.symbol)
                    .foregroundStyle(eintrag.istAnfrage ? Ampel.yellow.farbe : .secondary)
            }

            if !eintrag.anzeigetext.isEmpty {
                Text(eintrag.anzeigetext)
                    .font(.subheadline)
                    .fixedSize(horizontal: false, vertical: true)
            }

            if let frist = eintrag.fristtext() {
                Text(frist)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("anfrage-frist-\(eintrag.id)")
            }

            knoepfe
        }
        .padding(.vertical, 4)
        .accessibilityIdentifier("anfrage-\(eintrag.id)")
    }

    @ViewBuilder
    private var knoepfe: some View {
        if eintrag.istAnfrage {
            Button {
                Task { await modell.zusagen(eintrag) }
            } label: {
                HStack(spacing: 8) {
                    if modell.laeuftGerade(vorgang: eintrag.assignmentId) {
                        ProgressView().controlSize(.small)
                    }
                    Text("Ich mache das")
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .disabled(modell.laeuftGerade(vorgang: eintrag.assignmentId))
            .accessibilityIdentifier("zusagen-\(eintrag.assignmentId)")
        } else {
            // Ein Hinweis ist mit dem Lesen erledigt — mehr ist nicht zu tun.
            Button("Verstanden") {
                Task { await modell.gelesen(eintrag) }
            }
            .buttonStyle(.bordered)
            .disabled(modell.laeuftGerade(hinweis: eintrag.id))
            .accessibilityIdentifier("gelesen-\(eintrag.id)")
        }
    }
}

/// Eine Zusage, die ich gegeben habe — samt Frist und dem Weg zurück.
struct ZusageZeile: View {
    let modell: VergabeModell
    let stand: Zusagestand
    var zurueckgeben: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label {
                Text(stand.was.isEmpty ? "Deine Zusage" : stand.was).font(.headline)
            } icon: {
                Image(systemName: "hand.thumbsup.fill")
                    .foregroundStyle(Ampel.green.farbe)
            }
            Text(stand.fristtext)
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)
            Button("Zusage zurückgeben", role: .destructive) { zurueckgeben() }
                .buttonStyle(.bordered)
                .disabled(modell.laeuftGerade(vorgang: stand.id))
                .accessibilityIdentifier("zurueckgeben-\(stand.id)")
        }
        .padding(.vertical, 4)
        .accessibilityIdentifier("zusage-\(stand.id)")
    }
}
