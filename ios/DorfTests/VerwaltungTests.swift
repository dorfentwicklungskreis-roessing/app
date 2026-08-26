import Foundation
import Testing

@testable import Dorf

/// Was der Bereich „Verwaltung" versprechen muss.
///
/// Kein Test geht ins Netz und keiner öffnet eine Karte: Die Eingaben werden
/// als JSON geprüft (der Vertrag mit dem Backend), die Fehlerübersetzung als
/// reine Funktion, und das Modell bekommt seine Quelle als Bündel Verschlüsse
/// gereicht (`VerwaltungQuelle`).
@MainActor
struct VerwaltungTests {
    // MARK: - Werkzeug

    /// Eine Eingabe als Wörterbuch — so lässt sich auch prüfen, was **nicht**
    /// mitgeschickt wurde.
    private static func json(_ wert: some Encodable) -> [String: Any] {
        guard let daten = try? JSONEncoder().encode(wert) else { return [:] }
        return ((try? JSONSerialization.jsonObject(with: daten)) as? [String: Any]) ?? [:]
    }

    private static func fehlerdaten(_ text: String) -> Data {
        Data(#"{"error":"\#(text)"}"#.utf8)
    }

    private static func ort(_ id: Int64 = 1, aufgaben: [Aufgabe] = []) -> Ort {
        Ort(id: id, name: "Unter den Eichen", description: "Am Zaun", kind: "blumenkasten",
            lat: 52.1832, lon: 9.8168, active: true, tasks: aufgaben)
    }

    /// Eine Quelle, die jeden Aufruf mitschreibt und auf Wunsch ablehnt.
    private final class Protokoll {
        var angelegteOrte: [OrtEingabe] = []
        var geaenderteOrte: [(Int64, OrtEingabe)] = []
        var geloeschteOrte: [Int64] = []
        var angelegteAufgaben: [(Int64, AufgabeEingabe)] = []
        var geaenderteAufgaben: [(Int64, AufgabeEingabe)] = []
        var geloeschteAufgaben: [Int64] = []
        var gesetzteFaktoren: [Double] = []
        var neuGeladen = 0
        /// Womit die nächste Schreibanfrage abgewiesen wird.
        var abweisen: DorfFehler?
        var faktorDesBackends: Double = 1
    }

    private static func quelle(_ p: Protokoll) -> VerwaltungQuelle {
        VerwaltungQuelle(
            ortAnlegen: { eingabe in
                p.angelegteOrte.append(eingabe)
                if let fehler = p.abweisen { throw fehler }
                return Self.ort()
            },
            ortAendern: { id, eingabe in
                p.geaenderteOrte.append((id, eingabe))
                if let fehler = p.abweisen { throw fehler }
                return Self.ort(id)
            },
            ortLoeschen: { id in
                p.geloeschteOrte.append(id)
                if let fehler = p.abweisen { throw fehler }
            },
            aufgabeAnlegen: { ortId, eingabe in
                p.angelegteAufgaben.append((ortId, eingabe))
                if let fehler = p.abweisen { throw fehler }
                return Aufgabe(id: 7, placeId: ortId)
            },
            aufgabeAendern: { id, eingabe in
                p.geaenderteAufgaben.append((id, eingabe))
                if let fehler = p.abweisen { throw fehler }
                return Aufgabe(id: id)
            },
            aufgabeLoeschen: { id in
                p.geloeschteAufgaben.append(id)
                if let fehler = p.abweisen { throw fehler }
            },
            einstellungen: {
                if let fehler = p.abweisen { throw fehler }
                return Einstellungen(wateringFactor: p.faktorDesBackends)
            },
            hitzefaktorSetzen: { faktor in
                p.gesetzteFaktoren.append(faktor)
                if let fehler = p.abweisen { throw fehler }
                p.faktorDesBackends = faktor
                return Einstellungen(wateringFactor: faktor)
            },
            neuLaden: { p.neuGeladen += 1 }
        )
    }

    // MARK: - Eingabe „Ort"

    @Test func ortEingabeSchicktAlleFelderDesVertrags() {
        let felder = Self.json(OrtEingabe(
            name: "Unter den Eichen", description: "Am Zaun", kind: "beet",
            lat: 52.1832, lon: 9.8168, active: false
        ))
        #expect(felder["name"] as? String == "Unter den Eichen")
        #expect(felder["description"] as? String == "Am Zaun")
        #expect(felder["kind"] as? String == "beet")
        #expect(felder["lat"] as? Double == 52.1832)
        #expect(felder["lon"] as? Double == 9.8168)
        #expect(felder["active"] as? Bool == false)
    }

    // MARK: - Eingabe „Aufgabe": entweder Intervall oder Termin

    @Test func regelmaessigeAufgabeGehtOhneFaelligkeitsdatum() {
        // Das Backend weist „dueDate" bei einer regelmäßigen Aufgabe ab
        // („dueDate gibt es nur bei einmaligen Aufgaben"). Es darf also gar
        // nicht erst mitgehen.
        let felder = Self.json(AufgabeEingabe.regelmaessig(
            kind: "giessen", title: "Gießen", liters: 10, intervalDays: 7, redAfterDays: 14
        ))
        #expect(felder["oneOff"] as? Bool == false)
        #expect(felder["intervalDays"] as? Double == 7)
        #expect(felder["redAfterDays"] as? Double == 14)
        #expect(felder["dueDate"] == nil)
    }

    @Test func einmaligeAufgabeGehtOhneIntervall() {
        // Umgekehrt: Bei einem Termin nullt das Backend die Intervalle ohnehin
        // — sie mitzuschicken hieße, zwei Wahrheiten zu behaupten.
        let felder = Self.json(AufgabeEingabe.einmalig(
            kind: "sonstiges", title: "Zum Bahnhof fahren", dueDate: "2026-08-20"
        ))
        #expect(felder["oneOff"] as? Bool == true)
        #expect(felder["dueDate"] as? String == "2026-08-20")
        #expect(felder["intervalDays"] == nil)
        #expect(felder["redAfterDays"] == nil)
    }

    @Test func literNurBeimGiessen() {
        let giessen = Self.json(AufgabeEingabe.regelmaessig(
            kind: "giessen", liters: 10, intervalDays: 7, redAfterDays: 14
        ))
        #expect(giessen["liters"] as? Double == 10)

        // „Jäten, 10 Liter" kann niemand deuten — das Feld bleibt weg, auch
        // wenn im Formular noch eine Zahl von vorhin steht.
        let jaeten = Self.json(AufgabeEingabe.regelmaessig(
            kind: "jaeten", liters: 10, intervalDays: 7, redAfterDays: 14
        ))
        #expect(jaeten["liters"] == nil)

        // Eine 0 wäre keine Menge; das Backend wiese sie ab („liters muss
        // eine Zahl > 0 sein").
        let ohne = Self.json(AufgabeEingabe.regelmaessig(
            kind: "giessen", liters: 0, intervalDays: 7, redAfterDays: 14
        ))
        #expect(ohne["liters"] == nil)
    }

    @Test func formularBautNurEineDerBeidenBauarten() {
        var formular = AufgabeFormular(ortId: 1)
        formular.art = "giessen"
        formular.liter = "10,5"
        formular.intervall = "7"
        formular.rot = "14"
        let regelmaessig = Self.json(formular.eingabe ?? AufgabeEingabe(kind: "giessen"))
        #expect(regelmaessig["liters"] as? Double == 10.5)
        #expect(regelmaessig["intervalDays"] as? Double == 7)
        #expect(regelmaessig["dueDate"] == nil)

        formular.einmalig = true
        formular.abraeumenNachErledigung = true
        let einmalig = Self.json(formular.eingabe ?? AufgabeEingabe(kind: "giessen"))
        #expect(einmalig["oneOff"] as? Bool == true)
        #expect(einmalig["intervalDays"] == nil)
        #expect(einmalig["removeWhenDone"] as? Bool == true)
    }

    @Test func abraeumenGibtEsNurBeiEinmaligenAufgaben() {
        // Eine regelmäßige Aufgabe kommt wieder — „abräumen" ergäbe keinen
        // Sinn, und das Formular bietet es auch nicht an.
        var formular = AufgabeFormular(ortId: 1)
        formular.abraeumenNachErledigung = true
        let felder = Self.json(formular.eingabe ?? AufgabeEingabe(kind: "giessen"))
        #expect(felder["removeWhenDone"] as? Bool == false)
    }

    // MARK: - Datumsformat der Fälligkeit

    @Test func faelligkeitsdatumGehtImFormatDesBackends() {
        // `ParseTermin` im Backend liest genau „2026-08-20" (oder RFC3339).
        var teile = DateComponents()
        teile.year = 2026
        teile.month = 8
        teile.day = 20
        teile.hour = 12
        var kalender = Calendar(identifier: .gregorian)
        kalender.timeZone = Zeitpunkt.dorfZone
        let datum = kalender.date(from: teile) ?? Date()
        #expect(Verwaltungsdatum.text(datum) == "2026-08-20")
    }

    @Test func terminDesBackendsWirdInOrtszeitGelesen() {
        // Das Backend legt „bis zum 20." als 20.08. 23:59:59 Ortszeit ab. In
        // UTC gelesen wäre das der 20.08. um 21:59 — bei anderer Zeitzone
        // schnell der Vortag. Gerechnet wird deshalb in Ortszeit des Dorfes.
        #expect(Verwaltungsdatum.text(ausAntwort: "2026-08-20T23:59:59+02:00") == "2026-08-20")
        #expect(Verwaltungsdatum.text(ausAntwort: "2026-08-20T21:59:59Z") == "2026-08-20")
        #expect(Verwaltungsdatum.text(ausAntwort: nil).isEmpty)
        #expect(Verwaltungsdatum.text(ausAntwort: "").isEmpty)
    }

    @Test func bestehendeEinmaligeAufgabeKommtMitIhremTerminInsFormular() {
        let aufgabe = Aufgabe(
            id: 5, placeId: 1, kind: "sonstiges", title: "Zum Bahnhof",
            oneOff: true, dueDate: "2026-08-20T23:59:59+02:00", removeWhenDone: true
        )
        let formular = AufgabeFormular(ortId: 1, aufgabe: aufgabe)
        #expect(formular.einmalig)
        #expect(Verwaltungsdatum.text(formular.termin) == "2026-08-20")
        let felder = Self.json(formular.eingabe ?? AufgabeEingabe(kind: "sonstiges"))
        #expect(felder["dueDate"] as? String == "2026-08-20")
        #expect(felder["removeWhenDone"] as? Bool == true)
    }

    // MARK: - Koordinate aus dem Kartentipp

    @Test func kartentippLandetInDerEingabe() {
        var formular = OrtFormular()
        formular.name = "Neuer Kasten"
        // Ohne Standort gibt es nichts zu speichern.
        #expect(!formular.speicherbar)

        // Genau das, was `KarteView.flaecheGetippt` liefert.
        formular.punkt = Kartenpunkt(breite: 52.19012, laenge: 9.81234)
        #expect(formular.speicherbar)

        let felder = Self.json(formular.eingabe ?? OrtEingabe(name: "", lat: 0, lon: 0))
        #expect(felder["lat"] as? Double == 52.19012)
        #expect(felder["lon"] as? Double == 9.81234)
        #expect(felder["name"] as? String == "Neuer Kasten")
    }

    @Test func gewaehlterPunktStehtAlsGeoJsonBereit() {
        // GeoJSON zählt Länge vor Breite. Wer das dreht, legt das Dorf nach
        // Somalia — dieselbe Falle wie bei den Ortsnadeln.
        let daten = Kartendaten.auswahlGeoJson(Kartenpunkt(breite: 52.19, laenge: 9.81))
        let objekt = (try? JSONSerialization.jsonObject(with: daten)) as? [String: Any]
        let merkmale = objekt?["features"] as? [[String: Any]]
        let geometrie = merkmale?.first?["geometry"] as? [String: Any]
        #expect(geometrie?["coordinates"] as? [Double] == [9.81, 52.19])

        // Ohne Auswahl bleibt die Sammlung leer — und die Ebene zeigt nichts.
        let leer = Kartendaten.auswahlGeoJson(nil)
        let leeresObjekt = (try? JSONSerialization.jsonObject(with: leer)) as? [String: Any]
        #expect((leeresObjekt?["features"] as? [[String: Any]])?.isEmpty == true)

        // Genau 0/0 liegt im Golf von Guinea: keine Wahl, sondern eine Lücke.
        let ungueltig = Kartendaten.auswahlGeoJson(Kartenpunkt(breite: 0, laenge: 0))
        let ungueltigesObjekt = (try? JSONSerialization.jsonObject(with: ungueltig)) as? [String: Any]
        #expect((ungueltigesObjekt?["features"] as? [[String: Any]])?.isEmpty == true)
    }

    // MARK: - Das Backend entscheidet

    @Test func dreiundvierzigNulldreiZeigtDenWortlautDesBackends() {
        let fehler = DorfApi.fehler(
            status: 403, daten: Self.fehlerdaten("admin-Rolle erforderlich")
        )
        guard case .keineBerechtigung(let grund) = fehler else {
            Issue.record("403 ergab \(fehler) statt keineBerechtigung")
            return
        }
        #expect(grund == "admin-Rolle erforderlich")
        #expect(fehler.klartext == "admin-Rolle erforderlich")
    }

    @Test func ohneBegruendungStehtWenigstensEinSatzDa() {
        let fehler = DorfApi.fehler(status: 403, daten: Data())
        #expect(fehler.klartext == "Dafür fehlt die Berechtigung.")
    }

    @Test func vierhundertZeigtDenGrundDerPruefung() {
        let fehler = DorfApi.fehler(
            status: 400,
            daten: Self.fehlerdaten("dueDate gibt es nur bei einmaligen Aufgaben (oneOff)")
        )
        #expect(fehler.klartext == "dueDate gibt es nur bei einmaligen Aufgaben (oneOff)")
    }

    @Test func weitereStatuscodesWerdenUebersetzt() {
        #expect(DorfApi.fehler(status: 401, daten: Data()).klartext
            == DorfFehler.nichtAngemeldet.klartext)
        #expect(DorfApi.fehler(status: 404, daten: Data()).klartext
            == DorfFehler.nichtGefunden.klartext)
        #expect(DorfApi.fehler(status: 429, daten: Data()).klartext
            == DorfFehler.zuVieleAnfragen.klartext)
        #expect(DorfApi.fehler(status: 500, daten: Data()).klartext
            == DorfFehler.serverfehler(status: 500).klartext)
    }

    @Test func abgelehntesFormularBleibtStehenUndZeigtDenWortlaut() async {
        let protokoll = Protokoll()
        protokoll.abweisen = .keineBerechtigung(grund: "admin-Rolle erforderlich")
        let modell = VerwaltungModell(quelle: Self.quelle(protokoll))

        modell.ortBearbeiten(nil)
        modell.ortFormular?.name = "Neuer Kasten"
        modell.ortFormular?.punkt = Kartenpunkt(breite: 52.19, laenge: 9.81)
        await modell.ortSpeichern()

        // Das Formular ist noch da, samt allem Getippten — sonst tippt jemand
        // am Blumenkasten alles noch einmal.
        #expect(modell.ortFormular?.name == "Neuer Kasten")
        #expect(modell.ortFormular?.fehler == "admin-Rolle erforderlich")
        #expect(modell.ortFormular?.sendet == false)
        #expect(protokoll.neuGeladen == 0)
    }

    @Test func angenommenerOrtSchliesstDasFormularUndLaedtNeu() async {
        let protokoll = Protokoll()
        let modell = VerwaltungModell(quelle: Self.quelle(protokoll))

        modell.ortBearbeiten(nil)
        modell.ortFormular?.name = "  Neuer Kasten  "
        modell.ortFormular?.punkt = Kartenpunkt(breite: 52.19, laenge: 9.81)
        await modell.ortSpeichern()

        #expect(modell.ortFormular == nil)
        #expect(modell.bestaetigung == "Ort angelegt.")
        #expect(protokoll.neuGeladen == 1)
        // Leerzeichen am Rand gehören nicht in den Namen.
        #expect(protokoll.angelegteOrte.first?.name == "Neuer Kasten")
    }

    // MARK: - Pausieren und Löschen

    @Test func pausierenSchicktDenRestUnveraendertMit() async {
        let protokoll = Protokoll()
        let modell = VerwaltungModell(quelle: Self.quelle(protokoll))
        let aufgabe = Aufgabe(
            id: 9, placeId: 1, kind: "giessen", title: "Gießen", liters: 10,
            intervalDays: 5, redAfterDays: 9
        )
        await modell.aufgabeUmschalten(aufgabe, aktiv: false)

        let (id, eingabe) = protokoll.geaenderteAufgaben.first ?? (0, AufgabeEingabe(kind: ""))
        #expect(id == 9)
        let felder = Self.json(eingabe)
        #expect(felder["active"] as? Bool == false)
        #expect(felder["intervalDays"] as? Double == 5)
        #expect(felder["redAfterDays"] as? Double == 9)
        #expect(felder["liters"] as? Double == 10)
        #expect(modell.bestaetigung == "Aufgabe pausiert.")
        #expect(protokoll.neuGeladen == 1)
    }

    @Test func loeschenGehtUeberDieQuelleUndLaedtDanachNeu() async {
        let protokoll = Protokoll()
        let modell = VerwaltungModell(quelle: Self.quelle(protokoll))
        await modell.ortLoeschen(Self.ort(3))
        #expect(protokoll.geloeschteOrte == [3])
        #expect(protokoll.neuGeladen == 1)

        await modell.aufgabeLoeschen(Aufgabe(id: 4))
        #expect(protokoll.geloeschteAufgaben == [4])
        #expect(protokoll.neuGeladen == 2)
    }

    @Test func rueckfrageSagtWasAnDerAufgabeHaengt() {
        // „Wirklich löschen?" allein wäre zu wenig: Wer zugesagt hat, bekommt
        // vom Backend die Nachricht, dass es nicht mehr nötig ist.
        let frage = Rueckfrage.aufgabeLoeschen(Aufgabe(id: 1, kind: "giessen"))
        #expect(frage.text.contains("nicht mehr nötig"))
        #expect(Rueckfrage.ortLoeschen(Self.ort()).text.contains("nicht mehr nötig"))
        #expect(Rueckfrage.aufgabePausieren(Aufgabe(id: 1)).text.contains("nicht mehr nötig"))
        #expect(Rueckfrage.ortPausieren(Self.ort()).text.contains("nicht mehr nötig"))
        #expect(Rueckfrage.ortLoeschen(Self.ort()).text.contains("Unter den Eichen"))
    }

    // MARK: - Hitzefaktor

    @Test func hitzefaktorKommtVomBackendUndGehtDorthinZurueck() async {
        let protokoll = Protokoll()
        protokoll.faktorDesBackends = 0.5
        let modell = VerwaltungModell(quelle: Self.quelle(protokoll))

        await modell.einstellungenLaden()
        #expect(modell.hitzefaktor == 0.5)
        #expect(modell.hitzefaktorGeladen)

        await modell.hitzefaktorSetzen(1.5)
        #expect(protokoll.gesetzteFaktoren == [1.5])
        // Angezeigt wird der Stand des Backends, nicht der geschickte Wunsch.
        #expect(modell.hitzefaktor == 1.5)
        #expect(modell.bestaetigung == "Hitzefaktor gespeichert.")
    }

    @Test func abgelehnterHitzefaktorZeigtDenWortlautUndAendertNichts() async {
        let protokoll = Protokoll()
        let modell = VerwaltungModell(quelle: Self.quelle(protokoll))
        protokoll.abweisen = .abgelehnt(grund: "wateringFactor muss zwischen 0 und 4 liegen")

        await modell.hitzefaktorSetzen(9)
        #expect(modell.fehler == "wateringFactor muss zwischen 0 und 4 liegen")
        #expect(modell.hitzefaktor == 1)
    }

    @Test func nurDerHitzefaktorGehtMit() {
        // Die Vergabe-Regeln stehen in derselben Antwort. Sie gehören einem
        // anderen Bereich und dürfen von einem Zug am Hitzefaktor nicht
        // überschrieben werden.
        let felder = Self.json(HitzefaktorEingabe(wateringFactor: 0.5))
        #expect(felder.count == 1)
        #expect(felder["wateringFactor"] as? Double == 0.5)
    }

    @Test func einstellungenLesenUebergehtWasNichtHierherGehoert() {
        let roh = Data(#"{"wateringFactor":0.75,"assignment":{"enabled":true}}"#.utf8)
        let einstellungen = try? JSONDecoder().decode(Einstellungen.self, from: roh)
        #expect(einstellungen?.wateringFactor == 0.75)

        // Fehlt das Feld, gilt „normal" — wie im Backend.
        let leer = try? JSONDecoder().decode(Einstellungen.self, from: Data("{}".utf8))
        #expect(leer?.wateringFactor == 1)
    }

    // MARK: - Formular aus einem bestehenden Datensatz

    @Test func bestehenderOrtKommtVollstaendigInsFormular() {
        let formular = OrtFormular(ort: Self.ort(12))
        #expect(!formular.neu)
        #expect(formular.name == "Unter den Eichen")
        #expect(formular.beschreibung == "Am Zaun")
        #expect(formular.art == "blumenkasten")
        #expect(formular.punkt == Kartenpunkt(breite: 52.1832, laenge: 9.8168))
        #expect(formular.aktiv)
    }

    @Test func zahlenMitKommaWerdenGelesen() {
        // Auf einer deutschen Tastatur steht das Komma.
        #expect(Verwaltungszahl.wert("10,5") == 10.5)
        #expect(Verwaltungszahl.wert("10.5") == 10.5)
        #expect(Verwaltungszahl.wert(" 7 ") == 7)
        #expect(Verwaltungszahl.wert("") == nil)
        #expect(Verwaltungszahl.wert("viel") == nil)
        #expect(Verwaltungszahl.text(7) == "7")
        #expect(Verwaltungszahl.text(7.5) == "7,5")
    }

    @Test func ohneIntervallGibtEsNichtsZuSchicken() {
        var formular = AufgabeFormular(ortId: 1)
        formular.intervall = ""
        #expect(formular.eingabe == nil)
        #expect(!formular.speicherbar)
    }
}
