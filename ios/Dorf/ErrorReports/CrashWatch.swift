import Foundation

/// Notices that the app stopped last time — and offers to say so.
///
/// Why this exists at all: the crash that started this whole thing was only
/// noticed because somebody happened to tap a message and the log was pulled
/// out of TestFlight by hand. Apple's own crash reports go to Apple, not to
/// the Dorfentwicklungskreis, and only if the person agreed to that
/// separately. So the app has to notice for itself.
///
/// Two catchers, because one is not enough:
///
///  1. `NSSetUncaughtExceptionHandler` catches Objective-C exceptions and
///     brings name, reason and the call stack along. It does not catch Swift
///     runtime errors (`fatalError`, an index out of bounds) — those do not
///     raise an exception, they kill the process.
///  2. A **foreground mark**: while the app is in the foreground, a flag
///     stands in the defaults; going to the background clears it. Is the flag
///     still there at the next start, the last run ended while the app was
///     visible — and that is a crash, an out-of-memory kill or a watchdog.
///     Force-quitting from the app switcher does not produce a false alarm:
///     to reach the switcher the app has to go to the background first, which
///     clears the mark.
///
/// Deliberately **no signal handlers**. Writing from a signal handler is not
/// async-signal-safe; a crash reporter that crashes while reporting is worse
/// than none, and there is no library here to do it properly.
nonisolated enum CrashWatch {
    fileprivate static let markeSchluessel = "de.roessing.app.fehlerberichte.imVordergrund"
    fileprivate static let ausnahmeSchluessel = "de.roessing.app.fehlerberichte.letzteAusnahme"
    fileprivate static let zeitSchluessel = "de.roessing.app.fehlerberichte.letzteSichtung"

    /// Installs the exception handler. Once per app start.
    static func handlerEinhaengen() {
        NSSetUncaughtExceptionHandler { ausnahme in
            // Läuft, während die App schon stirbt: nur schreiben, nichts
            // anderes. Der Text ist absichtlich kurz — ein Bericht mit
            // dreihundert Zeilen Aufrufliste hilft niemandem mehr als die
            // ersten zwanzig.
            let stapel = ausnahme.callStackSymbols.prefix(20).joined(separator: "\n")
            let text = [ausnahme.name.rawValue, ausnahme.reason ?? "", stapel]
                .filter { !$0.isEmpty }
                .joined(separator: "\n")
            let d = UserDefaults.standard
            d.set(String(text.prefix(4000)), forKey: CrashWatch.ausnahmeSchluessel)
            d.set(Date().timeIntervalSince1970, forKey: CrashWatch.zeitSchluessel)
        }
    }

    /// The app is visible. From now on an end is an unwanted end.
    static func imVordergrund(_ d: UserDefaults = .standard) {
        d.set(true, forKey: markeSchluessel)
        d.set(Date().timeIntervalSince1970, forKey: zeitSchluessel)
    }

    /// The app went to the background in an orderly manner.
    static func imHintergrund(_ d: UserDefaults = .standard) {
        d.set(false, forKey: markeSchluessel)
    }

    /// Reads what the last run left behind — and clears it, so the same
    /// crash is not offered twice.
    ///
    /// Must run **before** `imVordergrund`, otherwise the mark of this start
    /// is read as the mark of the last one.
    static func offenerAbsturz(_ d: UserDefaults = .standard, jetzt: Date = Date()) -> ErrorIncident? {
        let ausnahme = (d.string(forKey: ausnahmeSchluessel) ?? "")
            .trimmingCharacters(in: .whitespacesAndNewlines)
        let abgebrochen = d.bool(forKey: markeSchluessel)
        d.removeObject(forKey: ausnahmeSchluessel)
        d.set(false, forKey: markeSchluessel)
        guard abgebrochen || !ausnahme.isEmpty else { return nil }

        let wann = d.object(forKey: zeitSchluessel) as? Double
        let zeitpunkt = wann.map { Date(timeIntervalSince1970: $0) } ?? jetzt
        return ErrorIncident(
            kind: .crash,
            message: "Die App hat sich beim letzten Mal unerwartet beendet.",
            detail: ausnahme.isEmpty
                ? "Kein Ausnahmetext — die App war zuletzt im Vordergrund und wurde nicht "
                    + "ordentlich beendet."
                : ausnahme,
            area: "Absturz",
            // Nicht später als jetzt: Eine falsch gestellte Uhr soll den
            // Bericht nicht in die Zukunft schieben.
            occurredAt: min(zeitpunkt, jetzt)
        )
    }
}
