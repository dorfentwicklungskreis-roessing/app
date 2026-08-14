# Veröffentlichung im Google Play Store — Anleitung

Alles, was im Repo liegen kann, liegt in `store/`. Was hier steht, geht nur mit
einem Play-Console-Konto und muss von Levin gemacht werden.

## 0. Kontotyp entscheiden (die wichtigste Weiche)

| | **Privatperson** | **Organisation** |
|---|---|---|
| Nachweis | Ausweis | **D-U-N-S-Nummer** (kostenlos bei Dun & Bradstreet, dauert 1–4 Wochen) + Nachweis der Organisation |
| Pflichttest vor Produktion | **Ja:** geschlossener Test mit **mindestens 12 Testern**, die **14 Tage durchgehend** angemeldet („opted in") bleiben | **Nein** |
| Angezeigter Entwicklername | echter Name der Person | Name des Vereins |
| Vorgeschriebene Kontaktdaten | Adresse muss im Store sichtbar sein | Adresse der Organisation |

Empfehlung: **Organisation**, sobald der Dorfentwicklungskreis eine
D-U-N-S-Nummer hat — dann entfällt die 12-Tester-Hürde und im Store steht der
Verein statt einer Privatperson. Solange die Nummer fehlt und die App schnell in
den internen Test soll: Privatkonto reicht, denn **interne Tests (bis 100
Tester) sind von der 12/14-Regel nicht betroffen**. Die Regel greift erst beim
Schritt in die Produktion.

## 1. Konto anlegen

1. <https://play.google.com/console> → Konto erstellen, Typ nach Schritt 0.
2. **25 US-Dollar** einmalige Gebühr.
3. Identität bzw. Organisation verifizieren (Ausweis / D-U-N-S). Dauert
   Tage bis Wochen — **zuerst erledigen**, alles andere hängt daran.
4. Unter *Kontodetails* eine Kontakt-E-Mail hinterlegen, die auch beantwortet
   wird; Google schreibt Ablehnungen nur dorthin.

## 2. App anlegen

*Alle Apps → App erstellen*:

| Feld | Wert |
|---|---|
| App-Name | `Dorf-App Rössing` |
| Standardsprache | Deutsch (Deutschland) — `de-DE` |
| App oder Spiel | App |
| Kostenlos oder kostenpflichtig | **Kostenlos** (später nicht mehr änderbar) |

Der Paketname `de.roessing.app` entsteht automatisch beim ersten Upload und ist
danach für immer festgelegt.

## 3. Store-Eintrag füllen

Aus `store/metadata/android/de-DE/` übernehmen:

| Play-Feld | Datei |
|---|---|
| App-Name | `title.txt` |
| Kurzbeschreibung | `short_description.txt` |
| Vollständige Beschreibung | `full_description.txt` |
| App-Symbol | `images/icon.png` (512×512) |
| Feature-Grafik | `images/featureGraphic.png` (1024×500) |
| Telefon-Screenshots | `images/phoneScreenshots/` — **fehlen noch**, siehe README dort |
| Kategorie | Tools / Dienstprogramme |
| Tags | Nachbarschaft, Garten (frei wählbar) |

`en-US` ist als Zweitsprache vorbereitet (Minimalfassung, Hinweis dass die App
deutschsprachig ist). Optional — kann auch weggelassen werden.

## 4. App-Inhalte (die lange Checkliste)

Alles unter *App-Inhalte*. Vorbereitet in diesem Verzeichnis:

Zum Durchklicken mit fertigen Antworten: **`store/app-inhalte-klickanleitung.md`**.

- **Datenschutzerklärung** → <https://xn--rssing-wxa.de/app/datenschutz/>
  (verbindliche Fassung; Arbeitsgrundlage dazu: `store/datenschutz.md`).
- **Datenlöschung** → <https://xn--rssing-wxa.de/app/daten-loeschen/>. Pflicht,
  weil die App ein Konto voraussetzt.
- **Datensicherheit** → Antworten stehen in `store/data-safety.md`.
  **Seit `0.1.7` ist „Daten werden geteilt" mit Ja zu beantworten** (Geräte-ID
  an Google/Firebase Cloud Messaging).
- **Altersfreigabe** → Antworten stehen in `store/content-rating.md`. Ebenfalls
  seit `0.1.7` neu abzusenden (Weitergabe an Dritte → Ja).
- **App-Zugriff** → **wichtig:** Die App ist ohne Rössing-ID nicht benutzbar.
  Hier „Alle Funktionen sind eingeschränkt" wählen und dem Prüfteam die
  Zugangsdaten des eigens dafür angelegten Kontos **`google-reviewer`**
  hinterlegen (Passwort in `.env`, Schlüssel `GOOGLE_REVIEWER_PASSWORD`), mit
  einem Satz Erklärung („Anmeldung über den eigenen Identitätsdienst
  id.rössing.de, im Anmeldebildschirm auf *Mit Rössing-ID anmelden* tippen").
  Ohne funktionierendes Testkonto wird die App abgelehnt. Bewusst getrennt vom
  Konto `test-dorf` der automatischen Tests.
- **Werbung**: enthält keine Werbung.
- **Zielgruppe**: 18+ bzw. „nicht für Kinder"; nicht am Familienprogramm
  teilnehmen.
- **Regierungs-App**: nein. **Finanz-App**: nein. **Gesundheits-App**: nein.
- **Steuern/Finanzen**: entfällt bei kostenloser App ohne Käufe.

## 5. Play App Signing (unser Keystore wird Upload-Key)

Beim Anlegen des ersten Release fragt Play nach dem Signaturschlüssel.

- **„Von Google generierten Schlüssel verwenden" wählen** (Standard und
  empfohlen). Google verwaltet dann den App-Signaturschlüssel.
- Der Keystore aus den Repo-Secrets (`KEYSTORE_BASE64`, `KEYSTORE_PASSWORD`,
  `KEY_ALIAS`, `KEY_PASSWORD`) wird dadurch automatisch zum **Upload-Schlüssel**:
  Der Release-Workflow signiert das AAB damit, Play erkennt daran den
  berechtigten Uploader und signiert die Auslieferung selbst neu.
- Folge: Der Keystore darf nicht verloren gehen — sonst muss ein neuer
  Upload-Schlüssel bei Google beantragt werden (geht, dauert aber). Er ist aber
  **nicht** mehr der Schlüssel, mit dem die Nutzer die App bekommen.
- **Nicht** „Vorhandenen App-Signaturschlüssel hochladen" wählen. Dafür gibt es
  keinen Grund, und es nimmt Google die Möglichkeit, den Schlüssel zu ersetzen.

## 6. Ersten Upload von Hand machen

Die Play-API kann keine App neu anlegen, und der erste Upload muss über die
Oberfläche laufen. Also einmal manuell:

1. *Testen → Interner Test → Neues Release erstellen*.
2. Das AAB aus dem GitHub-Release `v0.1.1` hochladen
   (`app-release.aab`, Anhang am Release).
3. Release-Notiz aus `store/metadata/android/de-DE/changelogs/2.txt`.
4. Tester: eigene E-Mail-Liste anlegen und sich selbst eintragen.
5. Speichern → Prüfen → Freigeben.

Ab hier läuft alles Weitere automatisch über die CI.

## 7. Service-Account für die CI

1. **Google Cloud Console** (<https://console.cloud.google.com>): Projekt
   anlegen oder wählen → *APIs & Dienste* → **„Google Play Android Developer
   API" aktivieren**.
2. *IAM & Verwaltung → Dienstkonten* → Dienstkonto anlegen, z.B.
   `play-upload`. Keine Projekt-Rollen nötig.
3. Beim Dienstkonto → *Schlüssel* → **Neuer Schlüssel → JSON** → Datei
   herunterladen. Diese Datei ist ein Passwort-Äquivalent.
4. **Play Console** → *Nutzer und Berechtigungen* → *Nutzer einladen* →
   E-Mail-Adresse des Dienstkontos (`…@….iam.gserviceaccount.com`).
   - Zugriff auf **nur diese App** beschränken (`Dorf-App Rössing`).
   - Rolle **„Release-Manager"** (bzw. die Berechtigungen *Releases in
     Testtracks verwalten* und *App-Informationen bearbeiten*).
   - Einladung senden — Dienstkonten nehmen sie automatisch an.
5. Bis die Berechtigung greift, können ein paar Minuten vergehen (gelegentlich
   auch Stunden). Erster Upload-Versuch kann deshalb mit `401`/`403` scheitern.

## 8. GitHub-Secret hinterlegen

Repo → *Settings → Secrets and variables → Actions → New repository secret*:

| Name | Inhalt |
|---|---|
| `PLAY_SERVICE_ACCOUNT_JSON` | **kompletter Inhalt** der JSON-Datei aus Schritt 7.3 |

Solange das Secret fehlt, überspringt der Release-Workflow den Play-Upload mit
einer Notiz im Log — der Rest (GitHub-Release, Firebase-Verteilung) läuft
weiter.

## 9. Ab jetzt: Tag setzen genügt

```sh
git tag v0.1.2 && git push origin v0.1.2
```

Der Workflow `.github/workflows/release.yml` baut das signierte AAB, hängt es
ans GitHub-Release, verteilt das APK per Firebase an die Gruppe „tester" und
lädt das AAB auf den Play-Track **`internal`**. Release-Notizen zieht er aus
`store/metadata/android/de-DE/changelogs/<versionCode>.txt`.

**Voraussetzung:** Jeder Upload braucht einen **höheren `versionCode`** als der
vorige. Die Ableitung des `versionCode` aus dem Tag ist noch offen — siehe
`store/README.md`, Abschnitt „Offene Änderung am Android-Build".

## 10. Weg in die Produktion

1. Interner Test läuft, App ist installierbar.
2. **Nur bei Privatkonto:** *Geschlossener Test* mit mindestens 12 Testern
   anlegen, die 14 Tage durchgehend angemeldet bleiben. Play zählt das
   automatisch mit und schaltet den Antrag danach frei.
3. *Produktion → Neues Release* mit demselben AAB, Rollout ggf. gestaffelt.
4. Erste Prüfung dauert erfahrungsgemäß mehrere Tage. Häufigste
   Ablehnungsgründe bei einer App wie dieser: fehlendes oder nicht
   funktionierendes Prüfkonto (Schritt 4), Datenschutz-URL nicht erreichbar,
   Angaben zur Datensicherheit passen nicht zum beobachteten Netzwerkverkehr.
