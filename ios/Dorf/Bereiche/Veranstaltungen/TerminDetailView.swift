import SwiftUI

/// Ein Termin des Dorfes, ausführlich — ohne die App zu verlassen.
///
/// Bewusst nur für **eigene** Termine: Verweist ein Eintrag auf eine fremde
/// Primärquelle, führt der Tipp dorthin. Denselben Inhalt an zwei Stellen zu
/// erzählen heißt, dass eine der beiden irgendwann falsch ist — dieselbe
/// Regel, nach der die Termine überhaupt nur auf rössing.de gepflegt werden.
struct TerminDetailView: View {
    let termin: Termin

    var body: some View {
        List {
            Section {
                VStack(alignment: .leading, spacing: 6) {
                    Text(termin.datumText)
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(.tint)
                    Text(termin.name)
                        .font(.title3.weight(.semibold))
                        .fixedSize(horizontal: false, vertical: true)
                }
                .padding(.vertical, 4)
                .accessibilityElement(children: .combine)
                .accessibilityLabel("\(termin.name). \(termin.vorlesetext)")
            }

            Section("Wann") {
                Beschriftet(symbol: "calendar", titel: "Tag", wert: termin.datumText)
                // Ganztägig heißt: Es gibt keine Uhrzeit — und es wird auch
                // keine erfunden.
                Beschriftet(symbol: "clock", titel: "Uhrzeit",
                            wert: termin.zeitText ?? "Ganztägig")
            }

            if termin.ortName != nil || termin.veranstalter != nil {
                Section("Wo und von wem") {
                    if let ortName = termin.ortName {
                        Beschriftet(symbol: "mappin.and.ellipse", titel: "Ort", wert: ortName)
                    }
                    if let adresse = termin.ortAdresse {
                        Beschriftet(symbol: nil, titel: "Adresse", wert: adresse)
                    }
                    if let veranstalter = termin.veranstalter {
                        Beschriftet(symbol: "person.2", titel: "Veranstalter", wert: veranstalter)
                    }
                }
            }

            if !termin.beschreibung.isEmpty {
                Section("Worum es geht") {
                    Text(termin.beschreibung)
                        .font(.body)
                        .fixedSize(horizontal: false, vertical: true)
                        .padding(.vertical, 2)
                        .accessibilityIdentifier("termin-beschreibung")
                }
            }

            if let adresse = termin.adresse {
                Section {
                    Link(destination: adresse) {
                        Label("Auf rössing.de ansehen", systemImage: "safari")
                    }
                    .accessibilityIdentifier("termin-quelle")
                } footer: {
                    Text("Gepflegt wird der Termin auf rössing.de. Dort steht "
                        + "immer der neueste Stand.")
                }
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle("Termin")
        .navigationBarTitleDisplayMode(.inline)
        .accessibilityIdentifier("termin-detail")
    }
}

/// Eine Zeile „Titel — Wert" mit Symbol, damit Wann und Wo gleich aussehen.
private struct Beschriftet: View {
    let symbol: String?
    let titel: String
    let wert: String

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            Group {
                if let symbol { Image(systemName: symbol) } else { Color.clear }
            }
            .font(.footnote)
            .frame(width: 18)
            .foregroundStyle(.secondary)
            .accessibilityHidden(true)
            Text(titel).foregroundStyle(.secondary)
            Spacer(minLength: 12)
            Text(wert)
                .multilineTextAlignment(.trailing)
                .fixedSize(horizontal: false, vertical: true)
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(titel): \(wert)")
    }
}
