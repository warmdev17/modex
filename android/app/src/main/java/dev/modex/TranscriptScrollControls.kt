package dev.modex

import android.animation.ValueAnimator
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.OutlinedIconButton
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp

internal suspend fun LazyListState.returnToLatest() {
    val layout = layoutInfo
    if (layout.totalItemsCount == 0) return
    val bottom = layout.totalItemsCount - 1
    val travel = layout.viewportSize.height / 2
    val lastVisible = layout.visibleItemsInfo.lastOrNull()
    if (lastVisible == null || lastVisible.index < bottom - 1 ||
        lastVisible.offset + lastVisible.size > layout.viewportEndOffset + travel
    ) {
        scrollToItem(bottom, -(layout.viewportEndOffset + travel))
    }
    if (ValueAnimator.areAnimatorsEnabled()) animateScrollToItem(bottom) else scrollToItem(bottom)
}

@Composable
internal fun BoxScope.ScrollToLatestButton(visible: Boolean, onClick: () -> Unit) {
    if (!visible) return
    OutlinedIconButton(
        onClick = onClick,
        modifier = Modifier.align(Alignment.BottomCenter).padding(bottom = 8.dp).size(48.dp),
        shape = RoundedCornerShape(12.dp),
        border = BorderStroke(1.dp, Mocha.Overlay),
        colors = IconButtonDefaults.outlinedIconButtonColors(
            containerColor = Mocha.Mantle,
            contentColor = Mocha.Text,
        ),
    ) { ChatGlyph(ChatIcon.Down, "Scroll to latest") }
}
