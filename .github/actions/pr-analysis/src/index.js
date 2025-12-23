const core = require('@actions/core');
const github = require('@actions/github');
const exec = require('@actions/exec');

const COMMENT_MARKER = '<!-- ckb-pr-analysis -->';

async function run() {
  try {
    // Get inputs
    const token = core.getInput('github-token', { required: true });
    const baseBranch = core.getInput('base-branch') || 'main';
    const riskThreshold = parseFloat(core.getInput('risk-threshold') || '0');
    const includeReviewers = core.getInput('include-reviewers') !== 'false';
    const includeAdrs = core.getInput('include-adrs') !== 'false';
    const failOnHighRisk = core.getInput('fail-on-high-risk') === 'true';
    const ckbVersion = core.getInput('ckb-version') || 'latest';

    const octokit = github.getOctokit(token);
    const context = github.context;

    // Only run on pull requests
    if (!context.payload.pull_request) {
      core.info('Not a pull request, skipping');
      return;
    }

    const prNumber = context.payload.pull_request.number;
    const headBranch = context.payload.pull_request.head.ref;

    core.info(`Analyzing PR #${prNumber}: ${headBranch} -> ${baseBranch}`);

    // Install CKB
    core.startGroup('Installing CKB');
    const versionSpec = ckbVersion === 'latest' ? '@tastehub/ckb' : `@tastehub/ckb@${ckbVersion}`;
    await exec.exec('npm', ['install', '-g', versionSpec]);
    core.endGroup();

    // Initialize CKB if needed
    core.startGroup('Initializing CKB');
    try {
      await exec.exec('ckb', ['init']);
    } catch (e) {
      core.info('CKB already initialized or init not needed');
    }
    core.endGroup();

    // Run pr-summary
    core.startGroup('Running CKB analysis');
    let output = '';
    let errorOutput = '';

    const exitCode = await exec.exec('ckb', [
      'pr-summary',
      `--base=${baseBranch}`,
      `--head=${headBranch}`,
      '--format=json'
    ], {
      listeners: {
        stdout: (data) => { output += data.toString(); },
        stderr: (data) => { errorOutput += data.toString(); }
      },
      ignoreReturnCode: true
    });
    core.endGroup();

    if (exitCode !== 0) {
      core.warning(`CKB analysis failed with exit code ${exitCode}`);
      core.warning(errorOutput);
      return;
    }

    // Parse analysis result
    let analysis;
    try {
      analysis = JSON.parse(output);
    } catch (e) {
      core.error(`Failed to parse CKB output: ${e.message}`);
      core.error(`Output was: ${output}`);
      return;
    }

    // Extract risk info
    const riskScore = analysis.risk?.score ?? analysis.facts?.risk?.score ?? 0;
    const riskLevel = analysis.risk?.level ?? analysis.facts?.risk?.level ?? 'unknown';

    core.setOutput('risk-score', riskScore.toString());
    core.setOutput('risk-level', riskLevel);

    core.info(`Risk score: ${riskScore} (${riskLevel})`);

    // Check threshold
    if (riskScore < riskThreshold) {
      core.info(`Risk score ${riskScore} below threshold ${riskThreshold}, skipping comment`);
      return;
    }

    // Format as markdown
    const markdown = formatAnalysis(analysis, { includeReviewers, includeAdrs });

    // Find existing comment
    const { data: comments } = await octokit.rest.issues.listComments({
      owner: context.repo.owner,
      repo: context.repo.repo,
      issue_number: prNumber
    });

    const existingComment = comments.find(c => c.body.includes(COMMENT_MARKER));

    // Post or update comment
    let commentUrl;
    if (existingComment) {
      core.info(`Updating existing comment ${existingComment.id}`);
      const { data } = await octokit.rest.issues.updateComment({
        owner: context.repo.owner,
        repo: context.repo.repo,
        comment_id: existingComment.id,
        body: markdown
      });
      commentUrl = data.html_url;
    } else {
      core.info('Creating new comment');
      const { data } = await octokit.rest.issues.createComment({
        owner: context.repo.owner,
        repo: context.repo.repo,
        issue_number: prNumber,
        body: markdown
      });
      commentUrl = data.html_url;
    }

    core.setOutput('comment-url', commentUrl);
    core.info(`Comment posted: ${commentUrl}`);

    // Fail if high risk and configured to do so
    if (failOnHighRisk && riskScore > 0.8) {
      core.setFailed(`Risk score ${riskScore} exceeds threshold 0.8`);
    }

  } catch (error) {
    core.setFailed(error.message);
  }
}

function formatAnalysis(analysis, options) {
  const facts = analysis.facts || analysis;
  const risk = facts.risk || {};
  const modules = facts.affectedModules || facts.changedModules || [];
  const reviewers = facts.suggestedReviewers || [];
  const decisions = facts.relatedDecisions || [];
  const changedFiles = facts.changedFiles || [];

  let md = `${COMMENT_MARKER}\n`;
  md += `## CKB Analysis\n\n`;

  // Risk assessment
  const riskEmoji = getRiskEmoji(risk.level);
  md += `### Risk Assessment\n\n`;
  md += `| Level | Score | Factors |\n`;
  md += `|-------|-------|--------|\n`;
  const factors = (risk.factors || []).join(', ') || 'None identified';
  md += `| ${riskEmoji} **${capitalize(risk.level || 'unknown')}** | ${(risk.score || 0).toFixed(2)} | ${factors} |\n\n`;

  // Changed files summary
  if (changedFiles.length > 0) {
    md += `### Changed Files\n\n`;
    md += `${changedFiles.length} file(s) changed\n\n`;
  }

  // Affected modules
  if (modules.length > 0) {
    md += `### Affected Modules\n\n`;
    md += `| Module | Risk | Notes |\n`;
    md += `|--------|------|-------|\n`;
    for (const mod of modules.slice(0, 10)) {
      const modRisk = mod.risk || mod.riskLevel || 'low';
      const notes = mod.isHotspot ? 'Hotspot' : '';
      md += `| \`${mod.path || mod.name || mod}\` | ${capitalize(modRisk)} | ${notes} |\n`;
    }
    if (modules.length > 10) {
      md += `\n*...and ${modules.length - 10} more modules*\n`;
    }
    md += '\n';
  }

  // Suggested reviewers
  if (options.includeReviewers && reviewers.length > 0) {
    md += `### Suggested Reviewers\n\n`;
    md += `| Reviewer | Reason |\n`;
    md += `|----------|--------|\n`;
    for (const reviewer of reviewers.slice(0, 5)) {
      const name = reviewer.name || reviewer.login || reviewer;
      const reason = reviewer.reason || 'Code ownership';
      md += `| @${name} | ${reason} |\n`;
    }
    md += '\n';
  }

  // Related ADRs
  if (options.includeAdrs && decisions.length > 0) {
    md += `### Related Decisions\n\n`;
    for (const adr of decisions.slice(0, 5)) {
      const title = adr.title || adr.id || adr;
      const status = adr.status ? ` (${adr.status})` : '';
      md += `- ${title}${status}\n`;
    }
    md += '\n';
  }

  // Footer
  md += `---\n`;
  md += `<sub>Generated by [CKB](https://github.com/tastehub/ckb) Code Intelligence</sub>\n`;

  return md;
}

function getRiskEmoji(level) {
  switch ((level || '').toLowerCase()) {
    case 'high': return '';
    case 'medium': return '';
    case 'low': return '';
    default: return '';
  }
}

function capitalize(str) {
  if (!str) return '';
  return str.charAt(0).toUpperCase() + str.slice(1);
}

run();
