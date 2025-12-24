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
  const DOC_COVERAGE_MIN = parseInt(process.env.DOC_COVERAGE_MIN || '70');

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
  const runUrl = `https://github.com/${context.repo.owner}/${context.repo.repo}/actions/runs/${process.env.GITHUB_RUN_ID}`;

  // Risk styling
  const riskStyle = {
    high: { icon: '🔴', color: 'e74c3c', label: 'HIGH' },
    medium: { icon: '🟡', color: 'f39c12', label: 'MEDIUM' },
    low: { icon: '🟢', color: '27ae60', label: 'LOW' }
  }[risk.level] || { icon: '⚪', color: '95a5a6', label: 'UNKNOWN' };

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
  // QUICK STATS
  // ═══════════════════════════════════════════════════════════════
  const stats = [];
  if (hotspots.length) stats.push(`🔥 ${hotspots.length} hotspot${hotspots.length > 1 ? 's' : ''}`);
  if (criticalItems.length + highItems.length) stats.push(`⚠️ ${criticalItems.length + highItems.length} risk items`);
  if (complexViolations.length) stats.push(`📊 ${complexViolations.length} complex`);
  if (coupling.missingCoupled?.length) stats.push(`🔗 ${coupling.missingCoupled.length} coupled`);
  if (breakingChanges.length) stats.push(`💥 ${breakingChanges.length} breaking`);
  if (blastSymbols.length + blastTests.length) stats.push(`💣 ${blastSymbols.length + blastTests.length} blast`);
  if (contracts.files?.length) stats.push(`📜 ${contracts.files.length} contract${contracts.files.length > 1 ? 's' : ''}`);
  if (docsStale.totalStale) stats.push(`📚 ${docsStale.totalStale} stale`);
  if (deadcode.candidates?.length) stats.push(`💀 ${deadcode.candidates.length} dead`);
  if (lowQualityLangs.length) stats.push(`🌐 ${lowQualityLangs.length} lang`);

  if (stats.length) {
    c.push(`> ${stats.join(' · ')}`);
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
    const list = reviewers.slice(0, 3).map(r => `**${r.owner.replace(/^@?/, '@')}** (${pct(r.coverage)}%)`).join(', ');
    c.push(`👥 Suggested: ${list}`);
    c.push('');
  }

  // ═══════════════════════════════════════════════════════════════
  // METRICS TABLE
  // ═══════════════════════════════════════════════════════════════
  c.push('| Metric | Value | |');
  c.push('|:-------|------:|:-:|');
  c.push(`| Doc Coverage | ${docsCov.coverage || 0}% | ${(docsCov.coverage || 0) >= DOC_COVERAGE_MIN ? '✅' : '⚠️'} |`);
  c.push(`| Complexity Issues | ${complexViolations.length} | ${complexViolations.length === 0 ? '✅' : '⚠️'} |`);
  c.push(`| Coupling Gaps | ${coupling.missingCoupled?.length || 0} | ${!coupling.missingCoupled?.length ? '✅' : '⚠️'} |`);
  c.push(`| Index | ${process.env.INDEX_MODE || 'unknown'} | ${process.env.CACHE_HIT === 'true' ? '💾' : '🆕'} |`);
  c.push('');

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
      const shown = failed.slice(0, 20);
      c.push('**Failed tests:**');
      shown.forEach(r => {
        c.push(`- \`${r.id || r.name}\`${r.reason ? `: ${r.reason}` : ''}`);
      });
      if (failed.length > shown.length) {
        c.push(`- … and **${failed.length - shown.length}** more → [Run Summary](${runUrl})`);
      }
      c.push('');

      // Full list in Step Summary
      let summary = `## 🧪 Failed Tests (${failed.length})\n\n`;
      summary += '| Test | Reason |\n|:-----|:-------|\n';
      failed.forEach(r => {
        summary += `| \`${r.id || r.name}\` | ${r.reason || '—'} |\n`;
      });
      await core.summary.addRaw(summary).write();
    }
    c.push('</details>');
    c.push('');
  }

  // ═══════════════════════════════════════════════════════════════
  // FOOTER
  // ═══════════════════════════════════════════════════════════════
  c.push('---');
  c.push(`<sub>Generated by <a href="https://github.com/SimplyLiz/CodeMCP">CKB</a> · <a href="${runUrl}">Run details</a></sub>`);

  // Post/update comment with hard-cap
  let body = c.join('\n');
  const MAX_COMMENT_SIZE = 65000;
  if (body.length > MAX_COMMENT_SIZE) {
    body = body.slice(0, MAX_COMMENT_SIZE - 200) + `\n\n---\n<sub>✂️ Truncated. <a href="${runUrl}">Full report in Run Summary</a></sub>`;
  }

  const { data: comments } = await github.rest.issues.listComments({
    owner: context.repo.owner,
    repo: context.repo.repo,
    issue_number: context.issue.number
  });
  // Only match Bot comments to avoid overwriting quoted comments
  const existing = comments.find(comment =>
    comment.user?.type === 'Bot' && comment.body?.includes('<!-- ckb -->')
  );

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
