package dev.modex

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class NotificationPermissionTest {
    @Test fun `first manual successful connection offers notification explanation on Android 13`() {
        assertTrue(shouldOfferNotificationPermission(33, seen = false, manual = true, connected = true, granted = false))
    }

    @Test fun `passive failed and repeated connections do not offer notification explanation`() {
        assertFalse(shouldOfferNotificationPermission(33, seen = false, manual = false, connected = true, granted = false))
        assertFalse(shouldOfferNotificationPermission(33, seen = false, manual = true, connected = false, granted = false))
        assertFalse(shouldOfferNotificationPermission(33, seen = true, manual = true, connected = true, granted = false))
        assertFalse(shouldOfferNotificationPermission(33, seen = false, manual = true, connected = true, granted = true))
        assertFalse(shouldOfferNotificationPermission(32, seen = false, manual = true, connected = true, granted = false))
    }

    @Test fun `settings requests permission while Android can still show its prompt`() {
        assertEquals(NotificationSettingsAction.RequestPermission,
            notificationSettingsAction(33, granted = false, requested = false, rationale = false))
        assertEquals(NotificationSettingsAction.RequestPermission,
            notificationSettingsAction(33, granted = false, requested = true, rationale = true))
    }

    @Test fun `settings falls back to system settings when permission prompt is unavailable`() {
        assertEquals(NotificationSettingsAction.OpenSystemSettings,
            notificationSettingsAction(33, granted = false, requested = true, rationale = false))
        assertEquals(NotificationSettingsAction.OpenSystemSettings,
            notificationSettingsAction(32, granted = true, requested = false, rationale = false))
    }
}
