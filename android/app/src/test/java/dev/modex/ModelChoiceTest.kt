package dev.modex

import org.junit.Assert.*
import org.junit.Test

class ModelChoiceTest {
    @Test fun savedChoiceWinsOverHostAndResumeDefaultsAcrossSerialization() {
        val original = mapOf("a" to ModelChoice("custom", "high"), "b" to ModelChoice("host", "low"))
        val restored = readModelChoices(modelChoicesJson(original))
        assertEquals(original, restored)
        assertEquals(original["a"], resolveModelChoice(restored["a"], ModelChoice("host", "low"), ModelChoice("new-host", "medium"), emptyList()))
        assertEquals("low", restored.getValue("b").effort)
    }
    @Test fun newThreadUsesHostAndCustomModelUsesItsOwnDefault() {
        val host = ModelChoice("host", "low")
        val models = listOf(ModelOption("custom", "Custom", "high", listOf("high" to "Deep")))
        assertEquals(host, resolveModelChoice(null, null, host, models))
        assertEquals(ModelChoice("custom", "high"), resolveModelChoice(ModelChoice("custom", ""), null, host, models))
        assertEquals("", resolveModelChoice(ModelChoice("unknown", ""), null, host, models).effort)
    }
}
