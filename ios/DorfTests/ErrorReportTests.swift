import Foundation
import Testing

@testable import Dorf

/// Fehlerberichte: Wenn etwas nicht klappt, sieht die Person das — und kann
/// mit einem Fingertipp einen Bericht schicken, ohne etwas zu beschreiben.
///
/// Geprüft wird das, woran es hängt: dass eine Regel, die greift, **kein**
/// Bericht wird (sonst ersäuft die echte Störung im Rauschen), dass ein
/// Absturz beim nächsten Start auftaucht, dass ein absichtliches Beenden
/// keiner ist, und dass wirklich nur das hinausgeht, was im Blatt steht.
///
/// Kein Test geht ins Netz: Die Sitzung wird über `protocolClasses`
/// abgefangen.
@MainActor
struct ErrorReportTests {
    private static let basis = URL(string: "http://127.0.0.1:8099")!

    private static func angaben() -> Geraeteangaben {
        Geraeteangaben(appVersion: "0.1.10 (42)", osVersion: "iOS 18.5", deviceModel: "iPhone14,3")
    }

    /// Ein Melder mit abgefangener Sitzung — er schickt nichts hinaus.
    private static func melder(status: Int = 201, rumpf: String = #"{"id":7,"status":"new"}"#)
        -> ErrorReporter
    {
        Berichtsablage.antwort = (status, Data(rumpf.utf8))
        Berichtsablage.letzteAnfrage = nil
        let k = URLSessionConfiguration.ephemeral
        k.protocolClasses = [Berichtsablage.self]
        let api = DorfApi(basis: basis, sitzung: URLSession(configuration: k), tokenGeber: { .abgemeldet })
        let melder = ErrorReporter(angaben: angaben())
        melder.verdrahten(api: api)
        return melder
    }

    // MARK: Was gemeldet wird — und was nicht

    @Test func eineRegelDieGreiftIstKeinFehler() {
        // Diese Abweisungen sind das Backend bei der Arbeit. Sie stehen dort,
        // wo sie hingehören, und ein Bericht darüber wäre Rauschen.
        let regeln: [DorfFehler] = [
            .abgelehnt(grund: "Bitte schreib mindestens fünf Zeichen."),
            .keineBerechtigung(grund: "Dafür fehlt die Berechtigung."),
            .schonVergeben(grund: "Das hat jemand anderes übernommen."),
            .gesperrt(wiederAb: nil),
            .zuVieleAnfragen,
            .nichtAngemeldet,
        ]
        for regel in regeln {
            #expect(ErrorIncident.aus(regel, pfad: "api/v1/places") == nil,
                    "\(regel) darf keinen Bericht auslösen")
        }
    }

    @Test func eineStoerungWirdGemeldetUndBehaeltDenWortlaut() throws {
        let vorfall = try #require(ErrorIncident.aus(
            .serverfehler(status: 500), pfad: "api/v1/places", methode: "GET"
        ))
        #expect(vorfall.kind == .server)
        // Der Satz ist der, den die Person gelesen hat — die App erfindet
        // keinen zweiten für den Bericht.
        #expect(vorfall.message == DorfFehler.serverfehler(status: 500).klartext)
        #expect(vorfall.detail.contains("HTTP 500"))
        #expect(vorfall.detail.contains("GET api/v1/places"))
        #expect(vorfall.area == "Mithelfen")

        let netz = try #require(ErrorIncident.aus(.netz("Zeitüberschreitung"), pfad: "api/v1/me"))
        #expect(netz.kind == .network)
        #expect(netz.area == "Konto")
    }

    @Test func derBereichKommtInAlltagsspracheAn() {
        // „api/v1/places" sagt dem Dorfentwicklungskreis nichts.
        #expect(Bereichsnamen.zu(pfad: "api/v1/places") == "Mithelfen")
        #expect(Bereichsnamen.zu(pfad: "/api/v1/tasks/3/completions") == "Mithelfen")
        #expect(Bereichsnamen.zu(pfad: "api/v1/stats/leaderboard") == "Rangliste")
        #expect(Bereichsnamen.zu(pfad: "api/v1/members") == "Dorfbewohner")
        // Der längere Treffer gewinnt: `me/devices` ist nicht `me`.
        #expect(Bereichsnamen.zu(pfad: "api/v1/me/devices") == "Benachrichtigungen")
        #expect(Bereichsnamen.zu(pfad: "api/v1/me/profile") == "Mein Profil")
        #expect(Bereichsnamen.zu(pfad: "api/v1/me") == "Konto")
        #expect(Bereichsnamen.zu(pfad: "api/v1/voellig/neu") == "App")
    }

    @Test func einGescheiterterBerichtMeldetSichNichtSelbst() {
        // Sonst drehte sich das im Kreis: Jeder Fehlversuch erzeugte einen
        // neuen Bericht, der wieder scheitert.
        let melder = Self.melder()
        melder.beobachte(.serverfehler(status: 500), methode: "POST",
                         pfad: "/api/v1/error-reports")
        #expect(melder.vorfall == nil)
    }

    // MARK: Ein Fingertipp

    @Test func einFingertippSchicktDenBericht() async throws {
        let melder = Self.melder()
        melder.beobachte(.serverfehler(status: 500), methode: "GET", pfad: "api/v1/places")
        let vorfall = try #require(melder.vorfall)
        #expect(vorfall.kind == .server)

        // Genau ein Knopfdruck, kein getippter Text.
        await melder.absenden()

        #expect(melder.gesendet)
        #expect(melder.sendefehler == nil)
        let anfrage = try #require(Berichtsablage.letzteAnfrage)
        #expect(anfrage.httpMethod == "POST")
        #expect(anfrage.url?.path == "/api/v1/error-reports")

        let rumpf = try #require(Berichtsablage.letzterRumpf)
        let gesendet = try JSONDecoder().decode(ErrorReportInput.self, from: rumpf)
        #expect(gesendet.kind == "server")
        #expect(gesendet.message == vorfall.message)
        #expect(gesendet.comment.isEmpty, "Ohne Tippen geht auch kein Text hinaus")
        #expect(gesendet.platform == "ios")
        #expect(gesendet.appVersion == "0.1.10 (42)")
        #expect(gesendet.deviceModel == "iPhone14,3")
        #expect(gesendet.area == "Mithelfen")
        #expect(!gesendet.occurredAt.isEmpty)
    }

    @Test func werEtwasDazuschreibtWirdGehoert() async throws {
        let melder = Self.melder()
        melder.melde(ErrorIncident(kind: .crash, message: "Die App hat sich beendet."))

        await melder.absenden(kommentar: "  Ich wollte gerade das Gießen melden.  ")

        let rumpf = try #require(Berichtsablage.letzterRumpf)
        let gesendet = try JSONDecoder().decode(ErrorReportInput.self, from: rumpf)
        #expect(gesendet.comment == "Ich wollte gerade das Gießen melden.")
        #expect(gesendet.kind == "crash")
    }

    @Test func esGehtNurHinausWasImBlattSteht() throws {
        let melder = Self.melder()
        let vorfall = ErrorIncident(
            kind: .server, message: "Der Server antwortet gerade nicht (500).",
            detail: "HTTP 500 · GET api/v1/places", area: "Mithelfen"
        )
        melder.melde(vorfall)

        let eingabe = melder.eingabeFuer(vorfall, kommentar: "Nur ein Satz.")
        let liste = melder.inhaltsliste(eingabe)
        let gezeigt = liste.map(\.1).joined(separator: "\n")

        // Was im Blatt steht, steht auch in der Anfrage — und umgekehrt.
        for wert in [eingabe.message, eingabe.detail, eingabe.comment,
                     eingabe.area, eingabe.appVersion, eingabe.deviceModel] {
            #expect(gezeigt.contains(wert), "\(wert) fehlt in der Aufstellung")
        }

        // Und was nicht mitgeht, geht auch wirklich nicht mit.
        let alsJson = String(data: try JSONEncoder().encode(eingabe), encoding: .utf8) ?? ""
        for verboten in ["identifierForVendor", "latitude", "longitude"] {
            #expect(!alsJson.contains(verboten))
        }
    }

    @Test func einGescheitertesAbschickenWirdGesagtNichtVerschluckt() async {
        let melder = Self.melder(status: 500, rumpf: "{}")
        melder.melde(ErrorIncident(kind: .crash, message: "Die App hat sich beendet."))

        await melder.absenden()

        #expect(!melder.gesendet)
        #expect(melder.sendefehler?.isEmpty == false)
        // Der Vorfall bleibt stehen — sonst wäre der Bericht weg, ohne
        // angekommen zu sein.
        #expect(melder.vorfall != nil)
    }

    @Test func schliessenIstEineGueltigeAntwort() {
        let melder = Self.melder()
        melder.melde(ErrorIncident(kind: .network, message: "Keine Verbindung."))
        melder.schliessen()
        #expect(melder.vorfall == nil)
    }

    // MARK: Abstürze

    @Test func einAbsturzTauchtBeimNaechstenStartAuf() throws {
        let d = try #require(UserDefaults(suiteName: "test-absturz-\(UUID().uuidString)"))
        // Die App war im Vordergrund und ist nicht ordentlich beendet worden.
        CrashWatch.imVordergrund(d)

        let vorfall = try #require(CrashWatch.offenerAbsturz(d))
        #expect(vorfall.kind == .crash)
        #expect(vorfall.message.contains("unerwartet beendet"))
        #expect(vorfall.area == "Absturz")

        // Und nur einmal: Derselbe Absturz wird nicht zweimal angeboten.
        #expect(CrashWatch.offenerAbsturz(d) == nil)
    }

    @Test func einAbsichtlichesBeendenIstKeinAbsturz() throws {
        let d = try #require(UserDefaults(suiteName: "test-beenden-\(UUID().uuidString)"))
        CrashWatch.imVordergrund(d)
        // Wer die App wegwischt, geht über den Umschalter — und der schiebt
        // sie vorher in den Hintergrund.
        CrashWatch.imHintergrund(d)

        #expect(CrashWatch.offenerAbsturz(d) == nil)
    }

    @Test func einSaubererErsterStartMeldetNichts() throws {
        let d = try #require(UserDefaults(suiteName: "test-erststart-\(UUID().uuidString)"))
        #expect(CrashWatch.offenerAbsturz(d) == nil)
    }

    @Test func einAbsturzZeitpunktLiegtNieInDerZukunft() throws {
        let d = try #require(UserDefaults(suiteName: "test-uhr-\(UUID().uuidString)"))
        CrashWatch.imVordergrund(d)
        let frueher = Date(timeIntervalSince1970: 1000)

        let vorfall = try #require(CrashWatch.offenerAbsturz(d, jetzt: frueher))
        #expect(vorfall.occurredAt <= frueher)
    }

    // MARK: Angaben über das Gerät

    @Test func dieAngabenSindVersionUndGeraetetypUndSonstNichts() {
        let bundle = Bundle(for: Marker.self)
        let angaben = Geraeteangaben.aktuell(
            bundle: bundle, systemVersion: "18.5", maschine: "iPhone14,3"
        )
        #expect(angaben.osVersion == "iOS 18.5")
        #expect(angaben.deviceModel == "iPhone14,3")
        // Der Gerätetyp hilft beim Nachstellen; eine Gerätekennung täte das
        // nicht und verfolgte nur eine Person.
        #expect(!angaben.deviceModel.contains("-"))
    }
}

/// Nur zum Auffinden des Test-Bundles.
private final class Marker {}

/// Örtliche Ablage statt Netz: Die Anfrage wird abgefangen und beantwortet,
/// ohne das Gerät zu verlassen. Kein Test darf nach draußen.
private nonisolated final class Berichtsablage: URLProtocol {
    nonisolated(unsafe) static var antwort: (Int, Data) = (201, Data())
    nonisolated(unsafe) static var letzteAnfrage: URLRequest?
    nonisolated(unsafe) static var letzterRumpf: Data?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        Berichtsablage.letzteAnfrage = request
        Berichtsablage.letzterRumpf = request.httpBody ?? request.httpBodyStream.map(Self.gelesen)
        let (status, daten) = Berichtsablage.antwort
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

    /// `URLSession` verwandelt einen gesetzten Rumpf unterwegs in einen
    /// Strom; ohne dieses Auslesen wäre er im Test nicht zu sehen.
    private static func gelesen(_ strom: InputStream) -> Data {
        strom.open()
        defer { strom.close() }
        var daten = Data()
        var puffer = [UInt8](repeating: 0, count: 4096)
        while strom.hasBytesAvailable {
            let gelesen = strom.read(&puffer, maxLength: puffer.count)
            if gelesen <= 0 { break }
            daten.append(contentsOf: puffer[0 ..< gelesen])
        }
        return daten
    }
}
