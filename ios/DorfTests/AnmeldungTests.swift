import Foundation
import Testing

@testable import Dorf

/// Dass eine einmal erteilte Anmeldung hält.
///
/// Der Kern dieser Datei ist ein einziger Unterschied: **„Der Server sagt
/// nein" ist etwas anderes als „ich konnte den Server nicht fragen."** Das
/// Erste ist eine Entscheidung der Rössing-ID — widerrufenes Token,
/// gesperrtes Konto, geändertes Passwort —, das Zweite ein Funkloch. Solange
/// beides zum selben Ergebnis führte, kostete jeder schlechte Augenblick im
/// Mobilfunknetz die Anmeldung, und die Person stand ohne Grund wieder vor
/// dem Anmeldeknopf.
///
/// Der zweite Kern ist der Kaltstart: Beim Start fragen mehrere Abrufe im
/// selben Augenblick nach einem Token (Profil, Orte, Gerätekennung). Zitadel
/// gibt bei jeder Erneuerung ein **neues** Erneuerungstoken aus und weist das
/// alte danach ab — wer parallel erneuert, gewinnt einmal und verliert
/// sonst. Genau das war nach einer Aktualisierung zu sehen.
///
/// **Kein Test geht ins Netz.** Die Rössing-ID ist eine Attrappe
/// ([RoessingId]), abgefangen über `protocolClasses`; der Schlüsselbund des
/// Rechners wird nicht angefasst, die Ablage ist ein Feld im Speicher. Und
/// kein Test wartet: Es wird nirgends geschlafen.
@Suite(.serialized)
@MainActor
struct AnmeldungTests {
    // MARK: Werkzeug

    /// Eine Ablage im Speicher — statt des echten Schlüsselbunds.
    private final class Ablagefach {
        var satz: Tokensatz?
        var geloescht = false

        init(_ satz: Tokensatz?) { self.satz = satz }

        var ablage: Tokenablage {
            Tokenablage(
                lesen: { self.satz },
                sichern: { self.satz = $0 },
                loeschen: {
                    self.satz = nil
                    self.geloescht = true
                }
            )
        }
    }

    private static let aussteller = URL(string: "http://127.0.0.1:8123")!

    /// Ein abgelaufenes Zugangstoken mit Erneuerungstoken — die Lage nach
    /// jedem Kaltstart am nächsten Morgen.
    private static func abgelaufen(erneuerung: String? = "r1") -> Tokensatz {
        Tokensatz(
            zugangstoken: "alt",
            erneuerungstoken: erneuerung,
            idToken: nil,
            laeuftAbAm: Date().addingTimeInterval(-60)
        )
    }

    private static func aufbau(_ satz: Tokensatz?,
                               _ antworten: [RoessingId.Antwort]) -> (Anmeldung, Ablagefach) {
        RoessingId.aufsetzen(antworten)
        let fach = Ablagefach(satz)
        let k = URLSessionConfiguration.ephemeral
        k.protocolClasses = [RoessingId.self]
        let anmeldung = Anmeldung(
            sitzung: URLSession(configuration: k),
            aussteller: Self.aussteller,
            clientId: "test-client",
            ruecksprung: "de.roessing.app:/oauth2redirect",
            ablage: fach.ablage
        )
        return (anmeldung, fach)
    }

    private static let erfolg = RoessingId.Antwort.erfolg(zugang: "neu", erneuerung: "r2")

    // MARK: Was die Rössing-ID wirklich entschieden hat

    @Test func einWiderrufenesErneuerungstokenMeldetAb() async {
        let (anmeldung, fach) = Self.aufbau(Self.abgelaufen(),
                                            [.abgewiesen(status: 400, kuerzel: "invalid_grant")])

        #expect(await anmeldung.frischesToken() == .abgemeldet)
        #expect(anmeldung.sitzung == .abgemeldet)
        // Und der Satz ist weg: Ein widerrufenes Token wieder und wieder zu
        // versuchen, hilft niemandem.
        #expect(fach.satz == nil)
        #expect(fach.geloescht)
    }

    @Test func ohneErneuerungstokenIstDieSitzungWirklichZuEnde() async {
        // Nichts mehr da, womit sich erneuern ließe — das ist kein Umstand.
        let (anmeldung, fach) = Self.aufbau(Self.abgelaufen(erneuerung: nil), [Self.erfolg])

        #expect(await anmeldung.frischesToken() == .abgemeldet)
        #expect(anmeldung.sitzung == .abgemeldet)
        #expect(fach.satz == nil)
        #expect(RoessingId.tokenanfragen == 0, "Ohne Erneuerungstoken gibt es nichts zu fragen")
    }

    // MARK: Was bloß ein Umstand war

    @Test func einFunklochKostetDieAnmeldungNicht() async {
        let (anmeldung, fach) = Self.aufbau(Self.abgelaufen(), [.netzausfall])

        #expect(await anmeldung.frischesToken() == .nichtErreichbar)
        #expect(anmeldung.sitzung == .angemeldet(entwicklerModus: false))
        // Der Satz bleibt unangetastet — samt Erneuerungstoken.
        #expect(fach.satz?.erneuerungstoken == "r1")
        #expect(!fach.geloescht)
    }

    @Test func einServerfehlerKostetDieAnmeldungNicht() async {
        // 5xx ist der Zustand des Servers, nicht sein Urteil über die Sitzung.
        let (anmeldung, fach) = Self.aufbau(Self.abgelaufen(), [.serverfehler(status: 503)])

        #expect(await anmeldung.frischesToken() == .nichtErreichbar)
        #expect(anmeldung.sitzung == .angemeldet(entwicklerModus: false))
        #expect(fach.satz?.erneuerungstoken == "r1")
    }

    @Test func eineAntwortOhneKuerzelKostetDieAnmeldungNicht() async {
        // 400, aber ohne das Kürzel, das RFC 6749 verlangt: Wir wissen nicht,
        // was der Server sagen wollte — dann wird nicht abgemeldet.
        let (anmeldung, fach) = Self.aufbau(Self.abgelaufen(), [.roh(status: 400, rumpf: Data())])

        #expect(await anmeldung.frischesToken() == .nichtErreichbar)
        #expect(anmeldung.sitzung == .angemeldet(entwicklerModus: false))
        #expect(fach.satz?.erneuerungstoken == "r1")
    }

    @Test func einFehlerInUnsererEinrichtungKostetDieAnmeldungNicht() async {
        // `invalid_client` heißt: An unserer Anmeldung ist etwas falsch. Wer
        // deswegen abmeldet, wirft eine gültige Sitzung weg — und die neue
        // Anmeldung scheiterte an derselben Stelle wieder.
        let (anmeldung, _) = Self.aufbau(Self.abgelaufen(),
                                         [.abgewiesen(status: 400, kuerzel: "invalid_client")])

        #expect(await anmeldung.frischesToken() == .nichtErreichbar)
        #expect(anmeldung.sitzung == .angemeldet(entwicklerModus: false))
    }

    @Test func nachDemFunklochGehtEsWeiterOhneNeuanmeldung() async {
        let (anmeldung, fach) = Self.aufbau(Self.abgelaufen(), [.netzausfall, Self.erfolg])

        #expect(await anmeldung.frischesToken() == .nichtErreichbar)
        // Wieder Empfang: derselbe Aufruf, kein Browser, kein Passwort.
        #expect(await anmeldung.frischesToken() == .token("neu"))
        #expect(anmeldung.sitzung == .angemeldet(entwicklerModus: false))
        #expect(fach.satz?.zugangstoken == "neu")
        #expect(fach.satz?.erneuerungstoken == "r2")
    }

    // MARK: Der Kaltstart — mehrere Abrufe, eine Erneuerung

    @Test func gleichzeitigeAbrufeErneuernNurEinMal() async {
        // Die zweite Antwort wäre ein anderes Token; sie darf nie gebraucht
        // werden. Bei Zitadel wäre sie in Wahrheit ein „invalid_grant", weil
        // das Erneuerungstoken mit dem ersten Gebrauch verfällt.
        let (anmeldung, fach) = Self.aufbau(Self.abgelaufen(), [
            Self.erfolg,
            .abgewiesen(status: 400, kuerzel: "invalid_grant"),
        ])

        async let erster = anmeldung.frischesToken()
        async let zweiter = anmeldung.frischesToken()
        let (a, b) = await (erster, zweiter)

        #expect(RoessingId.tokenanfragen == 1, "Zwei Abrufe dürfen nur einmal erneuern")
        #expect(a == .token("neu"))
        #expect(b == .token("neu"))
        #expect(anmeldung.sitzung == .angemeldet(entwicklerModus: false))
        #expect(fach.satz?.erneuerungstoken == "r2")
    }

    @Test func nachDerErneuerungWirdNichtWiederGefragt() async {
        let (anmeldung, _) = Self.aufbau(Self.abgelaufen(), [Self.erfolg])

        #expect(await anmeldung.frischesToken() == .token("neu"))
        #expect(await anmeldung.frischesToken() == .token("neu"))
        #expect(RoessingId.tokenanfragen == 1, "Das frische Token gilt weiter")
    }

    @Test func einGueltigesTokenFragtGarNichtNach() async {
        let gueltig = Tokensatz(zugangstoken: "gilt", erneuerungstoken: "r1", idToken: nil,
                                laeuftAbAm: Date().addingTimeInterval(3600))
        let (anmeldung, _) = Self.aufbau(gueltig, [Self.erfolg])

        #expect(await anmeldung.frischesToken() == .token("gilt"))
        #expect(RoessingId.tokenanfragen == 0)
    }

    // MARK: Die Regel für sich

    @Test func nurEinWiderrufBeendetEineSitzung() {
        #expect(Anmeldung.istSitzungsende("invalid_grant"))
        // Alles andere ist entweder unser Fehler oder der Zustand des Servers.
        #expect(!Anmeldung.istSitzungsende("invalid_client"))
        #expect(!Anmeldung.istSitzungsende("invalid_request"))
        #expect(!Anmeldung.istSitzungsende("server_error"))
        #expect(!Anmeldung.istSitzungsende("temporarily_unavailable"))
        #expect(!Anmeldung.istSitzungsende(""))
    }

    // MARK: Was die Person davon sieht

    @Test func eineNichtErneuerbareAnmeldungMeldetEinNetzproblem() async throws {
        let api = DorfApi(basis: URL(string: "http://127.0.0.1:8099")!,
                          tokenGeber: { .nichtErreichbar })
        do {
            _ = try await api.anfrage("GET", "api/v1/me")
            Issue.record("Ohne Token darf keine Anfrage gebaut werden")
        } catch let fehler as DorfFehler {
            #expect(fehler.klartext == DorfFehler.netz("").klartext)
            // Und ausdrücklich nicht der Satz, der zur Neuanmeldung auffordert:
            // Die Anmeldung gilt ja noch, es fehlt bloß die Verbindung.
            #expect(fehler.klartext != DorfFehler.nichtAngemeldet.klartext)
        }
    }

    @Test func mitTokenGehtDieKopfzeileMit() async throws {
        let api = DorfApi(basis: URL(string: "http://127.0.0.1:8099")!,
                          tokenGeber: { .token("abc") })
        let anfrage = try await api.anfrage("GET", "api/v1/me")
        #expect(anfrage.value(forHTTPHeaderField: "Authorization") == "Bearer abc")
    }

    @Test func ohneVerbindungSagtDieStartseiteWoranEsLiegt() async {
        let (anmeldung, _) = Self.aufbau(Self.abgelaufen(), [.netzausfall])
        let api = DorfApi(basis: URL(string: "http://127.0.0.1:8099")!,
                          tokenGeber: { await anmeldung.frischesToken() })
        let orte = OrteModell(quelle: OrteQuelle(
            orte: { throw DorfFehler.netz("nicht erreichbar") },
            erledigungen: { _ in [] },
            melden: { _, _, _ in throw DorfFehler.netz("nicht erreichbar") },
            zuruecknehmen: { _ in }
        ))
        let umgebung = AppUmgebung(anmeldung: anmeldung, api: api, ich: nil, orte: orte)

        #expect(umgebung.stoerungshinweis == nil, "Vor dem ersten Abruf steht da nichts")
        await orte.laden()

        // Es steht da, dass die Verbindung fehlt — und ausdrücklich nicht,
        // dass man sich neu anmelden soll.
        #expect(umgebung.stoerungshinweis == DorfFehler.netz("").klartext)
        #expect(umgebung.stoerungshinweis != DorfFehler.nichtAngemeldet.klartext)
        // Und die Anmeldung steht noch.
        #expect(anmeldung.sitzung == .angemeldet(entwicklerModus: false))
    }

    @Test func ohneAnmeldungGehtDieAnfrageOhneKopfzeileHinaus() async throws {
        // Die Ideen nimmt das Backend auch ohne Anmeldung an.
        let api = DorfApi(basis: URL(string: "http://127.0.0.1:8099")!,
                          tokenGeber: { .abgemeldet })
        let anfrage = try await api.anfrage("GET", "api/v1/ideen")
        #expect(anfrage.value(forHTTPHeaderField: "Authorization") == nil)
    }
}

/// Die Rössing-ID als Attrappe: Discovery und Token-Endpunkt, örtlich
/// abgefangen. Es geht nichts hinaus, und es wird nirgends gewartet.
private nonisolated final class RoessingId: URLProtocol {
    enum Antwort {
        case erfolg(zugang: String, erneuerung: String?)
        /// Der Server hat geantwortet und abgelehnt — mit Kürzel nach RFC 6749.
        case abgewiesen(status: Int, kuerzel: String)
        case serverfehler(status: Int)
        /// Beliebige Antwort, um auch das Unfertige zu prüfen.
        case roh(status: Int, rumpf: Data)
        /// Gar keine Antwort: kein Netz.
        case netzausfall
    }

    private static let schloss = NSLock()
    nonisolated(unsafe) private static var folge: [Antwort] = []
    nonisolated(unsafe) private static var gezaehlt = 0

    /// Legt fest, was der Token-Endpunkt der Reihe nach antwortet. Die letzte
    /// Antwort gilt weiter, wenn öfter gefragt wird.
    static func aufsetzen(_ antworten: [Antwort]) {
        schloss.lock()
        defer { schloss.unlock() }
        folge = antworten
        gezaehlt = 0
    }

    /// Wie oft der **Token**-Endpunkt gefragt wurde (Discovery zählt nicht).
    static var tokenanfragen: Int {
        schloss.lock()
        defer { schloss.unlock() }
        return gezaehlt
    }

    private static func naechste() -> Antwort {
        schloss.lock()
        defer { schloss.unlock() }
        guard !folge.isEmpty else { return .netzausfall }
        let antwort = folge[min(gezaehlt, folge.count - 1)]
        gezaehlt += 1
        return antwort
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let pfad = request.url?.path ?? ""
        if pfad.hasSuffix("openid-configuration") {
            antworte(200, Data(Self.entdeckung.utf8))
            return
        }
        switch Self.naechste() {
        case .erfolg(let zugang, let erneuerung):
            let erneuerungsfeld = erneuerung.map { ",\"refresh_token\":\"\($0)\"" } ?? ""
            antworte(200, Data("""
            {"access_token":"\(zugang)","expires_in":43200\(erneuerungsfeld)}
            """.utf8))
        case .abgewiesen(let status, let kuerzel):
            antworte(status, Data("{\"error\":\"\(kuerzel)\"}".utf8))
        case .serverfehler(let status):
            antworte(status, Data("{\"error\":\"server_error\"}".utf8))
        case .roh(let status, let rumpf):
            antworte(status, rumpf)
        case .netzausfall:
            client?.urlProtocol(self, didFailWithError: URLError(.notConnectedToInternet))
        }
    }

    override func stopLoading() {}

    private func antworte(_ status: Int, _ rumpf: Data) {
        let kopf = HTTPURLResponse(
            url: request.url ?? URL(string: "http://127.0.0.1:8123")!,
            statusCode: status,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json; charset=utf-8"]
        )!
        client?.urlProtocol(self, didReceive: kopf, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: rumpf)
        client?.urlProtocolDidFinishLoading(self)
    }

    private static let entdeckung = """
    {"authorization_endpoint":"http://127.0.0.1:8123/oauth/v2/authorize",
     "token_endpoint":"http://127.0.0.1:8123/oauth/v2/token",
     "end_session_endpoint":"http://127.0.0.1:8123/oidc/v1/end_session"}
    """
}
