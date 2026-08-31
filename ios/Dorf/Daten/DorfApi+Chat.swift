import Foundation

/// Der Chat des Dorfservers.
///
/// Der Schlüssel zu Anthropic liegt im Backend und geht nie in eine App —
/// eine ausgelieferte IPA ist ein offenes Buch. Die App fragt deshalb wie
/// überall nur den Dorfserver; von dort geht genau ein Weg nach draußen.
///
/// Bewusst NICHT hier: der Verleih. Die Mietplattform ist ein eigener Dienst
/// unter eigener Adresse mit eigener Inferenz — dieser Chat ist der des
/// Dorfservers und kennt Orte, Aufgaben, Erledigungen und Träger.
nonisolated extension DorfApi {
    /// Ob der Chat eingerichtet ist. Antwortet auch dann verständlich, wenn
    /// kein Schlüssel hinterlegt ist — der Bereich soll erklären können,
    /// warum er gerade nichts tut, statt wie ein Fehler auszusehen.
    func chatstand() async throws -> Chatstand {
        try await hole("api/v1/chat")
    }

    /// Stellt eine Frage und schickt den bisherigen Verlauf mit.
    ///
    /// `geduldig: true`: Die Antwort entsteht nicht im Dorfserver, sondern
    /// über Claude und mehrere Werkzeugrunden. Das dauert länger als eine
    /// Liste — siehe `URLSession.dorfGeduldigeSitzung`.
    func chatFragen(_ frage: String, verlauf: [Gespraechszug]) async throws -> ChatAntwort {
        try await schicke("POST", "api/v1/chat",
                          rumpf: ChatEingabe(frage: frage, verlauf: verlauf),
                          geduldig: true)
    }
}
