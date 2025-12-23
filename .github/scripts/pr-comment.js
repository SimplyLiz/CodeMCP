/**
 * CKB PR Comment Generator
 * Generates and posts/updates PR analysis comments
 */
module.exports = async ({ github, context, core }) => {
  const fs = require('fs');
  const read = (f, d) => { try { return JSON.parse(fs.readFileSync(f)); } catch { return d; } };

  // Thresholds from environment
  const COMPLEXITY_CYCLOMATIC = parseInt(process.env.COMPLEXITY_CYCLOMATIC || '15');
  const COMPLEXITY_COGNITIVE = parseInt(process.env.COMPLEXITY_COGNITIVE || '20');

  // Load data
  const pr = read('analysis.json', {});
  const complexity = read('complexity.json', []);
  const coupling = read('coupling.json', { missingCoupled: [] });
  const contracts = read('contracts.json', { files: [], breaking: [] });
  const audit = read('audit.json', { items: [], quickWins: [], summary: {} });
  const deadcode = read('deadcode.json', { candidates: [] });
  const docsCov = read('docs-coverage.json', { coverage: 0 });
  const docsStale = read('docs-stale.json', { totalStale: 0 });
  const drift = read('drift.json', []);
  const languages = read('languages.json', { languages: [], overallQuality: 1 });
  const evalResults = read('eval.json', { passed: 0, total: 0, results: [], skipped: true });
  const blast = read('blast.json', { affectedSymbols: [], affectedTests: [] });

  const s = pr.summary || {};
  const risk = pr.riskAssessment || {};
  const reviewers = pr.suggestedReviewers || [];
  const modules = pr.modulesAffected || [];
  const hotspots = (pr.changedFiles || []).filter(f => f.isHotspot);
  const breakingChanges = contracts.breaking || [];
  const blastSymbols = blast.affectedSymbols || [];
  const blastTests = blast.affectedTests || [];
  const lowQualityLangs = (languages.languages || []).filter(l => (l.quality || 1) < 0.7);

  // Computed
  const complexViolations = complexity.filter(c =>
    c.cyclomatic > COMPLEXITY_CYCLOMATIC ||
    c.cognitive > COMPLEXITY_COGNITIVE
  );
  const criticalItems = (audit.items || []).filter(i => i.riskLevel === 'critical');
  const highItems = (audit.items || []).filter(i => i.riskLevel === 'high');
  const riskyModules = modules.filter(m => m.riskLevel === 'high' || m.riskLevel === 'medium');

  // Helpers
  const pct = v => Math.round((v || 0) * 100);
  const safetyPct = Math.max(0, Math.round((1 - (risk.score || 0)) * 100));

  // Risk styling
  const riskStyle = {
    high: { icon: '🔴', color: 'e74c3c', label: 'HIGH' },
    medium: { icon: '🟡', color: 'f39c12', label: 'MEDIUM' },
    low: { icon: '🟢', color: '27ae60', label: 'LOW' }
  }[risk.level] || { icon: '⚪', color: '95a5a6', label: 'UNKNOWN' };

  // Colorful progress bar
  function makeProgressBar(safetyPercent, width = 15) {
    const p = Math.max(0, Math.min(100, safetyPercent));
    const safe = Math.round((p / 100) * width);
    const risky = width - safe;

    let bar = '🟩'.repeat(safe);
    if (risky > 0) {
      if (p >= 70) bar += '🟨'.repeat(risky);
      else if (p >= 40) bar += '🟧'.repeat(Math.ceil(risky / 2)) + '🟨'.repeat(Math.floor(risky / 2));
      else bar += '🟥'.repeat(Math.ceil(risky / 2)) + '🟧'.repeat(Math.floor(risky / 2));
    }
    return bar;
  }

  // Build comment
  let c = [];

  // ═══════════════════════════════════════════════════════════════
  // HEADER WITH BADGES
  // ═══════════════════════════════════════════════════════════════
  c.push('<!-- ckb -->');
  c.push('');
  c.push('## CKB Analysis');
  c.push('');
  c.push(`![Risk](https://img.shields.io/badge/${riskStyle.label}-${pct(risk.score)}%25-${riskStyle.color}?style=for-the-badge) ` +
         `![Files](https://img.shields.io/badge/Files-${s.totalFiles || 0}-3498db?style=flat-square) ` +
         `![Lines](https://img.shields.io/badge/%2B${s.totalAdditions || 0}%20%2F%20−${s.totalDeletions || 0}-3498db?style=flat-square) ` +
         `![Modules](https://img.shields.io/badge/Modules-${s.totalModules || 0}-3498db?style=flat-square)`);
  c.push('');

  // ═══════════════════════════════════════════════════════════════
  // COLORFUL SAFETY BAR
  // ═══════════════════════════════════════════════════════════════
  const progressBar = makeProgressBar(safetyPct, 15);
  const safetyLabel = safetyPct >= 70 ? `**${safetyPct}%** safe ✓` :
                     safetyPct >= 40 ? `**${safetyPct}%** safe` :
                     `**${safetyPct}%** safe ⚠️`;

  c.push(`| Health | ${progressBar} | ${safetyLabel} |`);
  c.push('|:-------|:-----|:-----|');
  c.push('');

  // ═══════════════════════════════════════════════════════════════
  // ISSUES SUMMARY
  // ═══════════════════════════════════════════════════════════════
  const issues = [];
  if (criticalItems.length) issues.push(`🔴 **${criticalItems.length}** critical`);
  if (highItems.length) issues.push(`🟠 **${highItems.length}** high`);
  if (hotspots.length) issues.push(`🔥 **${hotspots.length}** hotspots`);
  if (complexViolations.length) issues.push(`📊 **${complexViolations.length}** complex`);
  if (coupling.missingCoupled?.length) issues.push(`🔗 **${coupling.missingCoupled.length}** coupled`);
  if (breakingChanges.length) issues.push(`💥 **${breakingChanges.length}** breaking`);
  if (blastSymbols.length + blastTests.length) issues.push(`💣 **${blastSymbols.length + blastTests.length}** blast`);
  if (contracts.files?.length) issues.push(`📜 **${contracts.files.length}** contracts`);
  if (docsStale.totalStale) issues.push(`📚 **${docsStale.totalStale}** stale`);
  if (deadcode.candidates?.length) issues.push(`💀 **${deadcode.candidates.length}** dead`);
  if (lowQualityLangs.length) issues.push(`🌐 **${lowQualityLangs.length}** lang`);

  if (issues.length > 0) {
    c.push(`> ⚠️ ${issues.join(' · ')}`);
    c.push('');
  } else {
    c.push('> ✅ **All checks passed** — No issues detected');
    c.push('');
  }

  // ═══════════════════════════════════════════════════════════════
  // RISK FACTORS
  // ═══════════════════════════════════════════════════════════════
  if (risk.factors?.length) {
    c.push('**Risk factors:** ' + risk.factors.slice(0, 3).join(' • '));
    c.push('');
  }

  // ═══════════════════════════════════════════════════════════════
  // REVIEWERS
  // ═══════════════════════════════════════════════════════════════
  if (reviewers.length) {
    const list = reviewers.slice(0, 3).map(r => {
      const name = r.owner.startsWith('@') ? r.owner : `@${r.owner}`;
      return `**${name}** (${pct(r.coverage)}%)`;
    }).join(', ');
    c.push(`👥 **Suggested:** ${list}`);
    c.push('');
  }

  // ═══════════════════════════════════════════════════════════════
  // COLLAPSIBLE SECTIONS
  // ═══════════════════════════════════════════════════════════════

  // Breaking Changes (open by default)
  if (breakingChanges.length > 0) {
    c.push('<details open>');
    c.push(`<summary>💥 Breaking changes · ${breakingChanges.length} detected</summary>`);
    c.push('');
    c.push('| Symbol | Change |');
    c.push('|:-------|:-------|');
    breakingChanges.slice(0, 5).forEach(b => {
      c.push(`| \`${b.symbol || b.name || '?'}\` | ${b.change || b.description || '?'} |`);
    });
    c.push('');
    c.push('</details>');
    c.push('');
  }

  // Risk Audit
  if (criticalItems.length + highItems.length > 0) {
    c.push('<details>');
    c.push(`<summary>⚠️ Risk audit · ${criticalItems.length} critical · ${highItems.length} high</summary>`);
    c.push('');
    c.push('| | File | Score | Factor |');
    c.push('|:-:|:-----|------:|:-------|');
    [...criticalItems, ...highItems].slice(0, 6).forEach(item => {
      const icon = item.riskLevel === 'critical' ? '🔴' : '🟠';
      const factor = (item.factors || [])[0]?.factor || '—';
      c.push(`| ${icon} | \`${item.file}\` | ${item.riskScore} | ${factor} |`);
    });
    c.push('');
    c.push('</details>');
    c.push('');
  }

  // Hotspots
  if (hotspots.length > 0) {
    c.push('<details>');
    c.push(`<summary>🔥 Hotspots · ${hotspots.length} volatile files</summary>`);
    c.push('');
    c.push('| File | Churn |');
    c.push('|:-----|------:|');
    hotspots.slice(0, 5).forEach(f => {
      c.push(`| \`${f.path}\` | ${(f.hotspotScore || 0).toFixed(2)} |`);
    });
    c.push('');
    c.push('</details>');
    c.push('');
  }

  // Modules
  if (riskyModules.length > 0) {
    c.push('<details>');
    c.push(`<summary>📦 Modules · ${riskyModules.length} at risk</summary>`);
    c.push('');
    c.push('| | Module | Files |');
    c.push('|:-:|:-------|------:|');
    riskyModules.slice(0, 5).forEach(m => {
      const icon = m.riskLevel === 'high' ? '🔴' : '🟡';
      c.push(`| ${icon} | \`${m.moduleId}\` | ${m.filesChanged} |`);
    });
    c.push('');
    c.push('</details>');
    c.push('');
  }

  // Contracts
  if (contracts.files?.length > 0) {
    c.push('<details>');
    c.push(`<summary>📜 Contracts · ${contracts.files.length} changed</summary>`);
    c.push('');
    contracts.files.slice(0, 6).forEach(f => c.push(`- \`${f}\``));
    c.push('');
    c.push('</details>');
    c.push('');
  }

  // Complexity
  if (complexViolations.length > 0) {
    c.push('<details>');
    c.push(`<summary>📊 Complexity · ${complexViolations.length} violations</summary>`);
    c.push('');
    c.push('| File | Cyclomatic | Cognitive |');
    c.push('|:-----|----------:|----------:|');
    complexViolations.slice(0, 5).forEach(v => {
      const cyWarn = v.cyclomatic > COMPLEXITY_CYCLOMATIC ? '⚠️ ' : '';
      const cgWarn = v.cognitive > COMPLEXITY_COGNITIVE ? '⚠️ ' : '';
      c.push(`| \`${v.file}\` | ${cyWarn}${v.cyclomatic} | ${cgWarn}${v.cognitive} |`);
    });
    c.push('');
    c.push('</details>');
    c.push('');
  }

  // Coupling
  if (coupling.missingCoupled?.length > 0) {
    c.push('<details>');
    c.push(`<summary>🔗 Coupling · ${coupling.missingCoupled.length} missing</summary>`);
    c.push('');
    c.push('| Missing | Usually with | Score |');
    c.push('|:--------|:-------------|------:|');
    coupling.missingCoupled.slice(0, 5).forEach(w => {
      c.push(`| \`${w.file}\` | \`${w.coupledTo}\` | ${pct(w.correlation || w.couplingScore || 0)}% |`);
    });
    c.push('');
    c.push('</details>');
    c.push('');
  }

  // Quick Wins
  if (audit.quickWins?.length > 0) {
    c.push('<details>');
    c.push(`<summary>💡 Quick wins · ${audit.quickWins.length} suggestions</summary>`);
    c.push('');
    audit.quickWins.slice(0, 5).forEach(w => {
      const e = { low: '🟢', medium: '🟡', high: '🔴' }[w.effort] || '⚪';
      c.push(`- ${e} **${w.action}** → \`${w.target}\``);
    });
    c.push('');
    c.push('</details>');
    c.push('');
  }

  // Ownership Drift
  if (Array.isArray(drift) && drift.length > 0) {
    c.push('<details>');
    c.push(`<summary>👤 Ownership drift · ${drift.length} files</summary>`);
    c.push('');
    c.push('| File | Declared | Actual |');
    c.push('|:-----|:---------|:-------|');
    drift.slice(0, 5).forEach(d => {
      c.push(`| \`${d.path}\` | ${d.declaredOwner || '—'} | ${d.actualOwner || '—'} |`);
    });
    c.push('');
    c.push('</details>');
    c.push('');
  }

  // Dead Code
  if (deadcode.candidates?.length > 0) {
    c.push('<details>');
    c.push(`<summary>💀 Dead code · ${deadcode.candidates.length} candidates</summary>`);
    c.push('');
    c.push('| Symbol | Confidence |');
    c.push('|:-------|:-----------|');
    deadcode.candidates.slice(0, 5).forEach(d => {
      c.push(`| \`${d.name}\` | ${pct(d.confidence || 0)}% |`);
    });
    c.push('');
    c.push('</details>');
    c.push('');
  }

  // Stale Docs
  if (docsStale.totalStale > 0) {
    c.push('<details>');
    c.push(`<summary>📚 Stale docs · ${docsStale.totalStale} references</summary>`);
    c.push('');
    (docsStale.reports || []).slice(0, 3).forEach(r => {
      (r.stale || []).slice(0, 2).forEach(s => {
        c.push(`- \`${r.docPath}:${s.line}\` — ${s.rawText}`);
      });
    });
    c.push('');
    c.push('</details>');
    c.push('');
  }

  // Blast Radius
  if (blastSymbols.length > 0 || blastTests.length > 0) {
    c.push('<details>');
    c.push(`<summary>💣 Blast radius · ${blastSymbols.length} symbols · ${blastTests.length} tests</summary>`);
    c.push('');
    if (blastSymbols.length > 0) {
      c.push('**Affected symbols:**');
      blastSymbols.slice(0, 5).forEach(sym => c.push(`- \`${sym.name || sym}\``));
      c.push('');
    }
    if (blastTests.length > 0) {
      c.push('**Tests that may need updates:**');
      blastTests.slice(0, 5).forEach(t => c.push(`- \`${t.name || t}\``));
      c.push('');
    }
    c.push('</details>');
    c.push('');
  }

  // Language Quality
  if (lowQualityLangs.length > 0) {
    c.push('<details>');
    c.push(`<summary>🌐 Language quality · ${lowQualityLangs.length} issues</summary>`);
    c.push('');
    c.push('| Language | Quality | Issues |');
    c.push('|:---------|--------:|:-------|');
    lowQualityLangs.slice(0, 5).forEach(l => {
      const quality = Math.round((l.quality || 0) * 100);
      const issues = (l.issues || []).join(', ') || '—';
      c.push(`| ${l.name} | ${quality}% | ${issues} |`);
    });
    c.push('');
    c.push('</details>');
    c.push('');
  }

  // Eval Suite
  if (!evalResults.skipped && evalResults.total > 0) {
    const evalPassed = evalResults.passed || 0;
    const evalTotal = evalResults.total || 0;
    const evalPct = Math.round((evalPassed / evalTotal) * 100);
    const evalIcon = evalPct >= 90 ? '✅' : '⚠️';
    c.push('<details>');
    c.push(`<summary>🧪 Eval suite · ${evalIcon} ${evalPassed}/${evalTotal} passed (${evalPct}%)</summary>`);
    c.push('');
    c.push('| Passed | Total | Rate |');
    c.push('|:------:|:-----:|:----:|');
    c.push(`| ${evalPassed} | ${evalTotal} | ${evalPct}% |`);
    c.push('');
    const failed = (evalResults.results || []).filter(r => !r.passed);
    if (failed.length > 0) {
      c.push('**Failed tests:**');
      failed.slice(0, 3).forEach(r => {
        c.push(`- \`${r.id || r.name}\`: ${r.reason || 'failed'}`);
      });
      c.push('');
    }
    c.push('</details>');
    c.push('');
  }

  // ═══════════════════════════════════════════════════════════════
  // FOOTER
  // ═══════════════════════════════════════════════════════════════
  c.push('---');
  const cacheIcon = process.env.CACHE_HIT === 'true' ? '💾' : '🔄';
  const modeIcon = process.env.INDEX_MODE === 'incremental' ? '⚡' : '🔨';
  c.push(`<sub>${cacheIcon} ${modeIcon} ${process.env.INDEX_TIME || '?'}s · <a href="https://github.com/SimplyLiz/CodeMCP">CKB</a></sub>`);

  // Post/update comment
  const body = c.join('\n');
  const { data: comments } = await github.rest.issues.listComments({
    owner: context.repo.owner,
    repo: context.repo.repo,
    issue_number: context.issue.number
  });
  const existing = comments.find(comment => comment.body?.includes('<!-- ckb -->'));

  if (existing) {
    await github.rest.issues.updateComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      comment_id: existing.id,
      body
    });
  } else {
    await github.rest.issues.createComment({
      owner: context.repo.owner,
      repo: context.repo.repo,
      issue_number: context.issue.number,
      body
    });
  }
};
