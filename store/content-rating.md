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
| Schimpfwörter, vulgäre Sprache | **Nein** | Oberfläche ist sachlich. Seit `0.1.7` gibt es unter „Idee vorschlagen" ein freies Textfeld — der Text geht aber ausschließlich an den Dorfentwicklungskreis, wird nicht veröffentlicht und ist für andere Nutzer nirgends sichtbar. Es kann also niemand einer anderen Person etwas zu lesen geben. |
| Drogen, Alkohol, Tabak | **Nein** | — |
| Glücksspiel — echtes Geld | **Nein** | keine Zahlungen, keine In-App-Käufe |
| Glücksspiel — simuliert (Casino-Optik, Lootboxen) | **Nein** | die Rangliste zeigt gezählte Erledigungen, es gibt keinen Zufallsmechanismus |
| Angst einflößende oder verstörende Inhalte | **Nein** | — |
| Nutzer können miteinander kommunizieren (Chat, Nachrichten, Kommentare) | **Nein** | Es gibt keinen Chat und keine Kommentarfunktion. Die App zeigt lediglich, **wer wann** eine Pflegeaufgabe erledigt hat — Name und Zeitpunkt, kein von Nutzern verfasster Text. Das Freitextfeld `note` im Backend ist in der App nicht bedienbar. **Auch die Vergabe seit `0.1.7` ändert das nicht:** Anfragen, Rundruf und Hinweise formuliert der Server aus festen Bausteinen (`vergabe.texte()` — Ortsname, Aufgabenname, Frist); niemand tippt Text für jemand anderen, und auf eine Anfrage antwortet man mit „Ich mache das" oder „Zurückgeben", nicht mit Worten. |
| Nutzer können Inhalte teilen (Fotos, Dateien, Position) | **Nein** | keine Upload-, Foto- oder Teilen-Funktion. Der Ideen-Text erreicht keine anderen Nutzer. |
| Werden persönliche Daten an Dritte weitergegeben? | **Ja** (seit `0.1.7`) | Für Push-Benachrichtigungen laufen die Kennung der App-Installation und der Meldungstext über Google (Firebase Cloud Messaging). Von Nutzern eingegebene Angaben — Name, E-Mail, Telefonnummer — gehen nicht hinaus. Wir antworten hier trotzdem **Ja**, damit die Angabe zur Datensicherheit passt; Google gleicht beide Formulare ab. Server und Nutzerkonten betreibt das Dorf im Übrigen weiterhin selbst, es gibt keine Werbe- oder Analysepartner. |
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

**Für `0.1.7` ist er bereits neu abzugeben**, weil die Antwort auf die Frage
nach der Weitergabe an Dritte von Nein auf Ja wechselt (Push über Firebase
Cloud Messaging). An der Einstufung ändert das nichts — es kommt allenfalls
ein Hinweis „gibt Daten weiter" hinzu.
