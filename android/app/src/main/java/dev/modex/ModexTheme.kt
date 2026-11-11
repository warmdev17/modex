package dev.modex

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.*
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.unit.dp

internal object Mocha {
    val Base = Color(0xFF1E1E2E)
    val Mantle = Color(0xFF181825)
    val Surface = Color(0xFF313244)
    val Overlay = Color(0xFF585B70)
    val Text = Color(0xFFCDD6F4)
    val Subtext = Color(0xFFBAC2DE)
    val Blue = Color(0xFF89B4FA)
    val Red = Color(0xFFF38BA8)
}

@Composable
internal fun ModexTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = darkColorScheme(
            primary = Mocha.Blue, onPrimary = Mocha.Mantle,
            primaryContainer = Color(0xFF28384F), onPrimaryContainer = Mocha.Blue,
            background = Mocha.Base, onBackground = Mocha.Text,
            surface = Mocha.Base, onSurface = Mocha.Text, onSurfaceVariant = Mocha.Subtext,
            surfaceContainer = Mocha.Mantle, surfaceContainerHighest = Mocha.Surface,
            outline = Mocha.Overlay, outlineVariant = Mocha.Surface,
            error = Mocha.Red, errorContainer = Color(0xFF402C3C), onErrorContainer = Mocha.Text
        ),
        shapes = Shapes(small = RoundedCornerShape(8.dp), medium = RoundedCornerShape(10.dp), large = RoundedCornerShape(14.dp), extraLarge = RoundedCornerShape(14.dp)),
        content = content
    )
}
