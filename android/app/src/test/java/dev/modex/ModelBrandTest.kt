package dev.modex

import org.junit.Assert.assertEquals
import org.junit.Test

class ModelBrandTest {
    @Test fun modelFamilyWinsOverProviderAndDisplayLabel() {
        assertEquals(ModelBrand.DeepSeek, modelBrand("opencode-go/deepseek-v4", "DeepSeek V4"))
        assertEquals(ModelBrand.Gemini, modelBrand("openai/GEMINI_3_PRO", "Claude Opus"))
        assertEquals(ModelBrand.Claude, modelBrand("gemini/opaque", "Claude Opus 4.6"))
        assertEquals(ModelBrand.Claude, modelBrand("opaque", "Claude Opus 4.6 / Thinking"))
        assertEquals(ModelBrand.OpenAI, modelBrand("gpt-6.2-sol"))
    }

    @Test fun unmatchedNamesUseNeutralIcon() {
        assertEquals(ModelBrand.Unknown, modelBrand("openai/custom", "Superclaude and qwenish"))
        assertEquals(ModelBrand.Unknown, modelBrand("openai/custom", "openai/custom"))
        assertEquals(ModelBrand.Unknown, modelBrand("opus5ish"))
        assertEquals(ModelBrand.Unknown, modelBrand(""))
    }

    @Test fun recognizesAdditionalFamilies() {
        assertEquals(ModelBrand.Qwen, modelBrand("qwen3.5-plus"))
        assertEquals(ModelBrand.Kimi, modelBrand("moonshotai/Kimi-K2"))
        assertEquals(ModelBrand.GLM, modelBrand("z-ai/glm-5"))
        assertEquals(ModelBrand.Llama, modelBrand("meta-llama/llama3.3"))
        assertEquals(ModelBrand.Mistral, modelBrand("devstral-small"))
    }
}
