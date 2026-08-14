package de.roessing.app

import de.roessing.app.data.CompletionDto
import de.roessing.app.data.TaskDto
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Einmalig ist einmalig: Ist der Gang zum Bahnhof getan, gibt es nichts mehr
 * zu melden. Ohne den Schalter „nach dem Erledigen entfernen" bleibt die
 * Aufgabe sichtbar — der Knopf darf dann trotzdem nicht mehr angeboten
 * werden, sonst läuft man in ein 409 des Backends.
 */
class TaskDtoTest {
    private val erledigung = CompletionDto(id = 1, taskId = 1, userName = "Erna", doneAt = "2026-08-12T10:00:00Z")

    @Test
    fun `erledigte einmalige Aufgabe ist vorbei`() {
        val aufgabe = TaskDto(id = 1, kind = "sonstiges", oneOff = true, lastCompletion = erledigung)
        assertTrue(aufgabe.erledigtUndVorbei)
    }

    @Test
    fun `noch offene einmalige Aufgabe ist nicht vorbei`() {
        val aufgabe = TaskDto(id = 1, kind = "sonstiges", oneOff = true)
        assertFalse(aufgabe.erledigtUndVorbei)
    }

    @Test
    fun `eine regelmaessige Aufgabe ist nach dem Giessen nicht vorbei`() {
        val aufgabe = TaskDto(id = 1, kind = "giessen", lastCompletion = erledigung)
        assertFalse(aufgabe.erledigtUndVorbei)
    }
}
