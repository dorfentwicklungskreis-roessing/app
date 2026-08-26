import Foundation

/// Die Antwort von `DELETE /api/v1/me`.
///
/// Sie enthält bewusst zwei Sätze im Klartext, die die App **anzeigt statt
/// nachzubauen**: was mit den Erledigungen passiert und dass die Rössing-ID
/// bestehen bleibt. Was das Backend tut, sagt das Backend — sonst laufen
/// beide Fassungen über die Jahre auseinander.
nonisolated struct Kontoloeschung: Decodable, Sendable {
    var geloescht: Bool = false
    /// „Deine Meldungen bleiben anonym stehen …"
    var erledigungen: String = ""
    /// „Deine Rössing-ID bleibt bestehen …"
    var roessingId: String = ""
    var roessingIdUrl: String = ""

    enum CodingKeys: String, CodingKey { case geloescht, erledigungen, roessingId, roessingIdUrl }

    init(geloescht: Bool = false, erledigungen: String = "",
         roessingId: String = "", roessingIdUrl: String = "") {
        self.geloescht = geloescht; self.erledigungen = erledigungen
        self.roessingId = roessingId; self.roessingIdUrl = roessingIdUrl
    }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        geloescht = c.wert(.geloescht, false)
        erledigungen = c.wert(.erledigungen, "")
        roessingId = c.wert(.roessingId, "")
        roessingIdUrl = c.wert(.roessingIdUrl, "")
    }
}

/// Kontolöschung — der Weg, den Apples Richtlinie 5.1.1 (v) verlangt und den
/// die DSGVO (Art. 17) ohnehin voraussetzt.
nonisolated extension DorfApi {
    /// Löscht das eigene Konto im Dorf-Backend.
    ///
    /// Der Aufruf ist bewusst **unschädlich wiederholbar**: Kommt die Antwort
    /// im wackeligen Netz nicht an, darf die App es noch einmal versuchen,
    /// ohne dass etwas Zweites passiert.
    ///
    /// Die Anfrage wird hier selbst gebaut, statt die Innereien von `DorfApi`
    /// zu benutzen: Basis, Sitzung und Tokengeber liegen dort `private`, und
    /// `DorfApi.swift` bleibt unangetastet. Es sind dieselben Werte —
    /// `Konfiguration.apiBasis` und `URLSession.dorfSitzung` —, und beide
    /// lassen sich für einen Test übersteuern, damit kein Test ins Netz geht.
    func kontoLoeschen(token: String?,
                       basis: URL = Konfiguration.apiBasis,
                       sitzung: URLSession = .dorfSitzung) async throws -> Kontoloeschung {
        var anfrage = URLRequest(url: basis.appending(path: "api/v1/me"))
        anfrage.httpMethod = "DELETE"
        anfrage.setValue("application/json", forHTTPHeaderField: "Accept")
        if let token {
            anfrage.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

        let daten: Data
        let antwort: URLResponse
        do {
            (daten, antwort) = try await sitzung.data(for: anfrage)
        } catch {
            throw DorfFehler.netz(error.localizedDescription)
        }
        guard let http = antwort as? HTTPURLResponse else {
            throw DorfFehler.netz("Unerwartete Antwort")
        }
        guard (200 ..< 300).contains(http.statusCode) else {
            // Der Wortlaut kommt aus dem Backend — dort sitzt die Prüfung,
            // dort steht die Begründung.
            let grund = (try? JSONDecoder().decode(ApiFehlerAntwort.self, from: daten))?
                .error.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            switch http.statusCode {
            case 401: throw DorfFehler.nichtAngemeldet
            case 403: throw DorfFehler.keineBerechtigung(
                grund: grund.isEmpty ? "Es lässt sich nur das eigene Konto löschen." : grund)
            case 429: throw DorfFehler.zuVieleAnfragen
            default: throw DorfFehler.serverfehler(status: http.statusCode)
            }
        }
        // Ein leerer Rumpf ist kein Fehler: Gelöscht ist gelöscht, auch wenn
        // eine spätere Backend-Fassung nichts mehr dazu zu sagen hätte.
        return (try? JSONDecoder().decode(Kontoloeschung.self, from: daten))
            ?? Kontoloeschung(geloescht: true)
    }
}
