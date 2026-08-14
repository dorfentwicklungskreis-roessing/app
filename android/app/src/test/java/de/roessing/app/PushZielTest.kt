package de.roessing.app

import de.roessing.app.push.PushKanal
import de.roessing.app.push.PushZiel
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Der Datenteil einer Push-Nachricht sagt der App, wohin der Fingertipp
 * führt. Er kommt vom Backend (internal/push/fcm.go) und besteht
 * ausschließlich aus Zeichenketten — das ist alles, was FCM zulässt.
 */
class PushZielTest {
    private val daten = mapOf(
        "notificationId" to "7",
        "assignmentId" to "3",
        "taskId" to "5",
        "placeId" to "2",
        "kind" to "anfrage",
        "taskKind" to "giessen",
        "placeName" to "Unter den Eichen",
        "taskName" to "Gießen",
        "title" to "Gießen an „Unter den Eichen“ ist dran",
        "body" to "Du bist als Nächste(r) an der Reihe.",
        "expiresAt" to "2026-08-14T12:00:00Z",
    )

    @Test
    fun `liest den Datenteil des Backends`() {
        val ziel = PushZiel.ausDaten(daten)!!
        assertEquals(2L, ziel.placeId)
        assertEquals(5L, ziel.taskId)
        assertEquals(3L, ziel.assignmentId)
        assertEquals(7L, ziel.notificationId)
        assertEquals("anfrage", ziel.kind)
        assertEquals("Gießen an „Unter den Eichen“ ist dran", ziel.titel)
        assertEquals("Du bist als Nächste(r) an der Reihe.", ziel.text)
        assertTrue(ziel.istAnfrage)
    }

    @Test
    fun `Anfragen und Hinweise haben eigene Kanaele`() {
        assertEquals(PushKanal.ANFRAGEN, PushZiel.ausDaten(daten)!!.kanal)
        assertEquals(PushKanal.ANFRAGEN, PushZiel.ausDaten(daten + ("kind" to "rundruf"))!!.kanal)
        for (art in listOf("zusage_abgelaufen", "zusage_aufgehoben", "vorgang_beendet", "vorgang_entfallen")) {
            val ziel = PushZiel.ausDaten(daten + ("kind" to art))!!
            assertEquals(PushKanal.HINWEISE, ziel.kanal)
            assertTrue(!ziel.istAnfrage)
        }
    }

    // Eine Nachricht ohne Ort führt nirgendwohin — dann bleibt es bei der
    // Anzeige, ohne dass die App irgendwo hinspringt.
    @Test
    fun `ohne Ort kein Ziel`() {
        assertNull(PushZiel.ausDaten(daten - "placeId"))
        assertNull(PushZiel.ausDaten(daten + ("placeId" to "keine Zahl")))
        assertNull(PushZiel.ausDaten(emptyMap()))
    }

    // Der Sprung aus der Systembenachrichtigung reicht das Ziel als
    // Zeichenketten durch den Intent — hin und zurück muss dasselbe stehen.
    @Test
    fun `Ziel ueberlebt den Weg durch den Intent`() {
        val ziel = PushZiel.ausDaten(daten)!!
        assertEquals(ziel, PushZiel.ausDaten(ziel.alsDaten()))
    }
}
