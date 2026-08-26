import Foundation
import Testing

@testable import Dorf

/// Was die Rangliste sagt, sagt sie über Menschen — deshalb steht hier als
/// Test, dass sie niemandem „Platz 0" ins Gesicht sagt, dass der Zeitraum
/// stimmt und dass eine Antwort ohne eigenen Eintrag die Seite nicht kostet.
struct RanglisteTests {

    // MARK: - Zeitraum im Klartext

    @Test func zeitraumDerSaisonStehtImKlartext() {
        // Das Backend liefert [von, bis) — der 1. November zählt nicht mehr.
        let text = Ranglistentexte.zeitraum(
            von: "2026-03-01T00:00:00+01:00",
            bis: "2026-11-01T00:00:00+01:00"
        )
        #expect(text == "1. März bis 31. Oktober 2026")
    }

    @Test func zeitraumImSelbenMonatNenntDenMonatNurEinmal() {
        let text = Ranglistentexte.zeitraum(
            von: "2026-08-24T00:00:00+02:00",
            bis: "2026-08-31T00:00:00+02:00"
        )
        #expect(text == "24. bis 30. August 2026")
    }

    @Test func zeitraumUeberDenJahreswechselNenntBeideJahre() {
        let text = Ranglistentexte.zeitraum(
            von: "2025-12-29T00:00:00+01:00",
            bis: "2026-01-05T00:00:00+01:00"
        )
        #expect(text == "29. Dezember 2025 bis 4. Januar 2026")
    }

    @Test func zeitraumGesamtIstNachBeidenSeitenOffen() {
        // „gesamt": Das Backend schickt das Jahr 1 und das Jahr 9999.
        let text = Ranglistentexte.zeitraum(
            von: "0001-01-01T00:00:00Z",
            bis: "9999-01-01T00:00:00Z"
        )
        #expect(text == "Alles, was je gemeldet wurde")
    }

    @Test func ohneGrenzenGibtEsKeinenZeitraumtext() {
        #expect(Ranglistentexte.zeitraum(von: "", bis: "") == nil)
    }

    // MARK: - Der eigene Rang

    @Test func ohneEigeneMeldungKommtEinFreundlicherHinweis() {
        let ich = Ranglistenzeile(rank: 0, userSub: "abc", userName: "Anna")
        let text = Ranglistentexte.eigenerRang(ich)
        #expect(text.contains("noch nichts gemeldet"))
        #expect(!text.contains("Platz 0"))
    }

    @Test func mitRangStehtDerPlatzDa() {
        let ich = Ranglistenzeile(rank: 4, userSub: "abc", userName: "Anna")
        #expect(Ranglistentexte.eigenerRang(ich).contains("Platz 4"))
    }

    @Test func ohneEigeneZeileBleibtEsBeimHinweis() {
        #expect(Ranglistentexte.eigenerRang(nil).contains("noch nichts gemeldet"))
    }

    // MARK: - Die eigene Zeile

    @Test func eigeneZeileWirdErkannt() {
        let meine = Ranglistenzeile(rank: 2, userSub: "sub-anna", userName: "Anna")
        let fremde = Ranglistenzeile(rank: 1, userSub: "sub-bernd", userName: "Bernd")
        #expect(meine.istMeine("sub-anna"))
        #expect(!fremde.istMeine("sub-anna"))
    }

    @Test func ohneAngemeldetesSubIstKeineZeileDieEigene() {
        // Zwei leere Zeichenketten sind keine Übereinstimmung.
        let ohneSub = Ranglistenzeile(rank: 1, userSub: "", userName: "Anna")
        #expect(!ohneSub.istMeine(nil))
        #expect(!ohneSub.istMeine(""))
    }

    @Test func ohneMeWirdDieEigeneZeileInDerListeGesucht() {
        let stand = Rangliste(
            entries: [
                Ranglistenzeile(rank: 1, userSub: "sub-bernd", userName: "Bernd"),
                Ranglistenzeile(rank: 2, userSub: "sub-anna", userName: "Anna"),
            ],
            me: nil
        )
        let gefunden = Ranglistentexte.eigeneZeile(stand, meinSub: "sub-anna")
        #expect(gefunden?.userName == "Anna")
        #expect(Ranglistentexte.eigeneZeile(stand, meinSub: "sub-cora") == nil)
    }

    // MARK: - Arten

    @Test func artenWerdenAufgeloest() {
        #expect(Ranglistentexte.arten(["giessen": 12, "jaeten": 3]) == "12× Gießen · 3× Jäten")
    }

    @Test func unbekannteArtenLaufenUnterPflege() {
        // Das Backend darf neue Arten einführen, ohne dass die App verstummt.
        #expect(Ranglistentexte.arten(["sonstiges": 2, "muellsammeln": 1]) == "3× Pflege")
        #expect(Ranglistentexte.artName("giessen") == "Gießen")
        #expect(Ranglistentexte.artName("jaeten") == "Jäten")
        #expect(Ranglistentexte.artName("was-auch-immer") == "Pflege")
    }

    @Test func leereArtenErgebenKeinenText() {
        #expect(Ranglistentexte.arten([:]).isEmpty)
        #expect(Ranglistentexte.arten(["giessen": 0]).isEmpty)
    }

    // MARK: - Antwort des Backends

    @Test func antwortOhneMeBrichtNichts() throws {
        let roh = """
        {"period":"saison","from":"2026-03-01T00:00:00+01:00","to":"2026-11-01T00:00:00+01:00",
         "entries":[{"rank":1,"userSub":"sub-bernd","userName":"Bernd","completions":4,
                     "byKind":{"giessen":4},"liters":40}],
         "totals":{"completions":4,"liters":40,"participants":1}}
        """.data(using: .utf8)!

        let stand = try JSONDecoder().decode(Rangliste.self, from: roh)
        #expect(stand.me == nil)
        #expect(stand.entries.count == 1)
        #expect(stand.totals.participants == 1)
        #expect(Ranglistentexte.eigeneZeile(stand, meinSub: "sub-anna") == nil)
        #expect(Ranglistentexte.eigenerRang(Ranglistentexte.eigeneZeile(stand, meinSub: "sub-anna"))
            .contains("noch nichts gemeldet"))
        #expect(Ranglistentexte.zeitraum(von: stand.from, bis: stand.to)
            == "1. März bis 31. Oktober 2026")
    }

    @Test func auszeichnungenKommenMit() throws {
        let roh = """
        {"entries":[{"rank":1,"userSub":"s","userName":"Erna","completions":9,
          "badges":[{"key":"giesskanne","label":"Gießkanne des Monats",
                     "description":"Die meisten Liter im laufenden Monat."}]}]}
        """.data(using: .utf8)!
        let stand = try JSONDecoder().decode(Rangliste.self, from: roh)
        let auszeichnung = try #require(stand.entries.first?.badges.first)
        #expect(auszeichnung.label == "Gießkanne des Monats")
        #expect(!auszeichnung.description.isEmpty)
    }

    // MARK: - Vorlesen

    @Test func vorlesetextNenntDenPlatzAusgeschrieben() {
        let zeile = Ranglistenzeile(
            rank: 2, userSub: "sub-anna", userName: "Anna", completions: 7,
            byKind: ["giessen": 7], liters: 70
        )
        let text = Ranglistentexte.vorlesen(zeile, eigen: true)
        #expect(text.contains("Platz 2"))
        #expect(text.contains("Anna"))
        #expect(text.contains("das bist du"))
        #expect(text.contains("7 Erledigungen"))
    }

    @Test func ohneRangSagtDerVorlesetextKeineNull() {
        let zeile = Ranglistenzeile(rank: 0, userSub: "sub-anna", userName: "Anna")
        let text = Ranglistentexte.vorlesen(zeile, eigen: false)
        #expect(text.contains("Noch kein Platz"))
        #expect(!text.contains("Platz 0"))
    }

    @Test func medaillenGibtEsNurFuerDieErstenDrei() {
        #expect(Ranglistentexte.medaille(1) == "🥇")
        #expect(Ranglistentexte.medaille(2) == "🥈")
        #expect(Ranglistentexte.medaille(3) == "🥉")
        #expect(Ranglistentexte.medaille(4) == nil)
        #expect(Ranglistentexte.medaille(0) == nil)
    }

    @Test func letzteErledigungKommtRelativ() {
        let vorDreiTagen = Date().addingTimeInterval(-3 * 24 * 3600)
        let roh = ISO8601DateFormatter().string(from: vorDreiTagen)
        let text = Ranglistentexte.letzteErledigung(roh)
        #expect(text?.contains("3") == true)
        #expect(Ranglistentexte.letzteErledigung(nil) == nil)
        #expect(Ranglistentexte.letzteErledigung("kein Datum") == nil)
    }
}
