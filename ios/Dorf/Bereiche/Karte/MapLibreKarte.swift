import MapLibre
import SwiftUI
import UIKit

/// Die eigentliche Karte: ein `MLNMapView` in SwiftUI-Kleidung.
///
/// Die Orte kommen als **eine** GeoJSON-Quelle mit **einer** Kreis-Ebene in
/// die Karte, nicht als einzelne Nadeln — genau wie in der Android-App
/// (`MapScreen.kt`). Das hält den Kartenkern bei hunderten Orten flüssig und
/// macht das Aktualisieren billig: Es wird nur die Quelle neu gefüttert, die
/// Ebene bleibt stehen.
struct MapLibreKarte: UIViewRepresentable {
    /// Kennungen von Quelle und Ebene. Sie werden nur einmal je Stil angelegt
    /// (`stil.source(withIdentifier:)` prüft das) — eine Ebene doppelt
    /// anzulegen wirft in MapLibre.
    static let quellenkennung = "orte"
    static let ebenenkennung = "orte-kreise"

    let orte: [Ort]
    /// Die Größe der Karte in Punkten. Sie kommt aus SwiftUI, weil
    /// `updateUIView` das erste Mal läuft, bevor die Ansicht ausgelegt ist —
    /// ohne sie bliebe der Startausschnitt beim Standardzoom stehen.
    var groesse: CGSize
    /// Eigenen Standortpunkt zeigen. Erst wahr, wenn die Freigabe da ist.
    var eigenenStandortZeigen: Bool
    /// Zählt jeden Druck auf „Mein Standort". Steigt er, fährt die Karte
    /// einmal hin; ein wiederholter Druck fährt wieder hin.
    var hinfahren: Int
    /// Steigt bei „Erneut versuchen", wenn der Stil nicht geladen werden konnte.
    var stilVersuch: Int
    var auswahl: (Ort) -> Void
    /// Meldet der Oberfläche, ob der Stil steht (`nil`) oder woran es hakt.
    var stilzustand: (String?) -> Void

    func makeCoordinator() -> Koordinator {
        Koordinator(auswahl: auswahl, stilzustand: stilzustand)
    }

    func makeUIView(context: Context) -> MLNMapView {
        // Der Stil kommt ausschließlich aus der Konfiguration: So kann die CI
        // einen lokalen Stil einsetzen, ohne dass ein Test einen fremden
        // Kachelserver anfasst.
        let karte = MLNMapView(frame: .zero, styleURL: Konfiguration.kartenstil)
        karte.delegate = context.coordinator
        karte.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        karte.setCenter(
            context.coordinator.koordinate(Karteneinstellungen.roessing),
            zoomLevel: Karteneinstellungen.startZoom,
            animated: false
        )
        // Neigen und Drehen bringen auf einer Dorfkarte nichts und verstellen
        // sie nur versehentlich.
        karte.allowsTilting = false
        karte.allowsRotating = false
        karte.accessibilityIdentifier = "dorfkarte"

        let tipp = UITapGestureRecognizer(
            target: context.coordinator,
            action: #selector(Koordinator.getippt(_:))
        )
        // Erst darf die Karte selbst zugreifen (Doppeltipp zum Zoomen);
        // bleibt sie ohne Treffer, kommt unser Tipp zum Zug.
        for erkenner in karte.gestureRecognizers ?? [] where erkenner is UITapGestureRecognizer {
            tipp.require(toFail: erkenner)
        }
        karte.addGestureRecognizer(tipp)
        context.coordinator.tippErkenner = tipp

        return karte
    }

    func updateUIView(_ karte: MLNMapView, context: Context) {
        let koordinator = context.coordinator
        koordinator.auswahl = auswahl
        koordinator.stilzustand = stilzustand
        koordinator.orte = orte
        koordinator.groesse = groesse
        koordinator.merkmaleSetzen(karte)
        koordinator.standortSetzen(karte, zeigen: eigenenStandortZeigen)
        koordinator.hinfahrenWennGewuenscht(karte, zaehler: hinfahren)
        koordinator.stilNeuLadenWennGewuenscht(karte, versuch: stilVersuch)
        koordinator.startausschnittWennNoetig(karte)
    }

    /// SwiftUI nimmt die Ansicht aus dem Baum, sobald jemand wegblättert. Der
    /// Kartenkern lädt aber weiter (Stil, Sprites, Kacheln) und ruft danach
    /// seinen Delegaten — auf Android kostete genau das einen Absturz
    /// (Issue #24). Hier wird deshalb abgemeldet, bevor die Ansicht
    /// verschwindet.
    static func dismantleUIView(_ karte: MLNMapView, coordinator: Koordinator) {
        coordinator.abmelden(karte)
    }

    // MARK: - Koordinator

    final class Koordinator: NSObject, MLNMapViewDelegate {
        var auswahl: (Ort) -> Void
        var stilzustand: (String?) -> Void
        var orte: [Ort] = []
        var groesse: CGSize = .zero
        var tippErkenner: UITapGestureRecognizer?

        /// Der Stil steht und Quelle samt Ebene sind angelegt.
        private var stilSteht = false
        /// Der Startausschnitt wird genau einmal gesetzt — danach gehört die
        /// Kamera der Nutzerin.
        private var startGesetzt = false
        private var vonHandBewegt = false
        private var letzterHinfahrZaehler = 0
        private var letzterStilVersuch = 0
        /// „Mein Standort" gedrückt, aber die Ortung weiß noch nicht, wo wir
        /// sind: Sobald der erste Punkt kommt, wird gefahren.
        private var hinfahrenOffen = false

        init(auswahl: @escaping (Ort) -> Void, stilzustand: @escaping (String?) -> Void) {
            self.auswahl = auswahl
            self.stilzustand = stilzustand
        }

        func koordinate(_ punkt: Kartenpunkt) -> CLLocationCoordinate2D {
            CLLocationCoordinate2D(latitude: punkt.breite, longitude: punkt.laenge)
        }

        // MARK: Stil, Quelle und Ebene

        func mapView(_ mapView: MLNMapView, didFinishLoading stil: MLNStyle) {
            quelleUndEbeneAnlegen(in: stil)
            stilSteht = true
            stilzustand(nil)
            startausschnittWennNoetig(mapView)
            vorleseElementeSetzen(mapView)
        }

        func mapViewDidFailLoadingMap(_ mapView: MLNMapView, withError fehler: Error) {
            stilSteht = false
            stilzustand("Die Karte konnte nicht geladen werden. Vielleicht ist gerade kein Netz da.")
        }

        /// Quelle und Ebene entstehen einmal je geladenem Stil. Ein zweiter
        /// Aufruf (etwa nach „Erneut versuchen") findet sie vor und lässt sie
        /// in Ruhe.
        private func quelleUndEbeneAnlegen(in stil: MLNStyle) {
            let vorhanden = stil.source(withIdentifier: MapLibreKarte.quellenkennung) as? MLNShapeSource
            let quelle: MLNShapeSource
            if let vorhanden {
                quelle = vorhanden
            } else {
                quelle = MLNShapeSource(
                    identifier: MapLibreKarte.quellenkennung,
                    shape: gestalt(),
                    options: nil
                )
                stil.addSource(quelle)
            }
            quelle.shape = gestalt()

            guard stil.layer(withIdentifier: MapLibreKarte.ebenenkennung) == nil else { return }
            let ebene = MLNCircleStyleLayer(
                identifier: MapLibreKarte.ebenenkennung,
                source: quelle
            )
            ebene.circleRadius = NSExpression(forConstantValue: Karteneinstellungen.nadelradius)
            ebene.circleColor = farbausdruck()
            ebene.circleStrokeWidth = NSExpression(forConstantValue: Karteneinstellungen.nadelrand)
            ebene.circleStrokeColor = NSExpression(forConstantValue: UIColor.white)
            ebene.circleOpacity = NSExpression(forConstantValue: 0.95)
            stil.addLayer(ebene)
        }

        /// Die Farbe der Nadel kommt aus der Eigenschaft `ampel` — dieselben
        /// Farben wie in Liste und Detailseite (`Ampel.farbe`), damit derselbe
        /// Zustand nirgends anders aussieht.
        private func farbausdruck() -> NSExpression {
            var stopps: [NSExpression: NSExpression] = [:]
            for ampel in [Ampel.yellow, .red] {
                stopps[NSExpression(forConstantValue: ampel.rawValue)] =
                    NSExpression(forConstantValue: UIColor(ampel.farbe))
            }
            return NSExpression(
                forMLNMatchingKey: NSExpression(forKeyPath: Kartendaten.eigenschaftAmpel),
                in: stopps,
                default: NSExpression(forConstantValue: UIColor(Ampel.green.farbe))
            )
        }

        private func gestalt() -> MLNShape? {
            let daten = Kartendaten.geoJson(aus: orte)
            return try? MLNShape(data: daten, encoding: String.Encoding.utf8.rawValue)
        }

        /// Die Orte haben sich geändert: nur die Quelle neu füttern. Die Ebene
        /// bleibt, die Kamera bleibt, der Stil wird nicht neu geladen.
        func merkmaleSetzen(_ karte: MLNMapView) {
            guard stilSteht,
                  let quelle = karte.style?.source(withIdentifier: MapLibreKarte.quellenkennung)
                  as? MLNShapeSource
            else { return }
            quelle.shape = gestalt()
            vorleseElementeSetzen(karte)
        }

        func stilNeuLadenWennGewuenscht(_ karte: MLNMapView, versuch: Int) {
            guard versuch != letzterStilVersuch else { return }
            letzterStilVersuch = versuch
            guard versuch > 0 else { return }
            stilSteht = false
            karte.styleURL = Konfiguration.kartenstil
        }

        // MARK: Kamera

        func startausschnittWennNoetig(_ karte: MLNMapView) {
            guard !startGesetzt, !vonHandBewegt else { return }
            let breite = Double(groesse.width > 0 ? groesse.width : karte.bounds.width)
            let hoehe = Double(groesse.height > 0 ? groesse.height : karte.bounds.height)
            guard breite > 0, hoehe > 0 else { return }
            let start = Kartendaten.start(fuer: orte, breiteInPunkten: breite, hoeheInPunkten: hoehe)
            karte.setCenter(koordinate(start.mitte), zoomLevel: start.zoom, animated: false)
            // Erst wenn wirklich Orte da waren, ist der Ausschnitt endgültig —
            // sonst rückt die Karte nach, sobald sie geladen sind.
            if !Kartendaten.merkmale(aus: orte).isEmpty { startGesetzt = true }
        }

        func mapView(
            _ mapView: MLNMapView,
            regionWillChangeWith grund: MLNCameraChangeReason,
            animated: Bool
        ) {
            let gesten: MLNCameraChangeReason = [
                .gesturePan, .gesturePinch, .gestureRotate,
                .gestureZoomIn, .gestureZoomOut, .gestureOneFingerZoom,
            ]
            if !grund.intersection(gesten).isEmpty { vonHandBewegt = true }
        }

        func mapView(_ mapView: MLNMapView, regionDidChangeAnimated animated: Bool) {
            vorleseElementeSetzen(mapView)
        }

        // MARK: Eigener Standort

        func standortSetzen(_ karte: MLNMapView, zeigen: Bool) {
            guard karte.showsUserLocation != zeigen else { return }
            karte.showsUserLocation = zeigen
        }

        func hinfahrenWennGewuenscht(_ karte: MLNMapView, zaehler: Int) {
            guard zaehler != letzterHinfahrZaehler else { return }
            letzterHinfahrZaehler = zaehler
            guard zaehler > 0 else { return }
            hinfahrenOffen = true
            hinfahrenVersuchen(karte)
        }

        /// Der Druck auf „Mein Standort" ist die bewusste Ansage und schlägt
        /// den automatischen Startausschnitt.
        private func hinfahrenVersuchen(_ karte: MLNMapView) {
            guard hinfahrenOffen,
                  let ort = karte.userLocation?.location?.coordinate,
                  CLLocationCoordinate2DIsValid(ort)
            else { return }
            hinfahrenOffen = false
            vonHandBewegt = true
            startGesetzt = true
            karte.setCenter(ort, zoomLevel: Karteneinstellungen.standortZoom, animated: true)
        }

        func mapView(_ mapView: MLNMapView, didUpdate benutzerort: MLNUserLocation?) {
            hinfahrenVersuchen(mapView)
        }

        // MARK: Tipp auf eine Nadel

        @objc func getippt(_ geste: UITapGestureRecognizer) {
            guard let karte = geste.view as? MLNMapView else { return }
            let punkt = geste.location(in: karte)
            // Großzügige Trefferfläche: gezielt wird mit dem Finger.
            let kante = Karteneinstellungen.trefferbreite
            let flaeche = CGRect(
                x: punkt.x - kante / 2, y: punkt.y - kante / 2,
                width: kante, height: kante
            )
            let treffer = karte.visibleFeatures(
                in: flaeche,
                styleLayerIdentifiers: [MapLibreKarte.ebenenkennung]
            )
            guard let ort = naechsterOrt(zu: punkt, unter: treffer, karte: karte) else { return }
            auswahl(ort)
        }

        /// Liegen mehrere Nadeln in der Trefferfläche, gewinnt die, die dem
        /// Finger am nächsten war.
        private func naechsterOrt(
            zu punkt: CGPoint,
            unter merkmale: [MLNFeature],
            karte: MLNMapView
        ) -> Ort? {
            var bester: (ort: Ort, abstand: CGFloat)?
            for merkmal in merkmale {
                guard let kennung = (merkmal.attributes[Kartendaten.eigenschaftId] as? NSNumber)?.int64Value,
                      let ort = orte.first(where: { $0.id == kennung })
                else { continue }
                let auf = karte.convert(merkmal.coordinate, toPointTo: karte)
                let abstand = hypot(auf.x - punkt.x, auf.y - punkt.y)
                if bester == nil || abstand < bester!.abstand { bester = (ort, abstand) }
            }
            return bester?.ort
        }

        // MARK: VoiceOver

        /// Farbe allein ist keine Information: Jede Nadel bekommt ein
        /// Vorleseelement mit Name und Zustand, das sich auch aktivieren lässt.
        /// Die Elemente sitzen dort, wo die Nadel gerade zu sehen ist, und
        /// werden nach jeder Kamerabewegung neu gesetzt.
        func vorleseElementeSetzen(_ karte: MLNMapView) {
            let merkmale = Kartendaten.merkmale(aus: orte)
            var elemente: [Any] = []

            let ganzeKarte = UIAccessibilityElement(accessibilityContainer: karte)
            ganzeKarte.accessibilityLabel = "Dorfkarte"
            ganzeKarte.accessibilityValue = merkmale.isEmpty
                ? "Keine Orte"
                : "\(merkmale.count) Orte"
            ganzeKarte.accessibilityFrameInContainerSpace = karte.bounds
            elemente.append(ganzeKarte)

            let kante = Karteneinstellungen.trefferbreite
            for merkmal in merkmale {
                let auf = karte.convert(koordinate(merkmal.punkt), toPointTo: karte)
                guard karte.bounds.insetBy(dx: -kante, dy: -kante).contains(auf) else { continue }
                let element = Nadelelement(accessibilityContainer: karte)
                element.accessibilityLabel = "\(merkmal.name), \(merkmal.ampel.vorlesetext)"
                element.accessibilityTraits = .button
                element.accessibilityFrameInContainerSpace = CGRect(
                    x: auf.x - kante / 2, y: auf.y - kante / 2, width: kante, height: kante
                )
                let kennung = merkmal.id
                element.antippen = { [weak self] in
                    guard let ort = self?.orte.first(where: { $0.id == kennung }) else { return }
                    self?.auswahl(ort)
                }
                elemente.append(element)
            }

            // Der Quellenhinweis der Karte bleibt erreichbar — er ist bei
            // OpenStreetMap-Daten Pflicht und keine Zierde.
            elemente.append(karte.attributionButton)
            karte.isAccessibilityElement = false
            karte.accessibilityElements = elemente
        }

        // MARK: Abmelden

        func abmelden(_ karte: MLNMapView) {
            karte.delegate = nil
            if let tippErkenner { karte.removeGestureRecognizer(tippErkenner) }
            tippErkenner = nil
            karte.showsUserLocation = false
            karte.accessibilityElements = nil
            stilSteht = false
        }
    }
}

/// Ein Vorleseelement, das sich aktivieren lässt („Doppeltippen zum Öffnen").
final class Nadelelement: UIAccessibilityElement {
    var antippen: () -> Void = {}

    override func accessibilityActivate() -> Bool {
        antippen()
        return true
    }
}
