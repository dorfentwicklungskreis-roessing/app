package de.roessing.app

import de.roessing.app.data.CareStatus
import de.roessing.app.data.PlaceDto
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Test

/**
 * Eine Aufgabe, die zu dieser Jahreszeit nicht anfällt, ist nicht „grün".
 * Grün hieße „alles gut" — eine Aussage über etwas, das gerade gar nicht
 * ansteht. Das Beet vor dem Dorfgemeinschaftshaus wird im Winter nicht
 * gejätet, und genau das soll dort stehen.
 */
class AusserDienstTest {
    private val json = Json { ignoreUnknownKeys = true; coerceInputValues = true }

    @Test
    fun `ruhender Status wird gelesen`() {
        val ort = json.decodeFromString<PlaceDto>(
            """{"id":3,"name":"Beet","lat":52.18,"lon":9.81,"status":"dormant"}""",
        )
        assertEquals(CareStatus.dormant, ort.careStatus)
    }

    /** Ein Wert, den diese Fassung nicht kennt, darf die Antwort nicht kosten. */
    @Test
    fun `unbekannter Status kostet nicht die Antwort`() {
        val ort = json.decodeFromString<PlaceDto>(
            """{"id":9,"name":"Neu","lat":52.18,"lon":9.81,"status":"irgendwas"}""",
        )
        assertEquals(CareStatus.green, ort.careStatus)
    }
}
