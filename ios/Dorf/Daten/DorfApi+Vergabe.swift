import Foundation

/// Die Vergabe der Pflegeaufgaben: anmelden, gefragt werden, zusagen.
///
/// Die Regeln — Reihenfolge, Staffelung, Ruhezeiten, Verfall — stehen
/// vollständig im Backend (`backend/internal/vergabe`). Hier wird nichts
/// nachgerechnet: Die App fragt, was für mich offen ist, und schickt zurück,
/// was ich angetippt habe. Ein 409 heißt nicht „Panne", sondern „jemand war
/// schneller" — der Satz dazu kommt vom Backend und wird im Wortlaut gezeigt.
///
/// Eine eigene Datei nur der Übersicht wegen: Es sind **Methoden auf
/// `DorfApi`**, die denselben Transport benutzen wie alle anderen Endpunkte
/// (`hole`, `schicke`, `schickeOhneAntwort`). Die DTOs stehen bei den
/// übrigen in `Modelle.swift`.
nonisolated extension DorfApi {
    // MARK: Anmeldung zum Mithelfen

    /// „Ich helfe hier mit." `art` = nil meldet für alle Aufgaben des Ortes an.
    func anmelden(ort: Int64, art: String? = nil) async throws {
        try await schickeOhneAntwort("POST", "api/v1/places/\(ort)/signup",
                                     rumpf: AnmeldeEingabe(taskKind: art ?? ""))
    }

    /// „Ich mag nicht mehr." Ohne Aufgabenart wird der ganze Ort abgemeldet.
    func abmelden(ort: Int64, art: String? = nil) async throws {
        var abfrage: [String: String] = [:]
        if let art, !art.isEmpty { abfrage["taskKind"] = art }
        try await schickeOhneAntwort("DELETE", "api/v1/places/\(ort)/signup", abfrage: abfrage)
    }

    // MARK: Benachrichtigungen

    func benachrichtigungen() async throws -> [Benachrichtigung] {
        let antwort: BenachrichtigungenAntwort = try await hole("api/v1/me/notifications")
        return antwort.notifications
    }

    /// Gelesen. Hinweise sind damit erledigt; Anfragen bleiben stehen, bis
    /// der Vorgang sie schließt — sonst wäre die Aufgabe aus der App
    /// verschwunden, bevor jemand zugesagt hat.
    func gelesen(benachrichtigung id: Int64) async throws {
        try await schickeOhneAntwort("POST", "api/v1/me/notifications/\(id)/ack")
    }

    // MARK: Zusagen

    /// Zusagen. **409 heißt: jemand anderes war schneller** — daraus wird
    /// `DorfFehler.schonVergeben`, und der Grund nennt Name und Frist im
    /// Wortlaut des Backends.
    func zusagen(vorgang id: Int64) async throws -> Vorgang {
        try await schicke("POST", "api/v1/assignments/\(id)/claim")
    }

    /// Zusage zurückgeben. Der Vorgang läuft danach weiter — die Nächsten
    /// werden gefragt.
    func zurueckgeben(vorgang id: Int64) async throws -> Vorgang {
        try await schicke("POST", "api/v1/assignments/\(id)/release")
    }
}
