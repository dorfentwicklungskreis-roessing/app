import SwiftUI

/// The one place in the app that says „das hat nicht geklappt" and offers to
/// do something about it.
///
/// Sits at the root, not inside a single area: a failure can happen anywhere,
/// and a person should never have to find the right screen to be told about
/// it. Two actions, and the important one is a single tap:
///
///   - **Bericht schicken** sends right away. Nobody has to describe
///     anything — that was the whole point.
///   - **Dazuschreiben** opens the sheet for those who want to say what they
///     were doing, and shows exactly what leaves the phone.
///
/// Here also lives the retry that used to sit as its own section on the start
/// page: two boxes about one failure would be one too many, and the one on
/// the start page was blind to everything that goes wrong anywhere else.
/// `erneutVersuchen` is passed in when a reload could actually help — that is
/// what `AppUmgebung.stoerungshinweis` answers.
struct Fehlerbanner: ViewModifier {
    @ObservedObject var melder: ErrorReporter
    /// Noch einmal versuchen — nur gesetzt, wenn ein erneuter Abruf etwas
    /// bringen kann.
    var erneutVersuchen: (() -> Void)?
    @State private var zeigeBlatt = false

    func body(content: Content) -> some View {
        content
            .safeAreaInset(edge: .bottom, spacing: 0) {
                if melder.vorfall != nil {
                    banner
                        .padding(.horizontal, 12)
                        .padding(.bottom, 8)
                        .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
            .animation(.easeInOut(duration: 0.2), value: melder.vorfall)
            .sheet(isPresented: $zeigeBlatt) {
                FehlerberichtBlatt(melder: melder)
            }
    }

    @ViewBuilder private var banner: some View {
        VStack(alignment: .leading, spacing: 8) {
            if melder.gesendet {
                Label {
                    Text("Danke, der Bericht ist angekommen.")
                        .font(.subheadline.weight(.medium))
                } icon: {
                    Image(systemName: "checkmark.circle.fill")
                }
                .accessibilityIdentifier("fehler-banner-dank")
                Button("Schließen") { melder.schliessen() }
                    .buttonStyle(.bordered)
                    .accessibilityIdentifier("fehler-banner-schliessen")
            } else {
                HStack(alignment: .top, spacing: 8) {
                    // Farbe allein ist nie die Information: Das Zeichen steht
                    // neben dem Text, nicht an seiner Stelle.
                    Image(systemName: "exclamationmark.triangle.fill")
                        .foregroundStyle(.orange)
                    Text(melder.vorfall?.message ?? "")
                        .font(.subheadline)
                        .fixedSize(horizontal: false, vertical: true)
                        .accessibilityIdentifier("fehler-banner-meldung")
                    Spacer(minLength: 0)
                    Button {
                        melder.schliessen()
                    } label: {
                        Image(systemName: "xmark")
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("Hinweis schließen")
                    .accessibilityIdentifier("fehler-banner-schliessen")
                }

                if let fehler = melder.sendefehler {
                    Text(fehler)
                        .font(.footnote)
                        .foregroundStyle(.red)
                        .accessibilityIdentifier("fehler-banner-sendefehler")
                }

                if erneutVersuchen != nil, melder.vorfall?.kind == .network {
                    // Wer eben noch angemeldet war, sucht den Fehler sonst bei
                    // sich. Die Anmeldung gilt weiter — das gehört dazu.
                    Text("Du bleibst angemeldet — sobald die Verbindung wieder steht, "
                        + "geht es weiter, wo du warst.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                        .accessibilityIdentifier("fehler-banner-bleibt-angemeldet")
                }

                HStack(spacing: 12) {
                    if let erneutVersuchen {
                        Button("Erneut versuchen") {
                            erneutVersuchen()
                            melder.schliessen()
                        }
                        .buttonStyle(.bordered)
                        .disabled(melder.sendet)
                        .accessibilityIdentifier("fehler-erneut-versuchen")
                    }

                    Button {
                        Task { await melder.absenden() }
                    } label: {
                        HStack(spacing: 8) {
                            if melder.sendet {
                                ProgressView()
                            } else {
                                Image(systemName: "paperplane.fill")
                            }
                            Text(melder.sendet ? "Wird geschickt …" : "Bericht schicken")
                        }
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(melder.sendet)
                    .accessibilityIdentifier("fehler-bericht-schicken")

                    Button("Dazuschreiben") { zeigeBlatt = true }
                        .buttonStyle(.bordered)
                        .disabled(melder.sendet)
                        .accessibilityIdentifier("fehler-bericht-dazuschreiben")
                }
                .font(.subheadline)
            }
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.regularMaterial, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: 14, style: .continuous)
                .stroke(.quaternary, lineWidth: 1)
        )
        .shadow(radius: 6, y: 2)
        .accessibilityIdentifier("fehler-banner")
    }
}

extension View {
    /// Hangs the report banner onto the root of the app.
    func fehlerbanner(_ melder: ErrorReporter,
                      erneutVersuchen: (() -> Void)? = nil) -> some View
    {
        modifier(Fehlerbanner(melder: melder, erneutVersuchen: erneutVersuchen))
    }
}

/// „Dazuschreiben": one voluntary sentence — and, above all, the full list of
/// what leaves the phone. Not a promise about it: the sheet shows the very
/// values the request is built from.
struct FehlerberichtBlatt: View {
    @ObservedObject var melder: ErrorReporter
    @Environment(\.dismiss) private var schliessen
    @State private var kommentar = ""

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text(melder.vorfall?.message ?? "")
                        .font(.subheadline)
                        .accessibilityIdentifier("bericht-meldung")
                } header: {
                    Text("Was passiert ist")
                }

                Section {
                    TextField("Zum Beispiel: Ich wollte gerade das Gießen melden.",
                              text: $kommentar, axis: .vertical)
                        .lineLimit(3 ... 8)
                        .accessibilityIdentifier("feld-kommentar")
                        .accessibilityLabel("Was hast du gerade gemacht?")
                } header: {
                    Text("Was hast du gerade gemacht? — freiwillig")
                } footer: {
                    Text("Ein Fingertipp auf „Abschicken\" hilft auch ohne Text.")
                }

                Section {
                    ForEach(zeilen, id: \.0) { titel, wert in
                        HStack(alignment: .top) {
                            Text(titel)
                                .foregroundStyle(.secondary)
                            Spacer(minLength: 12)
                            Text(wert)
                                .multilineTextAlignment(.trailing)
                        }
                        .font(.footnote)
                    }
                } header: {
                    Text("Das wird geschickt")
                } footer: {
                    Text("Mehr nicht: kein Protokoll, kein Bildschirmfoto, kein Standort und "
                        + "keine Gerätekennung. Dass du es warst, sieht der "
                        + "Dorfentwicklungskreis nur, solange du angemeldet bist.")
                }
                .accessibilityIdentifier("bericht-inhalt")

                if let fehler = melder.sendefehler {
                    Section {
                        Label(fehler, systemImage: "exclamationmark.triangle.fill")
                            .font(.subheadline)
                            .foregroundStyle(.red)
                            .accessibilityIdentifier("bericht-sendefehler")
                    }
                }

                Section {
                    Button {
                        Task {
                            await melder.absenden(kommentar: kommentar)
                            if melder.gesendet { schliessen() }
                        }
                    } label: {
                        HStack(spacing: 12) {
                            if melder.sendet {
                                ProgressView()
                            } else {
                                Image(systemName: "paperplane.fill")
                            }
                            Text(melder.sendet ? "Wird geschickt …" : "Abschicken")
                        }
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 4)
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(melder.sendet)
                    .accessibilityIdentifier("bericht-abschicken")
                }
                .listRowBackground(Color.clear)
            }
            .navigationTitle("Bericht schicken")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Nicht schicken") { schliessen() }
                        .accessibilityIdentifier("bericht-abbrechen")
                }
            }
        }
    }

    private var zeilen: [(String, String)] {
        guard let vorfall = melder.vorfall else { return [] }
        return melder.inhaltsliste(melder.eingabeFuer(vorfall, kommentar: kommentar))
    }
}
