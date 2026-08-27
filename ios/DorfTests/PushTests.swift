import Foundation
import Testing
import UserNotifications

@testable import Dorf

/// Was an den Benachrichtigungen ohne Gerät prüfbar ist — und das ist
/// erstaunlich viel.
///
/// Im Simulator kommt **keine echte APNs-Kennung** an; ein Test, der darauf
/// wartet, wartet für immer. Die Logik ist deshalb von der Systemschicht
/// getrennt: Hex-Wandlung, das Ziel eines Fingertipps, die Erlaubnisfrage und
/// die beiden Anfragen an den Dorfserver kommen alle ohne
/// `UNUserNotificationCenter` und ohne Netz aus.
///
/// **Kein Test hier fasst Apple an.** Weder die Produktions- noch die
/// Sandbox-Adresse von APNs kommt in dieser Datei vor; mit Apple spricht
/// ohnehin nur das Backend (`backend/internal/push/apns.go`).
struct PushTests {
    // MARK: Gerätekennung

    /// Der Fehler, an dem die meisten iOS-Push-Anbindungen scheitern: Apple
    /// liefert `Data`, und wer daraus mit `description` eine Zeichenkette
    /// macht, schickt „<8a5f1c2d 3e4b5a69>" ans Backend.
    @Test func kennungWirdReinesHexOhneKlammernUndLeerzeichen() {
        let roh = Data([0x8a, 0x5f, 0x1c, 0x2d, 0x3e, 0x4b, 0x5a, 0x69])
        let kennung = Geraetekennung.hex(roh)

        #expect(kennung == "8a5f1c2d3e4b5a69")
        #expect(!kennung.contains("<"))
        #expect(!kennung.contains(">"))
        #expect(!kennung.contains(" "))
        // Und ausdrücklich: nicht das, was Data von sich aus liefert.
        #expect(kennung != String(describing: roh))
        #expect(kennung != roh.description)
    }

    @Test func fuehrendeNullenBleibenErhalten() {
        // Ein Byte wird immer zu zwei Zeichen — sonst verschiebt sich alles
        // dahinter, und Apple sieht eine völlig andere Kennung.
        #expect(Geraetekennung.hex(Data([0x00, 0x01, 0x0f, 0xff])) == "00010fff")
        #expect(Geraetekennung.hex(Data([0x00])).count == 2)
    }

    @Test func eineEchteKennungHatVollstaendigeLaenge() {
        // APNs-Kennungen sind heute 32 Byte lang; als Hex also 64 Zeichen.
        let roh = Data((0 ..< 32).map { UInt8($0) })
        let kennung = Geraetekennung.hex(roh)
        #expect(kennung.count == 64)
        #expect(Geraetekennung.istBrauchbar(kennung))
    }

    @Test func leereKennungIstNichtBrauchbar() {
        #expect(Geraetekennung.hex(Data()) == "")
        #expect(!Geraetekennung.istBrauchbar(""))
    }

    @Test func unbrauchbareKennungenWerdenErkannt() {
        // Genau die Formen, die versehentlich entstehen.
        #expect(!Geraetekennung.istBrauchbar("<8a5f1c2d 3e4b5a69>"))
        #expect(!Geraetekennung.istBrauchbar("8a5f 1c2d"))
        #expect(!Geraetekennung.istBrauchbar("abc")) // ungerade Länge
        #expect(!Geraetekennung.istBrauchbar("cXyZ:APA91b")) // Firebase
        #expect(Geraetekennung.istBrauchbar("DEADBEEF")) // Großschreibung ist in Ordnung
    }

    // MARK: Anfragen an den Dorfserver

    /// Ein Zugang mit fester Adresse und festem Token — gebaut wird die
    /// Anfrage vom **gemeinsamen** Transport (`DorfApi.anfrage`), damit hier
    /// geprüft wird, was tatsächlich hinausginge. Geschickt wird nichts.
    private static func zugang(token: String?) -> DorfApi {
        DorfApi(basis: testBasis, tokenGeber: { token.map { .token($0) } ?? .abgemeldet })
    }

    private static func geraeteanfrage(_ methode: String, token: String?) async throws -> URLRequest {
        try await zugang(token: token).anfrage(
            methode, DorfApi.geraetePfad, rumpf: GeraetEingabe(kennung: "8a5f1c2d"))
    }

    @Test func anmeldungSchicktKennungUndPlattform() async throws {
        let anfrage = try await Self.geraeteanfrage("POST", token: "tok-123")

        #expect(anfrage.httpMethod == "POST")
        #expect(anfrage.url?.path == "/api/v1/me/devices")
        #expect(anfrage.value(forHTTPHeaderField: "Authorization") == "Bearer tok-123")

        let rumpf = try #require(anfrage.httpBody)
        let gelesen = try #require(
            try JSONSerialization.jsonObject(with: rumpf) as? [String: String])
        #expect(gelesen["token"] == "8a5f1c2d")
        #expect(gelesen["platform"] == "ios")
        // Genau zwei Felder — das Backend kennt nicht mehr.
        #expect(gelesen.count == 2)
    }

    @Test func abmeldungSchicktDenselbenRumpf() async throws {
        let an = try await Self.geraeteanfrage("POST", token: "tok")
        let ab = try await Self.geraeteanfrage("DELETE", token: "tok")

        #expect(ab.httpMethod == "DELETE")
        #expect(ab.url == an.url)
        // Verglichen wird der Inhalt, nicht die Bytefolge: JSONEncoder ordnet
        // die Felder nicht garantiert gleich an.
        #expect(try Self.rumpf(ab) == Self.rumpf(an))
    }

    /// Liest den JSON-Rumpf einer Anfrage als Wörterbuch.
    private static func rumpf(_ anfrage: URLRequest) throws -> [String: String] {
        let roh = try #require(anfrage.httpBody)
        return try #require(try JSONSerialization.jsonObject(with: roh) as? [String: String])
    }

    @Test func ohneTokenGehtKeineAutorisierungMit() async throws {
        // Nicht angemeldet: Der Server weist das ab (401), und das ist auch
        // richtig so — eine Kennung ohne Person hätte er nirgends abzulegen.
        let anfrage = try await Self.geraeteanfrage("POST", token: nil)
        #expect(anfrage.value(forHTTPHeaderField: "Authorization") == nil)
    }

    /// Eine lokale Adresse: Kein Test darf gegen einen entfernten Server
    /// laufen (siehe `.github/scripts/pruefe_lokale_tests.py`).
    private static let testBasis = URL(string: "http://127.0.0.1:8099")!

    // MARK: Ziel eines Fingertipps

    @Test func tippFuehrtZurAufgabe() throws {
        // So sieht die Nutzlast aus, die das Backend schickt — alle Werte
        // sind Zeichenketten (backend/internal/push/fcm.go, daten()).
        let ziel = try #require(PushZiel.ausDaten([
            "placeId": "2", "taskId": "5", "assignmentId": "3", "notificationId": "7",
            "kind": "anfrage", "taskKind": "giessen",
            "placeName": "Unter den Eichen", "taskName": "Gießen",
            "title": "Gießen ist dran", "body": "Du bist als Nächste(r) an der Reihe.",
        ]))

        #expect(ziel.ortId == 2)
        #expect(ziel.aufgabeId == 5)
        #expect(ziel.vorgangId == 3)
        #expect(ziel.meldungId == 7)
        #expect(ziel.ortsname == "Unter den Eichen")
        #expect(ziel.istAnfrage)
        #expect(ziel.kanal == .anfragen)
    }

    @Test func ohneOrtGibtEsNichtsAnzuspringen() {
        #expect(PushZiel.ausDaten([:]) == nil)
        #expect(PushZiel.ausDaten(["kind": "anfrage"]) == nil)
        #expect(PushZiel.ausDaten(["placeId": "0"]) == nil)
        #expect(PushZiel.ausDaten(["placeId": "keine Zahl"]) == nil)
    }

    @Test func fehlendeFelderKostenDasZielNicht() throws {
        let ziel = try #require(PushZiel.ausDaten(["placeId": "2", "kind": "vorgang_beendet"]))
        #expect(ziel.aufgabeId == 0)
        #expect(ziel.ortsname == "")
        #expect(!ziel.istAnfrage)
        #expect(ziel.kanal == .hinweise)
    }

    @Test func rundrufIstAuchEineAnfrage() throws {
        let ziel = try #require(PushZiel.ausDaten(["placeId": "2", "kind": "rundruf"]))
        #expect(ziel.istAnfrage)
        #expect(ziel.kanal == .anfragen)
    }

    @Test func hinweiseGehenInDenLeisenKanal() throws {
        for art in ["zusage_abgelaufen", "zusage_aufgehoben", "vorgang_beendet", "vorgang_entfallen"] {
            let ziel = try #require(PushZiel.ausDaten(["placeId": "2", "kind": art]))
            #expect(ziel.kanal == .hinweise, "\(art) gehört zu den Hinweisen")
        }
    }

    @Test func echteZahlenWerdenAuchGelesen() throws {
        // Robustheit: Käme eine Zahl doch einmal als JSON-Zahl an, soll die
        // Meldung nicht ins Leere führen.
        let ziel = try #require(PushZiel.ausDaten(["placeId": 2, "taskId": 5]))
        #expect(ziel.ortId == 2)
        #expect(ziel.aufgabeId == 5)
    }

    // MARK: Kanäle

    @Test func esGibtGenauZweiKanaeleMitDenBezeichnernDesBackends() {
        // Dieselben Zeichenketten wie in backend/internal/push/fcm.go
        // (KanalAnfragen/KanalHinweise) und wie die Kanal-IDs auf Android.
        #expect(PushKanal.allCases.count == 2)
        #expect(PushKanal.anfragen.rawValue == "anfragen")
        #expect(PushKanal.hinweise.rawValue == "hinweise")
        for kanal in PushKanal.allCases {
            #expect(!kanal.bezeichnung.isEmpty)
            #expect(!kanal.beschreibung.isEmpty)
        }
    }

    // MARK: Erlaubnis

    @Test func ohneAntwortWirdNichtAngemeldet() {
        // Vor der Frage und nach einem Nein entsteht keine Gerätekennung —
        // sonst läge im Server eine Kennung von jemandem, der ausdrücklich
        // abgelehnt hat.
        #expect(!Benachrichtigungserlaubnis.wirksam(.notDetermined))
        #expect(!Benachrichtigungserlaubnis.wirksam(.denied))
    }

    @Test func erteilteErlaubnisZaehlt() {
        #expect(Benachrichtigungserlaubnis.wirksam(.authorized))
        // Die stille Zustellung ohne Nachfrage liefert echte Meldungen aus —
        // dafür braucht das Backend auch eine Kennung.
        #expect(Benachrichtigungserlaubnis.wirksam(.provisional))
        #expect(Benachrichtigungserlaubnis.wirksam(.ephemeral))
    }
}
