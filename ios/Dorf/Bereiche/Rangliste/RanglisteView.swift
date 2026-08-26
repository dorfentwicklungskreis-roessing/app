import SwiftUI

/// Die Rangliste des Mithelfens: Was das ganze Dorf geschafft hat, wer wie
/// viel beigetragen hat und wo man selbst steht.
///
/// Bewusst ohne Punkte und ohne Abzeichen für Versäumtes — die Liste soll
/// Lust machen, nicht Druck. Die ersten drei bekommen eine Medaille, mehr
/// Podest braucht es nicht.
struct RanglisteView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @StateObject private var modell = RanglisteModell()
    @State private var zeitraum: Zeitraum = .saison
    @State private var erklaerteAuszeichnung: Auszeichnung?

    private var eigene: Ranglistenzeile? { modell.eigeneZeile(meinSub: umgebung.meinSub) }

    var body: some View {
        List {
            zeitraumAbschnitt
            if let hinweis = modell.hinweis { hinweisAbschnitt(hinweis) }
            dorfAbschnitt
            ichAbschnitt
            listenAbschnitt
        }
        .accessibilityIdentifier("rangliste-liste")
        .navigationTitle("Rangliste")
        .refreshable { await modell.laden(api: umgebung.api, zeitraum: zeitraum) }
        // `id:` sorgt für beides: einmal beim Öffnen und noch einmal, sobald
        // jemand den Zeitraum wechselt.
        .task(id: zeitraum) { await modell.laden(api: umgebung.api, zeitraum: zeitraum) }
        .alert(
            erklaerteAuszeichnung?.label ?? "Auszeichnung",
            isPresented: Binding(
                get: { erklaerteAuszeichnung != nil },
                set: { if !$0 { erklaerteAuszeichnung = nil } }
            ),
            presenting: erklaerteAuszeichnung
        ) { _ in
            Button("Alles klar", role: .cancel) {}
        } message: { auszeichnung in
            Text(auszeichnung.description.isEmpty
                 ? "Dazu ist nichts weiter hinterlegt."
                 : auszeichnung.description)
        }
    }

    // MARK: - Zeitraum

    private var zeitraumAbschnitt: some View {
        Section {
            Picker("Zeitraum", selection: $zeitraum) {
                ForEach(Zeitraum.allCases) { einer in
                    Text(einer.titel).tag(einer)
                }
            }
            .pickerStyle(.segmented)
            .accessibilityIdentifier("rangliste-zeitraum")
            .accessibilityLabel("Zeitraum der Rangliste")

            if let text = modell.zeitraumtext {
                Text(text)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity, alignment: .center)
                    .accessibilityIdentifier("rangliste-zeitraumtext")
                    .accessibilityLabel("Ausgewertet wird \(text).")
            }
        }
    }

    /// Der Hinweis steht über den Zahlen, damit niemand einen alten Stand für
    /// den aktuellen hält.
    private func hinweisAbschnitt(_ hinweis: String) -> some View {
        Section {
            Label {
                VStack(alignment: .leading, spacing: 2) {
                    Text(hinweis)
                    if !modell.nochNieGeladen {
                        Text("Angezeigt wird der zuletzt geladene Stand.")
                            .font(.footnote)
                            .foregroundStyle(.secondary)
                    }
                }
            } icon: {
                Image(systemName: "wifi.exclamationmark")
            }
            .accessibilityIdentifier("rangliste-hinweis")
            .accessibilityElement(children: .combine)
        }
    }

    // MARK: - Das ganze Dorf

    private var dorfAbschnitt: some View {
        Section("Das ganze Dorf") {
            let summen = modell.summen
            HStack(alignment: .top, spacing: 0) {
                Kennzahl(wert: "\(summen.completions)", einheit: "Erledigungen")
                Kennzahl(wert: Zahl.liter(summen.liters), einheit: "Liter")
                Kennzahl(wert: "\(summen.participants)", einheit: "Beteiligte")
            }
            .padding(.vertical, 4)
            .accessibilityIdentifier("rangliste-gesamt")
            .accessibilityElement(children: .combine)
            .accessibilityLabel(
                "Das ganze Dorf: \(summen.completions) Erledigungen, "
                + "\(Zahl.liter(summen.liters)) Liter, \(summen.participants) Beteiligte."
            )
        }
    }

    // MARK: - Der eigene Rang

    private var ichAbschnitt: some View {
        Section("Du") {
            VStack(alignment: .leading, spacing: 4) {
                Text(Ranglistentexte.eigenerRang(eigene))
                    .font(.headline)
                if let eigene, eigene.rank > 0 {
                    Text(eigenerNachsatz(eigene))
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            .padding(.vertical, 2)
            .accessibilityIdentifier("rangliste-ich")
            .accessibilityElement(children: .combine)
        }
    }

    private func eigenerNachsatz(_ zeile: Ranglistenzeile) -> String {
        var teile = ["\(zeile.completions) Erledigungen"]
        if zeile.liters > 0 { teile.append("\(Zahl.liter(zeile.liters)) Liter") }
        if let zuletzt = Ranglistentexte.letzteErledigung(zeile.lastCompletion) {
            teile.append("zuletzt \(zuletzt)")
        }
        return teile.joined(separator: " · ")
    }

    // MARK: - Die Liste

    private var listenAbschnitt: some View {
        Section("Wer mitgemacht hat") {
            if modell.nochNieGeladen && modell.laedt {
                HStack(spacing: 10) {
                    ProgressView()
                    Text("Wird geladen …").foregroundStyle(.secondary)
                }
            } else if modell.zeilen.isEmpty {
                Text("Im gewählten Zeitraum hat noch niemand etwas gemeldet. "
                     + "Die erste Meldung darf gern von dir kommen — jede Kanne zählt.")
                    .foregroundStyle(.secondary)
                    .accessibilityIdentifier("rangliste-leer")
            } else {
                ForEach(modell.zeilen) { zeile in
                    Ranglistenzeilenansicht(
                        zeile: zeile,
                        eigen: zeile.istMeine(umgebung.meinSub),
                        aufAuszeichnung: { erklaerteAuszeichnung = $0 }
                    )
                }
            }
        }
    }
}

// MARK: - Bausteine

/// Eine Zahl der Kopfkarte mit ihrer Beschriftung.
private struct Kennzahl: View {
    let wert: String
    let einheit: String

    var body: some View {
        VStack(spacing: 2) {
            Text(wert)
                .font(.title2.bold().monospacedDigit())
            Text(einheit)
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
    }
}

/// Eine Zeile der Rangliste. Eigene Ansicht, damit alle Zeilen gleich
/// aussehen — und damit die eigene sich nur durch Hintergrund und Marke
/// unterscheidet, nicht durch einen anderen Aufbau.
private struct Ranglistenzeilenansicht: View {
    let zeile: Ranglistenzeile
    let eigen: Bool
    let aufAuszeichnung: (Auszeichnung) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            angaben
                .accessibilityElement(children: .combine)
                .accessibilityLabel(Ranglistentexte.vorlesen(zeile, eigen: eigen))
            if !zeile.badges.isEmpty { auszeichnungen }
        }
        .padding(.vertical, 4)
        .listRowBackground(eigen ? Color.accentColor.opacity(0.12) : nil)
        .accessibilityIdentifier("rangliste-zeile-\(zeile.id)")
    }

    private var angaben: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack(alignment: .firstTextBaseline, spacing: 10) {
                rangzeichen
                Text(zeile.userName.isEmpty ? "Ohne Namen" : zeile.userName)
                    .font(eigen ? .headline : .body.weight(.medium))
                if eigen {
                    Text("Du")
                        .font(.caption2.bold())
                        .padding(.horizontal, 7)
                        .padding(.vertical, 2)
                        .background(Capsule().fill(Color.accentColor.opacity(0.25)))
                }
                Spacer(minLength: 8)
                VStack(alignment: .trailing, spacing: 1) {
                    Text("\(zeile.completions)×")
                        .font(.headline.monospacedDigit())
                    if zeile.liters > 0 {
                        Text("\(Zahl.liter(zeile.liters)) Liter")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            let nachArt = zeile.artenText
            if !nachArt.isEmpty {
                Text(nachArt)
                    .font(.footnote)
                    .foregroundStyle(.secondary)
            }
            if let zuletzt = Ranglistentexte.letzteErledigung(zeile.lastCompletion) {
                Text("Zuletzt \(zuletzt)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
        }
    }

    @ViewBuilder private var rangzeichen: some View {
        if let medaille = Ranglistentexte.medaille(zeile.rank) {
            Text(medaille).font(.title3)
        } else {
            Text(zeile.rank > 0 ? "\(zeile.rank)." : "—")
                .font(.subheadline.monospacedDigit())
                .foregroundStyle(.secondary)
                .frame(minWidth: 28, alignment: .trailing)
        }
    }

    /// Kleine Marken. Was eine Auszeichnung bedeutet, steht im Backend —
    /// angetippt erscheint sie im Wortlaut.
    private var auszeichnungen: some View {
        ScrollView(.horizontal) {
            HStack(spacing: 6) {
                ForEach(zeile.badges) { auszeichnung in
                    Button {
                        aufAuszeichnung(auszeichnung)
                    } label: {
                        Text(auszeichnung.label)
                            .font(.caption.weight(.medium))
                            .padding(.horizontal, 9)
                            .padding(.vertical, 4)
                            .background(Capsule().fill(Color.secondary.opacity(0.15)))
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("rangliste-auszeichnung-\(auszeichnung.key)")
                    .accessibilityLabel("Auszeichnung \(auszeichnung.label)")
                    .accessibilityHint("Zeigt, wofür es sie gibt.")
                }
            }
            .padding(.vertical, 1)
        }
        .scrollIndicators(.hidden)
    }
}
