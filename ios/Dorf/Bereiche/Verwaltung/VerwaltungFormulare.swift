import SwiftUI

/// Die Formulare der Verwaltung: ein Ort, eine Aufgabe, der Hitzefaktor.
///
/// Der Zustand liegt im `VerwaltungModell`, nicht in der Ansicht — so lässt
/// er sich ohne Oberfläche prüfen, und ein abgelehnter Vorgang kann das
/// Formular samt allem Getippten stehen lassen.

// MARK: - Bindungen ins Formular

extension VerwaltungModell {
    /// Eine Bindung auf ein Feld des offenen Ortsformulars. Jede Eingabe
    /// räumt zugleich die letzte Ablehnung weg — sie gilt nicht mehr, sobald
    /// jemand etwas ändert.
    func ortBindung<Wert>(_ pfad: WritableKeyPath<OrtFormular, Wert>, vorgabe: Wert) -> Binding<Wert> {
        Binding(
            get: { self.ortFormular?[keyPath: pfad] ?? vorgabe },
            set: { neu in
                self.ortFormular?[keyPath: pfad] = neu
                self.ortFormular?.fehler = nil
            }
        )
    }

    func aufgabeBindung<Wert>(
        _ pfad: WritableKeyPath<AufgabeFormular, Wert>, vorgabe: Wert
    ) -> Binding<Wert> {
        Binding(
            get: { self.aufgabeFormular?[keyPath: pfad] ?? vorgabe },
            set: { neu in
                self.aufgabeFormular?[keyPath: pfad] = neu
                self.aufgabeFormular?.fehler = nil
            }
        )
    }
}

// MARK: - Formular „Ort"

/// Name, Beschreibung, Art, Zustand — und der Standort.
///
/// Die Koordinate kommt entweder vom eigenen Gerät („Ich stehe davor") oder
/// aus einem Tipp auf die Karte. Getippt wird sie nicht: Wer Zahlen abtippt,
/// vertauscht Breite und Länge.
struct OrtFormularView: View {
    let modell: VerwaltungModell
    /// Die vorhandenen Orte — als Orientierung auf der Auswahlkarte.
    let orte: [Ort]

    @State private var standort = Standortgeber()
    @State private var standortHinweis: String?
    @State private var wartetAufFreigabe = false

    var body: some View {
        NavigationStack {
            Form {
                if let fehler = modell.ortFormular?.fehler {
                    Section {
                        Fehlerzeile(text: fehler, kennung: "ort-formular-fehler")
                    }
                }

                Section("Ort") {
                    TextField("Name", text: modell.ortBindung(\.name, vorgabe: ""))
                        .accessibilityIdentifier("ort-name")
                    TextField(
                        "Beschreibung (optional)",
                        text: modell.ortBindung(\.beschreibung, vorgabe: ""),
                        axis: .vertical
                    )
                    .lineLimit(1 ... 4)
                    .accessibilityIdentifier("ort-beschreibung")
                    Picker("Art", selection: modell.ortBindung(\.art, vorgabe: OrtEingabe.blumenkasten)) {
                        ForEach(OrtEingabe.arten, id: \.self) { art in
                            Text(OrtEingabe.bezeichnung(art: art)).tag(art)
                        }
                    }
                    .accessibilityIdentifier("ort-art")
                }

                standortAbschnitt

                Section {
                    Toggle("Ort ist aktiv", isOn: modell.ortBindung(\.aktiv, vorgabe: true))
                        .accessibilityIdentifier("ort-aktiv")
                } footer: {
                    Text(
                        "Pausiert heißt: Hier ist bis auf Weiteres nichts zu tun. "
                            + Verwaltungstexte.hinweisEntfaelltOrt
                    )
                }
            }
            .navigationTitle(modell.ortFormular?.titel ?? "Ort")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Abbrechen") { modell.ortAbbrechen() }
                        .accessibilityIdentifier("ort-abbrechen")
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Speichern") { Task { await modell.ortSpeichern() } }
                        .disabled(!(modell.ortFormular?.speicherbar ?? false))
                        .accessibilityIdentifier("ort-speichern")
                }
            }
            .overlay {
                if modell.ortFormular?.sendet == true {
                    ProgressView().controlSize(.large)
                }
            }
        }
        .task { standort.beobachten() }
        .onDisappear { standort.ruhen() }
        .onChange(of: standort.freigabe) { _, neue in freigabeGeaendert(neue) }
    }

    // MARK: Standort

    @ViewBuilder private var standortAbschnitt: some View {
        Section {
            Text(punkttext)
                .font(.subheadline)
                .foregroundStyle(modell.ortFormular?.punkt == nil ? .secondary : .primary)
                .accessibilityIdentifier("ort-position")

            Button {
                standortUebernehmen()
            } label: {
                Label("Meinen Standort übernehmen", systemImage: "location.fill")
            }
            .accessibilityIdentifier("ort-standort-uebernehmen")

            if let standortHinweis {
                Text(standortHinweis)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("ort-standort-hinweis")
            }

            KarteView(
                orte: orte,
                auswahl: { _ in },
                gewaehlterPunkt: modell.ortFormular?.punkt,
                flaecheGetippt: { punkt in
                    modell.ortFormular?.punkt = punkt
                    modell.ortFormular?.fehler = nil
                    standortHinweis = nil
                }
            )
            .frame(height: 240)
            .listRowInsets(EdgeInsets())
            .accessibilityIdentifier("ort-karte")
        } header: {
            Text("Standort")
        } footer: {
            Text("Tipp auf die Karte, wo der Ort liegt — oder übernimm deinen eigenen Standort.")
        }
    }

    private var punkttext: String {
        guard let punkt = modell.ortFormular?.punkt else { return "Noch kein Standort gewählt" }
        return Standorttext.koordinate(breite: punkt.breite, laenge: punkt.laenge)
    }

    private func standortUebernehmen() {
        switch standort.freigabe {
        case .erlaubt:
            standort.beobachten()
            guard let punkt = standort.letzterPunkt else {
                standortHinweis = "Der Standort steht noch nicht fest. Einen Moment — "
                    + "oder tippe so lange auf die Karte."
                return
            }
            modell.ortFormular?.punkt = punkt
            modell.ortFormular?.fehler = nil
            standortHinweis = nil
        case .ungefragt:
            wartetAufFreigabe = true
            standort.anfragen()
        case .verweigert:
            standortHinweis = "Ohne Freigabe zur Ortung kann die App deinen Standort nicht "
                + "übernehmen. Tippe stattdessen auf die Karte — oder trage die Freigabe in "
                + "den Einstellungen nach."
        }
    }

    private func freigabeGeaendert(_ neue: Standortgeber.Freigabe) {
        guard wartetAufFreigabe else {
            if neue == .erlaubt { standort.beobachten() }
            return
        }
        switch neue {
        case .ungefragt:
            return
        case .erlaubt:
            wartetAufFreigabe = false
            standort.beobachten()
            standortUebernehmen()
        case .verweigert:
            wartetAufFreigabe = false
            standortUebernehmen()
        }
    }
}

// MARK: - Formular „Aufgabe"

/// Art, Titel, Liter (nur beim Gießen) — und dann die Weiche: regelmäßig
/// **oder** einmalig. Beides zusammen bietet das Formular gar nicht erst an;
/// das Backend wiese es ab, und die Ampel wüsste nicht, woran sie sich halten
/// soll.
struct AufgabeFormularView: View {
    let modell: VerwaltungModell

    var body: some View {
        NavigationStack {
            Form {
                if let fehler = modell.aufgabeFormular?.fehler {
                    Section {
                        Fehlerzeile(text: fehler, kennung: "aufgabe-formular-fehler")
                    }
                }

                Section("Aufgabe") {
                    Picker("Art", selection: modell.aufgabeBindung(\.art, vorgabe: AufgabeEingabe.giessen)) {
                        ForEach(AufgabeEingabe.arten, id: \.self) { art in
                            Text(AufgabeEingabe.bezeichnung(art: art)).tag(art)
                        }
                    }
                    .accessibilityIdentifier("aufgabe-art")

                    TextField("Bezeichnung (optional)", text: modell.aufgabeBindung(\.titel, vorgabe: ""))
                        .accessibilityIdentifier("aufgabe-titel")

                    // Liter gibt es nur beim Gießen — „Jäten, 10 Liter" wäre
                    // eine Angabe, die niemand deuten kann.
                    if modell.aufgabeFormular?.literSichtbar == true {
                        TextField("Liter je Durchgang", text: modell.aufgabeBindung(\.liter, vorgabe: ""))
                            .keyboardType(.decimalPad)
                            .accessibilityIdentifier("aufgabe-liter")
                    }
                }

                weiche

                Section {
                    Toggle("Aufgabe ist aktiv", isOn: modell.aufgabeBindung(\.aktiv, vorgabe: true))
                        .accessibilityIdentifier("aufgabe-aktiv")
                } footer: {
                    Text(
                        "Pausiert heißt: Sie wird bis auf Weiteres nicht mehr fällig. "
                            + Verwaltungstexte.hinweisEntfaellt
                    )
                }
            }
            .navigationTitle(modell.aufgabeFormular?.titelzeile ?? "Aufgabe")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Abbrechen") { modell.aufgabeAbbrechen() }
                        .accessibilityIdentifier("aufgabe-abbrechen")
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Speichern") { Task { await modell.aufgabeSpeichern() } }
                        .disabled(!(modell.aufgabeFormular?.speicherbar ?? false))
                        .accessibilityIdentifier("aufgabe-speichern")
                }
            }
            .overlay {
                if modell.aufgabeFormular?.sendet == true {
                    ProgressView().controlSize(.large)
                }
            }
        }
    }

    @ViewBuilder private var weiche: some View {
        let einmalig = modell.aufgabeFormular?.einmalig ?? false
        Section {
            Picker("Wie oft?", selection: modell.aufgabeBindung(\.einmalig, vorgabe: false)) {
                Text("Regelmäßig").tag(false)
                Text("Einmalig").tag(true)
            }
            .pickerStyle(.segmented)
            .accessibilityIdentifier("aufgabe-wiederholung")

            if einmalig {
                DatePicker(
                    "Fällig am",
                    selection: modell.aufgabeBindung(\.termin, vorgabe: Verwaltungsdatum.heute()),
                    displayedComponents: .date
                )
                .environment(\.timeZone, Zeitpunkt.dorfZone)
                .accessibilityIdentifier("aufgabe-termin")

                Toggle(
                    "Nach dem Erledigen abräumen",
                    isOn: modell.aufgabeBindung(\.abraeumenNachErledigung, vorgabe: false)
                )
                .accessibilityIdentifier("aufgabe-abraeumen")
            } else {
                TextField("Intervall (Tage)", text: modell.aufgabeBindung(\.intervall, vorgabe: "7"))
                    .keyboardType(.decimalPad)
                    .accessibilityIdentifier("aufgabe-intervall")
                TextField("Rot nach (Tage)", text: modell.aufgabeBindung(\.rot, vorgabe: "14"))
                    .keyboardType(.decimalPad)
                    .accessibilityIdentifier("aufgabe-rot")
            }
        } header: {
            Text("Wie oft?")
        } footer: {
            Text(einmalig ? Self.hinweisEinmalig : Self.hinweisRegelmaessig)
        }
    }

    static let hinweisEinmalig =
        "Einmalig heißt: ein Termin statt eines Intervalls. Die Ampel wird drei Tage vorher "
            + "gelb und rot, sobald der Termin verstrichen ist. „Abräumen“ nimmt die Aufgabe "
            + "nach dem Erledigen von Karte und Liste — die Erledigungen bleiben in der "
            + "Rangliste."

    static let hinweisRegelmaessig =
        "Nach dem Intervall wird die Aufgabe gelb, nach der Rot-Schwelle rot. Beim Gießen "
            + "beschleunigt der Hitzefaktor beides."
}

// MARK: - Hitzefaktor

/// Der Hitzefaktor: eine tagesaktuelle Einstellung für das ganze Dorf.
struct HitzefaktorAbschnitt: View {
    let modell: VerwaltungModell

    @State private var wert: Double = 1

    var body: some View {
        Section {
            Stepper(value: $wert, in: 0.1 ... 4, step: 0.1) {
                HStack {
                    Text("Faktor")
                    Spacer()
                    Text(Verwaltungszahl.text(gerundet))
                        .monospacedDigit()
                        .foregroundStyle(.secondary)
                }
            }
            .disabled(modell.hitzefaktorLaeuft)
            .accessibilityIdentifier("hitzefaktor-stepper")

            Button("Hitzefaktor speichern") {
                Task { await modell.hitzefaktorSetzen(gerundet) }
            }
            .disabled(modell.hitzefaktorLaeuft || gerundet == modell.hitzefaktor)
            .accessibilityIdentifier("hitzefaktor-speichern")
        } header: {
            Text("Hitzefaktor")
        } footer: {
            Text(Verwaltungstexte.hitzefaktor)
        }
        .task { wert = modell.hitzefaktor }
        .onChange(of: modell.hitzefaktor) { _, neu in wert = neu }
    }

    /// Der Stepper rechnet in Zehnteln; ohne Runden stünde nach ein paar
    /// Schritten „0,7000000000000001" da.
    private var gerundet: Double { (wert * 100).rounded() / 100 }
}

// MARK: - Kleinteile

/// Die Ablehnung des Backends — im Wortlaut, nicht umformuliert.
struct Fehlerzeile: View {
    let text: String
    let kennung: String

    var body: some View {
        Label(text, systemImage: "exclamationmark.triangle.fill")
            .font(.subheadline)
            .foregroundStyle(.red)
            .fixedSize(horizontal: false, vertical: true)
            .accessibilityIdentifier(kennung)
    }
}
