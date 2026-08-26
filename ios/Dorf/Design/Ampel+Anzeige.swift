import SwiftUI

/// Farben und Texte der Ampel — an einer Stelle, damit Karte, Liste und
/// Detailseite nie Verschiedenes über denselben Zustand sagen.
///
/// Die Farbwerte sind dieselben wie in der Android-App, damit ein Screenshot
/// aus der einen App neben der anderen nicht wie eine andere Anwendung
/// aussieht.
extension Ampel {
    var farbe: Color {
        switch self {
        case .green: return Color(red: 0.18, green: 0.56, blue: 0.24)
        case .yellow: return Color(red: 0.92, green: 0.65, blue: 0.08)
        case .red: return Color(red: 0.78, green: 0.20, blue: 0.16)
        }
    }

    /// Der Text auf der Kachel — abhängig von der Aufgabenart, weil „Dringend
    /// gießen!" mehr sagt als „Dringend!".
    func text(fuer art: String? = nil) -> String {
        switch (self, art) {
        case (.green, _): return "Alles gut"
        case (.yellow, "giessen"): return "Bitte gießen"
        case (.yellow, "jaeten"): return "Bitte jäten"
        case (.yellow, _): return "Bitte erledigen"
        case (.red, "giessen"): return "Dringend gießen!"
        case (.red, "jaeten"): return "Dringend jäten!"
        case (.red, _): return "Dringend!"
        }
    }

    /// Für VoiceOver: die Farbe allein ist keine Information.
    var vorlesetext: String {
        switch self {
        case .green: return "Status grün"
        case .yellow: return "Status gelb"
        case .red: return "Status rot"
        }
    }

    /// Sortierung „was am dringendsten ist zuerst".
    var dringlichkeit: Int {
        switch self {
        case .red: return 0
        case .yellow: return 1
        case .green: return 2
        }
    }
}

/// Ein kleiner farbiger Punkt mit Beschriftung.
struct Ampelpunkt: View {
    let ampel: Ampel
    var art: String?
    var mitText = true

    var body: some View {
        HStack(spacing: 6) {
            Circle()
                .fill(ampel.farbe)
                .frame(width: 12, height: 12)
                .overlay(Circle().strokeBorder(.white.opacity(0.7), lineWidth: 1))
            if mitText {
                Text(ampel.text(fuer: art))
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(ampel.vorlesetext). \(ampel.text(fuer: art))")
    }
}

/// Liter mit deutschem Dezimalkomma und ohne unnötige Nullen („7,5" statt
/// „7.500000").
enum Zahl {
    static func liter(_ wert: Double) -> String {
        let f = NumberFormatter()
        f.locale = Locale(identifier: "de_DE")
        f.numberStyle = .decimal
        f.maximumFractionDigits = 1
        f.minimumFractionDigits = 0
        return f.string(from: NSNumber(value: wert)) ?? "\(wert)"
    }
}

/// Datum und Uhrzeit in Ortszeit des Dorfes.
enum Zeitpunkt {
    static let dorfZone = TimeZone(identifier: "Europe/Berlin") ?? .current

    static func kurz(_ datum: Date) -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "de_DE")
        f.timeZone = dorfZone
        f.dateFormat = "dd.MM.yyyy"
        return f.string(from: datum)
    }

    static func mitUhrzeit(_ datum: Date) -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "de_DE")
        f.timeZone = dorfZone
        f.dateFormat = "dd.MM.yyyy, HH:mm"
        return f.string(from: datum) + " Uhr"
    }

    /// „vor 3 Tagen" — für die Historie angenehmer als ein Datum.
    static func relativ(_ datum: Date, jetzt: Date = Date()) -> String {
        let f = RelativeDateTimeFormatter()
        f.locale = Locale(identifier: "de_DE")
        f.unitsStyle = .full
        return f.localizedString(for: datum, relativeTo: jetzt)
    }
}
