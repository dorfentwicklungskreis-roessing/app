# Datenschutz — Kurzfassung für den Play Store

> **Achtung:** Dieses Dokument ist die Arbeitsgrundlage, **nicht** die verbindliche
> Erklärung. Google Play verlangt eine **öffentlich erreichbare Datenschutzerklärung
> unter einer eigenen URL**, die ohne Anmeldung, ohne PDF-Download und in der
> Sprache der Store-Eintragung abrufbar ist. Diese Erklärung wird auf
> **roessing.de** veröffentlicht; die URL trägt Levin in der Play Console unter
> *App-Inhalte → Datenschutzerklärung* nach und ergänzt sie hier:
>
> **URL: _(noch einzutragen)_**
>
> Der Text unten kann als Vorlage für diese Seite dienen.

## Verantwortlich

Dorfentwicklungskreis Rössing. Ladungsfähige Anschrift und E-Mail-Adresse müssen
in der veröffentlichten Fassung stehen (Play prüft das stichprobenartig, und die
DSGVO verlangt es ohnehin). — **offen: welche Anschrift und welche
Kontaktadresse genau?**

## Welche Daten verarbeitet die App

**Anmeldedaten (Rössing-ID).** Die Anmeldung läuft über die Rössing-ID, eine vom
Dorf selbst betriebene Zitadel-Instanz unter `id.xn--rssing-wxa.de`. Die App
öffnet dafür den System-Browser (OAuth 2.0 Authorization Code mit PKCE). Aus dem
Konto erhält die App die Nutzerkennung (`sub`), den angezeigten Namen, die
E-Mail-Adresse und die Rollen im Projekt. Anmeldedaten (Passwort) sieht die App
nie — sie werden nur im Browser gegenüber der Rössing-ID eingegeben.

**Zugangs- und Erneuerungstoken.** Sie liegen ausschließlich auf dem Gerät, in
einem privaten Bereich der App (DataStore). Beim Abmelden werden sie gelöscht.

**Erledigungsmeldungen.** Meldet jemand „gegossen" oder „gejätet", speichert der
Dorfserver: Nutzerkennung, angezeigter Name, betroffene Aufgabe, Zeitpunkt,
gegebenenfalls die Litermenge und eine optionale Notiz. Diese Meldungen sind für
alle angemeldeten Dorfbewohner sichtbar — in der Historie eines Ortes und in der
Rangliste. Genau das ist der Zweck der App.

**Kein Gerätestandort.** Die App fordert keine Standortberechtigung an und greift
nicht auf den Standort des Geräts zu. Die Karte zeigt feste Koordinaten der
Blumenkästen und Beete, nicht die eigene Position.

**Kein Tracking.** Keine Werbung, keine Analyse- oder Absturz-SDKs, keine
Werbe-ID, kein Profiling.

## Empfänger

- **Dorfserver** `app.xn--rssing-wxa.de` — betrieben vom Dorfentwicklungskreis auf
  eigener Infrastruktur (K3S-Cluster, SQLite-Datenbank).
- **Rössing-ID** `id.xn--rssing-wxa.de` — ebenfalls selbst betrieben.
- **OpenFreeMap** (`tiles.openfreemap.org`) — liefert die Kartenkacheln auf Basis
  von OpenStreetMap-Daten. Dabei erfährt dieser Dienst die IP-Adresse des Geräts
  und die abgerufenen Kachelbereiche, wie bei jedem Abruf einer Internetseite.
  Kein Vertragsverhältnis, kein Konto, kein API-Schlüssel.

Darüber hinaus gibt es keine Weitergabe an Dritte. Nichts wird verkauft.

## Rechtsgrundlage

Berechtigtes Interesse bzw. Vertragserfüllung (Art. 6 Abs. 1 lit. b und f DSGVO):
Ohne Kennung und Namen lässt sich nicht darstellen, wer welchen Kasten wann
gegossen hat — das ist der Sinn der gemeinsamen Übersicht.

## Speicherdauer und Löschung

- Eine irrtümliche Meldung kann die meldende Person selbst zurücknehmen
  (in der App bzw. über `DELETE /api/v1/completions/{id}`); Admins können jede
  Meldung löschen.
- Auf Wunsch werden alle Meldungen einer Person gelöscht bzw. anonymisiert und
  das Konto in der Rössing-ID entfernt. Anfragen an den Dorfentwicklungskreis.
- **offen:** Google Play verlangt für Apps mit Konto zusätzlich eine
  **öffentliche Web-Seite zur Löschung von Konto und Daten** (Abschnitt
  *Datenlöschung* in der Play Console). Auch die muss auf roessing.de entstehen.

## Rechte der Betroffenen

Auskunft, Berichtigung, Löschung, Einschränkung, Widerspruch, Datenübertragbarkeit
sowie Beschwerde bei der Landesbeauftragten für den Datenschutz Niedersachsen.

## Verschlüsselung

Sämtliche Verbindungen der App laufen über HTTPS (TLS). Die App spricht keinen
Klartext-Endpunkt an.

## Kinder

Die App richtet sich nicht an Kinder und ist nicht für den Bereich „Familien"
gedacht. Eine Nutzung setzt eine Rössing-ID voraus, die der Dorfentwicklungskreis
vergibt.
