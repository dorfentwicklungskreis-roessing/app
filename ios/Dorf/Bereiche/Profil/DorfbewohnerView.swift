import Observation
import SwiftUI

/// Eine Person, so wie sie in dieser Liste erscheinen darf.
///
/// `restricted` schickt das Backend nur an Verwaltende. Die Kennzeichnung
/// hängt hier trotzdem zusätzlich an `adminView`: Steht die Liste nicht in
/// der Verwaltungs-Sicht, wird nichts als „nur für die Verwaltung sichtbar"
/// ausgewiesen — es gäbe dort auch nichts zu kennzeichnen.
struct Bewohneransicht: Identifiable, Sendable {
    let person: Dorfbewohner
    let verwaltungssicht: Bool

    var id: String { person.id }

    func nurFuerVerwaltung(_ feld: String) -> Bool {
        verwaltungssicht && person.nurFuerVerwaltung(feld)
    }
}

/// Der Zustand der Dorfbewohner-Liste.
@Observable
final class Dorfbewohnermodell {
    private(set) var bewohner: [Dorfbewohner] = []
    /// Kam die Liste in der Verwaltungs-Sicht (`adminView`)?
    private(set) var verwaltungssicht = false
    private(set) var laedt = false
    private(set) var geladen = false
    private(set) var fehler: String?
    var suche = ""

    /// Gesucht wird über die Namen — der angezeigte Name, der Anzeigename und
    /// der Nickname. Andere Angaben bleiben außen vor: Wer eine Rufnummer
    /// eingibt, sucht nicht nach einem Menschen.
    var gefiltert: [Bewohneransicht] {
        let begriff = suche.trimmingCharacters(in: .whitespacesAndNewlines)
        let treffer = begriff.isEmpty ? bewohner : bewohner.filter { person in
            [person.name, person.displayName, person.nickname]
                .contains { $0.localizedCaseInsensitiveContains(begriff) }
        }
        return treffer.map { Bewohneransicht(person: $0, verwaltungssicht: verwaltungssicht) }
    }

    func laden(mit holen: () async throws -> DorfbewohnerAntwort) async {
        laedt = true
        defer { laedt = false }
        do {
            let antwort = try await holen()
            bewohner = antwort.members
            verwaltungssicht = antwort.adminView
            geladen = true
            fehler = nil
        } catch let abweisung as DorfFehler {
            fehler = abweisung.klartext
        } catch {
            fehler = "Die Liste konnte nicht geladen werden."
        }
    }
}

/// „Dorfbewohner": wer mitmacht — mit genau den Angaben, die freigegeben
/// wurden. Was hier fehlt, hat die Person nicht freigegeben; das Backend
/// schickt es gar nicht erst mit.
struct DorfbewohnerView: View {
    @Environment(AppUmgebung.self) private var umgebung
    @State private var modell = Dorfbewohnermodell()

    var body: some View {
        @Bindable var modell = modell

        List {
            if let fehler = modell.fehler {
                Section {
                    Label(fehler, systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(.red)
                        .accessibilityIdentifier("dorfbewohner-fehler")
                }
            }

            if modell.verwaltungssicht {
                Section {
                    Label {
                        Text("""
                        Du siehst als Verwaltung auch Angaben, die nicht \
                        freigegeben sind. Sie sind gekennzeichnet — behandle \
                        sie entsprechend.
                        """)
                    } icon: {
                        Image(systemName: "lock.shield")
                    }
                    .accessibilityIdentifier("dorfbewohner-verwaltungshinweis")
                }
            }

            ForEach(modell.gefiltert) { ansicht in
                Section {
                    Bewohnerzeile(ansicht: ansicht)
                }
            }
        }
        .listStyle(.insetGrouped)
        .navigationTitle("Dorfbewohner")
        .navigationBarTitleDisplayMode(.inline)
        .searchable(text: $modell.suche, prompt: "Nach Namen suchen")
        .overlay { leerhinweis }
        .refreshable { await laden() }
        .task { if !modell.geladen { await laden() } }
    }

    private func laden() async {
        await modell.laden(mit: { try await umgebung.api.dorfbewohner() })
    }

    /// Eine leere Liste bekommt einen Satz, keine leere Fläche.
    @ViewBuilder private var leerhinweis: some View {
        if modell.laedt && modell.bewohner.isEmpty {
            ProgressView("Wird geladen …")
        } else if modell.gefiltert.isEmpty && !modell.suche.isEmpty {
            Hinweistafel.suche(text: modell.suche)
        } else if modell.gefiltert.isEmpty && modell.fehler == nil {
            Hinweistafel(
                "Noch niemand dabei",
                symbol: "person.2",
                beschreibung: """
                Bisher hat niemand Angaben freigegeben. Wer in „Mein Profil“ \
                Anzeigenamen oder Nickname freigibt, steht hier.
                """
            )
            .accessibilityIdentifier("dorfbewohner-leer")
        }
    }
}

/// Eine Person: Name, darunter die freigegebenen Angaben. Rufnummer und
/// E-Mail sind antippbar.
private struct Bewohnerzeile: View {
    let ansicht: Bewohneransicht

    private var person: Dorfbewohner { ansicht.person }

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 8) {
                Text(person.name)
                    .font(.headline)
                if ansicht.nurFuerVerwaltung("displayName") && person.nickname.isEmpty {
                    NurVerwaltungsmarke()
                }
            }

            // Der Anzeigename steht nur dann noch einmal da, wenn der Name
            // oben der Nickname ist — sonst stünde er zweimal.
            if !person.nickname.isEmpty, !person.displayName.isEmpty,
               person.nickname != person.displayName {
                Angabe(
                    symbol: "person",
                    text: person.displayName,
                    nurVerwaltung: ansicht.nurFuerVerwaltung("displayName")
                )
            }

            if !person.phone.isEmpty {
                HStack(spacing: 8) {
                    if let adresse = Kontakt.telefon(person.phone) {
                        Link(destination: adresse) {
                            Label(person.phone, systemImage: "phone.fill")
                        }
                        .accessibilityIdentifier("bewohner-anrufen-\(person.userSub)")
                        .accessibilityLabel("\(person.name) anrufen: \(person.phone)")
                    } else {
                        Label(person.phone, systemImage: "phone.fill")
                    }
                    if ansicht.nurFuerVerwaltung("phone") { NurVerwaltungsmarke() }
                }
            }

            if !person.email.isEmpty {
                HStack(spacing: 8) {
                    if let adresse = Kontakt.mail(person.email) {
                        Link(destination: adresse) {
                            Label(person.email, systemImage: "envelope.fill")
                        }
                        .accessibilityIdentifier("bewohner-mailen-\(person.userSub)")
                        .accessibilityLabel("\(person.name) eine E-Mail schreiben: \(person.email)")
                    } else {
                        Label(person.email, systemImage: "envelope.fill")
                    }
                    if ansicht.nurFuerVerwaltung("email") { NurVerwaltungsmarke() }
                }
            }

            if !person.note.isEmpty {
                Angabe(
                    symbol: "text.bubble",
                    text: person.note,
                    nurVerwaltung: ansicht.nurFuerVerwaltung("note")
                )
            }
        }
        .padding(.vertical, 4)
        .accessibilityIdentifier("bewohner-\(person.userSub)")
    }
}

/// Eine schlichte Angabe mit Symbol, ggf. gekennzeichnet.
private struct Angabe: View {
    let symbol: String
    let text: String
    let nurVerwaltung: Bool

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 8) {
            Label(text, systemImage: symbol)
                .font(.subheadline)
                .foregroundStyle(.secondary)
            if nurVerwaltung { NurVerwaltungsmarke() }
        }
    }
}

/// Kennzeichnet, was die Person **nicht** freigegeben hat und was hier nur
/// steht, weil die Liste in der Verwaltungs-Sicht kam. Die Farbe allein trägt
/// die Information nicht — der Satz steht daneben.
private struct NurVerwaltungsmarke: View {
    var body: some View {
        Text("nur für die Verwaltung sichtbar")
            .font(.caption2.weight(.semibold))
            .padding(.horizontal, 6)
            .padding(.vertical, 2)
            .background(Color.red.opacity(0.12), in: Capsule())
            .foregroundStyle(.red)
            .accessibilityIdentifier("nur-verwaltung")
    }
}

#Preview {
    NavigationStack {
        DorfbewohnerView()
    }
    .environment(AppUmgebung(
        anmeldung: Anmeldung(),
        api: DorfApi(tokenGeber: { nil }),
        ich: Ich(sub: "abc", name: "Anna Beispiel")
    ))
}
