# App-Inhalte in der Play Console — zum Durchklicken

Der einzige Teil der Veröffentlichung, für den es **keine Schnittstelle** gibt:
Google verlangt hier eine Erklärung von einem Menschen. Unten steht zu jedem
Abschnitt die Antwort, abgeleitet aus dem tatsächlichen Funktionsumfang.
Ausführliche Begründungen: `data-safety.md` und `content-rating.md`.

Ort: **Play Console → Richtlinien und Programme → App-Inhalte**
(je nach Ansicht auch „Testen und veröffentlichen → App-Inhalte").

**Stand: 0.1.7 (`versionCode 1000107`).**

## Was sich mit 0.1.7 geändert hat — zwei Abschnitte neu ausfüllen

Push-Benachrichtigungen sind ausgeliefert, und damit ist erstmals ein anderes
Unternehmen beteiligt. Wer die Angaben schon einmal ausgefüllt hat, muss
**nur** hier nachziehen:

| Abschnitt | Was sich ändert |
|---|---|
| **6. Datensicherheit** | „Werden Daten mit Dritten geteilt?" → **jetzt Ja** (Geräte-IDs, Zweck App-Funktionalität). Zusätzlich neu: Datentyp *Geräte- oder andere Kennungen* und *App-Aktivitäten → sonstige nutzergenerierte Inhalte* (Ideen-Formular). |
| **4. Altersfreigabe** | Nur die Frage nach der Weitergabe an Dritte → **jetzt Ja**. Die Frage nach Nutzer-zu-Nutzer-Kommunikation bleibt **Nein** (Begründung dort). Der Fragebogen muss dafür neu abgesendet werden; die Einstufung ändert sich dadurch nicht. |

Alles andere bleibt, wie es war. Insbesondere bleibt **Werbe-ID → Nein**: Die
Kennung von Firebase ist keine Werbe-ID.

---

## 1. Datenschutzerklärung

**URL eintragen:**

```
https://xn--rssing-wxa.de/app/datenschutz/
```

## 2. App-Zugriff

**„Alle oder Teile der App sind eingeschränkt"** auswählen und Zugangsdaten für
das Prüfteam hinterlegen — **ohne das wird die App abgelehnt**, weil der Prüfer
sonst nur den Anmeldebildschirm sieht.

| Feld | Eintrag |
|---|---|
| Name der Anleitung | Anmeldung mit der Rössing-ID |
| Nutzername | `google-reviewer` |
| Passwort | (steht in `.env` im Arbeitsverzeichnis, Schlüssel `GOOGLE_REVIEWER_PASSWORD`) |
| Weitere Hinweise | Auf „Mit Rössing-ID anmelden" tippen. Die Anmeldung läuft über den selbst betriebenen Dienst id.xn--rssing-wxa.de. Danach ist die App vollständig nutzbar. |

## 3. Anzeigen

**„Nein, die App enthält keine Werbung."**

## 4. Altersfreigabe (IARC-Fragebogen)

Kategorie: **Dienstprogramm / Produktivität / Kommunikation / Sonstiges**
(kein Spiel — die Rangliste ist eine Auswertung ohne Spielmechanik).

Alle inhaltlichen Fragen mit **Nein** beantworten: keine Gewalt, keine
Sexualität, keine Schimpfwörter, keine Drogen, kein Glücksspiel (weder echt
noch simuliert), nichts Verstörendes.

Besonders beachten:

| Frage | Antwort | Warum |
|---|---|---|
| Können Nutzer miteinander kommunizieren? | **Nein** | Siehe die Prüfung darunter. |
| Können Nutzer Inhalte teilen (Fotos, Dateien, Standort)? | **Nein** | Keine Upload- oder Teilen-Funktion. Der Ideen-Text geht nur an den Dorfentwicklungskreis, er erscheint nirgends in der App. |
| Wird der Standort erhoben und anderen angezeigt? | **Nein** | Der Standort bleibt auf dem Gerät; er wird nie übertragen. |
| Werden personenbezogene Daten an Dritte weitergegeben? | **Ja** (seit 0.1.7) | Für Benachrichtigungen laufen die Kennung der App-Installation und der Text der Meldung über Google (Firebase Cloud Messaging). Siehe unten. |

### Warum die Kommunikationsfrage trotz Anfragen und Zusagen „Nein" bleibt

Geprüft an `backend/internal/vergabe/vergabe.go` (Funktion `texte`) und
`android/app/src/main/java/de/roessing/app/ui/`:

- **Niemand schreibt jemandem.** Titel und Text jeder Anfrage erzeugt der
  Server aus festen Bausteinen — Ortsname, Aufgabenname, Frist („Du bist als
  Nächste(r) an der Reihe: Gießen an ‚Kirchplatz'"). Es gibt kein Feld, in
  das eine Person Text für eine andere Person eingeben könnte, und keinen
  Rückkanal: Auf eine Anfrage antwortet man mit „Ich mache das" oder
  „Zurückgeben", nicht mit Worten.
- **Auslösen ist keine Kommunikation.** Wer zusagt, erscheint für die übrigen
  Eingetragenen mit Namen und Frist — dieselbe Art Angabe wie „X hat am
  Dienstag gegossen" in der Historie, die den Fragebogen schon bisher nicht
  berührt hat.
- **Der Ideen-Text ist kein Gegenbeispiel.** Seit 0.1.7 gibt es „Idee
  vorschlagen" mit einem freien Textfeld. Der Text geht ausschließlich an den
  Dorfentwicklungskreis (`POST /api/v1/ideen`), wird nicht veröffentlicht und
  ist für andere Nutzer nicht sichtbar — ein Rückmeldeformular an den
  Betreiber, keine Nutzer-zu-Nutzer-Kommunikation. Für die **Datensicherheit**
  ist er trotzdem anzugeben, siehe Schritt 6.

Neu abgeben muss man den Fragebogen trotzdem — wegen der Weitergabe an Dritte
in der Zeile darüber.

**Zur Weitergabe-Frage:** An Google gehen keine von Nutzern eingegebenen
Angaben (kein Name, keine E-Mail, keine Telefonnummer), sondern die technische
Kennung der Installation und der maschinell erzeugte Meldungstext. Man könnte
darüber streiten, ob das die IARC-Frage überhaupt meint. Wir antworten
**Ja**, weil in der Datensicherheit (Schritt 6) eine Weitergabe deklariert
ist: Google gleicht die Angaben untereinander ab, und eine Zuvielangabe ist
folgenlos (sie ergänzt höchstens einen Hinweis „gibt Daten weiter"), ein
Widerspruch zwischen beiden Formularen nicht.

Erwartetes Ergebnis: USK 0 bzw. PEGI 3 — an der Altersstufe ändert die
Weitergabe nichts.

## 5. Zielgruppe und Inhalte

**Altersgruppe: 18 und älter.**
Grund: Die App richtet sich an Erwachsene im Dorf und setzt ein Konto der
Rössing-ID voraus. Damit greifen die besonderen Auflagen für Kinder-Apps nicht.
Frage „Sollen Kinder von der App angesprochen werden?" → **Nein**.

## 6. Datensicherheit

Die vollständigen Antworten stehen in **`data-safety.md`**. Kurzfassung:

**Einstiegsfragen:**

| Frage | Antwort |
|---|---|
| Erhebt die App Nutzerdaten? | **Ja** |
| Werden Daten mit Dritten **geteilt**? | **Ja** — seit 0.1.7 (vorher Nein) |
| Werden Daten bei der Übertragung verschlüsselt? | **Ja**, ausschließlich HTTPS |
| Können Nutzer die Löschung beantragen? | **Ja** |

Alle Datentypen: per HTTPS verschlüsselt, löschbar. „Geteilt" heißt bei Play
**Weitergabe an ein anderes Unternehmen** — das trifft nur auf die letzte
Zeile zu:

| Datentyp | Pflicht/optional | Zweck | Geteilt |
|---|---|---|---|
| Name (Anzeigename, Nickname) | Pflicht | App-Funktionalität, Kontoverwaltung | nein |
| E-Mail-Adresse | Pflicht (Anmeldung), optional (Profil) | Kontoverwaltung | nein |
| Telefonnummer | **optional** (Profil) | App-Funktionalität | nein |
| Nutzer-IDs | Pflicht | App-Funktionalität | nein |
| App-Aktivitäten (Erledigungen, Helfer-Eintrag, Zusagen) | Pflicht | App-Funktionalität | nein |
| **Sonstige nutzergenerierte Inhalte** (Ideen-Text) — *neu in 0.1.7* | **optional** | App-Funktionalität | nein |
| **Geräte- oder andere Kennungen** (Geräte-ID) — *neu in 0.1.7* | **optional** | App-Funktionalität | **ja** |

**Die geteilte Zeile im Einzelnen** (Play fragt bei „geteilt" noch einmal
nach): Datentyp *Geräte- oder andere Kennungen → Geräte-ID*, Zweck
**App-Funktionalität** (kein Werbe-, kein Analysezweck), **nicht** nur
kurzzeitig verarbeitet. Empfänger ist Google (Firebase Cloud Messaging); es
gehen die Kennung der App-Installation, Titel und Text der Meldung sowie die
internen Nummern von Ort, Aufgabe und Vorgang hinaus. Namen anderer Personen
nie.

**Ausdrücklich NICHT erhoben:** Standort (bleibt auf dem Gerät), Fotos,
Kontakte, Kalender, Zahlungsdaten, **Werbe-ID** (die Firebase-Kennung ist
keine), Absturz- und Analysedaten.

Fragen zur **Löschung**: Ja, Konto und Daten sind löschbar —
`https://xn--rssing-wxa.de/app/daten-loeschen/`

> **Vor dem Absenden prüfen:** `data-safety.md`, Abschnitt „Abweichung im
> Code" — die Gerätekennung wird derzeit auch dann angemeldet, wenn die
> Erlaubnis für Benachrichtigungen nicht erteilt ist. Davon hängt ab, ob
> „optional" in der Zeile *Geräte-ID* stehen bleiben darf.

## 7.–11. Die übrigen Abschnitte

Alle mit **Nein** bzw. „trifft nicht zu":

- **Nachrichten-App** → Nein. Die Push-Benachrichtigungen ändern daran
  nichts: Gemeint ist eine App, mit der Menschen einander schreiben (SMS,
  Chat). Hier verschickt der Server Hinweise über eine anstehende Aufgabe.
- **COVID-19-Kontaktverfolgung/-Statusanwendung** → Nein
- **Regierungs-App** → Nein
- **Finanzfeatures** → Nein (keine Zahlungen, keine Kredite, keine Krypto)
- **Gesundheits-App** → Nein (keine Gesundheitsdaten, keine Medizinfunktionen)
- **Werbe-ID** → Nein, die App verwendet keine Werbe-ID.
  Seit 0.1.7 ist `firebase-messaging` eingebunden, aber **nur** dafür: keine
  `play-services-ads-identifier`-Abhängigkeit, keine Analyse-Bibliothek. Die
  Kennung, die Firebase vergibt, gilt nur für diese Installation und lässt
  sich nicht über Apps hinweg zusammenführen.

---

## Prüfkonto

Für die Google-Prüfung existiert ein **eigenes** Konto der Rössing-ID:
`google-reviewer` (angelegt 14.08.2026, keine Rollen — kann Orte sehen und
Erledigungen melden, nichts verwalten). Bewusst getrennt vom Testkonto
`test-dorf`, das die automatischen Tests benutzen: So lässt sich das
Prüfkonto jederzeit sperren, ohne die CI zu brechen.
