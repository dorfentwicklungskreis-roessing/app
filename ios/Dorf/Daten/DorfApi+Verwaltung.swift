import Foundation

/// Die Endpunkte der Verwaltung — Orte und Aufgaben pflegen, Hitzefaktor
/// stellen.
///
/// **Das Backend entscheidet.** Geprüft wird dort (`adminOnly`,
/// `PlaceInput.Validate`, `TaskInput.Validate`); hier wird nur geschickt und
/// der Ablehnungsgrund im Wortlaut weitergereicht. Ein 403 ohne die Rolle
/// `admin` ist keine Panne, sondern die Regel.
///
/// Eine eigene Datei nur der Übersicht wegen: Es sind **Methoden auf
/// `DorfApi`** mit demselben Transport wie alle anderen Endpunkte. Die
/// Eingabe-Datensätze (`OrtEingabe`, `AufgabeEingabe`, `Einstellungen`)
/// stehen bei den übrigen DTOs in `Modelle.swift`.
nonisolated extension DorfApi {
    // MARK: Orte

    func ortAnlegen(_ eingabe: OrtEingabe) async throws -> Ort {
        try await schicke("POST", "api/v1/places", rumpf: eingabe)
    }

    func ortAendern(id: Int64, _ eingabe: OrtEingabe) async throws -> Ort {
        try await schicke("PUT", "api/v1/places/\(id)", rumpf: eingabe)
    }

    func ortLoeschen(id: Int64) async throws {
        try await schickeOhneAntwort("DELETE", "api/v1/places/\(id)")
    }

    // MARK: Aufgaben

    func aufgabeAnlegen(ort: Int64, _ eingabe: AufgabeEingabe) async throws -> Aufgabe {
        try await schicke("POST", "api/v1/places/\(ort)/tasks", rumpf: eingabe)
    }

    func aufgabeAendern(id: Int64, _ eingabe: AufgabeEingabe) async throws -> Aufgabe {
        try await schicke("PUT", "api/v1/tasks/\(id)", rumpf: eingabe)
    }

    func aufgabeLoeschen(id: Int64) async throws {
        try await schickeOhneAntwort("DELETE", "api/v1/tasks/\(id)")
    }

    // MARK: Hitzefaktor

    func einstellungen() async throws -> Einstellungen {
        try await hole("api/v1/settings")
    }

    /// Das Backend antwortet mit dem neuen Stand — der wird angezeigt,
    /// nicht der Wert, den wir gerade geschickt haben.
    func hitzefaktorSetzen(_ faktor: Double) async throws -> Einstellungen {
        try await schicke("PUT", "api/v1/settings",
                          rumpf: HitzefaktorEingabe(wateringFactor: faktor))
    }
}
