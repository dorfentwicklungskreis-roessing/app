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

    /// Der Melder für Fehlerberichte. Einer für die ganze App — jede Stelle
    /// muss melden können, und angezeigt wird es an genau einer.
    @ObservedObject private var melder = ErrorReporter.gemeinsam

    init() {
        // Muss vor allem anderen stehen: Was jetzt noch schiefgeht, soll
        // schon gefangen werden.
        CrashWatch.handlerEinhaengen()
        // Was der letzte Lauf hinterlassen hat, kommt beim Start auf den
        // Schirm — angezeigt, nicht verschickt. Abschicken tut die Person.
        if let absturz = CrashWatch.offenerAbsturz() {
            ErrorReporter.gemeinsam.melde(absturz)
        }
    }

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
                // Solange die App sichtbar ist, steht eine Marke. Endet der
                // Lauf, ohne dass sie geräumt wurde, war es kein ordentliches
                // Ende — siehe `CrashWatch`. `onChange` meldet den ersten
                // Zustand nicht, deshalb die Aufgabe daneben.
                .task { CrashWatch.imVordergrund() }
                .fehlerbanner(melder)
                .onChange(of: szene) { neu in
                    switch neu {
                    case .active:
                        CrashWatch.imVordergrund()
                        Task { await Benachrichtigungen.gemeinsam.abgleichen() }
                    default:
                        // Auch `.inactive`: Wer die App aus dem Umschalter
                        // wegwischt, geht dort vorbei — sonst zählte jedes
                        // absichtliche Beenden als Absturz.
                        CrashWatch.imHintergrund()
                    }
                }
        }
    }
}
