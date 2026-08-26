import Foundation

/// Die Gerätekennung von Apple, in der Form, die das Backend erwartet.
///
/// **Hier scheitern die meisten iOS-Push-Anbindungen.** Apple liefert die
/// Kennung als rohe Binärdaten (`Data`) an
/// `application(_:didRegisterForRemoteNotificationsWithDeviceToken:)`. Wer sie
/// naheliegenderweise mit `"\(daten)"` oder `daten.description` in eine
/// Zeichenkette verwandelt, bekommt so etwas:
///
///     <8a5f1c2d 3e4b5a69 78889 9aa>
///
/// — mit spitzen Klammern und Leerzeichen. Das Backend nimmt es klaglos
/// entgegen, Apple weist es als `BadDeviceToken` ab, und niemandem fällt auf,
/// warum keine Meldung ankommt. Richtig ist eine reine Hex-Zeichenkette, Byte
/// für Byte, klein geschrieben, ohne Trennzeichen.
///
/// Bewusst als eigener, gerätefreier Typ: So lässt sich genau diese
/// Umwandlung prüfen, ohne dass ein Gerät oder ein Simulator im Spiel sein
/// muss (im Simulator kommt ohnehin keine echte Kennung an).
nonisolated enum Geraetekennung {
    /// Wandelt die rohen Kennungsdaten von Apple in Hex.
    static func hex(_ daten: Data) -> String {
        let ziffern = Array("0123456789abcdef".utf8)
        var aus: [UInt8] = []
        aus.reserveCapacity(daten.count * 2)
        for byte in daten {
            aus.append(ziffern[Int(byte >> 4)])
            aus.append(ziffern[Int(byte & 0x0F)])
        }
        return String(decoding: aus, as: UTF8.self)
    }

    /// Sieht die Zeichenkette aus wie eine Kennung, die APNs annehmen kann?
    ///
    /// Dieselbe Prüfung macht das Backend noch einmal
    /// (`backend/internal/push/apns.go`, `istHex`) und wirft weg, was sie
    /// nicht besteht. Hier vorne verhindert sie, dass wir Unfug überhaupt
    /// erst hinschicken.
    static func istBrauchbar(_ kennung: String) -> Bool {
        guard !kennung.isEmpty, kennung.count % 2 == 0 else { return false }
        return kennung.allSatisfy(\.isHexDigit)
    }
}
