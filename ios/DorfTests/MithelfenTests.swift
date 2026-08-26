import Foundation
import Testing

@testable import Dorf

/// Was der Bereich „Mithelfen" versprechen muss — geprüft ohne Oberfläche und
/// ohne Netz: Die Quelle des Modells ist ein Bündel Verschlüsse, das der Test
/// selbst füllt (`OrteQuelle`). Es wird also nie ein Server angefasst.
@MainActor
struct MithelfenTests {
    // MARK: Werkzeug

    /// Eine Quelle, die eine vorbereitete Folge von Antworten ausgibt. Die
    /// letzte Antwort bleibt für alle weiteren Abrufe stehen.
    private final class Folge {
        var antworten: [Result<OrteAntwort, DorfFehler>]
        var abrufe = 0
        var meldungen: [(Int64, Double?)] = []
        var ruecknahmen: [Int64] = []
        var meldeFehler: DorfFehler?

        init(_ antworten: [Result<OrteAntwort, DorfFehler>]) { self.antworten = antworten }

        func naechste() throws -> OrteAntwort {
            let stelle = min(abrufe, antworten.count - 1)
            abrufe += 1
            return try antworten[stelle].get()
        }
    }

    private static func quelle(_ folge: Folge) -> OrteQuelle {
        OrteQuelle(
            orte: { try folge.naechste() },
            erledigungen: { _ in [] },
            melden: { id, liter, _ in
                folge.meldungen.append((id, liter))
                if let fehler = folge.meldeFehler { throw fehler }
                return Erledigung(id: 1, taskId: id, userName: "Ich", liters: liter)
            },
            zuruecknehmen: { folge.ruecknahmen.append($0) }
        )
    }

    private static func ort(_ id: Int64, _ name: String, _ status: String,
                           aufgaben: [Aufgabe] = []) -> Ort {
        Ort(id: id, name: name, lat: 52.1, lon: 9.8, status: status, tasks: aufgaben)
    }

    private static func zeitpunkt(_ verschiebung: TimeInterval) -> String {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f.string(from: Date().addingTimeInterval(verschiebung))
    }

    // MARK: Sortierung

    @Test func listeStehtNachDringlichkeit() async {
        let folge = Folge([.success(OrteAntwort(places: [
            Self.ort(1, "Anger", "green"),
            Self.ort(2, "Bahnhof", "yellow"),
            Self.ort(3, "Chaussee", "red"),
            Self.ort(4, "Anger Süd", "red"),
        ]))])
        let modell = OrteModell(quelle: Self.quelle(folge))
        await modell.laden()

        // Rot vor gelb vor grün, bei gleicher Ampel alphabetisch.
        #expect(modell.nachDringlichkeit.map(\.name)
            == ["Anger Süd", "Chaussee", "Bahnhof", "Anger"])
    }

    // MARK: Spielschutz

    @Test func sperrfristSperrtDenKnopfUndNenntDenZeitpunkt() {
        let bis = Date().addingTimeInterval(3600)
        let aufgabe = Aufgabe(id: 7, kind: "giessen", liters: 10,
                              lockedUntil: Self.zeitpunkt(3600))

        guard case .gesperrt(let genannt) = aufgabe.meldeknopf() else {
            Issue.record("Bei laufender Sperrfrist muss der Knopf gesperrt sein.")
            return
        }
        // Der Zeitpunkt kommt vom Backend und wird angezeigt, nicht gerechnet.
        #expect(abs(genannt.timeIntervalSince(bis)) < 2)
        #expect(!aufgabe.meldenMoeglich())
    }

    @Test func abgelaufeneSperrfristGibtDenKnopfWiederFrei() {
        let aufgabe = Aufgabe(id: 7, kind: "jaeten", lockedUntil: Self.zeitpunkt(-60))
        #expect(aufgabe.meldeknopf() == .bereit(titel: "Ich habe gejätet 🌿"))
    }

    @Test func erledigteEinmalaufgabeHatKeinenKnopfMehr() {
        let aufgabe = Aufgabe(
            id: 9, kind: "sonstiges", title: "Zum Bahnhof fahren", oneOff: true,
            dueDate: "2026-09-01T00:00:00Z",
            lastCompletion: Erledigung(id: 4, taskId: 9, userName: "Anna",
                                       doneAt: "2026-08-20T08:00:00Z")
        )
        #expect(aufgabe.erledigtUndVorbei)
        #expect(aufgabe.meldeknopf() == .keiner)
    }

    @Test func offeneEinmalaufgabeDarfGemeldetWerden() {
        let aufgabe = Aufgabe(id: 9, kind: "sonstiges", oneOff: true,
                              dueDate: "2026-09-01T00:00:00Z")
        #expect(aufgabe.meldeknopf() == .bereit(titel: "Als erledigt melden"))
    }

    // MARK: Netzausfall

    @Test func alterStandBleibtBeiNetzfehlerStehen() async {
        let folge = Folge([
            .success(OrteAntwort(places: [Self.ort(1, "Anger", "red"),
                                          Self.ort(2, "Bahnhof", "green")],
                                 wateringFactor: 1.5)),
            .failure(.netz("Verbindung verloren")),
        ])
        let modell = OrteModell(quelle: Self.quelle(folge))

        await modell.laden()
        #expect(modell.orte.count == 2)
        #expect(modell.giessfaktor == 1.5)
        #expect(modell.hinweis == nil)

        await modell.laden()
        // Keine leere Seite: Der letzte Stand bleibt, dazu kommt der Hinweis.
        #expect(modell.orte.count == 2)
        #expect(modell.giessfaktor == 1.5)
        #expect(modell.hinweis == "Keine Verbindung zum Server. Es werden ggf. alte Daten angezeigt.")
    }

    @Test func nachErfolgreichemAbrufIstDerHinweisWiederWeg() async {
        let folge = Folge([
            .failure(.netz("weg")),
            .success(OrteAntwort(places: [Self.ort(1, "Anger", "green")])),
        ])
        let modell = OrteModell(quelle: Self.quelle(folge))
        await modell.laden()
        #expect(modell.orte.isEmpty)
        #expect(modell.hinweis != nil)

        await modell.laden()
        #expect(modell.orte.count == 1)
        #expect(modell.hinweis == nil)
    }

    // MARK: Melden

    @Test func meldungSchicktDieMengeUndDanktDanach() async {
        let aufgabe = Aufgabe(id: 7, kind: "giessen", liters: 10)
        let folge = Folge([.success(OrteAntwort(places: [
            Self.ort(1, "Anger", "red", aufgaben: [aufgabe]),
        ]))])
        let modell = OrteModell(quelle: Self.quelle(folge))
        await modell.laden()

        await modell.melden(aufgabe)
        #expect(folge.meldungen.count == 1)
        #expect(folge.meldungen.first?.0 == 7)
        #expect(folge.meldungen.first?.1 == 10)
        #expect(modell.bestaetigung == "Danke fürs Gießen! 💚")
        // Nach der Meldung wird neu geladen — der Stand kommt vom Backend.
        #expect(folge.abrufe == 2)
    }

    @Test func abgelehnteMeldungZeigtDenWortlautDesBackends() async {
        let aufgabe = Aufgabe(id: 7, kind: "giessen")
        let folge = Folge([.success(OrteAntwort(places: [
            Self.ort(1, "Anger", "red", aufgaben: [aufgabe]),
        ]))])
        let wiederAb = Date().addingTimeInterval(7200)
        folge.meldeFehler = .gesperrt(wiederAb: wiederAb)

        let modell = OrteModell(quelle: Self.quelle(folge))
        await modell.laden()
        await modell.melden(aufgabe)

        // Kein selbst gebauter Satz: genau der Klartext des Fehlers.
        #expect(modell.fehler == DorfFehler.gesperrt(wiederAb: wiederAb).klartext)
        #expect(modell.bestaetigung == nil)
    }

    @Test func ruecknahmeGehtNurUeberDieEigeneLetzteMeldung() async {
        let meine = Erledigung(id: 42, taskId: 7, userSub: "ich", userName: "Ich")
        let fremde = Erledigung(id: 43, taskId: 8, userSub: "anna", userName: "Anna")
        let meineAufgabe = Aufgabe(id: 7, kind: "giessen", lastCompletion: meine)
        let fremdeAufgabe = Aufgabe(id: 8, kind: "giessen", lastCompletion: fremde)

        #expect(meineAufgabe.eigeneLetzteMeldung("ich") == meine)
        #expect(fremdeAufgabe.eigeneLetzteMeldung("ich") == nil)
        #expect(meineAufgabe.eigeneLetzteMeldung(nil) == nil)

        let folge = Folge([.success(OrteAntwort(places: [
            Self.ort(1, "Anger", "green", aufgaben: [meineAufgabe]),
        ]))])
        let modell = OrteModell(quelle: Self.quelle(folge))
        await modell.laden()
        await modell.zuruecknehmen(meine)
        #expect(folge.ruecknahmen == [42])
        #expect(modell.bestaetigung == "Meldung zurückgenommen.")
    }

    // MARK: Zeile der Liste

    @Test func dieZeileZeigtDieOffeneAufgabeMitDerKuerzestenFrist() {
        let bald = Aufgabe(id: 1, kind: "giessen", liters: 10, status: "red",
                           dueAt: Self.zeitpunkt(-3600))
        let spaeter = Aufgabe(id: 2, kind: "jaeten", status: "yellow",
                              dueAt: Self.zeitpunkt(86_400))
        let erledigteEinmalige = Aufgabe(
            id: 3, kind: "sonstiges", oneOff: true,
            lastCompletion: Erledigung(id: 5, taskId: 3, userName: "Anna"),
            dueAt: Self.zeitpunkt(-86_400)
        )
        let ort = Self.ort(1, "Anger", "red",
                           aufgaben: [spaeter, erledigteEinmalige, bald])

        // Die erledigte Einmalaufgabe ist nicht mehr offen, obwohl ihre Frist
        // am weitesten zurückliegt.
        #expect(ort.kuerzesteOffeneAufgabe?.id == 1)
        #expect(ort.kuerzesteOffeneAufgabe?.kurztext == "Gießen · 10 Liter")
    }
}
