import SwiftUI

/// Der Bereich „Verwaltung": Orte und Aufgaben pflegen — am Blumenkasten
/// stehend statt später am Rechner.
///
/// Sichtbar nur für Verwaltende (`umgebung.binAdmin`). Das ist eine
/// Höflichkeit, keine Sicherung: Durchgesetzt wird die Regel im Backend, das
/// jede Änderung ohne die Rolle `admin` mit 403 abweist. Kommt trotzdem
/// jemand hierher, steht hier der Satz des Backends — nicht ein erfundener.
struct VerwaltungView: View {
    @Environment(AppUmgebung.self) private var umgebung
    /// Die Ortsliste kommt aus demselben Modell wie „Mithelfen" — ein
    /// zweiter Abruf wäre eine zweite Wahrheit.
    @State private var orte: OrteModell?
    @State private var modell: VerwaltungModell?

    var body: some View {
        Group {
            if !umgebung.binAdmin {
                ContentUnavailableView(
                    "Nur für die Verwaltung",
                    systemImage: "lock",
                    description: Text(
                        "Orte und Aufgaben pflegt die Verwaltung des Dorfes. "
                            + "Melden und Mithelfen kannst du in „Mithelfen“."
                    )
                )
                .accessibilityIdentifier("verwaltung-gesperrt")
            } else if let orte, let modell {
                VerwaltungInhalt(orte: orte, modell: modell)
            } else {
                ProgressView().controlSize(.large)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle("Verwaltung")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            guard umgebung.binAdmin else { return }
            let liste = orte ?? OrteModell(api: umgebung.api)
            orte = liste
            let vorhanden = modell ?? VerwaltungModell(quelle: .vom(
                DorfApi.verwaltung(tokenGeber: { [anmeldung = umgebung.anmeldung] in
                    await anmeldung.frischesToken()
                }),
                neuLaden: { await liste.laden() }
            ))
            modell = vorhanden
            await liste.laden()
            await vorhanden.einstellungenLaden()
        }
    }
}

// MARK: - Inhalt

struct VerwaltungInhalt: View {
    let orte: OrteModell
    let modell: VerwaltungModell

    @State private var rueckfrage: Rueckfrage?

    var body: some View {
        List {
            Section {
                Text(
                    "Hier legst du an, was im Dorf zu tun ist — am besten dort, wo es zu tun "
                        + "ist: Standort übernehmen, Aufgabe eintragen, fertig."
                )
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .accessibilityIdentifier("verwaltung-einleitung")
            }

            HitzefaktorAbschnitt(modell: modell)

            Section {
                Button {
                    modell.ortBearbeiten(nil)
                } label: {
                    Label("Neuen Ort anlegen", systemImage: "plus.circle.fill")
                }
                .accessibilityIdentifier("verwaltung-ort-neu")
            }

            if orte.orte.isEmpty {
                Section {
                    if orte.laeuft && !orte.jeGeladen {
                        HStack { Spacer(); ProgressView(); Spacer() }
                    } else {
                        Text("Noch kein Ort angelegt.")
                            .foregroundStyle(.secondary)
                            .accessibilityIdentifier("verwaltung-keine-orte")
                    }
                }
            }

            ForEach(orte.nachDringlichkeit) { ort in
                OrtAbschnitt(ort: ort, modell: modell) { rueckfrage = $0 }
            }
        }
        .listStyle(.insetGrouped)
        .refreshable {
            await orte.laden()
            await modell.einstellungenLaden()
        }
        .safeAreaInset(edge: .top, spacing: 0) { streifen }
        .sheet(isPresented: ortFormularOffen) {
            OrtFormularView(modell: modell, orte: orte.orte)
        }
        .sheet(isPresented: aufgabeFormularOffen) {
            AufgabeFormularView(modell: modell)
        }
        .alert(rueckfrage?.titel ?? "", isPresented: rueckfrageOffen, presenting: rueckfrage) { frage in
            Button(frage.knopf, role: .destructive) {
                let aktion = frage
                rueckfrage = nil
                Task { await ausfuehren(aktion) }
            }
            .accessibilityIdentifier("verwaltung-rueckfrage-ja")
            Button("Abbrechen", role: .cancel) { rueckfrage = nil }
        } message: { frage in
            Text(frage.text)
        }
        .alert("Das hat nicht geklappt", isPresented: fehlerOffen) {
            Button("Ok", role: .cancel) { modell.fehlerVerwerfen() }
                .accessibilityIdentifier("verwaltung-fehler-ok")
        } message: {
            Text(modell.fehler ?? "")
        }
    }

    // MARK: Streifen

    @ViewBuilder private var streifen: some View {
        VStack(spacing: 0) {
            if let hinweis = orte.hinweis {
                Hinweisstreifen(
                    text: hinweis, symbol: "wifi.slash", farbe: .orange,
                    kennung: "verwaltung-netzhinweis"
                )
            }
            if let bestaetigung = modell.bestaetigung {
                Hinweisstreifen(
                    text: bestaetigung, symbol: "checkmark.circle.fill",
                    farbe: Ampel.green.farbe, kennung: "verwaltung-bestaetigung"
                )
                .onTapGesture { modell.bestaetigungVerwerfen() }
            }
        }
    }

    // MARK: Rückfragen ausführen

    private func ausfuehren(_ frage: Rueckfrage) async {
        switch frage {
        case .ortLoeschen(let ort): await modell.ortLoeschen(ort)
        case .aufgabeLoeschen(let aufgabe): await modell.aufgabeLoeschen(aufgabe)
        case .ortPausieren(let ort): await modell.ortUmschalten(ort, aktiv: false)
        case .aufgabePausieren(let aufgabe): await modell.aufgabeUmschalten(aufgabe, aktiv: false)
        }
    }

    // MARK: Bindungen

    private var ortFormularOffen: Binding<Bool> {
        Binding(get: { modell.ortFormular != nil }, set: { if !$0 { modell.ortAbbrechen() } })
    }

    private var aufgabeFormularOffen: Binding<Bool> {
        Binding(get: { modell.aufgabeFormular != nil }, set: { if !$0 { modell.aufgabeAbbrechen() } })
    }

    private var rueckfrageOffen: Binding<Bool> {
        Binding(get: { rueckfrage != nil }, set: { if !$0 { rueckfrage = nil } })
    }

    private var fehlerOffen: Binding<Bool> {
        Binding(get: { modell.fehler != nil }, set: { if !$0 { modell.fehlerVerwerfen() } })
    }
}

// MARK: - Rückfrage

/// Gelöscht und pausiert wird nie mit einem Fingertipp, sondern erst nach
/// einer Rückfrage — und die sagt auch, was daran hängt: An einer zugesagten
/// Aufgabe hängt ein Mensch, der sonst umsonst losgeht.
enum Rueckfrage: Identifiable, Hashable {
    case ortLoeschen(Ort)
    case aufgabeLoeschen(Aufgabe)
    case ortPausieren(Ort)
    case aufgabePausieren(Aufgabe)

    var id: String {
        switch self {
        case .ortLoeschen(let ort): return "ort-loeschen-\(ort.id)"
        case .aufgabeLoeschen(let aufgabe): return "aufgabe-loeschen-\(aufgabe.id)"
        case .ortPausieren(let ort): return "ort-pausieren-\(ort.id)"
        case .aufgabePausieren(let aufgabe): return "aufgabe-pausieren-\(aufgabe.id)"
        }
    }

    var titel: String {
        switch self {
        case .ortLoeschen, .aufgabeLoeschen: return "Wirklich löschen?"
        case .ortPausieren, .aufgabePausieren: return "Wirklich pausieren?"
        }
    }

    var text: String {
        switch self {
        case .ortLoeschen(let ort): return Verwaltungstexte.ortLoeschen(ort)
        case .aufgabeLoeschen(let aufgabe): return Verwaltungstexte.aufgabeLoeschen(aufgabe)
        case .ortPausieren(let ort): return Verwaltungstexte.ortPausieren(ort)
        case .aufgabePausieren(let aufgabe): return Verwaltungstexte.aufgabePausieren(aufgabe)
        }
    }

    var knopf: String {
        switch self {
        case .ortLoeschen, .aufgabeLoeschen: return "Ja, löschen"
        case .ortPausieren, .aufgabePausieren: return "Ja, pausieren"
        }
    }
}

// MARK: - Ein Ort mit seinen Aufgaben

struct OrtAbschnitt: View {
    let ort: Ort
    let modell: VerwaltungModell
    var frage: (Rueckfrage) -> Void

    var body: some View {
        Section {
            VStack(alignment: .leading, spacing: 8) {
                Text(zustandstext)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                if !ort.description.isEmpty {
                    Text(ort.description).font(.subheadline)
                }
                Text(Standorttext.koordinate(breite: ort.lat, laenge: ort.lon))
                    .font(.caption)
                    .foregroundStyle(.tertiary)
                Ampelpunkt(ampel: ort.ampel)
            }
            .padding(.vertical, 2)
            .accessibilityElement(children: .combine)

            Werkzeugzeile(
                laeuft: modell.laeuftGerade(ort.id),
                aktiv: ort.active,
                kennung: "ort-\(ort.id)",
                bearbeiten: { modell.ortBearbeiten(ort) },
                umschalten: {
                    if ort.active {
                        frage(.ortPausieren(ort))
                    } else {
                        Task { await modell.ortUmschalten(ort, aktiv: true) }
                    }
                },
                loeschen: { frage(.ortLoeschen(ort)) }
            )

            ForEach(ort.tasks) { aufgabe in
                AufgabeZeile(ort: ort, aufgabe: aufgabe, modell: modell, frage: frage)
            }

            Button {
                modell.aufgabeBearbeiten(ort: ort.id, aufgabe: nil)
            } label: {
                Label("Aufgabe hinzufügen", systemImage: "plus")
            }
            .accessibilityIdentifier("aufgabe-neu-\(ort.id)")
        } header: {
            Text(ort.name)
        }
    }

    private var zustandstext: String {
        let art = OrtEingabe.bezeichnung(art: ort.kind)
        return ort.active ? "\(art) · aktiv" : "\(art) · pausiert"
    }
}

// MARK: - Eine Aufgabe

struct AufgabeZeile: View {
    let ort: Ort
    let aufgabe: Aufgabe
    let modell: VerwaltungModell
    var frage: (Rueckfrage) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(aufgabe.anzeigename).font(.headline)
            Text(aufgabe.planText)
                .font(.subheadline)
                .foregroundStyle(.secondary)
                .accessibilityIdentifier("aufgabe-plan-\(aufgabe.id)")
            Text(aufgabe.active ? "läuft" : "pausiert")
                .font(.caption)
                .foregroundStyle(aufgabe.active ? Color.secondary : Color.orange)

            Werkzeugzeile(
                laeuft: modell.laeuftGerade(aufgabe.id),
                aktiv: aufgabe.active,
                kennung: "aufgabe-\(aufgabe.id)",
                bearbeiten: { modell.aufgabeBearbeiten(ort: ort.id, aufgabe: aufgabe) },
                umschalten: {
                    if aufgabe.active {
                        frage(.aufgabePausieren(aufgabe))
                    } else {
                        Task { await modell.aufgabeUmschalten(aufgabe, aktiv: true) }
                    }
                },
                loeschen: { frage(.aufgabeLoeschen(aufgabe)) }
            )
        }
        .padding(.vertical, 2)
    }
}

/// Bearbeiten · Pausieren/Fortsetzen · Löschen — für Orte und Aufgaben
/// dieselbe Zeile, damit dasselbe überall gleich aussieht und gleich groß
/// antippbar ist.
struct Werkzeugzeile: View {
    let laeuft: Bool
    let aktiv: Bool
    let kennung: String
    var bearbeiten: () -> Void
    var umschalten: () -> Void
    var loeschen: () -> Void

    var body: some View {
        HStack(spacing: 16) {
            Button("Bearbeiten", action: bearbeiten)
                .accessibilityIdentifier("\(kennung)-bearbeiten")
            Button(aktiv ? "Pausieren" : "Fortsetzen", action: umschalten)
                .accessibilityIdentifier("\(kennung)-pausieren")
            Spacer(minLength: 0)
            if laeuft {
                ProgressView().controlSize(.small)
            }
            Button("Löschen", role: .destructive, action: loeschen)
                .accessibilityIdentifier("\(kennung)-loeschen")
        }
        .font(.subheadline.weight(.medium))
        // Ohne diesen Stil macht die Liste aus der ganzen Zeile einen einzigen
        // Knopf — dann löschte ein Tipp auf „Bearbeiten“.
        .buttonStyle(.borderless)
        .disabled(laeuft)
        .padding(.top, 2)
    }
}

/// Koordinaten in der Anzeige — fünf Nachkommastellen sind etwa ein Meter,
/// mehr wäre Schein-Genauigkeit.
enum Standorttext {
    static func koordinate(breite: Double, laenge: Double) -> String {
        String(format: "%.5f, %.5f", breite, laenge)
    }
}
