import Foundation
import Testing

@testable import Dorf

/// Was das Verzeichnis der Vereine und Gruppen versprechen muss — geprüft
/// ohne Oberfläche und ohne Netz.
///
/// Die Quelle des Modells ist ein Bündel Verschlüsse (`TraegerSource`), das
/// der Test selbst füllt. Es wird also nie ein Server angefasst.
///
/// Und vor allem: Hier wird nirgends nachgerechnet, wer beitreten darf oder
/// wer einen Träger sehen darf. Das steht in `model.Zugriff` im Backend, und
/// die Antwort kommt fertig mit. Geprüft wird, dass die App sie **übernimmt**
/// — samt Wortlaut, wenn etwas schiefgeht.
@MainActor
struct TraegerTests {
    // MARK: Werkzeug

    /// Merkt sich, was das Modell tatsächlich getan hat, und gibt vor, was
    /// der Server antwortet.
    private final class Spur {
        var listen: [[Traeger]]
        var abrufe = 0
        var eigene: [Beitritt] = []
        var antraege: [Beitritt] = []
        var villagers: [Dorfbewohner] = []

        var ladeFehler: DorfFehler?
        var beitrittFehler: DorfFehler?
        var entscheidungsFehler: DorfFehler?
        var aufnahmeFehler: DorfFehler?

        var beitritte: [(Int64, String)] = []
        var entscheidungen: [(Int64, String)] = []
        var aufnahmen: [(Int64, String)] = []

        init(_ listen: [[Traeger]]) { self.listen = listen }

        func naechste() throws -> [Traeger] {
            let stelle = min(abrufe, max(listen.count - 1, 0))
            abrufe += 1
            if let ladeFehler { throw ladeFehler }
            return listen.isEmpty ? [] : listen[stelle]
        }
    }

    private static func quelle(_ spur: Spur) -> TraegerSource {
        TraegerSource(
            list: { try spur.naechste() },
            detail: { id in
                guard let t = (try? spur.naechste())?.first(where: { $0.id == id }) else {
                    throw DorfFehler.nichtGefunden
                }
                return t
            },
            join: { id, grund in
                spur.beitritte.append((id, grund))
                if let fehler = spur.beitrittFehler { throw fehler }
                return Beitritt(id: 1, traegerId: id, status: "beantragt", begruendung: grund)
            },
            requests: { _ in spur.antraege },
            decide: { id, status in
                spur.entscheidungen.append((id, status))
                if let fehler = spur.entscheidungsFehler { throw fehler }
                return Beitritt(id: id, status: status)
            },
            addMember: { id, sub in
                spur.aufnahmen.append((id, sub))
                if let fehler = spur.aufnahmeFehler { throw fehler }
                return Beitritt(id: 7, traegerId: id, userSub: sub, status: "erteilt")
            },
            myRequests: { spur.eigene },
            villagers: { spur.villagers }
        )
    }

    private static func modell(_ spur: Spur) -> TraegerModel {
        TraegerModel(source: Self.quelle(spur))
    }

    private static func verein(_ id: Int64, _ name: String, parent: Int64 = 0,
                               mitglied: Bool = false, verwaltet: Bool = false,
                               moeglich: Bool = true, hindernis: String = "",
                               antrag: String = "", offen: Int = 0,
                               sichtbarkeit: String = "offen") -> Traeger {
        Traeger(id: id, name: name, status: "zugelassen", sichtbarkeit: sichtbarkeit,
                parentId: parent, istMitglied: mitglied, darfVerwalten: verwaltet,
                beitrittMoeglich: moeglich, beitrittHindernis: hindernis,
                beitrittStatus: antrag, offeneBeitritte: offen)
    }

    // MARK: Was vom Server kommt, wird gelesen

    @Test func dieSichtDesServersWirdUebernommen() throws {
        let roh = """
        {"id":2,"name":"AK 2 Umwelt und Natur","beschreibung":"Beete und Blumenkästen",
         "status":"zugelassen","sichtbarkeit":"offen","parentId":1,
         "istMitglied":false,"darfVerwalten":true,"beitrittMoeglich":true,
         "beitrittStatus":"beantragt","offeneBeitritte":3}
        """.data(using: .utf8)!
        let t = try JSONDecoder().decode(Traeger.self, from: roh)
        #expect(t.name == "AK 2 Umwelt und Natur")
        #expect(t.parentId == 1)
        #expect(t.darfVerwalten)
        #expect(t.beitrittMoeglich)
        #expect(t.beitrittStatus == "beantragt")
        #expect(t.offeneBeitritte == 3)
    }

    /// Ein Träger, dem man nicht beitreten kann, bringt den Grund im Klartext
    /// mit. Die App erfindet ihn nicht — sie zeigt ihn.
    @Test func dasHindernisKommtImWortlaut() throws {
        let satz = "Diese Gruppe ist geschlossen: Wer dazugehören soll, wird von ihr aufgenommen."
        let roh = """
        {"id":5,"name":"Der stille Kreis","sichtbarkeit":"geschlossen",
         "beitrittMoeglich":false,"beitrittHindernis":"\(satz)"}
        """.data(using: .utf8)!
        let t = try JSONDecoder().decode(Traeger.self, from: roh)
        #expect(!t.beitrittMoeglich)
        #expect(t.beitrittHindernis == satz)
        #expect(t.istGeschlossen)
    }

    /// Ein älterer Backend-Stand schickt die Sichtfelder nicht mit. Dann darf
    /// nichts unlesbar werden — es gilt schlicht „nein".
    @Test func fehlendeFelderMachenNichtsKaputt() throws {
        let roh = #"{"id":9,"name":"Dorfpflege Rössing e.V."}"#.data(using: .utf8)!
        let t = try JSONDecoder().decode(Traeger.self, from: roh)
        #expect(t.name == "Dorfpflege Rössing e.V.")
        #expect(!t.istMitglied)
        #expect(!t.darfVerwalten)
        #expect(!t.beitrittMoeglich)
        #expect(t.parentId == 0)
    }

    @Test func derOrtBringtDieKennungSeinesTraegersMit() throws {
        let roh = """
        {"id":3,"name":"Beet vor dem Dorfgemeinschaftshaus","lat":52.18,"lon":9.81,
         "traegerName":"AK 2 Umwelt und Natur","traegerId":2}
        """.data(using: .utf8)!
        let ort = try JSONDecoder().decode(Ort.self, from: roh)
        #expect(ort.traegerId == 2)
    }

    // MARK: Verein und Arbeitskreis

    @Test func arbeitskreiseStehenUnterIhremVerein() async {
        let spur = Spur([[
            Self.verein(1, "Dorfpflege Rössing e.V."),
            Self.verein(2, "AK 2 Umwelt und Natur", parent: 1),
            Self.verein(3, "AK 1 Bauen", parent: 1),
        ]])
        let modell = Self.modell(spur)
        await modell.load()

        #expect(modell.roots.map(\.id) == [1])
        #expect(modell.children(of: 1).map(\.name) == ["AK 1 Bauen", "AK 2 Umwelt und Natur"])
        #expect(modell.children(of: 2).isEmpty, "Genau eine Ebene, nicht tiefer")
    }

    /// Ein Arbeitskreis kann sichtbar sein, sein Verein aber nicht — dann
    /// verschwände er aus dem Verzeichnis, hinge er nur unter seinem Dach.
    @Test func einArbeitskreisOhneSichtbaresDachStehtObenDrin() async {
        let spur = Spur([[Self.verein(2, "AK 2 Umwelt und Natur", parent: 99)]])
        let modell = Self.modell(spur)
        await modell.load()

        #expect(modell.roots.map(\.id) == [2])
    }

    // MARK: Der Weg vom Ort zum Träger

    /// Das Verzeichnis des Servers ist die einzige Auskunft darüber, ob es
    /// hinter einem Namen etwas zu sehen gibt. Eine eigene Regel daneben wäre
    /// die Sichtbarkeitsprüfung zum zweiten Mal.
    @Test func einWegZumTraegerGibtEsNurWennErImVerzeichnisSteht() async {
        let spur = Spur([[Self.verein(1, "Dorfpflege Rössing e.V.")]])
        let modell = Self.modell(spur)
        await modell.load()

        #expect(modell.inDirectory(1))
        #expect(!modell.inDirectory(5), "Eine geschlossene Gruppe steht nicht drin")
        #expect(!modell.inDirectory(0), "Ein Ort ohne Träger führt nirgendwohin")
    }

    // MARK: Mitmachen

    @Test func mitmachenSchicktDieBegruendungUndLaedtNeu() async {
        let spur = Spur([
            [Self.verein(1, "Dorfpflege Rössing e.V.")],
            [Self.verein(1, "Dorfpflege Rössing e.V.", moeglich: false, antrag: "beantragt")],
        ])
        let modell = Self.modell(spur)
        await modell.load()

        let geklappt = await modell.join(traeger: 1, reason: "  Ich wohne nebenan.  ")
        #expect(geklappt)
        #expect(spur.beitritte.count == 1)
        #expect(spur.beitritte.first?.1 == "Ich wohne nebenan.", "Leerraum gehört nicht mit")
        #expect(modell.traeger(id: 1)?.beitrittStatus == "beantragt", "Der Stand wird neu geholt")
        #expect(modell.confirmation != nil)
        #expect(modell.error == nil)
    }

    /// Ein 409 ist keine Panne: Die Lage passt nicht, und der Server sagt in
    /// welcher Weise. Genau dieser Satz gehört auf den Schirm.
    @Test func einAbgelehnterBeitrittZeigtDenSatzDesServers() async {
        let spur = Spur([[Self.verein(1, "Dorfpflege Rössing e.V.")]])
        spur.beitrittFehler = .schonVergeben(grund: "Du gehörst schon dazu.")
        let modell = Self.modell(spur)
        await modell.load()

        let geklappt = await modell.join(traeger: 1, reason: "")
        #expect(!geklappt)
        #expect(modell.error == "Du gehörst schon dazu.")
        #expect(modell.confirmation == nil)
    }

    // MARK: Entscheiden

    @Test func freigebenMeldetDenNamenZurueck() async {
        let spur = Spur([[Self.verein(1, "Dorfpflege", verwaltet: true, offen: 1)]])
        spur.antraege = [Beitritt(id: 4, traegerId: 1, userName: "Anna Beispiel")]
        let modell = Self.modell(spur)
        await modell.load()
        await modell.loadRequests(traeger: 1)

        await modell.decide(request: spur.antraege[0], status: "erteilt")
        #expect(spur.entscheidungen.first?.1 == "erteilt")
        #expect(modell.confirmation == "Anna Beispiel gehört jetzt dazu.")
    }

    /// Der wichtigste Fall überhaupt: Die Freigabe schreibt zuerst in die
    /// Rössing-ID. Klappt das nicht, bleibt der Antrag offen — und die App
    /// darf auf keinen Fall „aufgenommen" melden, während die Tür zu bleibt.
    @Test func eineGescheiterteFreigabeMeldetKeinenErfolg() async {
        let satz = "Die Mitgliedschaft konnte in der Rössing-ID nicht eingetragen werden — "
            + "der Antrag bleibt deshalb offen. Bitte gleich noch einmal versuchen."
        let spur = Spur([[Self.verein(1, "Dorfpflege", verwaltet: true, offen: 1)]])
        spur.antraege = [Beitritt(id: 4, traegerId: 1, userName: "Anna Beispiel")]
        spur.entscheidungsFehler = .nichtVerfuegbar(grund: satz)
        let modell = Self.modell(spur)
        await modell.load()

        await modell.decide(request: spur.antraege[0], status: "erteilt")
        #expect(modell.error == satz, "Der Wortlaut des Servers, nicht ein eigener")
        #expect(modell.confirmation == nil)
        #expect(modell.requests[1]?.count == 1, "Der Antrag bleibt stehen")
    }

    @Test func direktAufnehmenSchicktDieKennungDerPerson() async {
        let spur = Spur([[Self.verein(1, "Der stille Kreis", verwaltet: true,
                                      moeglich: false, sichtbarkeit: "geschlossen")]])
        spur.villagers = [Dorfbewohner(userSub: "anna-sub", name: "Anna Beispiel")]
        let modell = Self.modell(spur)
        await modell.load()
        await modell.loadVillagers()

        await modell.addMember(traeger: 1, person: modell.villagers[0])
        #expect(spur.aufnahmen.first?.1 == "anna-sub")
        #expect(modell.confirmation == "Anna Beispiel gehört jetzt dazu.")
    }

    // MARK: Der letzte Stand bleibt stehen

    @Test func einAusfallLeertDasVerzeichnisNicht() async {
        let spur = Spur([[Self.verein(1, "Dorfpflege Rössing e.V.")]])
        let modell = Self.modell(spur)
        await modell.load()
        #expect(modell.all.count == 1)

        spur.ladeFehler = .netz("weg")
        await modell.load()
        #expect(modell.all.count == 1, "Eine leere Liste im Funkloch wäre eine Falschaussage")
        #expect(modell.notice != nil)
    }

    // MARK: Zahlen und Sätze

    @Test func offeneAnfragenWerdenUeberAlleTraegerGezaehlt() async {
        let spur = Spur([[
            Self.verein(1, "Dorfpflege", verwaltet: true, offen: 2),
            Self.verein(2, "AK 2", parent: 1, verwaltet: true, offen: 1),
            Self.verein(3, "Feuerwehr"),
        ]])
        let modell = Self.modell(spur)
        await modell.load()
        #expect(modell.openRequestsForMe == 3)
    }

    @Test func derKachelhinweisSchweigtBeiNull() {
        #expect(Startseitentexte.traegerHinweis(offeneAnfragen: 0) == nil)
        #expect(Startseitentexte.traegerHinweis(offeneAnfragen: 1) == "Eine Anfrage wartet auf dich")
        #expect(Startseitentexte.traegerHinweis(offeneAnfragen: 4) == "4 Anfragen warten auf dich")
    }

    @Test func standUndSichtbarkeitStehenAufDeutschDa() {
        #expect(TraegerTexte.status("zugelassen") == "Zugelassen")
        #expect(TraegerTexte.sichtbarkeit("geschlossen") == "Geschlossene Gruppe")
        #expect(TraegerTexte.beitrittStatus("beantragt") == "Deine Anfrage liegt beim Vorstand.")
        #expect(TraegerTexte.beitrittStatus("") == nil)
        // Ein Wert, den diese App-Fassung nicht kennt, verschwindet nicht
        // stillschweigend — er steht so da, wie er kam.
        #expect(TraegerTexte.status("ruhend") == "ruhend")
    }
}
