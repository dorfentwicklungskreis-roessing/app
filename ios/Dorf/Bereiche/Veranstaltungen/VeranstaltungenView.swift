import SwiftUI

/// „Was ist los in Rössing" — die kommenden Termine, der nächste zuoberst.
///
/// Die Liste kommt von rössing.de und wird dort gepflegt. Ein Tipp führt
/// dorthin, wo der Termin zu Hause ist: bei einer externen Primärquelle zum
/// Veranstalter, sonst auf die Seite des Dorfes. Doppelt erzählt wird nichts.
struct VeranstaltungenView: View {
    @State private var modell = VeranstaltungenModell()

    var body: some View {
        List {
            // Erst der Hinweis, dann die (womöglich ältere) Liste — eine leere
            // Seite ohne Erklärung wäre das schlechteste Ergebnis.
            if let hinweis = modell.hinweis {
                Section {
                    Hinweiszeile(text: hinweis) {
                        Task { await modell.aktualisieren() }
                    }
                }
            }

            Section {
                Text("Die Termine kommen von rössing.de — gepflegt werden sie dort, "
                    + "damit sie nur an einer Stelle stehen.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .listRowBackground(Color.clear)
            }

            if modell.laedt && modell.termine.isEmpty {
                Section {
                    HStack(spacing: 10) {
                        ProgressView()
                        Text("Termine werden geholt …").foregroundStyle(.secondary)
                    }
                    .accessibilityIdentifier("veranstaltungen-laedt")
                }
            }

            Section {
                ForEach(modell.termine) { termin in
                    TerminZeile(termin: termin)
                }
            }

            if modell.leer {
                Section {
                    Text("Gerade steht kein Termin an. Sobald etwas eingetragen ist, "
                        + "steht es hier.")
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("veranstaltungen-leer")
                }
            }
        }
        .accessibilityIdentifier("veranstaltungen")
        .navigationTitle("Was ist los in Rössing")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await modell.aktualisieren() }
        .task { await modell.laden() }
    }
}

/// Der Hinweis über der Liste — mit dem Weg zurück, nicht nur mit der
/// schlechten Nachricht.
private struct Hinweiszeile: View {
    let text: String
    let erneut: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(text, systemImage: "exclamationmark.triangle.fill")
                .font(.subheadline)
            Button("Erneut versuchen", action: erneut)
                .buttonStyle(.borderless)
                .accessibilityIdentifier("veranstaltungen-erneut")
        }
        .padding(.vertical, 4)
        .accessibilityIdentifier("veranstaltungen-hinweis")
    }
}

/// Eine Zeile der Terminliste. Der ganze Block ist antippbar und für
/// VoiceOver ein einziges Element mit ausformuliertem Datum.
private struct TerminZeile: View {
    let termin: Termin

    var body: some View {
        Gruppierung(ziel: termin.adresse) {
            VStack(alignment: .leading, spacing: 6) {
                HStack(spacing: 6) {
                    Text(termin.datumText)
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(.tint)
                    if termin.extern {
                        Image(systemName: "arrow.up.forward.square")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }

                Text(termin.name).font(.headline)

                // Ganztägig heißt: Es gibt keine Uhrzeit. Dann wird auch keine
                // erfunden, sondern schlicht „Ganztägig" gesagt.
                Angabe(symbol: "clock", text: termin.zeitText ?? "Ganztägig")
                if let ortName = termin.ortName {
                    Angabe(symbol: "mappin.and.ellipse", text: ortName)
                    if let adresse = termin.ortAdresse {
                        Angabe(symbol: nil, text: adresse)
                    }
                }
                if let veranstalter = termin.veranstalter {
                    Angabe(symbol: "person.2", text: veranstalter)
                }

                if !termin.beschreibung.isEmpty {
                    Text(termin.beschreibung)
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .lineLimit(3)
                }
                if termin.extern {
                    Text("Zur Seite des Veranstalters")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
            }
            .padding(.vertical, 4)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(vorlesetext)
        .accessibilityHint(termin.extern
            ? "Öffnet die Seite des Veranstalters."
            : "Öffnet die Seite auf rössing.de.")
        .accessibilityIdentifier("termin-\(termin.id)")
    }

    /// Datum und Uhrzeit ausformuliert; „Mo, 17.08." wäre vorgelesen keine
    /// Angabe, sondern eine Buchstabenfolge.
    private var vorlesetext: String {
        var teile = [termin.name, termin.vorlesetext]
        if let ortName = termin.ortName { teile.append(ortName) }
        if let veranstalter = termin.veranstalter { teile.append("Veranstalter: \(veranstalter)") }
        return teile.joined(separator: ". ")
    }
}

/// Ein Termin ohne brauchbare Adresse bekommt keinen Knopf ins Leere, sondern
/// bleibt schlicht eine Zeile.
private struct Gruppierung<Inhalt: View>: View {
    let ziel: URL?
    @ViewBuilder let inhalt: () -> Inhalt

    var body: some View {
        if let ziel {
            Link(destination: ziel) { inhalt() }
        } else {
            inhalt()
        }
    }
}

/// Eine Angabe mit Symbol — dieselbe Anordnung für Zeit, Ort und
/// Veranstalter, damit die Zeilen nicht jede für sich aussehen.
private struct Angabe: View {
    let symbol: String?
    let text: String

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Group {
                if let symbol {
                    Image(systemName: symbol)
                } else {
                    Color.clear
                }
            }
            .font(.footnote)
            .frame(width: 16, alignment: .center)
            .foregroundStyle(.secondary)
            .accessibilityHidden(true)
            Text(text)
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
    }
}

#Preview {
    NavigationStack {
        VeranstaltungenView()
    }
}
