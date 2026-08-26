import Combine
import Foundation

/// Woher „Einstellungen" das Löschen bekommt und wie danach abgemeldet wird.
///
/// Zwei Verschlüsse statt eines Protokolls — wie `OrteQuelle`: Der Test
/// reicht seine eigenen herein und fasst dabei weder Netz noch
/// Schlüsselbund an, es sei denn, er will genau das prüfen.
struct Kontoquelle {
    var loeschen: @MainActor () async throws -> Kontoloeschung
    /// Abmelden heißt hier: Tokensatz weg, Schlüsselbund leer, Sitzung
    /// beendet — genau das, was `Anmeldung.abmelden()` tut.
    var abmelden: @MainActor () -> Void

    static func vom(_ umgebung: AppUmgebung) -> Kontoquelle {
        Kontoquelle(
            loeschen: { [api = umgebung.api] in try await api.kontoLoeschen() },
            abmelden: { [anmeldung = umgebung.anmeldung] in anmeldung.abmelden() }
        )
    }
}

/// Der Stand der Kontolöschung: die Rückfrage, ihr Ergebnis und was danach
/// zu sagen ist.
///
/// Die wichtigste Eigenschaft steht in `darfLoeschen`: **Versehentlich geht
/// das nicht.** Es genügt nicht, zweimal irgendwo zu tippen — der eigene
/// Name (oder, wenn keiner bekannt ist, das Wort „löschen") muss
/// abgeschrieben werden. Ein Konto zu löschen ist nicht rückgängig zu
/// machen; ein Fehlgriff auf einem Telefon in der Hosentasche darf das nicht
/// auslösen.
///
/// Die zweite wichtige Eigenschaft: **Scheitert das Löschen, wird nicht
/// abgemeldet.** Sonst stünde jemand vor dem Anmeldebildschirm und wüsste
/// nicht, ob sein Konto nun weg ist oder nicht.
final class KontoModell: ObservableObject {
    /// Der Name, der abgeschrieben werden muss — der eigene, wie ihn die
    /// App auch sonst anzeigt.
    let erwarteterName: String

    private let quelle: Kontoquelle

    /// Steht die Rückfrage offen? Erst danach gibt es überhaupt ein Feld zum
    /// Tippen — das ist die erste der beiden Stufen.
    @Published private(set) var rueckfrageOffen = false
    @Published var getippterName = ""
    @Published private(set) var laeuft = false
    /// Ein gescheiterter Versuch — im Wortlaut des Backends.
    @Published private(set) var fehler: String?
    /// Steht nach dem Löschen: was mit den Erledigungen passiert ist und dass
    /// die Rössing-ID bleibt. Beides sagt das Backend, nicht die App.
    @Published private(set) var abschied: Kontoloeschung?

    init(erwarteterName: String, quelle: Kontoquelle) {
        self.erwarteterName = erwarteterName
        self.quelle = quelle
    }

    convenience init(umgebung: AppUmgebung) {
        self.init(erwarteterName: umgebung.anrede ?? "", quelle: .vom(umgebung))
    }

    /// Was getippt werden muss. Der eigene Name, wenn es einen gibt — sonst
    /// das Wort „löschen": Irgendetwas Bewusstes muss es sein.
    var bestaetigungswort: String {
        let name = erwarteterName.trimmingCharacters(in: .whitespacesAndNewlines)
        return name.isEmpty ? "löschen" : name
    }

    /// Nur wenn das Wort stimmt und gerade nichts läuft. Groß- und
    /// Kleinschreibung sowie Leerraum am Rand sind egal — abgetippt werden
    /// soll der Name, nicht die Genauigkeit einer Tastatur.
    var darfLoeschen: Bool {
        !laeuft && Self.vergleichbar(getippterName) == Self.vergleichbar(bestaetigungswort)
    }

    private static func vergleichbar(_ text: String) -> String {
        text.trimmingCharacters(in: .whitespacesAndNewlines).localizedLowercase
    }

    // MARK: Rückfrage

    func rueckfrageOeffnen() {
        getippterName = ""
        fehler = nil
        rueckfrageOffen = true
    }

    func rueckfrageSchliessen() {
        rueckfrageOffen = false
        getippterName = ""
    }

    func fehlerVerwerfen() { fehler = nil }

    // MARK: Löschen

    /// Löscht das Konto — und meldet erst danach ab.
    ///
    /// Die Reihenfolge ist wichtig: Wer zuerst abmeldet, hat kein Token mehr
    /// und kann nicht mehr löschen.
    func loeschen() async {
        guard darfLoeschen else { return }
        laeuft = true
        fehler = nil
        defer { laeuft = false }
        do {
            let antwort = try await quelle.loeschen()
            abschied = antwort
            rueckfrageOffen = false
            getippterName = ""
            quelle.abmelden()
        } catch let abgewiesen as DorfFehler {
            // Wortlaut des Backends — und ausdrücklich **kein** Abmelden.
            fehler = abgewiesen.klartext
        } catch {
            fehler = DorfFehler.netz("").klartext
        }
    }
}
