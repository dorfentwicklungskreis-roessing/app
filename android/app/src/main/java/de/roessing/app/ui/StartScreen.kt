package de.roessing.app.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.Build
import androidx.compose.material.icons.filled.Group
import androidx.compose.material.icons.filled.LocalFlorist
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.outlined.Lightbulb
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import de.roessing.app.R
import de.roessing.app.ui.theme.statusFarben

/**
 * Die Bereiche der App. „Mithelfen" ist der erste — weitere kommen, deshalb
 * ist die Startseite eine Übersicht und nicht die Gieß-Karte.
 */
enum class Bereich { START, MITHELFEN, PROFIL, DORFBEWOHNER, IDEEN, VERWALTUNG }

/**
 * Startseite: freundlicher Einstieg mit dem Namen aus dem Profil, darunter
 * die Bereiche. Die Kachel „Mithelfen" trägt eine kurze Statuszeile, damit
 * die Seite etwas zu erzählen hat, ohne dass man erst hineingehen muss.
 */
@Composable
fun StartScreen(
    name: String,
    faelligeOrte: Int,
    ladend: Boolean,
    modifier: Modifier = Modifier,
    /** Nur Verwaltende sehen die Kachel „Verwaltung". */
    istVerwaltung: Boolean = false,
    notifications: List<de.roessing.app.data.NotificationDto> = emptyList(),
    pendingAssignments: Set<Long> = emptySet(),
    meineVorgaenge: Set<Long> = emptySet(),
    onClaim: (Long) -> Unit = {},
    onRelease: (Long) -> Unit = {},
    onAck: (Long) -> Unit = {},
    onBereich: (Bereich) -> Unit,
) {
    Column(
        modifier
            .fillMaxWidth()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp)
            .padding(top = 8.dp, bottom = 28.dp)
            .testTag("startseite"),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(
                if (name.isBlank()) {
                    stringResource(R.string.home_greeting_anon)
                } else {
                    stringResource(R.string.home_greeting, name)
                },
                style = MaterialTheme.typography.headlineLarge,
                modifier = Modifier.testTag("begruessung"),
            )
            Text(
                stringResource(R.string.home_intro),
                style = MaterialTheme.typography.bodyLarge,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }

        // Benachrichtigungen stehen oben: Sie sind der Grund, die App
        // überhaupt zu öffnen — jemand wartet auf eine Antwort.
        BenachrichtigungenAbschnitt(
            notifications = notifications,
            pendingAssignments = pendingAssignments,
            meineVorgaenge = meineVorgaenge,
            onClaim = onClaim,
            onRelease = onRelease,
            onAck = onAck,
        )

        MithelfenKachel(
            faelligeOrte = faelligeOrte,
            ladend = ladend,
            offeneAnfragen = notifications.count { it.istAnfrage },
            onClick = { onBereich(Bereich.MITHELFEN) },
        )

        Row(
            Modifier.height(IntrinsicSize.Min),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            BereichKachel(
                titel = stringResource(R.string.profile_title),
                text = stringResource(R.string.area_profile_subtitle),
                symbol = Icons.Filled.Person,
                testTag = "bereich-profil",
                modifier = Modifier.weight(1f),
                onClick = { onBereich(Bereich.PROFIL) },
            )
            BereichKachel(
                titel = stringResource(R.string.members_title),
                text = stringResource(R.string.area_members_subtitle),
                symbol = Icons.Filled.Group,
                testTag = "bereich-dorfbewohner",
                modifier = Modifier.weight(1f),
                onClick = { onBereich(Bereich.DORFBEWOHNER) },
            )
        }

        IdeenKachel(onClick = { onBereich(Bereich.IDEEN) })

        // Die Verwaltung steht unten und nur für die, die sie brauchen. Das
        // Backend weist Änderungen ohne die Rolle „admin" ohnehin ab — die
        // Kachel ist die Bequemlichkeit, nicht die Absicherung.
        if (istVerwaltung) {
            BereichKachel(
                titel = stringResource(R.string.area_admin_title),
                text = stringResource(R.string.area_admin_subtitle),
                symbol = Icons.Filled.Build,
                testTag = "bereich-verwaltung",
                onClick = { onBereich(Bereich.VERWALTUNG) },
            )
        }
    }
}

/**
 * Die Kachel des ersten Bereichs — größer als die anderen, weil dahinter
 * heute die meiste Arbeit steckt, aber eben eine Kachel unter mehreren.
 */
@Composable
private fun MithelfenKachel(
    faelligeOrte: Int,
    ladend: Boolean,
    offeneAnfragen: Int,
    onClick: () -> Unit,
) {
    Card(
        onClick = onClick,
        shape = MaterialTheme.shapes.extraLarge,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.primaryContainer,
            contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
        ),
        modifier = Modifier
            .fillMaxWidth()
            .testTag("bereich-mithelfen"),
    ) {
        Column(Modifier.padding(20.dp), verticalArrangement = Arrangement.spacedBy(14.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Symbolkreis(
                    symbol = Icons.Filled.LocalFlorist,
                    hintergrund = MaterialTheme.colorScheme.primary,
                    vordergrund = MaterialTheme.colorScheme.onPrimary,
                    groesse = 52,
                )
                Spacer(Modifier.width(16.dp))
                Column(Modifier.weight(1f)) {
                    Text(
                        stringResource(R.string.area_care_title),
                        style = MaterialTheme.typography.headlineSmall,
                    )
                    Text(
                        stringResource(R.string.area_care_subtitle),
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
                Icon(
                    Icons.AutoMirrored.Filled.ArrowForward,
                    contentDescription = null,
                    modifier = Modifier.size(22.dp),
                )
            }
            Statuszeile(faelligeOrte = faelligeOrte, ladend = ladend, offeneAnfragen = offeneAnfragen)
        }
    }
}

/** „2 Orte warten auf dich" bzw. „Alles erledigt — danke!" */
@Composable
private fun Statuszeile(faelligeOrte: Int, ladend: Boolean, offeneAnfragen: Int) {
    val farben = statusFarben
    val alles = faelligeOrte == 0 && !ladend && offeneAnfragen == 0
    val text = when {
        // Eine Anfrage an mich geht allem anderen vor: Da wartet jemand.
        offeneAnfragen > 0 -> anfragenZeile(offeneAnfragen).orEmpty()
        ladend -> stringResource(R.string.area_care_loading)
        alles -> stringResource(R.string.area_care_done)
        else -> pluralStringResource(R.plurals.area_care_due, faelligeOrte, faelligeOrte)
    }
    Surface(
        shape = MaterialTheme.shapes.large,
        color = if (alles) farben.gruenFlaeche else farben.gelbFlaeche,
        contentColor = if (alles) farben.gruen else farben.gelb,
        modifier = Modifier.testTag("mithelfen-status"),
    ) {
        Row(
            Modifier.padding(horizontal = 16.dp, vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                Modifier
                    .size(10.dp)
                    .clip(CircleShape)
                    .background(if (alles) farben.gruen else farben.gelb),
            )
            Spacer(Modifier.width(10.dp))
            Text(text, style = MaterialTheme.typography.labelLarge)
        }
    }
}

/** Gleich große Kachel für die kleineren Bereiche. */
@Composable
private fun BereichKachel(
    titel: String,
    text: String,
    symbol: ImageVector,
    testTag: String,
    modifier: Modifier = Modifier,
    onClick: () -> Unit,
) {
    Card(
        onClick = onClick,
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.surfaceContainerHigh,
        ),
        modifier = modifier
            .fillMaxWidth()
            .testTag(testTag),
    ) {
        Column(
            Modifier.padding(16.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
            Symbolkreis(
                symbol = symbol,
                hintergrund = MaterialTheme.colorScheme.secondaryContainer,
                vordergrund = MaterialTheme.colorScheme.onSecondaryContainer,
                groesse = 44,
            )
            Text(titel, style = MaterialTheme.typography.titleMedium)
            Text(
                text,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
        }
    }
}

/**
 * Der ehrliche Ausblick — und gleichzeitig der Weg dorthin: Es kommt mehr,
 * und was als Nächstes kommt, entscheidet das Dorf. Deshalb ist die Kachel
 * anklickbar und führt direkt in das Formular „Was soll die App noch
 * können?". Versprochen wird hier nichts.
 */
@Composable
private fun IdeenKachel(onClick: () -> Unit) {
    Card(
        onClick = onClick,
        shape = MaterialTheme.shapes.large,
        colors = CardDefaults.cardColors(
            containerColor = MaterialTheme.colorScheme.tertiaryContainer,
            contentColor = MaterialTheme.colorScheme.onTertiaryContainer,
        ),
        modifier = Modifier
            .fillMaxWidth()
            .testTag("bereich-ideen"),
    ) {
        Row(Modifier.padding(16.dp), verticalAlignment = Alignment.CenterVertically) {
            Symbolkreis(
                symbol = Icons.Outlined.Lightbulb,
                hintergrund = MaterialTheme.colorScheme.tertiary,
                vordergrund = MaterialTheme.colorScheme.onTertiary,
                groesse = 44,
            )
            Spacer(Modifier.width(14.dp))
            Column(Modifier.weight(1f)) {
                Text(
                    stringResource(R.string.area_ideas_title),
                    style = MaterialTheme.typography.titleMedium,
                )
                Text(
                    stringResource(R.string.area_ideas_subtitle),
                    style = MaterialTheme.typography.bodyMedium,
                )
                Text(
                    stringResource(R.string.home_more_text),
                    style = MaterialTheme.typography.bodySmall,
                )
            }
            Icon(
                Icons.AutoMirrored.Filled.ArrowForward,
                contentDescription = null,
                modifier = Modifier.size(22.dp),
            )
        }
    }
}

/** Rundes Symbolfeld — gibt den Kacheln ihren Wiedererkennungswert. */
@Composable
private fun Symbolkreis(
    symbol: ImageVector,
    hintergrund: Color,
    vordergrund: Color,
    groesse: Int,
) {
    Box(
        Modifier
            .size(groesse.dp)
            .clip(CircleShape)
            .background(hintergrund),
        contentAlignment = Alignment.Center,
    ) {
        Icon(symbol, contentDescription = null, tint = vordergrund, modifier = Modifier.size((groesse * 0.5).dp))
    }
}
