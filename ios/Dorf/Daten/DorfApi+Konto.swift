import Foundation

/// Kontolöschung — der Weg, den Apples Richtlinie 5.1.1 (v) verlangt und den
/// die DSGVO (Art. 17) ohnehin voraussetzt.
nonisolated extension DorfApi {
    /// Löscht das eigene Konto im Dorf-Backend.
    ///
    /// Der Aufruf ist bewusst **unschädlich wiederholbar**: Kommt die Antwort
    /// im wackeligen Netz nicht an, darf die App es noch einmal versuchen,
    /// ohne dass etwas Zweites passiert.
    ///
    /// Als einziger Endpunkt der App deutet er die Antwort selbst, statt
    /// `hole`/`schicke` zu benutzen: Ein **leerer Rumpf ist hier kein
    /// Fehler**. Gelöscht ist gelöscht, auch wenn eine spätere
    /// Backend-Fassung nichts mehr dazu zu sagen hätte. Geschickt wird
    /// trotzdem über denselben Weg wie alles andere — dieselbe Sitzung,
    /// dasselbe frische Token, dieselbe Fehlerübersetzung.
    func kontoLoeschen() async throws -> Kontoloeschung {
        let daten = try await rohAusfuehren(anfrage("DELETE", "api/v1/me"))
        return (try? JSONDecoder().decode(Kontoloeschung.self, from: daten))
            ?? Kontoloeschung(geloescht: true)
    }
}
