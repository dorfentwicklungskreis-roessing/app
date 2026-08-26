# Veröffentlichung im App Store — Anleitung

Geschrieben für jemanden, der das zum ersten Mal macht. Alles, was im Repo
liegen kann, liegt in `store/`; was hier steht, geht nur mit einem
Apple-Developer-Konto und muss von Hand gemacht werden.

**Ausgangslage**

| | |
|---|---|
| Apple-ID | `post+apple@levinkeller.de` |
| Apple Developer Program | **soeben abgeschlossen** (99 $/Jahr) |
| Bundle-ID | `de.roessing.app` — steht schon in `ios/project.yml` als `PRODUCT_BUNDLE_IDENTIFIER` |
| App-Name im Store | **Rössing** (`store/metadata/ios/de-DE/name.txt`) |
| Anzeigename auf dem Home-Bildschirm | **Rössing** (`CFBundleDisplayName` in `ios/Dorf/Info.plist`) |
| Version | `MARKETING_VERSION 0.1.0`, `CURRENT_PROJECT_VERSION 1` (`ios/project.yml`) |

Die beiden Apple-Portale heißen ähnlich und werden gern verwechselt:

- **developer.apple.com/account** — Mitgliedschaft, Zertifikate, Bundle-IDs.
  Hier wird die App *technisch* angemeldet.
- **appstoreconnect.apple.com** — der Store-Eintrag, TestFlight, die Prüfung.
  Hier wird die App *veröffentlicht*.

Die Reihenfolge unten ist die Reihenfolge, in der es gemacht werden muss.
Schritt 1 blockiert alles Weitere.

---

## 1. Team-ID holen und eintragen (blockiert alles andere)

Ohne Team-ID lässt sich nichts signieren, und ohne Signatur gibt es kein
TestFlight. `ios/project.yml` hat das Feld schon, aber leer:

```yaml
DEVELOPMENT_TEAM: ""
CODE_SIGNING_REQUIRED: NO
CODE_SIGNING_ALLOWED: NO
```

**Wo die Team-ID steht:** <https://developer.apple.com/account> → in der
linken Spalte **Membership details** (früher „Membership"). Dort steht neben
*Team ID* eine zehnstellige Kennung aus Großbuchstaben und Ziffern, z.B.
`A1B2C3D4E5`. Sie ist kein Geheimnis — sie steckt in jeder signierten App.

**Was damit passiert** (Änderung an `ios/project.yml`, die *nicht* zu diesem
Arbeitsstand gehört — sieben andere Leute arbeiten gerade an der Datei):

```yaml
DEVELOPMENT_TEAM: "A1B2C3D4E5"   # die echte Kennung
CODE_SIGN_STYLE: Automatic
# Für Simulator-Builds und CI bleibt das Abschalten richtig; für den
# Archivlauf (TestFlight) müssen die beiden Zeilen weg bzw. auf YES:
CODE_SIGNING_REQUIRED: YES
CODE_SIGNING_ALLOWED: YES
```

Danach `cd ios && make projekt` — das erzeugt `Dorf.xcodeproj` neu. Beim
ersten Öffnen in Xcode (`make oeffnen`) meldet sich *Signing & Capabilities*
und legt das Entwicklungszertifikat automatisch an; man muss dort nur einmal
das Team auswählen und Xcode „Automatically manage signing" lassen.

> **Zweiter Handgriff an `ios/project.yml`, der ebenfalls noch fehlt:** Der
> Asset-Katalog liegt seit diesem Arbeitsstand unter
> `ios/Dorf/Assets.xcassets/`, aber Xcode muss noch gesagt bekommen, welches
> Symbol und welche Akzentfarbe daraus die der App sind:
>
> ```yaml
> ASSETCATALOG_COMPILER_APPICON_NAME: AppIcon
> ASSETCATALOG_COMPILER_GLOBAL_ACCENT_COLOR_NAME: AccentColor
> ```
>
> Ohne die erste Zeile baut die App **ohne Icon**, und der Upload scheitert
> mit „Missing app icon".

---

## 2. App ID (Bundle-ID) anlegen

<https://developer.apple.com/account> → **Certificates, Identifiers &
Profiles** → in der linken Spalte **Identifiers** → blaues **＋** oben.

1. **Register a new identifier** → *App IDs* → **Continue**
2. **Select a type** → *App* → **Continue**
3. Formular:
   - **Description:** `Roessing Dorf-App` (nur intern, keine Umlaute und
     keine Sonderzeichen erlaubt)
   - **Bundle ID:** **Explicit** (nicht Wildcard) → `de.roessing.app`
   - Das muss **exakt** so geschrieben sein wie
     `PRODUCT_BUNDLE_IDENTIFIER` in `ios/project.yml`. Ein Tippfehler ist
     hier teuer: Eine Bundle-ID lässt sich nicht umbenennen und nicht löschen.
4. **Capabilities: keine ankreuzen.** Die App braucht heute keine:
   - **Push Notifications** — nein. Push ist ausdrücklich nach der ersten
     Fassung vorgesehen (`ios/README.md`, „Was noch fehlt"); im ganzen
     Verzeichnis `ios/` gibt es keinen Aufruf von `UNUserNotificationCenter`.
   - **Sign in with Apple** — nein, siehe Schritt 7.
   - **Associated Domains**, **App Groups**, **HealthKit**, **In-App
     Purchase**, **Maps** — alles nein. Die Karte ist MapLibre mit
     OpenFreeMap-Kacheln (`MAP_STYLE_URL` in `ios/project.yml`), nicht
     Apple Maps; dafür braucht es keine Capability.
   - Die Anmeldung braucht ebenfalls keine: Sie läuft über
     `ASWebAuthenticationSession` mit dem eigenen URL-Schema
     `de.roessing.app` (`CFBundleURLTypes` in `ios/Dorf/Info.plist`) — das
     ist eine Info.plist-Angabe, keine Capability.
5. **Continue** → **Register**

**Was später nachgetragen wird:** Capabilities lassen sich jederzeit
hinzufügen. Wenn Push kommt, wird hier *Push Notifications* angehakt, dazu
gehört dann ein **APNs-Key** (Keys → ＋ → *Apple Push Notifications service*)
und ein `aps-environment`-Entitlement in der App. Beides ist heute nicht
nötig und würde die Prüfung nur mit unbenutzten Berechtigungen belasten.

---

## 3. App-Datensatz in App Store Connect anlegen

<https://appstoreconnect.apple.com> → **Apps** (Kacheln auf der Startseite)
→ blaues **＋** oben links → **New App**.

| Feld | Wert | Anmerkung |
|---|---|---|
| **Platforms** | ☑ **iOS** | tvOS/visionOS/macOS leer lassen |
| **Name** | `Rössing` | aus `store/metadata/ios/de-DE/name.txt`; im ganzen Store eindeutig — ist der Name vergeben, meldet Apple das sofort |
| **Primary Language** | **German (Germany)** | die App ist deutschsprachig (`CFBundleDevelopmentRegion` = `de`) |
| **Bundle ID** | `de.roessing.app` | Auswahlliste. Steht die ID nicht drin, ist Schritt 2 nicht fertig oder die Seite muss neu geladen werden |
| **SKU** | `roessing-ios` | frei wählbar, wird nie öffentlich gezeigt, lässt sich nicht mehr ändern. Nur für die eigene Abrechnung |
| **User Access** | **Full Access** | Ein-Personen-Team |

**Create** drücken. Danach steht die App in der Liste, Status *Prepare for
Submission*.

### Was gleich danach ausgefüllt wird

Alles unter **App Store** (Reiter oben) bzw. der linken Spalte:

| Ort in App Store Connect | Woher der Inhalt kommt |
|---|---|
| *Localizations* → **German (Germany)** und **English (U.S.)** | `store/metadata/ios/de-DE/` und `store/metadata/ios/en-US/` |
| Name / Subtitle | `name.txt` / `subtitle.txt` |
| Promotional Text | `promotional_text.txt` (änderbar **ohne** neue Prüfung — dafür ist es da) |
| Description | `description.txt` |
| Keywords | `keywords.txt` — kommagetrennt, **ohne Leerzeichen hinter dem Komma**, Apple zählt sie mit |
| Support URL | `support_url.txt` → <https://xn--rssing-wxa.de/impressum/> |
| Marketing URL | `marketing_url.txt` → <https://xn--rssing-wxa.de/> |
| What's New in This Version | `release_notes.txt` |
| *App Information* → **Privacy Policy URL** | `privacy_url.txt` → <https://xn--rssing-wxa.de/app/datenschutz/> |
| *App Information* → **Category** | Primary: **Utilities**, Secondary: **Lifestyle** (frei wählbar, jederzeit änderbar) |
| *App Information* → **Content Rights** | „Enthält keine Inhalte Dritter" |
| *Age Rating* → Fragebogen | Antworten stehen in `store/content-rating.md`. **Abweichung für iOS:** „Werden Daten an Dritte weitergegeben?" ist hier **Nein** — es gibt kein Push und damit kein Firebase |
| **App Privacy** | `store/ios-datenschutz.md` — je Datenart erhoben/verknüpft/Tracking |
| *Pricing and Availability* | **Free**, weltweit verfügbar |
| **App Screenshots** | **fehlen noch** — mindestens einer für 6,9″ iPhone (1290×2796 oder 1320×2868). Apple rechnet daraus die kleineren Größen selbst |

Die Zeichengrenzen sind nachgerechnet, nicht geschätzt:

```sh
python3 store/check_ios_metadata.py
```

Das Skript prüft Vollständigkeit beider Sprachen, alle Grenzen (Name 30,
Untertitel 30, Schlüsselwörter 100, Werbetext 170, Beschreibung 4000,
Neuerungen 4000), die URL-Felder und das App-Icon. Es läuft ohne
Fremdbibliotheken.

---

## 4. App Store Connect API-Key für die CI

Damit die GitHub-Action später Builds hochladen kann, ohne dass jemand ein
Apple-Passwort in ein Secret schreibt.

<https://appstoreconnect.apple.com> → **Users and Access** (oben) → Reiter
**Integrations** → links **App Store Connect API** → Abschnitt **Team Keys**
→ **＋**.

1. **Name:** `CI GitHub Actions`
2. **Access (Rolle):** **App Manager**.
   *Developer* würde zum reinen Hochladen reichen, kann aber die
   TestFlight-Angaben und die Store-Metadaten nicht pflegen — genau das soll
   die CI später tun. *Admin* wäre mehr als nötig; ein Schlüssel in einer CI
   soll nie mehr dürfen als seine Aufgabe.
3. **Generate**.

Danach stehen auf derselben Seite drei Dinge:

| Was | Wo | Beispiel |
|---|---|---|
| **Issuer ID** | ganz oben über der Schlüsselliste, gilt fürs ganze Team | `57246542-96fe-1a63-e053-0824d011072a` |
| **Key ID** | in der Zeile des neuen Schlüssels | `2X9R4HXF34` |
| **Die `.p8`-Datei** | Spalte *Download*, Button **Download API Key** | `AuthKey_2X9R4HXF34.p8` |

> **Die `.p8`-Datei lässt sich genau einmal herunterladen.** Danach ist der
> Knopf weg — für immer. Geht sie verloren, hilft nur: Schlüssel widerrufen
> („Revoke") und einen neuen anlegen. Sie ist ein Passwort-Äquivalent:
> nicht ins Repo, nicht in eine Chat-Nachricht, sondern in den Passwort-Safe.

### Die drei GitHub-Secrets

Repo → *Settings → Secrets and variables → Actions → New repository secret*.
Benennung im Stil des bestehenden `PLAY_SERVICE_ACCOUNT_JSON`:

| Name | Inhalt |
|---|---|
| `APP_STORE_CONNECT_ISSUER_ID` | die Issuer ID (UUID) |
| `APP_STORE_CONNECT_KEY_ID` | die Key ID (10 Zeichen) |
| `APP_STORE_CONNECT_PRIVATE_KEY` | der **komplette Inhalt** der `.p8`-Datei, einschließlich der Zeilen `-----BEGIN PRIVATE KEY-----` und `-----END PRIVATE KEY-----` (`cat AuthKey_*.p8 \| pbcopy`) |

Solange die drei Secrets fehlen, kann ein iOS-Workflow den Upload
überspringen — genauso, wie es der Android-Workflow ohne
`PLAY_SERVICE_ACCOUNT_JSON` tut.

---

## 5. TestFlight

**Vor jedem TestFlight-Test muss ein Build da sein.** Der erste kommt von
Hand aus Xcode: *Product → Destination → Any iOS Device (arm64)* → *Product →
Archive* → im Organizer **Distribute App** → **TestFlight & App Store** →
**Upload**. Danach dauert es 5–30 Minuten, bis der Build in App Store Connect
unter *TestFlight → iOS Builds* auftaucht („Processing").

### Interne Tester — der schnelle Weg

*TestFlight* → linke Spalte **Internal Testing** → **＋** neben *Testers* →
Gruppe anlegen (z.B. `Dorf`) → Personen hinzufügen.

- Interne Tester sind **Mitglieder des eigenen App-Store-Connect-Teams**.
  Wer noch keins ist, wird vorher unter *Users and Access* eingeladen (Rolle
  *Developer* reicht; die Person braucht eine Apple-ID).
- Bis **100 interne Tester**, je bis zu 30 Geräte.
- **Interne Tests brauchen keine Beta App Review.** Apple prüft hier nur
  automatisch: gültige Signatur, vollständige Info.plist, keine verbotenen
  APIs — und die **Export-Compliance-Angabe** (Schritt 6). Sobald der Build
  „Processing" verlassen hat, kann er freigegeben und installiert werden.
  Das ist der Weg, um die App in den nächsten Tagen aufs eigene Telefon zu
  bekommen.
- Ein Build läuft **90 Tage** ab dem Hochladen.

### Externe Tester — was zusätzlich nötig ist

*TestFlight* → **External Testing** → Gruppe anlegen. Hier prüft ein Mensch
bei Apple, deshalb kommt Folgendes dazu:

1. **Beta App Review** — dauert meist ein bis zwei Werktage, bei der **ersten**
   Einreichung einer App auch länger. Bei jedem Build mit neuer
   Versionsnummer erneut, kleine Build-Nummern-Erhöhungen meist automatisch.
2. **Test Information** (*TestFlight → Test Information*), Pflicht:
   - **Beta App Description** — was zu testen ist
   - **Feedback Email** — eine Adresse, die auch gelesen wird
   - **Marketing URL** und **Privacy Policy URL** (dieselben wie oben)
3. **Ein funktionierendes Demo-Konto** — ohne das wird abgelehnt, siehe
   Schritt 8. Bei External Testing steht es unter *Test Information → Sign-in
   Information*.
4. Einladung per E-Mail oder über einen **öffentlichen Link** (bis 10.000
   Tester).

**Empfehlung:** erst intern testen, bis die App steht. Externe Tester lohnen
sich erst, wenn mehr als der eigene Kreis mitmachen soll — und der Aufwand ist
derselbe wie eine echte Store-Prüfung.

---

## 6. Export-Compliance ein für alle Mal erledigen

Bei **jedem** Upload fragt App Store Connect: *„Does your app use encryption?"*
Wer nichts tut, muss das je Build von Hand beantworten, und ein
unbeantworteter Build lässt sich nicht an Tester verteilen.

**Die richtige Antwort für diese App:** Sie nutzt ausschließlich HTTPS über
die Systemschnittstellen — `URLSession` in `ios/Dorf/Daten/DorfApi.swift`,
`ASWebAuthenticationSession` in `ios/Dorf/Anmeldung/Anmeldung.swift`, dazu
`CryptoKit` für den PKCE-Hash (SHA-256, ein Hash, keine Verschlüsselung).
Alle Adressen in `ios/project.yml` beginnen mit `https://`. Es gibt keine
eigene Kryptographie und keine Verschlüsselung fremder Daten. Damit greift
die Standardausnahme: **„No" bzw. `ITSAppUsesNonExemptEncryption = false`.**

**Dauerhaft erledigt** ist das mit einem Eintrag in `ios/Dorf/Info.plist` —
dann fragt Apple nie wieder:

```xml
<key>ITSAppUsesNonExemptEncryption</key>
<false/>
```

Der Eintrag fehlt heute noch (`ios/Dorf/Info.plist` gehört zu einem anderen
Arbeitsstand). Bis er drin ist, erscheint die Frage bei jedem Build unter
*TestFlight → Builds → [Build] → Manage* und ist mit **No** zu beantworten.

---

## 7. Sign in with Apple — braucht die App das?

**Nein.** Begründung, an der Richtlinie geprüft:

**App Review Guideline 4.8 „Login Services"** greift ausdrücklich nur für
Apps, die einen **Anmeldedienst eines Dritten oder eines sozialen Netzwerks**
anbieten, um das Hauptkonto der App einzurichten — Apple nennt als Beispiele
Facebook Login, Google Sign-In, Sign in with Twitter, LinkedIn, Login with
Amazon, WeChat Login. Wer so etwas anbietet, muss **zusätzlich** eine
gleichwertige, datensparsame Anmeldung anbieten (seit 2022 nicht mehr
zwingend Sign in with Apple, aber eine, die auf Name und E-Mail beschränkt
bleibt, das Verbergen der Adresse erlaubt und nicht trackt).

Diese App bietet **keine** solche Anmeldung an. Es gibt genau einen Weg
hinein: die **Rössing-ID**, den eigenen OIDC-Dienst des Dorfes auf
`id.xn--rssing-wxa.de` (Zitadel, vom Dorfentwicklungskreis selbst betrieben).
Belegt in `ios/Dorf/Anmeldung/Anmeldung.swift` — dort steht ein einziger
Authorization-Code-Fluss gegen `Konfiguration.oidcAussteller`, und
`ios/Dorf/Anmeldung/AnmeldungView.swift` hat genau einen Anmeldeknopf („Mit
Rössing-ID anmelden"); der zweite, der dort im Quelltext steht, ist der
Entwickler-Login und in einer ausgelieferten App nicht vorhanden
(`#if DEBUG` in `Konfiguration.entwicklerLoginErlaubt`). Eine
Anmeldung, die direkt beim Anbieter der App selbst stattfindet, fällt nicht
unter 4.8 — genauso wenig wie ein klassisches Konto mit E-Mail und Passwort.

### Text für das Feld „Notes" der App-Prüfung

> Die App bietet keine Anmeldung über Dienste Dritter oder soziale Netzwerke
> an (kein Google, kein Facebook, kein Apple). Es gibt genau einen
> Anmeldeweg: die „Rössing-ID", den eigenen OpenID-Connect-Dienst des
> Betreibers unter id.xn--rssing-wxa.de. Die Konten vergibt der
> Dorfentwicklungskreis Rössing selbst an Einwohnerinnen und Einwohner des
> Dorfes. Guideline 4.8 ist daher nicht einschlägig, und Sign in with Apple
> wird nicht angeboten.

> *(Englisch, falls das Feld englisch ausgefüllt werden soll:)*
> The app offers no third-party or social login (no Google, no Facebook, no
> Apple). There is exactly one way to sign in: the "Rössing-ID", the
> operator's own OpenID Connect service at id.xn--rssing-wxa.de. Accounts are
> issued by the Dorfentwicklungskreis Rössing to residents of the village.
> Guideline 4.8 therefore does not apply and Sign in with Apple is not
> offered.

**Wenn sich das ändert:** Sobald jemals ein „Mit Google anmelden" dazukommt,
wird 4.8 einschlägig — dann muss die Capability *Sign in with Apple* in
Schritt 2 nachgetragen und die zweite Anmeldung eingebaut werden.

---

## 8. Review-Notiz und Demo-Konto — der häufigste Ablehnungsgrund

**Ohne Rössing-ID sieht ein Prüfer nichts.** Der Anmeldebildschirm ist alles,
was er ohne Konto zu sehen bekommt (`ios/Dorf/Navigation/WurzelView.swift`
zeigt bei `.abgemeldet` nur `AnmeldungView`). Apple lehnt solche Apps
zuverlässig nach **Guideline 2.1 — Performance: App Completeness** ab, mit
dem Hinweis „we were unable to sign in".

Ort in App Store Connect: beim Einreichen unter **App Review Information**
(bzw. für TestFlight unter *Test Information → Sign-in Information*).

### Was einzutragen ist

1. **☑ Sign-in required** ankreuzen.
2. **User name** und **Password** des Prüfkontos.
3. **Notes** — Vorschlag zum Übernehmen:

> Die App ist ohne Konto nicht benutzbar; die Anmeldung läuft über den
> eigenen Identitätsdienst des Dorfes („Rössing-ID", id.xn--rssing-wxa.de).
>
> So kommen Sie hinein:
> 1. App starten, auf „Mit Rössing-ID anmelden" tippen.
> 2. Es öffnet sich der Systembrowser mit der Anmeldeseite von
>    id.xn--rssing-wxa.de. Dort Benutzername und Passwort von oben eingeben.
> 3. Der Browser springt automatisch in die App zurück; danach ist die
>    Startseite mit den Bereichen sichtbar.
>
> Die App richtet sich an die Einwohner des Dorfes Rössing (Gemeinde
> Nordstemmen, Deutschland) und verwaltet die Pflege öffentlicher
> Blumenkästen und Beete. Die Oberfläche ist ausschließlich deutschsprachig.
> Es gibt keine Käufe, keine Werbung und kein Tracking.
>
> [Absatz aus Schritt 7 zu Guideline 4.8 hier anhängen.]

### ⚠️ Dieses Konto gibt es noch nicht — es muss angelegt werden

Vorschlag, parallel zum bereits vorhandenen Play-Prüfkonto `google-reviewer`
(`store/veroeffentlichung.md`, Schritt 4):

| | |
|---|---|
| **Wo** | Rössing-ID (Zitadel) auf `id.xn--rssing-wxa.de`, Organisation des Dorfes |
| **Benutzername** | `apple-reviewer` |
| **Rolle** | `member` im Projekt `dorf-app` — **nicht** `admin`. Ein Prüfer soll die App sehen, nicht das Dorf verwalten |
| **Passwort** | erzeugen und in `.env` ablegen unter dem Schlüssel `APPLE_REVIEWER_PASSWORD` — genau wie `GOOGLE_REVIEWER_PASSWORD` |
| **Einstellungen** | Passwortwechsel beim ersten Anmelden **aus**, Zwei-Faktor **aus**. Ein Prüfer kommt sonst nicht durch — er hat kein Telefon des Dorfes |
| **Bewusst getrennt** | von `test-dorf` (automatische Tests) und von `google-reviewer` (Play). Drei Konten, drei Zwecke; fällt eines aus, fällt nicht alles aus |

Vor dem Einreichen **einmal selbst damit anmelden** — am besten auf einem
Gerät, auf dem sonst niemand angemeldet ist. Ein Prüfkonto, das nicht
funktioniert, kostet einen kompletten Prüfdurchlauf (mehrere Tage).

---

## 9. App-Icon

Liegt fertig im Repo: `ios/Dorf/Assets.xcassets/AppIcon.appiconset/` mit
`icon-1024.png` (hell), `icon-1024-dark.png` (dunkel) und
`icon-1024-tinted.png` (eingefärbt). Ab iOS 17 genügt je Darstellung **ein**
Bild mit 1024×1024; die kleineren Größen rechnet Xcode beim Bauen selbst.
Das Marketing-Icon im Store zieht Apple aus demselben Bild — es muss nicht
mehr getrennt hochgeladen werden.

**Neu erzeugen** (nach einer Änderung an den SVG-Quellen):

```sh
bash store/assets/render-ios.sh
```

Quellen sind `store/assets/ios-icon.svg`, `…-dark.svg` und `…-tinted.svg` —
dieselbe Blume wie das Play-Icon, auf 1024 gerechnet. Das Skript braucht
**ImageMagick 7** (`brew install imagemagick`) und prüft am Ende selbst Maße
und Alphakanal.

**Warum der Alphakanal wichtig ist:** Ein App-Store-Icon darf keinen haben —
ein Upload mit Transparenz wird mit **ITMS-90717 „Invalid App Store Icon"**
abgewiesen. Das helle Symbol hat deshalb einen deckenden Hintergrund
(`-alpha remove -alpha off` in `render-ios.sh`); dunkel und eingefärbt dürfen
und sollen transparent sein, weil dort das System den Hintergrund stellt.

Gegenprobe von Hand:

```sh
sips -g hasAlpha ios/Dorf/Assets.xcassets/AppIcon.appiconset/icon-1024.png
#   hasAlpha: no        <- so muss es aussehen
```

Falls doch einmal Alpha drin ist:

```sh
magick icon-1024.png -background "#3B6939" -alpha remove -alpha off PNG24:icon-1024.png
```

Zwei Dinge, die das Icon **nicht** haben darf und hier auch nicht hat: eigene
runde Ecken (iOS schneidet die Maske selbst zu — ein mitgeliefertes Eck ergäbe
ein Eck im Eck) und einen Schlagschatten.

`python3 store/check_ios_metadata.py` prüft Maße, Bittiefe und Alphakanal
aller drei Bilder mit.

---

## 10. Und dann?

1. Build hochladen (Schritt 5), intern testen, Fehler beheben, neuen Build
   mit erhöhtem `CURRENT_PROJECT_VERSION` hochladen.
2. Wenn die App steht: *App Store → [Version] → **Add for Review*** →
   **Submit for Review**.
3. Erste Prüfung dauert erfahrungsgemäß ein bis drei Werktage. Die häufigsten
   Ablehnungsgründe bei einer App wie dieser:
   - **2.1 App Completeness** — Prüfkonto fehlt oder funktioniert nicht
     (Schritt 8). Mit Abstand Platz eins.
   - **2.1** — die App ist erkennbar unfertig; Bereiche, die nur „wird gerade
     gebaut" anzeigen, sollten vor der Einreichung entweder fertig oder aus
     der Startseite draußen sein.
   - **5.1.1(v)** — Kontolöschung: Apple verlangt bei Apps mit Konto einen
     Weg, das Konto zu löschen. Siehe `store/ios-datenschutz.md`, offene
     Punkte.
   - **5.1.2 / App Privacy** — die Angaben passen nicht zum beobachteten
     Verhalten. Deshalb ist `store/ios-datenschutz.md` mit Fundstellen
     belegt.
   - **4.2 Minimum Functionality** — kommt bei sehr kleinen Apps vor. Die
     Review-Notiz aus Schritt 8 erklärt den Zweck; das hilft.

---

## Was noch am Repo zu tun ist (nicht in diesem Arbeitsstand)

Diese Änderungen gehören in Dateien, an denen gerade andere arbeiten, und
sind hier nur festgehalten:

- `ios/project.yml`: `DEVELOPMENT_TEAM` füllen, `CODE_SIGNING_REQUIRED`/
  `CODE_SIGNING_ALLOWED` für den Archivlauf auf `YES`,
  `ASSETCATALOG_COMPILER_APPICON_NAME: AppIcon` und
  `ASSETCATALOG_COMPILER_GLOBAL_ACCENT_COLOR_NAME: AccentColor` ergänzen.
- `ios/Dorf/Info.plist`: `ITSAppUsesNonExemptEncryption` = `false`
  (Schritt 6).
- `ios/Dorf/Bereiche/Rechtliches/RechtlichesLeiste.swift`: Link auf
  <https://xn--rssing-wxa.de/app/daten-loeschen/> für Guideline 5.1.1(v).
- `.github/workflows/store.yml`: `python3 store/check_ios_metadata.py`
  als zweiten Prüfschritt neben `check_metadata.py` aufnehmen.
- Ein iOS-Release-Workflow, der mit den drei Secrets aus Schritt 4 baut und
  hochlädt — es gibt bislang nur `android.yml` und `release.yml`.
