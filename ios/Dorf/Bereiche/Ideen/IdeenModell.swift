import Combine
import Foundation

/// Zustand des Ideen-Formulars — dasselbe Verhalten wie im
/// `IdeenViewModel` der Android-App.
///
/// Der Wunsch ist Pflicht, Name und E-Mail sind freiwillig und aus dem
/// Profil vorbelegt. Der Missbrauchsschutz (Zugriffsgrenze, verstecktes
/// Feld, Mindestzeit) sitzt im Backend und wird hier bewusst **nicht**
/// nachgebaut: Für Website und App sollen dieselben Regeln gelten.
final class IdeenModell: ObservableObject {
    /// Kürzer ergibt keinen Wunsch. Verbindlich prüft das Backend
    /// (5–2000 Zeichen) — hier geht es nur darum, niemanden mit einer
    /// vermeidbaren Fehlermeldung zu ärgern.
    static let mindestZeichen = 5
    static let maxZeichen = 2000

    @Published private(set) var wunsch = ""
    @Published private(set) var name = ""
    @Published private(set) var email = ""
    @Published private(set) var sendet = false
    /// Begründung des Backends im Wortlaut, wenn die Einreichung abgewiesen
    /// wurde — sonst nil.
    @Published private(set) var fehler: String?
    /// Nach dem Abschicken: „Danke, deine Idee ist angekommen!"
    @Published private(set) var dank = false

    init() {}

    // MARK: Eingabe

    /// Der Zeichenzähler zählt, was auch die Leserin zählt: Buchstaben, nicht
    /// Speicherplatz. „ä" ist eins, ein Emoji ist eins — Swift zählt
    /// Graphem-Cluster, und das ist hier genau richtig.
    var zeichen: Int { wunsch.count }

    var zaehlerText: String { "\(zeichen) von \(Self.maxZeichen) Zeichen" }

    /// Der Knopf ist erst brauchbar, wenn wirklich etwas dasteht.
    var absendbar: Bool {
        !sendet && wunsch.trimmingCharacters(in: .whitespacesAndNewlines).count >= Self.mindestZeichen
    }

    func setzeWunsch(_ neu: String) {
        wunsch = String(neu.prefix(Self.maxZeichen))
        getipptWirdWieder()
    }

    func setzeName(_ neu: String) {
        name = neu
        getipptWirdWieder()
    }

    func setzeEmail(_ neu: String) {
        email = neu
        getipptWirdWieder()
    }

    /// Wer weitertippt, hat die alte Meldung gelesen.
    private func getipptWirdWieder() {
        fehler = nil
        dank = false
    }

    /// Belegt Name und E-Mail aus dem Profil vor — aber nur, was noch leer
    /// ist. Wer schon getippt hat, soll seine Eingabe nicht verlieren, bloß
    /// weil das Profil eine Sekunde später geladen wurde. Ohne Profil (und
    /// ohne `ich`) bleibt schlicht alles leer; beides ist freiwillig.
    func vorbelegen(aus ich: Ich?) {
        if name.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            name = Self.ersterInhalt(ich?.profile?.displayName, ich?.name)
        }
        if email.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            email = Self.ersterInhalt(ich?.profile?.email, ich?.email)
        }
    }

    private static func ersterInhalt(_ kandidaten: String?...) -> String {
        kandidaten
            .compactMap { $0 }
            .first { !$0.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty } ?? ""
    }

    // MARK: Absenden

    /// Schickt den Wunsch ab. Übergeben wird der Weg zum Backend
    /// (`api.ideeEinreichen`), damit der Zustand ohne Netz prüfbar bleibt.
    ///
    /// Ein Fehler kostet nie den getippten Text: Bei einer Ablehnung bleibt
    /// alles stehen, und die Begründung des Backends steht wörtlich darüber.
    /// Nach dem Erfolg wird **nur** das Wunschfeld frei — Name und E-Mail
    /// bleiben, damit die nächste Idee ohne Tipparbeit hineinpasst.
    func absenden(ueber einreichen: (IdeeEingabe) async throws -> Idee) async {
        guard absendbar else { return }
        sendet = true
        fehler = nil
        dank = false
        let eingabe = IdeeEingabe(
            wunsch: wunsch.trimmingCharacters(in: .whitespacesAndNewlines),
            name: name.trimmingCharacters(in: .whitespacesAndNewlines),
            email: email.trimmingCharacters(in: .whitespacesAndNewlines)
        )
        do {
            _ = try await einreichen(eingabe)
            wunsch = ""
            fehler = nil
            dank = true
        } catch {
            fehler = Self.fehlertext(error)
        }
        sendet = false
    }

    /// Die Begründung des Backends ist genauer als alles, was die App raten
    /// könnte — sie wird wörtlich übernommen.
    static func fehlertext(_ fehler: Error) -> String {
        guard let dorf = fehler as? DorfFehler else { return nichtGeklappt }
        switch dorf {
        case .abgelehnt(let grund): return grund
        case .zuVieleAnfragen: return zuVieleIdeen
        case .netz: return nichtGeklappt
        default: return dorf.klartext
        }
    }

    static let zuVieleIdeen =
        "Das waren gerade viele Ideen auf einmal. Bitte in einer Stunde noch einmal versuchen."
    static let nichtGeklappt = "Das Abschicken hat nicht geklappt. Besteht eine Verbindung?"
}
