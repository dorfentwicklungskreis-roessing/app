import Foundation
import Testing

@testable import Dorf

/// Der Chat der App.
///
/// Geprüft wird der Vertrag zum Dorfserver und das Verhalten des Modells —
/// nicht, was Claude antwortet: Das entscheidet das Backend, und dort steht
/// auch die Rechteprüfung. Kein Test geht ins Netz; die Sitzung wird über
/// `protocolClasses` abgefangen, die Basis steht ausdrücklich örtlich.
struct ChatTests {
    /// Ein Zugang, dessen beide Sitzungen (die gewöhnliche und die geduldige)
    /// über die Ablage laufen.
    static func api() -> DorfApi {
        let k = URLSessionConfiguration.ephemeral
        k.protocolClasses = [Chatablage.self]
        let sitzung = URLSession(configuration: k)
        return DorfApi(basis: URL(string: "http://127.0.0.1:8099")!,
                       sitzung: sitzung,
                       geduldigeSitzung: sitzung,
                       tokenGeber: { .token("abc") })
    }

    // MARK: Der Vertrag

    @Test func dieFrageGehtMitVerlaufHinaus() async throws {
        Chatablage.antwort = (200, Data("""
        {"antwort":"Am Kirchplatz muss gegossen werden.","werkzeuge":["orte_liste"]}
        """.utf8))
        let verlauf = [
            Gespraechszug(rolle: Gespraechszug.rolleIch, text: "Moin"),
            Gespraechszug(rolle: Gespraechszug.rolleApp, text: "Moin!"),
        ]
        let antwort = try await Self.api().chatFragen("Was steht an?", verlauf: verlauf)

        #expect(antwort.antwort == "Am Kirchplatz muss gegossen werden.")
        #expect(antwort.werkzeuge == ["orte_liste"])

        let anfrage = try #require(Chatablage.letzteAnfrage)
        #expect(anfrage.url?.path == "/api/v1/chat")
        #expect(anfrage.httpMethod == "POST")
        #expect(anfrage.value(forHTTPHeaderField: "Authorization") == "Bearer abc")

        let rumpf = try #require(Chatablage.letzterRumpf)
        let gelesen = try #require(try JSONSerialization.jsonObject(with: rumpf) as? [String: Any])
        #expect(gelesen["frage"] as? String == "Was steht an?")
        let mitgeschickt = try #require(gelesen["verlauf"] as? [[String: Any]])
        #expect(mitgeschickt.count == 2)
        #expect(mitgeschickt[0]["rolle"] as? String == "ich")
        #expect(mitgeschickt[1]["rolle"] as? String == "app")
    }

    /// Eine Antwort ohne die freiwilligen Felder darf nicht scheitern — sonst
    /// bricht die App an einem Feld, das der Server weglassen darf.
    @Test func eineKnappeAntwortReicht() async throws {
        Chatablage.antwort = (200, Data(#"{"antwort":"Moin!"}"#.utf8))
        let antwort = try await Self.api().chatFragen("Moin", verlauf: [])
        #expect(antwort.antwort == "Moin!")
        #expect(antwort.werkzeuge.isEmpty)
        #expect(antwort.abgebrochen == false)
    }

    @Test func ohneSchluesselSagtDerStandWarum() async throws {
        Chatablage.antwort = (200, Data("""
        {"verfuegbar":false,"hinweis":"Der Chat ist noch nicht eingerichtet."}
        """.utf8))
        let stand = try await Self.api().chatstand()
        #expect(stand.verfuegbar == false)
        #expect(stand.hinweis == "Der Chat ist noch nicht eingerichtet.")
    }

    // MARK: Das Modell

    @MainActor
    @Test func eineFrageLandetMitAntwortImVerlauf() async throws {
        Chatablage.antwort = (200, Data("""
        {"antwort":"Am Kirchplatz.","werkzeuge":["orte_liste"]}
        """.utf8))
        let modell = ChatModell()
        modell.entwurf = "  Wo muss gegossen werden?  "
        await modell.fragen(api: Self.api())

        #expect(modell.verlauf.count == 2)
        #expect(modell.verlauf[0].vonMir)
        // Der Rand des Getippten geht nicht mit hinaus.
        #expect(modell.verlauf[0].text == "Wo muss gegossen werden?")
        #expect(modell.verlauf[1].vonMir == false)
        #expect(modell.verlauf[1].werkzeuge == ["orte_liste"])
        #expect(modell.entwurf.isEmpty)
        #expect(modell.hinweis == nil)
    }

    /// Bei einer Störung bleibt der getippte Text stehen: Niemand tippt gern
    /// zweimal, und der Satz des Backends steht daneben.
    @MainActor
    @Test func einFehlerKostetDenTextNicht() async throws {
        Chatablage.antwort = (503, Data("""
        {"error":"Der Chat ist gerade überlastet. Bitte gleich noch einmal versuchen."}
        """.utf8))
        let modell = ChatModell()
        modell.entwurf = "Was steht an?"
        await modell.fragen(api: Self.api())

        #expect(modell.verlauf.isEmpty)
        #expect(modell.entwurf == "Was steht an?")
        #expect(modell.hinweis?.contains("überlastet") == true)
        #expect(modell.laeuft == false)
    }

    /// Die Absage des Backends wird im Wortlaut gezeigt — die App erfindet
    /// keine eigene Begründung.
    @MainActor
    @Test func dieAbsageDesBackendsStehtImWortlautDa() async throws {
        Chatablage.antwort = (429, Data("""
        {"error":"Das waren gerade viele Fragen auf einmal. Bitte später noch einmal."}
        """.utf8))
        let modell = ChatModell()
        modell.entwurf = "Und jetzt?"
        await modell.fragen(api: Self.api())
        #expect(modell.hinweis?.isEmpty == false)
    }

    /// Ohne eingerichteten Chat lässt der Bereich gar nicht erst tippen.
    @MainActor
    @Test func ohneEinrichtungIstNichtsAbsendbar() async throws {
        Chatablage.antwort = (200, Data(#"{"verfuegbar":false,"hinweis":"Noch nicht da."}"#.utf8))
        let modell = ChatModell()
        await modell.standLaden(api: Self.api())
        modell.entwurf = "Moin"
        #expect(modell.eingerichtet == false)
        #expect(modell.absendbar == false)
    }

    /// Ein Aussetzer der Leitung beim Öffnen ist keine dauerhafte
    /// Abschaltung: Der Bereich bleibt bedienbar, und der erste Versuch sagt
    /// dann die Wahrheit. „Noch nicht eingerichtet" sagt er nur, wenn das
    /// Backend es sagt.
    @MainActor
    @Test func einNetzausfallSchaltetDenBereichNichtAb() async throws {
        Chatablage.antwort = (500, Data())
        let modell = ChatModell()
        await modell.standLaden(api: Self.api())
        #expect(modell.eingerichtet)
        modell.entwurf = "Moin"
        #expect(modell.absendbar)
    }

    @MainActor
    @Test func leeresUndZuLangesGehtNichtHinaus() async throws {
        Chatablage.antwort = (200, Data(#"{"verfuegbar":true}"#.utf8))
        let modell = ChatModell()
        await modell.standLaden(api: Self.api())
        #expect(modell.eingerichtet)

        modell.entwurf = "   "
        #expect(modell.absendbar == false)
        modell.entwurf = String(repeating: "ä", count: ChatModell.maxFrage + 1)
        #expect(modell.absendbar == false)
        modell.entwurf = "Was steht an?"
        #expect(modell.absendbar)
    }

    /// Der Verlauf wird gekürzt, bevor er hinausgeht — dieselbe Grenze wie im
    /// Backend, damit die App nicht für etwas bezahlt, was dort ohnehin
    /// vorne herausfällt.
    @MainActor
    @Test func derVerlaufWirdGekuerzt() async throws {
        Chatablage.antwort = (200, Data(#"{"antwort":"Ja."}"#.utf8))
        let modell = ChatModell()
        let api = Self.api()
        for i in 0 ..< (ChatModell.maxVerlauf + 4) {
            modell.entwurf = "Frage \(i)"
            await modell.fragen(api: api)
        }
        let rumpf = try #require(Chatablage.letzterRumpf)
        let gelesen = try #require(try JSONSerialization.jsonObject(with: rumpf) as? [String: Any])
        let verlauf = try #require(gelesen["verlauf"] as? [[String: Any]])
        #expect(verlauf.count <= ChatModell.maxVerlauf)
    }

    @MainActor
    @Test func neuAnfangenLeertDenVerlauf() async throws {
        Chatablage.antwort = (200, Data(#"{"antwort":"Ja."}"#.utf8))
        let modell = ChatModell()
        modell.entwurf = "Moin"
        await modell.fragen(api: Self.api())
        #expect(!modell.verlauf.isEmpty)
        modell.neuAnfangen()
        #expect(modell.verlauf.isEmpty)
        #expect(modell.hinweis == nil)
    }
}

/// Örtliche Ablage: Sie beantwortet jede Anfrage aus dem, was der Test
/// hinterlegt hat, und merkt sich, was hinausgegangen wäre.
private nonisolated final class Chatablage: URLProtocol {
    nonisolated(unsafe) static var antwort: (Int, Data) = (200, Data())
    nonisolated(unsafe) static var letzteAnfrage: URLRequest?
    nonisolated(unsafe) static var letzterRumpf: Data?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        Chatablage.letzteAnfrage = request
        // `httpBody` ist bei einer durchgereichten Anfrage leer; der Rumpf
        // steht im Strom.
        Chatablage.letzterRumpf = request.httpBody ?? request.httpBodyStream.map(Chatablage.lies)
        let (status, daten) = Chatablage.antwort
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

    static func lies(_ strom: InputStream) -> Data {
        strom.open()
        defer { strom.close() }
        var daten = Data()
        let groesse = 4096
        var puffer = [UInt8](repeating: 0, count: groesse)
        while strom.hasBytesAvailable {
            let gelesen = strom.read(&puffer, maxLength: groesse)
            if gelesen <= 0 { break }
            daten.append(puffer, count: gelesen)
        }
        return daten
    }
}
