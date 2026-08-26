import Foundation
import Testing

@testable import Dorf

/// Das Ideen-Formular hat eine einzige harte Zusage: Niemand tippt seine Idee
/// zweimal. Deshalb steht hier vor allem, was bei einem Fehler **nicht**
/// passiert. Kein Test geht ins Netz — der Weg zum Backend wird als Abschluss
/// übergeben.
struct IdeenTests {
    /// Eine Antwort des Backends, wie sie nach einer angenommenen Idee kommt.
    private static func idee(_ roh: String = "{}") throws -> Idee {
        try JSONDecoder().decode(Idee.self, from: Data(roh.utf8))
    }

    private static func angenommen(_ eingabe: IdeeEingabe) async throws -> Idee {
        try idee()
    }

    // MARK: Vorbelegung

    @Test func vorbelegungKommtAusDemProfil() {
        let modell = IdeenModell()
        modell.vorbelegen(aus: Ich(
            sub: "abc", name: "Anna aus der Rössing-ID", email: "id@example.org",
            profile: Profil(displayName: "Anna Beispiel", email: "anna@example.org")
        ))
        #expect(modell.name == "Anna Beispiel")
        #expect(modell.email == "anna@example.org")
    }

    @Test func ohneProfilTraegtDieRoessingIdNach() {
        let modell = IdeenModell()
        modell.vorbelegen(aus: Ich(sub: "abc", name: "Anna", email: "anna@example.org"))
        #expect(modell.name == "Anna")
        #expect(modell.email == "anna@example.org")
    }

    @Test func ohneAngemeldetePersonBleibtAllesLeer() {
        // Name und E-Mail sind freiwillig: Wer nichts hinterlegt hat, sieht
        // leere Felder — und keinen Fehler.
        let modell = IdeenModell()
        modell.vorbelegen(aus: nil)
        #expect(modell.name.isEmpty)
        #expect(modell.email.isEmpty)
    }

    @Test func getipptesUeberlebtDieVorbelegung() {
        // Das Profil kommt womöglich erst nach dem Öffnen an — wer schon
        // getippt hat, verliert nichts.
        let modell = IdeenModell()
        modell.setzeName("Jemand anderes")
        modell.setzeEmail("")
        modell.vorbelegen(aus: Ich(
            sub: "abc", name: "Anna",
            profile: Profil(displayName: "Anna Beispiel", email: "anna@example.org")
        ))
        #expect(modell.name == "Jemand anderes")
        #expect(modell.email == "anna@example.org")
    }

    // MARK: Absenden

    @Test func leererWunschSperrtDasAbsenden() async {
        let modell = IdeenModell()
        #expect(!modell.absendbar)

        modell.setzeWunsch("   ")
        #expect(!modell.absendbar)

        var gerufen = false
        await modell.absenden(ueber: { _ in
            gerufen = true
            return try Self.idee()
        })
        #expect(!gerufen)
        #expect(modell.fehler == nil)
        #expect(!modell.dank)

        modell.setzeWunsch("Bushaltestelle anzeigen")
        #expect(modell.absendbar)
    }

    @Test func nachErfolgIstNurDerWunschLeer() async throws {
        let modell = IdeenModell()
        modell.setzeWunsch("  Ich möchte sehen, wann der nächste Bus fährt.  ")
        modell.setzeName("Anna Beispiel")
        modell.setzeEmail("anna@example.org")

        var gesehen: IdeeEingabe?
        await modell.absenden(ueber: { eingabe in
            gesehen = eingabe
            return try Self.idee(#"{"id":7,"wunsch":"…","quelle":"app"}"#)
        })

        // Abgeschickt wird ohne Leerraum am Rand.
        let eingabe = try #require(gesehen)
        #expect(eingabe.wunsch == "Ich möchte sehen, wann der nächste Bus fährt.")
        #expect(eingabe.name == "Anna Beispiel")
        #expect(eingabe.email == "anna@example.org")

        // Nur das Wunschfeld wird frei — Name und E-Mail bleiben stehen,
        // damit die nächste Idee ohne Tipparbeit hineinpasst.
        #expect(modell.wunsch.isEmpty)
        #expect(modell.name == "Anna Beispiel")
        #expect(modell.email == "anna@example.org")
        #expect(modell.dank)
        #expect(modell.fehler == nil)
        #expect(!modell.sendet)
    }

    @Test func nachAblehnungBleibtDerTextStehen() async {
        let modell = IdeenModell()
        modell.setzeWunsch("Ein Fahrplan für den Bus")
        modell.setzeName("Anna Beispiel")

        await modell.absenden(ueber: { _ in
            throw DorfFehler.abgelehnt(grund: "Bitte schreib mindestens fünf Zeichen.")
        })

        // Die Begründung des Backends im Wortlaut — die App erfindet keine
        // eigene.
        #expect(modell.fehler == "Bitte schreib mindestens fünf Zeichen.")
        // Niemand tippt seine Idee zweimal.
        #expect(modell.wunsch == "Ein Fahrplan für den Bus")
        #expect(modell.name == "Anna Beispiel")
        #expect(!modell.dank)
        #expect(!modell.sendet)
        #expect(modell.absendbar)
    }

    @Test func zuVieleAnfragenErgibtDenEigenenHinweis() async {
        let modell = IdeenModell()
        modell.setzeWunsch("Noch eine Idee")

        await modell.absenden(ueber: { _ in throw DorfFehler.zuVieleAnfragen })

        #expect(modell.fehler == IdeenModell.zuVieleIdeen)
        #expect(modell.fehler?.contains("in einer Stunde") == true)
        #expect(modell.wunsch == "Noch eine Idee")
    }

    @Test func ohneVerbindungBleibtEbenfallsAllesStehen() async {
        let modell = IdeenModell()
        modell.setzeWunsch("Noch eine Idee")

        await modell.absenden(ueber: { _ in throw DorfFehler.netz("nicht erreichbar") })

        #expect(modell.fehler == IdeenModell.nichtGeklappt)
        #expect(modell.wunsch == "Noch eine Idee")
    }

    @Test func weiterTippenRaeumtDieMeldungWeg() async {
        let modell = IdeenModell()
        modell.setzeWunsch("Ein Fahrplan für den Bus")
        await modell.absenden(ueber: { _ in throw DorfFehler.abgelehnt(grund: "Nein.") })
        #expect(modell.fehler != nil)

        modell.setzeWunsch("Ein Fahrplan für den Bus mit Abfahrtszeit")
        #expect(modell.fehler == nil)
    }

    // MARK: Zeichenzähler

    @Test func zaehlerZaehltBuchstabenNichtBytes() {
        let modell = IdeenModell()

        modell.setzeWunsch("Grüße")
        #expect(modell.zeichen == 5)
        #expect(modell.zaehlerText == "5 von 2000 Zeichen")

        // Ein Emoji ist ein Zeichen, auch wenn es aus mehreren
        // Unicode-Bausteinen besteht.
        modell.setzeWunsch("Bus 👍")
        #expect(modell.zeichen == 5)

        modell.setzeWunsch("👨‍👩‍👧")
        #expect(modell.zeichen == 1)

        modell.setzeWunsch("")
        #expect(modell.zaehlerText == "0 von 2000 Zeichen")
    }

    @Test func laengerAlsErlaubtWirdGarNichtErstAngenommen() {
        let modell = IdeenModell()
        modell.setzeWunsch(String(repeating: "ä", count: IdeenModell.maxZeichen + 50))
        #expect(modell.zeichen == IdeenModell.maxZeichen)
        #expect(modell.zaehlerText == "2000 von 2000 Zeichen")
    }
}
