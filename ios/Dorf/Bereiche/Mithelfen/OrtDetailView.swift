import SwiftUI

/// Ein Ort mit allem, was dort ansteht: Beschreibung, Aufgaben mit Ampel,
/// Plan bzw. Termin, letzte Erledigung, Historie — und der Knopf zum Melden.
struct OrtDetailView: View {
    @ObservedObject var modell: OrteModell
    let ortId: Int64
    let meinSub: String?

    /// Die Aufgabe, zu der gerade nachgefragt wird. Ein Fehltipp darf nichts
    /// eintragen — deshalb geht keine Meldung ohne diese Rückfrage raus.
    @State private var nachfrage: Aufgabe?
    @State private var ruecknahme: Erledigung?

    private var ort: Ort? { modell.ort(id: ortId) }

    var body: some View {
        Group {
            if let ort {
                inhalt(ort)
            } else {
                Hinweistafel(
                    "Der Ort ist nicht mehr da",
                    symbol: "mappin.slash",
                    beschreibung: "Vielleicht wurde er gerade abgeschaltet oder erledigt."
                )
            }
        }
        .navigationTitle(ort?.name ?? "Ort")
        .navigationBarTitleDisplayMode(.inline)
        // Nachladen, nicht blockieren: Die Seite steht sofort, die Historie
        // kommt hinterher.
        .task(id: ortId) { await historieHolen() }
        .alert(nachfrage?.nachfrageTitel ?? "", isPresented: nachfrageOffen, presenting: nachfrage) { aufgabe in
            Button("Abbrechen", role: .cancel) { nachfrage = nil }
                .accessibilityIdentifier("melden-abbrechen")
            Button("Ja, erledigt") {
                nachfrage = nil
                Task { await modell.melden(aufgabe) }
            }
            .accessibilityIdentifier("melden-bestaetigen")
        } message: { aufgabe in
            Text(nachfragetext(aufgabe))
        }
        .alert("Meldung zurücknehmen?", isPresented: ruecknahmeOffen, presenting: ruecknahme) { erledigung in
            Button("Abbrechen", role: .cancel) { ruecknahme = nil }
            Button("Zurücknehmen", role: .destructive) {
                ruecknahme = nil
                Task { await modell.zuruecknehmen(erledigung) }
            }
            .accessibilityIdentifier("ruecknahme-bestaetigen")
        } message: { erledigung in
            Text(ruecknahmetext(erledigung))
        }
    }

    @ViewBuilder
    private func inhalt(_ ort: Ort) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                Streifen(modell: modell)

                VStack(alignment: .leading, spacing: 8) {
                    Ampelpunkt(ampel: ort.ampel)
                    // Wer sich kümmert. Ohne diese Zeile weiß niemand, an wen
                    // er sich wenden soll — und für einen Ort, den man von
                    // außen sehen, aber nur mit Einweisung anfassen darf, ist
                    // genau das die wichtigste Angabe.
                    if !ort.traegerName.isEmpty {
                        Label(ort.traegerName, systemImage: "person.2")
                            .font(.subheadline)
                            .foregroundStyle(.secondary)
                            .accessibilityLabel("Betreut von \(ort.traegerName)")
                            .accessibilityIdentifier("ort-traeger")
                    }
                    if !ort.description.isEmpty {
                        Text(ort.description)
                            .font(.body)
                            .foregroundStyle(.secondary)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                }
                .padding(.horizontal, 16)

                if !ort.aktiveAufgaben.isEmpty {
                    HelferKarte(modell: modell, ort: ort)
                        .padding(.horizontal, 16)
                }

                if ort.aktiveAufgaben.isEmpty {
                    Text("An diesem Ort ist gerade keine Aufgabe eingetragen.")
                        .foregroundStyle(.secondary)
                        .padding(.horizontal, 16)
                } else {
                    ForEach(ort.aktiveAufgaben) { aufgabe in
                        AufgabenKarte(
                            modell: modell,
                            aufgabe: aufgabe,
                            meinSub: meinSub,
                            nachfragen: { nachfrage = $0 },
                            ruecknahmeFragen: { ruecknahme = $0 }
                        )
                        .padding(.horizontal, 16)
                    }
                }
            }
            .padding(.vertical, 12)
        }
        .refreshable {
            await modell.laden()
            await historieHolen()
        }
        .accessibilityIdentifier("ort-detail-\(ort.id)")
    }

    private func historieHolen() async {
        guard let ort else { return }
        for aufgabe in ort.aktiveAufgaben {
            await modell.historieLaden(aufgabe: aufgabe.id)
        }
    }

    /// Ort und Menge stehen in der Rückfrage — damit klar ist, was gleich
    /// eingetragen wird.
    private func nachfragetext(_ aufgabe: Aufgabe) -> String {
        var zeilen = ["Ort: \(ort?.name ?? "")"]
        if let menge = aufgabe.liters { zeilen.append("Menge: \(Zahl.liter(menge)) Liter") }
        zeilen.append("")
        zeilen.append("Die Meldung erscheint mit deinem Namen in der Historie.")
        return zeilen.joined(separator: "\n")
    }

    private func ruecknahmetext(_ erledigung: Erledigung) -> String {
        guard let wann = erledigung.zeitpunkt else {
            return "Deine Meldung wird gelöscht. Die Ampel rechnet danach wieder ohne sie."
        }
        return "Deine Meldung von \(Zeitpunkt.mitUhrzeit(wann)) wird gelöscht. "
            + "Die Ampel rechnet danach wieder ohne sie."
    }

    private var nachfrageOffen: Binding<Bool> {
        Binding(get: { nachfrage != nil }, set: { if !$0 { nachfrage = nil } })
    }

    private var ruecknahmeOffen: Binding<Bool> {
        Binding(get: { ruecknahme != nil }, set: { if !$0 { ruecknahme = nil } })
    }
}

/// Der Eintrag als Helferin oder Helfer für diesen Ort.
///
/// Wer hier zusagt, wird gefragt, sobald etwas ansteht — der Reihe nach,
/// nicht alle auf einmal. Wann und wen, entscheidet das Backend; die App
/// meldet nur an und ab.
struct HelferKarte: View {
    @ObservedObject var modell: OrteModell
    let ort: Ort

    /// Wofür ich mithelfen will. Nur vor dem Anmelden zu wählen — zum
    /// Wechseln erst abmelden, sonst stünden zwei Anmeldungen nebeneinander.
    @State private var wahl: Helferwahl = .alles

    private var laeuft: Bool { modell.laeuftGerade(ort: ort.id) }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Label(ort.helfeIchMit ? "Du hilfst hier mit" : "Ich helfe hier mit",
                  systemImage: ort.helfeIchMit ? "hands.clap.fill" : "hand.raised")
                .font(.headline)

            Text(erklaerung)
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .fixedSize(horizontal: false, vertical: true)

            if ort.helferArten.count > 1 && !ort.helfeIchMit {
                Picker("Wobei möchtest du helfen?", selection: $wahl) {
                    Text(Helferwahl.alles.titel).tag(Helferwahl.alles)
                    ForEach(ort.helferArten, id: \.self) { art in
                        Text(Helferwahl.name(art)).tag(Helferwahl.art(art))
                    }
                }
                .pickerStyle(.segmented)
                .disabled(laeuft)
                .accessibilityIdentifier("helfer-auswahl-\(ort.id)")
            }

            knopf

            if ort.helferzahl > 0 {
                Text(ort.helferzahl == 1
                    ? "Eine Person hilft hier mit."
                    : "\(ort.helferzahl) helfen hier mit.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("helferzahl-\(ort.id)")
            }
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .fill(Color(uiColor: .secondarySystemGroupedBackground))
        )
        .accessibilityIdentifier("helfer-karte-\(ort.id)")
    }

    private var erklaerung: String {
        guard ort.helfeIchMit else {
            return "Dann wirst du gefragt, sobald hier etwas ansteht — der Reihe nach, "
                + "nicht alle auf einmal. Zusagen musst du trotzdem nie."
        }
        switch ort.meineHelferwahl {
        case .alles: return "Du wirst gefragt, wenn hier etwas ansteht."
        case .art(let kind):
            return "Du wirst gefragt, wenn hier \(Helferwahl.name(kind)) ansteht."
        }
    }

    @ViewBuilder
    private var knopf: some View {
        if ort.helfeIchMit {
            Button {
                Task { await modell.abmelden(ort: ort.id) }
            } label: {
                HStack(spacing: 8) {
                    if laeuft { ProgressView().controlSize(.small) }
                    Text("Ich mag nicht mehr")
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
            .disabled(laeuft)
            .accessibilityIdentifier("abmelden-\(ort.id)")
        } else {
            Button {
                Task { await modell.anmelden(ort: ort.id, art: wahl.taskKind) }
            } label: {
                HStack(spacing: 8) {
                    if laeuft { ProgressView().controlSize(.small) }
                    Text("Ich helfe hier mit")
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .disabled(laeuft)
            .accessibilityIdentifier("anmelden-\(ort.id)")
        }
    }
}

/// Eine Aufgabe des Ortes.
struct AufgabenKarte: View {
    @ObservedObject var modell: OrteModell
    let aufgabe: Aufgabe
    let meinSub: String?
    var nachfragen: (Aufgabe) -> Void
    var ruecknahmeFragen: (Erledigung) -> Void

    private var laeuft: Bool { modell.laeuftGerade(aufgabe.id) }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(alignment: .firstTextBaseline) {
                Text(aufgabe.anzeigename).font(.headline)
                Spacer(minLength: 8)
                Ampelpunkt(ampel: aufgabe.ampel, art: aufgabe.kind)
            }

            Text(aufgabe.planText)
                .font(.subheadline)
                .foregroundStyle(.secondary)

            Text(aufgabe.letzteMeldungText)
                .font(.footnote)
                .foregroundStyle(.secondary)
                .accessibilityIdentifier("aufgabe-zuletzt-\(aufgabe.id)")

            vergabestand

            knopf

            if let eigene = aufgabe.eigeneLetzteMeldung(meinSub) {
                Button("Meldung zurücknehmen") { ruecknahmeFragen(eigene) }
                    .font(.footnote)
                    .buttonStyle(.borderless)
                    .disabled(laeuft)
                    .accessibilityIdentifier("zuruecknehmen-\(aufgabe.id)")
            }

            historie
        }
        .padding(16)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .fill(Color(uiColor: .secondarySystemGroupedBackground))
        )
        .accessibilityIdentifier("aufgabe-\(aufgabe.id)")
    }

    /// Wer hier zugesagt hat und wie viele mithelfen.
    ///
    /// Steht eine Zusage, ist das die wichtigere Nachricht: Wer sie gegeben
    /// hat, weiß, dass er dran ist; alle anderen wissen, dass es keine
    /// zweite braucht. Die Zahl der Helfenden steht darunter.
    @ViewBuilder
    private var vergabestand: some View {
        if let text = aufgabe.zusagetext(meinSub: meinSub) {
            Label(text, systemImage: aufgabe.zusagesymbol(meinSub: meinSub))
                .font(.subheadline)
                .foregroundStyle(aufgabe.assignment?.vonMir(meinSub) == true
                    ? Ampel.green.farbe : .secondary)
                .fixedSize(horizontal: false, vertical: true)
                .accessibilityIdentifier("vergabe-\(aufgabe.id)")
        }
        if let helfer = aufgabe.helfertext {
            Text(helfer)
                .font(.footnote)
                .foregroundStyle(.secondary)
                .accessibilityIdentifier("helfer-\(aufgabe.id)")
        }
    }

    /// Melden, gesperrt oder gar nicht — die Entscheidung steckt in
    /// `Aufgabe.meldeknopf`, damit sie prüfbar ist.
    @ViewBuilder
    private var knopf: some View {
        switch aufgabe.meldeknopf() {
        case .keiner:
            // Einmalig ist einmalig. Statt eines Knopfes, der in ein 409
            // läuft, steht hier, dass es getan ist.
            Label("Erledigt — diese Aufgabe war einmalig.", systemImage: "checkmark.seal.fill")
                .font(.subheadline)
                .foregroundStyle(Ampel.green.farbe)
                .accessibilityIdentifier("aufgabe-einmalig-erledigt-\(aufgabe.id)")

        case .gesperrt(let bis):
            Button {} label: {
                Label("Wieder ab \(Zeitpunkt.mitUhrzeit(bis))", systemImage: "clock")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
            .disabled(true)
            .accessibilityIdentifier("melden-\(aufgabe.id)")
            .accessibilityLabel("Gesperrt. Wieder ab \(Zeitpunkt.mitUhrzeit(bis))")

        case .bereit(let titel):
            Button { nachfragen(aufgabe) } label: {
                HStack(spacing: 8) {
                    if laeuft { ProgressView().controlSize(.small) }
                    Text(titel)
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .disabled(laeuft)
            .accessibilityIdentifier("melden-\(aufgabe.id)")
        }
    }

    @ViewBuilder
    private var historie: some View {
        let eintraege = modell.historie[aufgabe.id] ?? []
        if !eintraege.isEmpty {
            Divider()
            Text("Historie")
                .font(.footnote.weight(.semibold))
            VStack(alignment: .leading, spacing: 2) {
                ForEach(eintraege.prefix(5)) { eintrag in
                    Text(historienzeile(eintrag))
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            .accessibilityIdentifier("historie-\(aufgabe.id)")
        }
    }

    private func historienzeile(_ eintrag: Erledigung) -> String {
        let name = eintrag.userName.isEmpty ? "jemand" : eintrag.userName
        let wann = eintrag.zeitpunkt.map(Zeitpunkt.mitUhrzeit) ?? eintrag.doneAt
        guard let menge = eintrag.liters else { return "\(wann) — \(name)" }
        return "\(wann) — \(name) (\(Zahl.liter(menge)) Liter)"
    }
}
