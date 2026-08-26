import Foundation
import Observation

/// Der Stand des Bereichs „Verwaltung": welches Formular offen ist, was
/// gerade geschickt wird, was das Backend zuletzt abgelehnt hat.
///
/// Die **Liste** der Orte kommt aus `OrteModell` — dasselbe Modell, das
/// „Mithelfen" benutzt. Es gibt keinen zweiten Abruf und keine zweite
/// Wahrheit: Nach jeder Änderung wird dort neu geladen, und was dann
/// dasteht, ist der Stand des Backends.
///
/// **Das Backend entscheidet.** Ob jemand verwalten darf (403), ob eine
/// einmalige Aufgabe ein Datum hat, ob ein Intervall brauchbar ist — all das
/// wird dort geprüft. Hier wird kein Grund erfunden und keine Prüfung
/// nachgebaut; die Oberfläche baut den unmöglichen Fall nur gar nicht erst
/// (entweder Intervall oder Termin), damit niemand in eine vermeidbare
/// Ablehnung läuft.

// MARK: - Quelle

/// Ein Bündel Verschlüsse statt eines Protokolls — wie `OrteQuelle`. Ein Test
/// füllt es selbst und geht damit nie ins Netz.
struct VerwaltungQuelle {
    var ortAnlegen: @MainActor (OrtEingabe) async throws -> Ort
    var ortAendern: @MainActor (Int64, OrtEingabe) async throws -> Ort
    var ortLoeschen: @MainActor (Int64) async throws -> Void
    var aufgabeAnlegen: @MainActor (Int64, AufgabeEingabe) async throws -> Aufgabe
    var aufgabeAendern: @MainActor (Int64, AufgabeEingabe) async throws -> Aufgabe
    var aufgabeLoeschen: @MainActor (Int64) async throws -> Void
    var einstellungen: @MainActor () async throws -> Einstellungen
    var hitzefaktorSetzen: @MainActor (Double) async throws -> Einstellungen
    /// Die Ortsliste neu holen — nach jeder angenommenen Änderung.
    var neuLaden: @MainActor () async -> Void

    static func vom(
        _ zugang: DorfApi.Verwaltung,
        neuLaden: @escaping @MainActor () async -> Void
    ) -> VerwaltungQuelle {
        VerwaltungQuelle(
            ortAnlegen: { try await zugang.ortAnlegen($0) },
            ortAendern: { try await zugang.ortAendern(id: $0, $1) },
            ortLoeschen: { try await zugang.ortLoeschen(id: $0) },
            aufgabeAnlegen: { try await zugang.aufgabeAnlegen(ort: $0, $1) },
            aufgabeAendern: { try await zugang.aufgabeAendern(id: $0, $1) },
            aufgabeLoeschen: { try await zugang.aufgabeLoeschen(id: $0) },
            einstellungen: { try await zugang.einstellungen() },
            hitzefaktorSetzen: { try await zugang.hitzefaktorSetzen($0) },
            neuLaden: neuLaden
        )
    }
}

// MARK: - Zahlen und Daten

/// Getipptes in eine Zahl — mit Komma wie mit Punkt, denn auf einer deutschen
/// Tastatur steht das Komma.
enum Verwaltungszahl {
    static func wert(_ text: String) -> Double? {
        let sauber = text.trimmingCharacters(in: .whitespaces).replacingOccurrences(of: ",", with: ".")
        guard let zahl = Double(sauber), zahl.isFinite else { return nil }
        return zahl
    }

    /// „7" statt „7,0" — für die Vorbelegung eines Feldes.
    static func text(_ wert: Double) -> String {
        if wert == wert.rounded(), abs(wert) < 1e15 { return String(Int64(wert)) }
        return String(wert).replacingOccurrences(of: ".", with: ",")
    }
}

/// Das Fälligkeitsdatum einer einmaligen Aufgabe.
///
/// Gerechnet wird in **Ortszeit des Dorfes**. Das Backend legt ein reines
/// Datum als „23:59:59 Ortszeit" ab (`ParseTermin`); in UTC gelesen wäre der
/// 20.08. schnell der 20.08. um 21:59 — und bei anderer Zeitzone der Vortag.
enum Verwaltungsdatum {
    /// Genau das Format, das `ParseTermin` im Backend als Datum annimmt.
    static let muster = "yyyy-MM-dd"

    private static func formatierer() -> DateFormatter {
        let f = DateFormatter()
        // Fester Kalender und feste Sprache: Das ist ein Datenformat, keine
        // Anzeige — es darf sich mit den Geräteeinstellungen nicht ändern.
        f.locale = Locale(identifier: "en_US_POSIX")
        f.calendar = Calendar(identifier: .gregorian)
        f.timeZone = Zeitpunkt.dorfZone
        f.dateFormat = muster
        return f
    }

    static func text(_ datum: Date) -> String { formatierer().string(from: datum) }

    /// Der Termin, wie ihn das Backend zurückschickt (RFC3339), als Datum
    /// für das Formular.
    static func datum(ausAntwort roh: String?) -> Date? {
        guard let roh, !roh.isEmpty else { return nil }
        if let zeitpunkt = RFC3339.datum(roh) { return zeitpunkt }
        return formatierer().date(from: String(roh.prefix(10)))
    }

    /// Derselbe Weg wie im Formular: Antwort des Backends → Datumstext.
    static func text(ausAntwort roh: String?) -> String {
        guard let datum = datum(ausAntwort: roh) else { return "" }
        return text(datum)
    }

    /// Vorbelegung für eine neue einmalige Aufgabe: heute.
    static func heute(jetzt: Date = Date()) -> Date { jetzt }
}

// MARK: - Formular „Ort"

/// Das Formular für einen Ort. `id == nil` heißt: neu.
struct OrtFormular: Hashable, Sendable {
    var id: Int64?
    var name = ""
    var beschreibung = ""
    var art = OrtEingabe.blumenkasten
    /// Die Koordinate — aus dem eigenen Gerät oder aus einem Tipp auf die Karte.
    var punkt: Kartenpunkt?
    var aktiv = true
    var sendet = false
    /// Der Wortlaut des Backends, wenn es abgelehnt hat.
    var fehler: String?

    init() {}

    init(ort: Ort) {
        id = ort.id
        name = ort.name
        beschreibung = ort.description
        art = ort.kind
        punkt = Kartenpunkt(breite: ort.lat, laenge: ort.lon)
        aktiv = ort.active
    }

    var neu: Bool { id == nil }
    var titel: String { neu ? "Neuer Ort" : "Ort bearbeiten" }

    var eingabe: OrtEingabe? {
        guard let punkt else { return nil }
        let sauber = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !sauber.isEmpty else { return nil }
        return OrtEingabe(
            name: sauber,
            description: beschreibung.trimmingCharacters(in: .whitespacesAndNewlines),
            kind: art, lat: punkt.breite, lon: punkt.laenge, active: aktiv
        )
    }

    /// Ohne Namen und ohne Standort gibt es nichts zu speichern. Alles
    /// andere prüft das Backend.
    var speicherbar: Bool { !sendet && eingabe != nil }

    /// Einen bestehenden Ort unverändert weiterschicken — nur `active`
    /// wechselt. Für Pausieren und Fortsetzen ohne Formular.
    static func eingabe(aus ort: Ort, aktiv: Bool) -> OrtEingabe {
        OrtEingabe(
            name: ort.name, description: ort.description, kind: ort.kind,
            lat: ort.lat, lon: ort.lon, active: aktiv
        )
    }
}

// MARK: - Formular „Aufgabe"

/// Das Formular für eine Aufgabe. `id == nil` heißt: neu.
///
/// `einmalig` ist die Weiche: Entweder es gibt ein Intervall **oder** einen
/// Termin. Beides zusammen bietet das Formular nicht an — das Backend wiese
/// es ohnehin ab.
struct AufgabeFormular: Hashable, Sendable {
    var ortId: Int64
    var id: Int64?
    var art = AufgabeEingabe.giessen
    var titel = ""
    var liter = ""
    var einmalig = false
    var termin = Verwaltungsdatum.heute()
    var intervall = "7"
    var rot = "14"
    /// Nur bei einmaligen Aufgaben: nach dem Erledigen abräumen.
    var abraeumenNachErledigung = false
    var aktiv = true
    var sendet = false
    var fehler: String?

    init(ortId: Int64) { self.ortId = ortId }

    init(ortId: Int64, aufgabe: Aufgabe) {
        self.ortId = ortId
        id = aufgabe.id
        art = aufgabe.kind
        titel = aufgabe.title
        liter = aufgabe.liters.map(Verwaltungszahl.text) ?? ""
        einmalig = aufgabe.oneOff
        termin = Verwaltungsdatum.datum(ausAntwort: aufgabe.dueDate) ?? Verwaltungsdatum.heute()
        intervall = Verwaltungszahl.text(aufgabe.intervalDays > 0 ? aufgabe.intervalDays : 7)
        rot = Verwaltungszahl.text(aufgabe.redAfterDays > 0 ? aufgabe.redAfterDays : 14)
        abraeumenNachErledigung = aufgabe.removeWhenDone
        aktiv = aufgabe.active
    }

    var neu: Bool { id == nil }
    var titelzeile: String { neu ? "Neue Aufgabe" : "Aufgabe bearbeiten" }

    /// Liter gibt es nur beim Gießen.
    var literSichtbar: Bool { AufgabeEingabe.literErlaubt(art: art) }

    var eingabe: AufgabeEingabe? {
        let bezeichnung = titel.trimmingCharacters(in: .whitespacesAndNewlines)
        let menge = literSichtbar ? Verwaltungszahl.wert(liter) : nil
        if einmalig {
            return .einmalig(
                kind: art, title: bezeichnung, liters: menge,
                dueDate: Verwaltungsdatum.text(termin),
                removeWhenDone: abraeumenNachErledigung, active: aktiv
            )
        }
        guard let tage = Verwaltungszahl.wert(intervall),
              let rotNach = Verwaltungszahl.wert(rot)
        else { return nil }
        return .regelmaessig(
            kind: art, title: bezeichnung, liters: menge,
            intervalDays: tage, redAfterDays: rotNach,
            // Abräumen ergibt nur bei einer einmaligen Aufgabe einen Sinn:
            // Eine regelmäßige kommt ja wieder.
            removeWhenDone: false, active: aktiv
        )
    }

    var speicherbar: Bool { !sendet && eingabe != nil }

    /// Eine bestehende Aufgabe unverändert weiterschicken — nur `active`
    /// wechselt. Für Pausieren und Fortsetzen ohne Formular.
    static func eingabe(aus aufgabe: Aufgabe, aktiv: Bool) -> AufgabeEingabe {
        if aufgabe.oneOff {
            return .einmalig(
                kind: aufgabe.kind, title: aufgabe.title, liters: aufgabe.liters,
                dueDate: Verwaltungsdatum.text(ausAntwort: aufgabe.dueDate),
                removeWhenDone: aufgabe.removeWhenDone, active: aktiv
            )
        }
        return .regelmaessig(
            kind: aufgabe.kind, title: aufgabe.title, liters: aufgabe.liters,
            intervalDays: aufgabe.intervalDays, redAfterDays: aufgabe.redAfterDays,
            removeWhenDone: aufgabe.removeWhenDone, active: aktiv
        )
    }

}

// MARK: - Texte der Rückfragen

/// Was in einer Rückfrage steht.
///
/// „Wirklich löschen?" allein wäre zu wenig: Wer eine Aufgabe zugesagt hat,
/// bekommt vom Backend die Nachricht, dass er nicht mehr losziehen muss
/// (`AufgabeEntfaellt`). Wer löscht oder pausiert, soll das vorher wissen —
/// es geht eine Nachricht an eine Person raus.
enum Verwaltungstexte {
    static let hinweisEntfaellt =
        "Wer diese Aufgabe gerade zugesagt hat, bekommt die Nachricht, "
            + "dass sie nicht mehr nötig ist."

    static let hinweisEntfaelltOrt =
        "Wer eine Aufgabe an diesem Ort gerade zugesagt hat, bekommt die Nachricht, "
            + "dass sie nicht mehr nötig ist."

    static func ortLoeschen(_ ort: Ort) -> String {
        "\(inAnfuehrung(ort.name)) wird mit allen Aufgaben und der Historie gelöscht. "
            + hinweisEntfaelltOrt
    }

    static func aufgabeLoeschen(_ aufgabe: Aufgabe) -> String {
        "\(inAnfuehrung(aufgabe.anzeigename)) wird mit der Historie gelöscht. " + hinweisEntfaellt
    }

    static func ortPausieren(_ ort: Ort) -> String {
        "\(inAnfuehrung(ort.name)) taucht dann nicht mehr auf der Karte und in der Liste auf. "
            + hinweisEntfaelltOrt + " Fortsetzen geht jederzeit."
    }

    static func aufgabePausieren(_ aufgabe: Aufgabe) -> String {
        "\(inAnfuehrung(aufgabe.anzeigename)) wird bis auf Weiteres nicht mehr fällig. "
            + hinweisEntfaellt + " Fortsetzen geht jederzeit."
    }

    /// Deutsche Anführungszeichen um einen Namen — „Unter den Eichen“.
    static func inAnfuehrung(_ text: String) -> String { "\u{201E}" + text + "\u{201C}" }

    static let hitzefaktor =
        "Der Hitzefaktor beschleunigt ausschließlich Gieß-Aufgaben: 1 ist normal, "
            + "0,5 bedeutet Hitzewelle (doppelt so schnell fällig). "
            + "Auf Jäten und auf Termine wirkt er nicht."
}

// MARK: - Modell

@Observable
final class VerwaltungModell {
    private let quelle: VerwaltungQuelle

    /// Das offene Ortsformular (oder keins).
    var ortFormular: OrtFormular?
    /// Das offene Aufgabenformular (oder keins).
    var aufgabeFormular: AufgabeFormular?

    /// Orte und Aufgaben, für die gerade geschrieben wird — solange bleiben
    /// ihre Knöpfe gesperrt.
    private(set) var laufend: Set<Int64> = []
    /// Ein abgelehnter Vorgang außerhalb eines Formulars, im Wortlaut des
    /// Backends.
    private(set) var fehler: String?
    /// Kurze Rückmeldung („Ort gespeichert.").
    private(set) var bestaetigung: String?

    private(set) var hitzefaktor: Double = 1
    private(set) var hitzefaktorGeladen = false
    private(set) var hitzefaktorLaeuft = false

    @ObservationIgnored private var verblassen: Task<Void, Never>?

    init(quelle: VerwaltungQuelle) { self.quelle = quelle }

    func laeuftGerade(_ id: Int64) -> Bool { laufend.contains(id) }

    // MARK: Orte

    func ortBearbeiten(_ ort: Ort?) {
        ortFormular = ort.map(OrtFormular.init(ort:)) ?? OrtFormular()
    }

    func ortAbbrechen() { ortFormular = nil }

    func ortSpeichern() async {
        guard let formular = ortFormular, let eingabe = formular.eingabe, !formular.sendet else { return }
        ortFormular?.sendet = true
        ortFormular?.fehler = nil
        do {
            if let id = formular.id {
                _ = try await quelle.ortAendern(id, eingabe)
            } else {
                _ = try await quelle.ortAnlegen(eingabe)
            }
            ortFormular = nil
            zeige(bestaetigung: formular.neu ? "Ort angelegt." : "Ort gespeichert.")
            await quelle.neuLaden()
        } catch {
            // Das Formular bleibt stehen, samt allem Getippten — sonst tippt
            // jemand am Blumenkasten alles noch einmal.
            ortFormular?.sendet = false
            ortFormular?.fehler = klartext(error)
        }
    }

    func ortLoeschen(_ ort: Ort) async {
        await mitLaufendem(ort.id, erfolg: "Ort gelöscht.") {
            try await self.quelle.ortLoeschen(ort.id)
        }
    }

    /// Pausieren und Fortsetzen — „Kasten abgebaut" oder „im Urlaub".
    func ortUmschalten(_ ort: Ort, aktiv: Bool) async {
        await mitLaufendem(ort.id, erfolg: aktiv ? "Ort läuft wieder." : "Ort pausiert.") {
            _ = try await self.quelle.ortAendern(ort.id, OrtFormular.eingabe(aus: ort, aktiv: aktiv))
        }
    }

    // MARK: Aufgaben

    func aufgabeBearbeiten(ort: Int64, aufgabe: Aufgabe?) {
        aufgabeFormular = aufgabe.map { AufgabeFormular(ortId: ort, aufgabe: $0) }
            ?? AufgabeFormular(ortId: ort)
    }

    func aufgabeAbbrechen() { aufgabeFormular = nil }

    func aufgabeSpeichern() async {
        guard let formular = aufgabeFormular, let eingabe = formular.eingabe, !formular.sendet else { return }
        aufgabeFormular?.sendet = true
        aufgabeFormular?.fehler = nil
        do {
            if let id = formular.id {
                _ = try await quelle.aufgabeAendern(id, eingabe)
            } else {
                _ = try await quelle.aufgabeAnlegen(formular.ortId, eingabe)
            }
            aufgabeFormular = nil
            zeige(bestaetigung: formular.neu ? "Aufgabe angelegt." : "Aufgabe gespeichert.")
            await quelle.neuLaden()
        } catch {
            aufgabeFormular?.sendet = false
            aufgabeFormular?.fehler = klartext(error)
        }
    }

    func aufgabeLoeschen(_ aufgabe: Aufgabe) async {
        await mitLaufendem(aufgabe.id, erfolg: "Aufgabe gelöscht.") {
            try await self.quelle.aufgabeLoeschen(aufgabe.id)
        }
    }

    func aufgabeUmschalten(_ aufgabe: Aufgabe, aktiv: Bool) async {
        await mitLaufendem(aufgabe.id, erfolg: aktiv ? "Aufgabe läuft wieder." : "Aufgabe pausiert.") {
            _ = try await self.quelle.aufgabeAendern(
                aufgabe.id, AufgabeFormular.eingabe(aus: aufgabe, aktiv: aktiv)
            )
        }
    }

    // MARK: Hitzefaktor

    func einstellungenLaden() async {
        do {
            hitzefaktor = try await quelle.einstellungen().wateringFactor
            hitzefaktorGeladen = true
        } catch {
            // Kein Grund, den Bereich anzuhalten: Orte und Aufgaben lassen
            // sich auch pflegen, wenn der Faktor gerade nicht zu holen ist.
            hitzefaktorGeladen = false
        }
    }

    func hitzefaktorSetzen(_ faktor: Double) async {
        guard !hitzefaktorLaeuft else { return }
        hitzefaktorLaeuft = true
        defer { hitzefaktorLaeuft = false }
        do {
            hitzefaktor = try await quelle.hitzefaktorSetzen(faktor).wateringFactor
            hitzefaktorGeladen = true
            zeige(bestaetigung: "Hitzefaktor gespeichert.")
        } catch {
            fehler = klartext(error)
        }
    }

    // MARK: Meldungen wegräumen

    func fehlerVerwerfen() { fehler = nil }

    func bestaetigungVerwerfen() {
        verblassen?.cancel()
        bestaetigung = nil
    }

    // MARK: Innereien

    private func mitLaufendem(
        _ id: Int64, erfolg: String, aktion: @escaping () async throws -> Void
    ) async {
        guard !laufend.contains(id) else { return }
        laufend.insert(id)
        defer { laufend.remove(id) }
        do {
            try await aktion()
            zeige(bestaetigung: erfolg)
        } catch {
            fehler = klartext(error)
        }
        // Auch nach einer Ablehnung: Der Stand der App ist dann ohnehin
        // überholt (jemand anderes war schneller).
        await quelle.neuLaden()
    }

    /// Der Wortlaut des Backends — die App erfindet keine eigene Begründung.
    private func klartext(_ fehler: Error) -> String {
        (fehler as? DorfFehler)?.klartext ?? DorfFehler.netz("").klartext
    }

    private func zeige(bestaetigung text: String) {
        verblassen?.cancel()
        bestaetigung = text
        verblassen = Task { [weak self] in
            try? await Task.sleep(for: .seconds(4))
            guard !Task.isCancelled else { return }
            self?.bestaetigung = nil
        }
    }
}
