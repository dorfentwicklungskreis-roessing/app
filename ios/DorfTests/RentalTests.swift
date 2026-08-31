import Foundation
import Testing

@testable import Dorf

/// Der Vertrag mit der Mietplattform („Maschinchenring"): So sieht ihre
/// Antwort aus, und so wird sie gelesen.
///
/// Die Beispiele stammen wörtlich aus `docs/mieten-api.md` — der einzigen
/// Quelle für diesen Bereich. Ändert sich die Form dort, muss dieser Test
/// auffallen und nicht erst eine leere Liste auf dem Telefon.
///
/// Kein Test geht ins Netz: Geprüft wird die Aufbereitung, und wo ein Client
/// gebraucht wird, bekommt er eine eigene Basis und eine örtliche Ablage
/// untergeschoben.
struct RentalTests {
    // MARK: Beispiele aus dem Vertrag

    static let itemsJson = """
    {
      "items": [
        {
          "id": "as-585-km-kreiselmaeher",
          "name": "AS 585 KM Kreiselmäher",
          "description": "Kreiselmäher für hohes Gras und Böschungen.\\n\\n- Arbeitsbreite 85 cm\\n- Benzin, Radantrieb",
          "pricePerDay": 25,
          "pricePerWeekend": 40,
          "pricePerWeek": 120,
          "deposit": 100,
          "tags": ["garten", "motorgeraet"],
          "thumbnailUrl": "https://cdn.example.invalid/front.jpg",
          "productUrl": "https://www.example.invalid/as-585",
          "webUrl": "https://example.invalid/geraete/as-585-km-kreiselmaeher/"
        },
        {
          "id": "018f2c1a-7b3d-4e91-9a0c-2f5d8e6b4a11",
          "name": "Rasenwalze",
          "description": null,
          "pricePerDay": 8,
          "pricePerWeekend": null,
          "pricePerWeek": null,
          "deposit": null,
          "tags": ["garten"],
          "thumbnailUrl": null,
          "productUrl": null,
          "webUrl": "https://example.invalid/geraete/018f2c1a-7b3d-4e91-9a0c-2f5d8e6b4a11/"
        }
      ]
    }
    """

    static let occupancyJson = """
    {
      "periods": [
        {
          "deviceId": "as-585-km-kreiselmaeher",
          "setId": null,
          "startDate": "2026-09-05",
          "endDate": "2026-09-07",
          "status": "approved"
        },
        {
          "deviceId": "as-585-km-kreiselmaeher",
          "setId": null,
          "startDate": "2026-09-12",
          "endDate": "2026-09-14",
          "status": "pending"
        },
        {
          "deviceId": "as-585-km-kreiselmaeher",
          "setId": null,
          "startDate": "2026-10-01",
          "endDate": "2026-10-08",
          "status": "blocked"
        },
        {
          "deviceId": "rasenwalze",
          "setId": null,
          "startDate": "2026-09-05",
          "endDate": "2026-09-06",
          "status": "approved"
        }
      ]
    }
    """

    static let myBookingsJson = """
    {
      "bookings": [
        {
          "id": "8f14c2b0-91ae-4c77-b1b2-0a3d5e7c9f01",
          "deviceId": "as-585-km-kreiselmaeher",
          "setId": null,
          "deviceName": "AS 585 KM Kreiselmäher",
          "startDate": "2026-09-05",
          "endDate": "2026-09-07",
          "status": "approved",
          "notes": "Hole ich Samstag früh ab.",
          "canCancel": true,
          "pickup": {
            "address": "Hauptstraße 1, 31171 Nordstemmen",
            "phone": "+49 5069 123456"
          }
        },
        {
          "id": "1c9e77a3-1234-4bb8-9a0e-77ce31d2a456",
          "deviceId": "018f2c1a-7b3d-4e91-9a0c-2f5d8e6b4a11",
          "setId": null,
          "deviceName": "Rasenwalze",
          "startDate": "2026-10-01",
          "endDate": "2026-10-03",
          "status": "pending",
          "notes": null,
          "canCancel": true,
          "pickup": null
        }
      ]
    }
    """

    static let profileJson = """
    {
      "profile": {
        "name": "Erika Musterfrau",
        "email": "erika@example.de",
        "phone": "+49 5069 123456",
        "addressStreet": "Hauptstraße 1",
        "addressZip": "31171",
        "addressCity": null,
        "lender": false,
        "lenderStatus": "none",
        "profileComplete": false,
        "missingFields": ["addressCity"]
      }
    }
    """

    static func decode<T: Decodable>(_ text: String) throws -> T {
        try JSONDecoder().decode(T.self, from: Data(text.utf8))
    }

    static func day(_ text: String) throws -> Date {
        try #require(RentalDay.parse(text))
    }

    // MARK: Die Antworten der Mietplattform

    @Test func dieGeraetelisteWirdVollstaendigGelesen() throws {
        let dto: RentalItemsDto = try Self.decode(Self.itemsJson)
        #expect(dto.items.count == 2)

        let devices = dto.items.asDevices()
        // Die Reihenfolge der Plattform bleibt — sie sortiert nach Namen,
        // die Suche nach Passung. Ein zweites Sortieren hier würfe das weg.
        #expect(devices.map(\.id) == [
            "as-585-km-kreiselmaeher",
            "018f2c1a-7b3d-4e91-9a0c-2f5d8e6b4a11",
        ])

        let maeher = try #require(devices.first)
        #expect(maeher.name == "AS 585 KM Kreiselmäher")
        #expect(maeher.tags == ["garten", "motorgeraet"])
        #expect(maeher.thumbnailURL?.absoluteString == "https://cdn.example.invalid/front.jpg")
        #expect(maeher.webURL != nil)
        #expect(maeher.productURL != nil)

        // Jeder Tarif eine Zeile — und keine Summe. Welcher Tarif für welche
        // Dauer gilt, steht nirgends; eine erfundene Regel wäre der Bruch
        // zwischen Web und App. (`\u{00A0}` ist das geschützte Leerzeichen
        // vor dem Eurozeichen, damit der Preis nicht umbricht.)
        #expect(maeher.tariffs == [
            "25,00\u{00A0}€ pro Tag", "40,00\u{00A0}€ pro Wochenende", "120,00\u{00A0}€ pro Woche",
        ])
        #expect(maeher.depositText == "Kaution 100,00\u{00A0}€")

        // Fehlende Tarife sind nicht null-gleich-0: Sie kommen gar nicht vor.
        let walze = devices[1]
        #expect(walze.tariffs == ["8,00\u{00A0}€ pro Tag"])
        #expect(walze.depositText == nil)
        #expect(walze.thumbnailURL == nil)
        #expect(walze.blocks.isEmpty)
    }

    @Test func einFehlendesFeldKostetNichtDieGanzeListe() throws {
        // Die Plattform darf etwas ergänzen oder weglassen, ohne dass eine
        // ältere App-Fassung leer dasteht.
        let dto: RentalItemsDto = try Self.decode("""
        {"items": [{"id": "x", "name": "Bohrhammer", "somethingNew": 7}]}
        """)
        let devices = dto.items.asDevices()
        #expect(devices.count == 1)
        #expect(devices[0].tariffs.isEmpty)

        // Ein Eintrag ohne Kennung wäre eine Zeile, die niemand öffnen kann.
        let ohneId: RentalItemsDto = try Self.decode("""
        {"items": [{"name": "Namenlos"}]}
        """)
        #expect(ohneId.items.asDevices().isEmpty)
    }

    @Test func dasGeraetImDetailBringtSeineBilder() throws {
        let dto: RentalItemEnvelopeDto = try Self.decode("""
        {"item": {"id": "a", "name": "Kreiselmäher", "thumbnailUrl": "https://cdn.example.invalid/front.jpg",
          "images": [
            {"id": "img-7f3a", "url": "https://cdn.example.invalid/front.jpg", "isThumbnail": true},
            {"id": "img-9b21", "url": "https://cdn.example.invalid/seite.jpg", "isThumbnail": false}
          ]}}
        """)
        let device = try #require(dto.item.asDevice())
        // Das Titelbild steht vorn und nicht zweimal.
        #expect(device.images.map(\.absoluteString) == [
            "https://cdn.example.invalid/front.jpg",
            "https://cdn.example.invalid/seite.jpg",
        ])
    }

    @Test func setsHabenKlartextUndKeinBild() throws {
        let dto: RentalSetsDto = try Self.decode("""
        {"sets": [{"id": "gartenset", "name": "Gartenset",
          "description": "Vertikutierer, Rasenwalze und Streuwagen zusammen.",
          "pricePerDay": 30, "deposit": 150,
          "itemIds": ["a", "vertikutierer", "streuwagen"]}]}
        """)
        let sets = dto.sets.asSets()
        #expect(sets.count == 1)
        #expect(sets[0].tariffs == ["30,00\u{00A0}€ pro Tag"])
        #expect(sets[0].depositText == "Kaution 150,00\u{00A0}€")
        #expect(sets[0].itemIds.count == 3)
    }

    // MARK: Zeiträume sind halboffen

    @Test func derRueckgabetagGehoertNichtMehrDazu() throws {
        // „Eine Buchung vom 05. bis 07. belegt den 5. und den 6."
        #expect(RentalDay.occupiedText(startDate: "2026-09-05", endDate: "2026-09-07")
            == "Sa, 05.09.2026 – So, 06.09.2026")
        // Ein einziger Tag bleibt ein einziger Tag.
        #expect(RentalDay.occupiedText(startDate: "2026-09-05", endDate: "2026-09-06")
            == "Sa, 05.09.2026")
        #expect(RentalDay.returnText(endDate: "2026-09-07") == "Rückgabe: Mo, 07.09.2026")
    }

    @Test func ausDemLetztenLeihtagWirdDerRueckgabetag() throws {
        let letzter = try Self.day("2026-09-06")
        #expect(RentalDay.api(RentalDay.nextDay(letzter)) == "2026-09-07")
        // Und die Datumsangabe bleibt, was die Plattform schreibt.
        #expect(RentalDay.api(try Self.day("2026-09-05")) == "2026-09-05")
    }

    @Test func unlesbareTageKostenNichtDieAnzeige() {
        #expect(RentalDay.parse("") == nil)
        #expect(RentalDay.parse("übermorgen") == nil)
        // Statt einer Lücke steht da, was die Plattform geschickt hat.
        #expect(RentalDay.occupiedText(startDate: "kaputt", endDate: "auch") == "kaputt – auch")
    }

    @Test func belegtIstBelegt() throws {
        let dto: RentalOccupancyDto = try Self.decode(Self.occupancyJson)
        let periods = dto.periods.occupied(
            deviceId: "as-585-km-kreiselmaeher", now: try Self.day("2026-09-01")
        )
        #expect(periods.count == 3)
        // Angefragt, vergeben, gesperrt: drei Wörter, eine Bedeutung.
        #expect(periods.map(\.kind) == [.approved, .pending, .blocked])
        #expect(periods[0].text == "Sa, 05.09.2026 – So, 06.09.2026")

        // Ein fremdes Gerät gehört nicht in diese Liste.
        #expect(!periods.contains { $0.id.contains("rasenwalze") })
    }

    @Test func vergangeneZeitraeumeVerschwinden() throws {
        let dto: RentalOccupancyDto = try Self.decode(Self.occupancyJson)
        // Am Rückgabetag ist der Zeitraum vorbei: Wer will, fängt heute an.
        let periods = dto.periods.occupied(
            deviceId: "as-585-km-kreiselmaeher", now: try Self.day("2026-09-07")
        )
        #expect(periods.count == 2)
        #expect(periods.first?.kind == .pending)
    }

    // MARK: Markdown

    @Test func dieBeschreibungWirdNichtRohMitSternchenGezeigt() {
        let blocks = RentalMarkdown.blocks("""
        Kreiselmäher für hohes Gras und Böschungen.

        ## Technisches
        - Arbeitsbreite 85 cm
        - **Benzin**, Radantrieb
        """)
        #expect(blocks.count == 4)
        guard case .paragraph(let ersterAbsatz) = blocks[0] else {
            Issue.record("Der erste Block ist kein Absatz")
            return
        }
        #expect(String(ersterAbsatz.characters) == "Kreiselmäher für hohes Gras und Böschungen.")

        guard case .heading(let level, let ueberschrift) = blocks[1] else {
            Issue.record("Der zweite Block ist keine Überschrift")
            return
        }
        #expect(level == 2)
        #expect(String(ueberschrift.characters) == "Technisches")

        guard case .bullet(let ersterPunkt) = blocks[2] else {
            Issue.record("Der dritte Block ist kein Aufzählungspunkt")
            return
        }
        #expect(String(ersterPunkt.characters) == "Arbeitsbreite 85 cm")

        guard case .bullet(let zweiterPunkt) = blocks[3] else {
            Issue.record("Der vierte Block ist kein Aufzählungspunkt")
            return
        }
        // Die Sternchen sind weg, das Wort steht noch da.
        #expect(String(zweiterPunkt.characters) == "Benzin, Radantrieb")
    }

    @Test func ohneBeschreibungGibtEsKeinenBlock() {
        #expect(RentalMarkdown.blocks(nil).isEmpty)
        #expect(RentalMarkdown.blocks("   \n  ").isEmpty)
    }

    // MARK: Meine Buchungen

    @Test func meineBuchungenStehenInDerReihenfolgeDieZaehlt() throws {
        let dto: RentalBookingsDto = try Self.decode(Self.myBookingsJson)
        let bookings = dto.bookings.asBookings(now: try Self.day("2026-09-01"))
        #expect(bookings.map(\.deviceName) == ["AS 585 KM Kreiselmäher", "Rasenwalze"])

        let erste = try #require(bookings.first)
        #expect(erste.state == .approved)
        #expect(erste.periodText == "Sa, 05.09.2026 – So, 06.09.2026")
        #expect(erste.returnText == "Rückgabe: Mo, 07.09.2026")
        #expect(erste.canCancel)
        // Die Abholadresse steht erst nach der Zusage da — und sonst nirgends.
        #expect(erste.pickupAddress == "Hauptstraße 1, 31171 Nordstemmen")
        #expect(erste.pickupPhone == "+49 5069 123456")
        #expect(bookings[1].pickupAddress == nil)
    }

    @Test func abgelaufeneBuchungenRutschenNachHinten() throws {
        let dto: RentalBookingsDto = try Self.decode(Self.myBookingsJson)
        let bookings = dto.bookings.asBookings(now: try Self.day("2026-09-20"))
        // Die vergangene Buchung steht hinten, die kommende vorn.
        #expect(bookings.map(\.deviceName) == ["Rasenwalze", "AS 585 KM Kreiselmäher"])
    }

    @Test func einUnbekannterZustandBehaeltSeinWort() {
        #expect(RentalBookingState(raw: "pending") == .pending)
        #expect(RentalBookingState(raw: "cancelled") == .cancelled)
        // Nichts wird stillschweigend in einen unserer Zustände gepresst.
        #expect(RentalBookingState(raw: "expired") == .other("expired"))
        #expect(RentalBookingState(raw: "expired").label == "expired")
    }

    // MARK: Profil

    @Test func dasProfilSagtSelbstWasIhmFehlt() throws {
        let dto: RentalProfileEnvelopeDto = try Self.decode(Self.profileJson)
        let profile = dto.profile.asProfile()
        #expect(profile.name == "Erika Musterfrau")
        #expect(profile.addressCity.isEmpty)
        #expect(!profile.complete)
        // Die Plattform nennt ihre Felder englisch, gelesen wird deutsch.
        #expect(profile.missingLabels == ["Ort"])
        // Die Vermieteransicht hängt an der Antwort der Plattform, nicht an
        // einer Bedingung in der App.
        #expect(!profile.showsLenderArea)
        #expect(profile.canAskToLend)
    }

    @Test func nurEinFreigeschalteterVermieterSiehtDieVermieteransicht() {
        let freigeschaltet = RentalProfileDto(lender: true, lenderStatus: "approved").asProfile()
        #expect(freigeschaltet.showsLenderArea)
        #expect(!freigeschaltet.canAskToLend)

        let beantragt = RentalProfileDto(lenderStatus: "pending").asProfile()
        #expect(!beantragt.showsLenderArea)
        #expect(!beantragt.canAskToLend)
    }

    @Test func unbekannteFeldnamenVerschwindenNicht() {
        #expect(RentalFieldNames.labels(["phone", "addressZip"]) == ["Telefonnummer", "Postleitzahl"])
        #expect(RentalFieldNames.labels(["addressCountry"]) == ["addressCountry"])
        #expect(RentalFieldNames.sentence([]) == nil)
    }

    // MARK: Fehler — die App verzweigt auf den Code, nie auf den Text

    @Test func einAbgelaufenesTokenUndEinTokenOhneEmpfaengerSindZweiDinge() {
        let abgelaufen = RentalClient.error(status: 401, data: Data("""
        {"error": {"code": "unauthorized", "message": "Kein Token"}}
        """.utf8))
        #expect(abgelaufen.needsSignIn)
        #expect(!abgelaufen.needsFreshSignIn)

        // Der Fall, der dieses Projekt schon einmal getroffen hat: Das Gerät
        // war vor der Aktualisierung angemeldet und behält seinen Tokensatz.
        let ohneEmpfaenger = RentalClient.error(status: 401, data: Data("""
        {"error": {"code": "token_audience", "message": "Token gilt nicht für die Mietplattform"}}
        """.utf8))
        #expect(ohneEmpfaenger.needsFreshSignIn)
        #expect(!ohneEmpfaenger.needsSignIn)
        // Die Meldung der Plattform gewinnt über unsere Vorgabe.
        #expect(ohneEmpfaenger.message == "Token gilt nicht für die Mietplattform")

        let lage = RentalTrouble(ohneEmpfaenger)
        #expect(lage.needsFreshSignIn)
        #expect(lage.wantsSignIn)
    }

    @Test func einUnvollstaendigesProfilSagtWasFehlt() {
        let fehler = RentalClient.error(status: 400, data: Data("""
        {"error": {"code": "profile_incomplete", "message": "Dein Profil ist unvollständig",
          "missingFields": ["phone", "addressStreet", "addressZip", "addressCity"]}}
        """.utf8))
        #expect(fehler.code == .profileIncomplete)
        #expect(fehler.missingFields.count == 4)
        #expect(RentalFieldNames.labels(fehler.missingFields).first == "Telefonnummer")
    }

    @Test func ohneLesbarenKoerperSpringtDerStatusEin() {
        // Eine Fehlerseite eines Vorschalters darf nicht wie ein
        // Empfängerproblem aussehen — sonst schicken wir jemanden ohne Not
        // durch eine neue Anmeldung.
        let vorschalter = RentalClient.error(status: 401, data: Data("<html>nope</html>".utf8))
        #expect(vorschalter.code == .unauthorized)
        #expect(!vorschalter.needsFreshSignIn)

        let serverfehler = RentalClient.error(status: 502, data: Data())
        #expect(serverfehler.code == .serverFault)
    }

    @Test func einNeuerFehlercodeWirdGezeigtUndNichtVerschluckt() {
        let neu = RentalClient.error(status: 409, data: Data("""
        {"error": {"code": "seasonal_closure", "message": "Im Winter verleihen wir nicht."}}
        """.utf8))
        #expect(neu.code == .unknown)
        #expect(neu.message == "Im Winter verleihen wir nicht.")
    }

    @Test func belegtIstEinEigenerFall() {
        let belegt = RentalClient.error(status: 409, data: Data("""
        {"error": {"code": "occupied", "message": "Zeitraum ist belegt"}}
        """.utf8))
        #expect(belegt.code == .occupied)
    }

    // MARK: Der Scope, ohne den kein Token gilt

    @Test func derEmpfaengerScopeHatDieFormDieZitadelVerlangt() {
        #expect(projectAudienceScope("377276525071827047")
            == "urn:zitadel:iam:org:project:id:377276525071827047:aud")
        // Und die Anmeldung fordert einen solchen Scope tatsächlich an —
        // ohne ihn weist die Mietplattform jedes Token ab.
        #expect(ANMELDE_SCOPES.contains {
            $0.hasPrefix("urn:zitadel:iam:org:project:id:") && $0.hasSuffix(":aud")
        })
    }
}

// MARK: - Der Weg zur Mietplattform

/// Was tatsächlich hinausginge — Pfad, Verfahren, Körper und vor allem: ob
/// ein Token mitfährt. Abgefangen wird örtlich, es verlässt nichts das Gerät.
struct RentalClientTests {
    static func client(_ token: Tokenlage = .token("abc")) -> RentalClient {
        let k = URLSessionConfiguration.ephemeral
        k.protocolClasses = [RentalStub.self]
        return RentalClient(
            base: URL(string: "http://127.0.0.1:8099")!,
            session: URLSession(configuration: k),
            tokenProvider: { token }
        )
    }

    @Test func derKatalogGehtOhneToken() async throws {
        RentalStub.reset()
        RentalStub.answer = (200, Data(RentalTests.itemsJson.utf8))

        let items = try await Self.client().items()
        #expect(items.count == 2)

        let request = try #require(RentalStub.lastRequest)
        #expect(request.url?.path == "/api/v1/items")
        // Umsehen geht ohne Anmeldung — und ein Token an eine öffentliche
        // Route wäre eine unnötige Preisgabe. Vor allem aber bräche der
        // Katalog sonst genau für die, die schon angemeldet sind.
        #expect(request.value(forHTTPHeaderField: "Authorization") == nil)
    }

    @Test func dieVerfuegbarkeitFragtMitDenParameternDesVertrags() async throws {
        RentalStub.reset()
        RentalStub.answer = (200, Data(#"{"available": false, "reason": "occupied"}"#.utf8))

        let answer = try await Self.client().availability(
            deviceId: "rasenwalze", startDate: "2026-09-05", endDate: "2026-09-07"
        )
        #expect(!answer.available)
        #expect(answer.reason == "occupied")

        let request = try #require(RentalStub.lastRequest)
        #expect(request.url?.path == "/api/v1/availability")
        let query = try #require(request.url?.query())
        #expect(query.contains("deviceId=rasenwalze"))
        #expect(query.contains("startDate=2026-09-05"))
        #expect(query.contains("endDate=2026-09-07"))
        #expect(request.value(forHTTPHeaderField: "Authorization") == nil)
    }

    @Test func meineBuchungenNehmenDasTokenMit() async throws {
        RentalStub.reset()
        RentalStub.answer = (200, Data(RentalTests.myBookingsJson.utf8))

        _ = try await Self.client().myBookings()
        let request = try #require(RentalStub.lastRequest)
        #expect(request.url?.path == "/api/v1/bookings/mine")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer abc")
    }

    @Test func ohneAnmeldungFragtDieAppGarNichtErstNach() async throws {
        RentalStub.reset()
        RentalStub.answer = (200, Data("{}".utf8))

        await #expect(throws: RentalError.self) {
            _ = try await Self.client(.abgemeldet).myBookings()
        }
        // Es ist nichts hinausgegangen: Der Fehler entsteht vor der Anfrage.
        #expect(RentalStub.lastRequest == nil)
    }

    @Test func einFunklochIstKeineAbgelaufeneAnmeldung() async throws {
        RentalStub.reset()
        RentalStub.answer = (200, Data("{}".utf8))

        do {
            _ = try await Self.client(.nichtErreichbar).myBookings()
            Issue.record("Es hätte einen Fehler geben müssen")
        } catch let fehler as RentalError {
            // „Ich konnte nicht fragen" darf niemanden abmelden.
            #expect(!fehler.needsSignIn)
            #expect(!fehler.needsFreshSignIn)
        }
    }

    @Test func eineBuchungSchicktGenauDasWasDerVertragVerlangt() async throws {
        RentalStub.reset()
        RentalStub.answer = (201, Data("""
        {"booking": {"id": "neu", "deviceId": "rasenwalze", "deviceName": "Rasenwalze",
          "startDate": "2026-09-05", "endDate": "2026-09-07", "status": "pending",
          "canCancel": true}}
        """.utf8))

        let booking = try await Self.client().book(RentalBookingRequestDto(
            deviceId: "rasenwalze", startDate: "2026-09-05", endDate: "2026-09-07",
            notes: "Hole ich früh ab."
        ))
        #expect(booking.id == "neu")

        let request = try #require(RentalStub.lastRequest)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/api/v1/bookings")
        let body = try #require(RentalStub.lastBody)
        let gelesen = try #require(
            try JSONSerialization.jsonObject(with: body) as? [String: Any]
        )
        #expect(gelesen["deviceId"] as? String == "rasenwalze")
        #expect(gelesen["endDate"] as? String == "2026-09-07")
        // Name und Nummer holt der Server aus dem Profil — abtippen lassen
        // wir niemanden.
        #expect(!gelesen.keys.contains("firstName"))
        #expect(!gelesen.keys.contains("phone"))
        #expect(!gelesen.keys.contains("setId"))
    }

    @Test func eineFehlerseiteGehtNichtAlsLeereListeDurch() async throws {
        RentalStub.reset()
        RentalStub.answer = (200, Data("<html>Wartung</html>".utf8))

        do {
            _ = try await Self.client().items()
            Issue.record("Eine Fehlerseite darf nicht als Katalog durchgehen")
        } catch let fehler as RentalError {
            #expect(fehler == .unreadable)
        }
    }

    @Test func dasProfilAendernSchicktNurWasDasteht() async throws {
        RentalStub.reset()
        RentalStub.answer = (200, Data(RentalTests.profileJson.utf8))

        _ = try await Self.client().updateProfile(RentalProfilePatchDto(
            phone: "+49 5069 123456", addressCity: "Nordstemmen"
        ))
        let request = try #require(RentalStub.lastRequest)
        #expect(request.httpMethod == "PATCH")
        let body = try #require(RentalStub.lastBody)
        let gelesen = try #require(
            try JSONSerialization.jsonObject(with: body) as? [String: Any]
        )
        #expect(gelesen["phone"] as? String == "+49 5069 123456")
        #expect(gelesen["addressCity"] as? String == "Nordstemmen")
        // Was nicht ausgefüllt wurde, wird auch nicht überschrieben.
        #expect(!gelesen.keys.contains("name"))
        #expect(!gelesen.keys.contains("addressStreet"))
        // Die E-Mail-Adresse kommt aus der Rössing-ID und geht gar nicht mit.
        #expect(!gelesen.keys.contains("email"))
    }

    @Test func einSperrwunschNenntSeinGeraet() async throws {
        RentalStub.reset()
        RentalStub.answer = (201, Data("""
        {"block": {"id": "b1", "deviceId": "rasenwalze", "deviceName": "Rasenwalze",
          "startDate": "2026-10-01", "endDate": "2026-10-08", "reason": "Eigener Einsatz"}}
        """.utf8))

        let block = try await Self.client().addBlock(RentalBlockRequestDto(
            deviceId: "rasenwalze", startDate: "2026-10-01", endDate: "2026-10-08",
            reason: "Eigener Einsatz"
        ))
        #expect(block.id == "b1")
        let request = try #require(RentalStub.lastRequest)
        #expect(request.url?.path == "/api/v1/owner/blocks")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer abc")
    }

    @Test func eineSperreAufhebenIstEinLoeschen() async throws {
        RentalStub.reset()
        RentalStub.answer = (200, Data(#"{"deleted": true}"#.utf8))

        try await Self.client().removeBlock(id: "b1")
        let request = try #require(RentalStub.lastRequest)
        #expect(request.httpMethod == "DELETE")
        #expect(request.url?.path == "/api/v1/owner/blocks/b1")
    }
}

// MARK: - Der Zustand der Ansichten

@MainActor
struct RentalModelTests {
    /// Eine Quelle, die nichts kann außer dem, was der Test ihr sagt. Alles
    /// Unbenutzte scheitert laut — ein Modell, das heimlich etwas anderes
    /// abruft, fällt damit auf.
    static func source(
        items: @escaping @MainActor () async throws -> [RentalItemDto] = { [] },
        item: @escaping @MainActor (String) async throws -> RentalItemDto = { _ in
            RentalItemDto()
        },
        search: @escaping @MainActor (String) async throws -> [RentalItemDto] = { _ in [] },
        sets: @escaping @MainActor () async throws -> [RentalSetDto] = { [] },
        availability: @escaping @MainActor (String, String, String) async throws
            -> RentalAvailabilityDto = { _, _, _ in RentalAvailabilityDto() },
        occupancy: @escaping @MainActor (String?) async throws -> [RentalPeriodDto] = { _ in [] },
        profile: @escaping @MainActor () async throws -> RentalProfileDto = {
            RentalProfileDto()
        },
        updateProfile: @escaping @MainActor (RentalProfilePatchDto) async throws
            -> RentalProfileDto = { _ in RentalProfileDto() },
        requestLender: @escaping @MainActor () async throws -> RentalLenderRequestDto = {
            RentalLenderRequestDto()
        },
        myBookings: @escaping @MainActor () async throws -> [RentalBookingDto] = { [] },
        book: @escaping @MainActor (RentalBookingRequestDto) async throws -> RentalBookingDto = {
            _ in RentalBookingDto()
        },
        cancel: @escaping @MainActor (String) async throws -> Void = { _ in },
        ownerBookings: @escaping @MainActor () async throws -> [RentalOwnerBookingDto] = { [] },
        approve: @escaping @MainActor (String) async throws -> Void = { _ in },
        reject: @escaping @MainActor (String) async throws -> Void = { _ in },
        ownerItems: @escaping @MainActor () async throws -> [RentalItemDto] = { [] },
        blocks: @escaping @MainActor () async throws -> [RentalBlockDto] = { [] },
        addBlock: @escaping @MainActor (RentalBlockRequestDto) async throws -> RentalBlockDto = {
            _ in RentalBlockDto()
        },
        removeBlock: @escaping @MainActor (String) async throws -> Void = { _ in }
    ) -> RentalSource {
        RentalSource(
            items: items, item: item, search: search, sets: sets,
            availability: availability, occupancy: occupancy,
            profile: profile, updateProfile: updateProfile, requestLender: requestLender,
            myBookings: myBookings, book: book, cancel: cancel,
            ownerBookings: ownerBookings, approve: approve, reject: reject,
            ownerItems: ownerItems, blocks: blocks, addBlock: addBlock, removeBlock: removeBlock
        )
    }

    static let geraet = RentalItemDto(id: "rasenwalze", name: "Rasenwalze", pricePerDay: 8)
    static let maeher = RentalItemDto(id: "maeher", name: "Kreiselmäher", pricePerDay: 25)

    static func fehler(_ code: String, status: Int = 400) -> RentalError {
        RentalClient.error(
            status: status,
            data: Data(#"{"error": {"code": "\#(code)", "message": ""}}"#.utf8)
        )
    }

    static func day(_ text: String) throws -> Date {
        try #require(RentalDay.parse(text))
    }

    // MARK: Katalog

    @Test func einFehlerIstNichtNichtsDa() async throws {
        // Erst eine Liste, dann ein Ausfall: Die Liste bleibt stehen, und
        // darüber steht, dass sie alt sein könnte. Eine leere Seite ohne
        // Erklärung wäre die schlechteste Antwort.
        let ausfall = Schalter()
        let model = RentalCatalogModel(source: Self.source(items: {
            if await ausfall.an { throw RentalError.network("kaputt") }
            return [Self.geraet, Self.maeher]
        }))

        await model.load()
        #expect(model.devices.count == 2)
        #expect(model.hint == nil)

        await ausfall.einschalten()
        await model.refresh()

        #expect(model.devices.count == 2)
        #expect(model.hint?.contains("nicht mehr aktuell") == true)
        #expect(!model.empty)
    }

    @Test func ohneGeraeteUndOhneFehlerIstWirklichNichtsDa() async {
        let model = RentalCatalogModel(source: Self.source())
        await model.load()
        #expect(model.empty)
        #expect(model.hint == nil)
    }

    @Test func ohneListeNenntDerHinweisDenGrund() async {
        let model = RentalCatalogModel(source: Self.source(items: {
            throw Self.fehler("internal", status: 500)
        }))
        await model.load()
        #expect(model.devices.isEmpty)
        #expect(model.hint?.isEmpty == false)
        // „Fehler" heißt nicht „nichts da".
        #expect(!model.empty)
    }

    @Test func dieReihenfolgeDerSucheBleibtWieSieKam() async {
        // Die Plattform sortiert nach Passung; wir sortieren nicht um.
        let model = RentalCatalogModel(source: Self.source(
            items: { [Self.geraet, Self.maeher] },
            search: { _ in [Self.maeher, Self.geraet] }
        ))
        await model.load()
        model.query = "rasen"
        await model.runSearch()

        #expect(model.visible.map(\.id) == ["maeher", "rasenwalze"])
        #expect(!model.withoutMatch)
    }

    @Test func ohneSucheDerPlattformWirdDasDurchsuchtWasSchonDaIst() async {
        let model = RentalCatalogModel(source: Self.source(
            items: { [Self.geraet, Self.maeher] },
            search: { _ in throw RentalError.network("kaputt") }
        ))
        await model.load()
        model.query = "walze"
        await model.runSearch()

        #expect(model.visible.map(\.id) == ["rasenwalze"])
        // Und es steht dran, dass hier etwas anderes passiert ist.
        #expect(model.hint?.contains("Suche des Maschinchenrings") == true)
    }

    @Test func einLeeresSuchfeldZeigtWiederAlles() async {
        let model = RentalCatalogModel(source: Self.source(
            items: { [Self.geraet, Self.maeher] },
            search: { _ in [Self.maeher] }
        ))
        await model.load()
        model.query = "mäher"
        await model.runSearch()
        #expect(model.visible.count == 1)

        model.query = "   "
        await model.runSearch()
        #expect(model.visible.count == 2)
        #expect(!model.withoutMatch)
    }

    @Test func keinTrefferIstKeinFehler() async {
        let model = RentalCatalogModel(source: Self.source(
            items: { [Self.geraet] }, search: { _ in [] }
        ))
        await model.load()
        model.query = "bagger"
        await model.runSearch()
        #expect(model.visible.isEmpty)
        #expect(model.withoutMatch)
    }

    // MARK: Ein Gerät

    @Test func dieAntwortGiltNurFuerDenZeitraumFuerDenSieKam() async throws {
        let model = RentalDeviceModel(
            device: try #require(Self.geraet.asDevice()),
            source: Self.source(availability: { _, _, _ in
                RentalAvailabilityDto(available: true)
            }),
            now: { try! Self.day("2026-09-05") }
        )
        model.setLastDay(try Self.day("2026-09-06"))
        await model.check()

        #expect(model.answer?.free == true)
        #expect(model.canBook)
        // Der Rückgabetag ist der Tag nach dem letzten Leihtag.
        #expect(model.endDate == "2026-09-07")

        // Wer die Tage verschiebt, hat keine Antwort mehr — sonst bucht
        // jemand ein freies Wochenende auf einem belegten.
        model.setLastDay(try Self.day("2026-09-10"))
        #expect(model.answer == nil)
        #expect(!model.canBook)
    }

    @Test func ohneJaDerPlattformBleibtDerKnopfAus() async throws {
        let model = RentalDeviceModel(
            device: try #require(Self.geraet.asDevice()),
            source: Self.source(availability: { _, _, _ in
                RentalAvailabilityDto(available: false, reason: "occupied")
            }),
            now: { try! Self.day("2026-09-05") }
        )
        await model.check()
        #expect(model.answer?.free == false)
        #expect(!model.canBook)
        #expect(model.answer?.message.contains("vergeben") == true)
    }

    @Test func einWettlaufIstKeinAbsturz() async throws {
        // Zwischen dem Zeichnen des Kalenders und dem Tippen kann eine Minute
        // liegen. Dann sagt die Plattform „belegt" — und die App holt den
        // Kalender neu, statt zu behaupten, es habe geklappt.
        let neuGeholt = Schalter()
        let model = RentalDeviceModel(
            device: try #require(Self.geraet.asDevice()),
            source: Self.source(
                availability: { _, _, _ in RentalAvailabilityDto(available: true) },
                occupancy: { _ in
                    await neuGeholt.einschalten()
                    return []
                },
                book: { _ in throw Self.fehler("occupied", status: 409) }
            ),
            now: { try! Self.day("2026-09-05") }
        )
        await model.check()
        #expect(model.canBook)

        await model.book()
        #expect(model.confirmed == nil)
        #expect(model.trouble?.code == .occupied)
        #expect(model.answer == nil)
        #expect(await neuGeholt.an)
    }

    @Test func einUnvollstaendigesProfilFuehrtWeiterStattZuSchweigen() async throws {
        let model = RentalDeviceModel(
            device: try #require(Self.geraet.asDevice()),
            source: Self.source(
                availability: { _, _, _ in RentalAvailabilityDto(available: true) },
                book: { _ in
                    throw RentalClient.error(status: 400, data: Data("""
                    {"error": {"code": "profile_incomplete", "message": "Dein Profil ist unvollständig",
                      "missingFields": ["phone", "addressCity"]}}
                    """.utf8))
                }
            ),
            now: { try! Self.day("2026-09-05") }
        )
        await model.check()
        await model.book()

        #expect(model.missingProfileFields == ["Telefonnummer", "Ort"])
        #expect(model.trouble?.code == .profileIncomplete)
    }

    @Test func eineGebuchteAnfrageBleibtAufDemSchirm() async throws {
        let model = RentalDeviceModel(
            device: try #require(Self.geraet.asDevice()),
            source: Self.source(
                availability: { _, _, _ in RentalAvailabilityDto(available: true) },
                book: { wunsch in
                    RentalBookingDto(
                        id: "neu", deviceId: wunsch.deviceId, deviceName: "Rasenwalze",
                        startDate: wunsch.startDate, endDate: wunsch.endDate,
                        status: "pending", canCancel: true
                    )
                }
            ),
            now: { try! Self.day("2026-09-05") }
        )
        await model.check()
        await model.book()

        #expect(model.confirmed?.id == "neu")
        #expect(model.confirmed?.state == .pending)
        // Zweimal buchen geht nicht.
        #expect(!model.canBook)
    }

    // MARK: Meine Buchungen

    @Test func einAbgelehntesStornoNimmtKeineZeileWeg() async throws {
        let model = RentalBookingsModel(
            source: Self.source(
                myBookings: {
                    [RentalBookingDto(id: "b1", deviceName: "Rasenwalze",
                                      startDate: "2026-09-05", endDate: "2026-09-07",
                                      status: "approved", canCancel: true)]
                },
                cancel: { _ in throw Self.fehler("conflict", status: 409) }
            ),
            now: { try! Self.day("2026-09-01") }
        )
        await model.load()
        #expect(model.bookings.count == 1)

        await model.cancel(try #require(model.bookings.first))
        // Eine Zeile, die verschwindet, während die Plattform die Buchung
        // noch hält, fällt erst auf, wenn das Gerät weg ist.
        #expect(model.bookings.count == 1)
        #expect(model.hint?.isEmpty == false)
    }

    @Test func einTokenOhneEmpfaengerFuehrtZurNeuenAnmeldung() async {
        let model = RentalBookingsModel(source: Self.source(myBookings: {
            throw Self.fehler("token_audience", status: 401)
        }))
        await model.load()

        #expect(model.bookings.isEmpty)
        // Keine leere Liste ohne Erklärung: Der Bildschirm bietet die neue
        // Anmeldung an.
        #expect(model.needsSignIn)
        #expect(model.trouble?.needsFreshSignIn == true)
        #expect(!model.empty)
    }

    // MARK: Vermieter

    @Test func offeneAnfragenStehenOben() async throws {
        let model = RentalOwnerModel(
            source: Self.source(ownerBookings: {
                [
                    RentalOwnerBookingDto(id: "alt", deviceName: "Rasenwalze",
                                          startDate: "2026-09-01", endDate: "2026-09-02",
                                          status: "approved", canCancel: true),
                    RentalOwnerBookingDto(id: "offen", deviceName: "Kreiselmäher",
                                          startDate: "2026-10-01", endDate: "2026-10-03",
                                          status: "pending", canDecide: true, canCancel: true),
                ]
            }),
            now: { try! Self.day("2026-09-15") }
        )
        await model.load()
        #expect(model.bookings.map(\.id) == ["offen", "alt"])
        #expect(model.waiting == 1)
    }

    @Test func nachEinerZusageZaehltWiederDieListeDerPlattform() async throws {
        let zugesagt = Schalter()
        let model = RentalOwnerModel(
            source: Self.source(
                ownerBookings: {
                    if await zugesagt.an {
                        return [RentalOwnerBookingDto(id: "b1", deviceName: "Rasenwalze",
                                                      startDate: "2026-10-01", endDate: "2026-10-03",
                                                      status: "approved", canCancel: true)]
                    }
                    return [RentalOwnerBookingDto(id: "b1", deviceName: "Rasenwalze",
                                                  startDate: "2026-10-01", endDate: "2026-10-03",
                                                  status: "pending", canDecide: true)]
                },
                approve: { _ in await zugesagt.einschalten() }
            ),
            now: { try! Self.day("2026-09-15") }
        )
        await model.load()
        #expect(model.bookings.first?.canDecide == true)

        await model.approve(try #require(model.bookings.first))
        #expect(model.bookings.first?.state == .approved)
        #expect(model.bookings.first?.canDecide == false)
    }

    @Test func eineSperreNenntDenRueckgabetag() async throws {
        let gesendet = Merker()
        let model = RentalOwnerModel(
            source: Self.source(addBlock: { wunsch in
                await gesendet.merken(wunsch.endDate)
                return RentalBlockDto(id: "b1", deviceId: wunsch.deviceId,
                                      startDate: wunsch.startDate, endDate: wunsch.endDate)
            }),
            now: { try! Self.day("2026-09-15") }
        )
        let erfolg = await model.block(
            deviceId: "rasenwalze",
            firstDay: try Self.day("2026-10-01"),
            lastDay: try Self.day("2026-10-07"),
            reason: ""
        )
        #expect(erfolg)
        // Letzter Tag der Sperre ist der 7., zurück gibt es das Gerät am 8.
        #expect(await gesendet.wert == "2026-10-08")
    }

    // MARK: Profil

    @Test func dasProfilFuelltDasFormularUndNimmtNurWasDasteht() async {
        let gesendet = Merker()
        let model = RentalProfileModel(source: Self.source(
            profile: {
                RentalProfileDto(name: "Erika", email: "erika@example.de", phone: nil,
                                 addressStreet: "Hauptstraße 1", addressZip: "31171",
                                 addressCity: nil, profileComplete: false,
                                 missingFields: ["phone", "addressCity"])
            },
            updateProfile: { patch in
                await gesendet.merken(patch.phone ?? "")
                return RentalProfileDto(name: patch.name, phone: patch.phone,
                                        profileComplete: true)
            }
        ))
        await model.load()
        #expect(model.name == "Erika")
        #expect(model.phone.isEmpty)
        #expect(model.missingLabels == ["Telefonnummer", "Ort"])

        model.phone = "+49 5069 123456"
        let gespeichert = await model.save()
        #expect(gespeichert)
        #expect(await gesendet.wert == "+49 5069 123456")
        #expect(model.profile?.complete == true)
    }

    @Test func nachDerAnfrageAlsVermieterStehtDieAntwortDerPlattformDa() async {
        let model = RentalProfileModel(source: Self.source(
            profile: { RentalProfileDto(lenderStatus: "pending") },
            requestLender: {
                RentalLenderRequestDto(
                    lenderStatus: "pending",
                    message: "Deine Anfrage wurde weitergeleitet."
                )
            }
        ))
        await model.load()
        await model.askToLend()
        // Der Satz der Plattform, unverändert.
        #expect(model.lenderMessage == "Deine Anfrage wurde weitergeleitet.")
        #expect(model.profile?.lenderStatus == .pending)
        #expect(model.profile?.showsLenderArea == false)
    }
}

// MARK: - Kleine Helfer für die Tests

/// Ein Schalter, den eine Quelle über Aktorgrenzen hinweg umlegen kann.
private actor Schalter {
    private(set) var an = false
    func einschalten() { an = true }
}

/// Merkt sich einen Wert, den eine Quelle bekommen hat.
private actor Merker {
    private(set) var wert = ""
    func merken(_ neu: String) { wert = neu }
}

/// Örtliche Ablage statt Netz: Die Anfrage wird abgefangen und beantwortet,
/// ohne das Gerät zu verlassen. Kein Test darf nach draußen.
private nonisolated final class RentalStub: URLProtocol {
    nonisolated(unsafe) static var answer: (Int, Data) = (200, Data())
    nonisolated(unsafe) static var lastRequest: URLRequest?
    nonisolated(unsafe) static var lastBody: Data?

    static func reset() {
        answer = (200, Data())
        lastRequest = nil
        lastBody = nil
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        RentalStub.lastRequest = request
        // `URLSession` schiebt den Körper in einen Datenstrom um, bevor der
        // Protokollhandler ihn sieht — deshalb beide Wege.
        RentalStub.lastBody = request.httpBody ?? request.httpBodyStream.map { strom in
            strom.open()
            defer { strom.close() }
            var gesammelt = Data()
            var puffer = [UInt8](repeating: 0, count: 4096)
            while strom.hasBytesAvailable {
                let gelesen = strom.read(&puffer, maxLength: puffer.count)
                if gelesen <= 0 { break }
                gesammelt.append(contentsOf: puffer[0 ..< gelesen])
            }
            return gesammelt
        }

        let (status, daten) = RentalStub.answer
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
