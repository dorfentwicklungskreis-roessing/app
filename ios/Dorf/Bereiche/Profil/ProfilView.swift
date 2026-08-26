import SwiftUI

/// Der Zustand der Profilseite.
///
/// Eigenes Modell, damit die Regeln — was ist geändert, was ist freigegeben,
/// was passiert bei einer Ablehnung — ohne Oberfläche prüfbar sind. Die
/// Speicherfunktion wird von außen hereingereicht: Die Seite kennt das
/// Backend nur als „das hier tut es".
final class Profilmodell: ObservableObject {
    /// Der bearbeitete Stand — daran hängen die Eingabefelder.
    @Published var stand = Profilstand()
    /// Der zuletzt vom Backend bestätigte Stand.
    @Published private(set) var gespeichert = Profilstand()
    @Published private(set) var speichert = false
    /// Begründung einer Ablehnung — im Wortlaut des Backends.
    @Published private(set) var fehler: String?
    @Published private(set) var hinweis: String?

    /// Ob schon einmal etwas hereingereicht wurde. Solange nicht, darf die
    /// Vorbelegung nachgeholt werden, sobald `GET /api/v1/me` da ist.
    @Published private(set) var vorbelegt = false

    /// Der Speichern-Knopf ist nur offen, wenn es etwas zu speichern gibt.
    var hatAenderungen: Bool { stand.geaendert(gegenueber: gespeichert) }
    var kannSpeichern: Bool { hatAenderungen && !speichert }

    /// Vorbelegung aus `GET /api/v1/me`. Ein zweiter Aufruf überschreibt
    /// nichts mehr — sonst verschwände, was gerade getippt wird.
    func vorbelegen(mit ich: Ich?) {
        guard !vorbelegt, let ich else { return }
        stand = Profilstand(ich: ich)
        gespeichert = stand
        vorbelegt = true
    }

    /// Speichert und übernimmt, was das Backend zurückgibt.
    ///
    /// Wird abgelehnt, bleibt das Getippte stehen und die Begründung des
    /// Backends wird wörtlich angezeigt — sie ist genauer als alles, was die
    /// App raten könnte.
    func speichern(
        mit schicken: (ProfilEingabe) async throws -> Profil,
        uebernehmen: (Profil) -> Void = { _ in }
    ) async {
        guard !speichert else { return }
        speichert = true
        fehler = nil
        hinweis = nil
        defer { speichert = false }

        do {
            let profil = try await schicken(stand.alsEingabe)
            stand = Profilstand(profil: profil)
            gespeichert = stand
            vorbelegt = true
            // Damit die Startseite sofort mit dem neuen Namen grüßt.
            uebernehmen(profil)
            hinweis = "Profil gespeichert."
        } catch let abweisung as DorfFehler {
            fehler = abweisung.klartext
        } catch {
            fehler = "Speichern hat nicht geklappt. Besteht eine Verbindung?"
        }
    }
}

/// „Mein Profil": die eigenen Angaben — und je Angabe ein Schalter, wer sie
/// sehen darf.
///
/// Der Hinweis, was andere zu sehen bekommen, steht ganz oben und in voller
/// Größe, nicht im Kleingedruckten (so auch in der Android-App und in
/// `backend/SICHERHEIT.md` festgehalten). Jeder Schalter sagt in Worten, wer
/// das Feld sieht — ein nackter Schalter wäre hier zu wenig.
struct ProfilView: View {
    @EnvironmentObject private var umgebung: AppUmgebung
    @StateObject private var modell = Profilmodell()

    var body: some View {
        Form {
            sichtbarkeitshinweis

            if modell.stand.fuerAndereUnsichtbar {
                Section {
                    Label {
                        Text("""
                        Weder Anzeigename noch Nickname sind freigegeben — \
                        damit tauchst du für die anderen gar nicht in der \
                        Dorfbewohner-Liste auf. Nur die Verwaltung sieht dich.
                        """)
                    } icon: {
                        Image(systemName: "eye.slash")
                    }
                    .accessibilityIdentifier("profil-unsichtbar-hinweis")
                }
            }

            if let fehler = modell.fehler {
                Section {
                    Label {
                        Text(fehler)
                    } icon: {
                        Image(systemName: "exclamationmark.triangle.fill")
                    }
                    .foregroundStyle(.red)
                    .accessibilityIdentifier("profil-fehler")
                } header: {
                    Text("Nicht gespeichert")
                }
            }

            if let hinweis = modell.hinweis {
                Section {
                    Label(hinweis, systemImage: "checkmark.circle.fill")
                        .foregroundStyle(.green)
                        .accessibilityIdentifier("profil-gespeichert")
                }
            }

            Profilfeld(
                kennung: "anzeigename",
                titel: "Anzeigename",
                hinweis: "Vorbelegt aus deiner Rössing-ID — du kannst ihn überschreiben.",
                wert: $modell.stand.anzeigename,
                oeffentlich: $modell.stand.anzeigenameOeffentlich
            )

            Profilfeld(
                kennung: "nickname",
                titel: "Nickname für die Rangliste",
                hinweis: """
                Steht statt des Anzeigenamens in Rangliste und Historie — \
                darunter bist du im Dorf unterwegs. Leer lassen heißt: \
                Anzeigename.
                """,
                wert: $modell.stand.nickname,
                oeffentlich: $modell.stand.nicknameOeffentlich
            )

            Profilfeld(
                kennung: "telefon",
                titel: "Telefon (freiwillig)",
                hinweis: "Nur ausfüllen, wenn du erreichbar sein möchtest.",
                wert: $modell.stand.telefon,
                oeffentlich: $modell.stand.telefonOeffentlich,
                tastatur: .phonePad
            )

            Profilfeld(
                kennung: "email",
                titel: "E-Mail (freiwillig)",
                hinweis: "Vorbelegt aus deiner Rössing-ID — du kannst sie überschreiben.",
                wert: $modell.stand.email,
                oeffentlich: $modell.stand.emailOeffentlich,
                tastatur: .emailAddress,
                grossschreibung: .never
            )

            Profilfeld(
                kennung: "notiz",
                titel: "Notiz (freiwillig)",
                hinweis: "Ein kurzer Hinweis, z.B. „erreichbar abends“.",
                wert: $modell.stand.notiz,
                oeffentlich: $modell.stand.notizOeffentlich,
                mehrzeilig: true
            )

            speichernAbschnitt
        }
        .navigationTitle("Mein Profil")
        .navigationBarTitleDisplayMode(.inline)
        .task {
            if umgebung.ich == nil { await umgebung.ichLaden() }
            modell.vorbelegen(mit: umgebung.ich)
        }
    }

    /// „Das sehen andere" — der Datenschutz-Hinweis über dem Formular, dazu
    /// eine Zeile, die laufend sagt, was gerade tatsächlich freigegeben ist.
    private var sichtbarkeitshinweis: some View {
        Section {
            VStack(alignment: .leading, spacing: 8) {
                Text("Das sehen andere")
                    .font(.headline)
                Text("""
                Alles, was du auf „Sehen alle im Dorf“ stellst, können alle \
                angemeldeten Dorfbewohner lesen — auch Telefonnummer und \
                E-Mail-Adresse. Was auf „Sieht nur die Verwaltung“ steht, \
                bleibt bei den Verwaltenden der Dorf-App.
                """)
                .font(.subheadline)
                Divider()
                Text(freigabezeile)
                    .font(.subheadline.weight(.semibold))
                    .accessibilityIdentifier("profil-sichtbar-jetzt")
            }
            .padding(.vertical, 4)
        }
    }

    private var freigabezeile: String {
        let felder = modell.stand.freigegebeneFelder
        if felder.isEmpty {
            return "Für alle sichtbar: nichts. Nur die Verwaltung sieht deine Angaben."
        }
        return "Für alle sichtbar: \(felder.joined(separator: ", "))."
    }

    private var speichernAbschnitt: some View {
        Section {
            Button {
                Task {
                    await modell.speichern(
                        mit: { try await umgebung.api.profilSpeichern($0) },
                        uebernehmen: { umgebung.profilUebernehmen($0) }
                    )
                }
            } label: {
                HStack(spacing: 8) {
                    if modell.speichert { ProgressView() }
                    Text(modell.speichert ? "Wird gespeichert …" : "Profil speichern")
                }
                .frame(maxWidth: .infinity)
            }
            .disabled(!modell.kannSpeichern)
            .accessibilityIdentifier("profil-speichern")
        } footer: {
            Text(modell.hatAenderungen
                 ? "Gespeichert wird erst, wenn du hier tippst."
                 : "Es gibt gerade nichts zu speichern.")
        }
    }
}

/// Ein Eingabefeld mit seinem Sichtbarkeits-Schalter.
///
/// Der Schalter trägt immer den Satz, wer das Feld sieht. „An" und „aus"
/// allein wäre eine Zumutung: Es geht um Kontaktdaten.
private struct Profilfeld: View {
    let kennung: String
    let titel: String
    let hinweis: String
    @Binding var wert: String
    @Binding var oeffentlich: Bool
    var tastatur: UIKeyboardType = .default
    var grossschreibung: TextInputAutocapitalization = .sentences
    var mehrzeilig: Bool = false

    var body: some View {
        Section {
            TextField(titel, text: $wert, axis: mehrzeilig ? .vertical : .horizontal)
                .keyboardType(tastatur)
                .textInputAutocapitalization(grossschreibung)
                .autocorrectionDisabled(tastatur == .emailAddress)
                .lineLimit(mehrzeilig ? 1 ... 3 : 1 ... 1)
                .accessibilityIdentifier("profil-feld-\(kennung)")
                .accessibilityLabel(titel)

            Toggle(isOn: $oeffentlich) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(oeffentlich ? "Sehen alle im Dorf" : "Sieht nur die Verwaltung")
                    Text(oeffentlich
                         ? "Alle angemeldeten Dorfbewohner können das lesen."
                         : "Niemand außer den Verwaltenden der Dorf-App sieht das.")
                        .font(.footnote)
                        .foregroundStyle(.secondary)
                }
            }
            .accessibilityIdentifier("profil-sicht-\(kennung)")
            .accessibilityLabel("\(titel): für alle Dorfbewohner sichtbar")
        } header: {
            Text(titel)
        } footer: {
            Text(hinweis)
        }
    }
}

#Preview {
    NavigationStack {
        ProfilView()
    }
    .environmentObject(AppUmgebung(
        anmeldung: Anmeldung(),
        api: DorfApi(tokenGeber: { nil }),
        ich: Ich(sub: "abc", name: "Anna Beispiel", email: "anna@example.org")
    ))
}
