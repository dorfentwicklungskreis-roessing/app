import Foundation
import Testing

@testable import Dorf

/// Der Vertrag mit der Website: So sieht `/events.json` aus, und so wird es
/// gelesen. Der Ausschnitt stammt aus der tatsächlich gebauten Datei von
/// rössing.de (ergänzt um einen Termin mit externer Primärquelle).
///
/// Ändert sich das Format dort, muss dieser Test hier auffallen — und nicht
/// erst eine leere Liste auf dem Telefon. Kein Test geht ins Netz: geprüft
/// wird die Aufbereitung, und der Client bekommt eine örtliche Ablage
/// untergeschoben.
struct VeranstaltungenTests {
    // MARK: Vorlagen

    static let feed = """
    {
      "version": 1,
      "generatedAt": "2026-08-14T13:37:58.000Z",
      "events": [
        {
          "id": "2026-08-17-blutspende-drk",
          "name": "Blutspende",
          "description": "DRK-Blutspende im Dorfgemeinschaftshaus Rössing.",
          "start": "2026-08-17",
          "allDay": true,
          "url": "https://xn--rssing-wxa.de/events/2026-08-17-blutspende-drk",
          "external": false,
          "location": {
            "name": "Dorfgemeinschaftshaus Rössing",
            "address": "Kirchstraße 3, 31171 Nordstemmen"
          },
          "organizer": { "name": "DRK-Blutspendedienst" }
        },
        {
          "id": "2026-08-20-grillen-kirchenstiftung",
          "name": "Grillen im Pfarrgarten",
          "description": "Die Kirchenstiftung Rössing lädt zum Grillen im Pfarrgarten ein.",
          "start": "2026-08-20T18:00:00+02:00",
          "allDay": false,
          "url": "https://xn--rssing-wxa.de/events/2026-08-20-grillen-kirchenstiftung",
          "external": false,
          "location": {
            "name": "Pfarrgarten Rössing",
            "address": "Pfarrstr. 1, 31171 Nordstemmen",
            "lat": 52.1843,
            "lon": 9.8162
          },
          "organizer": { "name": "Kirchenstiftung Rössing" }
        },
        {
          "id": "2026-09-05-konzert",
          "name": "Jahreskonzert",
          "description": "Das Jahreskonzert des Musikzugs.",
          "start": "2026-09-05T19:00:00+02:00",
          "end": "2026-09-05T21:30:00+02:00",
          "allDay": false,
          "url": "https://musikzug-roessing.de/jahreskonzert",
          "external": true
        }
      ]
    }
    """

    static func gelesen(_ text: String) throws -> VeranstaltungenFeedDto {
        try JSONDecoder().decode(VeranstaltungenFeedDto.self, from: Data(text.utf8))
    }

    static func zeitpunkt(_ text: String) throws -> Date {
        try #require(RFC3339.datum(text))
    }

    // MARK: Die Datei der Website

    @Test func dieDateiDerWebsiteWirdVollstaendigGelesen() throws {
        let feed = try Self.gelesen(Self.feed)
        #expect(feed.version == 1)

        let termine = feed.events.alsTermine(jetzt: try Self.zeitpunkt("2026-08-14T10:00:00Z"))

        #expect(termine.map(\.id) == [
            "2026-08-17-blutspende-drk",
            "2026-08-20-grillen-kirchenstiftung",
            "2026-09-05-konzert",
        ])

        let blutspende = try #require(termine.first)
        #expect(blutspende.ganztaegig)
        #expect(blutspende.zeitText == nil)
        #expect(blutspende.datumText == "Mo, 17.08.2026")
        #expect(blutspende.ortName == "Dorfgemeinschaftshaus Rössing")
        #expect(blutspende.ortAdresse == "Kirchstraße 3, 31171 Nordstemmen")
        #expect(blutspende.veranstalter == "DRK-Blutspendedienst")
        #expect(blutspende.koordinate == nil)

        let grillen = termine[1]
        #expect(grillen.datumText == "Do, 20.08.2026")
        #expect(grillen.zeitText == "18:00 Uhr")
        #expect(grillen.koordinate?.lat == 52.1843)
        #expect(grillen.koordinate?.lon == 9.8162)
    }

    @Test func externePrimaerquelleGewinnt() throws {
        let feed = try Self.gelesen(Self.feed)
        let termine = feed.events.alsTermine(jetzt: try Self.zeitpunkt("2026-08-14T10:00:00Z"))

        // Externe Primärquelle: Der Tipp führt dorthin, nicht auf rössing.de.
        let konzert = try #require(termine.last)
        #expect(konzert.extern)
        #expect(konzert.url == "https://musikzug-roessing.de/jahreskonzert")
        #expect(konzert.adresse?.host() == "musikzug-roessing.de")

        // Und umgekehrt: ohne externe Quelle bleibt es bei der Dorfseite.
        let blutspende = try #require(termine.first)
        #expect(!blutspende.extern)
        #expect(blutspende.url.contains("/events/2026-08-17-blutspende-drk"))
    }

    // MARK: Zeitzone

    @Test func sommerOffsetErgibtOrtszeit() throws {
        let mitOffset = VeranstaltungDto(id: "a", start: "2026-07-15T18:00:00+02:00")
        let termin = try #require(mitOffset.alsTermin())
        #expect(termin.zeitText == "18:00 Uhr")
        #expect(termin.datumText == "Mi, 15.07.2026")
        #expect(termin.beginn == (try Self.zeitpunkt("2026-07-15T16:00:00Z")))

        // Derselbe Zeitpunkt in UTC muss dieselbe Ortszeit ergeben — sonst
        // wurde die Zeichenkette naiv abgeschnitten statt umgerechnet.
        let inUtc = VeranstaltungDto(id: "b", start: "2026-07-15T16:00:00Z")
        #expect(try #require(inUtc.alsTermin()).zeitText == "18:00 Uhr")
    }

    @Test func winterOffsetErgibtOrtszeit() throws {
        let mitOffset = VeranstaltungDto(id: "a", start: "2026-01-15T18:00:00+01:00")
        let termin = try #require(mitOffset.alsTermin())
        #expect(termin.zeitText == "18:00 Uhr")
        #expect(termin.datumText == "Do, 15.01.2026")
        #expect(termin.beginn == (try Self.zeitpunkt("2026-01-15T17:00:00Z")))

        let inUtc = VeranstaltungDto(id: "b", start: "2026-01-15T17:00:00Z")
        #expect(try #require(inUtc.alsTermin()).zeitText == "18:00 Uhr")
    }

    // MARK: Ganztägig

    @Test func ganztaegigErfindetKeineUhrzeit() throws {
        let dto = VeranstaltungDto(id: "a", start: "2026-08-17", allDay: true)
        let termin = try #require(dto.alsTermin())
        #expect(termin.ganztaegig)
        #expect(termin.zeitText == nil)
        #expect(termin.datumText == "Mo, 17.08.2026")
        // Der Tag beginnt um Mitternacht in Ortszeit, nicht in UTC.
        #expect(termin.beginn == (try Self.zeitpunkt("2026-08-16T22:00:00Z")))
    }

    @Test func datumUndUhrzeitSindFuerVoiceOverAusformuliert() throws {
        let ganztaegig = try #require(
            VeranstaltungDto(id: "a", start: "2026-08-17", allDay: true).alsTermin()
        )
        #expect(ganztaegig.vorlesetext.contains("17. August 2026"))
        #expect(ganztaegig.vorlesetext.contains("ganztägig"))

        let abends = try #require(
            VeranstaltungDto(id: "b", start: "2026-08-20T18:00:00+02:00").alsTermin()
        )
        #expect(abends.vorlesetext.contains("20. August 2026"))
        #expect(abends.vorlesetext.contains("18 Uhr"))
    }

    // MARK: Vorbei erst am Tagesende

    @Test func einTerminUmNeunzehnUhrIstUmZwanzigUhrNochDa() throws {
        let dto = VeranstaltungDto(id: "a", start: "2026-08-26T19:00:00+02:00")
        let liste = [dto]

        let umZwanzigUhr = try Self.zeitpunkt("2026-08-26T20:00:00+02:00")
        #expect(liste.alsTermine(jetzt: umZwanzigUhr).map(\.id) == ["a"])

        // Kurz vor Mitternacht ebenfalls — vorbei ist er erst am Tagesende.
        let kurzVorMitternacht = try Self.zeitpunkt("2026-08-26T23:59:00+02:00")
        #expect(liste.alsTermine(jetzt: kurzVorMitternacht).count == 1)

        let amNaechstenMorgen = try Self.zeitpunkt("2026-08-27T07:00:00+02:00")
        #expect(liste.alsTermine(jetzt: amNaechstenMorgen).isEmpty)
    }

    @Test func mehrtaegigerTerminBleibtBisZumLetztenTag() throws {
        let dto = VeranstaltungDto(
            id: "dorffest", start: "2026-08-28", end: "2026-08-30", allDay: true
        )
        let liste = [dto]

        #expect(liste.alsTermine(jetzt: try Self.zeitpunkt("2026-08-29T12:00:00+02:00")).count == 1)
        #expect(liste.alsTermine(jetzt: try Self.zeitpunkt("2026-08-30T23:30:00+02:00")).count == 1)
        #expect(liste.alsTermine(jetzt: try Self.zeitpunkt("2026-08-31T00:30:00+02:00")).isEmpty)
    }

    @Test func mehrtaegigMitUhrzeitZaehltEbenfallsBisTagesende() throws {
        // Ende 21:30 Uhr — trotzdem verschwindet der Termin erst am
        // Tagesende, genau wie auf der Website.
        let dto = VeranstaltungDto(
            id: "konzert",
            start: "2026-09-05T19:00:00+02:00",
            end: "2026-09-05T21:30:00+02:00"
        )
        #expect([dto].alsTermine(jetzt: try Self.zeitpunkt("2026-09-05T22:00:00+02:00")).count == 1)
        #expect([dto].alsTermine(jetzt: try Self.zeitpunkt("2026-09-06T00:05:00+02:00")).isEmpty)
    }

    // MARK: Sieben und Sortieren

    @Test func vergangenesFliegtRausUndKommendesStehtVorne() throws {
        let liste = [
            VeranstaltungDto(id: "spaeter", start: "2026-09-05T19:00:00+02:00"),
            VeranstaltungDto(id: "vergangen", start: "2026-07-01T19:00:00+02:00"),
            VeranstaltungDto(id: "naechster", start: "2026-08-20T18:00:00+02:00"),
            VeranstaltungDto(id: "heuteFrueh", start: "2026-08-14T08:00:00+02:00"),
        ]

        let termine = liste.alsTermine(jetzt: try Self.zeitpunkt("2026-08-14T10:00:00Z"))

        // „heuteFrueh" war um 8 Uhr — er ist erst am Tagesende vorbei und
        // steht deshalb noch vorne.
        #expect(termine.map(\.id) == ["heuteFrueh", "naechster", "spaeter"])
    }

    @Test func einKaputterEintragKostetNichtDieGanzeListe() throws {
        let liste = [
            VeranstaltungDto(id: "ohneDatum", start: ""),
            VeranstaltungDto(id: "gut", start: "2026-08-20T18:00:00+02:00"),
            VeranstaltungDto(id: "unlesbar", start: "übermorgen abends"),
            VeranstaltungDto(id: "auchGut", start: "2026-08-21"),
        ]

        let termine = liste.alsTermine(jetzt: try Self.zeitpunkt("2026-08-14T10:00:00Z"))
        #expect(termine.map(\.id) == ["gut", "auchGut"])
    }

    // MARK: Was fehlen darf

    @Test func fehlendeFelderKostenDieListeNicht() throws {
        // Nur Kennung und Beginn — alles andere muss auf die Vorgabe fallen.
        let feed = try Self.gelesen("""
        {"events":[{"id":"karg","start":"2026-09-01"}]}
        """)
        #expect(feed.version == 1)
        let termin = try #require(feed.events.first?.alsTermin())
        #expect(termin.name.isEmpty)
        #expect(termin.beschreibung.isEmpty)
        #expect(termin.ortName == nil)
        #expect(termin.ortAdresse == nil)
        #expect(termin.veranstalter == nil)
        #expect(!termin.extern)
        #expect(termin.adresse == nil)
    }

    @Test func unbekannteFelderStoerenNicht() throws {
        // Die Website darf etwas ergänzen, ohne ältere App-Versionen zu brechen.
        let feed = try Self.gelesen("""
        {"version":2,"generatedAt":"","neuesFeld":42,
         "events":[{"id":"a","start":"2026-09-01","kategorie":"Sport"}]}
        """)
        #expect(feed.events.count == 1)
    }

    @Test func einOrtOhneNamenGiltNichtAlsOrt() throws {
        let dto = VeranstaltungDto(
            id: "a", start: "2026-09-01",
            location: VeranstaltungsortDto(name: "  ", address: "Irgendwo"),
            organizer: VeranstalterDto(name: " ")
        )
        let termin = try #require(dto.alsTermin())
        #expect(termin.ortName == nil)
        #expect(termin.ortAdresse == nil)
        #expect(termin.veranstalter == nil)
    }

    @Test func einLeererKalenderIstKeinFehler() throws {
        let feed = try Self.gelesen("""
        {"version":1,"generatedAt":"","events":[]}
        """)
        #expect(feed.events.isEmpty)
    }

    @Test func eineAntwortDieKeinJsonIstReisstNichtsMit() {
        // Kommt statt der Datei eine Fehlerseite (Zwischenspeicher, Portal,
        // umgezogene Adresse), darf das nicht abstürzen.
        let html = Data("<!doctype html><title>Nicht gefunden</title>".utf8)
        #expect(throws: (any Error).self) {
            try JSONDecoder().decode(VeranstaltungenFeedDto.self, from: html)
        }
    }

    // MARK: Der Zustand der Ansicht

    @Test func einFehlerLaesstDieAeltereListeStehen() async throws {
        let jetzt = try Self.zeitpunkt("2026-08-14T10:00:00Z")
        let folge = Antwortfolge(daten: try Self.gelesen(Self.feed).events)
        let modell = VeranstaltungenModell(holen: { try await folge.liefern() }, uhr: { jetzt })

        await modell.laden()
        #expect(modell.termine.count == 3)
        #expect(modell.hinweis == nil)
        #expect(!modell.leer)

        folge.scheitern = true
        await modell.aktualisieren()

        // Die alte Liste bleibt stehen, der Hinweis kommt darüber — eine
        // leere Seite ohne Erklärung wäre das schlechteste Ergebnis.
        #expect(modell.termine.count == 3)
        #expect(modell.hinweis != nil)
        #expect(!modell.leer)
    }

    @Test func ohneListeNenntDerHinweisDenGrund() async throws {
        let folge = Antwortfolge(daten: [])
        folge.scheitern = true
        let modell = VeranstaltungenModell(holen: { try await folge.liefern() }, uhr: { Date() })

        await modell.laden()

        #expect(modell.termine.isEmpty)
        #expect(modell.hinweis?.isEmpty == false)
        // „Fehler" heißt nicht „nichts los" — die leere Aussage bleibt aus.
        #expect(!modell.leer)
    }

    @Test func ohneTermineUndOhneFehlerIstWirklichNichtsLos() async {
        let modell = VeranstaltungenModell(holen: { [] }, uhr: { Date() })
        await modell.laden()
        #expect(modell.leer)
        #expect(modell.hinweis == nil)
    }

    // MARK: An die Website geht kein Zugangstoken

    @Test func anDieWebsiteGehtKeinZugangstokenUndKeineAnfrageInsNetz() async throws {
        // Die Anfrage wird örtlich abgefangen; es geht nichts hinaus.
        Ablage.antwort = (200, Data(Self.feed.utf8))
        Ablage.letzteAnfrage = nil

        let k = URLSessionConfiguration.ephemeral
        k.protocolClasses = [Ablage.self]
        let quelle = WebseiteVeranstaltungen(
            basis: URL(string: "http://127.0.0.1:8099")!,
            sitzung: URLSession(configuration: k)
        )

        let events = try await quelle.kommende()
        #expect(events.count == 3)

        let anfrage = try #require(Ablage.letzteAnfrage)
        #expect(anfrage.url?.path == "/events.json")
        // Die Website ist öffentlich und hat mit unserer Anmeldung nichts zu
        // tun. Ein Token dorthin wäre eine unnötige Preisgabe.
        #expect(anfrage.value(forHTTPHeaderField: "Authorization") == nil)
    }

    @Test func eineKaputteAntwortWirftStattHeimlichNichtsZuZeigen() async throws {
        Ablage.antwort = (500, Data())
        let k = URLSessionConfiguration.ephemeral
        k.protocolClasses = [Ablage.self]
        let quelle = WebseiteVeranstaltungen(
            basis: URL(string: "http://127.0.0.1:8099")!,
            sitzung: URLSession(configuration: k)
        )

        await #expect(throws: VeranstaltungenFehler.self) { try await quelle.kommende() }
    }
}

/// Eine Quelle, die auf Wunsch scheitert — für den Zustand der Ansicht.
private final class Antwortfolge: @unchecked Sendable {
    var scheitern = false
    let daten: [VeranstaltungDto]

    init(daten: [VeranstaltungDto]) { self.daten = daten }

    func liefern() async throws -> [VeranstaltungDto] {
        if scheitern { throw VeranstaltungenFehler.serverfehler(status: 500) }
        return daten
    }
}

/// Örtliche Ablage statt Netz: Die Anfrage wird abgefangen und beantwortet,
/// ohne das Gerät zu verlassen. Kein Test darf nach draußen.
private nonisolated final class Ablage: URLProtocol {
    nonisolated(unsafe) static var antwort: (Int, Data) = (200, Data())
    nonisolated(unsafe) static var letzteAnfrage: URLRequest?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        Ablage.letzteAnfrage = request
        let (status, daten) = Ablage.antwort
        let kopf = HTTPURLResponse(
            url: request.url ?? URL(string: "http://127.0.0.1")!,
            statusCode: status,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json; charset=utf-8"]
        )!
        client?.urlProtocol(self, didReceive: kopf, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: daten)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}
