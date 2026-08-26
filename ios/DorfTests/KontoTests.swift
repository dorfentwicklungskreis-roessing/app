import Foundation
import Testing

@testable import Dorf

/// Konto, Einstellungen und die Zahlen der Startseite.
///
/// Der Schwerpunkt liegt auf dem, was sich nicht zurücknehmen lässt: Ein
/// Konto zu löschen ist endgültig. Deshalb steht hier als Erstes, dass es
/// **nicht** aus Versehen geht, und als Zweites, dass ein gescheiterter
/// Versuch niemanden im Ungewissen abmeldet.
///
/// Kein Test geht ins Netz: Die Quelle des Modells ist ein Bündel
/// Verschlüsse, das der Test selbst füllt.
@MainActor
struct KontoTests {
    // MARK: Werkzeug

    /// Merkt sich, was das Modell tatsächlich getan hat.
    private final class Spur {
        var loeschversuche = 0
        var abmeldungen = 0
        var fehler: DorfFehler?
    }

    private static func quelle(_ spur: Spur) -> Kontoquelle {
        Kontoquelle(
            loeschen: {
                spur.loeschversuche += 1
                if let fehler = spur.fehler { throw fehler }
                return Kontoloeschung(
                    geloescht: true,
                    erledigungen: "Deine Meldungen bleiben anonym stehen.",
                    roessingId: "Deine Rössing-ID bleibt bestehen."
                )
            },
            abmelden: { spur.abmeldungen += 1 }
        )
    }

    private static func modell(_ spur: Spur, name: String = "Erna Beispiel") -> KontoModell {
        KontoModell(erwarteterName: name, quelle: Self.quelle(spur))
    }

    private static func ort(_ id: Int64, _ name: String, _ status: String,
                            aktiv: Bool = true) -> Ort {
        Ort(id: id, name: name, lat: 52.1, lon: 9.8, active: aktiv, status: status)
    }

    // MARK: Die Rückfrage

    @Test func rueckfrageLaesstSichNichtVersehentlichBestaetigen() async {
        let spur = Spur()
        let modell = Self.modell(spur)
        modell.rueckfrageOeffnen()

        // Nichts getippt: Der Knopf ist zu.
        #expect(!modell.darfLoeschen)

        // Etwas anderes getippt: immer noch zu.
        modell.getippterName = "ja"
        #expect(!modell.darfLoeschen)
        modell.getippterName = "Erna"
        #expect(!modell.darfLoeschen, "Ein Teil des Namens genügt nicht")

        // Und selbst ein Aufruf trotz gesperrtem Knopf löscht nichts.
        await modell.loeschen()
        #expect(spur.loeschversuche == 0)
        #expect(spur.abmeldungen == 0)
        #expect(modell.abschied == nil)
    }

    @Test func derAbgetippteNameOeffnetDenKnopf() {
        let modell = Self.modell(Spur())
        modell.getippterName = "  erna beispiel "
        #expect(modell.darfLoeschen, "Leerraum und Großschreibung dürfen nicht entscheiden")
    }

    @Test func ohneBekanntenNamenWirdEinWortVerlangt() {
        let modell = Self.modell(Spur(), name: "")
        #expect(modell.bestaetigungswort == "löschen")
        modell.getippterName = ""
        #expect(!modell.darfLoeschen)
        modell.getippterName = "löschen"
        #expect(modell.darfLoeschen)
    }

    @Test func abbrechenLaesstDasFeldNichtStehen() {
        let modell = Self.modell(Spur())
        modell.rueckfrageOeffnen()
        modell.getippterName = "Erna Beispiel"
        modell.rueckfrageSchliessen()
        #expect(!modell.rueckfrageOffen)
        #expect(modell.getippterName.isEmpty, "Sonst wäre der Knopf beim nächsten Öffnen schon scharf")
        #expect(!modell.darfLoeschen)
    }

    // MARK: Nach dem Löschen

    @Test func nachDemLoeschenIstDerSchluesselbundLeerUndDieSitzungAbgemeldet() async {
        // Eine echte Anmeldung mit echtem Schlüsselbund — hier ist genau das
        // die Aussage: Nach dem Löschen liegt nichts mehr auf dem Gerät.
        Schluesselbund.sichern(Tokensatz(
            zugangstoken: "tok", erneuerungstoken: "neu", idToken: "id",
            laeuftAbAm: Date().addingTimeInterval(3600)
        ))
        let anmeldung = Anmeldung()
        #expect(anmeldung.sitzung == .angemeldet(entwicklerModus: false))

        let modell = KontoModell(
            erwarteterName: "Erna Beispiel",
            quelle: Kontoquelle(
                loeschen: { Kontoloeschung(geloescht: true) },
                abmelden: { anmeldung.abmelden() }
            )
        )
        modell.getippterName = "Erna Beispiel"
        await modell.loeschen()

        #expect(modell.abschied?.geloescht == true)
        #expect(anmeldung.sitzung == .abgemeldet)
        #expect(Schluesselbund.lesen() == nil, "Ein Token auf dem Gerät überlebt das Konto nicht")
        #expect(!modell.rueckfrageOffen)
        #expect(modell.fehler == nil)
    }

    @Test func dieAntwortDesBackendsWirdUebernommen() async {
        let spur = Spur()
        let modell = Self.modell(spur)
        modell.getippterName = "Erna Beispiel"
        await modell.loeschen()

        // Was mit den Erledigungen passiert und dass die Rössing-ID bleibt,
        // sagt das Backend — die App denkt es sich nicht aus.
        #expect(modell.abschied?.roessingId.contains("Rössing-ID") == true)
        #expect(modell.abschied?.erledigungen.isEmpty == false)
        #expect(spur.abmeldungen == 1)
    }

    // MARK: Wenn es schiefgeht

    @Test func fehlerBeimLoeschenMeldetDenWortlautUndMeldetNichtAb() async {
        let spur = Spur()
        spur.fehler = .keineBerechtigung(grund: "es lässt sich nur das eigene Konto löschen")
        let modell = Self.modell(spur)
        modell.getippterName = "Erna Beispiel"
        await modell.loeschen()

        #expect(spur.loeschversuche == 1)
        #expect(modell.fehler == "es lässt sich nur das eigene Konto löschen",
                "Der Wortlaut kommt aus dem Backend, nicht aus der App")
        #expect(spur.abmeldungen == 0,
                "Abmelden nach einem Fehlschlag ließe die Person im Ungewissen")
        #expect(modell.abschied == nil)
        #expect(!modell.laeuft)
    }

    @Test func einNetzausfallMeldetEbenfallsNichtAb() async {
        let spur = Spur()
        spur.fehler = .netz("offline")
        let modell = Self.modell(spur)
        modell.getippterName = "Erna Beispiel"
        await modell.loeschen()

        #expect(modell.fehler == DorfFehler.netz("").klartext)
        #expect(spur.abmeldungen == 0)
    }

    @Test func einZweiterVersuchIstNachEinemFehlerMoeglich() async {
        let spur = Spur()
        spur.fehler = .serverfehler(status: 500)
        let modell = Self.modell(spur)
        modell.getippterName = "Erna Beispiel"
        await modell.loeschen()
        #expect(modell.fehler != nil)

        spur.fehler = nil
        #expect(modell.darfLoeschen, "Der getippte Name steht noch — es geht sofort weiter")
        await modell.loeschen()
        #expect(modell.fehler == nil)
        #expect(spur.abmeldungen == 1)
    }

    // MARK: Startseite — wie viele Orte warten

    @Test func keinWartenderOrtGibtKeinenHinweis() {
        let orte = [Self.ort(1, "Anger", "green"), Self.ort(2, "Bahnhof", "green")]
        #expect(Startseitentexte.wartendeOrte(orte) == 0)
        #expect(Startseitentexte.mithelfenHinweis(orte: orte) == nil,
                "„0 Orte warten auf dich“ ist keine Nachricht")
        #expect(Startseitentexte.mithelfenHinweis(orte: []) == nil)
    }

    @Test func einWartenderOrtStehtImSingular() {
        let orte = [Self.ort(1, "Anger", "yellow"), Self.ort(2, "Bahnhof", "green")]
        #expect(Startseitentexte.wartendeOrte(orte) == 1)
        #expect(Startseitentexte.mithelfenHinweis(orte: orte) == "Ein Ort wartet auf dich")
    }

    @Test func mehrereWartendeOrteStehenImPlural() {
        let orte = [
            Self.ort(1, "Anger", "red"),
            Self.ort(2, "Bahnhof", "yellow"),
            Self.ort(3, "Kirche", "red"),
            Self.ort(4, "Schule", "green"),
        ]
        #expect(Startseitentexte.wartendeOrte(orte) == 3)
        #expect(Startseitentexte.mithelfenHinweis(orte: orte) == "3 Orte warten auf dich")
    }

    @Test func stillgelegteOrteZaehlenNicht() {
        let orte = [Self.ort(1, "Anger", "red", aktiv: false), Self.ort(2, "Bahnhof", "yellow")]
        #expect(Startseitentexte.wartendeOrte(orte) == 1)
    }

    // MARK: Startseite — Hitzefaktor

    @Test func hitzehinweisErscheintNurUnterEins() {
        #expect(Startseitentexte.hitzehinweis(giessfaktor: 0.5) == "Heiß — bitte großzügig gießen.")
        #expect(Startseitentexte.hitzehinweis(giessfaktor: 0.99) != nil)
        #expect(Startseitentexte.hitzehinweis(giessfaktor: 1) == nil, "1 ist der Normalfall")
        #expect(Startseitentexte.hitzehinweis(giessfaktor: 1.5) == nil,
                "Eine nasse Woche braucht keinen Hinweis auf der Startseite")
        #expect(Startseitentexte.hitzehinweis(giessfaktor: .nan) == nil)
    }

    @Test func dieStartseiteBenutztDenStandDesGeteiltenModells() async {
        // Ein Modell, zwei Leser: Was die Kachel zählt, ist derselbe Stand,
        // den „Mithelfen" zeigt.
        let modell = OrteModell(quelle: OrteQuelle(
            orte: {
                OrteAntwort(places: [Self.ort(1, "Anger", "red"), Self.ort(2, "Bahnhof", "green")],
                            wateringFactor: 0.5)
            },
            erledigungen: { _ in [] },
            melden: { id, liter, _ in Erledigung(id: 1, taskId: id, liters: liter) },
            zuruecknehmen: { _ in }
        ))
        await modell.laden()

        #expect(Startseitentexte.mithelfenHinweis(orte: modell.orte) == "Ein Ort wartet auf dich")
        #expect(Startseitentexte.hitzehinweis(giessfaktor: modell.giessfaktor) != nil)
    }

    // MARK: Über die App

    @Test func versionUndBuildKommenAusDemBundle() {
        // Der Wert selbst hängt am Build — geprüft wird, dass er nicht
        // erfunden ist und in der Anzeige beides zusammensteht.
        #expect(Appversion.anzeige == "\(Appversion.version) (\(Appversion.build))")
        #expect(!Appversion.anzeige.isEmpty)
    }
}
