import Foundation

/// Träger (Vereine und Gruppen) und die Beitritte zu ihnen.
///
/// Was hier ankommt, ist bereits entschieden: Die Liste enthält nur, was
/// diese Person sehen darf, und jeder Eintrag sagt mit, ob sie Mitglied ist,
/// verwalten darf, beitreten kann — und wenn nicht, warum nicht, in einem
/// fertigen deutschen Satz. Nichts davon wird hier nachgerechnet. Die Regeln
/// stehen in `model.Zugriff` im Backend; eine zweite Prüfung in der App gäbe
/// beim nächsten Sonderfall eine andere Antwort als der Server.
///
/// Eine eigene Datei nur der Übersicht wegen: Es sind **Methoden auf
/// `DorfApi`**, die denselben Transport benutzen wie alle anderen Endpunkte
/// (`hole`, `schicke`). Die DTOs stehen bei den übrigen in `Modelle.swift`.
nonisolated extension DorfApi {
    // MARK: Verzeichnis

    /// Alle Träger, die diese Person sehen darf. Eine geschlossene Gruppe
    /// steht nur für ihre Mitglieder darin — das entscheidet der Server.
    func traegerListe() async throws -> [Traeger] {
        let antwort: TraegerListeAntwort = try await hole("api/v1/traeger")
        return antwort.traeger
    }

    /// Ein einzelner Träger. 404 heißt hier auch „gibt es für dich nicht“ —
    /// so verrät kein Durchprobieren von Kennungen eine geschlossene Gruppe.
    func traeger(id: Int64) async throws -> Traeger {
        let antwort: TraegerAntwort = try await hole("api/v1/traeger/\(id)")
        return antwort.traeger
    }

    // MARK: Mitmachen sagen

    /// „Ich will mitmachen.“ Ein 409 heißt: geht gerade nicht, und der Text
    /// des Servers sagt warum (schon dabei, geschlossene Gruppe, kein
    /// Zitadel-Projekt). Er wird im Wortlaut gezeigt.
    func beitrittBeantragen(traeger id: Int64, begruendung: String) async throws -> Beitritt {
        try await schicke("POST", "api/v1/traeger/\(id)/beitritt",
                          rumpf: BeitrittEingabe(begruendung: begruendung))
    }

    /// Meine eigenen Anträge, quer über alle Träger.
    func meineBeitritte() async throws -> [Beitritt] {
        let antwort: BeitritteAntwort = try await hole("api/v1/me/beitritte")
        return antwort.beitritte
    }

    // MARK: Entscheiden (Vorstand)

    /// Die Anträge eines Trägers. Nur für die, die ihn verwalten — sonst
    /// antwortet der Server mit 403.
    func beitritte(traeger id: Int64, status: String = "") async throws -> [Beitritt] {
        var abfrage: [String: String] = [:]
        if !status.isEmpty { abfrage["status"] = status }
        let antwort: BeitritteAntwort = try await hole("api/v1/traeger/\(id)/beitritte",
                                                       abfrage: abfrage)
        return antwort.beitritte
    }

    /// Freigeben oder ablehnen.
    ///
    /// Eine Freigabe kann scheitern, ohne dass am Bedienen etwas falsch war:
    /// Das Backend trägt die Mitgliedschaft zuerst in die Rössing-ID ein und
    /// hakt erst danach ab. Klappt das nicht, kommt 502 oder 503, der Antrag
    /// bleibt offen — und der Satz des Servers gehört auf den Schirm.
    func beitrittEntscheiden(id: Int64, status: String,
                             notiz: String = "") async throws -> Beitritt {
        try await schicke("POST", "api/v1/beitritte/\(id)",
                          rumpf: BeitrittEntscheidung(status: status, notiz: notiz))
    }

    /// Jemanden ohne vorherigen Antrag aufnehmen — bei einer geschlossenen
    /// Gruppe der einzige Weg hinein, und auch sonst der Alltag: Zugesagt
    /// wird im Dorf öfter am Gartenzaun als in der App.
    func mitgliedAufnehmen(traeger id: Int64, userSub: String,
                           notiz: String = "") async throws -> Beitritt {
        try await schicke("POST", "api/v1/traeger/\(id)/mitglieder",
                          rumpf: MitgliedEingabe(userSub: userSub, notiz: notiz))
    }
}
