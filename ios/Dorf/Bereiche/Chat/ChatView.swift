import SwiftUI

/// Der Chat: fragen, was man wissen will — in normalem Deutsch.
///
/// Er sieht und darf genau das, was diese Person auch sonst sieht und darf.
/// Das entscheidet das Backend an einer einzigen Stelle; hier wird nur
/// gezeigt, was zurückkommt, und die Begründung im Wortlaut weitergereicht.
struct ChatView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @StateObject private var modell = ChatModell()
    @FocusState private var amTippen: Bool

    var body: some View {
        VStack(spacing: 0) {
            gespraech
            if modell.stand?.verfuegbar == true {
                eingabe
            }
        }
        .navigationTitle("Frag die App")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            if !modell.verlauf.isEmpty {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Neu anfangen", systemImage: "arrow.counterclockwise") {
                        modell.neuAnfangen()
                    }
                    .accessibilityIdentifier("chat-neu")
                }
            }
        }
        .task {
            if modell.stand == nil {
                await modell.standLaden(api: umgebung.api)
            }
        }
    }

    // MARK: Gespräch

    @ViewBuilder private var gespraech: some View {
        ScrollViewReader { blick in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 12) {
                    if modell.stand == nil {
                        ProgressView("Einen Moment …")
                            .frame(maxWidth: .infinity)
                            .padding(.top, 40)
                    } else if !modell.eingerichtet {
                        nichtEingerichtet
                    } else if modell.verlauf.isEmpty {
                        einleitung
                    }
                    ForEach(modell.verlauf) { zug in
                        Blase(zug: zug).id(zug.id)
                    }
                    if modell.laeuft {
                        denktNach.id(denktMarke)
                    }
                    if let hinweis = modell.hinweis, modell.eingerichtet {
                        Label {
                            Text(hinweis).font(.subheadline)
                        } icon: {
                            Image(systemName: "exclamationmark.triangle.fill")
                        }
                        .foregroundStyle(.red)
                        .accessibilityIdentifier("chat-hinweis")
                    }
                }
                .padding()
            }
            .accessibilityIdentifier("chat-verlauf")
            .onChange(of: modell.verlauf.count) { _ in ansEnde(blick) }
            .onChange(of: modell.laeuft) { _ in ansEnde(blick) }
        }
    }

    /// Die Marke, an die nach jedem Zug gescrollt wird.
    private var denktMarke: String { "chat-denkt" }

    private func ansEnde(_ blick: ScrollViewProxy) {
        withAnimation {
            if modell.laeuft {
                blick.scrollTo(denktMarke, anchor: .bottom)
            } else if let letzter = modell.verlauf.last {
                blick.scrollTo(letzter.id, anchor: .bottom)
            }
        }
    }

    private var einleitung: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Frag einfach")
                .font(.title2.bold())
                .accessibilityAddTraits(.isHeader)
            Text("Schreib in normalem Deutsch, was du wissen oder tun willst — was ansteht, wer zuletzt gegossen hat, oder trag gleich eine Erledigung ein.")
            Text("Zum Beispiel: „Was muss diese Woche gegossen werden?“ · „Wer war zuletzt am Kirchplatz?“ · „Ich habe gerade gegossen.“")
                .font(.subheadline)
            Text("Der Chat sieht genau das, was du auch sonst siehst, und tut nur, was du auch sonst tun darfst.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
        .accessibilityIdentifier("chat-einleitung")
    }

    private var nichtEingerichtet: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label {
                Text(modell.stand?.hinweis.isEmpty == false
                    ? modell.stand!.hinweis
                    : "Der Chat ist noch nicht eingerichtet.")
                    .font(.headline)
            } icon: {
                Image(systemName: "bubble.left.and.exclamationmark.bubble.right")
            }
            Text("Sobald der Dorfentwicklungskreis ihn freischaltet, kannst du hier fragen. Alles andere in der App funktioniert weiter.")
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
        .accessibilityIdentifier("chat-nicht-eingerichtet")
    }

    private var denktNach: some View {
        HStack(spacing: 8) {
            ProgressView()
            Text("Ich schaue im Dorf nach …")
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
        .accessibilityIdentifier("chat-laeuft")
    }

    // MARK: Eingabe

    private var eingabe: some View {
        VStack(spacing: 0) {
            Divider()
            HStack(alignment: .bottom, spacing: 8) {
                TextField("Deine Frage …", text: $modell.entwurf, axis: .vertical)
                    .lineLimit(1 ... 5)
                    .textFieldStyle(.plain)
                    .focused($amTippen)
                    .disabled(modell.laeuft)
                    .accessibilityIdentifier("chat-eingabe")
                Button {
                    amTippen = false
                    Task { await modell.fragen(api: umgebung.api) }
                } label: {
                    Image(systemName: "arrow.up.circle.fill")
                        .font(.title2)
                }
                .disabled(!modell.absendbar)
                .accessibilityLabel("Frage abschicken")
                .accessibilityIdentifier("chat-senden")
            }
            .padding(.horizontal)
            .padding(.vertical, 10)
        }
        .background(.bar)
    }
}

/// Ein Zug im Gespräch.
///
/// Unter der Antwort stehen die benutzten Werkzeuge. Das ist keine Spielerei:
/// Wer liest, dass „orte_liste“ befragt wurde, weiß, dass die Zahl aus dem
/// Dorfserver kommt und nicht aus dem Gedächtnis eines Modells.
private struct Blase: View {
    let zug: Gespraechszug

    var body: some View {
        VStack(alignment: zug.vonMir ? .trailing : .leading, spacing: 4) {
            Text(zug.text)
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
                .background(zug.vonMir ? Color.accentColor.opacity(0.15) : Color(.secondarySystemBackground))
                .clipShape(RoundedRectangle(cornerRadius: 14))
                .textSelection(.enabled)
            if zug.abgebrochen {
                Text("Das hat zu lange gedauert — die Antwort ist unvollständig.")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("chat-unvollstaendig")
            }
            if !zug.werkzeuge.isEmpty {
                Text("Nachgesehen in: " + zug.werkzeuge.joined(separator: ", "))
                    .font(.caption2)
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("chat-werkzeuge")
            }
        }
        .frame(maxWidth: .infinity, alignment: zug.vonMir ? .trailing : .leading)
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier(zug.vonMir ? "chat-zug-ich" : "chat-zug-app")
    }
}
