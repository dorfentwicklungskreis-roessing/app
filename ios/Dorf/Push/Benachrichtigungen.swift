import Foundation
import Observation
import UIKit
import UserNotifications

/// Darf die App überhaupt benachrichtigen?
///
/// Die Entscheidung für sich genommen — ohne System, damit prüfbar. Auf
/// Android steht dasselbe in `Benachrichtigungserlaubnis.wirksam(sdk:…)`.
nonisolated enum Benachrichtigungserlaubnis {
    /// `provisional` zählt mit: Das ist die stille Zustellung ohne Nachfrage,
    /// die Apple erlaubt. Sie liefert echte Meldungen aus, also braucht das
    /// Backend auch eine Kennung dafür.
    static func wirksam(_ stand: UNAuthorizationStatus) -> Bool {
        switch stand {
        case .authorized, .provisional, .ephemeral: true
        case .notDetermined, .denied: false
        @unknown default: false
        }
    }
}

/// Benachrichtigungen der Dorf-App: Erlaubnis, Gerätekennung, Anzeige.
///
/// **Push ist die Abkürzung, nicht der Weg.** Jede Anfrage steht auch in der
/// Abrufliste des Backends und erscheint beim nächsten Öffnen der App. Wer
/// die Erlaubnis verweigert, verpasst deshalb nichts — es dauert nur länger.
/// Genau darum wird die Erlaubnis auch **nicht beim Start** erfragt: Der
/// Systemdialog kommt genau einmal im Leben einer Installation, und wer ihn
/// sieht, ohne zu wissen wofür, sagt Nein. Gefragt wird deshalb erst, wenn
/// sich jemand als Helfer:in eingetragen hat — dann ist die Frage
/// selbsterklärend. So hält es auch die Android-App.
///
/// Der Weg dorthin ist bewusst ohne Firebase gebaut: Die App meldet ihre
/// **rohe APNs-Kennung** beim Dorfserver an, der Server spricht direkt mit
/// Apple (`backend/internal/push/apns.go`). Kein Google im Spiel, keine
/// schwere Fremdbibliothek — nur `UserNotifications` aus dem System.
@Observable
final class Benachrichtigungen {
    /// Die Instanz, die der Systemdelegat bedient. Eine App hat genau ein
    /// Mitteilungszentrum, und `@UIApplicationDelegateAdaptor` baut sich
    /// seinen Delegaten selbst — deshalb ein fester Ort statt Durchreichen.
    static let gemeinsam = Benachrichtigungen()

    /// Was das System zuletzt gesagt hat. Für die Oberfläche („Mitteilungen
    /// sind aus — in den Einstellungen einschaltbar").
    private(set) var stand: UNAuthorizationStatus = .notDetermined

    /// Ein Tipp auf eine Meldung. Wer die Navigation kennt, hängt sich hier
    /// ein; dieser Bereich weiß nichts über Bildschirme.
    var beiTipp: ((PushZiel) -> Void)?

    @ObservationIgnored private var api: DorfApi?
    @ObservationIgnored private let ablage: UserDefaults

    /// Für Tests und Vorschauen: eigene Ablage statt der der App.
    init(ablage: UserDefaults = .standard) {
        self.ablage = ablage
    }

    // MARK: Verdrahtung

    /// Verbindet die Benachrichtigungen mit dem Backend-Zugang.
    ///
    /// Passiert einmal beim Start in `AppUmgebung.init`; ohne das wird keine
    /// Kennung angemeldet, und die App läuft schlicht ohne Push. Das Token
    /// bringt `DorfApi` selbst mit — es holt vor jeder Anfrage ein frisches.
    func verdrahten(api: DorfApi) {
        self.api = api
    }

    // MARK: Erlaubnis

    /// Fragt die Person um Erlaubnis — **erst** an der Stelle, an der klar
    /// ist, wofür.
    ///
    /// Der Systemdialog erscheint nur beim ersten Mal. Danach liefert das
    /// System die einmal getroffene Entscheidung zurück, ohne zu fragen; wer
    /// sie ändern will, tut das in den iOS-Einstellungen.
    ///
    /// Ergebnis `true` heißt: Die Kennung ist unterwegs zum Server (Apple
    /// liefert sie asynchron an den Delegaten, siehe `kennungErhalten`).
    @discardableResult
    func erlaubnisErfragen() async -> Bool {
        let zentrale = UNUserNotificationCenter.current()
        kategorienAnlegen()
        let erteilt: Bool
        do {
            erteilt = try await zentrale.requestAuthorization(options: [.alert, .sound, .badge])
        } catch {
            // Kein Grund, irgendetwas anzuhalten: Ohne Erlaubnis holt die App
            // ihre Benachrichtigungen weiter selbst ab.
            standAktualisieren(.denied)
            return false
        }
        await standNachlesen()
        if erteilt {
            beiApfelAnmelden()
        }
        return erteilt
    }

    /// Der Abgleich beim Start und bei jeder Rückkehr in den Vordergrund.
    ///
    /// Die Kennung folgt der Erlaubnis: Wer sie in den iOS-Einstellungen
    /// wieder entzieht, dessen Kennung wird beim nächsten Mal weggeräumt.
    /// Ohne Erlaubnis wird auch keine angefordert — sonst entstünde eine
    /// Kennung bei jemandem, der ausdrücklich Nein gesagt hat.
    func abgleichen() async {
        await standNachlesen()
        if Benachrichtigungserlaubnis.wirksam(stand) {
            beiApfelAnmelden()
        } else {
            await abmelden()
        }
    }

    private func standNachlesen() async {
        let einstellungen = await UNUserNotificationCenter.current().notificationSettings()
        standAktualisieren(einstellungen.authorizationStatus)
    }

    private func standAktualisieren(_ neu: UNAuthorizationStatus) {
        stand = neu
    }

    /// Die beiden Kanäle. Auf iOS sind das `UNNotificationCategory`; das
    /// Backend schreibt den passenden Bezeichner in das Feld `category`.
    ///
    /// Angelegt werden sie beim Start — das ist harmlos und fragt niemanden
    /// etwas. Ohne sie stünde jede Meldung ohne Kanal da.
    func kategorienAnlegen() {
        let kategorien = Set(PushKanal.allCases.map { kanal in
            UNNotificationCategory(
                identifier: kanal.rawValue,
                actions: [],
                intentIdentifiers: [],
                options: []
            )
        })
        UNUserNotificationCenter.current().setNotificationCategories(kategorien)
    }

    // MARK: Gerätekennung

    /// Bittet Apple um die Kennung dieses Geräts. Die Antwort kommt asynchron
    /// im Delegaten an (`kennungErhalten`) — im Simulator meist gar nicht,
    /// das ist normal.
    private func beiApfelAnmelden() {
        UIApplication.shared.registerForRemoteNotifications()
    }

    /// Apple hat die Kennung geliefert. Ab hier ist sie ein personenbezogenes
    /// Datum: Sie steht für genau dieses Gerät, und wer sie hat, kann ihm
    /// Meldungen schicken. Sie geht deshalb nur an den Dorfserver, nirgendwo
    /// sonst hin.
    func kennungErhalten(_ roheKennung: Data) async {
        let kennung = Geraetekennung.hex(roheKennung)
        guard Geraetekennung.istBrauchbar(kennung) else { return }
        guard let api else { return }
        do {
            try await api.geraetAnmelden(kennung: kennung)
            merken(kennung)
        } catch {
            // Kein Netz oder abgelaufene Anmeldung: Der nächste Start
            // versucht es erneut. Gemerkt wird die Kennung erst, wenn der
            // Server sie wirklich hat.
        }
    }

    /// Apple konnte keine Kennung liefern (kein Netz, kein Profil, im
    /// Simulator der Normalfall). Ohne Push bleibt es bei der Abrufliste.
    func kennungFehlgeschlagen(_ fehler: Error) {
        // Bewusst still: Es gibt nichts, was die Person hier tun könnte, und
        // die App funktioniert vollständig weiter.
        _ = fehler
    }

    /// Meldet die Kennung beim Server ab und stellt das Gerät still.
    ///
    /// Muss **vor** dem Abmelden aus der App laufen: Danach gibt es kein
    /// gültiges Token mehr, und die Kennung bliebe für immer im Server
    /// stehen. Scheitert der Aufruf (kein Netz), bleibt die Merkung stehen —
    /// der nächste Versuch räumt auf.
    func abmelden() async {
        UIApplication.shared.unregisterForRemoteNotifications()
        guard let kennung = gemerkteKennung, let api else { return }
        do {
            try await api.geraetAbmelden(kennung: kennung)
            merken(nil)
        } catch {
            // stehen lassen, beim nächsten Mal erneut versuchen
        }
    }

    // MARK: Anzeige

    /// Jemand hat auf eine Meldung getippt. Der Weg zur Aufgabe ist ein
    /// Rückruf: Dieser Bereich kennt die Navigation nicht.
    func getippt(_ nutzlast: [AnyHashable: Any]) {
        guard let ziel = PushZiel.ausDaten(nutzlast) else { return }
        beiTipp?(ziel)
    }

    // MARK: Merkung

    /// Welche Kennung beim Server liegt, steht in den Voreinstellungen.
    ///
    /// Nötig, weil Apple die Kennung nur auf Anfrage und nur asynchron
    /// herausrückt — beim Abmelden wäre sie sonst nicht mehr greifbar, und
    /// eine Kennung, die niemand mehr löschen kann, ist genau das, was hier
    /// nicht passieren soll.
    private static let schluessel = "push.geraetekennung"

    private var gemerkteKennung: String? {
        ablage.string(forKey: Self.schluessel)
    }

    private func merken(_ kennung: String?) {
        if let kennung {
            ablage.set(kennung, forKey: Self.schluessel)
        } else {
            ablage.removeObject(forKey: Self.schluessel)
        }
    }
}
