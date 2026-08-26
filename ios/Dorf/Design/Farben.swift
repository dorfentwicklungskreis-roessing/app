import SwiftUI

extension Color {
    /// Die Trennlinienfarbe des Systems — dieselbe, die iOS zwischen
    /// Listenzeilen zeichnet, und damit auch im Dunkelmodus richtig.
    ///
    /// SwiftUI kennt sie als `ShapeStyle.separator` erst ab iOS 17. Die App
    /// läuft ab iOS 16, deshalb der Umweg über `UIColor` — es ist derselbe
    /// Farbwert, nur älter erreichbar.
    static let trennlinie = Color(uiColor: .separator)
}
