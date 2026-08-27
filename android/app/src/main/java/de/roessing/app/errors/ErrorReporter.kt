package de.roessing.app.errors

import de.roessing.app.data.AnfragenBeobachter
import de.roessing.app.data.ErrorReportInput
import de.roessing.app.data.ErrorReportRepository
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * Sammelt, was schiefgegangen ist — und schickt es, wenn die Person das will.
 *
 * Zwei Regeln ziehen sich durch diesen ganzen Bereich, wortgleich mit der
 * iOS-Fassung:
 *
 *  1. **Nichts geht von selbst hinaus.** Kein Bericht verlässt das Telefon,
 *     ohne dass jemand einen Knopf gedrückt hat. Ein Absturzmelder, der von
 *     allein sendet, wäre die eine Stelle in dieser App, an der still etwas
 *     erhoben wird — und dagegen gibt es hier eine Haltung
 *     (`store/data-safety.md`, `backend/SICHERHEIT.md`).
 *  2. **Nichts wird erfunden.** Im Bericht steht der Satz, den die Person
 *     gelesen hat.
 *
 * Der Melder lebt im [de.roessing.app.AppContainer] und wird von einem
 * OkHttp-Interceptor gefüttert: So kommt jede gescheiterte Anfrage vorbei,
 * und kein Bereich muss daran denken.
 */
class ErrorReporter(
    private val texte: ErrorReportTexte,
    private val facts: DeviceFacts,
    private val uhr: () -> Long = { System.currentTimeMillis() },
) : AnfragenBeobachter {
    private val _state = MutableStateFlow(ErrorReportUiState())
    val state: StateFlow<ErrorReportUiState> = _state

    private var repo: ErrorReportRepository? = null

    /** Der Weg zum Backend. Einmal verdrahtet, wie bei den Benachrichtigungen. */
    fun wire(repo: ErrorReportRepository) {
        this.repo = repo
    }

    // --- Hereinkommen --------------------------------------------------------

    /**
     * Eine Antwort des Servers. Gemeldet wird nur, was niemand wollte.
     *
     * 400, 401, 403, 409 und 429 sind Regeln bei der Arbeit: zu kurzer Text,
     * fehlende Rolle, jemand war schneller, zu viel auf einmal. Sie stehen
     * dort, wo sie hingehören, und ein Bericht darüber wäre Rauschen, in dem
     * die echte Störung untergeht.
     */
    override fun antwort(methode: String, pfad: String, code: Int) {
        if (istEigenerPfad(pfad)) return
        val vorfall = when {
            code in 500..599 -> ErrorIncident(
                kind = ErrorReportKind.SERVER,
                message = texte.serverfehler(code),
                detail = "HTTP $code · $methode $pfad",
                area = AreaNames.of(pfad),
                occurredAt = uhr(),
            )

            code == 404 -> ErrorIncident(
                kind = ErrorReportKind.UNEXPECTED,
                message = texte.nichtGefunden,
                detail = "HTTP 404 · $methode $pfad",
                area = AreaNames.of(pfad),
                occurredAt = uhr(),
            )

            else -> null
        }
        vorfall?.let { report(it) }
    }

    /** Die Anfrage kam gar nicht erst durch. */
    override fun fehlschlag(methode: String, pfad: String, grund: String) {
        if (istEigenerPfad(pfad)) return
        report(
            ErrorIncident(
                kind = ErrorReportKind.NETWORK,
                message = texte.ohneVerbindung,
                detail = listOf(grund, "$methode $pfad").filter { it.isNotBlank() }
                    .joinToString(" · "),
                area = AreaNames.of(pfad),
                occurredAt = uhr(),
            ),
        )
    }

    /**
     * Stellt einen Vorfall vor die Person.
     *
     * Ein Bericht, der gerade unterwegs ist, wird nicht überschrieben — sonst
     * verschluckte ein zweiter Fehler das „Danke" des ersten, und niemand
     * wüsste, ob etwas angekommen ist.
     */
    fun report(vorfall: ErrorIncident) {
        if (_state.value.sendet) return
        _state.value = ErrorReportUiState(vorfall = vorfall)
    }

    /**
     * Die Person hat es gelesen und will es nicht melden. Das ist eine
     * gültige Antwort, und sie kostet nichts.
     */
    fun dismiss() {
        _state.value = ErrorReportUiState()
    }

    // --- Abschicken ----------------------------------------------------------

    /**
     * Schickt den Bericht. [comment] ist freiwillig und meistens leer — ein
     * Fingertipp hilft genauso wie ein geschriebener Satz.
     */
    suspend fun send(comment: String = "") {
        val stand = _state.value
        val vorfall = stand.vorfall ?: return
        val repo = this.repo ?: return
        if (stand.sendet) return
        _state.update { it.copy(sendet = true, sendefehler = null) }
        runCatching { repo.send(inputFor(vorfall, comment)) }
            .onSuccess {
                _state.update { it.copy(sendet = false, gesendet = true, sendefehler = null) }
            }
            .onFailure {
                // Ein gescheitertes Abschicken wird gesagt, nicht verschluckt —
                // und der Vorfall bleibt stehen, damit es ein zweiter Versuch
                // noch gibt.
                _state.update {
                    it.copy(
                        sendet = false,
                        sendefehler = texte.abschickenGescheitert,
                    )
                }
            }
    }

    /**
     * Genau das, was hinausgeht — dieselben Werte, aus denen die Anfrage
     * gebaut wird. Die Aufstellung im Blatt ist damit kein Versprechen,
     * sondern die Sache selbst.
     */
    fun inputFor(vorfall: ErrorIncident, comment: String = ""): ErrorReportInput =
        ErrorReportInput(
            kind = vorfall.kind.wire,
            message = vorfall.message,
            detail = vorfall.detail,
            comment = comment.trim(),
            area = vorfall.area,
            platform = "android",
            appVersion = facts.appVersion,
            osVersion = facts.osVersion,
            deviceModel = facts.deviceModel,
            occurredAt = rfc3339(vorfall.occurredAt),
        )

    /** Der Inhalt eines Berichts, Zeile für Zeile — für „Das wird geschickt". */
    fun contentLines(input: ErrorReportInput, occurredAt: Long): List<Pair<String, String>> =
        buildList {
            add(texte.zeileWas to input.message)
            if (input.area.isNotBlank()) {
                add(texte.zeileBereich to input.area)
            }
            if (input.detail.isNotBlank()) {
                add(texte.zeileTechnisch to input.detail)
            }
            if (input.comment.isNotBlank()) {
                add(texte.zeileDeinText to input.comment)
            }
            add(texte.zeileApp to input.appVersion)
            add(
                texte.zeileGeraet to
                    listOf(input.deviceModel, input.osVersion).filter { it.isNotBlank() }
                        .joinToString(", "),
            )
            add(texte.zeileWann to anzeige(occurredAt))
        }

    private fun istEigenerPfad(path: String) = path.contains(EIGENER_PFAD)

    companion object {
        /**
         * Der Pfad des Eingangs selbst. Was dort scheitert, darf keinen
         * weiteren Bericht erzeugen — sonst dreht sich das im Kreis.
         */
        const val EIGENER_PFAD = "error-reports"

        private val RFC3339: DateTimeFormatter =
            DateTimeFormatter.ofPattern("yyyy-MM-dd'T'HH:mm:ss'Z'").withZone(ZoneId.of("UTC"))
        private val ANZEIGE: DateTimeFormatter =
            DateTimeFormatter.ofPattern("dd.MM.yyyy, HH:mm").withZone(ZoneId.of("Europe/Berlin"))

        fun rfc3339(millis: Long): String = RFC3339.format(Instant.ofEpochMilli(millis))
        fun anzeige(millis: Long): String = ANZEIGE.format(Instant.ofEpochMilli(millis))
    }
}

/** Was der Hinweis am unteren Rand gerade zeigt. */
data class ErrorReportUiState(
    val vorfall: ErrorIncident? = null,
    val sendet: Boolean = false,
    /** Nach dem Abschicken: „Danke, der Bericht ist angekommen." */
    val gesendet: Boolean = false,
    /** Das Abschicken selbst ist gescheitert. */
    val sendefehler: String? = null,
)

/**
 * Die sichtbaren Texte des Melders.
 *
 * Sie stehen wie alle Oberflächentexte in `res/values/strings.xml` und werden
 * hier einmal eingesammelt, statt einen `Context` durch den Melder zu
 * schleifen. Damit bleibt der Melder ohne Android-Laufzeit prüfbar — und die
 * Texte bleiben trotzdem an ihrem Platz.
 */
data class ErrorReportTexte(
    val ohneVerbindung: String,
    val nichtGefunden: String,
    val abschickenGescheitert: String,
    val zeileWas: String,
    val zeileBereich: String,
    val zeileTechnisch: String,
    val zeileDeinText: String,
    val zeileApp: String,
    val zeileGeraet: String,
    val zeileWann: String,
    /** Erwartet die Statuszahl als einzigen Platzhalter. */
    val serverfehlerVorlage: String,
) {
    fun serverfehler(code: Int): String = String.format(serverfehlerVorlage, code)
}
