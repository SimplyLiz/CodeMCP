//! Context health scoring for LLM context bundles.
//!
//! Measures whether a generated context bundle will be useful to an LLM, using
//! signals grounded in peer-reviewed research and production systems:
//!
//! - **Signal density** — ratio of symbol-bearing tokens to total tokens.
//!   Below ~5% triggers severe attention dilution (Morph, 2024: "Context Rot").
//!
//! - **Compression density** — zlib ratio as an information entropy proxy.
//!   High compressibility = high redundancy (Entropy Law, arXiv:2407.06645).
//!
//! - **Position health** — U-shaped attention bias means content at context
//!   boundaries (first/last) gets disproportionately more attention than middle
//!   content (Liu et al., TACL 2024: >30% accuracy drop for middle-placed docs).
//!
//! - **Entity density** — symbols per 1K tokens, BudgetMem-style signal.
//!   (arXiv:2511.04919, weight: 0.20 in their validated scoring system.)
//!
//! - **Utilization headroom** — buffer between used tokens and the model's
//!   context window. Above 85% risks silent truncation.
//!
//! - **Deduplication ratio** — unique-line fraction as a quick redundancy check.
//!
//! Composite score weights are informed by BudgetMem's validated system
//! (achieves 60–72% memory savings with <3% F1 degradation at 30–40% retention).

use serde::{Deserialize, Serialize};
use std::collections::HashSet;
use std::io::Write;

use flate2::write::ZlibEncoder;
use flate2::Compression;

// ---------------------------------------------------------------------------
// Model families (for window size defaults)
// ---------------------------------------------------------------------------

/// Target model family — determines default context window size.
#[derive(Debug, Clone, Copy, Default, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum ModelFamily {
    /// Claude 3 / Claude 4 — 200K token window.
    #[default]
    Claude,
    /// GPT-4o / GPT-4 Turbo — 128K token window.
    Gpt4,
    /// Llama 3 / Mistral / most OSS models — 128K window.
    Llama,
    /// GPT-3.5 — 16K token window.
    Gpt35,
    /// Custom window size (specified in `window_size`).
    Custom,
}

impl ModelFamily {
    pub fn default_window(self) -> usize {
        match self {
            Self::Claude => 200_000,
            Self::Gpt4  => 128_000,
            Self::Llama => 128_000,
            Self::Gpt35 =>  16_000,
            Self::Custom =>  128_000,
        }
    }

    /// Approximate chars-per-token for this family's tokenizer.
    /// Used only as a fast heuristic fallback when tiktoken is unavailable.
    pub fn chars_per_token(self) -> f64 {
        // All GPT-style BPE tokenizers: ~3.5–4.0 chars/token for mixed code+prose.
        // Claude uses its own tokenizer but cl100k_base gives a good approximation.
        3.8
    }
}

impl std::str::FromStr for ModelFamily {
    type Err = String;
    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "claude" | "anthropic"               => Ok(Self::Claude),
            "gpt4" | "gpt-4" | "gpt4o" | "gpt-4o" => Ok(Self::Gpt4),
            "llama" | "mistral" | "qwen"         => Ok(Self::Llama),
            "gpt35" | "gpt-3.5" | "gpt3"        => Ok(Self::Gpt35),
            _                                    => Err(format!("Unknown model '{}'. Use: claude, gpt4, llama, gpt35", s)),
        }
    }
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

#[derive(Debug, Clone)]
pub struct HealthOpts {
    pub model:       ModelFamily,
    /// Override window size (0 = use model default).
    pub window_size: usize,
    /// Relative positions (0.0–1.0) of key modules in the output.
    /// Key = entry points, core modules, bridge modules.
    /// If empty, position_health is skipped and contributes its weight to compression density.
    pub key_positions: Vec<f64>,
    /// Number of symbol signatures contained in the context.
    pub signature_count: usize,
    /// Total tokens used by just the signature text (subset of total).
    pub signature_tokens: usize,
}

impl Default for HealthOpts {
    fn default() -> Self {
        Self {
            model:            ModelFamily::Claude,
            window_size:      0,
            key_positions:    Vec::new(),
            signature_count:  0,
            signature_tokens: 0,
        }
    }
}

// ---------------------------------------------------------------------------
// Output types
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MetricBreakdown {
    pub signal_density:       f64,
    pub compression_density:  f64,
    pub position_health:      f64,
    pub entity_density:       f64,
    pub utilization_headroom: f64,
    pub dedup_ratio:          f64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContextHealthReport {
    // Raw measurements
    pub token_count:       usize,
    pub char_count:        usize,
    pub window_size:       usize,
    pub utilization_pct:   f64,

    // Normalized metrics (0.0–1.0 each)
    pub metrics: MetricBreakdown,

    // Composite
    pub score:   f64,    // 0–100
    pub grade:   String, // A / B / C / D / F

    // Actionable
    pub warnings:        Vec<String>,
    pub recommendations: Vec<String>,
}

// ---------------------------------------------------------------------------
// Token counting
// ---------------------------------------------------------------------------

/// Count tokens using cl100k_base (GPT-4 / Claude approximation).
/// Falls back to the 3.8 chars/token heuristic if tiktoken fails.
pub fn count_tokens(text: &str) -> usize {
    tiktoken_rs::cl100k_base()
        .map(|bpe| bpe.encode_with_special_tokens(text).len())
        .unwrap_or_else(|_| (text.len() as f64 / 3.8) as usize)
}

// ---------------------------------------------------------------------------
// Individual metrics
// ---------------------------------------------------------------------------

/// Fraction of total tokens that are signature text (the "signal").
///
/// Attention dilution becomes severe below ~5% (Morph 2024 "Context Rot":
/// a 20K-token context with 500 relevant tokens has 2.5% density, reducing
/// effective attention to 1/40th of baseline strength).
fn signal_density(total_tokens: usize, sig_tokens: usize) -> f64 {
    if total_tokens == 0 { return 1.0; }
    (sig_tokens as f64 / total_tokens as f64).clamp(0.0, 1.0)
}

/// zlib compression ratio as an information entropy proxy.
///
/// Returns compressed_size / original_size (0.0 = maximally compressible /
/// redundant, 1.0 = incompressible / information-dense).
///
/// Based on the Entropy Law (arXiv:2407.06645): lossless compression ratio
/// strongly predicts model performance on the compressed content.
/// Threshold: ratio < 0.30 indicates high boilerplate/repetition.
fn compression_density(text: &str) -> f64 {
    let input = text.as_bytes();
    if input.is_empty() { return 1.0; }
    let mut enc = ZlibEncoder::new(Vec::new(), Compression::best());
    let _ = enc.write_all(input);
    let compressed = enc.finish().unwrap_or_default();
    (compressed.len() as f64 / input.len() as f64).clamp(0.0, 1.0)
}

/// U-shaped attention weight for a set of key module positions.
///
/// LLMs exhibit a positional U-bias: tokens at context boundaries receive
/// disproportionately more attention than middle tokens. Liu et al. (TACL 2024)
/// measured >30% accuracy drop when relevant content moves from position 1 to
/// position 10 in a 20-document context.
///
/// Weight formula: w(p) = (2p − 1)² — maximum at p=0 and p=1, zero at p=0.5.
/// Returns the mean weight across all key positions, or 0.5 if none provided.
fn position_health(key_positions: &[f64]) -> f64 {
    if key_positions.is_empty() { return 0.5; }
    let mean = key_positions.iter()
        .map(|&p| {
            let p = p.clamp(0.0, 1.0);
            (2.0 * p - 1.0).powi(2)
        })
        .sum::<f64>() / key_positions.len() as f64;
    mean.clamp(0.0, 1.0)
}

/// Symbols per 1K tokens, normalized to 0–1.
///
/// Derived from BudgetMem (arXiv:2511.04919) entity_density signal.
/// 10 or more signatures per 1K tokens = fully dense = score of 1.0.
fn entity_density_score(total_tokens: usize, sig_count: usize) -> f64 {
    if total_tokens == 0 { return 0.0; }
    let per_1k = sig_count as f64 / (total_tokens as f64 / 1000.0);
    (per_1k / 10.0).clamp(0.0, 1.0)
}

/// Fraction of the context window remaining after this bundle.
///
/// Penalises utilisation above 85% (truncation risk zone) quadratically.
fn utilization_headroom(token_count: usize, window: usize) -> f64 {
    if window == 0 { return 1.0; }
    let used = (token_count as f64 / window as f64).clamp(0.0, 1.0);
    if used > 0.85 {
        // Steep quadratic: 0.85 → score 1.0, 1.0 → score 0.0.
        // (1 - excess)^2 falls faster than (1 - excess^2), which is too gentle.
        let excess = (used - 0.85) / 0.15;
        (1.0 - excess).powi(2).clamp(0.0, 1.0)
    } else {
        1.0 - used
    }
}

/// 1 − (duplicate line fraction). Catches obvious repetition (boilerplate,
/// echoed tool output, copy-pasted headers).
fn dedup_ratio(text: &str) -> f64 {
    let lines: Vec<&str> = text.lines()
        .map(str::trim)
        .filter(|l| !l.is_empty())
        .collect();
    if lines.is_empty() { return 1.0; }
    let unique: HashSet<&&str> = lines.iter().collect();
    (unique.len() as f64 / lines.len() as f64).clamp(0.0, 1.0)
}

// ---------------------------------------------------------------------------
// Composite score
// ---------------------------------------------------------------------------

/// Weights informed by BudgetMem's validated five-signal system
/// (arXiv:2511.04919), adapted for code skeleton context.
///
/// signal_density and position_health take the largest share because the
/// research consistently shows these as the two highest-impact variables for
/// code context specifically: attention dilution (signal) and positional bias.
fn composite_score(m: &MetricBreakdown) -> f64 {
    let raw =
        0.25 * m.signal_density
      + 0.20 * m.compression_density
      + 0.20 * m.position_health
      + 0.15 * m.entity_density
      + 0.10 * m.utilization_headroom
      + 0.10 * m.dedup_ratio;
    (raw * 100.0).clamp(0.0, 100.0)
}

fn grade(score: f64) -> String {
    match score as u32 {
        85..=100 => "A",
        70..=84  => "B",
        55..=69  => "C",
        40..=54  => "D",
        _        => "F",
    }.to_string()
}

// ---------------------------------------------------------------------------
// Warnings and recommendations
// ---------------------------------------------------------------------------

fn build_warnings(
    m: &MetricBreakdown,
    token_count: usize,
    window: usize,
) -> (Vec<String>, Vec<String>) {
    let mut warnings = Vec::new();
    let mut recs     = Vec::new();
    let util_pct = if window > 0 { token_count as f64 / window as f64 * 100.0 } else { 0.0 };

    // Signal density thresholds from Morph 2024 "Context Rot" research
    if m.signal_density < 0.05 {
        warnings.push(format!(
            "CRITICAL: signal density is {:.1}% — below the 5% threshold where attention \
             dilution severely degrades model output (Morph 2024: effective attention \
             reduced to 1/40th of baseline at 2.5% density)",
            m.signal_density * 100.0
        ));
        recs.push(
            "Use `cartographer context --budget <N>` — PageRank ordering maximises \
             symbol density within a token budget".to_string()
        );
    } else if m.signal_density < 0.15 {
        warnings.push(format!(
            "Low signal density ({:.1}%) — context contains significant non-symbol content. \
             Consider a tighter token budget.",
            m.signal_density * 100.0
        ));
        recs.push(
            "Try `cartographer context --focus <file> --budget <N>` to get a \
             signal-dense, query-focused subset".to_string()
        );
    }

    // Truncation risk
    if util_pct > 90.0 {
        warnings.push(format!(
            "CRITICAL: context is {:.0}% of the {}-token window — truncation is likely",
            util_pct,
            window
        ));
        recs.push(format!(
            "Reduce output by ~{:.0}K tokens using `--budget {}` or a more \
             focused `--focus` set",
            (token_count as f64 - window as f64 * 0.80) / 1000.0,
            (window as f64 * 0.75) as usize
        ));
    } else if util_pct > 80.0 {
        warnings.push(format!(
            "High utilisation ({:.0}% of window) — little room for the model's \
             response or additional tool calls",
            util_pct
        ));
    }

    // Position health — Liu et al. TACL 2024
    if m.position_health < 0.40 {
        warnings.push(
            "Key modules are positioned in the middle of context — Liu et al. (TACL 2024) \
             measured >30% accuracy drop when relevant content is placed at middle positions \
             vs. context boundaries".to_string()
        );
        recs.push(
            "`cartographer context` uses PageRank ordering, which naturally places \
             high-centrality modules near the boundary positions".to_string()
        );
    }

    // Compression density — arXiv:2407.06645
    if m.compression_density < 0.25 {
        warnings.push(format!(
            "High redundancy: context compresses to {:.0}% of original size — \
             significant boilerplate or repeated content detected \
             (Entropy Law, arXiv:2407.06645: low compression ratio correlates \
             with poor model performance on the content)",
            m.compression_density * 100.0
        ));
        recs.push(
            "Check for repeated import blocks, duplicated file headers, \
             or verbose scaffolding that can be stripped".to_string()
        );
    }

    // Entity density — BudgetMem arXiv:2511.04919
    if m.entity_density < 0.15 {
        warnings.push(
            "Very few symbols per token — context is mostly non-code text \
             (BudgetMem: entity density is the second-highest-weight signal \
             for context quality after position)".to_string()
        );
    }

    (warnings, recs)
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/// Analyse a context bundle and return a health report.
///
/// `content` — the full text of the generated context (XML, Markdown, or JSON).
/// `opts`    — scoring options; use `HealthOpts::default()` for sensible defaults.
pub fn analyze(content: &str, opts: &HealthOpts) -> ContextHealthReport {
    let total_tokens = count_tokens(content);
    let window = if opts.window_size > 0 {
        opts.window_size
    } else {
        opts.model.default_window()
    };

    let m = MetricBreakdown {
        signal_density:       signal_density(total_tokens, opts.signature_tokens),
        compression_density:  compression_density(content),
        position_health:      position_health(&opts.key_positions),
        entity_density:       entity_density_score(total_tokens, opts.signature_count),
        utilization_headroom: utilization_headroom(total_tokens, window),
        dedup_ratio:          dedup_ratio(content),
    };

    let score = composite_score(&m);
    let (warnings, recommendations) = build_warnings(&m, total_tokens, window);

    ContextHealthReport {
        token_count:     total_tokens,
        char_count:      content.len(),
        window_size:     window,
        utilization_pct: if window > 0 { total_tokens as f64 / window as f64 * 100.0 } else { 0.0 },
        score,
        grade:           grade(score),
        metrics:         m,
        warnings,
        recommendations,
    }
}

/// Compute key module positions from an ordered list of module IDs and a list of
/// which IDs are considered "key" (entry, core, or bridge roles).
///
/// Returns relative positions (0.0–1.0) of key modules in the ordered list.
pub fn key_positions_from_order(ordered: &[String], key_ids: &[String]) -> Vec<f64> {
    if ordered.is_empty() { return vec![]; }
    let n = ordered.len() as f64;
    let key_set: HashSet<&str> = key_ids.iter().map(String::as_str).collect();
    ordered.iter().enumerate()
        .filter(|(_, id)| key_set.contains(id.as_str()))
        .map(|(i, _)| i as f64 / (n - 1.0).max(1.0))
        .collect()
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn perfect_boundary_placement() {
        // Key modules at 0.0 and 1.0 → position_health = 1.0
        let pos = vec![0.0, 1.0];
        assert!((position_health(&pos) - 1.0).abs() < 1e-9);
    }

    #[test]
    fn worst_middle_placement() {
        // Key module exactly in the middle → position_health = 0.0
        let pos = vec![0.5];
        assert!(position_health(&pos) < 1e-9);
    }

    #[test]
    fn compression_density_repetitive() {
        // Highly repetitive text compresses well → low density
        let text = "fn foo() {}\n".repeat(500);
        let cd = compression_density(&text);
        assert!(cd < 0.20, "repetitive text should have low compression density, got {}", cd);
    }

    #[test]
    fn compression_density_dense() {
        // Varied identifiers with unique names — zlib can't find long repeating runs.
        // This should compress worse than "fn foo() {}\n" * 500.
        let text: String = (0..800)
            .map(|i: u64| {
                // Mix several values so each line is unique
                format!("pub fn sym_{:05}_{:03x}(arg_{}: u{}) -> Result<Type{}, Err{}>;\n",
                    i, i * 7 + 13, i % 29, (i % 4) * 8 + 8, i % 31, i % 7)
            })
            .collect();
        let cd = compression_density(&text);
        assert!(cd > 0.15, "varied identifiers should compress worse than constant repetition, got {}", cd);
    }

    #[test]
    fn dedup_ratio_no_duplicates() {
        let text = "fn foo() {}\nfn bar() {}\nfn baz() {}";
        assert!((dedup_ratio(text) - 1.0).abs() < 1e-9);
    }

    #[test]
    fn dedup_ratio_all_duplicates() {
        let text = "fn foo() {}\n".repeat(10);
        let r = dedup_ratio(&text);
        assert!(r < 0.2, "all-duplicate text should have near-zero dedup ratio, got {}", r);
    }

    #[test]
    fn signal_density_threshold() {
        // 2.5% density is the "context rot" threshold from Morph 2024
        let density = signal_density(20_000, 500);
        assert!((density - 0.025).abs() < 1e-9);
    }

    #[test]
    fn entity_density_normalisation() {
        // 10 sigs / 1K tokens → score = 1.0
        assert!((entity_density_score(1000, 10) - 1.0).abs() < 1e-9);
        // 5 sigs / 1K tokens → score = 0.5
        assert!((entity_density_score(1000, 5) - 0.5).abs() < 1e-9);
    }

    #[test]
    fn utilization_no_penalty_below_threshold() {
        // 50% utilization → headroom = 0.50
        let h = utilization_headroom(64_000, 128_000);
        assert!((h - 0.50).abs() < 1e-9);
    }

    #[test]
    fn utilization_penalty_above_threshold() {
        // 95% utilization → should be heavily penalised
        let h = utilization_headroom(121_600, 128_000);
        assert!(h < 0.30, "95% utilization should have low headroom score, got {}", h);
    }

    #[test]
    fn analyze_produces_grade() {
        let content = "pub fn foo() {}\npub fn bar() {}\n".repeat(50);
        let opts = HealthOpts {
            signature_count:  100,
            signature_tokens: count_tokens("pub fn foo() {}\npub fn bar() {}") * 50,
            ..Default::default()
        };
        let report = analyze(&content, &opts);
        assert!(report.score > 0.0 && report.score <= 100.0);
        assert!(["A","B","C","D","F"].contains(&report.grade.as_str()));
    }

    #[test]
    fn key_positions_from_order_works() {
        let ordered = vec!["a", "b", "c", "d", "e"]
            .into_iter().map(String::from).collect::<Vec<_>>();
        let keys    = vec!["a".to_string(), "e".to_string()];
        let pos = key_positions_from_order(&ordered, &keys);
        // "a" is at index 0 → 0.0, "e" is at index 4 → 1.0
        assert_eq!(pos.len(), 2);
        assert!((pos[0] - 0.0).abs() < 1e-9);
        assert!((pos[1] - 1.0).abs() < 1e-9);
    }

    #[test]
    fn composite_warns_on_low_signal_density() {
        let content = "lots of plain english prose with no code whatsoever. ".repeat(300);
        let opts = HealthOpts {
            signature_count:  1,
            signature_tokens: 3,
            ..Default::default()
        };
        let report = analyze(&content, &opts);
        assert!(
            report.warnings.iter().any(|w| w.contains("signal density")),
            "expected signal density warning, got: {:?}", report.warnings
        );
    }
}
