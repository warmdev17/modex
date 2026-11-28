package dev.modex

import androidx.compose.foundation.layout.size
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.unit.dp

private fun family(pattern: String) = Regex(
    "(?<![\\p{L}\\p{N}])(?:$pattern)[0-9]*(?![\\p{L}\\p{N}])",
    RegexOption.IGNORE_CASE,
)

internal enum class ModelBrand(val mark: Int, val pattern: Regex? = null) {
    Claude(R.drawable.ic_model_claude, family("claude|opus|sonnet|haiku")),
    Gemini(R.drawable.ic_model_gemini, family("gemini")),
    DeepSeek(R.drawable.ic_model_deepseek, family("deepseek")),
    Qwen(R.drawable.ic_model_qwen, family("qwen|qwq")),
    Kimi(R.drawable.ic_model_kimi, family("kimi|moonshot")),
    GLM(R.drawable.ic_model_zhipu, family("glm|chatglm")),
    Llama(R.drawable.ic_model_meta, family("llama")),
    Mistral(R.drawable.ic_model_mistral, family("mistral|mixtral|ministral|devstral|codestral|pixtral|magistral")),
    OpenAI(R.drawable.ic_openai, family("openai|gpt(?:[0-9]+o)?|codex|o[1-9]")),
    Unknown(0),
}

internal fun modelBrand(id: String, label: String = ""): ModelBrand {
    fun from(name: String) = ModelBrand.entries.firstOrNull { it.pattern?.containsMatchIn(name) == true }
    return from(id.substringAfterLast('/'))
        ?: label.takeUnless { it.equals(id, ignoreCase = true) }?.let(::from)
        ?: ModelBrand.Unknown
}

@Composable
internal fun ModelLogo(brand: ModelBrand) {
    if (brand == ModelBrand.Unknown) ChatGlyph(ChatIcon.ModelChoice)
    else Icon(painterResource(brand.mark), contentDescription = null, modifier = Modifier.size(22.dp))
}
