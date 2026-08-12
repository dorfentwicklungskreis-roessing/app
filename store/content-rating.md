# Alterseinstufung (IARC-Fragebogen) — vorbereitete Antworten

Ort in der Play Console: **App-Inhalte → Altersfreigabe**. Der Fragebogen wird
von IARC gestellt; die Einstufung entsteht automatisch aus den Antworten.
Falsche Angaben führen zur Sperrung der App — deshalb ist unten alles auf den
tatsächlichen Funktionsumfang bezogen.

## Vorab

- **Kategorie:** Dienstprogramme / Produktivität / Kommunikation → hier
  **„Dienstprogramm, Produktivität, Kommunikation oder Sonstiges"**
  (kein Spiel — die Rangliste ist eine Auswertung, kein Spiel mit Spielmechanik)
- **E-Mail-Adresse für Rückfragen:** Kontaktadresse des Dorfentwicklungskreises
  — **offen, trägt Levin ein**

## Antworten

| Frage (sinngemäß) | Antwort | Begründung |
|---|---|---|
| Gewalt jeder Art (realistisch, comichaft, Waffen) | **Nein** | keine Darstellung von Gewalt |
| Sexualität, Nacktheit, anzügliche Inhalte | **Nein** | — |
| Schimpfwörter, vulgäre Sprache | **Nein** | Oberfläche ist sachlich; es gibt keine freien Texteingaben in der App |
| Drogen, Alkohol, Tabak | **Nein** | — |
| Glücksspiel — echtes Geld | **Nein** | keine Zahlungen, keine In-App-Käufe |
| Glücksspiel — simuliert (Casino-Optik, Lootboxen) | **Nein** | die Rangliste zeigt gezählte Erledigungen, es gibt keinen Zufallsmechanismus |
| Angst einflößende oder verstörende Inhalte | **Nein** | — |
| Nutzer können miteinander kommunizieren (Chat, Nachrichten, Kommentare) | **Nein** | Es gibt keinen Chat und keine Kommentarfunktion. Die App zeigt lediglich, **wer wann** eine Pflegeaufgabe erledigt hat — Name und Zeitpunkt, kein von Nutzern verfasster Text. Das Freitextfeld `note` im Backend ist in der App nicht bedienbar. |
| Nutzer können Inhalte teilen (Fotos, Dateien, Position) | **Nein** | keine Upload-, Foto- oder Teilen-Funktion |
| Werden persönliche Daten an Dritte weitergegeben? | **Nein** | Server und Nutzerkonten betreibt das Dorf selbst; keine Werbe- oder Analysepartner |
| Nutzt die App den Gerätestandort und zeigt ihn anderen Nutzern? | **Nein** | Die App fragt zwar den Standort ab (Kartenausschnitt, Entfernung zu den Pflege-Orten, Sortierung nach Nähe), die Position bleibt aber auf dem Gerät. Sie wird weder gespeichert noch an den Server noch an andere Nutzer übertragen — siehe `data-safety.md`. Auf der Karte sehen andere nur die festen Koordinaten von Blumenkästen und Beeten. |
| Werbung | **Nein** | keine Werbung |
| Käufe in der App | **Nein** | — |
| Ist die App ausschließlich für einen abgegrenzten Nutzerkreis? | **Ja** (falls gefragt) | Nutzung nur mit einer Rössing-ID, die der Dorfentwicklungskreis vergibt |

## Erwartetes Ergebnis

Mit diesen Antworten ist die Einstufung in allen Regionen die niedrigste Stufe:
USK 0, PEGI 3, ESRB „Everyone", IARC 3+.

## Bei Änderungen neu ausfüllen

Der Fragebogen muss erneut beantwortet werden, sobald die App
Nutzer-zu-Nutzer-Kommunikation (Notizen, Kommentare, Chat), Fotos oder eine
für andere sichtbare Standortanzeige von Personen bekommt.
