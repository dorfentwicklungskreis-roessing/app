# Veröffentlichung im App Store — Anleitung

Geschrieben für jemanden, der das zum ersten Mal macht. Alles, was im Repo
liegen kann, liegt in `store/`; was hier steht, geht nur mit einem
Apple-Developer-Konto und muss von Hand gemacht werden.

**Ausgangslage** (Stand 26.08.2026)

| | | |
|---|---|---|
| Apple-ID | `post+apple@levinkeller.de` | |
| Apple Developer Program | abgeschlossen (99 $/Jahr) | ✅ |
| Team-ID | `SK8G83Y72R` — in `ios/project.yml` als `DEVELOPMENT_TEAM` und als GitHub-Secret `APPLE_TEAM_ID` | ✅ |
| Bundle-ID | `de.roessing.app`, id `K59697H99T`, Plattform `UNIVERSAL` | ✅ |
| App-Datensatz | App-ID **6805373613**, Name „Rössing", SKU `de-roessing-app`, Primärsprache `de-DE` | ✅ |
| App-Store-Connect-API-Schlüssel | angelegt; die drei GitHub-Secrets sind gesetzt | ✅ |
| App-Name im Store | **Rössing** (`store/metadata/ios/de-DE/name.txt`) | |
| Anzeigename auf dem Home-Bildschirm | **Rössing** (`CFBundleDisplayName` in `ios/Dorf/Info.plist`) | |
| Version | `MARKETING_VERSION 0.1.0` (`ios/project.yml`); die Buildnummer setzt die CI | |
| Signierzertifikat / Profil | **fehlt noch** — legt der Auslieferungs-Workflow beim ersten Lauf selbst an | ⬜ |
| Screenshots | **fehlen noch** | ⬜ |
| APNs-Schlüssel | **fehlt noch** (Push ist in der App noch nicht gebaut) | ⬜ |

Nachsehen lässt sich der Stand jederzeit — der Befehl fragt App Store Connect
und gibt App-ID, Store-Versionen, Builds und TestFlight-Gruppen aus:

```sh
export APP_STORE_CONNECT_KEY_ID=…
export APP_STORE_CONNECT_ISSUER_ID=…
python3 store/asc.py app-zeigen
```

Die beiden Apple-Portale heißen ähnlich und werden gern verwechselt:

- **developer.apple.com/account** — Mitgliedschaft, Zertifikate, Bundle-IDs.
  Hier wird die App *technisch* angemeldet.
- **appstoreconnect.apple.com** — der Store-Eintrag, TestFlight, die Prüfung.
  Hier wird die App *veröffentlicht*.

Die Reihenfolge unten ist die Reihenfolge, in der es gemacht werden muss.
Die Schritte 1 bis 4 sind inzwischen **erledigt** und stehen nur noch als
Beleg da — wer heute weitermacht, fängt bei **Schritt 5 (TestFlight)** an.
Was ein Mensch noch tun muss, steht am Ende gebündelt.

---

## 1. Team-ID — erledigt

Die Team-ID ist `SK8G83Y72R`. Sie steht in `ios/project.yml` als
`DEVELOPMENT_TEAM` und zusätzlich als GitHub-Secret `APPLE_TEAM_ID` — der
Auslieferungs-Workflow nimmt das Secret, damit der Wert an genau einer Stelle
änderbar bleibt, und fällt auf `project.yml` zurück, wenn es fehlt.

Sie ist **kein Geheimnis**: Sie steckt in jeder signierten App und in jedem
Provisioning-Profil. Nachzulesen auf
<https://developer.apple.com/account> → **Membership details**.

Für den Simulator bleibt in `ios/project.yml` die Ad-hoc-Signatur stehen
(`CODE_SIGN_IDENTITY: "-"`, `CODE_SIGNING_REQUIRED: NO`) — ohne irgendeine
Signatur startet die App im Simulator nicht (siehe `CLAUDE.md`). Für den
Archivlauf übersteuert `.github/workflows/ios-release.yml` das auf der
Kommandozeile:

```
DEVELOPMENT_TEAM=… CODE_SIGN_STYLE=Automatic
CODE_SIGN_IDENTITY="Apple Distribution"
CODE_SIGNING_REQUIRED=YES CODE_SIGNING_ALLOWED=YES
```

So bleibt `project.yml` für alle anderen Zwecke unangetastet.

---

## 2. App ID (Bundle-ID) — erledigt

`de.roessing.app` ist registriert (id `K59697H99T`, Plattform `UNIVERSAL`),
ohne Capabilities. Das ist so gewollt: Die App braucht heute keine.

- **Push Notifications** — noch nicht. Push ist nach der ersten Fassung
  vorgesehen (`ios/OFFEN.md`); im ganzen Verzeichnis `ios/` gibt es keinen
  Aufruf von `UNUserNotificationCenter`. Wenn es kommt, wird hier
  *Push Notifications* angehakt, dazu gehört ein **APNs-Key**
  (Keys → ＋ → *Apple Push Notifications service*) und ein
  `aps-environment`-Entitlement — siehe die Warnung in Schritt 5.
- **Sign in with Apple** — nein, Begründung in Schritt 7.
- **Associated Domains**, **App Groups**, **HealthKit**, **In-App Purchase**,
  **Maps** — alles nein. Die Karte ist MapLibre mit OpenFreeMap-Kacheln, nicht
  Apple Maps. Die Anmeldung läuft über `ASWebAuthenticationSession` mit dem
  eigenen URL-Schema (`CFBundleURLTypes` in `ios/Dorf/Info.plist`) — eine
  Info.plist-Angabe, keine Capability.

Falls die ID je neu angelegt werden muss (anderes Team, anderer Bezeichner),
geht das ohne Klicken:

```sh
python3 store/asc.py bundle-id-anlegen
# → de.roessing.app gibt es schon (id K59697H99T) — nichts zu tun.
```

Der Befehl sucht erst und legt nur an, was fehlt. Das ist Absicht: Eine
Bundle-ID lässt sich weder umbenennen noch löschen.

---

## 3. App-Datensatz in App Store Connect — angelegt

Der Datensatz existiert: **App-ID 6805373613**, Name „Rössing", SKU
`de-roessing-app`, Primärsprache Deutsch (Deutschland), Bundle-ID
`de.roessing.app`. Dafür gibt es bewusst keine API — einen App-Datensatz legt
nur ein Mensch an.

In App Store Connect steht bereits eine Version **1.0** im Zustand
*PREPARE_FOR_SUBMISSION*. `ios/project.yml` trägt dagegen
`MARKETING_VERSION 0.1.0`. **Für TestFlight ist das egal** — ein Build bringt
seine Versionsnummer selbst mit und erscheint unter ihr. Vor der
Store-Einreichung müssen die beiden Zahlen aber zusammenpassen: entweder
`MARKETING_VERSION` auf `1.0` heben oder in App Store Connect eine Version
`0.1.0` anlegen.

### Was noch auszufüllen ist

Alles unter **App Store** (Reiter oben) bzw. der linken Spalte. Die Inhalte
liegen im Repo, sie müssen nur hinüber:

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
| *App Information* → **Category** | Primary: **Utilities**, Secondary: **Lifestyle** — **gesetzt**, `python3 store/asc.py kategorien` (Begründung in Schritt 11) |
| *App Information* → **Content Rights** | **„Enthält Inhalte Dritter"** — gesetzt; die Karte zeigt OpenStreetMap-Daten (Schritt 11). Die frühere Angabe „keine" war falsch. |
| *Age Rating* → Fragebogen | **beantwortet**, Ergebnis 4+ — `python3 store/asc.py alterseinstufung`. Apple hat den Fragebogen 2025 umgebaut; die heutigen Felder und die Belege dazu stehen in Schritt 11, nicht mehr in `store/content-rating.md` (das ist der IARC-Bogen für Play) |
| **App Privacy** | `store/ios-datenschutz.md` — je Datenart erhoben/verknüpft/Tracking. **Nur von Hand:** Für die Datenschutzangaben gibt es keine API (Schritt 11) |
| *Pricing and Availability* | **Free**, weltweit — **gesetzt**, `python3 store/asc.py verfuegbarkeit` (Begründung in Schritt 11) |
| **App Screenshots** | **fehlen noch** — mindestens einer für 6,9″ iPhone (1290×2796 oder 1320×2868). Apple rechnet daraus die kleineren Größen selbst |

Die Zeichengrenzen sind nachgerechnet, nicht geschätzt:

```sh
python3 store/check_ios_metadata.py
```

Das Skript prüft Vollständigkeit beider Sprachen, alle Grenzen (Name 30,
Untertitel 30, Schlüsselwörter 100, Werbetext 170, Beschreibung 4000,
Neuerungen 4000), die URL-Felder und das App-Icon. Es läuft ohne
Fremdbibliotheken und ist der erste Schritt des Auslieferungs-Workflows.

---

## 4. App-Store-Connect-API-Schlüssel — vorhanden

Der Schlüssel existiert (Rolle **App Manager**), und die drei GitHub-Secrets
im Repo `dorfentwicklungskreis-roessing/app` sind gesetzt:

| Name | Inhalt |
|---|---|
| `APP_STORE_CONNECT_ISSUER_ID` | die Issuer ID (UUID, für das ganze Team dieselbe) |
| `APP_STORE_CONNECT_KEY_ID` | die Key ID (10 Zeichen) |
| `APP_STORE_CONNECT_PRIVATE_KEY` | der **komplette Inhalt** der `.p8`-Datei, einschließlich `-----BEGIN PRIVATE KEY-----` und `-----END PRIVATE KEY-----` |
| `APPLE_TEAM_ID` | `SK8G83Y72R` — kein Geheimnis, aber so an einer Stelle änderbar |

Damit tut der Auslieferungs-Workflow zweierlei mit **einem** Schlüssel:
`xcodebuild` weist sich damit aus, um Zertifikat und Provisioning-Profil
automatisch anzulegen (`-allowProvisioningUpdates`), und `xcrun altool`
lädt damit hoch. Kein zweites Geheimnis, kein Schlüsselbund-Gefummel, keine
Passwörter in Secrets.

### Auf dem eigenen Rechner

Die `.p8` gehört **nicht ins Repo**, sondern dorthin, wo Xcode und `altool`
von selbst nachsehen:

```sh
mkdir -p ~/.appstoreconnect/private_keys
cp AuthKey_<KEY_ID>.p8 ~/.appstoreconnect/private_keys/
chmod 600 ~/.appstoreconnect/private_keys/AuthKey_<KEY_ID>.p8
export APP_STORE_CONNECT_KEY_ID=<KEY_ID>
export APP_STORE_CONNECT_ISSUER_ID=<UUID>
```

> **Die `.p8`-Datei lässt sich genau einmal herunterladen.** Danach ist der
> Knopf weg — für immer. Geht sie verloren, hilft nur: widerrufen („Revoke")
> und einen neuen anlegen. Sie ist ein Passwort-Äquivalent: nicht ins Repo,
> nicht in eine Chat-Nachricht, sondern in den Passwortspeicher.

Im Workflow wird sie aus dem Secret in genau diese Datei geschrieben
(`chmod 600`) und am Ende jedes Laufs wieder gelöscht — auch nach einem
Fehlschlag.

### Was `store/asc.py` damit kann

```sh
python3 store/asc.py app-zeigen          # App-ID, Zustand, Builds, Gruppen
python3 store/asc.py bundle-id-anlegen   # de.roessing.app registrieren
python3 store/asc.py testflight-gruppe   # externe Gruppe „Dorf" + öffentlicher Link
python3 store/asc.py beta-info           # Feedback-Adresse und Beta-Beschreibung
python3 store/asc.py team-id             # Team-ID aus einem Zertifikat lesen
python3 store/asc.py GET '/v1/apps?limit=10'   # alles Übrige von Hand
```

Zwei Dinge dazu:

* **`--probe` zeigt erst, was passieren würde.** Jeder schreibende
  Unterbefehl gibt mit diesem Schalter nur Methode, Pfad und Rumpf aus und
  schickt nichts. Die Objekte gehören einem echten Verein — wer einen Befehl
  zum ersten Mal aufruft, sieht damit vorher hinein.
* **`team-id` funktioniert erst, wenn ein Zertifikat existiert.** Die API hat
  kein Feld für die Team-ID; das Skript liest sie aus dem `OU`-Feld eines
  Zertifikats. Solange keins da ist, meldet es das und verweist auf die
  Mitgliedschaftsseite.

---

## 5. TestFlight

### Einen Lauf auslösen

Die Auslieferung macht `.github/workflows/ios-release.yml`. **Getaggt und
ausgelöst wird von Hand** — dieselbe Haltung wie bei Android (siehe
`README.md`, „Releases (Android)").

```sh
# 1. Stand prüfen: der iOS-Workflow muss für diesen Commit grün sein
gh run list --workflow=ios.yml --limit 3

# 2. Metadaten prüfen (macht der Workflow auch, aber lieber vorher)
python3 store/check_ios_metadata.py

# 3. Taggen und pushen
git tag ios-v0.1.0 && git push origin ios-v0.1.0

# Der Tag-Push startet den Workflow normalerweise selbst. Passiert nach
# ~1 Minute nichts (kommt bei Pushes aus Workflows vor):
gh workflow run ios-release.yml --ref ios-v0.1.0
gh run watch "$(gh run list --workflow=ios-release.yml --limit 1 \
  --json databaseId --jq '.[0].databaseId')"
```

Ohne Tag geht es auch: *Actions → iOS-Auslieferung → Run workflow*. Zwei
Eingaben stehen dort:

- **`bauzahl`** — die `CFBundleVersion` für diesen Lauf. Leer lassen für die
  Vorgabe.
- **`hochladen`** — auf `false` bleibt es beim Archivieren und Exportieren.
  Damit lässt sich die Signierung prüfen, ohne eine Buildnummer zu verbrauchen.

**Das Tag-Muster ist `ios-v*`, nicht `v*`.** Android hängt an `v*`; die beiden
überschneiden sich nicht, ein iOS-Tag löst also keinen Play-Upload aus und
umgekehrt.

### Die Buildnummer steigt automatisch

Apple nimmt jede `CFBundleVersion` zu einer Marketing-Version **genau einmal**
an. Der Workflow setzt sie deshalb auf die **Zahl der Commits**
(`git rev-list --count HEAD`) — monoton mit dem Trunk, reproduzierbar, im
Protokoll nachlesbar, ohne dass jemand eine Datei anfassen muss.

Warum das hier anders läuft als der `versionCode` auf Android, der von Hand
gezählt wird: Dort steht die Zahl in einer committeten Datei, ist die
öffentliche Aktualisierungs-Reihenfolge im Play Store und gehört zu einer
Änderungshinweis-Datei, die **nach ihr benannt** ist —
`store/check_metadata.py` erzwingt das Paar. Eine maschinell erzeugte Zahl
zerrisse das. `CFBundleVersion` dagegen ist bloß ein Zähler *innerhalb* einer
Marketing-Version, es hängt kein Text daran, und sie steht in
`ios/project.yml` — einer Datei, die für ein Release niemand anfassen soll.

Soll derselbe Stand ein zweites Mal hochgeladen werden (der erste Versuch ist
in der Verarbeitung gescheitert), wäre die Commit-Zahl dieselbe und Apple
lehnte ab. Dafür gibt es die Eingabe `bauzahl`.

### Erst der Zustand, dann der Upload

Der Workflow schlägt zu Beginn den App-Datensatz nach
(`python3 store/asc.py app-zeigen`). Fehlt er, wird der Upload **übersprungen
statt rot**, mit einem Hinweis, wo er anzulegen ist — genauso, wie
`release.yml` es ohne `PLAY_SERVICE_ACCOUNT_JSON` hält. Dasselbe gilt für
fehlende Secrets; archiviert wird dann trotzdem, unsigniert, damit ein
kaputter Release-Bau auffällt.

### Interne Tester — der schnelle Weg

*TestFlight* → linke Spalte **Internal Testing**. Im Konto steht bereits eine
interne Gruppe **„Testerinnen"**.

- Interne Tester sind **Mitglieder des eigenen App-Store-Connect-Teams**. Wer
  noch keins ist, wird vorher unter *Users and Access* eingeladen (Rolle
  *Developer* reicht; die Person braucht eine Apple-ID).
- Bis **100 interne Tester**, je bis zu 30 Geräte.
- **Keine Beta App Review.** Apple prüft nur automatisch: gültige Signatur,
  vollständige Info.plist, keine verbotenen APIs — und die
  Export-Compliance-Angabe (Schritt 6). Sobald der Build „Processing"
  verlassen hat, ist er installierbar.
- Das ist der Weg, um die App in den nächsten Tagen aufs eigene Telefon zu
  bekommen.

### Externe Tester — was zusätzlich nötig ist

*TestFlight* → **External Testing**. Der Unterschied in einem Satz: **extern
heißt ohne Team-Zugang.** Wer eingeladen wird, braucht keine Rolle in App
Store Connect, sondern nur die TestFlight-App — dafür schaut einmal ein Mensch
bei Apple auf den Build.

| | intern | extern |
|---|---|---|
| Wer | Mitglieder des App-Store-Connect-Teams | jede Person mit Apple-ID |
| Plätze | 100 (je 30 Geräte) | **10.000** |
| Beta App Review | nein | **ja**, beim ersten Build und bei jeder neuen Versionsnummer |
| Öffentlicher Link | nein | **ja** |

Für die externe Gruppe samt öffentlichem Link gibt es einen Befehl:

```sh
python3 store/asc.py testflight-gruppe --probe   # erst zeigen
python3 store/asc.py testflight-gruppe           # dann anlegen
```

Er legt die externe Gruppe **„Dorf"** an, schaltet `publicLinkEnabled` ein
(ohne Platzdeckel — 10.000 reichen für ein Dorf mit rund 1.500 Einwohnern) und
gibt die URL aus. Der Link entsteht bei Apple asynchron und **trägt erst,
wenn ein Build die Beta-Prüfung bestanden hat**.

Was die **erste** Beta App Review verlangt:

1. **Zeit.** Meist ein bis zwei Werktage; beim allerersten Build einer App
   auch länger. Danach gehen reine Buildnummer-Erhöhungen innerhalb derselben
   Version meist automatisch durch.
2. **Test Information** (*TestFlight → Test Information*) — Pflicht:
   - **Beta App Description**: `store/metadata/ios/<sprache>/beta_description.txt`
   - **Feedback Email**: `post@levinkeller.de`
   - **Marketing URL** und **Privacy Policy URL** (dieselben wie im Store)

   Setzt der Auslieferungs-Workflow nach jedem Upload selbst
   (`python3 store/asc.py beta-info`), für beide Sprachen. Von Hand:

   ```sh
   python3 store/asc.py beta-info --probe
   python3 store/asc.py beta-info
   ```
3. **Ein funktionierendes Prüfkonto** — ohne das wird abgelehnt. Es
   existiert (`apple.review`, siehe unten), das Passwort trägt ein Mensch
   unter *Test Information → Sign-in Information* ein. **Nicht ins Repo.**
4. **Export-Compliance** (Schritt 6) muss beantwortet sein.

**Empfehlung:** erst intern testen, bis die App steht. Externe Tester lohnen
sich, sobald mehr als der eigene Kreis mitmachen soll — und der Aufwand ist
dann derselbe wie eine echte Store-Prüfung.

Ein Build läuft **90 Tage** ab dem Hochladen; danach müssen Tester einen
neuen bekommen.

### ⚠️ Push in TestFlight: das APNs-Umfeld ist die Falle

Push gibt es in der iOS-App **noch nicht** (`ios/OFFEN.md`). Wenn es kommt,
ist das hier der Punkt, an dem sonst stundenlang gerätselt wird — deshalb
steht er jetzt schon da:

**Ein APNs-Gerätetoken gilt nicht überall.** Es gehört entweder zum
**Sandbox**- oder zum **Produktions**-APNs, und welches, entscheidet das
`aps-environment`-Entitlement des Provisioning-Profils, mit dem der
installierte Build signiert wurde:

| Wie installiert | Profil | APNs-Umfeld | Server spricht mit |
|---|---|---|---|
| aus Xcode aufs Gerät | Development | **Sandbox** | `api.sandbox.push.apple.com` |
| über TestFlight | App-Store-Distribution | siehe Warnung | `api.push.apple.com` |
| aus dem App Store | App-Store-Distribution | Produktion | `api.push.apple.com` |

> **Der weitverbreitete Merksatz lautet: „TestFlight-Builds bekommen
> Sandbox-Tokens, keine Produktions-Tokens."** Er stammt aus älteren
> Apple-Unterlagen und hält sich hartnäckig; neuere Beschreibungen sagen für
> TestFlight das Produktions-Umfeld. **Verlass dich auf keine der beiden
> Aussagen, sondern prüfe es beim ersten Push-Test nach** — der Fehler sieht
> in beiden Richtungen gleich aus: Der Server bekommt von Apple
> `BadDeviceToken` (bzw. FCM meldet `Unregistered`/`InvalidRegistration`),
> obwohl Token und Schlüssel richtig sind. Auf dem Gerät kommt schlicht
> nichts an.

Praktische Folgen für dieses Projekt:

- Der Server pusht über **Firebase Cloud Messaging** (`backend/internal/push`,
  HTTP v1). Firebase braucht dafür den **APNs-Auth-Key** (`.p8`) — derselbe
  Schlüssel deckt **beide** Umfelder ab, Firebase wählt das passende. Der
  Schlüssel existiert noch nicht.
- Beim ersten Test also: eine Meldung an ein TestFlight-Gerät schicken und
  **nachsehen, was FCM antwortet**, statt zu vermuten. Kommt nichts an,
  ist das Umfeld der erste Verdacht — nicht der Code.
- Ein Gerätetoken aus einem TestFlight-Build und eines aus einem
  Xcode-Build sind **nicht austauschbar**. Wer beim Debuggen zwischen beiden
  wechselt, muss das Gerät neu registrieren lassen.

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

**Der Eintrag steht inzwischen in `ios/Dorf/Info.plist`** — damit fragt Apple
nicht mehr, und ein hochgeladener Build ist sofort verteilbar, statt auf eine
Antwort zu warten. Beim ersten echten Upload lohnt trotzdem ein Blick unter
*TestFlight → Builds → [Build]*: Steht dort „Missing Compliance", ist der
Schlüssel nicht in der ausgelieferten Info.plist angekommen.

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

### Das Konto existiert — das Passwort fehlt in App Store Connect

Angelegt ist es (Einzelheiten am Ende dieser Datei): Anmeldename
`apple.review`, Rolle `member` im Zitadel-Projekt `dorf-app`, kein
Passwortwechsel, keine Zwei-Faktor-Pflicht. Bewusst getrennt von `test-dorf`
(automatische Tests) und `google-reviewer` (Play): drei Konten, drei Zwecke.

**Offen bleibt der Handgriff, den nur ein Mensch tun kann:** das Passwort aus
dem Passwortspeicher in App Store Connect eintragen — für TestFlight unter
*Test Information → Sign-in Information*, für die Store-Prüfung unter
*App Review Information*. Ins Repo gehört es nicht (`CLAUDE.md`, „Keine
Secrets committen").

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

1. Build hochladen (Schritt 5), intern testen, Fehler beheben, neu
   hochladen. Die Buildnummer erhöht der Workflow selbst — von Hand ist da
   nichts zu zählen.
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

## 11. Store-Eintrag über die API vervollständigt — Stand 26.08.2026

Alles, was sich über die App-Store-Connect-API setzen ließ, steht. Die
Handgriffe sind Unterbefehle von `store/asc.py` und lassen sich jederzeit
wiederholen; jeder versteht `--probe`.

```sh
export APP_STORE_CONNECT_KEY_ID=…
export APP_STORE_CONNECT_ISSUER_ID=…

python3 store/asc.py einreichstand      # zuerst: was fehlt noch?
python3 store/asc.py kategorien
python3 store/asc.py alterseinstufung
python3 store/asc.py version-angaben
python3 store/asc.py verfuegbarkeit
PRUEFKONTO_PASSWORT='…' python3 store/asc.py pruefangaben
```

### Was jetzt gesetzt ist

| Feld | Wert | Warum genau der |
|---|---|---|
| Kategorie primär | **Utilities** (Dienstprogramme) | Die App ist ein Werkzeug für eine Arbeit: Welcher Blumenkasten ist fällig, wer hat gegossen, wann ist der nächste Termin. |
| Kategorie sekundär | **Lifestyle** | Die Schublade für Haus, Garten und Nachbarschaft. |
| — nicht gewählt: | ~~Social Networking~~ | Klang naheliegend, wäre aber eine Falschaussage: Es gibt keinen Chat, keine Kommentare, keinen Beitragsstrom und kein Teilen — nur ein Verzeichnis der angemeldeten Dorfbewohner. Die Kategorie hätte etwas versprochen, was die App nicht kann, und die Prüfung auf Richtlinie 1.2 unnötig scharf gestellt. |
| Content Rights | **Enthält Inhalte Dritter** (`USES_THIRD_PARTY_CONTENT`) | Korrektur der früheren Angabe in Schritt 3: Die Karte zeigt OpenStreetMap-Daten über OpenFreeMap (`Konfiguration.kartenstil`), MapLibre blendet dafür den Attributionsknopf ein (`MapLibreKarte.swift`), und Termine können auf Seiten fremder Veranstalter zeigen (`external` in `events.json`). Die Rechte sind da (ODbL mit sichtbarer Namensnennung) — „keine Inhalte Dritter" wäre trotzdem gelogen gewesen. |
| Alterseinstufung | **4+** (`FOUR_PLUS`) | Fragebogen vollständig beantwortet, siehe unten. |
| Copyright | `2026 Dorfentwicklungskreis Rössing` | |
| Freigabeart | **manuell** (`MANUAL`) | Apple gäbe eine bestandene Prüfung sonst sofort frei — auch nachts um drei. Die App nützt aber erst, wenn im Dorf bekannt ist, dass es sie gibt, und die Rössing-IDs verteilt sind. Der Dorfentwicklungskreis drückt den Knopf, wenn der Aushang hängt. |
| Werbekennung (IDFA) | **nein** | Einzige Fremdbibliothek ist MapLibre (`ios/project.yml`). |
| Preis | **kostenlos**, Basisland Deutschland | |
| Länder | **alle 175**, neue automatisch | Begründung unten. |
| Prüfangaben | Kontakt Levin Keller, `post@levinkeller.de`, `+4915156041082`; Prüfkonto `apple.review`, Anmeldung erforderlich; Notiz deutsch **und** englisch (2087 Zeichen) | Zweisprachig, weil die Prüfung nicht zwingend in Deutschland sitzt. |
| Build | Nr. 1 der Version 1.0 zugeordnet | ⚠ siehe offener Punkt 3 |

Das Passwort des Prüfkontos ist über die API gesetzt, steht aber **nicht im
Repo**: `pruefangaben` liest es aus `PRUEFKONTO_PASSWORT` und bricht ohne die
Variable mit einer Anleitung ab. Im Trockenlauf wird es durch Sterne ersetzt,
damit es nicht im Terminal oder in einem Workflow-Protokoll landet. Damit ist
Punkt 5 der Liste „Was der Mensch noch tun muss" erledigt.

### Weltweit oder nur Deutschland?

**Weltweit**, und neue Länder kommen von selbst dazu. Die Überlegung dahinter:

Die Absperrung dieser App ist die Rössing-ID, nicht der Ländershop. Ohne
Konto kommt niemand über den Anmeldebildschirm — egal, aus welchem Store die
App geladen wurde. Eine Beschränkung auf Deutschland schützt also nichts.

Sie kostet aber etwas: Welcher Store einem offensteht, hängt am Wohnsitz des
Apple-Kontos, nicht am Aufenthaltsort. Wer aus Rössing stammt und im Ausland
lebt, wer zugezogen ist und sein Apple-Konto behalten hat, wer für ein Jahr
weg ist — die alle hätten die Dorf-App nicht laden können. Für ein Dorf, das
seine Leute zusammenhalten will, ist das die falsche Richtung.

### Die Alterseinstufung, Frage für Frage

Apple hat den Fragebogen 2025 umgebaut; die hier beantworteten Felder sind die
von heute (über `/v1/appInfos/<id>/ageRatingDeclaration` geprüft, nicht aus
dem Gedächtnis). Alle Inhaltsfragen — Gewalt, Waffen, Sexualität, Schimpfwörter,
Alkohol/Tabak/Drogen, Horror, medizinische Auskünfte, simuliertes Glücksspiel,
Wettbewerbe — stehen auf `NONE`. Glücksspiel, Lootboxen, Werbung, Chat,
soziale Medien, Gesundheitsthemen, Kindersicherung und Altersprüfung stehen
auf **nein**. Zwei Antworten verdienen eine Begründung:

**Nutzergenerierte Inhalte: ja.** Anzeigename, Spitzname und Notiz im Profil
sind freie Texte (`ProfilView.swift`), und wer ein Feld auf „öffentlich"
stellt, zeigt es allen angemeldeten Dorfbewohnern (`DorfbewohnerView.swift`).
Auch die Verwaltung legt Orts- und Aufgabennamen als Text an. Wenig, aber
Nutzerinhalt.

Nicht mitgezählt, obwohl es zunächst danach aussieht:

- **Die Erledigungsnotiz.** `DorfApi.melden` kennt zwar ein Feld `notiz`, die
  Oberfläche schickt aber ausnahmslos den leeren String — `OrteModell.melden`
  ruft `quelle.melden(aufgabe.id, aufgabe.liters, "")`. In der App gibt es
  kein Eingabefeld dafür. (`store/content-rating.md` sagt dasselbe für Play.)
- **Die eingereichte Idee.** Der Text aus „Idee vorschlagen" geht an den
  Dorfentwicklungskreis und an sonst niemanden; kein anderes Konto bekommt
  ihn in der App zu sehen (`IdeenView.swift`).

**Unbeschränkter Web-Zugriff: nein.** Im ganzen Quelltext gibt es kein
`WKWebView` und kein `SFSafariViewController` — die App hat keinen eingebauten
Browser. Was sie öffnet, sind `Link`-Ziele, und die übergibt iOS an Safari,
wo Bildschirmzeit und Beschränkungen greifen. Geöffnet werden:

| Wohin | Wo im Code |
|---|---|
| Impressum, Datenschutzerklärung auf rössing.de | `RechtlichesLeiste.swift` |
| Rössing-ID (`id.rössing.de`) | `EinstellungenView.swift` |
| `tel:` und `mailto:` aus dem Bewohnerverzeichnis | `Profilstand.swift` (`Kontakt`) |
| iOS-Systemeinstellungen (Ortungsfreigabe) | `KarteView.swift` |
| **die Adresse eines Termins** | `VeranstaltungenView.swift`, `Termine.swift` |

Der letzte Punkt ist der einzige, der nicht auf eine feste Liste zeigt: Steht
ein Termin in der `events.json` der Dorf-Website als `external`, führt der
Tipp auf die Seite des Veranstalters — eine beliebige fremde Adresse. Ein
Surfbrett ist das trotzdem nicht: Der Sprung endet in Safari, nicht in der
App, und die Liste der Adressen pflegt das Dorf auf seiner eigenen Website.
Deshalb **nein** — mit dieser Fundstelle als Beleg, falls Apple nachfragt.

### Was zur Einreichung noch fehlt

1. **Screenshots**, de-DE und en-US, mindestens einer für 6,9″ iPhone
   (1290×2796 oder 1320×2868). Beide Sprachen haben derzeit **null**
   Screenshot-Sätze. Daran arbeitet ein anderer; `store/screenshots/**` ist
   deshalb hier nicht angefasst worden.
2. **App Privacy** — die Datenschutzangaben je Datenart. **Das geht nicht über
   die API**: `appDataUsages`, `appDataUsagePublishState` und
   `appPrivacyDeclarations` gibt es in der öffentlichen ASC-API schlicht
   nicht (alle drei antworten mit „The resource does not exist"). Das muss
   ein Mensch in App Store Connect klicken — die Antworten liegen fertig in
   `store/ios-datenschutz.md`, Abschnitte 2 und 3.
3. **Versionsnummern zusammenbringen.** Der einzige Build trägt die
   Marketing-Version **0.1.0** (`ios/project.yml`), der Store-Eintrag heißt
   **1.0**. Die API hat die Zuordnung angenommen, aber im Store stünde dann
   eine andere Zahl als in der App. Zwei Wege, beide brauchen eine Änderung
   außerhalb dieses Arbeitsstands:
   - `MARKETING_VERSION` in `ios/project.yml` auf `1.0` heben und einen neuen
     Build hochladen — die erste öffentliche Fassung heißt dann 1.0, oder
   - den Store-Eintrag auf `0.1.0` umbenennen
     (`PATCH /v1/appStoreVersions/<id>` mit `versionString`) — dann heißt die
     erste Fassung ehrlich 0.1.0.
   Das ist eine Entscheidung, keine Einstellung; sie ist hier bewusst nicht
   getroffen worden.
4. **Händlerstatus (Trader Status)** in App Store Connect unter *Business →
   App Information* nachsehen. Seit Februar 2025 verlangt die EU-Verordnung
   über digitale Dienste diese Angabe für jede App, die in der EU verkauft
   oder verteilt wird; ohne sie verschwindet die App aus den EU-Stores. Über
   die API ist das Feld **nicht abfragbar**, deshalb steht hier nur der
   Hinweis und keine Feststellung.
5. **„Add for Review" drücken.** Der letzte Knopf ist bewusst nicht gedrückt
   worden: *App Store → Version 1.0 → Add for Review → Submit for Review*.

Was **nicht** mehr fehlt und in der Liste „Was der Mensch noch tun muss"
weiter unten überholt ist: die Store-Texte (beide Sprachen stehen im
Eintrag), das Passwort des Prüfkontos (über `pruefangaben` gesetzt) und die
Export-Compliance (der Build meldet `usesNonExemptEncryption: false`).

### Bewusst offengelassen

- **Nicht eingereicht.** Siehe Punkt 5.
- **`kidsAgeBand` leer** — die App gehört nicht in die Kinderkategorie
  (`isOrEverWasMadeForKids` steht bei Apple auf `false`).
- **`developerAgeRatingInfoUrl` leer** — freiwillig; es gibt keine Seite, die
  die Einstufung zusätzlich erklärt.
- **Barrierefreiheits-Etikett** (`accessibilityDeclarations`) leer — seit 2025
  freiwillig. Die App ist durchgehend mit `accessibilityLabel` und
  `accessibilityHint` versehen; ob sie Apples Kriterien in voller Breite
  erfüllt, ist ohne echtes Gerät nicht zu behaupten.
- **„Neu in dieser Version" (`whatsNew`) leer** — bei einer ersten Version
  verlangt Apple das Feld nicht.

### Ein Risiko, das die Zahlen nicht zeigen

Weil „nutzergenerierte Inhalte: ja" stimmt, kann die Prüfung Richtlinie 1.2
anlegen: Apps mit Nutzerinhalten sollen eine Möglichkeit haben, Inhalte zu
melden und Nutzer zu blockieren. **Beides gibt es in der App nicht** — im
Quelltext findet sich keine Melde- und keine Blockierfunktion.

Dagegen steht, was ebenfalls stimmt und in der Prüfnotiz erwähnt ist: Konten
vergibt der Dorfentwicklungskreis von Hand, es gibt keine Selbstregistrierung,
die Inhalte sind auf das eigene Profil beschränkt und nur für angemeldete
Dorfbewohner sichtbar. Das ist eine gute Antwort, falls Apple fragt — aber
eben eine Antwort auf eine Frage, die kommen kann. Wer sie nicht riskieren
will, baut vorher einen Melde-Weg ein (und sei es ein `mailto:` an den
Dorfentwicklungskreis).

---

## Was der Mensch noch tun muss

Der Stand von heute, in der Reihenfolge, in der es sinnvoll ist. Alles
darüber Hinausgehende macht die CI.

1. **Einen TestFlight-Lauf auslösen** und ihn zu Ende sehen (Schritt 5).
   Das ist der Schritt, der die noch fehlenden Zertifikate und
   Provisioning-Profile bei Apple **selbst anlegt** — dafür ist der Lauf
   gedacht. Beim ersten Mal lohnt `hochladen: false`: Dann wird nur
   signiert, archiviert und exportiert, ohne eine Buildnummer zu verbrauchen.
2. **Export-Compliance nachsehen** (Schritt 6). Steht am ersten Build
   „Missing Compliance", ist `ITSAppUsesNonExemptEncryption` nicht in der
   ausgelieferten Info.plist angekommen.
3. **Screenshots aufnehmen und hochladen** — mindestens einer für 6,9″
   iPhone (1290×2796 oder 1320×2868). Der Simulator genügt dafür
   (`xcrun simctl io <id> screenshot …`, siehe `CLAUDE.md`). Ohne sie geht
   keine Store-Einreichung; für TestFlight sind sie nicht nötig.
4. ~~**Store-Texte in App Store Connect eintragen**~~ — steht drin, beide
   Sprachen (Schritt 11).
5. ~~**Passwort des Prüfkontos `apple.review` eintragen**~~ — über
   `python3 store/asc.py pruefangaben` gesetzt; das Passwort kommt dabei aus
   `PRUEFKONTO_PASSWORT` und nicht aus dem Repo (Schritt 11).
6. **Auf einem echten iPhone durchgehen.** Hier gibt es keins
   (`CLAUDE.md`); vor einer Einreichung gehört ein Durchlauf auf Hardware
   dazu.
7. **Externe Tests, falls gewünscht**: `python3 store/asc.py beta-info` und
   `python3 store/asc.py testflight-gruppe` (jeweils erst mit `--probe`),
   danach die Beta App Review abwarten.
8. **Version angleichen** vor der Store-Einreichung: App Store Connect führt
   `1.0`, `ios/project.yml` `0.1.0` (Schritt 3). Der Build ist der Version
   inzwischen zugeordnet, die Zahlen passen aber weiterhin nicht zueinander —
   die beiden Wege stehen in Schritt 11, offener Punkt 3.
9. **APNs-Schlüssel anlegen** — erst, wenn Push in der App wirklich gebaut
   ist. Dann auch die Capability *Push Notifications* an der Bundle-ID
   nachtragen und die Warnung zum APNs-Umfeld in Schritt 5 lesen.

## Was noch am Repo zu tun ist (nicht in diesem Arbeitsstand)

Diese Änderungen gehören in Dateien, an denen gerade andere arbeiten, und
sind hier nur festgehalten:

- ~~`ios/Dorf/Bereiche/Rechtliches/RechtlichesLeiste.swift`: Link auf
  <https://xn--rssing-wxa.de/app/daten-loeschen/> für Guideline 5.1.1(v)~~ —
  **überholt.** Die App löscht das Konto inzwischen selbst: *Einstellungen →
  Konto löschen*, mit Abschreiben des eigenen Namens als Bestätigung
  (`EinstellungenView.swift`, `KontoModell.swift`). Genau das verlangt
  5.1.1(v); die Prüfnotiz nennt den Weg. Ein zusätzlicher Link schadet nicht,
  ist aber nichts, worauf die Einreichung warten muss.
- `.github/workflows/store.yml`: `python3 store/check_ios_metadata.py`
  als zweiten Prüfschritt neben `check_metadata.py` aufnehmen. Der
  Auslieferungs-Workflow prüft die iOS-Metadaten bereits selbst; im
  Store-Workflow fehlen sie noch.

Erledigt, seit diese Datei zuletzt eine Liste hatte:

- ~~`ios/project.yml`: `DEVELOPMENT_TEAM` füllen~~ — steht drin
  (`SK8G83Y72R`), dazu `ASSETCATALOG_COMPILER_APPICON_NAME` und
  `…_GLOBAL_ACCENT_COLOR_NAME`. Die Signier-Einstellungen bleiben absichtlich
  auf Simulator-Werten; der Archivlauf übersteuert sie (Schritt 1).
- ~~`ios/Dorf/Info.plist`: `ITSAppUsesNonExemptEncryption`~~ — steht drin.
- ~~Ein Auslieferungs-Workflow für iOS~~ — das ist
  `.github/workflows/ios-release.yml`.

## Prüfkonto in der Rössing-ID — angelegt

Das Konto für den App-Review **existiert** (angelegt am 26.08.2026):

| | |
|---|---|
| Anmeldename | `apple.review` |
| E-Mail | `post+applereview@levinkeller.de` (als bestätigt hinterlegt) |
| Anzeigename | Apple App Review |
| Rolle | `member` im Zitadel-Projekt `dorf-app` — melden ja, verwalten nein |
| Passwortwechsel | nicht verlangt |
| Zwei-Faktor | nicht verlangt (die Anmelderichtlinie der Organisation erzwingt keine) |

**Das Passwort steht bewusst nicht in diesem Repo** (Regel „Keine Secrets
committen", `CLAUDE.md`). Es gehört an genau zwei Stellen: in den
Passwortspeicher des Betreibers und in App Store Connect unter
*App-Version → App-Prüfungsinformationen → Anmeldedaten erforderlich* mit
Benutzername `apple.review`.

Ohne diese Angabe sieht der Prüfer nur den Anmeldebildschirm und lehnt die
App nach Richtlinie 2.1 ab.

Läuft das Konto irgendwann nicht mehr, ist es in der Rössing-ID unter
*Benutzer → apple.review* zu erneuern — nicht zu löschen, solange eine
Version in Prüfung ist.
