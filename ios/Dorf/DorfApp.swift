import SwiftUI

@main
struct DorfApp: App {
    @State private var umgebung = AppUmgebung()

    var body: some Scene {
        WindowGroup {
            WurzelView()
                .environment(umgebung)
                .tint(Color(red: 0.18, green: 0.56, blue: 0.24))
        }
    }
}
