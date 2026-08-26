import Foundation
import Testing

@testable import Dorf

/// Die Karte selbst lässt sich nicht sinnvoll im Unit-Test laden — deshalb
/// liegt die ganze Rechnerei in `Kartenrechnung.swift` und wird hier geprüft.
/// Kein Test hier lädt einen Stil, eine Kachel oder sonst etwas aus dem Netz.
struct KarteTests {
    private let orte = [
        Ort(id: 1, name: "Unter den Eichen", lat: 52.1832, lon: 9.8168, status: "red"),
        Ort(id: 2, name: "Am Bahnhof", lat: 52.1961, lon: 9.8151, status: "yellow"),
        Ort(id: 3, name: "Kirchplatz", lat: 52.1902, lon: 9.8102),
    ]

    // MARK: - Merkmale

    @Test func merkmaleTragenNamenPunktUndAmpel() {
        let merkmale = Kartendaten.merkmale(aus: orte)
        #expect(merkmale.count == 3)

        let erstes = merkmale[0]
        #expect(erstes.id == 1)
        #expect(erstes.name == "Unter den Eichen")
        #expect(erstes.ampel == .red)
        #expect(erstes.punkt.breite == 52.1832)
        #expect(erstes.punkt.laenge == 9.8168)
        // Ohne Angabe ist die Ampel grün — wie im Backend.
        #expect(merkmale[2].ampel == .green)
    }

    @Test func abgeschalteteUndOrtloseOrteKommenNichtAufDieKarte() {
        let gemischt = [
            Ort(id: 1, name: "Sichtbar", lat: 52.19, lon: 9.81),
            Ort(id: 2, name: "Abgeschaltet", lat: 52.19, lon: 9.81, active: false),
            // Ohne Koordinate im Backend: 0/0 liegt im Golf von Guinea und
            // würde den Startausschnitt über den halben Globus ziehen.
            Ort(id: 3, name: "Ohne Koordinate", lat: 0, lon: 0),
        ]
        let merkmale = Kartendaten.merkmale(aus: gemischt)
        #expect(merkmale.map(\.id) == [1])
    }

    // MARK: - GeoJSON

    @Test func geoJsonIstEineFeatureCollectionMitAmpelEigenschaft() throws {
        let daten = Kartendaten.geoJson(aus: orte)
        let gelesen = try JSONSerialization.jsonObject(with: daten) as? [String: Any]
        let sammlung = try #require(gelesen)
        #expect(sammlung["type"] as? String == "FeatureCollection")

        let merkmale = try #require(sammlung["features"] as? [[String: Any]])
        #expect(merkmale.count == 3)

        let erstes = merkmale[0]
        #expect(erstes["type"] as? String == "Feature")

        let geometrie = try #require(erstes["geometry"] as? [String: Any])
        #expect(geometrie["type"] as? String == "Point")
        let koordinaten = try #require(geometrie["coordinates"] as? [Double])
        // GeoJSON zählt Länge vor Breite.
        #expect(koordinaten == [9.8168, 52.1832])

        let eigenschaften = try #require(erstes["properties"] as? [String: Any])
        #expect((eigenschaften["id"] as? NSNumber)?.int64Value == 1)
        #expect(eigenschaften["name"] as? String == "Unter den Eichen")
        #expect(eigenschaften["ampel"] as? String == "red")
        #expect(merkmale[1]["properties"].flatMap { ($0 as? [String: Any])?["ampel"] as? String } == "yellow")
        #expect(merkmale[2]["properties"].flatMap { ($0 as? [String: Any])?["ampel"] as? String } == "green")
    }

    @Test func leereListeErgibtEineLeereSammlung() throws {
        let daten = Kartendaten.geoJson(aus: [])
        let sammlung = try #require(try JSONSerialization.jsonObject(with: daten) as? [String: Any])
        #expect(sammlung["type"] as? String == "FeatureCollection")
        #expect((sammlung["features"] as? [[String: Any]])?.isEmpty == true)
    }

    // MARK: - Startausschnitt

    @Test func mittelpunktLiegtZwischenDenOrten() {
        let start = Kartendaten.start(fuer: orte, breiteInPunkten: 390, hoeheInPunkten: 700)
        #expect(start.mitte.laenge == (9.8102 + 9.8168) / 2)
        // In Nord-Süd-Richtung wird in Mercator gemittelt; auf dieser Breite
        // liegt das Ergebnis dicht an der Mitte der Grenzwerte.
        #expect(abs(start.mitte.breite - (52.1832 + 52.1961) / 2) < 0.0005)
        #expect(start.mitte.breite > 52.1832)
        #expect(start.mitte.breite < 52.1961)
    }

    @Test func leereListeZeigtDenOrtskernVonRoessing() {
        let start = Kartendaten.start(fuer: [], breiteInPunkten: 390, hoeheInPunkten: 700)
        #expect(start.mitte == Karteneinstellungen.roessing)
        #expect(start.zoom == Karteneinstellungen.startZoom)
    }

    @Test func ohneBekannteKartengroesseBleibtEsBeimStandardzoom() {
        let start = Kartendaten.start(fuer: orte, breiteInPunkten: 0, hoeheInPunkten: 0)
        #expect(start.zoom == Karteneinstellungen.startZoom)
        // Die Mitte steht trotzdem schon richtig.
        #expect(start.mitte.laenge == (9.8102 + 9.8168) / 2)
    }

    @Test func einzelnerOrtZoomtNichtBisAufDieHausnummer() {
        let start = Kartendaten.start(
            fuer: [Ort(id: 9, name: "Einziger", lat: 52.19, lon: 9.81)],
            breiteInPunkten: 390,
            hoeheInPunkten: 700
        )
        #expect(start.mitte.laenge == 9.81)
        #expect(start.zoom == Karteneinstellungen.hoechsterStartZoom)
    }

    @Test func alleOrteLiegenImStartausschnitt() {
        let breite = 390.0
        let hoehe = 700.0
        let start = Kartendaten.start(fuer: orte, breiteInPunkten: breite, hoeheInPunkten: hoehe)
        let masstab = Karteneinstellungen.kachelInPunkten * pow(2, start.zoom)

        // Der Ausschnitt sitzt genau auf Kante — ein halber Punkt Toleranz
        // gegen die Rundung der Fließkommarechnung.
        let toleranz = 0.5

        for merkmal in Kartendaten.merkmale(aus: orte) {
            let abstandX = abs(merkmal.punkt.laenge - start.mitte.laenge) / 360 * masstab
            let abstandY = abs(Mercator.y(merkmal.punkt.breite) - Mercator.y(start.mitte.breite)) * masstab
            // Der halbe Bildschirm abzüglich des Randes muss reichen.
            #expect(abstandX <= breite / 2 - Karteneinstellungen.randInPunkten + toleranz)
            #expect(abstandY <= hoehe / 2 - Karteneinstellungen.randInPunkten + toleranz)
        }
    }

    @Test func einGroesseresGebietWirdWeiterHerausgezoomt() {
        let eng = Kartenrechteck(sued: 52.19, west: 9.81, nord: 52.192, ost: 9.812)
        let weit = Kartenrechteck(sued: 52.15, west: 9.75, nord: 52.25, ost: 9.90)
        let zoomEng = Kartendaten.zoom(fuer: eng, breiteInPunkten: 390, hoeheInPunkten: 700)
        let zoomWeit = Kartendaten.zoom(fuer: weit, breiteInPunkten: 390, hoeheInPunkten: 700)
        #expect(zoomWeit < zoomEng)
        #expect(zoomEng <= Karteneinstellungen.hoechsterStartZoom)
        #expect(zoomWeit > 0)
    }

    // MARK: - Rechteck

    @Test func rechteckUmschliesstAllePunkte() throws {
        let punkte = Kartendaten.merkmale(aus: orte).map(\.punkt)
        let rechteck = try #require(Kartenrechteck(punkte: punkte))
        #expect(rechteck.sued == 52.1832)
        #expect(rechteck.nord == 52.1961)
        #expect(rechteck.west == 9.8102)
        #expect(rechteck.ost == 9.8168)
    }

    @Test func ohnePunkteGibtEsKeinRechteck() {
        #expect(Kartenrechteck(punkte: []) == nil)
    }
}
