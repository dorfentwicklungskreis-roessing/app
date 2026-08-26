import Foundation

/// Aus den Rohdaten der Website werden Termine, wie die Oberfläche sie
/// braucht. Bewusst reine Aufbereitung — kein Netz, kein SwiftUI. So läuft
/// sie im normalen Unit-Test, und die Fallstricke sind prüfbar statt bloß
/// behauptet.
///
/// Drei Fallstricke stecken hier drin:
///  - **Zeitzone.** Die Zeitpunkte tragen einen Offset (`+01:00` im Winter,
///    `+02:00` im Sommer). Sie werden als Zeitpunkt gelesen und in Ortszeit
///    angezeigt — nie naiv als Zeichenkette abgeschnitten.
///  - **Ganztägig.** Dann steht dort nur ein Datum. Ein solcher Termin hat
///    keine Uhrzeit, und es wird auch keine erfunden.
///  - **Vorbei erst am Tagesende.** Wer um 19 Uhr anfängt, verschwindet
///    nicht um 19:01 aus der Liste.

/// Eine Stelle auf der Karte. Termine mit Koordinaten sind für die Dorfkarte
/// vorbereitet; gepflegt werden die Koordinaten auf der Website.
struct Koordinate: Hashable, Sendable {
    var lat: Double
    var lon: Double
}

/// Ein Termin, wie ihn die Liste zeigt.
struct Termin: Identifiable, Hashable, Sendable {
    let id: String
    let name: String
    let beschreibung: String
    /// Beginn als Zeitpunkt; ganztägig ist das Mitternacht in Ortszeit.
    let beginn: Date
    /// Ab hier ist der Termin vorbei: Mitternacht nach seinem letzten Tag.
    let vorbeiAb: Date
    let ganztaegig: Bool
    /// „Mo, 17.08.2026"
    let datumText: String
    /// „18:00 Uhr" — oder `nil` bei ganztägigen Terminen.
    let zeitText: String?
    /// Für VoiceOver ausformuliert: „Montag, 17. August 2026, ganztägig".
    let vorlesetext: String
    /// Wohin der Tipp führt: zur externen Primärquelle, falls es eine gibt,
    /// sonst auf die Seite des Dorfes.
    let url: String
    /// true = die Seite gehört jemand anderem; wir zeigen den Inhalt nicht
    /// doppelt.
    let extern: Bool
    let ortName: String?
    let ortAdresse: String?
    let koordinate: Koordinate?
    let veranstalter: String?

    /// Die Adresse zum Antippen — eine unbrauchbare Zeichenkette ergibt
    /// keinen Knopf statt eines Knopfes ins Leere.
    var adresse: URL? { URL(string: url) }

    func istVorbei(_ jetzt: Date) -> Bool { jetzt >= vorbeiAb }
}

// MARK: - Aufbereitung

/// Alles, was aus einer Datumsangabe der Website ein Datum macht.
enum Terminzeit {
    /// Ortszeit des Dorfes.
    static var dorfZone: TimeZone { Zeitpunkt.dorfZone }

    /// Kurze Wochentage — fest verdrahtet, damit Gerät und Test dasselbe
    /// zeigen, egal welche Sprache eingestellt ist.
    static let wochentage = ["Mo", "Di", "Mi", "Do", "Fr", "Sa", "So"]

    static func kalender(_ zone: TimeZone) -> Calendar {
        var k = Calendar(identifier: .gregorian)
        k.timeZone = zone
        k.locale = Locale(identifier: "de_DE")
        return k
    }

    /// Liest einen Zeitpunkt: entweder eine Ortszeit mit Offset oder ein
    /// reines Datum (ganztägig, dann Mitternacht in Ortszeit). Was sich nicht
    /// lesen lässt, ergibt `nil` — ein einzelner kaputter Eintrag darf nicht
    /// die ganze Liste kosten.
    static func lesen(_ text: String, zone: TimeZone) -> Date? {
        let roh = text.trimmingCharacters(in: .whitespacesAndNewlines)
        if roh.isEmpty { return nil }
        if let mitOffset = RFC3339.datum(roh) { return mitOffset }
        let f = ISO8601DateFormatter()
        f.timeZone = zone
        f.formatOptions = [.withFullDate]
        return f.date(from: roh)
    }

    /// „Mo, 17.08.2026"
    static func datumText(_ datum: Date, zone: TimeZone) -> String {
        let t = kalender(zone).dateComponents([.year, .month, .day, .weekday], from: datum)
        // `weekday` zählt ab Sonntag = 1; die Liste beginnt bei Montag.
        let tag = wochentage[(((t.weekday ?? 1) - 2) + 7) % 7]
        return tag + ", " + String(format: "%02d.%02d.%04d", t.day ?? 0, t.month ?? 0, t.year ?? 0)
    }

    /// „18:00 Uhr"
    static func zeitText(_ datum: Date, zone: TimeZone) -> String {
        let t = kalender(zone).dateComponents([.hour, .minute], from: datum)
        return String(format: "%02d:%02d Uhr", t.hour ?? 0, t.minute ?? 0)
    }

    /// Datum und Uhrzeit für VoiceOver ausformuliert — „Mo, 17.08." wäre
    /// vorgelesen kein Datum, sondern eine Buchstabenfolge.
    static func vorlesetext(_ datum: Date, ganztaegig: Bool, zone: TimeZone) -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "de_DE")
        f.timeZone = zone
        f.dateFormat = "EEEE, d. MMMM yyyy"
        let tag = f.string(from: datum)
        if ganztaegig { return "\(tag), ganztägig" }
        let t = kalender(zone).dateComponents([.hour, .minute], from: datum)
        let stunde = t.hour ?? 0
        let minute = t.minute ?? 0
        return minute == 0 ? "\(tag), \(stunde) Uhr" : "\(tag), \(stunde) Uhr \(minute)"
    }
}

extension VeranstaltungDto {
    /// Macht aus einer Veranstaltung der Website einen Termin — oder `nil`,
    /// wenn die Datumsangabe unlesbar ist.
    func alsTermin(zone: TimeZone = Terminzeit.dorfZone) -> Termin? {
        guard let beginn = Terminzeit.lesen(start, zone: zone) else { return nil }

        // Ein Termin ist erst am Ende seines letzten Tages vorbei: Wer um
        // 19 Uhr anfängt, verschwindet nicht um 19:01 aus der Liste. Genauso
        // hält es die Website.
        let letzterTag = end.flatMap { Terminzeit.lesen($0, zone: zone) } ?? beginn
        let kalender = Terminzeit.kalender(zone)
        let tagesbeginn = kalender.startOfDay(for: letzterTag)
        let vorbeiAb = kalender.date(byAdding: .day, value: 1, to: tagesbeginn)
            ?? tagesbeginn.addingTimeInterval(24 * 60 * 60)

        let ort = location.flatMap { $0.name.trimmed.isEmpty ? nil : $0 }

        return Termin(
            id: id,
            name: name,
            beschreibung: description,
            beginn: beginn,
            vorbeiAb: vorbeiAb,
            ganztaegig: allDay,
            datumText: Terminzeit.datumText(beginn, zone: zone),
            zeitText: allDay ? nil : Terminzeit.zeitText(beginn, zone: zone),
            vorlesetext: Terminzeit.vorlesetext(beginn, ganztaegig: allDay, zone: zone),
            url: url,
            extern: external,
            ortName: ort?.name,
            ortAdresse: ort.flatMap { $0.address.trimmed.isEmpty ? nil : $0.address },
            koordinate: ort.flatMap { o in
                guard let lat = o.lat, let lon = o.lon else { return nil }
                return Koordinate(lat: lat, lon: lon)
            },
            veranstalter: organizer.flatMap { $0.name.trimmed.isEmpty ? nil : $0.name }
        )
    }
}

extension Collection where Element == VeranstaltungDto {
    /// Die Liste, wie sie angezeigt wird: kommende Termine zuerst, vergangene
    /// heraus, nach Beginn sortiert. Gefiltert wird hier noch einmal selbst —
    /// die Datei der Website entsteht beim Bauen und altert zwischen zwei
    /// Bauläufen.
    func alsTermine(jetzt: Date, zone: TimeZone = Terminzeit.dorfZone) -> [Termin] {
        compactMap { $0.alsTermin(zone: zone) }
            .filter { !$0.istVorbei(jetzt) }
            .sorted { $0.beginn < $1.beginn }
    }
}

private extension String {
    /// Kurzform, weil „Name ohne Inhalt" hier dreimal vorkommt.
    var trimmed: String { trimmingCharacters(in: .whitespacesAndNewlines) }
}
