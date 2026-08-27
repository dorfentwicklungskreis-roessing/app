import AuthenticationServices
import Combine
import CryptoKit
import Foundation

/// Der Scope, mit dem Zitadel die Projektrollen ins Token legt.
///
/// Angefordert wird er mit „projects" (Plural), zurück kommt der Claim
/// `urn:zitadel:iam:org:project:roles` mit „project" (Singular) — daraus liest
/// das Backend die Rolle `admin`. **Ohne diesen Scope stellt Zitadel ein Token
/// ganz ohne Rollen aus**: Dann ist niemand Verwaltung, und alles, was die
/// Rolle verlangt, antwortet mit 403. Genau dieser Fehler steckte lange in der
/// Android-App — siehe README, Abschnitt „Identität".
let ROLLEN_SCOPE = "urn:zitadel:iam:org:projects:roles"

/// `offline_access` hält die Sitzung über den Neustart hinweg.
let ANMELDE_SCOPES = ["openid", "profile", "email", "offline_access", ROLLEN_SCOPE]

enum Sitzungszustand: Equatable, Sendable {
    case laedt
    case abgemeldet
    case angemeldet(entwicklerModus: Bool)
}

/// Wie ein Anmeldeversuch ausgegangen ist.
///
/// `abgebrochen` ist bewusst **kein** Fehler: Wer den Browser zumacht, landet
/// kommentarlos wieder auf dem Anmeldebildschirm — ohne rote Meldung.
enum Anmeldeergebnis: Equatable, Sendable {
    case erfolg
    case abgebrochen
    case fehlgeschlagen(kuerzel: String)
}

/// Was die App gerade als Zugangstoken anbieten kann.
///
/// Der Unterschied zwischen den beiden letzten Fällen ist der ganze Punkt:
/// „Der Server sagt nein" ist eine Entscheidung, „ich konnte den Server nicht
/// fragen" ein Umstand. Nur das Erste beendet eine Anmeldung. Solange beides
/// gleich hieß (`nil`), kostete jedes Funkloch die Anmeldung.
enum Tokenlage: Equatable, Sendable {
    case token(String)
    /// Niemand ist angemeldet.
    case abgemeldet
    /// Jemand ist angemeldet, aber das Token ließ sich gerade nicht erneuern.
    case nichtErreichbar
}

/// Die Endpunkte der Rössing-ID, wie sie die Discovery meldet.
struct OidcEndpunkte: Codable, Sendable {
    let authorization_endpoint: URL
    let token_endpoint: URL
    let end_session_endpoint: URL?
}

/// Die Anmeldung gegen die Rössing-ID (Zitadel): Authorization Code + PKCE im
/// System-Browser, Erneuerungstoken, Ablage im Schlüsselbund.
///
/// Bewusst ohne Fremdbibliothek: `ASWebAuthenticationSession` ist der von
/// Apple vorgesehene Weg und bringt die geteilte Browser-Sitzung mit, an der
/// ein eingebettetes WebView scheitern würde. Der Rest ist ein
/// Authorization-Code-Fluss nach Vorschrift — knapp hundert Zeilen, die
/// niemand für uns pflegen muss.
final class Anmeldung: NSObject, ObservableObject {
    @Published private(set) var sitzung: Sitzungszustand = .laedt

    /// Das zuletzt erhaltene ID-Token — nur zur Diagnose („warum bin ich
    /// nicht Verwaltung?"). Zitadel legt Rollen je nach Einstellung ins
    /// Zugangs- oder ins ID-Token; wer das untersucht, braucht beide.
    @Published private(set) var letztesIdToken: String?

    private var tokensatz: Tokensatz?
    private var entwicklerToken: String?
    private var endpunkte: OidcEndpunkte?
    private var laufendeAnmeldung: ASWebAuthenticationSession?

    /// Die gerade laufende Erneuerung, falls es eine gibt.
    ///
    /// Vor **jeder** Anfrage wird ein Token verlangt, und beim Kaltstart
    /// verlangen mehrere Abrufe es im selben Augenblick (Profil, Orte,
    /// Gerätekennung). Ohne diese Stelle schickte jeder von ihnen seine
    /// eigene Erneuerung mit demselben Erneuerungstoken los — und Zitadel
    /// gibt bei jeder Erneuerung ein neues aus und weist das alte von da an
    /// ab. Der erste Abruf gewänne, alle übrigen bekämen `invalid_grant`,
    /// und das hat bis hierher abgemeldet. Genau das war nach einer
    /// Aktualisierung zu sehen. Jetzt warten alle auf dieselbe Erneuerung.
    private var laufendeErneuerung: Task<Tokensatz, Error>?

    private let sitzungsNetz: URLSession
    private let aussteller: URL
    private let clientId: String
    private let ruecksprung: String
    private let ablage: Tokenablage

    /// Die Vorbelegungen sind die der App; ein Test reicht seine eigenen
    /// herein und fasst damit weder das Netz noch den echten Schlüsselbund an.
    init(sitzung: URLSession = .dorfSitzung,
         aussteller: URL = Konfiguration.oidcAussteller,
         clientId: String = Konfiguration.oidcClientId,
         ruecksprung: String = Konfiguration.oidcRuecksprung,
         ablage: Tokenablage = .schluesselbund) {
        self.sitzungsNetz = sitzung
        self.aussteller = aussteller
        self.clientId = clientId
        self.ruecksprung = ruecksprung
        self.ablage = ablage
        super.init()
        wiederherstellen()
    }

    // MARK: Zustand

    private func wiederherstellen() {
        if Konfiguration.entwicklerLoginErlaubt,
           let dev = UserDefaults.standard.string(forKey: "entwicklerToken") {
            entwicklerToken = dev
            sitzung = .angemeldet(entwicklerModus: true)
            return
        }
        if let satz = ablage.lesen() {
            tokensatz = satz
            letztesIdToken = satz.idToken
            // Auch ein abgelaufenes Zugangstoken zählt als angemeldet, solange
            // ein Erneuerungstoken da ist — sonst stünde man nach jedem
            // längeren Wegstecken des Telefons wieder vor dem Anmeldeknopf.
            if satz.gueltig() || satz.erneuerungstoken != nil {
                sitzung = .angemeldet(entwicklerModus: false)
                return
            }
        }
        sitzung = .abgemeldet
    }

    /// Liefert die Tokenlage: ein gültiges Zugangstoken (erneuert bei Bedarf),
    /// „niemand angemeldet" oder „gerade nicht erreichbar".
    /// Wird vor **jeder** API-Anfrage aufgerufen.
    func frischesToken() async -> Tokenlage {
        if let entwicklerToken { return .token(entwicklerToken) }
        guard let satz = tokensatz else { return .abgemeldet }
        if satz.gueltig() { return .token(satz.zugangstoken) }
        guard let erneuerungstoken = satz.erneuerungstoken else {
            // Ohne Erneuerungstoken gibt es nichts mehr zu erneuern — das ist
            // wirklich das Ende der Sitzung und kein Umstand.
            abmelden()
            return .abgemeldet
        }
        do {
            return .token(try await erneuerungAbwarten(mit: erneuerungstoken).zugangstoken)
        } catch Anmeldefehler.abgewiesen(let kuerzel) where Self.istSitzungsende(kuerzel) {
            // Die Rössing-ID hat die Sitzung beendet: Token widerrufen, Konto
            // gesperrt, Passwort geändert. Hier ist Abmelden die Wahrheit.
            abmelden()
            return .abgemeldet
        } catch {
            // Alles andere heißt nur: gerade nicht gefragt werden können.
            // Die Anmeldung bleibt stehen, der nächste Abruf versucht es neu.
            return .nichtErreichbar
        }
    }

    /// Ob eine abgewiesene Erneuerung das Ende der Sitzung bedeutet.
    ///
    /// RFC 6749 lässt den Token-Endpunkt eine abgelehnte Zuteilung mit 400 und
    /// einem Kürzel beantworten. `invalid_grant` ist das einzige, das „diese
    /// Sitzung ist vorbei" heißt — widerrufenes Erneuerungstoken, gesperrtes
    /// Konto, geändertes Passwort. `invalid_client`, `invalid_request` und
    /// Verwandtes sind Fehler in **unserer** Einrichtung; wer daraufhin
    /// abmeldet, wirft eine gültige Anmeldung wegen eines eigenen Fehlers weg,
    /// und die neue Anmeldung scheiterte an derselben Stelle wieder.
    ///
    /// Als reine Funktion, damit sie sich ohne Netz prüfen lässt.
    static func istSitzungsende(_ kuerzel: String) -> Bool {
        kuerzel == "invalid_grant"
    }

    /// Erneuert — oder hängt sich an die Erneuerung, die schon läuft.
    private func erneuerungAbwarten(mit erneuerungstoken: String) async throws -> Tokensatz {
        if let laufendeErneuerung { return try await laufendeErneuerung.value }
        // Die Aufgabe wird **vor** dem ersten `await` hinterlegt: Erst dadurch
        // findet der nächste Aufrufer sie vor, statt eine zweite loszuschicken.
        let aufgabe = Task { try await self.erneuern(mit: erneuerungstoken) }
        laufendeErneuerung = aufgabe
        defer { laufendeErneuerung = nil }
        let neu = try await aufgabe.value
        uebernehmen(neu)
        return neu
    }

    private func uebernehmen(_ satz: Tokensatz) {
        tokensatz = satz
        letztesIdToken = satz.idToken ?? letztesIdToken
        ablage.sichern(satz)
        sitzung = .angemeldet(entwicklerModus: false)
    }

    func abmelden() {
        tokensatz = nil
        entwicklerToken = nil
        letztesIdToken = nil
        ablage.loeschen()
        UserDefaults.standard.removeObject(forKey: "entwicklerToken")
        sitzung = .abgemeldet
    }

    /// Entwickler-Login ohne Zitadel — nur im Debug-Build mit `DEV_AUTH=1`
    /// und nur gegen ein Backend im `AUTH_MODE=insecure-dev`.
    func entwicklerAnmeldung(alsAdmin: Bool) {
        guard Konfiguration.entwicklerLoginErlaubt else { return }
        let token = "e2e-user:E2E Tester:\(alsAdmin ? "admin" : "")"
        entwicklerToken = token
        UserDefaults.standard.set(token, forKey: "entwicklerToken")
        sitzung = .angemeldet(entwicklerModus: true)
    }

    // MARK: Anmeldefluss

    /// Das Schema, unter dem der Browser zurück in die App springt
    /// (`de.roessing.app` aus `de.roessing.app:/oauth2redirect`).
    private var ruecksprungSchema: String {
        String(ruecksprung.prefix(while: { $0 != ":" }))
    }

    func anmelden() async -> Anmeldeergebnis {
        do {
            let ziele = try await endpunkteHolen()
            let pruefer = Self.zufallstext()
            let herausforderung = Self.s256(pruefer)
            let zustand = Self.zufallstext()

            var bau = URLComponents(url: ziele.authorization_endpoint, resolvingAgainstBaseURL: false)!
            bau.queryItems = [
                .init(name: "client_id", value: clientId),
                .init(name: "response_type", value: "code"),
                .init(name: "redirect_uri", value: ruecksprung),
                .init(name: "scope", value: ANMELDE_SCOPES.joined(separator: " ")),
                .init(name: "state", value: zustand),
                .init(name: "code_challenge", value: herausforderung),
                .init(name: "code_challenge_method", value: "S256"),
            ]
            // Bewusst KEIN prompt-Parameter: Zitadel kennt nur none/login/
            // select_account/create. Ein unbekannter Wert ist laut Spec zwar zu
            // ignorieren, aber unnötiges Risiko.

            let rueckweg = try await browserAnmeldung(bau.url!)
            guard let teile = URLComponents(url: rueckweg, resolvingAgainstBaseURL: false) else {
                return .fehlgeschlagen(kuerzel: "ruecksprung_unlesbar")
            }
            let felder = teile.queryItems ?? []
            if let fehler = felder.first(where: { $0.name == "error" })?.value {
                return .fehlgeschlagen(kuerzel: fehler)
            }
            guard felder.first(where: { $0.name == "state" })?.value == zustand else {
                // Ein fremder state heißt: Diese Antwort gehört nicht zu
                // unserer Anfrage. Sie wird nicht eingetauscht.
                return .fehlgeschlagen(kuerzel: "state_stimmt_nicht")
            }
            guard let code = felder.first(where: { $0.name == "code" })?.value else {
                return .fehlgeschlagen(kuerzel: "kein_code")
            }

            let satz = try await tauschen(code: code, pruefer: pruefer, ziel: ziele.token_endpoint)
            uebernehmen(satz)
            return .erfolg
        } catch let fehler as ASWebAuthenticationSessionError
            where fehler.code == .canceledLogin {
            return .abgebrochen
        } catch let fehler as Anmeldefehler {
            return fehler == .abgebrochen ? .abgebrochen : .fehlgeschlagen(kuerzel: fehler.kuerzel)
        } catch {
            return .fehlgeschlagen(kuerzel: "unerwartet")
        }
    }

    /// Warum ein Anmelde- oder Erneuerungsschritt nicht durchkam.
    ///
    /// Die Trennung ist die Sache selbst: `abgewiesen` heißt, die Rössing-ID
    /// hat geantwortet und Nein gesagt. `nichtErreichbar` heißt, sie hat gar
    /// nicht geantwortet — kein Netz, Zeitüberschreitung, 5xx, unlesbare
    /// Antwort. Nur das Erste darf eine Anmeldung kosten.
    private enum Anmeldefehler: Error, Equatable {
        case abgebrochen
        /// Ihr OAuth-Kürzel: `invalid_grant`, `invalid_client`, `access_denied` …
        case abgewiesen(String)
        case nichtErreichbar(String)

        var kuerzel: String {
            switch self {
            case .abgebrochen: return "abgebrochen"
            case .abgewiesen(let k): return k
            case .nichtErreichbar(let k): return k
            }
        }
    }

    private func endpunkteHolen() async throws -> OidcEndpunkte {
        if let endpunkte { return endpunkte }
        let adresse = aussteller.appending(path: ".well-known/openid-configuration")
        guard let (daten, antwort) = try? await sitzungsNetz.data(from: adresse),
              let http = antwort as? HTTPURLResponse, http.statusCode == 200,
              let gelesen = try? JSONDecoder().decode(OidcEndpunkte.self, from: daten)
        // Die Discovery ist ein Netzabruf wie jeder andere: Scheitert sie,
        // konnten wir nicht fragen — das ist kein Nein der Rössing-ID.
        else { throw Anmeldefehler.nichtErreichbar("discovery") }
        endpunkte = gelesen
        return gelesen
    }

    private func browserAnmeldung(_ adresse: URL) async throws -> URL {
        try await withCheckedThrowingContinuation { fortsetzung in
            let sitzung = ASWebAuthenticationSession(
                url: adresse,
                callbackURLScheme: ruecksprungSchema
            ) { rueckweg, fehler in
                if let rueckweg {
                    fortsetzung.resume(returning: rueckweg)
                } else {
                    fortsetzung.resume(throwing: fehler ?? Anmeldefehler.abgebrochen)
                }
            }
            sitzung.presentationContextProvider = self
            // Die geteilte Browser-Sitzung ist gewollt: Wer schon in der
            // Rössing-ID angemeldet ist (Website, Verwaltung), kommt ohne
            // erneutes Passwort durch.
            sitzung.prefersEphemeralWebBrowserSession = false
            laufendeAnmeldung = sitzung
            sitzung.start()
        }
    }

    private func tauschen(code: String, pruefer: String, ziel: URL) async throws -> Tokensatz {
        try await tokenAnfrage(ziel: ziel, felder: [
            "grant_type": "authorization_code",
            "code": code,
            "redirect_uri": ruecksprung,
            "client_id": clientId,
            "code_verifier": pruefer,
        ])
    }

    private func erneuern(mit erneuerungstoken: String) async throws -> Tokensatz {
        let ziele = try await endpunkteHolen()
        var neu = try await tokenAnfrage(ziel: ziele.token_endpoint, felder: [
            "grant_type": "refresh_token",
            "refresh_token": erneuerungstoken,
            "client_id": clientId,
        ])
        // Zitadel schickt bei der Erneuerung nicht zwingend ein neues
        // Erneuerungstoken mit — dann gilt das alte weiter.
        if neu.erneuerungstoken == nil { neu.erneuerungstoken = erneuerungstoken }
        return neu
    }

    private struct TokenAntwort: Decodable {
        let access_token: String
        let refresh_token: String?
        let id_token: String?
        let expires_in: Double?
    }

    /// Die Fehlerantwort des Token-Endpunkts nach RFC 6749.
    private struct TokenFehlerAntwort: Decodable {
        let error: String?
    }

    private func tokenAnfrage(ziel: URL, felder: [String: String]) async throws -> Tokensatz {
        var anfrage = URLRequest(url: ziel)
        anfrage.httpMethod = "POST"
        anfrage.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")
        anfrage.httpBody = felder
            .map { "\($0.key)=\(Self.kodiert($0.value))" }
            .joined(separator: "&")
            .data(using: .utf8)

        guard let (daten, antwort) = try? await sitzungsNetz.data(for: anfrage),
              let http = antwort as? HTTPURLResponse
        else { throw Anmeldefehler.nichtErreichbar("netz") }

        if http.statusCode == 200 {
            guard let gelesen = try? JSONDecoder().decode(TokenAntwort.self, from: daten) else {
                throw Anmeldefehler.nichtErreichbar("antwort_unlesbar")
            }
            return Tokensatz(
                zugangstoken: gelesen.access_token,
                erneuerungstoken: gelesen.refresh_token,
                idToken: gelesen.id_token,
                laeuftAbAm: Date().addingTimeInterval(gelesen.expires_in ?? 3600)
            )
        }

        // Eine Entscheidung ist nur, was auch wie eine aussieht: 4xx **mit**
        // dem Kürzel, das RFC 6749 dafür vorschreibt. Ein 5xx, ein 429 oder
        // eine Antwort ohne Kürzel ist der Zustand des Servers, nicht sein
        // Urteil über diese Sitzung — und darf sie deshalb nicht beenden.
        let kuerzel = (try? JSONDecoder().decode(TokenFehlerAntwort.self, from: daten))?.error
        if (400 ..< 500).contains(http.statusCode), let kuerzel, !kuerzel.isEmpty {
            throw Anmeldefehler.abgewiesen(kuerzel)
        }
        throw Anmeldefehler.nichtErreichbar(kuerzel ?? "http_\(http.statusCode)")
    }

    // MARK: PKCE

    static func zufallstext() -> String {
        var roh = [UInt8](repeating: 0, count: 32)
        _ = SecRandomCopyBytes(kSecRandomDefault, roh.count, &roh)
        return Data(roh).base64URL
    }

    static func s256(_ pruefer: String) -> String {
        Data(SHA256.hash(data: Data(pruefer.utf8))).base64URL
    }

    private static func kodiert(_ wert: String) -> String {
        var erlaubt = CharacterSet.alphanumerics
        erlaubt.insert(charactersIn: "-._~")
        return wert.addingPercentEncoding(withAllowedCharacters: erlaubt) ?? wert
    }
}

extension Anmeldung: ASWebAuthenticationPresentationContextProviding {
    func presentationAnchor(for session: ASWebAuthenticationSession) -> ASPresentationAnchor {
        let fenster = UIApplication.shared.connectedScenes
            .compactMap { $0 as? UIWindowScene }
            .flatMap(\.windows)
            .first { $0.isKeyWindow }
        return fenster ?? ASPresentationAnchor()
    }
}

extension Data {
    /// base64url ohne Auffüllzeichen — so verlangt es RFC 7636 für PKCE.
    var base64URL: String {
        base64EncodedString()
            .replacingOccurrences(of: "+", with: "-")
            .replacingOccurrences(of: "/", with: "_")
            .replacingOccurrences(of: "=", with: "")
    }
}
