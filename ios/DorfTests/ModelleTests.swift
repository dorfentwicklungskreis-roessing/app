import Foundation
import Testing

@testable import Dorf

/// Der Vertrag mit dem Backend: Was dort steht, muss hier ankommen — und was
/// fehlt, darf die Antwort nicht kosten. Beides ist schon einmal schiefgegangen
/// (Android: `coerceInputValues`), deshalb steht es als Test da.
struct ModelleTests {
    @Test func ortMitAufgabeWirdGelesen() throws {
        let roh = """
        {"places":[{"id":1,"name":"Unter den Eichen","lat":52.1,"lon":9.8,
        "status":"yellow","tasks":[{"id":7,"kind":"giessen","liters":10,
        "status":"red","lockedUntil":"2026-08-26T10:00:00Z"}]}],
        "wateringFactor":0.5}
        """.data(using: .utf8)!

        let antwort = try JSONDecoder().decode(OrteAntwort.self, from: roh)
        #expect(antwort.wateringFactor == 0.5)
        let ort = try #require(antwort.places.first)
        #expect(ort.name == "Unter den Eichen")
        #expect(ort.ampel == .yellow)
        let aufgabe = try #require(ort.tasks.first)
        #expect(aufgabe.ampel == .red)
        #expect(aufgabe.anzeigename == "Gießen")
        #expect(aufgabe.gesperrtBis != nil)
    }

    @Test func fehlendeFelderKostenDieAntwortNicht() throws {
        // Nur die Pflichtfelder — alles andere muss auf die Vorgabe fallen.
        let roh = #"{"id":3,"name":"Beet","lat":52.0,"lon":9.7}"#.data(using: .utf8)!
        let ort = try JSONDecoder().decode(Ort.self, from: roh)
        #expect(ort.kind == "blumenkasten")
        #expect(ort.active)
        #expect(ort.ampel == .green)
        #expect(ort.tasks.isEmpty)
    }

    @Test func unbekannteFelderStoerenNicht() throws {
        // Das Backend darf Felder ergänzen, ohne alte App-Versionen zu brechen.
        let roh = #"{"id":3,"name":"Beet","lat":52.0,"lon":9.7,"neuesFeld":42}"#
            .data(using: .utf8)!
        #expect(throws: Never.self) { try JSONDecoder().decode(Ort.self, from: roh) }
    }

    @Test func einmaligeAufgabeNimmtKeineZweiteMeldung() throws {
        let roh = """
        {"id":9,"kind":"sonstiges","oneOff":true,"dueDate":"2026-09-01T00:00:00Z",
        "lastCompletion":{"id":4,"taskId":9,"userName":"Anna","doneAt":"2026-08-20T08:00:00Z"}}
        """.data(using: .utf8)!
        let aufgabe = try JSONDecoder().decode(Aufgabe.self, from: roh)
        #expect(aufgabe.erledigtUndVorbei)
        #expect(!aufgabe.meldenMoeglich())
    }

    @Test func spielschutzSperrtDenKnopf() throws {
        let inEinerStunde = Date().addingTimeInterval(3600)
        let text = ISO8601DateFormatter().string(from: inEinerStunde)
        let roh = #"{"id":9,"kind":"giessen","lockedUntil":"\#(text)"}"#.data(using: .utf8)!
        let aufgabe = try JSONDecoder().decode(Aufgabe.self, from: roh)
        #expect(!aufgabe.meldenMoeglich())
        #expect(aufgabe.meldenMoeglich(jetzt: inEinerStunde.addingTimeInterval(60)))
    }

    @Test func kontaktdatenSindNichtVonSelbstOeffentlich() throws {
        // Die Vorbelegung des Backends: Telefon, E-Mail und Notiz bleiben bei
        // der Verwaltung, bis jemand sie bewusst freigibt.
        let leer = Sichtbarkeit()
        #expect(leer.displayNameOeffentlich)
        #expect(leer.nicknameOeffentlich)
        #expect(!leer.phoneOeffentlich)
        #expect(!leer.emailOeffentlich)
        #expect(!leer.noteOeffentlich)
    }

    @Test func unbekannteSichtbarkeitGiltAlsNichtOeffentlich() throws {
        let roh = #"{"phone":"irgendwas-neues"}"#.data(using: .utf8)!
        let sicht = try JSONDecoder().decode(Sichtbarkeit.self, from: roh)
        #expect(!sicht.phoneOeffentlich)
    }

    @Test func rfc3339MitUndOhneBruchteile() {
        #expect(RFC3339.datum("2026-08-26T10:00:00Z") != nil)
        #expect(RFC3339.datum("2026-08-26T10:00:00.123Z") != nil)
        #expect(RFC3339.datum("") == nil)
        #expect(RFC3339.datum("kein Datum") == nil)
    }
}

// MARK: - Der Träger am Ort

/// Wer sich um einen Ort kümmert, kommt fertig vom Server — samt Verdeckung:
/// Eine geschlossene Gruppe heißt für Außenstehende „Eine Gruppe aus dem
/// Dorf". Die App entscheidet daran nichts, sonst gäbe es die
/// Sichtbarkeitsregel zweimal.
struct TraegerAmOrtTests {
    @Test func traegerNameWirdGelesen() throws {
        let roh = """
        {"id":3,"name":"Beet vor dem Dorfgemeinschaftshaus","lat":52.18,"lon":9.81,
         "traegerName":"AK 2 Umwelt und Natur"}
        """.data(using: .utf8)!
        let ort = try JSONDecoder().decode(Ort.self, from: roh)
        #expect(ort.traegerName == "AK 2 Umwelt und Natur")
    }

    /// Ältere Stände des Backends schicken das Feld nicht mit. Dann bleibt die
    /// Zeile leer statt dass die ganze Liste unlesbar wird.
    @Test func ohneTraegerBleibtEsLeer() throws {
        let roh = """
        {"id":1,"name":"Unter den Eichen","lat":52.18,"lon":9.81}
        """.data(using: .utf8)!
        let ort = try JSONDecoder().decode(Ort.self, from: roh)
        #expect(ort.traegerName.isEmpty)
    }
}

// MARK: - Außer Dienst

/// Eine Aufgabe, die zu dieser Jahreszeit nicht anfällt, ist nicht „grün".
/// Grün hieße „alles gut" — eine Aussage über etwas, das gerade gar nicht
/// ansteht. Das Beet vor dem Dorfgemeinschaftshaus wird im Winter nicht
/// gejätet, und genau das soll dort stehen.
struct AusserDienstTests {
    @Test func ruhenderStatusWirdGelesen() throws {
        let roh = """
        {"id":3,"name":"Beet","lat":52.18,"lon":9.81,"status":"dormant"}
        """.data(using: .utf8)!
        let ort = try JSONDecoder().decode(Ort.self, from: roh)
        #expect(ort.ampel == .dormant)
    }

    @Test func ruhendSagtWasSacheIst() {
        #expect(Ampel.dormant.text(fuer: "jaeten") == "Wird jetzt nicht gejätet")
        #expect(Ampel.dormant.text(fuer: "giessen") == "Wird jetzt nicht gegossen")
        #expect(Ampel.dormant.text() == "Außer Dienst")
        // Die Farbe allein ist keine Information.
        #expect(Ampel.dormant.vorlesetext == "Außer Dienst")
    }

    /// Was nicht ansteht, steht auch nicht oben in der Liste.
    @Test func ruhendSortiertGanzNachHinten() {
        let sortiert = [Ampel.dormant, .green, .red, .yellow]
            .sorted { $0.dringlichkeit < $1.dringlichkeit }
        #expect(sortiert == [.red, .yellow, .green, .dormant])
    }

    /// Ein Wert, den diese Fassung nicht kennt, darf die Antwort nicht kosten.
    @Test func unbekannterStatusKostetNichtDieAntwort() throws {
        let roh = """
        {"id":9,"name":"Neu","lat":52.18,"lon":9.81,"status":"irgendwas"}
        """.data(using: .utf8)!
        let ort = try JSONDecoder().decode(Ort.self, from: roh)
        #expect(ort.ampel == .green)
    }
}
