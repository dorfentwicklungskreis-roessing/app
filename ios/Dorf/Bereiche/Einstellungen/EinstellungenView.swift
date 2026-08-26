import SwiftUI

/// Einstellungen: das eigene Konto, der Weg hinaus und was die App über sich
/// selbst zu sagen hat.
///
/// Der Bereich ist kurz, weil er kurz sein soll — er sammelt genau das, was
/// nirgends sonst hingehört. Sein wichtigster Eintrag ist „Konto löschen":
/// Apples Richtlinie 5.1.1 (v) verlangt von jeder App mit Konto einen Weg
/// dorthin **in der App**, nicht per E-Mail an irgendwen.
struct EinstellungenView: View {
    @Environment(AppUmgebung.self) private var umgebung
    @State private var modell: KontoModell?

    var body: some View {
        List {
            KontoAbschnitt(umgebung: umgebung)

            Section {
                Bereichskachel(
                    ziel: .profil, symbol: "person.crop.circle", titel: "Mein Profil",
                    untertitel: "Deine Angaben — und was andere davon sehen."
                )
            } header: {
                Text("Deine Angaben")
            } footer: {
                Text("Was von deinem Profil für andere sichtbar ist, entscheidest du dort — Feld für Feld.")
            }

            Section {
                Button("Abmelden", role: .destructive) {
                    umgebung.anmeldung.abmelden()
                }
                .accessibilityIdentifier("einstellungen-abmelden")
            } footer: {
                Text("Abmelden löscht nichts — beim nächsten Anmelden ist alles wieder da.")
            }

            if let modell {
                LoeschAbschnitt(modell: modell)
            }

            UeberDieAppAbschnitt()
        }
        .navigationTitle("Einstellungen")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            if modell == nil { modell = KontoModell(umgebung: umgebung) }
        }
    }
}

/// „Mein Konto": wer angemeldet ist und mit welcher Rolle.
private struct KontoAbschnitt: View {
    let umgebung: AppUmgebung

    var body: some View {
        Section("Mein Konto") {
            LabeledContent("Angemeldet als") {
                Text(umgebung.anrede ?? "unbekannt")
                    .accessibilityIdentifier("konto-name")
            }
            if let mail = umgebung.ich?.email, !mail.isEmpty {
                LabeledContent("E-Mail", value: mail)
            }
            LabeledContent("Rolle") {
                Text(umgebung.binAdmin ? "Verwaltung" : "Mitglied")
                    .accessibilityIdentifier("konto-rolle")
            }
            if umgebung.anmeldung.sitzung == .angemeldet(entwicklerModus: true) {
                Text("Entwickler-Anmeldung (ohne Rössing-ID)")
                    .font(.footnote)
                    .foregroundStyle(.orange)
            }
        }
    }
}

/// Der Weg zum Löschen des Kontos — mit der Erklärung davor, nicht danach.
private struct LoeschAbschnitt: View {
    @Bindable var modell: KontoModell

    var body: some View {
        Section {
            Button("Konto löschen", role: .destructive) {
                modell.rueckfrageOeffnen()
            }
            .accessibilityIdentifier("konto-loeschen")
        } header: {
            Text("Konto löschen")
        } footer: {
            VStack(alignment: .leading, spacing: 6) {
                Text("Dein Profil, deine Anmeldungen zum Mithelfen, deine Anfragen und die Kennung dieses Geräts werden gelöscht.")
                Text("Deine Meldungen bleiben anonym stehen („Ehemaliges Mitglied“) — an ihnen hängen die Zahlen des ganzen Dorfes.")
                Text("Deine Rössing-ID bleibt bestehen. Sie gehört nicht der Dorf-App, sondern ist die gemeinsame Anmeldung fürs Dorf.")
            }
            .accessibilityIdentifier("konto-loeschen-erklaerung")
        }
        .sheet(isPresented: rueckfrage) {
            KontoLoeschenBlatt(modell: modell)
        }
        .alert("Das hat nicht geklappt", isPresented: fehlerOffen) {
            Button("Ok", role: .cancel) { modell.fehlerVerwerfen() }
                .accessibilityIdentifier("konto-fehler-ok")
        } message: {
            // Wortlaut des Backends. Und: Angemeldet ist man weiterhin.
            Text((modell.fehler ?? "") + "\n\nDu bist weiterhin angemeldet — dein Konto ist unverändert.")
        }
    }

    private var rueckfrage: Binding<Bool> {
        Binding(get: { modell.rueckfrageOffen },
                set: { if !$0 { modell.rueckfrageSchliessen() } })
    }

    private var fehlerOffen: Binding<Bool> {
        Binding(get: { modell.fehler != nil }, set: { if !$0 { modell.fehlerVerwerfen() } })
    }
}

/// Die Rückfrage. Bewusst ein eigenes Blatt und kein Hinweisfenster: Was hier
/// steht, passt in keine drei Zeilen — und der Name will abgetippt werden.
private struct KontoLoeschenBlatt: View {
    @Bindable var modell: KontoModell
    @Environment(\.dismiss) private var schliessen

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text("Das lässt sich nicht rückgängig machen.")
                        .font(.headline)
                }

                Section("Was gelöscht wird") {
                    Punkt("Dein Profil samt Kontaktdaten und Notiz")
                    Punkt("Deine Anmeldungen zum Mithelfen")
                    Punkt("Deine Anfragen und Hinweise")
                    Punkt("Die Kennung dieses Geräts — Push hört sofort auf")
                }

                Section("Was bleibt") {
                    Punkt("Deine Meldungen — ohne deinen Namen, als „Ehemaliges Mitglied“. "
                        + "An ihnen hängen die Gesamtsummen des Dorfes und die Geschichte der Orte; "
                        + "sie zu löschen würde die Arbeit aller anderen verfälschen.")
                    Punkt("Deine Rössing-ID. Sie gehört nicht der Dorf-App, sondern ist die gemeinsame "
                        + "Anmeldung fürs Dorf und wird auch von anderen Anwendungen benutzt. "
                        + "Wenn du auch sie löschen möchtest, wende dich an die Rössing-ID.")
                    Link("Zur Rössing-ID", destination: URL(string: "https://id.xn--rssing-wxa.de")!)
                        .accessibilityIdentifier("konto-roessing-id")
                }

                Section {
                    TextField(modell.bestaetigungswort, text: $modell.getippterName)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .accessibilityIdentifier("konto-bestaetigung")
                } header: {
                    Text("Zum Bestätigen abtippen")
                } footer: {
                    Text("Schreib „\(modell.bestaetigungswort)“ in das Feld. Damit das nicht aus Versehen passiert.")
                }

                Section {
                    Button(role: .destructive) {
                        Task { await modell.loeschen() }
                    } label: {
                        if modell.laeuft {
                            ProgressView()
                        } else {
                            Text("Konto endgültig löschen")
                        }
                    }
                    .disabled(!modell.darfLoeschen)
                    .accessibilityIdentifier("konto-loeschen-endgueltig")
                }
            }
            .navigationTitle("Konto löschen")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Abbrechen") { schliessen() }
                        .accessibilityIdentifier("konto-abbrechen")
                }
            }
        }
    }

    private struct Punkt: View {
        let text: String
        init(_ text: String) { self.text = text }

        var body: some View {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text("•").accessibilityHidden(true)
                Text(text)
            }
            .font(.subheadline)
        }
    }
}

/// „Über die App": Version und Build kommen aus dem Bundle — abgeschriebene
/// Versionsnummern im Quelltext laufen der Wirklichkeit hinterher.
private struct UeberDieAppAbschnitt: View {
    var body: some View {
        Section("Über die App") {
            LabeledContent("Version", value: Appversion.anzeige)
                .accessibilityIdentifier("app-version")
            RechtlichesLeiste()
                .frame(maxWidth: .infinity)
                .listRowBackground(Color.clear)
        }
    }
}

/// Version und Build aus der `Info.plist`.
enum Appversion {
    static var version: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "?"
    }

    static var build: String {
        Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? "?"
    }

    /// „0.1.0 (12)" — die Build-Nummer gehört dazu: Ohne sie lässt sich in
    /// TestFlight nicht sagen, welche Fassung jemand vor sich hat.
    static var anzeige: String { "\(version) (\(build))" }
}
