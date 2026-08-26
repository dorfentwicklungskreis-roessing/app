import SwiftUI

/// „Was ist los in Rössing" — die kommenden Termine, der nächste zuoberst.
///
/// Die Liste kommt von rössing.de und wird dort gepflegt. Ein Tipp führt
/// dorthin, wo der Termin zu Hause ist: bei einer externen Primärquelle zum
/// Veranstalter, sonst auf die Seite des Dorfes. Doppelt erzählt wird nichts.
struct VeranstaltungenView: View {
    @StateObject private var modell = VeranstaltungenModell()

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
        Gruppierung(termin: termin) {
            HStack(alignment: .top, spacing: 14) {
                Datumsmarke(termin: termin)

                VStack(alignment: .leading, spacing: 5) {
                    Text(termin.name)
                        .font(.headline)
                        .foregroundStyle(.primary)
                        .fixedSize(horizontal: false, vertical: true)

                    // Ganztägig heißt: Es gibt keine Uhrzeit. Dann wird auch
                    // keine erfunden, sondern schlicht „Ganztägig" gesagt.
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
                            .lineLimit(2)
                            .padding(.top, 1)
                    }
                    if termin.extern, let gastgeber = termin.veranstalter ?? termin.ortName {
                        Text("Mehr bei \(gastgeber)")
                            .font(.caption)
                            .foregroundStyle(.tint)
                            .padding(.top, 1)
                    }
                }

                Spacer(minLength: 0)

                // Ein Termin, der nach draußen führt, sagt das mit dem Symbol,
                // das iOS dafür benutzt — nicht mit einer anderen Textfarbe.
                Image(systemName: termin.extern ? "arrow.up.forward.app" : "chevron.right")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(.tertiary)
                    .padding(.top, 3)
            }
            .padding(.vertical, 4)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(vorlesetext)
        .accessibilityHint(termin.extern
            ? "Öffnet die Seite des Veranstalters."
            : "Zeigt den Termin ausführlich.")
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

/// Datum als kleine Marke statt als grüner Fließtext: Wochentag, Tag, Monat
/// untereinander. Das ist die einzige Stelle der Zeile, die Farbe trägt — so
/// findet das Auge beim Blättern die Daten, ohne dass alles bunt ist.
private struct Datumsmarke: View {
    let termin: Termin

    private static func teil(_ muster: String, _ datum: Date) -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "de_DE")
        f.timeZone = Zeitpunkt.dorfZone
        f.dateFormat = muster
        return f.string(from: datum)
    }

    var body: some View {
        VStack(spacing: 1) {
            Text(Self.teil("EEEEEE", termin.beginn).uppercased())
                .font(.caption2.weight(.semibold))
                .foregroundStyle(.secondary)
            Text(Self.teil("d", termin.beginn))
                .font(.title2.weight(.bold))
                .foregroundStyle(.tint)
            Text(Self.teil("MMM", termin.beginn))
                .font(.caption2)
                .foregroundStyle(.secondary)
        }
        .frame(width: 46)
        .padding(.vertical, 8)
        .background(Color.accentColor.opacity(0.10), in: RoundedRectangle(cornerRadius: 10))
        .accessibilityHidden(true)
    }
}

/// Wohin ein Tipp führt: bei einer fremden Primärquelle nach draußen, sonst
/// auf die eigene Detailseite.
private struct Gruppierung<Inhalt: View>: View {
    let termin: Termin
    @ViewBuilder let inhalt: () -> Inhalt

    var body: some View {
        if termin.extern, let ziel = termin.adresse {
            // Fremde Primärquelle: Der Tipp führt dorthin, wo der Termin zu
            // Hause ist. Ihn hier nachzuerzählen hieße, eine zweite Fassung in
            // die Welt zu setzen, die irgendwann von der ersten abweicht.
            //
            // .plain ist dabei der springende Punkt: Ohne ihn färbt Link
            // seinen *gesamten* Inhalt mit der Akzentfarbe — Titel, Ort,
            // Veranstalter, alles grün.
            Link(destination: ziel) { inhalt() }
                .buttonStyle(.plain)
        } else {
            // Termin des Dorfes: Alles, was wir dazu haben, steht schon in der
            // Datei. Dafür muss niemand die App verlassen.
            NavigationLink { TerminDetailView(termin: termin) } label: { inhalt() }
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
