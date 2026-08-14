package de.roessing.app.ui

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import de.roessing.app.data.PlaceDto
import de.roessing.app.data.PlaceEingabe
import de.roessing.app.data.TaskDto
import de.roessing.app.data.TaskEingabe
import de.roessing.app.data.VerwaltungAbgelehntException
import de.roessing.app.data.VerwaltungRepository
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * Die Verwaltung in der App: Orte und Aufgaben anlegen, ändern, pausieren
 * und löschen — am Blumenkasten stehend statt später am Rechner.
 *
 * Erlaubt ist das nur der Verwaltung. Durchgesetzt wird das im Backend
 * (403 ohne die Rolle „admin"); die App zeigt den Bereich zusätzlich gar
 * nicht erst an. Auch die fachlichen Regeln — etwa dass eine einmalige
 * Aufgabe ein Fälligkeitsdatum braucht — gelten dort verbindlich; hier
 * sorgen sie nur dafür, dass niemand in eine vermeidbare Fehlermeldung
 * läuft.
 */

/** Das Formular für einen Ort. id = null heißt: neu. */
data class OrtForm(
    val id: Long? = null,
    val name: String = "",
    val beschreibung: String = "",
    val art: String = "blumenkasten",
    val lat: Double? = null,
    val lon: Double? = null,
    val aktiv: Boolean = true,
    val sendet: Boolean = false,
    /** Klartext-Begründung des Backends, wenn etwas abgelehnt wurde. */
    val fehler: String? = null,
) {
    val neu: Boolean get() = id == null

    /** Ohne Namen und ohne Standort gibt es nichts zu speichern. */
    val speicherbar: Boolean
        get() = !sendet && name.isNotBlank() && lat != null && lon != null
}

/** Das Formular für eine Aufgabe. id = null heißt: neu. */
data class AufgabeForm(
    val placeId: Long,
    val id: Long? = null,
    val art: String = "giessen",
    val titel: String = "",
    val liter: String = "",
    val einmalig: Boolean = false,
    /** Fälligkeitsdatum als „2026-08-20" (nur bei einmalig). */
    val termin: String = "",
    val intervall: String = "7",
    val rot: String = "14",
    val entfernenNachErledigung: Boolean = false,
    val aktiv: Boolean = true,
    val sendet: Boolean = false,
    val fehler: String? = null,
) {
    val neu: Boolean get() = id == null

    val speicherbar: Boolean
        get() = !sendet && if (einmalig) {
            termin.isNotBlank()
        } else {
            intervall.toDoubleOrNull()?.let { it > 0 } == true &&
                rot.toDoubleOrNull()?.let { it > 0 } == true
        }
}

data class VerwaltungUiState(
    val ortForm: OrtForm? = null,
    val aufgabeForm: AufgabeForm? = null,
    /** IDs, für die gerade etwas läuft (Knöpfe sperren). */
    val laufend: Set<Long> = emptySet(),
)

/** Einmalige Ereignisse der Verwaltung (Snackbars, Neuladen). */
sealed interface VerwaltungEvent {
    data object OrtGespeichert : VerwaltungEvent
    data object OrtGeloescht : VerwaltungEvent
    data object AufgabeGespeichert : VerwaltungEvent
    data object AufgabeGeloescht : VerwaltungEvent
    data object AufgabePausiert : VerwaltungEvent
    data object AufgabeFortgesetzt : VerwaltungEvent

    /** Etwas wurde abgewiesen; der Grund kommt vom Backend. */
    data class Abgelehnt(val grund: String) : VerwaltungEvent
}

class VerwaltungViewModel(private val repo: VerwaltungRepository) : ViewModel() {
    private val _state = MutableStateFlow(VerwaltungUiState())
    val state: StateFlow<VerwaltungUiState> = _state

    private val _events = MutableSharedFlow<VerwaltungEvent>(extraBufferCapacity = 8)
    val events: SharedFlow<VerwaltungEvent> = _events

    // --- Orte -----------------------------------------------------------------

    /** Öffnet das Ortsformular. null = neuer Ort. */
    fun ortBearbeiten(ort: PlaceDto?) = _state.update {
        it.copy(
            ortForm = if (ort == null) {
                OrtForm()
            } else {
                OrtForm(
                    id = ort.id, name = ort.name, beschreibung = ort.description,
                    art = ort.kind, lat = ort.lat, lon = ort.lon, aktiv = ort.active,
                )
            },
        )
    }

    fun ortAbbrechen() = _state.update { it.copy(ortForm = null) }

    fun setOrtName(v: String) = imOrt { it.copy(name = v, fehler = null) }

    fun setOrtBeschreibung(v: String) = imOrt { it.copy(beschreibung = v, fehler = null) }

    fun setOrtArt(v: String) = imOrt { it.copy(art = v, fehler = null) }

    fun setOrtAktiv(v: Boolean) = imOrt { it.copy(aktiv = v, fehler = null) }

    /** Ort auf der Karte angetippt. */
    fun setOrtPosition(lat: Double, lon: Double) = imOrt { it.copy(lat = lat, lon = lon, fehler = null) }

    /**
     * „Mein Standort" übernehmen — der bequeme Weg, wenn man ohnehin vor dem
     * Kasten steht. Der Standort bleibt auf dem Gerät; hier geht er bewusst
     * als Ortsangabe mit, weil genau das der Zweck ist.
     */
    fun standortUebernehmen(lat: Double, lon: Double) = setOrtPosition(lat, lon)

    fun ortSpeichern() {
        val form = _state.value.ortForm ?: return
        if (!form.speicherbar) return
        val eingabe = PlaceEingabe(
            name = form.name.trim(), description = form.beschreibung.trim(),
            kind = form.art, lat = form.lat!!, lon = form.lon!!, active = form.aktiv,
        )
        viewModelScope.launch {
            imOrt { it.copy(sendet = true, fehler = null) }
            runCatching {
                if (form.neu) repo.ortAnlegen(eingabe) else repo.ortAendern(form.id!!, eingabe)
            }
                .onSuccess {
                    _state.update { it.copy(ortForm = null) }
                    _events.emit(VerwaltungEvent.OrtGespeichert)
                }
                .onFailure { fehler ->
                    // Das Formular bleibt stehen, samt allem Getippten.
                    imOrt { it.copy(sendet = false, fehler = grund(fehler)) }
                    _events.emit(VerwaltungEvent.Abgelehnt(grund(fehler)))
                }
        }
    }

    fun ortLoeschen(id: Long) = mitLaufend(id, VerwaltungEvent.OrtGeloescht) { repo.ortLoeschen(id) }

    // --- Aufgaben -------------------------------------------------------------

    /** Öffnet das Aufgabenformular. aufgabe = null heißt: neue Aufgabe. */
    fun aufgabeBearbeiten(placeId: Long, aufgabe: TaskDto?) = _state.update {
        it.copy(
            aufgabeForm = if (aufgabe == null) {
                AufgabeForm(placeId = placeId)
            } else {
                AufgabeForm(
                    placeId = placeId, id = aufgabe.id, art = aufgabe.kind, titel = aufgabe.title,
                    liter = aufgabe.liters?.let { l -> zahlText(l) } ?: "",
                    einmalig = aufgabe.oneOff,
                    termin = terminText(aufgabe.dueDate),
                    intervall = zahlText(aufgabe.intervalDays.takeIf { d -> d > 0 } ?: 7.0),
                    rot = zahlText(aufgabe.redAfterDays.takeIf { d -> d > 0 } ?: 14.0),
                    entfernenNachErledigung = aufgabe.removeWhenDone,
                    aktiv = aufgabe.active,
                )
            },
        )
    }

    fun aufgabeAbbrechen() = _state.update { it.copy(aufgabeForm = null) }

    fun setAufgabeArt(v: String) = inAufgabe { it.copy(art = v, fehler = null) }

    fun setAufgabeTitel(v: String) = inAufgabe { it.copy(titel = v, fehler = null) }

    fun setAufgabeLiter(v: String) = inAufgabe { it.copy(liter = v, fehler = null) }

    fun setAufgabeEinmalig(v: Boolean) = inAufgabe { it.copy(einmalig = v, fehler = null) }

    fun setAufgabeTermin(v: String) = inAufgabe { it.copy(termin = v, fehler = null) }

    fun setAufgabeIntervall(v: String) = inAufgabe { it.copy(intervall = v, fehler = null) }

    fun setAufgabeRot(v: String) = inAufgabe { it.copy(rot = v, fehler = null) }

    fun setAufgabeEntfernenNachErledigung(v: Boolean) =
        inAufgabe { it.copy(entfernenNachErledigung = v, fehler = null) }

    fun setAufgabeAktiv(v: Boolean) = inAufgabe { it.copy(aktiv = v, fehler = null) }

    fun aufgabeSpeichern() {
        val form = _state.value.aufgabeForm ?: return
        if (!form.speicherbar) return
        val eingabe = eingabeAus(form)
        viewModelScope.launch {
            inAufgabe { it.copy(sendet = true, fehler = null) }
            runCatching {
                if (form.neu) {
                    repo.aufgabeAnlegen(form.placeId, eingabe)
                } else {
                    repo.aufgabeAendern(form.id!!, eingabe)
                }
            }
                .onSuccess {
                    _state.update { it.copy(aufgabeForm = null) }
                    _events.emit(VerwaltungEvent.AufgabeGespeichert)
                }
                .onFailure { fehler ->
                    inAufgabe { it.copy(sendet = false, fehler = grund(fehler)) }
                    _events.emit(VerwaltungEvent.Abgelehnt(grund(fehler)))
                }
        }
    }

    /**
     * Pausieren und Fortsetzen — der Fall „Urlaub" oder „Kasten abgebaut".
     * Wer die Aufgabe gerade zugesagt hat, bekommt vom Backend einen Hinweis.
     */
    fun aufgabePausieren(aufgabe: TaskDto, pausiert: Boolean) {
        val ereignis = if (pausiert) VerwaltungEvent.AufgabePausiert else VerwaltungEvent.AufgabeFortgesetzt
        mitLaufend(aufgabe.id, ereignis) {
            repo.aufgabeAendern(aufgabe.id, eingabeAus(aufgabe).copy(active = !pausiert))
        }
    }

    fun aufgabeLoeschen(id: Long) =
        mitLaufend(id, VerwaltungEvent.AufgabeGeloescht) { repo.aufgabeLoeschen(id) }

    // --- Kleinkram ------------------------------------------------------------

    private fun imOrt(f: (OrtForm) -> OrtForm) =
        _state.update { s -> s.copy(ortForm = s.ortForm?.let(f)) }

    private fun inAufgabe(f: (AufgabeForm) -> AufgabeForm) =
        _state.update { s -> s.copy(aufgabeForm = s.aufgabeForm?.let(f)) }

    /** Führt eine Aktion aus, sperrt solange den Knopf und meldet das Ergebnis. */
    private fun mitLaufend(id: Long, erfolg: VerwaltungEvent, aktion: suspend () -> Unit) {
        viewModelScope.launch {
            _state.update { it.copy(laufend = it.laufend + id) }
            runCatching { aktion() }
                .onSuccess { _events.emit(erfolg) }
                .onFailure { _events.emit(VerwaltungEvent.Abgelehnt(grund(it))) }
            _state.update { it.copy(laufend = it.laufend - id) }
        }
    }

    private fun eingabeAus(form: AufgabeForm) = TaskEingabe(
        kind = form.art,
        title = form.titel.trim(),
        liters = form.liter.replace(',', '.').toDoubleOrNull()?.takeIf { it > 0 },
        intervalDays = if (form.einmalig) 0.0 else form.intervall.replace(',', '.').toDoubleOrNull() ?: 0.0,
        redAfterDays = if (form.einmalig) 0.0 else form.rot.replace(',', '.').toDoubleOrNull() ?: 0.0,
        oneOff = form.einmalig,
        dueDate = if (form.einmalig) form.termin.trim() else "",
        removeWhenDone = form.entfernenNachErledigung,
        active = form.aktiv,
    )

    /** Eingabe aus einer bestehenden Aufgabe — für Pausieren ohne Formular. */
    private fun eingabeAus(aufgabe: TaskDto) = TaskEingabe(
        kind = aufgabe.kind,
        title = aufgabe.title,
        liters = aufgabe.liters,
        intervalDays = aufgabe.intervalDays,
        redAfterDays = aufgabe.redAfterDays,
        oneOff = aufgabe.oneOff,
        dueDate = terminText(aufgabe.dueDate),
        removeWhenDone = aufgabe.removeWhenDone,
        active = aufgabe.active,
    )

    private fun grund(fehler: Throwable): String = when (fehler) {
        is VerwaltungAbgelehntException -> fehler.grund
        else -> "Das hat nicht geklappt. Besteht eine Verbindung?"
    }
}

/**
 * Macht aus dem Zeitpunkt des Backends das Datum, das im Formular steht.
 * Gerechnet wird in Ortszeit: Der Termin „20.08." steht als 20.08. 23:59
 * Ortszeit in der Datenbank — in UTC gelesen wäre das der 20.08. um 21:59,
 * bei anderen Zeitzonen schnell der Vortag.
 */
private fun terminText(dueDate: String?): String {
    val roh = dueDate?.takeIf { it.isNotBlank() } ?: return ""
    return runCatching {
        java.time.Instant.parse(roh).atZone(ZoneId.of("Europe/Berlin"))
            .format(DateTimeFormatter.ISO_LOCAL_DATE)
    }.getOrElse { roh.take(10) }
}

/** Zahlen ohne überflüssige Nachkommastellen: 7 statt 7.0. */
private fun zahlText(v: Double): String =
    if (v == v.toLong().toDouble()) v.toLong().toString() else v.toString()
