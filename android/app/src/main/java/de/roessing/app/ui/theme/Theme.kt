package de.roessing.app.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext

// Statusfarben der Blumenkästen (bewusst kräftig, auch auf der Karte gut lesbar).
val StatusGreen = Color(0xFF2E7D32)
val StatusYellow = Color(0xFFF9A825)
val StatusRed = Color(0xFFC62828)

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
)

@Composable
fun DorfAppTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    val context = LocalContext.current
    val colorScheme = when {
        // Dynamische Farben ab Android 12 — die App fühlt sich „wie vom Gerät" an.
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.S ->
            if (darkTheme) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        darkTheme -> DarkColors
        else -> LightColors
    }
    MaterialTheme(colorScheme = colorScheme, content = content)
}
