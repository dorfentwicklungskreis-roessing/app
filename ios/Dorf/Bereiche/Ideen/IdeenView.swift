import SwiftUI

/// Das Ideen-Formular: ein mehrzeiliges Wunschfeld (Pflicht), darunter Name
/// und E-Mail — beides freiwillig und aus dem Profil vorbelegt. Abgeschickt
/// wird an denselben Eingang wie von der Website; weil hier jemand angemeldet
/// ist, hängt die Idee danach am Konto.
///
/// Fehler kosten nie den getippten Text: Bei einer Ablehnung bleibt alles
/// stehen und die Begründung des Backends steht wörtlich darüber.
struct IdeenView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @StateObject private var modell = IdeenModell()

    var body: some View {
        Form {
            Section {
                VStack(alignment: .leading, spacing: 8) {
                    Text("Deine Idee")
                        .font(.title2.bold())
                        .accessibilityAddTraits(.isHeader)
                    Text("Die App fürs Dorf ist längst nicht fertig — „Mithelfen\" ist nur der Anfang. Was würde dir den Alltag in Rössing leichter machen? Ein Satz reicht.")
                    Text("Trau dich, groß zu denken: Vieles, was nach viel Aufwand klingt, ist gut machbar. Was wir nicht bauen können, sagen wir ehrlich.")
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                }
                .padding(.vertical, 4)
                .accessibilityIdentifier("ideen-einleitung")
            }

            if let fehler = modell.fehler {
                Section {
                    // Farbe allein ist nie die Information: Das Zeichen steht
                    // neben dem Text, nicht an seiner Stelle.
                    Label {
                        Text(fehler).font(.subheadline)
                    } icon: {
                        Image(systemName: "exclamationmark.triangle.fill")
                    }
                    .foregroundStyle(.red)
                    .accessibilityIdentifier("ideen-fehler")
                }
            }

            if modell.dank {
                Section {
                    Label {
                        Text("Danke, deine Idee ist angekommen!").font(.subheadline)
                    } icon: {
                        Image(systemName: "checkmark.circle.fill")
                    }
                    .foregroundStyle(.green)
                    .accessibilityIdentifier("ideen-dank")
                }
            }

            Section {
                TextField(
                    "Zum Beispiel: Ich möchte sehen, wann der nächste Bus fährt.",
                    text: wunschBindung,
                    axis: .vertical
                )
                .lineLimit(5 ... 12)
                .textInputAutocapitalization(.sentences)
                .accessibilityIdentifier("feld-wunsch")
                .accessibilityLabel("Was soll die App können?")

                // Der Zähler ist eine eigene Zeile und damit auch für
                // VoiceOver erreichbar — nicht bloß Zierrat am Feldrand.
                Text(modell.zaehlerText)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .trailing)
                    .listRowSeparator(.hidden)
                    .accessibilityIdentifier("ideen-zaehler")
                    .accessibilityLabel(modell.zaehlerText)
            } header: {
                Text("Was soll die App können?")
            }

            Section {
                TextField("Name (freiwillig)", text: nameBindung)
                    .textContentType(.name)
                    .textInputAutocapitalization(.words)
                    .accessibilityIdentifier("feld-name")

                TextField("E-Mail (freiwillig)", text: emailBindung)
                    .textContentType(.emailAddress)
                    .keyboardType(.emailAddress)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                    .accessibilityIdentifier("feld-email")

                Text("Nur, falls wir bei deiner Idee nachfragen dürfen.")
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            } header: {
                Text("Name und E-Mail — beides freiwillig")
            } footer: {
                Text("Name und E-Mail sind aus deinem Profil vorbelegt — du kannst sie ändern oder leeren.")
            }

            // Der Datenschutzhinweis steht direkt am Formular, nicht im
            // Kleingedruckten — hier wird gerade etwas über eine Person
            // gespeichert.
            Section {
                Label {
                    Text("Gespeichert werden dein Wunsch und, wenn du sie angibst, Name und E-Mail. Sie sind nur für den Dorfentwicklungskreis sichtbar und werden nicht veröffentlicht.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                } icon: {
                    Image(systemName: "info.circle")
                        .foregroundStyle(.secondary)
                }
                .accessibilityIdentifier("ideen-datenschutz")
            }

            Section {
                Button(action: absenden) {
                    HStack(spacing: 12) {
                        if modell.sendet {
                            ProgressView()
                        } else {
                            Image(systemName: "paperplane.fill")
                        }
                        Text(modell.sendet ? "Wird abgeschickt …" : "Idee abschicken")
                    }
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, 4)
                }
                .buttonStyle(.borderedProminent)
                .disabled(!modell.absendbar)
                .accessibilityIdentifier("idee-absenden")
            }
            .listRowBackground(Color.clear)
        }
        .navigationTitle("Idee vorschlagen")
        .navigationBarTitleDisplayMode(.inline)
        .task { modell.vorbelegen(aus: umgebung.ich) }
        // Das Profil kommt womöglich erst nach dem Öffnen an. Nachgetragen
        // wird nur, was noch leer ist (siehe `IdeenModell.vorbelegen`).
        .onChange(of: umgebung.ich?.profile) { _ in
            modell.vorbelegen(aus: umgebung.ich)
        }
    }

    private func absenden() {
        Task { await modell.absenden(ueber: umgebung.api.ideeEinreichen) }
    }

    private var wunschBindung: Binding<String> {
        Binding(get: { modell.wunsch }, set: { modell.setzeWunsch($0) })
    }

    private var nameBindung: Binding<String> {
        Binding(get: { modell.name }, set: { modell.setzeName($0) })
    }

    private var emailBindung: Binding<String> {
        Binding(get: { modell.email }, set: { modell.setzeEmail($0) })
    }
}
#Preview {
    NavigationStack {
        IdeenView()
    }
    .environmentObject(
        AppUmgebung(
            anmeldung: Anmeldung(),
            api: DorfApi(tokenGeber: { .abgemeldet }),
            ich: Ich(sub: "1", name: "Anna Beispiel", email: "anna@example.org")
        )
    )
}
