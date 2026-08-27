# Backend der Dorf-App

Der Überblick über Aufbau, Betrieb und Entwicklung steht im
[README des Repos](../README.md); die Sicherheits- und Datenschutzfragen in
[SICHERHEIT.md](SICHERHEIT.md). Diese Datei beschreibt Endpunkte, deren
Verhalten sich nicht von selbst versteht.

## `/dev/…` — die Test-Knöpfe (nur `AUTH_MODE=insecure-dev`)

Diese Pfade gibt es **nur**, solange das Backend mit `AUTH_MODE=insecure-dev`
läuft — dieselbe Bedingung, hinter der auch der Entwickler-Login sitzt. In der
Produktion sind sie **nicht registriert**, nicht bloß abgewiesen: Ein Pfad, der
nur bewacht ist, ist eine vergessene Prüfung davon entfernt, die Uhr des
laufenden Dorfes zu verstellen. Geprüft wird das in
`cmd/server/main_test.go: TestTestEndpunkteNurImDevModus`, indem der echte
Server einmal im Dev-Modus und einmal im Produktionspfad startet.

| Route | was sie tut |
| --- | --- |
| `GET /dev/clock` | welche Zeit das Backend gerade annimmt (`now`, `offset`) |
| `POST /dev/clock/set` | `{"time":"2026-09-06T10:00:00+02:00"}` — Uhr auf diesen Zeitpunkt stellen |
| `POST /dev/clock/advance` | `{"duration":"240h"}` — Uhr um eine Go-Dauer weiterstellen |
| `POST /dev/clock/reset` | zurück auf die Systemuhr |
| `POST /dev/assignment/run` | **einen** Vergabe-Durchlauf, synchron; die Antwort nennt die Zahl der erzeugten Benachrichtigungen |

Sie existieren, damit kein Test wartet. Die Vergabe rechnet in Tagen und
Stunden; ein Test, der auf sie wartet, behauptet nicht mehr „für eine fällige
Aufgabe wird der Helfer gefragt", sondern „…binnen 150 Sekunden Wanduhrzeit" —
und das hängt nur davon ab, wie beschäftigt der Rechner ist. Stattdessen:
**Uhr auf den Fälligkeitstag stellen, einen Durchlauf anstoßen, nachsehen.**

Zwei Dinge sind zu beachten:

- **Die Uhr gehört dem ganzen Prozess** (`internal/clock`). Wer sie verstellt,
  stellt sie zurück, sonst erbt der nächste Test ein Dorf in der Zukunft. Im
  Android-E2E erledigt das ein `@After` (siehe `DevBackend.kt`).
- **Die Uhrzeit ist kein Detail.** Zwischen 21 und 7 Uhr Ortszeit stellt die
  Vergabe nichts zu (Ruhezeit). Eine Zeitreise, die nachts landet, eröffnet den
  Vorgang, verschiebt die Anfrage aber auf den Morgen — deshalb reist der Test
  auf den Vormittag.

Der Durchlauf ist derselbe, den auch der Hintergrund-Zeitgeber fährt (dieselbe
`vergabe.Config`), er arbeitet synchron und ist beliebig oft wiederholbar: Er
tut nur etwas, wenn wirklich etwas ansteht.

## `DELETE /api/v1/me` — das eigene Konto löschen

Angemeldet wie alle `/api/v1`-Routen (JWT, `internal/api/api.go`). Der Rumpf
ist **optional**; ein `DELETE` ohne Inhalt ist der Normalfall. Wird einer
mitgeschickt, darf er nur `{"userSub": "<eigene Kennung>"}` enthalten — eine
fremde Kennung ergibt **403**, auch für Admins. Gelöscht wird ausschließlich
das eigene Konto; das Löschen von außen gehört in die Verwaltung, nicht in
die Selbstbedienung (dieselbe Regel wie bei `PUT /api/v1/me/profile`).

Es gibt diesen Weg, weil es ihn geben muss: Apples Richtlinie 5.1.1 (v)
verlangt von jeder App mit Konto einen Weg zum Löschen **in der App**, die
DSGVO (Art. 17) verlangt ihn ohnehin.

### Was passiert (`internal/db/konto.go`, alles in einer Transaktion)

| Daten | was damit geschieht |
| --- | --- |
| Profil (`profiles`) | gelöscht |
| Gerätekennungen (`push_devices`) | gelöscht — der Push hört sofort auf |
| Helfer-Eintragungen (`care_signups`) | gelöscht |
| Zustellungen (`care_notifications`) | gelöscht |
| Befähigungsanträge (`befaehigungs_antraege`) | gelöscht, samt Antrag, Begründung und interner Notiz |
| laufende Zusagen (`care_assignments`, `ended_at` leer) | **freigegeben**: wieder `offen`, Zusagender entfernt, neues Angebot fällig |
| beendete Zusagen | Kennung entfernt, Name durch den Ersatznamen ersetzt |
| Erledigungen (`completions`) | **bleiben**, aber ohne Kennung und unter dem Ersatznamen |
| eingereichte Ideen (`ideen`) | bleiben (die Verwaltung arbeitet sie ab), verlieren aber Kennung, Name und E-Mail |

Der Ersatzname ist „Ehemaliges Mitglied" (`api.LoeschErsatzname`). Warum die
Erledigungen bleiben, steht im Kopf von `internal/db/konto.go` und in
[SICHERHEIT.md](SICHERHEIT.md), Abschnitt „Kontolöschung“.

Ein zweiter Aufruf ist unschädlich — dann ist schlicht nichts mehr da.

### Was der Endpunkt **nicht** tut

Er löscht **nicht** das Konto in der **Rössing-ID**. Das gehört Zitadel, und
dieselbe Anmeldung dient auch anderen Anwendungen des Dorfes; die Dorf-App
darf sie nicht mit wegräumen. Die Antwort sagt das im Klartext, damit niemand
glaubt, mit dem einen Knopf sei beides erledigt.

### Antwort

**200** mit Erklärung statt 204 — hier gibt es etwas zu sagen, und die App
zeigt es an, bevor sie sich abmeldet:

```json
{
  "geloescht": true,
  "bilanz": {
    "profil": true, "geraete": 1, "anmeldungen": 3, "benachrichtigungen": 12,
    "erledigungen": 47, "zusagen": 2, "befaehigungen": 1, "ideen": 0
  },
  "erledigungen": "Deine Meldungen bleiben anonym stehen …",
  "roessingId": "Deine Rössing-ID bleibt bestehen: …",
  "roessingIdUrl": "https://id.xn--rssing-wxa.de"
}
```

Die `bilanz` sagt nur, wie viele Zeilen betroffen waren — sie enthält keine
personenbezogenen Daten und ist vor allem für die Tests und für die
Rückmeldung in der App da.
