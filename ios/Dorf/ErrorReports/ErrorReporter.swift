import Combine
import Foundation
import UIKit

/// Collects what went wrong and sends it — if the person wants that.
///
/// Two rules run through this whole area:
///
/// 1. **Nothing goes out on its own.** Not a single report leaves the phone
///    without somebody tapping a button. A crash reporter that sends by
///    itself would be the one place in this app where something is collected
///    silently, and there is a stance in this project against exactly that
///    (`store/data-safety.md`, `backend/SICHERHEIT.md`).
/// 2. **Nothing is invented.** The sentence in the report is the sentence the
///    person read on screen, and that in turn is the backend's wording
///    wherever the backend said something (`DorfFehler.klartext`).
///
/// One shared instance, like `Benachrichtigungen.gemeinsam`: every corner of
/// the app has to be able to report, and the way to the backend is wired in
/// once from `AppUmgebung`.
final class ErrorReporter: ObservableObject {
    static let gemeinsam = ErrorReporter()

    /// What went wrong and is waiting to be shown. `nil` = nothing pending.
    @Published private(set) var vorfall: ErrorIncident?
    @Published private(set) var sendet = false
    /// After sending: „Danke, der Bericht ist angekommen."
    @Published private(set) var gesendet = false
    /// Sending itself failed — said out loud, not swallowed.
    @Published private(set) var sendefehler: String?

    private var api: DorfApi?
    private let angaben: Geraeteangaben
    private let uhr: @Sendable () -> Date

    init(angaben: Geraeteangaben? = nil, uhr: @escaping @Sendable () -> Date = { Date() }) {
        self.angaben = angaben ?? Geraeteangaben.aktuell(
            systemVersion: UIDevice.current.systemVersion
        )
        self.uhr = uhr
    }

    /// The way to the backend. Wired in once by `AppUmgebung` — the same
    /// pattern the notifications use.
    func verdrahten(api: DorfApi) { self.api = api }

    // MARK: Reporting in

    /// A failed request. Called from `DorfApi` for every single one, so no
    /// area has to remember to report.
    ///
    /// Refusals by rule are not reported — `ErrorIncident.aus` sorts them out.
    func beobachte(_ fehler: DorfFehler, methode: String, pfad: String) {
        // Ein Bericht über einen gescheiterten Bericht wäre eine Schleife.
        guard !pfad.contains(Self.eigenerPfad) else { return }
        guard let vorfall = ErrorIncident.aus(fehler, pfad: pfad, methode: methode) else { return }
        melde(vorfall)
    }

    /// Puts an incident in front of the person.
    ///
    /// A report that is on its way is not overwritten — otherwise a second
    /// failure would swallow the „Danke" of the first, and the person would
    /// not know whether anything arrived.
    func melde(_ vorfall: ErrorIncident) {
        guard !sendet else { return }
        self.vorfall = vorfall
        gesendet = false
        sendefehler = nil
    }

    /// The person has read it and does not want to report it. That is a
    /// legitimate answer, and it costs nothing.
    func schliessen() {
        vorfall = nil
        gesendet = false
        sendefehler = nil
        sendet = false
    }

    // MARK: Sending

    /// Sends the report. `kommentar` is voluntary and usually empty — one tap
    /// helps just as much as a written sentence.
    func absenden(kommentar: String = "") async {
        guard let vorfall, let api, !sendet else { return }
        sendet = true
        sendefehler = nil
        let eingabe = eingabeFuer(vorfall, kommentar: kommentar)
        do {
            try await api.sendErrorReport(eingabe)
            gesendet = true
            sendefehler = nil
        } catch let fehler as DorfFehler {
            // Auch hier gilt: der Wortlaut des Backends, nicht ein eigener.
            sendefehler = fehler.klartext
        } catch {
            sendefehler = Self.nichtGeklappt
        }
        sendet = false
    }

    /// Exactly what goes out — the same values that build the request.
    /// Used by the sheet, so „das wird geschickt" is not a promise but the
    /// thing itself.
    func eingabeFuer(_ vorfall: ErrorIncident, kommentar: String = "") -> ErrorReportInput {
        ErrorReportInput(
            kind: vorfall.kind.rawValue,
            message: vorfall.message,
            detail: vorfall.detail,
            comment: kommentar.trimmingCharacters(in: .whitespacesAndNewlines),
            area: vorfall.area,
            platform: "ios",
            appVersion: angaben.appVersion,
            osVersion: angaben.osVersion,
            deviceModel: angaben.deviceModel,
            occurredAt: Self.rfc3339.string(from: vorfall.occurredAt)
        )
    }

    /// The content of a report in plain German, line by line — for the list
    /// under „Das wird geschickt".
    func inhaltsliste(_ eingabe: ErrorReportInput) -> [(String, String)] {
        var zeilen: [(String, String)] = [
            ("Was passiert ist", eingabe.message),
        ]
        if !eingabe.area.isEmpty { zeilen.append(("Bereich", eingabe.area)) }
        if !eingabe.detail.isEmpty { zeilen.append(("Technisch", eingabe.detail)) }
        if !eingabe.comment.isEmpty { zeilen.append(("Dein Text", eingabe.comment)) }
        zeilen.append(("App", eingabe.appVersion))
        zeilen.append(("Gerät", [eingabe.deviceModel, eingabe.osVersion]
                .filter { !$0.isEmpty }.joined(separator: ", ")))
        zeilen.append(("Wann", Self.anzeige.string(from: vorfall?.occurredAt ?? uhr())))
        return zeilen
    }

    // MARK: Kleinkram

    /// The path of the report entrance itself. Whatever fails there must not
    /// produce another report.
    static let eigenerPfad = "error-reports"

    static let nichtGeklappt = "Das Abschicken hat nicht geklappt. Besteht eine Verbindung?"

    nonisolated(unsafe) private static let rfc3339: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime]
        return f
    }()

    nonisolated(unsafe) private static let anzeige: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "de_DE")
        f.timeZone = TimeZone(identifier: "Europe/Berlin")
        f.dateFormat = "dd.MM.yyyy, HH:mm"
        return f
    }()
}
