package de.roessing.app.data

import java.time.Instant
import java.time.LocalDate
import java.time.OffsetDateTime
import java.time.ZoneId
import java.time.ZonedDateTime

/**
 * Aus den Rohdaten der Website werden Termine, wie die Oberfläche sie
 * braucht. Bewusst reine Funktionen ohne Android-Bezug — so laufen sie im
 * normalen Unit-Test.
 *
 * Zwei Fallstricke stecken hier drin:
 *  - **Zeitzone.** Die Zeitpunkte tragen einen Offset (`+01:00` im Winter,
 *    `+02:00` im Sommer). Sie werden als Zeitpunkt gelesen und in Ortszeit
 *    angezeigt — nie naiv als Zeichenkette abgeschnitten.
 *  - **Ganztägig.** Dann steht dort nur ein Datum. Ein solcher Termin hat
 *    keine Uhrzeit, und es wird auch keine erfunden.
 */

/** Ortszeit des Dorfes. */
val DORF_ZEITZONE: ZoneId = ZoneId.of("Europe/Berlin")

/** Kurze Wochentage — fest verdrahtet, damit Gerät und Test dasselbe zeigen. */
private val WOCHENTAGE = arrayOf("Mo", "Di", "Mi", "Do", "Fr", "Sa", "So")

data class Termin(
    val id: String,
    val name: String,
    val beschreibung: String,
    /** Beginn als Zeitpunkt; ganztägig ist das Mitternacht in Ortszeit. */
    val beginn: Instant,
    /** Ab hier ist der Termin vorbei: Mitternacht nach seinem letzten Tag. */
    val vorbeiAb: Instant,
    val ganztaegig: Boolean,
    /** „Mo, 17.08.2026" */
    val datumText: String,
    /** „18:00 Uhr" — oder null bei ganztägigen Terminen. */
    val zeitText: String?,
    /**
     * Wohin der Tipp führt: zur externen Primärquelle, falls es eine gibt,
     * sonst auf die Seite des Dorfes.
     */
    val url: String,
    /** true = die Seite gehört jemand anderem; wir zeigen den Inhalt nicht doppelt. */
    val extern: Boolean,
    val ortName: String?,
    val ortAdresse: String?,
    val koordinate: LatLon?,
    val veranstalter: String?,
) {
    fun istVorbei(jetzt: Instant): Boolean = !jetzt.isBefore(vorbeiAb)
}

/**
 * Liest einen Zeitpunkt: entweder ein reines Datum (ganztägig) oder eine
 * Ortszeit mit Offset. Was sich nicht lesen lässt, ergibt null — ein einzelner
 * kaputter Eintrag darf nicht die ganze Liste kosten.
 */
private fun zeitpunkt(text: String, zone: ZoneId): ZonedDateTime? = runCatching {
    OffsetDateTime.parse(text).atZoneSameInstant(zone)
}.recoverCatching {
    LocalDate.parse(text).atStartOfDay(zone)
}.recoverCatching {
    Instant.parse(text).atZone(zone)
}.getOrNull()

private fun datumText(zeit: ZonedDateTime): String {
    val tag = WOCHENTAGE[zeit.dayOfWeek.value - 1]
    return "%s, %02d.%02d.%04d".format(tag, zeit.dayOfMonth, zeit.monthValue, zeit.year)
}

private fun zeitText(zeit: ZonedDateTime): String =
    "%02d:%02d Uhr".format(zeit.hour, zeit.minute)

/**
 * Macht aus einer Veranstaltung der Website einen Termin — oder null, wenn
 * die Datumsangabe unlesbar ist.
 */
fun VeranstaltungDto.alsTermin(zone: ZoneId = DORF_ZEITZONE): Termin? {
    val beginn = zeitpunkt(start, zone) ?: return null
    // Ein Termin ist erst am Ende seines letzten Tages vorbei: Wer um 19 Uhr
    // anfängt, verschwindet nicht um 19:01 aus der Liste. Genauso hält es die
    // Website.
    val letzterTag = (end?.let { zeitpunkt(it, zone) } ?: beginn).toLocalDate()
    val ort = location?.takeIf { it.name.isNotBlank() }
    return Termin(
        id = id,
        name = name,
        beschreibung = description,
        beginn = beginn.toInstant(),
        vorbeiAb = letzterTag.plusDays(1).atStartOfDay(zone).toInstant(),
        ganztaegig = allDay,
        datumText = datumText(beginn),
        zeitText = if (allDay) null else zeitText(beginn),
        url = url,
        extern = external,
        ortName = ort?.name,
        ortAdresse = ort?.address?.takeIf { it.isNotBlank() },
        koordinate = ort?.let { o ->
            val lat = o.lat
            val lon = o.lon
            if (lat != null && lon != null) LatLon(lat, lon) else null
        },
        veranstalter = organizer?.name?.takeIf { it.isNotBlank() },
    )
}

/**
 * Die Liste, wie sie angezeigt wird: kommende Termine zuerst, vergangene
 * heraus. Gefiltert wird hier noch einmal selbst — die Datei der Website
 * entsteht beim Bauen und altert zwischen zwei Bauläufen.
 */
fun List<VeranstaltungDto>.alsTermine(
    jetzt: Instant,
    zone: ZoneId = DORF_ZEITZONE,
): List<Termin> = mapNotNull { it.alsTermin(zone) }
    .filterNot { it.istVorbei(jetzt) }
    .sortedBy { it.beginn }
