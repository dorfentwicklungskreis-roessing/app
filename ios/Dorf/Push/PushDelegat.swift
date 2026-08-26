import UIKit
import UserNotifications

/// Der Draht zwischen System und App.
///
/// SwiftUI hat für Push-Nachrichten keinen eigenen Weg: Die Gerätekennung von
/// Apple kommt ausschließlich über `UIApplicationDelegate`, und der Tipp auf
/// eine Meldung über `UNUserNotificationCenterDelegate`. Beides ist hier
/// gebündelt, damit `DorfApp.swift` mit einer einzigen Zeile auskommt:
///
/// ```swift
/// @UIApplicationDelegateAdaptor(PushDelegat.self) private var pushDelegat
/// ```
///
/// Hier wird bewusst **nichts entschieden**: Der Delegat reicht weiter, was
/// das System meldet. Die Regeln stehen in `Benachrichtigungen`.
final class PushDelegat: NSObject, UIApplicationDelegate, UNUserNotificationCenterDelegate {
    private var benachrichtigungen: Benachrichtigungen { .gemeinsam }

    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions optionen: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        // Der Delegat muss stehen, bevor der Start durch ist — sonst geht
        // eine Meldung verloren, die die App überhaupt erst geöffnet hat.
        UNUserNotificationCenter.current().delegate = self
        benachrichtigungen.kategorienAnlegen()
        return true
    }

    /// Apple hat die Kennung dieses Geräts geliefert — **rohe Binärdaten**.
    /// Was daraus wird, steht in `Geraetekennung.hex`.
    func application(
        _ application: UIApplication,
        didRegisterForRemoteNotificationsWithDeviceToken roheKennung: Data
    ) {
        Task { await benachrichtigungen.kennungErhalten(roheKennung) }
    }

    func application(
        _ application: UIApplication,
        didFailToRegisterForRemoteNotificationsWithError fehler: Error
    ) {
        benachrichtigungen.kennungFehlgeschlagen(fehler)
    }

    /// Eine Meldung trifft ein, während die App im Vordergrund läuft. Ohne
    /// diese Antwort zeigt iOS gar nichts an — wer gerade die Ortsliste
    /// ansieht, bekäme die Anfrage sonst nicht mit.
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        [.banner, .list, .sound]
    }

    /// Jemand hat auf die Meldung getippt.
    ///
    /// Die Nutzlast wird noch hier ausgelesen: `UNNotificationResponse` darf
    /// die Isolationsgrenze nicht überqueren, `PushZiel` schon — es ist
    /// `Sendable` und trägt genau die Zeichenketten, die das Backend
    /// mitgeschickt hat.
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse
    ) async {
        let ziel = PushZiel.ausDaten(response.notification.request.content.userInfo)
        guard let ziel else { return }
        await MainActor.run {
            Benachrichtigungen.gemeinsam.beiTipp?(ziel)
        }
    }
}
