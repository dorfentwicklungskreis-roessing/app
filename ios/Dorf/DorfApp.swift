import SwiftUI

@main
struct DorfApp: App {
    /// Der Draht zum System. Die Gerätekennung von Apple kommt
    /// ausschließlich über `UIApplicationDelegate`, und der Tipp auf eine
    /// Meldung über `UNUserNotificationCenterDelegate` — SwiftUI hat für
    /// beides keinen eigenen Weg. `PushDelegat` bündelt es; hier steht nur
    /// die Zeile, die ihn anhängt.
    @UIApplicationDelegateAdaptor(PushDelegat.self) private var pushDelegat

    @Environment(\.scenePhase) private var szene
    @StateObject private var umgebung = AppUmgebung()

    var body: some Scene {
        WindowGroup {
            WurzelView()
                .environmentObject(umgebung)
                .tint(Color(red: 0.18, green: 0.56, blue: 0.24))
                // Bei jeder Rückkehr in den Vordergrund: Die Kennung folgt
                // der Erlaubnis. Wer die Mitteilungen in den
                // iOS-Einstellungen wieder abdreht, dessen Kennung wird beim
                // nächsten Mal weggeräumt. Gefragt wird hier niemand —
                // das passiert erst beim Eintragen als Helfer:in.
                // `initial:` gibt es erst ab iOS 17 — der erste Durchlauf
                // steht deshalb als eigene Aufgabe daneben.
                .task { await Benachrichtigungen.gemeinsam.abgleichen() }
                .onChange(of: szene) { neu in
                    guard neu == .active else { return }
                    Task { await Benachrichtigungen.gemeinsam.abgleichen() }
                }
        }
    }
}
