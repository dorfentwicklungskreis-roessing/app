import SwiftUI

/// Die Startseite: Die App ist die Dorf-App, „Mithelfen" ist der erste
/// Bereich. Weitere kommen — deshalb eine Liste von Bereichen und keine
/// Registerkarten, in die sich nichts mehr einfügen ließe.
struct StartView: View {
    @Environment(AppUmgebung.self) private var umgebung
    @State private var pfad = NavigationPath()

    var body: some View {
        NavigationStack(path: $pfad) {
            List {
                Section {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(umgebung.anrede.map { "Moin, \($0)!" } ?? "Moin!")
                            .font(.title2.bold())
                        Text("Deine App fürs Dorf.")
                            .foregroundStyle(.secondary)
                    }
                    .padding(.vertical, 4)
                    .accessibilityIdentifier("start-gruss")
                }

                Section("Bereiche") {
                    Bereichskachel(
                        ziel: .mithelfen, symbol: "leaf.fill", titel: "Mithelfen",
                        untertitel: "Was gerade im Dorf ansteht."
                    )
                    Bereichskachel(
                        ziel: .veranstaltungen, symbol: "calendar", titel: "Was ist los in Rössing",
                        untertitel: "Termine aus dem Dorf."
                    )
                    Bereichskachel(
                        ziel: .rangliste, symbol: "trophy.fill", titel: "Rangliste",
                        untertitel: "Wer wie viel geschafft hat."
                    )
                }

                Section("Du und das Dorf") {
                    Bereichskachel(
                        ziel: .profil, symbol: "person.crop.circle", titel: "Mein Profil",
                        untertitel: "Deine Angaben — und was andere davon sehen."
                    )
                    Bereichskachel(
                        ziel: .dorfbewohner, symbol: "person.2.fill", titel: "Dorfbewohner",
                        untertitel: "Wer mitmacht, mit Kontakt nach Freigabe."
                    )
                }

                Section {
                    Bereichskachel(
                        ziel: .ideen, symbol: "lightbulb.fill", titel: "Idee vorschlagen",
                        untertitel: "Was soll die App noch können?"
                    )
                } header: {
                    Text("Weitere Bereiche kommen")
                } footer: {
                    Text("Was als Nächstes dazukommt, entscheidet ihr — sag uns, was die App können soll.")
                }

                Section {
                    Button("Abmelden", role: .destructive) {
                        umgebung.anmeldung.abmelden()
                    }
                    .accessibilityIdentifier("start-abmelden")
                }

                Section {
                    RechtlichesLeiste()
                        .frame(maxWidth: .infinity)
                        .listRowBackground(Color.clear)
                }
            }
            .navigationTitle("Rössing")
            .dorfZiele()
        }
    }
}

/// Eine Zeile der Bereichsliste. Eigene Ansicht, damit alle Bereiche gleich
/// aussehen und gleich groß antippbar sind.
struct Bereichskachel: View {
    let ziel: Ziel
    let symbol: String
    let titel: String
    let untertitel: String
    /// Optionaler Hinweis rechts („3 Orte warten auf dich").
    var hinweis: String?

    var body: some View {
        NavigationLink(value: ziel) {
            HStack(spacing: 12) {
                Image(systemName: symbol)
                    .font(.title2)
                    .frame(width: 32)
                    .foregroundStyle(.tint)
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 2) {
                    Text(titel).font(.headline)
                    Text(untertitel)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                    if let hinweis {
                        Text(hinweis)
                            .font(.footnote.weight(.medium))
                            .foregroundStyle(.tint)
                    }
                }
            }
            .padding(.vertical, 4)
        }
        .accessibilityIdentifier("bereich-\(titel.lowercased().replacingOccurrences(of: " ", with: "-"))")
    }
}
