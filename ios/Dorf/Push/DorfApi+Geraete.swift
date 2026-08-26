import Foundation

/// An- und Abmeldung der Gerätekennung beim Dorfserver.
///
/// **Methoden auf `DorfApi`** wie alle anderen Endpunkte auch: dieselbe
/// Sitzung, dasselbe frisch geholte Token, dieselbe Fehlerübersetzung. Nur
/// der Rumpf ist besonders — er ist für beide Richtungen derselbe
/// (`GeraetEingabe` in `Modelle.swift`).
nonisolated extension DorfApi {
    /// Der Endpunkt für beides — angemeldet wird mit POST, abgemeldet mit
    /// DELETE.
    static let geraetePfad = "api/v1/me/devices"

    /// Meldet die Gerätekennung an: `POST /api/v1/me/devices`.
    ///
    /// Auch wenn sie dort schon liegt — das Backend legt dieselbe Kennung
    /// nicht doppelt an (eindeutiger Index), und der Zeitstempel bleibt so
    /// frisch. Apple tauscht Kennungen von Zeit zu Zeit aus; wer sich lange
    /// nicht gemeldet hat, ist vermutlich nicht mehr da.
    func geraetAnmelden(kennung: String) async throws {
        try await geraet("POST", kennung: kennung)
    }

    /// Meldet die Gerätekennung ab: `DELETE /api/v1/me/devices`, derselbe
    /// Rumpf.
    ///
    /// Muss **vor** dem Abmelden aus der App passieren: Ohne gültiges Token
    /// bekäme das Backend nie mit, dass an diesem Gerät niemand mehr ist —
    /// und schickte weiter Anfragen dorthin.
    func geraetAbmelden(kennung: String) async throws {
        try await geraet("DELETE", kennung: kennung)
    }

    /// Der gemeinsame Versand. Die Antwort interessiert nicht weiter: Die
    /// Kennung kommt bewusst in keiner Antwort vor (sie ist ein Schlüssel zum
    /// Gerät), und mehr als „hat geklappt" gibt es hier nicht zu lernen.
    private func geraet(_ methode: String, kennung: String) async throws {
        guard Geraetekennung.istBrauchbar(kennung) else {
            throw DorfFehler.abgelehnt(
                grund: "Die Gerätekennung hat nicht die Form, die APNs erwartet.")
        }
        try await schickeOhneAntwort(methode, Self.geraetePfad,
                                     rumpf: GeraetEingabe(kennung: kennung))
    }
}
