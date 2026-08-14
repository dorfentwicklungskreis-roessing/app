package de.roessing.app

import de.roessing.app.data.IdeeDto
import de.roessing.app.data.IdeeInput
import de.roessing.app.data.IdeenAblehnungException
import de.roessing.app.data.IdeenRepository

/**
 * Ideen-Eingang für die Oberflächentests: merkt sich, was abgeschickt wurde,
 * und kann eine Ablehnung des Backends nachstellen.
 */
class FakeIdeen(private val ablehnung: String? = null) : IdeenRepository {
    val geschickt = mutableListOf<IdeeInput>()

    override suspend fun einreichen(input: IdeeInput): IdeeDto {
        ablehnung?.let { throw IdeenAblehnungException(it) }
        geschickt += input
        return IdeeDto(id = geschickt.size.toLong(), wunsch = input.wunsch, name = input.name, email = input.email)
    }
}
