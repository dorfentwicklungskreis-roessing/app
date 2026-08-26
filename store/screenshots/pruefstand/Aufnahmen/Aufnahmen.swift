import XCTest

/// Nimmt die sieben Bilder für den App Store auf.
///
/// Der Testlauf tippt sich durch die App wie ein Mensch — anders geht es
/// nicht: Die macOS-VM hat keinen Bildschirm, und `xcrun simctl` kann zwar
/// starten und knipsen, aber nicht tippen. Die Bilder gehen als
/// `XCTAttachment` in das Ergebnisbündel; herausgeholt werden sie danach mit
/// `xcresulttool export attachments` (siehe `store/screenshots/README.md`).
///
/// Die App wird **nicht** von diesem Projekt gebaut, sondern aus `ios/` und
/// vorher mit `simctl install` aufgespielt. Hier hängt sich der Lauf nur an
/// ihre Bundle-ID.
@MainActor
final class Aufnahmen: XCTestCase {
    private let app = XCUIApplication(bundleIdentifier: "de.roessing.app")

    /// Wie lange auf einen Netzabruf gewartet wird. Backend und Terminfeed
    /// laufen auf demselben Rechner; großzügig ist es trotzdem, weil ein
    /// kalter Simulator beim ersten Bild länger braucht.
    private let frist: TimeInterval = 40

    override func setUp() {
        continueAfterFailure = false
    }

    func testBilderAufnehmen() throws {
        app.launch()

        anmelden()
        knipsen("07-startseite")

        // 1./2. Mithelfen — erst die Liste (so öffnet der Bereich), dann die Karte.
        tippe(app.buttons["bereich-mithelfen"])
        warteAufOrte()
        knipsen("02-mithelfen-liste")

        let umschalter = app.segmentedControls["mithelfen-ansicht"]
        XCTAssertTrue(umschalter.waitForExistence(timeout: frist), "Umschalter Karte/Liste fehlt")
        umschalter.buttons["Karte"].tap()
        let karte = app.otherElements["dorfkarte"]
        XCTAssertTrue(karte.waitForExistence(timeout: frist), "Karte erschien nicht")
        // Der Kartenkern lädt Stil und Nadeln asynchron; ein Bild direkt nach
        // dem Umschalten zeigte eine leere Fläche.
        ruhe(4)
        knipsen("01-mithelfen-karte")

        // 3. Ortsdetail: der überfällige Kasten am Dorfplatz hat zwei Aufgaben,
        //    Historie und den Melden-Knopf.
        umschalter.buttons["Liste"].tap()
        warteAufOrte()
        tippe(app.staticTexts["Blumenkasten am Dorfplatz"])
        // Gesucht wird über die Beschriftung, nicht über
        // `accessibilityIdentifier("melden-…")`: SwiftUI fasst die Karte einer
        // Aufgabe zu einem Element zusammen, und dabei gewinnt die Kennung der
        // Karte („aufgabe-16") — der Knopf darunter trägt sie mit.
        let melden = app.buttons.matching(
            NSPredicate(format: "label BEGINSWITH %@", "Ich habe ")
        ).firstMatch
        XCTAssertTrue(melden.waitForExistence(timeout: frist), "Kein Melden-Knopf auf der Ortsseite")
        // Auf dem iPhone steht der Knopf ohne Rollen im Bild; auf kleineren
        // Geräten kann es eng werden, deshalb sicherheitshalber nachfassen.
        scrolleZu(melden)
        ruhe(1.5)
        knipsen("03-ortsdetail")
        zurueck()

        zurueckZurStartseite()

        // 4. Rangliste
        tippe(app.buttons["bereich-rangliste"])
        XCTAssertTrue(app.staticTexts["Anna B."].waitForExistence(timeout: frist),
                      "Rangliste blieb leer")
        ruhe(1)
        knipsen("04-rangliste")
        zurueck()

        // 5. Veranstaltungen
        tippe(app.buttons["bereich-was-ist-los-in-rössing"])
        XCTAssertTrue(app.staticTexts["Dorfflohmarkt"].waitForExistence(timeout: frist),
                      "Terminliste blieb leer")
        ruhe(1)
        knipsen("05-veranstaltungen")
        zurueck()

        // 6. Mein Profil
        tippe(app.buttons["bereich-mein-profil"])
        XCTAssertTrue(app.textFields["profil-feld-anzeigename"].waitForExistence(timeout: frist),
                      "Profilformular erschien nicht")
        // Ein Stück rollen: Oben stehen nur freigegebene Felder, und ein Bild
        // mit lauter gleichen Schaltern zeigt gerade nicht, worum es geht.
        // Nach dem Rollen liegen Anzeigename (frei), Nickname (frei) und
        // Telefon (nur Verwaltung) zusammen im Bild.
        rolle(um: 0.33)
        ruhe(1.5)
        knipsen("06-profil")
        zurueck()
    }

    // MARK: - Handgriffe

    /// Entwickler-Login (nur im Debug-Build mit `DEV_AUTH=1`). Ist die App
    /// noch angemeldet, gibt es den Knopf nicht — dann ist nichts zu tun.
    private func anmelden() {
        let knopf = app.buttons["anmeldung-entwickler"]
        if knopf.waitForExistence(timeout: 15) {
            knopf.tap()
        }
        XCTAssertTrue(app.buttons["bereich-mithelfen"].waitForExistence(timeout: frist),
                      "Startseite erschien nicht — läuft das lokale Backend?")
        // Der Hitzehinweis kommt erst, wenn die Orte geladen sind: ein
        // brauchbares Zeichen dafür, dass die Startseite fertig ist.
        _ = app.staticTexts["Heiß — bitte großzügig gießen."].waitForExistence(timeout: frist)
        ruhe(1)
    }

    private func warteAufOrte() {
        XCTAssertTrue(app.staticTexts["Blumenkasten am Dorfplatz"].waitForExistence(timeout: frist),
                      "Ortsliste blieb leer")
        ruhe(1)
    }

    private func tippe(_ element: XCUIElement, datei: StaticString = #filePath, zeile: UInt = #line) {
        XCTAssertTrue(element.waitForExistence(timeout: frist),
                      "Element nicht gefunden: \(element)", file: datei, line: zeile)
        element.tap()
    }

    private func zurueck() {
        let leiste = app.navigationBars.firstMatch
        XCTAssertTrue(leiste.waitForExistence(timeout: frist), "Keine Navigationsleiste")
        leiste.buttons.firstMatch.tap()
    }

    /// Nach mehreren `zurueck()` kann eine Zwischenseite stehen bleiben —
    /// solange zurück, bis die Bereichsliste wieder da ist.
    private func zurueckZurStartseite() {
        for _ in 0..<4 {
            if app.buttons["bereich-rangliste"].waitForExistence(timeout: 3) { return }
            zurueck()
        }
        XCTAssertTrue(app.buttons["bereich-rangliste"].waitForExistence(timeout: frist),
                      "Startseite nicht wieder erreicht")
    }

    /// Rollt, bis das Element wirklich sichtbar ist. `swipeUp()` verschiebt
    /// eine unbekannte Strecke; deshalb wird nach jedem Zug nachgesehen.
    private func scrolleZu(_ element: XCUIElement, hoechstens: Int = 6) {
        for _ in 0..<hoechstens {
            if element.exists && element.isHittable { return }
            app.swipeUp()
            ruhe(0.6)
        }
    }

    /// Rollt um einen Anteil der Bildschirmhöhe. `swipeUp()` wäre gröber und
    /// schwungvoller — hier soll genau ein Stück weit gerollt werden.
    private func rolle(um anteil: CGFloat) {
        let start = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.75))
        let ziel = app.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.75 - anteil))
        start.press(forDuration: 0.1, thenDragTo: ziel)
    }

    private func ruhe(_ sekunden: TimeInterval) {
        Thread.sleep(forTimeInterval: sekunden)
    }

    /// Ein Bild in voller Gerätegröße. `XCUIScreen` statt `app.screenshot()`:
    /// Nur so kommt die Statusleiste mit, und die gehört auf ein Store-Bild.
    private func knipsen(_ name: String) {
        let bild = XCUIScreen.main.screenshot()
        let anhang = XCTAttachment(screenshot: bild)
        anhang.name = name
        anhang.lifetime = .keepAlways
        add(anhang)
    }
}
