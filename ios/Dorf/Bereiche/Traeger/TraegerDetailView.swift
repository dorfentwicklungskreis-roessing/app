import SwiftUI

/// Ein Träger mit allem, was zu ihm gehört: wer er ist, welche Arbeitskreise
/// unter ihm arbeiten, welche Orte er betreut — und der Weg zum Mitmachen.
///
/// Wer im Vorstand ist, entscheidet die Anfragen hier, ohne die App zu
/// verlassen. Ob er das darf, sagt der Server (`darfVerwalten`); die App
/// prüft es nicht nach.
struct TraegerDetailView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    let traegerId: Int64

    var body: some View {
        TraegerDetailInhalt(modell: umgebung.traeger, orte: umgebung.orte, traegerId: traegerId)
            .navigationBarTitleDisplayMode(.inline)
            .task {
                await umgebung.traeger.load()
                await umgebung.traeger.refresh(traeger: traegerId)
                await umgebung.orte.laden()
            }
    }
}

struct TraegerDetailInhalt: View {
    @ObservedObject var modell: TraegerModel
    @ObservedObject var orte: OrteModell
    let traegerId: Int64

    /// Der Text zum Mitmachen — steht in einem eigenen Blatt, damit ein
    /// Fehlschlag ihn nicht kostet.
    @State private var mitmachenOffen = false
    @State private var begruendung = ""
    /// Wen der Vorstand gerade direkt aufnehmen will.
    @State private var aufnahmeOffen = false

    private var traeger: Traeger? { modell.traeger(id: traegerId) }

    var body: some View {
        Group {
            if let traeger {
                inhalt(traeger)
            } else if modell.loading && !modell.everLoaded {
                ProgressView().controlSize(.large)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            } else {
                Hinweistafel(
                    "Diesen Träger gibt es nicht",
                    symbol: "person.2.slash",
                    beschreibung: "Vielleicht wurde er gesperrt — oder er steht nicht "
                        + "für dich im Verzeichnis."
                )
            }
        }
        .navigationTitle(traeger?.name ?? "Träger")
        .alert("Das hat nicht geklappt", isPresented: fehlerOffen) {
            Button("Ok", role: .cancel) { modell.dismissError() }
                .accessibilityIdentifier("traeger-error-ok")
        } message: {
            Text(modell.error ?? "")
        }
        .sheet(isPresented: $mitmachenOffen) {
            if let traeger {
                MitmachenBlatt(modell: modell, traeger: traeger, begruendung: $begruendung) {
                    mitmachenOffen = false
                }
            }
        }
        .sheet(isPresented: $aufnahmeOffen) {
            if let traeger {
                AufnahmeBlatt(modell: modell, traeger: traeger) { aufnahmeOffen = false }
            }
        }
    }

    @ViewBuilder
    private func inhalt(_ traeger: Traeger) -> some View {
        List {
            if let dank = modell.confirmation {
                Hinweisstreifen(
                    text: dank, symbol: "checkmark.circle.fill", farbe: Ampel.green.farbe,
                    kennung: "traeger-confirmation"
                )
                .listRowInsets(EdgeInsets())
                .onTapGesture { modell.dismissConfirmation() }
            }

            kopf(traeger)
            mitmachen(traeger)
            arbeitskreise(traeger)
            orteAbschnitt(traeger)
            anfragen(traeger)
        }
        .listStyle(.insetGrouped)
        .refreshable {
            await modell.load()
            if traeger.darfVerwalten { await modell.loadRequests(traeger: traeger.id) }
        }
        .accessibilityIdentifier("traeger-detail")
        .task(id: traeger.darfVerwalten) {
            guard traeger.darfVerwalten else { return }
            await modell.loadRequests(traeger: traeger.id)
        }
    }

    // MARK: Kopf

    @ViewBuilder
    private func kopf(_ traeger: Traeger) -> some View {
        Section {
            VStack(alignment: .leading, spacing: 8) {
                Text(traeger.name)
                    .font(.title2.bold())
                    .accessibilityAddTraits(.isHeader)
                if !traeger.beschreibung.isEmpty {
                    Text(traeger.beschreibung)
                        .font(.subheadline)
                        .fixedSize(horizontal: false, vertical: true)
                }
                if let dach = dachTraeger(traeger) {
                    NavigationLink(value: Ziel.traegerDetail(dach.id)) {
                        Label("Arbeitskreis von \(dach.name)", systemImage: "arrow.turn.left.up")
                            .font(.footnote)
                    }
                    .accessibilityIdentifier("traeger-parent")
                }
                TraegerAbzeichen(traeger: traeger)
            }
            .padding(.vertical, 2)
            .accessibilityIdentifier("traeger-head")
        }

        Section("Wie dieser Träger steht") {
            Label(TraegerTexte.status(traeger.status), systemImage: "checkmark.shield")
                .font(.subheadline)
            Label(TraegerTexte.sichtbarkeit(traeger.sichtbarkeit),
                  systemImage: traeger.istGeschlossen ? "lock.fill" : "globe")
                .font(.subheadline)
        }
    }

    private func dachTraeger(_ traeger: Traeger) -> Traeger? {
        guard traeger.parentId != 0 else { return nil }
        return modell.traeger(id: traeger.parentId)
    }

    // MARK: Mitmachen

    @ViewBuilder
    private func mitmachen(_ traeger: Traeger) -> some View {
        Section {
            if traeger.istMitglied {
                Label("Du gehörst dazu.", systemImage: "checkmark.seal.fill")
                    .font(.subheadline.weight(.medium))
                    .foregroundStyle(Ampel.green.farbe)
                    .accessibilityIdentifier("traeger-i-am-member")
            } else if let stand = TraegerTexte.beitrittStatus(traeger.beitrittStatus),
                      traeger.beitrittStatus == "beantragt" {
                Label(stand, systemImage: "hourglass")
                    .font(.subheadline)
                    .accessibilityIdentifier("traeger-my-request")
            } else if traeger.beitrittMoeglich {
                Button {
                    begruendung = ""
                    mitmachenOffen = true
                } label: {
                    Label("Ich will mitmachen", systemImage: "hand.raised.fill")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .disabled(modell.isBusy(traeger: traeger.id))
                .accessibilityIdentifier("traeger-join")
            } else if !traeger.beitrittHindernis.isEmpty {
                // Der Satz kommt vom Server. Er weiß, warum es nicht geht —
                // die App wüsste es nur ungefähr.
                Label(traeger.beitrittHindernis, systemImage: "info.circle")
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
                    .accessibilityIdentifier("traeger-obstacle")
            }
        } header: {
            Text("Mitmachen")
        }
    }

    // MARK: Arbeitskreise

    @ViewBuilder
    private func arbeitskreise(_ traeger: Traeger) -> some View {
        let kinder = modell.children(of: traeger.id)
        if !kinder.isEmpty {
            Section {
                ForEach(kinder) { kind in
                    TraegerZeile(traeger: kind, unterTraeger: true)
                }
            } header: {
                Text("Arbeitskreise")
            } footer: {
                Text("Jeder Arbeitskreis entscheidet selbst, wer bei ihm mitmacht — "
                    + "eine Mitgliedschaft im Verein ist keine im Arbeitskreis.")
            }
        }
    }

    // MARK: Orte

    /// Der Weg zurück zu dem, was dieser Träger betreut. Die Orte kommen aus
    /// dem gemeinsamen Ortsmodell — dieselbe Liste, die „Mithelfen" zeigt,
    /// und damit auch dieselbe Sichtbarkeit.
    @ViewBuilder
    private func orteAbschnitt(_ traeger: Traeger) -> some View {
        let seine = orte.orte.filter { $0.traegerId == traeger.id }
        if !seine.isEmpty {
            Section {
                ForEach(seine) { ort in
                    NavigationLink(value: Ziel.ort(ort.id)) {
                        HStack(spacing: 12) {
                            Ampelpunkt(ampel: ort.ampel)
                            Text(ort.name).font(.subheadline)
                        }
                    }
                    .accessibilityIdentifier("traeger-place-\(ort.id)")
                }
            } header: {
                Text("Orte dieses Trägers")
            } footer: {
                Text("Was hier ansteht, findest du auch im Bereich Mithelfen.")
            }
        }
    }

    // MARK: Anfragen entscheiden

    @ViewBuilder
    private func anfragen(_ traeger: Traeger) -> some View {
        if traeger.darfVerwalten {
            let liste = modell.requests[traeger.id] ?? []
            let offene = liste.filter { $0.status == "beantragt" }
            Section {
                if offene.isEmpty {
                    Text("Gerade wartet niemand.")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("traeger-requests-empty")
                }
                ForEach(offene) { antrag in
                    AntragZeile(modell: modell, antrag: antrag)
                }
                Button {
                    aufnahmeOffen = true
                } label: {
                    Label("Jemanden direkt aufnehmen", systemImage: "person.badge.plus")
                }
                .accessibilityIdentifier("traeger-add-member")
            } header: {
                Text("Anfragen")
            } footer: {
                Text("Eine Freigabe trägt die Mitgliedschaft in die Rössing-ID ein. "
                    + "Klappt das nicht, bleibt die Anfrage offen — und es steht dabei, warum.")
            }
        }
    }

    private var fehlerOffen: Binding<Bool> {
        Binding(get: { modell.error != nil }, set: { if !$0 { modell.dismissError() } })
    }
}

/// Eine offene Anfrage mit den beiden Knöpfen.
struct AntragZeile: View {
    @ObservedObject var modell: TraegerModel
    let antrag: Beitritt

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(antrag.anzeigename).font(.headline)
            if !antrag.begruendung.isEmpty {
                Text(antrag.begruendung)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .fixedSize(horizontal: false, vertical: true)
            }
            HStack(spacing: 12) {
                Button("Aufnehmen") {
                    Task { await modell.decide(request: antrag, status: "erteilt") }
                }
                .buttonStyle(.borderedProminent)
                .disabled(modell.isBusy(request: antrag.id))
                .accessibilityIdentifier("request-grant-\(antrag.id)")

                Button("Ablehnen", role: .destructive) {
                    Task { await modell.decide(request: antrag, status: "abgelehnt") }
                }
                .buttonStyle(.bordered)
                .disabled(modell.isBusy(request: antrag.id))
                .accessibilityIdentifier("request-reject-\(antrag.id)")

                if modell.isBusy(request: antrag.id) {
                    ProgressView().controlSize(.small)
                }
            }
        }
        .padding(.vertical, 4)
        .accessibilityIdentifier("request-\(antrag.id)")
    }
}

/// „Ich will mitmachen" — mit einem Satz dazu, warum.
struct MitmachenBlatt: View {
    @ObservedObject var modell: TraegerModel
    let traeger: Traeger
    @Binding var begruendung: String
    var schliessen: () -> Void

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text("Schreib dem Vorstand von \(traeger.name) kurz, warum du dabei sein "
                        + "möchtest. Ein Satz reicht — er ist freiwillig.")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                Section {
                    TextField("Zum Beispiel: Ich wohne neben dem Beet und würde gern mitjäten.",
                              text: $begruendung, axis: .vertical)
                        .lineLimit(3 ... 8)
                        .accessibilityIdentifier("join-reason")
                }
                Section {
                    Button {
                        Task {
                            if await modell.join(traeger: traeger.id, reason: begruendung) {
                                schliessen()
                            }
                        }
                    } label: {
                        HStack(spacing: 12) {
                            if modell.isBusy(traeger: traeger.id) {
                                ProgressView()
                            } else {
                                Image(systemName: "paperplane.fill")
                            }
                            Text("Anfrage abschicken")
                        }
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 4)
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(modell.isBusy(traeger: traeger.id))
                    .accessibilityIdentifier("join-send")
                }
                .listRowBackground(Color.clear)
            }
            .navigationTitle("Mitmachen")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Abbrechen") { schliessen() }
                }
            }
        }
    }
}

/// Jemanden ohne Antrag aufnehmen — bei einer geschlossenen Gruppe der
/// einzige Weg hinein.
struct AufnahmeBlatt: View {
    @ObservedObject var modell: TraegerModel
    let traeger: Traeger
    var schliessen: () -> Void

    @State private var suche = ""

    private var treffer: [Dorfbewohner] {
        let begriff = suche.trimmingCharacters(in: .whitespaces).lowercased()
        guard !begriff.isEmpty else { return modell.villagers }
        return modell.villagers.filter {
            TraegerModel.name(of: $0).lowercased().contains(begriff)
        }
    }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    TextField("Name suchen", text: $suche)
                        .textInputAutocapitalization(.words)
                        .autocorrectionDisabled()
                        .accessibilityIdentifier("add-member-search")
                } footer: {
                    Text("Aufgenommen wird sofort: Die Mitgliedschaft wird in die "
                        + "Rössing-ID geschrieben, nicht bloß hier vermerkt.")
                }
                Section {
                    if modell.villagers.isEmpty {
                        Text("Die Dorfbewohner werden geladen …")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                    }
                    ForEach(treffer) { person in
                        Button {
                            Task {
                                await modell.addMember(traeger: traeger.id, person: person)
                                schliessen()
                            }
                        } label: {
                            HStack {
                                Text(TraegerModel.name(of: person))
                                Spacer()
                                Image(systemName: "person.badge.plus").foregroundStyle(.tint)
                            }
                        }
                        .disabled(modell.isBusy(traeger: traeger.id))
                        .accessibilityIdentifier("add-member-\(person.userSub)")
                    }
                }
            }
            .navigationTitle("Aufnehmen")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Abbrechen") { schliessen() }
                }
            }
            .task { await modell.loadVillagers() }
        }
    }
}
