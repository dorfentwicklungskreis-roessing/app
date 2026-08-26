import Foundation

/// An- und Abmeldung der Gerätekennung beim Dorfserver.
///
/// Die beiden Endpunkte liegen bewusst **hier** und nicht in
/// `Daten/DorfApi.swift`: Der Push-Bereich bringt seine eigene Ecke der API
/// mit, und die gemeinsame Datei bleibt unangetastet. Weil die Innereien von
/// `DorfApi` (Basisadresse, Sitzung, Tokengeber) privat sind, bekommt dieser
/// Weg sie als Parameter — mit genau denselben Vorgaben, die `AppUmgebung`
/// dem `DorfApi` mitgibt. Übersteuerbar bleiben sie damit auch, was Tests
/// brauchen: Kein Test darf ins Netz.
nonisolated enum Geraeteanmeldung {
    /// Der Endpunkt für beides — angemeldet wird mit POST, abgemeldet mit
    /// DELETE, jeweils mit demselben Rumpf.
    static let pfad = "api/v1/me/devices"

    /// Das Feld, an dem das Backend den Versandweg festmacht: „ios" spricht
    /// direkt mit Apple (APNs), alles andere geht über Firebase
    /// (`backend/internal/push/weiche.go`).
    static let plattform = "ios"

    /// Der Rumpf: `{"token": "<hex>", "platform": "ios"}`.
    ///
    /// Die Feldnamen sind englisch, weil sie der JSON-Vertrag des Backends
    /// sind (`api.DeviceInput`) — wie alle DTOs der App.
    struct Eingabe: Encodable, Sendable {
        let token: String
        let platform: String

        init(kennung: String) {
            self.token = kennung
            self.platform = Geraeteanmeldung.plattform
        }
    }

    /// Baut die Anfrage. Als eigener Schritt, damit sich ohne Netz prüfen
    /// lässt, was tatsächlich hinausginge — Pfad, Methode, Rumpf und
    /// Autorisierung.
    static func anfrage(
        _ methode: String,
        kennung: String,
        basis: URL = Konfiguration.apiBasis,
        zugangstoken: String?
    ) throws -> URLRequest {
        var anfrage = URLRequest(url: basis.appending(path: pfad))
        anfrage.httpMethod = methode
        anfrage.setValue("application/json", forHTTPHeaderField: "Content-Type")
        anfrage.setValue("application/json", forHTTPHeaderField: "Accept")
        if let zugangstoken {
            anfrage.setValue("Bearer \(zugangstoken)", forHTTPHeaderField: "Authorization")
        }
        anfrage.httpBody = try JSONEncoder().encode(Eingabe(kennung: kennung))
        return anfrage
    }
}

nonisolated extension DorfApi {
    /// Meldet die Gerätekennung an: `POST /api/v1/me/devices`.
    ///
    /// Auch wenn sie dort schon liegt — das Backend legt dieselbe Kennung
    /// nicht doppelt an (eindeutiger Index), und der Zeitstempel bleibt so
    /// frisch. Apple tauscht Kennungen von Zeit zu Zeit aus; wer sich lange
    /// nicht gemeldet hat, ist vermutlich nicht mehr da.
    func geraetAnmelden(
        kennung: String,
        basis: URL = Konfiguration.apiBasis,
        sitzung: URLSession = .dorfSitzung,
        zugangstoken: @Sendable () async -> String?
    ) async throws {
        try await Geraeteanmeldung.schicken(
            "POST", kennung: kennung, basis: basis, sitzung: sitzung, zugangstoken: zugangstoken)
    }

    /// Meldet die Gerätekennung ab: `DELETE /api/v1/me/devices`, derselbe
    /// Rumpf.
    ///
    /// Muss **vor** dem Abmelden aus der App passieren: Ohne gültiges Token
    /// bekäme das Backend nie mit, dass an diesem Gerät niemand mehr ist —
    /// und schickte weiter Anfragen dorthin.
    func geraetAbmelden(
        kennung: String,
        basis: URL = Konfiguration.apiBasis,
        sitzung: URLSession = .dorfSitzung,
        zugangstoken: @Sendable () async -> String?
    ) async throws {
        try await Geraeteanmeldung.schicken(
            "DELETE", kennung: kennung, basis: basis, sitzung: sitzung, zugangstoken: zugangstoken)
    }
}

nonisolated extension Geraeteanmeldung {
    /// Der gemeinsame Versand. Die Antwort interessiert nicht weiter: Die
    /// Kennung kommt bewusst in keiner Antwort vor (sie ist ein Schlüssel zum
    /// Gerät), und mehr als „hat geklappt" gibt es hier nicht zu lernen.
    fileprivate static func schicken(
        _ methode: String,
        kennung: String,
        basis: URL,
        sitzung: URLSession,
        zugangstoken: @Sendable () async -> String?
    ) async throws {
        guard Geraetekennung.istBrauchbar(kennung) else {
            throw DorfFehler.abgelehnt(grund: "Die Gerätekennung hat nicht die Form, die APNs erwartet.")
        }
        let versand = try anfrage(methode, kennung: kennung, basis: basis,
                                  zugangstoken: await zugangstoken())
        let antwort: URLResponse
        do {
            (_, antwort) = try await sitzung.data(for: versand)
        } catch {
            throw DorfFehler.netz(error.localizedDescription)
        }
        guard let http = antwort as? HTTPURLResponse else {
            throw DorfFehler.netz("Unerwartete Antwort")
        }
        guard (200 ..< 300).contains(http.statusCode) else {
            if http.statusCode == 401 { throw DorfFehler.nichtAngemeldet }
            throw DorfFehler.serverfehler(status: http.statusCode)
        }
    }
}
