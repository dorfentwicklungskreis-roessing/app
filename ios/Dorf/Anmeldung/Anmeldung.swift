import AuthenticationServices
import CryptoKit
import Foundation
import Observation

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
@Observable
final class Anmeldung: NSObject {
    private(set) var sitzung: Sitzungszustand = .laedt

    /// Das zuletzt erhaltene ID-Token — nur zur Diagnose („warum bin ich
    /// nicht Verwaltung?"). Zitadel legt Rollen je nach Einstellung ins
    /// Zugangs- oder ins ID-Token; wer das untersucht, braucht beide.
    private(set) var letztesIdToken: String?

    @ObservationIgnored private var tokensatz: Tokensatz?
    @ObservationIgnored private var entwicklerToken: String?
    @ObservationIgnored private var endpunkte: OidcEndpunkte?
    @ObservationIgnored private var laufendeAnmeldung: ASWebAuthenticationSession?

    override init() {
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
        if let satz = Schluesselbund.lesen() {
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

    /// Liefert ein gültiges Zugangstoken (erneuert bei Bedarf) oder `nil`.
    /// Wird vor **jeder** API-Anfrage aufgerufen.
    func frischesToken() async -> String? {
        if let entwicklerToken { return entwicklerToken }
        guard let satz = tokensatz else { return nil }
        if satz.gueltig() { return satz.zugangstoken }
        guard let erneuerung = satz.erneuerungstoken else {
            abmelden()
            return nil
        }
        do {
            let neu = try await erneuern(mit: erneuerung)
            uebernehmen(neu)
            return neu.zugangstoken
        } catch {
            // Erneuerung endgültig gescheitert (z.B. Token widerrufen):
            // abmelden ist ehrlicher, als weiter 401 zu sammeln.
            abmelden()
            return nil
        }
    }

    private func uebernehmen(_ satz: Tokensatz) {
        tokensatz = satz
        letztesIdToken = satz.idToken ?? letztesIdToken
        Schluesselbund.sichern(satz)
        sitzung = .angemeldet(entwicklerModus: false)
    }

    func abmelden() {
        tokensatz = nil
        entwicklerToken = nil
        letztesIdToken = nil
        Schluesselbund.loeschen()
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

    func anmelden() async -> Anmeldeergebnis {
        do {
            let ziele = try await endpunkteHolen()
            let pruefer = Self.zufallstext()
            let herausforderung = Self.s256(pruefer)
            let zustand = Self.zufallstext()

            var bau = URLComponents(url: ziele.authorization_endpoint, resolvingAgainstBaseURL: false)!
            bau.queryItems = [
                .init(name: "client_id", value: Konfiguration.oidcClientId),
                .init(name: "response_type", value: "code"),
                .init(name: "redirect_uri", value: Konfiguration.oidcRuecksprung),
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

    private enum Anmeldefehler: Error, Equatable {
        case abgebrochen
        case discovery
        case tokenTausch(String)

        var kuerzel: String {
            switch self {
            case .abgebrochen: return "abgebrochen"
            case .discovery: return "discovery"
            case .tokenTausch(let k): return k
            }
        }
    }

    private func endpunkteHolen() async throws -> OidcEndpunkte {
        if let endpunkte { return endpunkte }
        let adresse = Konfiguration.oidcAussteller
            .appending(path: ".well-known/openid-configuration")
        guard let (daten, antwort) = try? await URLSession.dorfSitzung.data(from: adresse),
              let http = antwort as? HTTPURLResponse, http.statusCode == 200,
              let gelesen = try? JSONDecoder().decode(OidcEndpunkte.self, from: daten)
        else { throw Anmeldefehler.discovery }
        endpunkte = gelesen
        return gelesen
    }

    private func browserAnmeldung(_ adresse: URL) async throws -> URL {
        try await withCheckedThrowingContinuation { fortsetzung in
            let sitzung = ASWebAuthenticationSession(
                url: adresse,
                callbackURLScheme: Konfiguration.ruecksprungSchema
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
            "redirect_uri": Konfiguration.oidcRuecksprung,
            "client_id": Konfiguration.oidcClientId,
            "code_verifier": pruefer,
        ])
    }

    private func erneuern(mit erneuerungstoken: String) async throws -> Tokensatz {
        let ziele = try await endpunkteHolen()
        var neu = try await tokenAnfrage(ziel: ziele.token_endpoint, felder: [
            "grant_type": "refresh_token",
            "refresh_token": erneuerungstoken,
            "client_id": Konfiguration.oidcClientId,
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

    private func tokenAnfrage(ziel: URL, felder: [String: String]) async throws -> Tokensatz {
        var anfrage = URLRequest(url: ziel)
        anfrage.httpMethod = "POST"
        anfrage.setValue("application/x-www-form-urlencoded", forHTTPHeaderField: "Content-Type")
        anfrage.httpBody = felder
            .map { "\($0.key)=\(Self.kodiert($0.value))" }
            .joined(separator: "&")
            .data(using: .utf8)

        guard let (daten, antwort) = try? await URLSession.dorfSitzung.data(for: anfrage),
              let http = antwort as? HTTPURLResponse
        else { throw Anmeldefehler.tokenTausch("netz") }
        guard http.statusCode == 200,
              let gelesen = try? JSONDecoder().decode(TokenAntwort.self, from: daten)
        else {
            let kuerzel = (try? JSONDecoder().decode([String: String].self, from: daten))?["error"]
            throw Anmeldefehler.tokenTausch(kuerzel ?? "http_\(http.statusCode)")
        }
        return Tokensatz(
            zugangstoken: gelesen.access_token,
            erneuerungstoken: gelesen.refresh_token,
            idToken: gelesen.id_token,
            laeuftAbAm: Date().addingTimeInterval(gelesen.expires_in ?? 3600)
        )
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
