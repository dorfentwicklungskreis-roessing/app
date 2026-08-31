package de.roessing.app

import de.roessing.app.data.PlaceDto
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Wer sich um einen Ort kümmert, kommt fertig vom Server — samt Verdeckung:
 * Eine geschlossene Gruppe heißt für Außenstehende „Eine Gruppe aus dem Dorf".
 * Die App entscheidet daran nichts, sonst gäbe es die Sichtbarkeitsregel
 * zweimal.
 */
class TraegerAmOrtTest {
    private val json = Json { ignoreUnknownKeys = true; coerceInputValues = true }

    @Test
    fun `Traegername wird gelesen`() {
        val ort = json.decodeFromString<PlaceDto>(
            """{"id":3,"name":"Beet vor dem Dorfgemeinschaftshaus","lat":52.18,"lon":9.81,
                "traegerName":"AK 2 Umwelt und Natur"}""",
        )
        assertEquals("AK 2 Umwelt und Natur", ort.traegerName)
    }

    /**
     * Ältere Stände des Backends schicken das Feld nicht mit. Dann bleibt die
     * Zeile leer, statt dass die ganze Liste unlesbar wird.
     */
    @Test
    fun `ohne Traeger bleibt es leer`() {
        val ort = json.decodeFromString<PlaceDto>(
            """{"id":1,"name":"Unter den Eichen","lat":52.18,"lon":9.81}""",
        )
        assertTrue(ort.traegerName.isEmpty())
    }
}
