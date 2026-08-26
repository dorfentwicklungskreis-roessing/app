import Foundation
import Testing

@testable import Dorf

/// Der Bereich „Mein Profil" und „Dorfbewohner".
///
/// Hier steht der Datenschutz auf dem Spiel: Was vorbelegt sichtbar ist, was
/// ein Schalter im Backend bedeutet und was Verwaltende gekennzeichnet zu
/// sehen bekommen. Deshalb steht das als Test da und nicht nur als Absicht.
@MainActor
struct ProfilTests {
    // MARK: Vorbelegung

    @Test func kontaktdatenSindNichtVorbelegtSichtbar() {
        let stand = Profilstand()
        #expect(stand.anzeigenameOeffentlich)
        #expect(stand.nicknameOeffentlich)
        #expect(!stand.telefonOeffentlich)
        #expect(!stand.emailOeffentlich)
        #expect(!stand.notizOeffentlich)
        #expect(stand.freigegebeneFelder.isEmpty, "Leere Felder geben nichts preis")
    }

    @Test func vorbelegungKommtAusDerRoessingIdWennNochKeinProfilDaIst() {
        let stand = Profilstand(ich: Ich(sub: "abc", name: "Anna Beispiel",
                                         email: "anna@example.org"))
        #expect(stand.anzeigename == "Anna Beispiel")
        #expect(stand.email == "anna@example.org")
        #expect(stand.nickname.isEmpty)
        // Auch mit vorbelegter E-Mail bleibt sie bei der Verwaltung.
        #expect(!stand.emailOeffentlich)
        #expect(stand.freigegebeneFelder == ["Anzeigename"])
    }

    @Test func gespeichertesProfilSchlaegtDieRoessingId() {
        let profil = Profil(userSub: "abc", displayName: "Anna aus der Gartenstraße",
                            email: "anna@dorf.example")
        let stand = Profilstand(profil: profil,
                                ausweis: Ich(sub: "abc", name: "Anna Beispiel",
                                             email: "anna@example.org"))
        #expect(stand.anzeigename == "Anna aus der Gartenstraße")
        #expect(stand.email == "anna@dorf.example")
    }

    @Test func vorbelegungUebernimmtDieSichtbarkeitAusDemProfil() {
        let profil = Profil(userSub: "abc", displayName: "Anna", phone: "05069 1234",
                            visibility: Sichtbarkeit(phone: Sichtbarkeit.dorf))
        let stand = Profilstand(profil: profil)
        #expect(stand.telefonOeffentlich)
        #expect(stand.freigegebeneFelder == ["Anzeigename", "Telefonnummer"])
    }

    // MARK: Schalter → Backend-Wert

    @Test func schalterWerdenZuBackendwerten() {
        var stand = Profilstand()
        stand.anzeigename = "  Anna  "
        stand.nickname = "Gießkanne"
        stand.telefon = "05069 1234"
        stand.email = "anna@example.org"
        stand.notiz = "erreichbar abends"
        stand.anzeigenameOeffentlich = false
        stand.telefonOeffentlich = true

        let eingabe = stand.alsEingabe
        #expect(eingabe.displayName == "Anna", "Das Backend stutzt, also stutzt die App auch")
        #expect(eingabe.visibility.displayName == Sichtbarkeit.verwaltung)
        #expect(eingabe.visibility.nickname == Sichtbarkeit.dorf)
        #expect(eingabe.visibility.phone == Sichtbarkeit.dorf)
        #expect(eingabe.visibility.email == Sichtbarkeit.verwaltung)
        #expect(eingabe.visibility.note == Sichtbarkeit.verwaltung)
    }

    // MARK: Geändert oder nicht

    @Test func geaenderteEingabeErkenntGleichUndUngleich() {
        let gespeichert = Profilstand(profil: Profil(userSub: "abc", displayName: "Anna",
                                                     nickname: "Gießkanne"))
        var stand = gespeichert
        #expect(!stand.geaendert(gegenueber: gespeichert))

        // Ein angetipptes Leerzeichen ist keine Änderung — sonst stünde der
        // Speichern-Knopf offen, obwohl gespeichert dasselbe herauskäme.
        stand.anzeigename = "Anna  "
        #expect(!stand.geaendert(gegenueber: gespeichert))

        stand.anzeigename = "Anna B."
        #expect(stand.geaendert(gegenueber: gespeichert))

        // Auch ein umgelegter Schalter allein ist eine Änderung.
        var nurSchalter = gespeichert
        nurSchalter.telefonOeffentlich = true
        #expect(nurSchalter.geaendert(gegenueber: gespeichert))
    }

    // MARK: Wer nichts freigibt, taucht nicht auf

    @Test func ohneFreigegebenenNamenTauchtNiemandAuf() {
        var stand = Profilstand(profil: Profil(userSub: "abc", displayName: "Anna"))
        #expect(!stand.fuerAndereUnsichtbar)

        stand.anzeigenameOeffentlich = false
        #expect(stand.fuerAndereUnsichtbar, "Weder Anzeigename noch Nickname freigegeben")

        stand.nickname = "Gießkanne"
        #expect(!stand.fuerAndereUnsichtbar, "Der Nickname reicht")

        // Ein freigegebenes, aber leeres Feld gibt nichts preis.
        stand.nickname = "   "
        #expect(stand.fuerAndereUnsichtbar)
    }

    // MARK: Speichern

    @Test func abgelehntesSpeichernLaesstDasGetippteStehen() async {
        let modell = Profilmodell()
        modell.vorbelegen(mit: Ich(sub: "abc", name: "Anna Beispiel"))
        modell.stand.nickname = "ein viel zu langer Nickname"

        await modell.speichern(mit: { _ in
            throw DorfFehler.abgelehnt(grund: "nickname ist zu lang (höchstens 40 Zeichen)")
        })

        // Wortlaut des Backends, nicht der eigene Reim darauf.
        #expect(modell.fehler == "nickname ist zu lang (höchstens 40 Zeichen)")
        #expect(modell.stand.nickname == "ein viel zu langer Nickname")
        #expect(modell.hatAenderungen, "Ungespeichertes bleibt ungespeichert")
        #expect(!modell.speichert)
        #expect(modell.hinweis == nil)
    }

    @Test func erfolgreichesSpeichernUebernimmtDasProfil() async {
        let modell = Profilmodell()
        modell.vorbelegen(mit: Ich(sub: "abc", name: "Anna Beispiel"))
        modell.stand.nickname = "Gießkanne"
        modell.stand.telefonOeffentlich = true
        #expect(modell.kannSpeichern)

        var uebernommen: Profil?
        var gesendet: ProfilEingabe?
        await modell.speichern(
            mit: { eingabe in
                gesendet = eingabe
                return Profil(userSub: "abc", displayName: eingabe.displayName,
                              nickname: eingabe.nickname, visibility: eingabe.visibility)
            },
            uebernehmen: { uebernommen = $0 }
        )

        #expect(gesendet?.nickname == "Gießkanne")
        #expect(uebernommen?.nickname == "Gießkanne", "Die Startseite grüßt sofort neu")
        #expect(modell.fehler == nil)
        #expect(modell.hinweis == "Profil gespeichert.")
        #expect(!modell.hatAenderungen, "Nach dem Speichern gibt es nichts mehr zu speichern")
        #expect(!modell.kannSpeichern)
    }

    @Test func vorbelegungUeberschreibtGetipptesNichtMehr() {
        let modell = Profilmodell()
        modell.vorbelegen(mit: Ich(sub: "abc", name: "Anna Beispiel"))
        modell.stand.nickname = "Gießkanne"
        modell.vorbelegen(mit: Ich(sub: "abc", name: "Anna Beispiel"))
        #expect(modell.stand.nickname == "Gießkanne")
    }

    // MARK: Dorfbewohner

    @Test func restrictedWirdNurInDerVerwaltungssichtGekennzeichnet() throws {
        let person = Dorfbewohner(userSub: "abc", name: "Anna", displayName: "Anna",
                                  phone: "05069 1234", restricted: ["phone"])

        let verwaltung = Bewohneransicht(person: person, verwaltungssicht: true)
        #expect(verwaltung.nurFuerVerwaltung("phone"))
        #expect(!verwaltung.nurFuerVerwaltung("email"))

        // Ohne adminView kennzeichnet die App nichts — dort steht ohnehin nur
        // Freigegebenes.
        let dorf = Bewohneransicht(person: person, verwaltungssicht: false)
        #expect(!dorf.nurFuerVerwaltung("phone"))
    }

    @Test func verwaltungssichtKommtAusDerAntwort() async throws {
        let modell = Dorfbewohnermodell()
        await modell.laden(mit: { try Self.antwort(#"""
        {"adminView":true,"members":[
          {"userSub":"a","name":"Anna","displayName":"Anna","phone":"05069 1234",
           "restricted":["phone"]}]}
        """#) })

        #expect(modell.verwaltungssicht)
        let zeile = try #require(modell.gefiltert.first)
        #expect(zeile.nurFuerVerwaltung("phone"))

        // Dieselbe Liste als gewöhnliches Mitglied: keine Kennzeichnung.
        let mitglied = Dorfbewohnermodell()
        await mitglied.laden(mit: { try Self.antwort(#"""
        {"adminView":false,"members":[{"userSub":"a","name":"Anna","displayName":"Anna"}]}
        """#) })
        let ohne = try #require(mitglied.gefiltert.first)
        #expect(!ohne.verwaltungssicht)
        #expect(!ohne.nurFuerVerwaltung("phone"))
    }

    @Test func sucheFindetUeberDieNamen() async throws {
        let modell = Dorfbewohnermodell()
        await modell.laden(mit: { try Self.antwort(#"""
        {"adminView":false,"members":[
          {"userSub":"a","name":"Gießkanne","displayName":"Anna Beispiel","nickname":"Gießkanne"},
          {"userSub":"b","name":"Bernd","displayName":"Bernd Beispiel"}]}
        """#) })
        #expect(modell.gefiltert.count == 2)

        modell.suche = "bernd"
        #expect(modell.gefiltert.map(\.id) == ["b"], "Groß und klein ist egal")

        modell.suche = " anna "
        #expect(modell.gefiltert.map(\.id) == ["a"], "Auch der Anzeigename zählt")

        modell.suche = "Meier"
        #expect(modell.gefiltert.isEmpty)
    }

    @Test func fehlgeschlagenesLadenNenntDenGrund() async {
        let modell = Dorfbewohnermodell()
        await modell.laden(mit: { throw DorfFehler.nichtAngemeldet })
        #expect(modell.fehler == "Die Anmeldung ist abgelaufen. Bitte neu anmelden.")
        #expect(!modell.laedt)
    }

    // MARK: Anrufen und Mailen

    @Test func telefonadresseVertraegtLeerzeichenUndTrennzeichen() throws {
        #expect(try #require(Kontakt.telefon("+49 5069 1234")).absoluteString == "tel:+4950691234")
        #expect(try #require(Kontakt.telefon("05069 / 12-34")).absoluteString == "tel:050691234")
        #expect(try #require(Kontakt.telefon("(05069) 1234")).absoluteString == "tel:050691234")
        #expect(Kontakt.telefon("") == nil)
        #expect(Kontakt.telefon("  ") == nil)
    }

    @Test func mailadresseWirdKorrektGebildet() throws {
        #expect(try #require(Kontakt.mail("anna@example.org")).absoluteString
                == "mailto:anna@example.org")
        #expect(try #require(Kontakt.mail("  anna@example.org ")).absoluteString
                == "mailto:anna@example.org")
        #expect(Kontakt.mail("") == nil)
        #expect(Kontakt.mail("keine Adresse") == nil)
    }

    // MARK: Hilfen

    private static func antwort(_ roh: String) throws -> DorfbewohnerAntwort {
        try JSONDecoder().decode(DorfbewohnerAntwort.self, from: Data(roh.utf8))
    }
}
