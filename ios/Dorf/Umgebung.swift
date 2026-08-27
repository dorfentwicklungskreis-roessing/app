import Combine
import SwiftUI

/// Alles, was die Bereiche der App brauchen, an einer Stelle — die
/// Handverdrahtung entspricht dem `AppContainer` der Android-App. Bewusst kein
/// DI-Framework: Die App hat eine Handvoll Abhängigkeiten, und ein Framework
/// müsste über Jahre mitgepflegt werden.
///
/// Zugriff aus einer View:
/// ```swift
/// @EnvironmentObject private var umgebung: AppUmgebung
/// ```
final class AppUmgebung: ObservableObject {
    let anmeldung: Anmeldung
    let api: DorfApi

    /// Die Orte des Dorfes — **ein** Modell für alle, die sie brauchen.
    ///
    /// Die Startseite zeigt daraus nur eine Zahl („3 Orte warten auf dich")
    /// und den Hitzehinweis, der Bereich „Mithelfen" die ganze Liste. Zwei
    /// Modelle hießen zwei Abrufe und — schlimmer — zwei Stände: Die Kachel
    /// zählte dann etwas anderes, als die Liste dahinter zeigt.
    let orte: OrteModell

    /// Wer gerade angemeldet ist. Wird nach der Anmeldung einmal geladen und
    /// danach von den Bereichen mitbenutzt — `isAdmin` entscheidet, ob der
    /// Bereich „Verwaltung" überhaupt auftaucht.
    @Published private(set) var ich: Ich?
    @Published private(set) var ichFehler: String?

    /// Die Änderungen der beiden eigenen Modelle weiterreichen.
    ///
    /// `ObservableObject` beobachtet — anders als das Observation-Framework —
    /// **nicht** durch verschachtelte Objekte hindurch: Eine Ansicht, die
    /// `umgebung.orte.giessfaktor` liest, erführe von einer neuen Ortsliste
    /// sonst nichts. Deshalb sagt die Umgebung selbst Bescheid, sobald eines
    /// ihrer Modelle sich meldet. Gröber als vorher (die ganze Ansicht wird
    /// neu gezeichnet statt nur das gelesene Feld), aber inhaltlich gleich —
    /// und niemand muss daran denken.
    private var weiterleitungen: [AnyCancellable] = []

    init(anmeldung: Anmeldung = Anmeldung()) {
        let api = DorfApi(tokenGeber: { [anmeldung] in
            await anmeldung.frischesToken()
        })
        self.anmeldung = anmeldung
        self.api = api
        self.orte = OrteModell(api: api)
        // Die Benachrichtigungen brauchen denselben Zugang: Sie melden die
        // Gerätekennung an und beim Abmelden wieder ab. Gefragt wird damit
        // noch niemand — das passiert erst beim Eintragen als Helfer:in.
        Benachrichtigungen.gemeinsam.verdrahten(api: api)
        kinderWeiterreichen()
    }

    /// Für Vorschauen und Tests: eine Umgebung, die nichts abruft.
    init(anmeldung: Anmeldung, api: DorfApi, ich: Ich?, orte: OrteModell? = nil) {
        self.anmeldung = anmeldung
        self.api = api
        self.orte = orte ?? OrteModell(api: api)
        self.ich = ich
        kinderWeiterreichen()
    }

    private func kinderWeiterreichen() {
        weiterleitungen = [
            anmeldung.objectWillChange.sink { [weak self] in self?.objectWillChange.send() },
            orte.objectWillChange.sink { [weak self] in self?.objectWillChange.send() },
        ]
    }

    var binAdmin: Bool { ich?.isAdmin ?? false }
    var meinSub: String? { ich?.sub }

    func ichLaden() async {
        guard case .angemeldet = anmeldung.sitzung else { return }
        do {
            ich = try await api.ich()
            ichFehler = nil
        } catch let fehler as DorfFehler {
            // Kein Grund, die App anzuhalten: Ohne Profil zeigt die Startseite
            // eben „Moin!" statt „Moin, Anna!".
            ichFehler = fehler.klartext
        } catch {
            ichFehler = "Unerwarteter Fehler."
        }
    }

    /// Was die Startseite meldet, wenn gerade nichts geht.
    ///
    /// Eine App, die stumm nichts lädt, ist so schlecht wie eine, die
    /// abmeldet: In beiden Fällen weiß niemand, woran es liegt. Der Satz
    /// kommt aus `DorfFehler` und redet deshalb von der Verbindung — nicht
    /// von der Anmeldung, denn die gilt weiter.
    var stoerungshinweis: String? { ichFehler ?? orte.hinweis }

    /// Noch einmal versuchen — was der Hinweis auf der Startseite anbietet.
    func erneutVersuchen() async {
        await ichLaden()
        await orte.laden()
    }

    func profilUebernehmen(_ profil: Profil) {
        guard var stand = ich else { return }
        stand.profile = profil
        ich = stand
    }

    /// Der Name, mit dem die Startseite grüßt: Nickname vor Anzeigename vor
    /// dem Namen aus der Rössing-ID.
    var anrede: String? {
        let kandidaten = [ich?.profile?.nickname, ich?.profile?.displayName, ich?.name]
        return kandidaten.compactMap { $0 }.first { !$0.trimmingCharacters(in: .whitespaces).isEmpty }
    }
}
