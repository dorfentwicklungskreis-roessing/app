import Foundation

/// Der Bearbeitungsstand der Profilseite.
///
/// Eigener Typ und nicht `Profil` selbst: In der Oberfläche ist ein Schalter
/// „an" oder „aus", im Backend steht dort `dorf` oder `verwaltung`. Die
/// Umrechnung passiert an genau einer Stelle (`alsEingabe`) — und sie ist
/// prüfbar, ohne eine Ansicht zu bauen.
///
/// **Die Vorbelegung ist die Vorbelegung des Backends** (`Sichtbarkeit()`):
/// Anzeigename und Nickname sind sichtbar, Telefon, E-Mail und Notiz nicht.
/// Kontaktdaten werden nie still veröffentlicht.
struct Profilstand: Equatable, Sendable {
    var anzeigename: String = ""
    var nickname: String = ""
    var telefon: String = ""
    var email: String = ""
    var notiz: String = ""

    var anzeigenameOeffentlich: Bool = Sichtbarkeit().displayNameOeffentlich
    var nicknameOeffentlich: Bool = Sichtbarkeit().nicknameOeffentlich
    var telefonOeffentlich: Bool = Sichtbarkeit().phoneOeffentlich
    var emailOeffentlich: Bool = Sichtbarkeit().emailOeffentlich
    var notizOeffentlich: Bool = Sichtbarkeit().noteOeffentlich

    init() {}

    /// Vorbelegung aus dem gespeicherten Profil; wo dort nichts steht, treten
    /// Anzeigename und E-Mail aus der Rössing-ID ein — beides bleibt
    /// überschreibbar.
    init(profil: Profil?, ausweis: Ich? = nil) {
        anzeigename = Profilstand.ersterMitInhalt(profil?.displayName, ausweis?.name)
        nickname = profil?.nickname ?? ""
        telefon = profil?.phone ?? ""
        email = Profilstand.ersterMitInhalt(profil?.email, ausweis?.email)
        notiz = profil?.note ?? ""

        let sicht = profil?.visibility ?? Sichtbarkeit()
        anzeigenameOeffentlich = sicht.displayNameOeffentlich
        nicknameOeffentlich = sicht.nicknameOeffentlich
        telefonOeffentlich = sicht.phoneOeffentlich
        emailOeffentlich = sicht.emailOeffentlich
        notizOeffentlich = sicht.noteOeffentlich
    }

    init(ich: Ich?) {
        self.init(profil: ich?.profile, ausweis: ich)
    }

    private static func ersterMitInhalt(_ kandidaten: String?...) -> String {
        for kandidat in kandidaten {
            let wert = (kandidat ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            if !wert.isEmpty { return wert }
        }
        return ""
    }

    /// Gestutzt wie das Backend es tut. Damit zählt ein versehentliches
    /// Leerzeichen am Ende nicht als Änderung — sonst wäre der
    /// Speichern-Knopf offen, obwohl gespeichert nichts anderes herauskäme.
    var bereinigt: Profilstand {
        var kopie = self
        kopie.anzeigename = anzeigename.trimmingCharacters(in: .whitespacesAndNewlines)
        kopie.nickname = nickname.trimmingCharacters(in: .whitespacesAndNewlines)
        kopie.telefon = telefon.trimmingCharacters(in: .whitespacesAndNewlines)
        kopie.email = email.trimmingCharacters(in: .whitespacesAndNewlines)
        kopie.notiz = notiz.trimmingCharacters(in: .whitespacesAndNewlines)
        return kopie
    }

    /// Was an `PUT /api/v1/me/profile` geht. Die Prüfung der Werte sitzt im
    /// Backend — hier wird nur umgerechnet, nicht geurteilt.
    var alsEingabe: ProfilEingabe {
        let stand = bereinigt
        return ProfilEingabe(
            displayName: stand.anzeigename,
            nickname: stand.nickname,
            phone: stand.telefon,
            email: stand.email,
            note: stand.notiz,
            visibility: Sichtbarkeit(
                displayName: Sichtbarkeit.wert(stand.anzeigenameOeffentlich),
                nickname: Sichtbarkeit.wert(stand.nicknameOeffentlich),
                phone: Sichtbarkeit.wert(stand.telefonOeffentlich),
                email: Sichtbarkeit.wert(stand.emailOeffentlich),
                note: Sichtbarkeit.wert(stand.notizOeffentlich)
            )
        )
    }

    /// Ob sich gegenüber dem gespeicherten Stand etwas geändert hat.
    func geaendert(gegenueber gespeichert: Profilstand) -> Bool {
        bereinigt != gespeichert.bereinigt
    }

    /// Was gerade tatsächlich alle im Dorf sehen — für den Hinweis über dem
    /// Formular. Ein leeres Feld gibt nichts preis, auch wenn sein Schalter an
    /// ist, und taucht deshalb hier nicht auf.
    var freigegebeneFelder: [String] {
        let stand = bereinigt
        var felder: [String] = []
        if stand.anzeigenameOeffentlich && !stand.anzeigename.isEmpty { felder.append("Anzeigename") }
        if stand.nicknameOeffentlich && !stand.nickname.isEmpty { felder.append("Nickname") }
        if stand.telefonOeffentlich && !stand.telefon.isEmpty { felder.append("Telefonnummer") }
        if stand.emailOeffentlich && !stand.email.isEmpty { felder.append("E-Mail-Adresse") }
        if stand.notizOeffentlich && !stand.notiz.isEmpty { felder.append("Notiz") }
        return felder
    }

    /// Wer weder Anzeigenamen noch Nickname freigibt, taucht für die anderen
    /// gar nicht in der Dorfbewohner-Liste auf — dieselbe Regel wie im
    /// Backend (`Profile.AsMember`). Das gehört gesagt, bevor jemand sich
    /// wundert, warum niemand ihn findet.
    var fuerAndereUnsichtbar: Bool {
        let stand = bereinigt
        let nameSichtbar = stand.anzeigenameOeffentlich && !stand.anzeigename.isEmpty
        let nickSichtbar = stand.nicknameOeffentlich && !stand.nickname.isEmpty
        return !nameSichtbar && !nickSichtbar
    }
}

/// Adressen für Telefon- und E-Mail-App.
///
/// Rufnummern stehen so in der Liste, wie sie aufgeschrieben wurden
/// („05069 / 12 34"). Zum Wählen taugt davon nur, was gewählt wird: Ziffern,
/// führendes Plus und die Wahlzeichen `*` und `#`. Alles andere — Leerzeichen,
/// Schrägstriche, Bindestriche, Klammern — fällt weg, statt in der URL zu
/// landen, wo es je nach Telefon-App den Anruf verhindert.
enum Kontakt {
    private static let waehlbar = Set("0123456789+*#")

    static func telefon(_ nummer: String) -> URL? {
        let gewaehlt = String(nummer.filter { waehlbar.contains($0) })
        guard gewaehlt.contains(where: \.isNumber) else { return nil }
        return URL(string: "tel:\(gewaehlt)")
    }

    static func mail(_ adresse: String) -> URL? {
        let gestutzt = adresse.trimmingCharacters(in: .whitespacesAndNewlines)
        guard gestutzt.contains("@"), !gestutzt.contains(where: \.isWhitespace) else { return nil }
        guard let kodiert = gestutzt.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed)
        else { return nil }
        return URL(string: "mailto:\(kodiert)")
    }
}
