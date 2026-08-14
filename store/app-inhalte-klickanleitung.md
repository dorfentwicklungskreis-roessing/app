# App-Inhalte in der Play Console — zum Durchklicken

Der einzige Teil der Veröffentlichung, für den es **keine Schnittstelle** gibt:
Google verlangt hier eine Erklärung von einem Menschen. Unten steht zu jedem
Abschnitt die Antwort, abgeleitet aus dem tatsächlichen Funktionsumfang.
Ausführliche Begründungen: `data-safety.md` und `content-rating.md`.

Ort: **Play Console → Richtlinien und Programme → App-Inhalte**
(je nach Ansicht auch „Testen und veröffentlichen → App-Inhalte").

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
| Nutzername | `test-dorf` |
| Passwort | (das hinterlegte Testkonto-Passwort) |
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
| Können Nutzer miteinander kommunizieren? | **Nein** | Es gibt keinen Chat und keine Kommentare. Sichtbar ist nur, **wer wann** etwas erledigt hat. |
| Können Nutzer Inhalte teilen (Fotos, Dateien, Standort)? | **Nein** | Keine Upload- oder Teilen-Funktion. |
| Wird der Standort erhoben und anderen angezeigt? | **Nein** | Der Standort bleibt auf dem Gerät; er wird nie übertragen. |
| Werden personenbezogene Daten an Dritte weitergegeben? | **Nein** | Server und Konten betreibt das Dorf selbst. |

Erwartetes Ergebnis: USK 0 bzw. PEGI 3.

## 5. Zielgruppe und Inhalte

**Altersgruppe: 18 und älter.**
Grund: Die App richtet sich an Erwachsene im Dorf und setzt ein Konto der
Rössing-ID voraus. Damit greifen die besonderen Auflagen für Kinder-Apps nicht.
Frage „Sollen Kinder von der App angesprochen werden?" → **Nein**.

## 6. Datensicherheit

Die vollständigen Antworten stehen in **`data-safety.md`**. Kurzfassung:

**Erhoben, nicht an Dritte weitergegeben, per HTTPS verschlüsselt, löschbar:**

| Datentyp | Pflicht/optional | Zweck |
|---|---|---|
| Name (Anzeigename, Nickname) | Pflicht | App-Funktionalität, Kontoverwaltung |
| E-Mail-Adresse | Pflicht (Anmeldung), optional (Profil) | Kontoverwaltung |
| Telefonnummer | **optional** (Profil) | App-Funktionalität |
| Nutzer-IDs | Pflicht | App-Funktionalität |
| App-Aktivitäten (Erledigungen) | Pflicht | App-Funktionalität |

**Ausdrücklich NICHT erhoben:** Standort (bleibt auf dem Gerät), Fotos,
Kontakte, Kalender, Zahlungsdaten, Werbe-ID, Absturzdaten Dritter.

Fragen zur **Löschung**: Ja, Konto und Daten sind löschbar —
`https://xn--rssing-wxa.de/app/daten-loeschen/`

## 7.–11. Die übrigen Abschnitte

Alle mit **Nein** bzw. „trifft nicht zu":

- **Nachrichten-App** → Nein
- **COVID-19-Kontaktverfolgung/-Statusanwendung** → Nein
- **Regierungs-App** → Nein
- **Finanzfeatures** → Nein (keine Zahlungen, keine Kredite, keine Krypto)
- **Gesundheits-App** → Nein (keine Gesundheitsdaten, keine Medizinfunktionen)
- **Werbe-ID** → Nein, die App verwendet keine Werbe-ID
  (kein Werbe- oder Analyse-Baustein enthalten)

---

## Wenn Push dazukommt

Sobald Benachrichtigungen ausgeliefert sind, ändert sich **ein** Punkt: Es
wandert dann eine **Gerätekennung an Google** (Firebase Cloud Messaging). Dann
sind Datensicherheit und Datenschutzerklärung nachzuziehen — das wird hier
vermerkt, sobald es so weit ist.
