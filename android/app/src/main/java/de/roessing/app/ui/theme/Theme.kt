package de.roessing.app.ui.theme

import android.os.Build
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.SpringSpec
import androidx.compose.animation.core.spring
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.LineHeightStyle
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp

// Statusfarben der Karte. Sie liegen auf hellen Kartenkacheln und bleiben
// deshalb in beiden Designs gleich kräftig — MapScreen nutzt dieselben Werte
// als Hex-Literale für die MapLibre-Ausdrücke.
val StatusGreen = Color(0xFF2E7D32)
val StatusYellow = Color(0xFFF9A825)
val StatusRed = Color(0xFFC62828)

/**
 * Statusfarben der Oberfläche — je Design ein eigener Satz.
 *
 * Auf dunklem Grund sind die satten Kartenfarben schlecht lesbar (das dunkle
 * Rot kommt auf #10140F nicht mal auf den 4,5:1 des WCAG-Mindestwerts).
 * Deshalb gibt es aufgehellte Varianten und dazu passende Container-Farben
 * für die Status-Plaketten.
 */
data class StatusFarben(
    val gruen: Color,
    val gelb: Color,
    val rot: Color,
    /** Außer Dienst: grau. Was nicht ansteht, fordert zu nichts auf. */
    val ruhend: Color,
    val gruenFlaeche: Color,
    val gelbFlaeche: Color,
    val rotFlaeche: Color,
    val ruhendFlaeche: Color,
)

private val StatusHell = StatusFarben(
    gruen = Color(0xFF1B5E20),
    gelb = Color(0xFF7A5200),
    rot = Color(0xFFB3261E),
    ruhend = Color(0xFF4A4E52),
    gruenFlaeche = Color(0xFFD7F0D3),
    gelbFlaeche = Color(0xFFFDECC8),
    rotFlaeche = Color(0xFFFCDAD6),
    ruhendFlaeche = Color(0xFFE6E8EA),
)

private val StatusDunkel = StatusFarben(
    gruen = Color(0xFF9BE3A2),
    gelb = Color(0xFFFFD479),
    rot = Color(0xFFFFB4AB),
    ruhend = Color(0xFFC4C8CC),
    gruenFlaeche = Color(0xFF23422A),
    gelbFlaeche = Color(0xFF4A3A12),
    rotFlaeche = Color(0xFF5C2420),
    ruhendFlaeche = Color(0xFF34383C),
)

val LocalStatusFarben = staticCompositionLocalOf { StatusHell }

/** Statusfarben der aktuellen Oberfläche. */
val statusFarben: StatusFarben
    @Composable @ReadOnlyComposable
    get() = LocalStatusFarben.current

// Dorf-Palette: sattes Grün mit warmem Akzent — passt zu Wiesen und Fachwerk.
private val LightColors = lightColorScheme(
    primary = Color(0xFF3B6939),
    onPrimary = Color(0xFFFFFFFF),
    primaryContainer = Color(0xFFBCF0B4),
    onPrimaryContainer = Color(0xFF002204),
    secondary = Color(0xFF52634F),
    onSecondary = Color(0xFFFFFFFF),
    secondaryContainer = Color(0xFFD5E8CF),
    onSecondaryContainer = Color(0xFF101F10),
    tertiary = Color(0xFF7B5733),
    onTertiary = Color(0xFFFFFFFF),
    tertiaryContainer = Color(0xFFFFDCBE),
    onTertiaryContainer = Color(0xFF2C1600),
    background = Color(0xFFF7FBF1),
    surface = Color(0xFFF7FBF1),
    surfaceContainer = Color(0xFFEBEFE5),
    surfaceContainerHigh = Color(0xFFE5EADF),
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFFA1D399),
    onPrimary = Color(0xFF0A390F),
    primaryContainer = Color(0xFF235024),
    onPrimaryContainer = Color(0xFFBCF0B4),
    secondary = Color(0xFFB9CCB4),
    onSecondary = Color(0xFF243424),
    tertiary = Color(0xFFEDBE91),
    onTertiary = Color(0xFF452B09),
    background = Color(0xFF10140F),
    surface = Color(0xFF10140F),
    surfaceContainer = Color(0xFF1C201A),
    surfaceContainerHigh = Color(0xFF262B24),
)

/**
 * Großzügige Rundungen. Material 3 Expressive lebt von weichen, deutlich
 * gerundeten Flächen — die alten Standardwerte (4/8/12/16/28 dp) wirken
 * dagegen kantig und eng.
 */
private val DorfShapes = Shapes(
    extraSmall = RoundedCornerShape(10.dp),
    small = RoundedCornerShape(14.dp),
    medium = RoundedCornerShape(20.dp),
    large = RoundedCornerShape(28.dp),
    extraLarge = RoundedCornerShape(36.dp),
)

private val zeilenAusgleich = LineHeightStyle(
    alignment = LineHeightStyle.Alignment.Center,
    trim = LineHeightStyle.Trim.None,
)

/**
 * Typografie mit mehr Ausdruck: Überschriften kräftiger und enger gesetzt,
 * Fließtext etwas luftiger. Bewusst mit der Systemschrift — eine mitgelieferte
 * Schriftdatei würde das APK aufblähen und die Lesbarkeit nicht verbessern.
 */
private val DorfTypography = Typography().let { basis ->
    fun TextStyle.ausdruck(weight: FontWeight, tracking: Float) =
        copy(fontWeight = weight, letterSpacing = tracking.sp, lineHeightStyle = zeilenAusgleich)
    basis.copy(
        displaySmall = basis.displaySmall.ausdruck(FontWeight.Bold, (-0.5).toFloat()),
        headlineLarge = basis.headlineLarge.ausdruck(FontWeight.Bold, (-0.5).toFloat()),
        headlineMedium = basis.headlineMedium.ausdruck(FontWeight.Bold, (-0.4).toFloat()),
        headlineSmall = basis.headlineSmall.ausdruck(FontWeight.SemiBold, (-0.2).toFloat()),
        titleLarge = basis.titleLarge.ausdruck(FontWeight.SemiBold, (-0.2).toFloat()),
        titleMedium = basis.titleMedium.ausdruck(FontWeight.SemiBold, 0f),
        labelLarge = basis.labelLarge.ausdruck(FontWeight.SemiBold, 0.1f),
        bodyLarge = basis.bodyLarge.copy(lineHeight = 26.sp, lineHeightStyle = zeilenAusgleich),
        bodyMedium = basis.bodyMedium.copy(lineHeight = 22.sp, lineHeightStyle = zeilenAusgleich),
    )
}

/**
 * Bewegungskurven im Geist von Material 3 Expressive: Lagewechsel federn
 * leicht nach (statt linear zu gleiten), Ein- und Ausblenden bleibt ruhig.
 * Eigene Federn statt `MotionScheme`, weil dessen API in 1.4.0 nicht
 * öffentlich ist — die Werte sind an die Expressive-Vorgaben angelehnt.
 */
object DorfMotion {
    /** Für Bewegung im Raum (Wechsel zwischen Bereichen, Größenänderungen). */
    fun <T> raeumlich(): SpringSpec<T> =
        spring(dampingRatio = 0.75f, stiffness = Spring.StiffnessMediumLow)

    /** Für Farbe und Deckkraft — hier wäre Nachfedern nur Unruhe. */
    fun <T> effekt(): SpringSpec<T> =
        spring(dampingRatio = Spring.DampingRatioNoBouncy, stiffness = Spring.StiffnessMedium)
}

@Composable
fun DorfAppTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    val context = LocalContext.current
    val colorScheme: ColorScheme = when {
        // Dynamische Farben ab Android 12 — die App fühlt sich „wie vom Gerät" an.
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.S ->
            if (darkTheme) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        darkTheme -> DarkColors
        else -> LightColors
    }
    CompositionLocalProvider(
        LocalStatusFarben provides if (darkTheme) StatusDunkel else StatusHell,
    ) {
        // Bewusst das reguläre MaterialTheme: MaterialExpressiveTheme und
        // MotionScheme sind in material3 1.4.0 noch `internal` — öffentlich
        // gibt es sie erst in 1.5.0-alpha. Der Ausdruck kommt deshalb aus
        // eigenen Formen, eigener Typografie und eigenen Bewegungskurven
        // (siehe DorfMotion), nicht aus einer Alpha-Abhängigkeit.
        MaterialTheme(
            colorScheme = colorScheme,
            shapes = DorfShapes,
            typography = DorfTypography,
            content = content,
        )
    }
}
