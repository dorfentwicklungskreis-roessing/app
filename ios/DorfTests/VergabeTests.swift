import Foundation
import Testing

@testable import Dorf

/// Was die Vergabe der Pflegeaufgaben in der App versprechen muss — geprüft
/// ohne Oberfläche und ohne Netz.
///
/// Die Quelle des Modells ist ein Bündel Verschlüsse (`VergabeQuelle`), das
/// der Test selbst füllt; die Fehlerübersetzung wird direkt auf den
/// Statuscode losgelassen. Es wird also nie ein Server angefasst.
///
/// Und: Hier wird nirgends nachgerechnet, wer wann dran ist. Reihenfolge,
/// Fristen und Ruhezeiten stehen im Backend (`backend/internal/vergabe`) —
/// die App fragt und zeigt.
@MainActor
struct VergabeTests {
    // MARK: Werkzeug

    /// Eine Quelle mit vorbereiteten Antworten. Die letzte Liste bleibt für
    /// alle weiteren Abrufe stehen.
    private final class Folge {
        var listen: [[Benachrichtigung]]
        var vorgang: Vorgang?
        var ladeFehler: DorfFehler?
        var zusageFehler: DorfFehler?
        var rueckgabeFehler: DorfFehler?
        var abrufe = 0
        var zusagen: [Int64] = []
        var rueckgaben: [Int64] = []
        var gelesen: [Int64] = []

        init(_ listen: [[Benachrichtigung]], vorgang: Vorgang? = nil) {
            self.listen = listen
            self.vorgang = vorgang
        }

        func naechste() throws -> [Benachrichtigung] {
            let stelle = min(abrufe, listen.count - 1)
            abrufe += 1
            if let ladeFehler { throw ladeFehler }
            return listen.isEmpty ? [] : listen[stelle]
        }
    }

    private static func quelle(_ folge: Folge) -> VergabeQuelle {
        VergabeQuelle(
            benachrichtigungen: { try folge.naechste() },
            gelesen: { folge.gelesen.append($0) },
            zusagen: { id in
                folge.zusagen.append(id)
                if let fehler = folge.zusageFehler { throw fehler }
                return folge.vorgang ?? Self.vorgang()
            },
            zurueckgeben: { id in
                folge.rueckgaben.append(id)
                if let fehler = folge.rueckgabeFehler { throw fehler }
                return Self.vorgang(zusage: nil, name: "", state: "offen")
            }
        )
    }

    /// `Vorgang` und `Benachrichtigung` kennen keinen eigenen Bauplan mit
    /// Argumenten — sie kommen aus dem JSON des Backends. Der Test baut sie
    /// deshalb genauso, wie die App sie bekommt.
    private static func lies<T: Decodable>(_ json: String) -> T {
        do {
            return try JSONDecoder().decode(T.self, from: Data(json.utf8))
        } catch {
            Issue.record("JSON nicht lesbar: \(error)")
            fatalError("JSON nicht lesbar")
        }
    }

    private static func vorgang(id: Int64 = 9, aufgabe: Int64 = 3,
                                zusage: String? = "2026-08-21T09:00:00Z",
                                wer: String = "anna-sub", name: String = "Anna",
                                state: String = "uebernommen") -> Vorgang {
        let bis = zusage.map { "\"claimedUntil\":\"\($0)\"," } ?? ""
        return lies("""
        {"id":\(id),"taskId":\(aufgabe),"state":"\(state)",\(bis)
         "claimedBy":"\(wer)","claimedByName":"\(name)","askedCount":2}
        """)
    }

    private static func anfrage(_ id: Int64, vorgang: Int64 = 9,
                                kind: String = Benachrichtigung.anfrage,
                                erstellt: String = "2026-08-20T09:00:00Z",
                                frist: String? = "2026-08-20T10:00:00Z") -> Benachrichtigung {
        Benachrichtigung(id: id, assignmentId: vorgang, taskId: 3, taskKind: "giessen",
                         taskName: "Gießen", placeId: 2, placeName: "Am Anger", kind: kind,
                         title: "Gießen an „Am Anger“ ist dran",
                         text: "Du bist als Nächste(r) an der Reihe.",
                         createdAt: erstellt, expiresAt: frist)
    }

    private static func datum(_ text: String) -> Date {
        guard let d = RFC3339.datum(text) else {
            Issue.record("Zeitpunkt nicht lesbar: \(text)")
            return Date()
        }
        return d
    }

    // MARK: Die Benachrichtigung des Backends

    @Test func benachrichtigungWirdGelesen() {
        let n: Benachrichtigung = Self.lies("""
        {"id":42,"assignmentId":9,"taskId":3,"taskKind":"giessen","taskName":"Gießen",
         "placeId":2,"placeName":"Am Anger","kind":"anfrage",
         "title":"Gießen an „Am Anger“ ist dran",
         "text":"Du bist als Nächste(r) an der Reihe. Wenn du zusagst, hast du 24 Stunden Zeit.",
         "createdAt":"2026-08-20T09:00:00Z","expiresAt":"2026-08-20T10:00:00Z"}
        """)

        #expect(n.id == 42)
        #expect(n.assignmentId == 9)
        #expect(n.taskId == 3)
        #expect(n.placeName == "Am Anger")
        #expect(n.istAnfrage)
        #expect(!n.gelesen)
        #expect(n.frist == Self.datum("2026-08-20T10:00:00Z"))
        // Titel und Text kommen im Wortlaut des Backends durch.
        #expect(n.anzeigetitel == "Gießen an „Am Anger“ ist dran")
        #expect(n.anzeigetext.hasPrefix("Du bist als Nächste(r) an der Reihe."))
    }

    /// Ein älteres Gerät gegen ein neueres Backend — und umgekehrt: Fehlende
    /// Felder dürfen die ganze Liste nicht kippen.
    @Test func benachrichtigungMitFehlendenFeldernBleibtBrauchbar() {
        let n: Benachrichtigung = Self.lies(#"{"id":7,"kind":"anfrage"}"#)

        #expect(n.id == 7)
        #expect(n.assignmentId == 0)
        #expect(n.taskName.isEmpty)
        #expect(n.frist == nil)
        #expect(n.acknowledgedAt == nil)
        #expect(n.istAnfrage)
        // Ohne Überschrift steht wenigstens da, worum es geht.
        #expect(n.anzeigetitel == "Du bist dran")
        #expect(!n.anzeigetext.isEmpty)
    }

    /// Eine unbekannte Art aus einem neueren Backend ist ein Hinweis — kein
    /// Absturz und kein Zusagen-Knopf ins Leere.
    @Test func unbekannteArtIstEinHinweis() {
        let n: Benachrichtigung = Self.lies(#"{"id":8,"kind":"regenbogen"}"#)

        #expect(!n.istAnfrage)
        #expect(n.anzeigetitel == "Hinweis")
    }

    // MARK: Anfrage oder Hinweis

    @Test func anfrageWillEineAntwortHinweisNicht() {
        // Dieselbe Regel wie im Backend (`NotificationKind.IsRequest`).
        for art in ["anfrage", "rundruf"] {
            #expect(Self.anfrage(1, kind: art).istAnfrage, "\(art) ist eine Anfrage")
        }
        for art in ["zusage_abgelaufen", "zusage_aufgehoben", "vorgang_beendet",
                    "vorgang_entfallen"] {
            #expect(!Self.anfrage(1, kind: art).istAnfrage, "\(art) ist ein Hinweis")
        }
    }

    @Test func sortierungStelltAnfragenNachOben() {
        let liste = [
            Self.anfrage(1, kind: "vorgang_beendet", erstellt: "2026-08-20T12:00:00Z"),
            Self.anfrage(2, kind: "anfrage", erstellt: "2026-08-20T09:00:00Z"),
            Self.anfrage(3, kind: "zusage_abgelaufen", erstellt: "2026-08-20T11:00:00Z"),
            Self.anfrage(4, kind: "rundruf", erstellt: "2026-08-20T10:00:00Z"),
        ]

        let geordnet = Benachrichtigung.geordnet(liste)

        // Erst das, was eine Antwort will — darin das Neueste zuerst.
        let reihenfolge = geordnet.map(\.id)
        let obenNurAnfragen = geordnet.prefix(2).allSatisfy(\.istAnfrage)
        #expect(reihenfolge == [4, 2, 1, 3])
        #expect(obenNurAnfragen)
    }

    // MARK: Fristen

    @Test func abgelaufeneAnfrageBleibtZusagbar() {
        let n = Self.anfrage(1, frist: "2026-08-20T10:00:00Z")
        let danach = Self.datum("2026-08-20T10:30:00Z")

        #expect(n.abgelaufen(jetzt: danach))
        #expect(!n.abgelaufen(jetzt: Self.datum("2026-08-20T09:30:00Z")))
        // Abgelaufen heißt nur: Der Vortritt ist weg. Der Knopf bleibt — ob
        // die Zusage noch geht, entscheidet das Backend.
        let text = n.fristtext(jetzt: danach) ?? ""
        #expect(text.contains("Vortritt ist abgelaufen"))
        #expect(text.contains("Zusagen kannst du weiterhin"))
        #expect(n.istAnfrage)
    }

    @Test func laufendeFristNenntDenZeitpunkt() {
        let n = Self.anfrage(1, frist: "2026-08-20T10:00:00Z")
        let text = n.fristtext(jetzt: Self.datum("2026-08-20T09:15:00Z")) ?? ""

        #expect(text.contains("Vortritt bis"))
        #expect(text.contains(Zeitpunkt.mitUhrzeit(Self.datum("2026-08-20T10:00:00Z"))))
    }

    @Test func ohneFristStehtNichtsDa() {
        #expect(Self.anfrage(1, frist: nil).fristtext() == nil)
    }

    // MARK: Wem gehört die Zusage

    @Test func vonMirErkenntDieEigeneZusage() {
        let meiner = Self.vorgang(wer: "anna-sub", name: "Anna")
        let fremder = Self.vorgang(wer: "bernd-sub", name: "Bernd")

        #expect(meiner.vonMir("anna-sub"))
        #expect(!meiner.vonMir("bernd-sub"))
        #expect(!fremder.vonMir("anna-sub"))
        // Ohne eigene Kennung gehört einem gar nichts — lieber nichts
        // behaupten als das Falsche.
        #expect(!meiner.vonMir(nil))
        #expect(meiner.uebernommen)
        #expect(meiner.zusageFrist == Self.datum("2026-08-21T09:00:00Z"))
    }

    @Test func aufgabeSagtWerZugesagtHat() {
        let meine = Aufgabe(id: 3, kind: "giessen",
                            assignment: Self.vorgang(wer: "anna-sub", name: "Anna"))
        let fremde = Aufgabe(id: 3, kind: "giessen",
                             assignment: Self.vorgang(wer: "bernd-sub", name: "Bernd"))
        let frist = Zeitpunkt.mitUhrzeit(Self.datum("2026-08-21T09:00:00Z"))

        let meinText = meine.zusagetext(meinSub: "anna-sub") ?? ""
        #expect(meinText.contains("Du hast zugesagt"))
        #expect(meinText.contains(frist))

        // Hat jemand anderes zugesagt, braucht es keine zweite.
        let fremdText = fremde.zusagetext(meinSub: "anna-sub") ?? ""
        #expect(fremdText.contains("Bernd"))
        #expect(fremdText.contains("keine zweite Zusage"))

        // Ohne Vorgang gibt es dazu nichts zu sagen.
        #expect(Aufgabe(id: 3).zusagetext(meinSub: "anna-sub") == nil)
    }

    @Test func helferzahlStehtAlsSatzDa() {
        #expect(Aufgabe(id: 1, signupCount: 0).helfertext == nil)
        #expect(Aufgabe(id: 1, signupCount: 1).helfertext == "Eine Person hilft hier mit.")
        #expect(Aufgabe(id: 1, signupCount: 4).helfertext == "4 helfen hier mit.")
        #expect(Aufgabe(id: 1, signupCount: 4, signedUp: true).helfertext
            == "4 helfen hier mit, du bist dabei.")
    }

    // MARK: Zusagen

    @Test func zusageMerktSichFristUndLaedtNeu() async {
        let n = Self.anfrage(1)
        let folge = Folge([[n], []], vorgang: Self.vorgang(wer: "anna-sub", name: "Anna"))
        let modell = VergabeModell(quelle: Self.quelle(folge), meinSub: "anna-sub")
        await modell.laden()

        await modell.zusagen(n)

        #expect(folge.zusagen == [9])
        // Das Backend schließt die Anfrage mit der Zusage — deshalb wird neu
        // geladen, und die eigene Zusage bleibt sichtbar stehen.
        #expect(folge.abrufe == 2)
        #expect(modell.eintraege.isEmpty)
        let meine = modell.zusagen.map(\.id)
        #expect(meine == [9])
        #expect(modell.zusagen.first?.fristtext.contains(
            Zeitpunkt.mitUhrzeit(Self.datum("2026-08-21T09:00:00Z"))) == true)
        #expect(modell.fehler == nil)
        #expect(modell.hinweis == nil)
        #expect(!modell.leer)
    }

    /// Der wichtigste Fall: Zwei tippen gleichzeitig. Das ist keine Panne —
    /// das Backend sagt im Klartext, wer schneller war und bis wann.
    @Test func zusageBei409ZeigtDenWortlautDesBackends() async {
        let wortlaut = "Diese Aufgabe wurde gerade schon von Bernd übernommen "
            + "(bis 21.08.2026, 09:00)."
        let n = Self.anfrage(1)
        let folge = Folge([[n], []])
        folge.zusageFehler = .schonVergeben(grund: wortlaut)
        let modell = VergabeModell(quelle: Self.quelle(folge), meinSub: "anna-sub")
        await modell.laden()

        await modell.zusagen(n)

        // Wortlaut unverändert, keine erfundene Fehlermeldung darüber.
        #expect(modell.hinweis == wortlaut)
        #expect(modell.fehler == nil)
        // Und die Liste ist neu geholt, weil sie ohnehin überholt war.
        #expect(folge.abrufe == 2)
        #expect(modell.eintraege.isEmpty)
        #expect(modell.zusagen.isEmpty)
    }

    /// Der Klartext kommt aus der Antwort des Backends, nicht aus der App.
    @Test func neunundvierzigNeunWirdZuSchonVergeben() {
        let rumpf = Data(#"{"error":"Diese Aufgabe wurde gerade schon von Bernd übernommen."}"#.utf8)

        let fehler = VergabeApi.fehler(status: 409, daten: rumpf)

        guard case .schonVergeben(let grund) = fehler else {
            Issue.record("409 muss „schon vergeben“ heißen, nicht „Serverfehler“.")
            return
        }
        #expect(grund == "Diese Aufgabe wurde gerade schon von Bernd übernommen.")
        #expect(fehler.klartext == grund)
        // Ohne Begründung bleibt ein Satz übrig, der niemanden ratlos lässt.
        #expect(VergabeApi.fehler(status: 409, daten: Data()).klartext
            == "Das hat gerade jemand anderes übernommen.")
    }

    @Test func abgelehnteZusageOhneKonfliktIstEineMeldung() async {
        let n = Self.anfrage(1)
        let folge = Folge([[n]])
        folge.zusageFehler = .keineBerechtigung(
            grund: "Für diesen Ort bist du nicht angemeldet — melde dich zuerst zum Mithelfen an.")
        let modell = VergabeModell(quelle: Self.quelle(folge), meinSub: "anna-sub")
        await modell.laden()

        await modell.zusagen(n)

        #expect(modell.fehler?.contains("nicht angemeldet") == true)
        #expect(modell.zusagen.isEmpty)
    }

    // MARK: Zurückgeben und Lesen

    @Test func rueckgabeRaeumtDieEigeneZusageWeg() async {
        let n = Self.anfrage(1)
        let folge = Folge([[n], []], vorgang: Self.vorgang(wer: "anna-sub", name: "Anna"))
        let modell = VergabeModell(quelle: Self.quelle(folge), meinSub: "anna-sub")
        await modell.laden()
        await modell.zusagen(n)

        await modell.zurueckgeben(vorgang: 9)

        #expect(folge.rueckgaben == [9])
        #expect(modell.zusagen.isEmpty)
        #expect(modell.leer)
        #expect(modell.fehler == nil)
    }

    @Test func hinweisIstMitDemLesenErledigt() async {
        let hinweis = Self.anfrage(5, kind: "vorgang_beendet", frist: nil)
        let folge = Folge([[hinweis], []])
        let modell = VergabeModell(quelle: Self.quelle(folge), meinSub: "anna-sub")
        await modell.laden()
        #expect(!modell.leer)

        await modell.gelesen(hinweis)

        #expect(folge.gelesen == [5])
        #expect(modell.leer)
    }

    // MARK: Netz weg

    @Test func netzausfallLeertDieListeNicht() async {
        let folge = Folge([[Self.anfrage(1)]])
        let modell = VergabeModell(quelle: Self.quelle(folge), meinSub: "anna-sub")
        await modell.laden()

        // Der zweite Abruf scheitert — eine leere Seite wäre die Aussage
        // „es steht nichts an", und die wäre im Funkloch schlicht falsch.
        folge.ladeFehler = .netz("weg")
        await modell.laden()

        #expect(modell.eintraege.count == 1)
        #expect(modell.hinweis == VergabeModell.netzhinweis)
        #expect(modell.jeGeladen)
    }

    // MARK: Mithelfen — An- und Abmelden

    /// Mitschrift der Schreibvorgänge — als Klasse, damit die Verschlüsse
    /// nichts einfangen müssen, was sich nebenher ändert.
    private final class Mitschrift {
        var angemeldet: [(Int64, String?)] = []
        var abgemeldet: [(Int64, String?)] = []
        var abrufe = 0
    }

    @Test func anmeldenSchicktDieAufgabenartUndLaedtNeu() async {
        let mit = Mitschrift()
        let aufgabe = Aufgabe(id: 3, placeId: 2, kind: "giessen", signupCount: 3)
        let quelle = OrteQuelle(
            orte: {
                mit.abrufe += 1
                return OrteAntwort(places: [Ort(id: 2, name: "Am Anger", lat: 52.1, lon: 9.8,
                                                tasks: [aufgabe])])
            },
            erledigungen: { _ in [] },
            melden: { id, _, _ in Erledigung(id: 1, taskId: id) },
            zuruecknehmen: { _ in },
            anmelden: { mit.angemeldet.append(($0, $1)) },
            abmelden: { mit.abgemeldet.append(($0, $1)) }
        )
        let modell = OrteModell(quelle: quelle)
        await modell.laden()

        await modell.anmelden(ort: 2, art: "giessen")
        await modell.abmelden(ort: 2)

        let anmeldungen = mit.angemeldet.map(\.0)
        let abmeldungen = mit.abgemeldet.map(\.0)
        #expect(anmeldungen == [2])
        #expect(mit.angemeldet.first?.1 == "giessen")
        #expect(abmeldungen == [2])
        // Ohne Aufgabenart heißt: der ganze Ort.
        #expect(mit.abgemeldet.first?.1 == nil)
        // Nach jedem Schreiben wird neu geladen — signedUp und signupCount
        // hängen am Backend, nicht an der App.
        #expect(mit.abrufe == 3)
        #expect(modell.fehler == nil)
    }

    @Test func ortWeissWofuerIchAngemeldetBin() {
        let giessen = Aufgabe(id: 1, placeId: 2, kind: "giessen", signupCount: 4, signedUp: true)
        let jaeten = Aufgabe(id: 2, placeId: 2, kind: "jaeten", signupCount: 1)
        let ort = Ort(id: 2, name: "Am Anger", lat: 52.1, lon: 9.8, tasks: [giessen, jaeten])

        #expect(ort.helferArten == ["giessen", "jaeten"])
        #expect(ort.helfeIchMit)
        #expect(ort.meineHelferwahl == .art("giessen"))
        #expect(ort.helferzahl == 4)

        let ohne = Ort(id: 3, name: "Bahnhof", lat: 52.1, lon: 9.8,
                       tasks: [Aufgabe(id: 4, placeId: 3, kind: "giessen")])
        #expect(!ohne.helfeIchMit)
        #expect(ohne.meineHelferwahl == .alles)
        #expect(ohne.helferzahl == 0)
    }

    @Test func helferwahlSchicktDenWertDesBackends() {
        #expect(Helferwahl.alles.taskKind == nil)
        #expect(Helferwahl.art("giessen").taskKind == "giessen")
        #expect(Helferwahl.art("giessen").titel == "Gießen")
        #expect(Helferwahl.art("jaeten").titel == "Jäten")
        // Eine Art, die diese App-Version nicht kennt, verschwindet nicht.
        #expect(Helferwahl.art("neues").titel == "neues")
    }
}
