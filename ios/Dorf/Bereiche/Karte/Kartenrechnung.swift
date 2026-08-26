import Foundation

/// Die Rechnerei hinter der Karte — Punkte, Ausschnitt und die GeoJSON-Merkmale
/// der Orte, als reine Funktionen.
///
/// Bewusst ohne MapLibre, ohne UIKit und ohne Netz: Nur so lässt sich der
/// Kartenaufbau in gewöhnlichen Unit-Tests prüfen (`DorfTests/KarteTests.swift`),
/// ohne dass ein Test eine Karte lädt oder einen Kachelserver anfasst. Dieselbe
/// Trennung wie in der Android-App (`android/.../data/Geo.kt`).

// MARK: - Punkt und Rechteck

/// Ein Ort auf der Erde. `breite` ist Nord/Süd (`lat`), `laenge` ist Ost/West
/// (`lon`) — in GeoJSON stehen sie in der umgekehrten Reihenfolge, deshalb
/// heißen sie hier nicht `x` und `y`.
struct Kartenpunkt: Hashable, Sendable {
    var breite: Double
    var laenge: Double

    init(breite: Double, laenge: Double) {
        self.breite = breite
        self.laenge = laenge
    }

    /// Eine Koordinate, die das Backend wirklich gesetzt hat. Genau 0/0 liegt
    /// im Golf von Guinea — das ist immer eine fehlende Angabe und kein
    /// Blumenkasten. Ein solcher Punkt würde sonst den Startausschnitt über
    /// den halben Globus ziehen.
    var gueltig: Bool {
        (-90...90).contains(breite)
            && (-180...180).contains(laenge)
            && !(breite == 0 && laenge == 0)
    }
}

/// Rechteck in Geo-Koordinaten (Süd/West/Nord/Ost).
struct Kartenrechteck: Hashable, Sendable {
    var sued: Double
    var west: Double
    var nord: Double
    var ost: Double

    init(sued: Double, west: Double, nord: Double, ost: Double) {
        self.sued = sued
        self.west = west
        self.nord = nord
        self.ost = ost
    }

    /// Das umschließende Rechteck aller Punkte — `nil`, wenn es keine gibt.
    init?(punkte: [Kartenpunkt]) {
        guard let erster = punkte.first else { return nil }
        var rechteck = Kartenrechteck(
            sued: erster.breite, west: erster.laenge,
            nord: erster.breite, ost: erster.laenge
        )
        for punkt in punkte.dropFirst() { rechteck = rechteck.erweitert(um: punkt) }
        self = rechteck
    }

    func erweitert(um punkt: Kartenpunkt) -> Kartenrechteck {
        Kartenrechteck(
            sued: min(sued, punkt.breite),
            west: min(west, punkt.laenge),
            nord: max(nord, punkt.breite),
            ost: max(ost, punkt.laenge)
        )
    }

    /// Die Mitte. In Nord-Süd-Richtung in Mercator-Projektion gemittelt, damit
    /// oben und unten wirklich gleich viel Luft bleibt.
    var mitte: Kartenpunkt {
        Kartenpunkt(
            breite: Mercator.breite(ausY: (Mercator.y(sued) + Mercator.y(nord)) / 2),
            laenge: (west + ost) / 2
        )
    }
}

/// Kameraeinstellung beim Öffnen der Karte.
struct Kartenstart: Hashable, Sendable {
    var mitte: Kartenpunkt
    var zoom: Double
}

// MARK: - Feste Werte

/// Alles, was die Karte an Zahlen braucht — an einer Stelle, damit Ansicht und
/// Test dieselben Werte benutzen.
enum Karteneinstellungen {
    /// Ortskern von Rössing. Startpunkt, solange keine Orte geladen sind.
    static let roessing = Kartenpunkt(breite: 52.196, laenge: 9.815)

    /// Zoomstufe für „ganz Rössing", wenn nichts anderes bekannt ist.
    static let startZoom = 15.0

    /// Obergrenze für den berechneten Startausschnitt: Ein einzelner Ort soll
    /// nicht bis auf Hausnummern-Ebene aufziehen, sondern die Umgebung zeigen.
    static let hoechsterStartZoom = 16.5

    /// „Ich sehe meine Umgebung" — nur für den bewussten Druck auf
    /// „Mein Standort".
    static let standortZoom = 16.0

    /// Rand um den Startausschnitt in Punkten. Deckt den Nadelradius samt Luft
    /// ab, damit keine Nadel am Bildrand klebt.
    static let randInPunkten = 48.0

    /// Kachelgröße der MapLibre-Zoomstufen: Bei Zoom z ist die Welt
    /// 512 · 2^z Punkte breit.
    static let kachelInPunkten = 512.0

    /// Radius der Nadel und ihres weißen Randes.
    static let nadelradius = 11.0
    static let nadelrand = 2.5

    /// Trefferfläche für den Tipp: ein Finger ist keine Mauszeigerspitze.
    /// 44 Punkte sind Apples Mindestmaß für eine Bedienfläche.
    static let trefferbreite = 44.0
}

// MARK: - Merkmale

/// Ein Ort, wie ihn die Karte braucht: Punkt, Name und Ampel. Aus dieser Liste
/// wird die GeoJSON-Quelle gebaut — die Karte legt keine einzelnen Nadeln an,
/// sondern eine Quelle mit einer Kreis-Ebene (wie auf Android).
struct Kartenmerkmal: Hashable, Sendable {
    var id: Int64
    var name: String
    var ampel: Ampel
    var punkt: Kartenpunkt
}

/// Der Bau der Kartendaten aus einer Ortsliste.
enum Kartendaten {
    /// Namen der GeoJSON-Eigenschaften. Die Kreis-Ebene färbt nach `ampel`,
    /// der Tipp findet den Ort über `id`, VoiceOver liest `name`.
    static let eigenschaftId = "id"
    static let eigenschaftName = "name"
    static let eigenschaftAmpel = "ampel"

    /// Die Orte, die auf die Karte gehören: abgeschaltete und solche ohne
    /// brauchbare Koordinate bleiben weg.
    static func merkmale(aus orte: [Ort]) -> [Kartenmerkmal] {
        orte.compactMap { ort in
            guard ort.active else { return nil }
            let punkt = Kartenpunkt(breite: ort.lat, laenge: ort.lon)
            guard punkt.gueltig else { return nil }
            return Kartenmerkmal(id: ort.id, name: ort.name, ampel: ort.ampel, punkt: punkt)
        }
    }

    /// Eine GeoJSON-`FeatureCollection` als Wörterbuch — so ist sie im Test
    /// ohne Umweg über eine Kartenbibliothek prüfbar.
    static func merkmalsammlung(_ merkmale: [Kartenmerkmal]) -> [String: Any] {
        [
            "type": "FeatureCollection",
            "features": merkmale.map { merkmal in
                [
                    "type": "Feature",
                    "geometry": [
                        "type": "Point",
                        // GeoJSON zählt Länge vor Breite. Wer das dreht, legt
                        // das Dorf nach Somalia.
                        "coordinates": [merkmal.punkt.laenge, merkmal.punkt.breite],
                    ] as [String: Any],
                    "properties": [
                        eigenschaftId: merkmal.id,
                        eigenschaftName: merkmal.name,
                        eigenschaftAmpel: merkmal.ampel.rawValue,
                    ] as [String: Any],
                ] as [String: Any]
            },
        ]
    }

    /// Dieselbe Sammlung als Daten, wie die Kartenquelle sie liest. Scheitert
    /// die Umwandlung wider Erwarten, bleibt die Karte leer statt kaputt.
    static func geoJson(_ merkmale: [Kartenmerkmal]) -> Data {
        let leer = Data(#"{"type":"FeatureCollection","features":[]}"#.utf8)
        guard let daten = try? JSONSerialization.data(withJSONObject: merkmalsammlung(merkmale))
        else { return leer }
        return daten
    }

    static func geoJson(aus orte: [Ort]) -> Data { geoJson(merkmale(aus: orte)) }

    // MARK: Ausschnitt

    /// Größte Zoomstufe, bei der das Rechteck samt Rand noch vollständig auf
    /// eine Fläche von `breiteInPunkten` × `hoeheInPunkten` passt.
    static func zoom(
        fuer rechteck: Kartenrechteck,
        breiteInPunkten: Double,
        hoeheInPunkten: Double,
        rand: Double = Karteneinstellungen.randInPunkten,
        hoechstens: Double = Karteneinstellungen.hoechsterStartZoom
    ) -> Double {
        let nutzbareBreite = max(breiteInPunkten - 2 * rand, 1)
        let nutzbareHoehe = max(hoeheInPunkten - 2 * rand, 1)
        // Anteil des Rechtecks an der ganzen Weltkarte (Web-Mercator, 0…1).
        let anteilX = max((rechteck.ost - rechteck.west) / 360, 1e-12)
        let anteilY = max(Mercator.y(rechteck.sued) - Mercator.y(rechteck.nord), 1e-12)
        let zoomX = log2(nutzbareBreite / (Karteneinstellungen.kachelInPunkten * anteilX))
        let zoomY = log2(nutzbareHoehe / (Karteneinstellungen.kachelInPunkten * anteilY))
        return min(max(min(zoomX, zoomY), 0), hoechstens)
    }

    /// Der Ausschnitt beim Öffnen: Sind Orte da, liegen sie möglichst alle im
    /// Bild; sonst zeigt die Karte den Ortskern von Rössing.
    ///
    /// Ist die Größe der Karte noch nicht bekannt (Breite oder Höhe 0), bleibt
    /// es beim Standardzoom — gerechnet wird erst, wenn es etwas zu rechnen
    /// gibt.
    static func start(
        fuer orte: [Ort],
        breiteInPunkten: Double,
        hoeheInPunkten: Double
    ) -> Kartenstart {
        start(
            fuerPunkte: merkmale(aus: orte).map(\.punkt),
            breiteInPunkten: breiteInPunkten,
            hoeheInPunkten: hoeheInPunkten
        )
    }

    static func start(
        fuerPunkte punkte: [Kartenpunkt],
        breiteInPunkten: Double,
        hoeheInPunkten: Double
    ) -> Kartenstart {
        guard let rechteck = Kartenrechteck(punkte: punkte) else {
            return Kartenstart(mitte: Karteneinstellungen.roessing, zoom: Karteneinstellungen.startZoom)
        }
        guard breiteInPunkten > 0, hoeheInPunkten > 0 else {
            return Kartenstart(mitte: rechteck.mitte, zoom: Karteneinstellungen.startZoom)
        }
        return Kartenstart(
            mitte: rechteck.mitte,
            zoom: zoom(fuer: rechteck, breiteInPunkten: breiteInPunkten, hoeheInPunkten: hoeheInPunkten)
        )
    }
}

// MARK: - Web-Mercator

/// Umrechnung Breitengrad ↔ Anteil der Weltkarte von oben (0…1).
enum Mercator {
    static func y(_ breite: Double) -> Double {
        let bogen = min(max(breite, -85.05112878), 85.05112878) * .pi / 180
        return (1 - log(tan(bogen) + 1 / cos(bogen)) / .pi) / 2
    }

    static func breite(ausY y: Double) -> Double {
        atan(sinh(.pi * (1 - 2 * y))) * 180 / .pi
    }
}
