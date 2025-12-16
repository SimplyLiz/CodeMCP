# CKB v5.1 Addendum: AI-Native Navigation Tools

> **Status**: Implementation-Ready
> 
> **Purpose**: AI-native navigation tools providing compressed, opinionated, agent-friendly answers.
> 
> **Version**: 5.1.0

-----

## Table of Contents

1. [Design Principles](#1-design-principles)
1. [Common Types](#2-common-types)
1. [Response Contract](#3-response-contract)
1. [Symbol Resolution](#4-symbol-resolution)
1. [Tool: explainSymbol](#5-tool-explainsymbol)
1. [Tool: justifySymbol](#6-tool-justifysymbol)
1. [Tool: getCallGraph](#7-tool-getcallgraph)
1. [Tool: getModuleOverview](#8-tool-getmoduleoverview)
1. [Enhanced: analyzeImpact](#9-enhanced-analyzeimpact)
1. [Verbosity Levels](#10-verbosity-levels)
1. [Error Handling](#11-error-handling)
1. [Ranking Policy v5.1](#12-ranking-policy-v51)
1. [Backend Staleness Rules](#13-backend-staleness-rules)
1. [Test Detection Rules](#14-test-detection-rules)
1. [Implementation Priority](#15-implementation-priority)
1. [MCP Schemas](#16-mcp-schemas)
1. [Appendix: Example Responses](#17-appendix-example-responses)

-----

## 1. Design Principles

### 1.1 AI-Native, Not IDE-Native

|Raw LSP/IDE              |CKB AI-Native                                      |
|-------------------------|---------------------------------------------------|
|“Here are 847 references”|“Used in 4 modules, mostly by AuthController”      |
|“Here’s the git blame”   |“Created by Alice 6 months ago to fix auth timeout”|
|“Here are all callers”   |“Entry point for 3 API routes, critical path”      |

### 1.2 Core Design Rules

1. **Flexible input**: Accept multiple formats with defined precedence
1. **Budget-aware**: Respect `verbosity` and `maxEvidence`
1. **Facts vs Summary vs Evidence**: Strictly separated
1. **Conservative heuristics**: No fake precision
1. **Single ordering**: `relevanceScore` descending, deterministic
1. **Auditable ranking**: Every item includes `ranking.signals` and `ranking.policyVersion`
1. **Explicit limitations**: Never hide truncation or backend issues

-----

## 2. Common Types

### 2.1 Range

All line/column values are **1-based** (first line is 1, first column is 1).

```typescript
interface Range {
  startLine: number;    // 1-based
  startCol?: number;    // 1-based, optional
  endLine: number;      // 1-based
  endCol?: number;      // 1-based, optional
}
```

### 2.2 Stable Pointer

For citations that survive line drift:

```typescript
interface StablePointer {
  repoStateId: string;
  filePath: string;
  symbolId?: SymbolId;
  scipOccurrenceId?: string;
  range?: Range;  // Informational, may drift
}
```

### 2.3 Derived Text

```typescript
interface DerivedText {
  text: string;
  derivedFrom: string[];  // Fact field paths
  confidence: number;     // 0.0-1.0, resolution/derivation score (not probability)
}
```

### 2.4 Drilldown

```typescript
interface Drilldown {
  label: string;
  query: string;
}
```

### 2.5 Ranking Info

Every ranked item includes:

```typescript
interface RankingInfo {
  signals: Record<string, number | boolean | string>;
  policyVersion: string;  // "5.1"
}
```

-----

## 3. Response Contract

### 3.1 Universal Response Structure

```typescript
interface AINavigationResponse<TFacts, TSections, TEvidence> {
  // Metadata
  ckbVersion: string;
  schemaVersion: number;  // 1
  tool: string;
  
  // Resolution
  resolved: ResolvedTarget;
  
  // Core data (strictly separated)
  facts: TFacts;
  summary: Summary<TSections>;
  evidence: TEvidence[];  // Sorted by relevanceScore DESC
  
  // Context
  provenance: AIProvenance;
  truncation: TruncationInfo;
  drilldowns: Drilldown[];
}
```

### 3.2 Resolved Target

```typescript
interface ResolvedTarget {
  symbolId?: SymbolId;
  moduleId?: ModuleId;
  resolvedFrom: 'id' | 'query' | 'path';
  confidence: number;  // 0.0-1.0, resolution score (not probability)
  ambiguous?: boolean;
  alternatives?: Alternative[];
}

interface Alternative {
  id: string;
  name: string;
  path?: string;
  matchScore: number;  // Resolution score, not relevance
}
```

**Note**: `confidence` and `matchScore` are resolution/matching scores from the search algorithm, normalized to [0,1]. They are NOT probabilities.

### 3.3 Summary Structure

Summary is always present. For `verbosity: 'tiny'`, sections may be empty.

```typescript
interface Summary<TSections> {
  tldr: string;
  sections: TSections;  // May be {} for tiny verbosity
}
```

### 3.4 Provenance

```typescript
interface AIProvenance {
  repoStateId: string;
  repoStateDirty: boolean;
  
  // Immediate clarity on data availability
  backendCoverage: {
    scip: 'available' | 'stale' | 'missing';
    lsp: 'available' | 'partial' | 'missing';
    git: 'available' | 'missing';
  };
  
  sources: {
    scip?: { indexPath: string; indexedCommit: string; indexedAt: string };
    lsp?: { server: string; methods: string[] };
    git?: { commitRange: string };
  };
  
  completeness: {
    scope: 'workspace' | 'module' | 'file' | 'symbol';
    estimate: 'high' | 'medium' | 'low' | 'unknown';
    rationale: string;
  };
  
  confidence: {
    overall: number;
    bySection: Record<string, number>;
  };
  
  rankingPolicyVersion: string;  // "5.1"
  limitations: string[];
  computedAt: string;  // ISO 8601
  durationMs: number;
}
```

### 3.5 Truncation Info

```typescript
interface TruncationInfo {
  truncated: boolean;
  reason?: 'max-evidence' | 'max-depth' | 'timeout' | 'budget';
  returned: number;
  available?: number;  // null/omitted if unknown (e.g., LSP-only)
}
```

**Rule**: If `available` cannot be determined without fetching all results, omit it. Do not lie.

### 3.6 includeEvidence Behavior

When `includeEvidence === false`:

- `evidence` MUST be `[]` (empty array, not omitted)
- `truncation.returned` MUST be `0`
- `truncation.truncated` MUST be `false`
- `truncation.available` may still be present if known

This ensures clients can distinguish “no evidence requested” from “evidence requested but none found”.

-----

## 4. Symbol Resolution

### 4.1 Input Variants

Tools accept ONE of these input combinations:

```typescript
type SymbolInput =
  | { symbolId: SymbolId }                                    // Variant A: Direct ID
  | { filePath: string; line: number; column?: number }       // Variant B: Location
  | { query: string; pathHint?: string }                      // Variant C: Query
  | { name: string; kind?: SymbolKind; moduleHint?: string }; // Variant D: Structured
```

### 4.2 Input Validation

**Clients MUST send exactly one input variant.**

Validation rules:

1. Count how many variants are present
1. If zero → Error `INVALID_INPUT` (“No symbol identification provided”)
1. If more than one → Error `INVALID_INPUT` (“Multiple symbol identification variants provided; send exactly one”)
1. If exactly one → Use that variant

**Variant detection**:

- Variant A present if: `symbolId` is non-empty
- Variant B present if: `filePath` AND `line` are both present
- Variant C present if: `query` is non-empty
- Variant D present if: `name` is non-empty (and none of the above)

```typescript
function validateInput(input: SymbolInput): void {
  const variants: string[] = [];
  
  if (input.symbolId) variants.push('symbolId');
  if (input.filePath && input.line !== undefined) variants.push('location');
  if (input.query) variants.push('query');
  if (input.name && !input.query && !input.symbolId) variants.push('name');
  
  if (variants.length === 0) {
    throw new CkbError('INVALID_INPUT', 'No symbol identification provided');
  }
  if (variants.length > 1) {
    throw new CkbError('INVALID_INPUT', 
      `Multiple identification variants provided (${variants.join(', ')}); send exactly one`);
  }
}
```

### 4.3 Resolution Algorithm

```typescript
async function resolveSymbol(input: SymbolInput): Promise<ResolvedTarget> {
  
  // Variant A: Direct ID
  if ('symbolId' in input && input.symbolId) {
    const symbol = await db.symbols.get(input.symbolId);
    if (symbol?.state === 'active') {
      return { symbolId: input.symbolId, resolvedFrom: 'id', confidence: 1.0 };
    }
    if (symbol?.state === 'deleted') {
      throw new CkbError('SYMBOL_DELETED', { deletedAt: symbol.deletedAt });
    }
    const aliased = await resolveAlias(input.symbolId);
    if (aliased) return { ...aliased, resolvedFrom: 'id' };
    throw new CkbError('SYMBOL_NOT_FOUND');
  }
  
  // Variant B: Location
  if ('filePath' in input && 'line' in input) {
    const symbol = await lsp.getSymbolAtLocation(input.filePath, input.line, input.column);
    if (symbol) {
      return {
        symbolId: await getOrCreateStableId(symbol),
        resolvedFrom: 'path',
        confidence: 0.95
      };
    }
    throw new CkbError('SYMBOL_NOT_FOUND');
  }
  
  // Variant C/D: Query
  const queryStr = 'query' in input ? input.query : input.name;
  const hint = 'pathHint' in input ? input.pathHint : input.moduleHint;
  const kind = 'kind' in input ? input.kind : undefined;
  
  const results = await searchSymbols(queryStr, { pathHint: hint, kind, limit: 10 });
  
  if (results.length === 0) {
    throw new CkbError('SYMBOL_NOT_FOUND');
  }
  
  const best = results[0];
  const alternatives = results.slice(1, 5).map(r => ({
    id: r.symbolId,
    name: r.name,
    path: r.filePath,
    matchScore: r.score
  }));
  
  // Clear winner: high score AND significant gap to second
  const isUnambiguous = best.score >= 0.7 && 
    (results.length === 1 || best.score - results[1].score > 0.2);
  
  return {
    symbolId: best.symbolId,
    resolvedFrom: 'query',
    confidence: best.score,
    ambiguous: !isUnambiguous,
    alternatives: alternatives.length > 0 ? alternatives : undefined
  };
}
```

-----

## 5. Tool: explainSymbol

### 5.1 Purpose

Answer: “What is this, who uses it, what does it use, what’s its history?”

### 5.2 Input Schema

```typescript
interface ExplainSymbolInput {
  // Symbol identification (ONE of these — see precedence rules)
  symbolId?: SymbolId;
  filePath?: string;
  line?: number;
  column?: number;
  query?: string;
  pathHint?: string;
  name?: string;
  kind?: SymbolKind;
  moduleHint?: string;
  
  // Budget
  verbosity?: 'tiny' | 'default' | 'full';
  maxEvidence?: number;
  includeEvidence?: boolean;
  
  // Expansion
  expand?: {
    justify?: boolean;
    callGraph?: { depth: 1; direction?: 'callers' | 'callees' | 'both' };
  };
}
```

**Expansion guardrails** (when `expand` present):

- Forces `verbosity ≤ 'default'`
- Forces `maxEvidence ≤ 20`
- `callGraph.depth` capped at 1
- `justify` returns minimal verdict only

### 5.3 Output Schema

```typescript
interface ExplainSymbolResponse extends AINavigationResponse<
  ExplainSymbolFacts,
  ExplainSymbolSections,
  ExplainSymbolEvidence
> {
  expanded?: {
    justify?: JustifyMinimal;
    callGraph?: CallGraphMinimal;
  };
}
```

### 5.4 Facts

```typescript
interface ExplainSymbolFacts {
  symbol: {
    stableId: SymbolId;
    name: string;
    qualifiedName: string;
    kind: SymbolKind;
    signature?: string;
    visibility: 'public' | 'private' | 'internal' | 'unknown';
    moduleId: ModuleId;
    moduleName: string;
    filePath: string;
    location: Range;
  };
  
  usage: {
    callerCount: number;
    calleeCount: number;
    referenceCount: number;
    moduleCount: number;
  };
  
  usageByModule: {
    moduleId: ModuleId;
    moduleName: string;
    callerCount: number;
    percentage: number;
  }[];
  
  callees: {
    symbolId: SymbolId;
    name: string;
    kind: SymbolKind;
    callCount: number;
  }[];
  
  history: {
    createdAt: string;
    createdBy: string;
    createdInCommit: string;
    lastModifiedAt: string;
    lastModifiedBy: string;
    lastModifiedInCommit: string;
    lastCallsiteTouchedAt?: string;
    totalCommits: number;
    commitFrequency: 'stable' | 'moderate' | 'volatile';
  };
  
  flags: {
    isPublicApi: boolean;
    isExported: boolean;
    isEntrypoint: boolean;
    hasTests: boolean;
    isDeprecated: boolean;
  };
}
```

### 5.5 Summary Sections

```typescript
interface ExplainSymbolSections {
  identity?: DerivedText;
  usage?: DerivedText;
  history?: DerivedText;
  risk?: DerivedText;  // Only with expand.justify
}
```

For `verbosity: 'tiny'`, sections may be `{}`.

### 5.6 Evidence Types (Enum)

```typescript
type ExplainSymbolEvidence =
  | ExplainCallerEvidence
  | ExplainCalleeEvidence
  | ExplainCommitEvidence;

interface ExplainCallerEvidence {
  type: 'caller';  // Literal
  symbolId: SymbolId;
  name: string;
  qualifiedName: string;
  moduleName: string;
  callCount: number;
  relevanceScore: number;
  ranking: RankingInfo;
  pointer: StablePointer;
}

interface ExplainCalleeEvidence {
  type: 'callee';  // Literal
  symbolId: SymbolId;
  name: string;
  callCount: number;
  relevanceScore: number;
  ranking: RankingInfo;
  pointer: StablePointer;
}

interface ExplainCommitEvidence {
  type: 'commit';  // Literal
  hash: string;
  message: string;
  author: string;
  date: string;
  linesChanged: number;
  category: 'origin' | 'recent' | 'significant';
  relevanceScore: number;
  ranking: RankingInfo;
}
```

### 5.7 Expanded Results

```typescript
interface JustifyMinimal {
  verdict: {
    recommendation: 'keep' | 'investigate' | 'remove-candidate';
    confidence: number;
    reasoning: string;
  };
}

interface CallGraphMinimal {
  callers: { symbolId: SymbolId; name: string; callCount: number }[];
  callees: { symbolId: SymbolId; name: string; callCount: number }[];
  stats: { totalCallers: number; totalCallees: number };
}
```

-----

## 6. Tool: justifySymbol

### 6.1 Purpose

Answer: “Why does this exist? Should it be kept or removed?”

### 6.2 Input Schema

```typescript
interface JustifySymbolInput {
  // Symbol identification (ONE of these — see precedence rules)
  symbolId?: SymbolId;
  filePath?: string;
  line?: number;
  column?: number;
  query?: string;
  pathHint?: string;
  
  verbosity?: 'tiny' | 'default' | 'full';
  includeRiskFactors?: boolean;
}
```

### 6.3 Output Schema

```typescript
interface JustifySymbolResponse extends AINavigationResponse<
  JustifySymbolFacts,
  JustifySymbolSections,
  JustifySymbolEvidence
> {}
```

### 6.4 Facts

```typescript
interface JustifySymbolFacts {
  symbol: {
    stableId: SymbolId;
    name: string;
    qualifiedName: string;
    kind: SymbolKind;
    visibility: 'public' | 'private' | 'internal' | 'unknown';
    moduleName: string;
  };
  
  origin: {
    introducedIn: {
      commit: string;
      date: string;
      author: string;
      message: string;
    };
    ageDays: number;
  };
  
  usage: {
    callerCount: number;
    externalCallerCount: number;
    testCallerCount: number;
    productionCallerCount: number;
    lastCallsiteTouchedAt?: string;
  };
  
  apiSurface: {
    isExported: boolean;
    isPublicApi: boolean;
    isEntrypoint: boolean;
    isInterfaceImpl: boolean;
    isOverride: boolean;
    isDeprecated: boolean;
  };
  
  riskFactors: {
    dynamicUsagePossible: boolean;
    usedInConfig: boolean;
    hasDecorators: boolean;
    isEventHandler: boolean;
    isPublicPackageExport: boolean;
    frameworkManagedRisk: boolean;
  };
  
  verdictInputs: {
    hasProductionCallers: boolean;
    hasOnlyTestCallers: boolean;
    hasNoCallers: boolean;
    hasKnownExternalReferences: boolean | 'unknown';
    isRecentlyAdded: boolean;
    isRecentlyUsed: boolean;
    hasRiskFactors: boolean;
  };
}
```

### 6.5 Summary Sections

```typescript
interface JustifySymbolSections {
  verdict?: {
    recommendation: 'keep' | 'investigate' | 'remove-candidate';
    confidence: number;
    reasoning: string;
  };
  origin?: DerivedText;
  justification?: DerivedText;
  nextSteps?: DerivedText;
}
```

### 6.6 Evidence Types (Enum)

```typescript
type JustifySymbolEvidence =
  | JustifyCommitEvidence
  | JustifyCallerEvidence
  | JustifyRiskEvidence;

interface JustifyCommitEvidence {
  type: 'commit';
  hash: string;
  message: string;
  author: string;
  date: string;
  linesChanged: number;
  category: 'origin' | 'recent';
  relevanceScore: number;
  ranking: RankingInfo;
}

interface JustifyCallerEvidence {
  type: 'caller';
  symbolId: SymbolId;
  name: string;
  moduleName: string;
  isTest: boolean;
  relevanceScore: number;
  ranking: RankingInfo;
  pointer: StablePointer;
}

interface JustifyRiskEvidence {
  type: 'risk';
  factor: string;
  description: string;
  relevanceScore: number;
  ranking: RankingInfo;
}
```

### 6.7 Verdict Logic

```typescript
function computeVerdict(facts: JustifySymbolFacts): Verdict {
  const { usage, apiSurface, riskFactors, verdictInputs } = facts;
  
  // === KEEP ===
  
  if (verdictInputs.hasProductionCallers) {
    return { recommendation: 'keep', confidence: 0.95,
      reasoning: `Has ${usage.productionCallerCount} production callers` };
  }
  
  if (apiSurface.isPublicApi && usage.externalCallerCount > 0) {
    return { recommendation: 'keep', confidence: 0.95,
      reasoning: `Public API with ${usage.externalCallerCount} external callers` };
  }
  
  if (apiSurface.isEntrypoint) {
    return { recommendation: 'keep', confidence: 0.9,
      reasoning: 'Entry point' };
  }
  
  if (apiSurface.isInterfaceImpl || apiSurface.isOverride) {
    return { recommendation: 'keep', confidence: 0.85,
      reasoning: 'Required by interface/base class' };
  }
  
  // === INVESTIGATE ===
  
  if (riskFactors.dynamicUsagePossible || riskFactors.frameworkManagedRisk) {
    return { recommendation: 'investigate', confidence: 0.7,
      reasoning: 'Possible dynamic/framework usage' };
  }
  
  if (riskFactors.hasDecorators || riskFactors.isEventHandler) {
    return { recommendation: 'investigate', confidence: 0.7,
      reasoning: 'May be invoked by framework/decorators/events' };
  }
  
  if (verdictInputs.hasOnlyTestCallers) {
    return { recommendation: 'investigate', confidence: 0.75,
      reasoning: `Only called by tests (${usage.testCallerCount})` };
  }
  
  if (verdictInputs.isRecentlyAdded && verdictInputs.hasNoCallers) {
    return { recommendation: 'investigate', confidence: 0.6,
      reasoning: 'Recently added with no callers—may be WIP' };
  }
  
  if (verdictInputs.hasKnownExternalReferences === 'unknown') {
    return { recommendation: 'investigate', confidence: 0.6,
      reasoning: 'Unable to determine external references' };
  }
  
  // === REMOVE CANDIDATE ===
  
  if (verdictInputs.hasNoCallers && !apiSurface.isExported &&
      !verdictInputs.hasRiskFactors && !verdictInputs.isRecentlyAdded) {
    return { recommendation: 'remove-candidate', confidence: 0.85,
      reasoning: 'No callers, not exported, no risk factors' };
  }
  
  if (facts.symbol.visibility === 'private' &&
      verdictInputs.hasNoCallers && !verdictInputs.isRecentlyAdded) {
    return { recommendation: 'remove-candidate', confidence: 0.8,
      reasoning: 'Private symbol with no callers' };
  }
  
  // Default
  return { recommendation: 'investigate', confidence: 0.5,
    reasoning: 'Unable to determine clear justification' };
}
```

-----

## 7. Tool: getCallGraph

### 7.1 Purpose

Answer: “What calls this, and what does it call?”

### 7.2 Input Schema

```typescript
interface GetCallGraphInput {
  // Symbol identification (ONE of these)
  symbolId?: SymbolId;
  filePath?: string;
  line?: number;
  column?: number;
  query?: string;
  pathHint?: string;
  
  direction?: 'callers' | 'callees' | 'both';
  depth?: number;  // 1-4, default 2
  maxNodes?: number;  // 1-100, default 30
  maxEdgesPerNode?: number;  // default 10
  rankBy?: 'frequency' | 'fanout' | 'churn' | 'distance';
  excludeTests?: boolean;
  excludeModules?: ModuleId[];
}
```

**Note**: When called via `expand.callGraph`, only `depth: 1` is allowed.

### 7.3 Output Schema

```typescript
interface GetCallGraphResponse extends AINavigationResponse<
  CallGraphFacts,
  CallGraphSections,
  CallGraphEvidence
> {}
```

### 7.4 Facts

```typescript
interface CallGraphFacts {
  root: {
    symbolId: SymbolId;
    name: string;
    kind: SymbolKind;
    moduleName: string;
  };
  
  // nodes[0] is always root
  nodes: CallGraphNode[];
  
  // All edges are calls (no kind field in v5.1)
  edges: CallGraphEdge[];
  
  stats: {
    totalCallers: number;
    totalCallees: number;
    nodesReturned: number;
    nodesAvailable?: number;  // Omit if unknown
    maxDepthReached: number;
  };
  
  patterns: {
    candidateEntrypoints: SymbolId[];
    leaves: SymbolId[];
    hubs: SymbolId[];
    hotspots: SymbolId[];
  };
}

interface CallGraphNode {
  symbolId: SymbolId;
  name: string;
  qualifiedName: string;
  kind: SymbolKind;
  moduleName: string;
  depth: number;  // 0=root, positive=callees, negative=callers
  direction: 'caller' | 'callee' | 'root';
  metrics: {
    fanIn: number;
    fanOut: number;
    frequency: number;
    churnScore?: number;
  };
  relevanceScore: number;
  ranking: RankingInfo;
}

interface CallGraphEdge {
  from: SymbolId;
  to: SymbolId;
  callCount: number;
  callSites: number;
  confidence: number;
  // No "kind" field in v5.1 — all edges are calls
}
```

### 7.5 Summary Sections

```typescript
interface CallGraphSections {
  overview?: DerivedText;
  callerSummary?: DerivedText;
  calleeSummary?: DerivedText;
}
```

### 7.6 Evidence Types (Enum)

```typescript
type CallGraphEvidence =
  | CallGraphNodeEvidence
  | CallGraphLimitationEvidence;

interface CallGraphNodeEvidence {
  type: 'node';
  symbolId: SymbolId;
  name: string;
  direction: 'caller' | 'callee';
  significance: 'entrypoint' | 'hub' | 'hotspot' | 'leaf' | 'normal';
  relevanceScore: number;
  ranking: RankingInfo;
  pointer?: StablePointer;  // Optional: may be absent for SCIP-only nodes without location
}

interface CallGraphLimitationEvidence {
  type: 'limitation';
  description: string;
  affectedScope: 'callers' | 'callees' | 'both';
  relevanceScore: number;
  ranking: RankingInfo;
}
```

**Note on `pointer`**: For nodes derived from SCIP call edges without occurrence data, `pointer` may be omitted. When this happens, add "Some call graph nodes lack precise location data" to `provenance.limitations`.

```
---

## 8. Tool: getModuleOverview

### 8.1 Purpose

Answer: "What is this module, what does it expose, what does it depend on?"

### 8.2 Input Schema

```typescript
interface GetModuleOverviewInput {
  moduleId?: ModuleId;
  path?: string;
  
  verbosity?: 'tiny' | 'default' | 'full';
  maxPublicSymbols?: number;
  maxDependencies?: number;
  maxCommits?: number;
  
  include?: {
    publicApi?: boolean;
    dependencies?: boolean;
    health?: boolean;
    recentChanges?: boolean;
  };
}
```

### 8.3 Output Schema

```typescript
interface GetModuleOverviewResponse extends AINavigationResponse<
  ModuleOverviewFacts,
  ModuleOverviewSections,
  ModuleOverviewEvidence
> {}
```

### 8.4 Facts

```typescript
interface ModuleOverviewFacts {
  module: {
    moduleId: ModuleId;
    name: string;
    path: string;
    isPackageRoot: boolean;
    manifestFile?: string;
  };
  
  size: {
    fileCount: number;
    symbolCount: number;
    publicSymbolCount: number;
    lineCount?: number;
  };
  
  publicApi: {
    exports: PublicSymbol[];
    entrypoints: PublicSymbol[];
  };
  
  keyAbstractions: {
    symbolId: SymbolId;
    name: string;
    kind: SymbolKind;
    internalRefCount: number;
    externalRefCount: number;
  }[];
  
  dependencies: {
    imports: ModuleDep[];
    importedBy: ModuleDep[];
    externalDeps: string[];
  };
  
  health: {
    churnScore: number;
    hotspotCount: number;
    couplingScore: number;
    hasTests: boolean;
    hasReadme: boolean;
    hasCyclicDeps: boolean;
    signals: HealthSignal[];
  };
  
  recentActivity: {
    commits: CommitSummary[];
    activeAuthors: string[];
    lastModified: string;
  };
}

interface PublicSymbol {
  symbolId: SymbolId;
  name: string;
  kind: SymbolKind;
  signature?: string;
  referenceCount: number;
}

interface ModuleDep {
  moduleId: ModuleId;
  moduleName: string;
  strength: number;
}

interface HealthSignal {
  signal: string;
  type: 'positive' | 'negative' | 'neutral';
  grounding: string;
}

interface CommitSummary {
  hash: string;
  message: string;
  author: string;
  date: string;
}
```

### 8.5 Summary Sections

```typescript
interface ModuleOverviewSections {
  oneLiner?: DerivedText;
  role?: DerivedText;
  dependencySummary?: DerivedText;
  healthAssessment?: DerivedText;
}
```

### 8.6 Evidence Types (Enum)

```typescript
type ModuleOverviewEvidence =
  | ModuleDepEvidence
  | ModuleExportEvidence
  | ModuleHotspotEvidence
  | ModuleCommitEvidence;

interface ModuleDepEvidence {
  type: 'dependency';
  moduleId: ModuleId;
  moduleName: string;
  direction: 'import' | 'imported-by';
  strength: number;
  relevanceScore: number;
  ranking: RankingInfo;
}

interface ModuleExportEvidence {
  type: 'export';
  symbolId: SymbolId;
  name: string;
  kind: SymbolKind;
  referenceCount: number;
  relevanceScore: number;
  ranking: RankingInfo;
  pointer: StablePointer;
}

interface ModuleHotspotEvidence {
  type: 'hotspot';
  filePath: string;
  churnScore: number;
  recentCommits: number;
  relevanceScore: number;
  ranking: RankingInfo;
}

interface ModuleCommitEvidence {
  type: 'commit';
  hash: string;
  message: string;
  author: string;
  date: string;
  filesChanged: number;
  relevanceScore: number;
  ranking: RankingInfo;
}
```

### 8.7 Canonical Drilldowns

Always include “what to look at first”:

```typescript
function generateModuleDrilldowns(facts: ModuleOverviewFacts): Drilldown[] {
  const drilldowns: Drilldown[] = [];
  
  if (facts.health.hotspotCount > 0) {
    drilldowns.push({
      label: 'Explore top hotspot',
      query: `explainSymbol --query=... --pathHint=${facts.module.path}`
    });
  }
  
  if (facts.keyAbstractions.length > 0) {
    const key = facts.keyAbstractions[0];
    drilldowns.push({
      label: `Key abstraction: ${key.name}`,
      query: `explainSymbol --symbolId=${key.symbolId}`
    });
  }
  
  if (facts.dependencies.imports.length > 0) {
    const top = facts.dependencies.imports[0];
    drilldowns.push({
      label: `Top dependency: ${top.moduleName}`,
      query: `getModuleOverview --moduleId=${top.moduleId}`
    });
  }
  
  return drilldowns;
}
```

-----

## 9. Enhanced: analyzeImpact

### 9.1 New Input Fields

```typescript
interface AnalyzeImpactInput {
  // ... existing ...
  changeType?: 'rename' | 'modify-signature' | 'delete' | 'move' | 'unknown';
  includeMigrationPlan?: boolean;
  includeSuggestedTests?: boolean;
}
```

### 9.2 Suggested Tests Detection

```typescript
interface SuggestedTests {
  summary: DerivedText;
  
  // Direct tests: test files that reference impacted symbol(s) or file(s)
  directTests: {
    filePath: string;
    testCount?: number;
    confidence: number;
  }[];
  
  // Transitive tests: tests that reference top caller modules or impacted modules
  transitiveTests: {
    filePath: string;
    pathThrough: SymbolId[];  // The chain from test to impacted symbol
    confidence: number;
  }[];
  
  coverage: {
    assessment: 'good' | 'partial' | 'minimal' | 'unknown';
    reasoning: string;
  };
  
  suggestedCommand?: string;
}
```

**Detection rules**:

- **Direct tests**: Files matching test patterns (see Section 14) that contain references to impacted symbols or files
- **Transitive tests**: Test files that reference modules containing top callers of impacted symbols

-----

## 10. Verbosity Levels

### 10.1 Definition

|Level    |maxEvidence|Summary Behavior           |
|---------|-----------|---------------------------|
|`tiny`   |5          |`tldr` only, `sections: {}`|
|`default`|10         |Full sections              |
|`full`   |30         |Full + extended evidence   |

### 10.2 Application

```typescript
function applyVerbosity(input: AINavigationInput): ResolvedLimits {
  const v = input.verbosity ?? 'default';
  
  const base = {
    tiny: { maxEvidence: 5, includeSections: false },
    default: { maxEvidence: 10, includeSections: true },
    full: { maxEvidence: 30, includeSections: true }
  }[v];
  
  // Expansion forces stricter limits
  if (input.expand) {
    return {
      ...base,
      verbosity: v === 'full' ? 'default' : v,
      maxEvidence: Math.min(input.maxEvidence ?? 20, 20)
    };
  }
  
  return {
    ...base,
    maxEvidence: input.maxEvidence ?? base.maxEvidence
  };
}
```

-----

## 11. Error Handling

### 11.1 Error Codes

```typescript
type AINavigationError =
  | 'INVALID_INPUT'        // No valid input variant
  | 'SYMBOL_NOT_FOUND'     // Symbol doesn't exist
  | 'SYMBOL_DELETED'       // Symbol was deleted (tombstone)
  | 'SYMBOL_AMBIGUOUS'     // Multiple matches, none clear winner
  | 'MODULE_NOT_FOUND'     // Module doesn't exist
  | 'BACKEND_UNAVAILABLE'  // Required backend not available
  | 'TIMEOUT'              // Query timed out
  | 'BUDGET_EXCEEDED';     // Hit resource limits

interface CkbError {
  code: AINavigationError;
  message: string;
  details?: Record<string, any>;
  drilldowns?: Drilldown[];
}
```

### 11.2 Error Details

```typescript
// SYMBOL_AMBIGUOUS
{
  code: 'SYMBOL_AMBIGUOUS',
  message: 'Multiple matches for "AuthService"',
  details: {
    alternatives: [
      { id: 'ckb:repo:sym:auth1', name: 'AuthService', path: 'lib/auth/', matchScore: 0.85 },
      { id: 'ckb:repo:sym:auth2', name: 'AuthService', path: 'lib/legacy/', matchScore: 0.82 }
    ]
  },
  drilldowns: [
    { label: 'Try: AuthService in lib/auth/', query: 'explainSymbol --symbolId=ckb:repo:sym:auth1' },
    { label: 'Try: AuthService in lib/legacy/', query: 'explainSymbol --symbolId=ckb:repo:sym:auth2' }
  ]
}

// SYMBOL_DELETED
{
  code: 'SYMBOL_DELETED',
  message: 'Symbol was deleted',
  details: {
    deletedAt: '2024-12-01T10:00:00Z',
    deletedInCommit: 'abc123'
  }
}
```

-----

## 12. Ranking Policy v5.1

### 12.1 General Pattern

Score = Σ(weight × normalized(signal))

Every ranked item includes:

```typescript
ranking: {
  signals: { /* the inputs */ },
  policyVersion: "5.1"
}
```

### 12.2 Caller Ranking

**Signals**:

1. `callCount` (SCIP) or `refCount` (fallback)
1. `isEntrypoint` (1.5× boost)
1. `sameModule` (1.15× boost)
1. `recency` (1.0 + min(0.25, max(0, (30 - daysSinceChanged) / 120)))

```typescript
function scoreCaller(c: Caller): number {
  const freq = Math.log1p(c.callCount ?? c.refCount);
  const entry = c.isEntrypoint ? 1.5 : 1.0;
  const module = c.sameModule ? 1.15 : 1.0;
  const days = daysBetween(c.lastChanged, now);
  const recency = 1.0 + Math.min(0.25, Math.max(0, (30 - days) / 120));
  return freq * entry * module * recency;
}
```

**Entrypoint heuristic**:

- Path contains: `controller`, `handler`, `route`, `api`, `cli`, `main`
- Name matches: `main`, `handle*`, `on*`, `*Controller.*`

### 12.3 Callee Ranking

```typescript
function scoreCallee(c: Callee): number {
  const freq = Math.log1p(c.callCount ?? c.refCount);
  const crossModule = c.sameModule ? 1.0 : 1.2;
  const volatility = (c.churnScore ?? 0) > 0.7 ? 1.15 : 1.0;
  return freq * crossModule * volatility;
}
```

### 12.4 Commit Ranking

**Categories** (include one from each if available):

1. `origin`: First commit
1. `recent`: Most recent behavioral change
1. `significant`: Largest diff

```typescript
function scoreCommit(c: Commit): number {
  let score = Math.log1p(c.linesChanged);
  if (/fix|bug|refactor|perf|security|break/i.test(c.message)) {
    score *= 1.3;
  }
  return score;
}
```

### 12.5 Call Graph Node Ranking

By `rankBy` parameter:

|rankBy     |Formula                    |
|-----------|---------------------------|
|`frequency`|`node.frequency`           |
|`fanout`   |`node.fanIn + node.fanOut` |
|`churn`    |`node.churnScore ?? 0`     |
|`distance` |`1 / (abs(node.depth) + 1)`|

**Pattern detection**:

- `candidateEntrypoints`: `fanIn === 0 && fanOut >= 1`, sorted by fanOut desc
- `hubs`: Sorted by `fanIn × fanOut × (1 + churnScore)`
- `hotspots`: Sorted by churnScore desc

### 12.6 Module Dependency Strength

```typescript
function computeStrength(dep: Dependency): number {
  const importWeight = 0.6;
  const symbolWeight = 0.4;
  return importWeight * normalize(dep.importCount) +
         symbolWeight * normalize(dep.symbolsUsed ?? 0);
}
```

If `symbolsUsed` unavailable, use `importCount` only.

-----

## 13. Backend Staleness Rules

### 13.1 SCIP Staleness

`backendCoverage.scip` is `'stale'` if ANY of:

1. `indexedCommit !== HEAD` (not current commit)
1. Index file age > 7 days
1. Files touched since `indexedCommit` exist (git diff has changes)

```typescript
function isScipStale(index: ScipIndex, repoState: RepoState): boolean {
  // Rule 1: Not at HEAD
  if (index.indexedCommit !== repoState.headCommit) {
    return true;
  }
  
  // Rule 2: Old index
  const indexAgeDays = daysBetween(index.indexedAt, now);
  if (indexAgeDays > 7) {
    return true;
  }
  
  // Rule 3: Dirty working tree
  if (repoState.repoStateDirty) {
    return true;
  }
  
  return false;
}
```

### 13.2 LSP Coverage

`backendCoverage.lsp` is:

- `'available'`: LSP ready, workspace fully loaded
- `'partial'`: LSP running but workspace still indexing
- `'missing'`: LSP not running or failed to start

-----

## 14. Test Detection Rules

### 14.1 Test File Heuristics

A file is considered a test file if ANY of:

1. Path contains: `/test/`, `/tests/`, `/__tests__/`, `/spec/`
1. Filename matches: `*_test.dart`, `*_test.go`, `*.test.ts`, `*.spec.ts`, `test_*.py`, `*_test.py`
1. Filename is exactly: `test.dart`, `test.go`, etc.

```typescript
function isTestFile(filePath: string): boolean {
  // Path patterns
  if (/[\/\\](tests?|__tests__|spec)[\/\\]/i.test(filePath)) {
    return true;
  }
  
  // Filename patterns
  const filename = path.basename(filePath);
  const testPatterns = [
    /_test\.(dart|go|rs)$/,
    /\.test\.(ts|tsx|js|jsx)$/,
    /\.spec\.(ts|tsx|js|jsx)$/,
    /^test_.*\.py$/,
    /_test\.py$/
  ];
  
  return testPatterns.some(p => p.test(filename));
}
```

### 14.2 hasTests Meaning

`flags.hasTests` means: At least one test file in the workspace references this symbol.

`usage.testCallerCount` means: Number of call sites from test files.

-----

## 15. Implementation Priority

### Phase 1: Core (Weeks 1-2)

|Task             |DoD                                  |
|-----------------|-------------------------------------|
|Symbol resolution|All variants work, precedence correct|
|`explainSymbol`  |Full facts/summary/evidence          |
|`justifySymbol`  |Conservative verdicts                |
|Ranking policy   |Auditable signals                    |

### Phase 2: Graphs (Week 3)

|Task              |DoD                         |
|------------------|----------------------------|
|`getCallGraph`    |Depth ≤ 2, patterns detected|
|`expand.callGraph`|Caps enforced               |

### Phase 3: Modules (Week 4)

|Task                |DoD            |
|--------------------|---------------|
|`getModuleOverview` |No fake metrics|
|Canonical drilldowns|Always present |

### Phase 4: Enhancements (Week 5)

|Task            |DoD              |
|----------------|-----------------|
|`expand.justify`|Minimal verdict  |
|`analyzeImpact` |Migration + tests|

-----

## 16. MCP Schemas

### 16.1 explainSymbol

```json
{
  "name": "explainSymbol",
  "description": "Comprehensive symbol explanation. Accepts symbolId OR location OR query.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "symbolId": { "type": "string", "description": "Direct symbol ID" },
      "filePath": { "type": "string", "description": "File path for location-based lookup" },
      "line": { "type": "integer", "description": "Line number (1-based)" },
      "column": { "type": "integer", "description": "Column number (1-based)" },
      "query": { "type": "string", "description": "Symbol name or qualified name to search" },
      "pathHint": { "type": "string", "description": "Path hint to narrow search" },
      "verbosity": { "type": "string", "enum": ["tiny", "default", "full"] },
      "maxEvidence": { "type": "integer", "minimum": 1, "maximum": 50 },
      "includeEvidence": { "type": "boolean" },
      "expand": {
        "type": "object",
        "properties": {
          "justify": { "type": "boolean" },
          "callGraph": {
            "type": "object",
            "properties": {
              "depth": { "type": "integer", "enum": [1] },
              "direction": { "type": "string", "enum": ["callers", "callees", "both"] }
            }
          }
        }
      }
    },
    "oneOf": [
      { "required": ["symbolId"] },
      { "required": ["filePath", "line"] },
      { "required": ["query"] }
    ]
  }
}
```

### 16.2 justifySymbol

```json
{
  "name": "justifySymbol",
  "description": "Analyze why a symbol exists. Conservative dead-code detection.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "symbolId": { "type": "string" },
      "filePath": { "type": "string" },
      "line": { "type": "integer" },
      "column": { "type": "integer" },
      "query": { "type": "string" },
      "pathHint": { "type": "string" },
      "verbosity": { "type": "string", "enum": ["tiny", "default", "full"] },
      "includeRiskFactors": { "type": "boolean" }
    },
    "oneOf": [
      { "required": ["symbolId"] },
      { "required": ["filePath", "line"] },
      { "required": ["query"] }
    ]
  }
}
```

### 16.3 getCallGraph

```json
{
  "name": "getCallGraph",
  "description": "Get call graph: callers and callees with depth limiting and ranking.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "symbolId": { "type": "string" },
      "filePath": { "type": "string" },
      "line": { "type": "integer" },
      "column": { "type": "integer" },
      "query": { "type": "string" },
      "pathHint": { "type": "string" },
      "direction": { "type": "string", "enum": ["callers", "callees", "both"], "default": "both" },
      "depth": { "type": "integer", "minimum": 1, "maximum": 4, "default": 2 },
      "maxNodes": { "type": "integer", "minimum": 1, "maximum": 100, "default": 30 },
      "maxEdgesPerNode": { "type": "integer", "default": 10 },
      "rankBy": { "type": "string", "enum": ["frequency", "fanout", "churn", "distance"], "default": "frequency" },
      "excludeTests": { "type": "boolean", "default": false },
      "excludeModules": { "type": "array", "items": { "type": "string" } }
    },
    "oneOf": [
      { "required": ["symbolId"] },
      { "required": ["filePath", "line"] },
      { "required": ["query"] }
    ]
  }
}
```

### 16.4 getModuleOverview

```json
{
  "name": "getModuleOverview",
  "description": "Module overview: public API, dependencies, health signals.",
  "inputSchema": {
    "type": "object",
    "properties": {
      "moduleId": { "type": "string" },
      "path": { "type": "string" },
      "verbosity": { "type": "string", "enum": ["tiny", "default", "full"] },
      "maxPublicSymbols": { "type": "integer" },
      "maxDependencies": { "type": "integer" },
      "maxCommits": { "type": "integer" },
      "include": {
        "type": "object",
        "properties": {
          "publicApi": { "type": "boolean", "default": true },
          "dependencies": { "type": "boolean", "default": true },
          "health": { "type": "boolean", "default": true },
          "recentChanges": { "type": "boolean", "default": true }
        }
      }
    },
    "oneOf": [
      { "required": ["moduleId"] },
      { "required": ["path"] }
    ]
  }
}
```

-----

## 17. Appendix: Example Responses

### 17.1 explainSymbol (default verbosity)

```json
{
  "ckbVersion": "0.5.1",
  "schemaVersion": 1,
  "tool": "explainSymbol",
  
  "resolved": {
    "symbolId": "ckb:tastehub:sym:auth-refresh",
    "resolvedFrom": "query",
    "confidence": 0.95,
    "ambiguous": false
  },
  
  "facts": {
    "symbol": {
      "stableId": "ckb:tastehub:sym:auth-refresh",
      "name": "refreshToken",
      "qualifiedName": "AuthService.refreshToken",
      "kind": "method",
      "signature": "(userId: String, token: String) -> Future<TokenPair>",
      "visibility": "public",
      "moduleId": "ckb:tastehub:mod:auth",
      "moduleName": "auth",
      "filePath": "lib/auth/auth_service.dart",
      "location": { "startLine": 45, "startCol": 3, "endLine": 72, "endCol": 4 }
    },
    "usage": {
      "callerCount": 23,
      "calleeCount": 8,
      "referenceCount": 31,
      "moduleCount": 4
    },
    "usageByModule": [
      { "moduleId": "ckb:tastehub:mod:api", "moduleName": "api", "callerCount": 15, "percentage": 65 },
      { "moduleId": "ckb:tastehub:mod:auth", "moduleName": "auth", "callerCount": 5, "percentage": 22 }
    ],
    "callees": [
      { "symbolId": "ckb:tastehub:sym:validate", "name": "validateToken", "kind": "method", "callCount": 2 }
    ],
    "history": {
      "createdAt": "2024-01-15",
      "createdBy": "alice@tastehub.com",
      "createdInCommit": "a1b2c3d",
      "lastModifiedAt": "2024-11-20",
      "lastModifiedBy": "bob@tastehub.com",
      "lastModifiedInCommit": "x9y8z7",
      "lastCallsiteTouchedAt": "2024-12-01",
      "totalCommits": 12,
      "commitFrequency": "stable"
    },
    "flags": {
      "isPublicApi": true,
      "isExported": true,
      "isEntrypoint": false,
      "hasTests": true,
      "isDeprecated": false
    }
  },
  
  "summary": {
    "tldr": "JWT refresh handler, stable, used by 4 modules",
    "sections": {
      "identity": {
        "text": "Public method in auth module that refreshes JWT tokens",
        "derivedFrom": ["symbol.kind", "symbol.signature", "symbol.moduleName"],
        "confidence": 0.9
      },
      "usage": {
        "text": "Called 23 times across 4 modules, primarily by api (65%)",
        "derivedFrom": ["usage.callerCount", "usage.moduleCount", "usageByModule"],
        "confidence": 0.95
      },
      "history": {
        "text": "Stable code, 12 commits, last modified 26 days ago by bob@",
        "derivedFrom": ["history.totalCommits", "history.commitFrequency", "history.lastModifiedAt"],
        "confidence": 0.95
      }
    }
  },
  
  "evidence": [
    {
      "type": "caller",
      "symbolId": "ckb:tastehub:sym:api-handler",
      "name": "handleRefresh",
      "qualifiedName": "AuthController.handleRefresh",
      "moduleName": "api",
      "callCount": 8,
      "relevanceScore": 0.95,
      "ranking": {
        "signals": { "callCount": 8, "isEntrypoint": true, "sameModule": false },
        "policyVersion": "5.1"
      },
      "pointer": {
        "repoStateId": "abc123",
        "filePath": "lib/api/auth_controller.dart",
        "symbolId": "ckb:tastehub:sym:api-handler",
        "range": { "startLine": 32, "endLine": 32 }
      }
    },
    {
      "type": "commit",
      "hash": "x9y8z7",
      "message": "Fix token expiry for long sessions",
      "author": "bob@tastehub.com",
      "date": "2024-11-20",
      "linesChanged": 15,
      "category": "recent",
      "relevanceScore": 0.9,
      "ranking": {
        "signals": { "linesChanged": 15, "category": "recent", "messageMatch": true },
        "policyVersion": "5.1"
      }
    }
  ],
  
  "provenance": {
    "repoStateId": "abc123def456",
    "repoStateDirty": false,
    "backendCoverage": {
      "scip": "available",
      "lsp": "available",
      "git": "available"
    },
    "sources": {
      "scip": { "indexPath": ".scip/index.scip", "indexedCommit": "x9y8z7", "indexedAt": "2024-12-15" },
      "git": { "commitRange": "HEAD~100..HEAD" }
    },
    "completeness": {
      "scope": "workspace",
      "estimate": "high",
      "rationale": "SCIP index available and fresh"
    },
    "confidence": {
      "overall": 0.92,
      "bySection": { "identity": 0.9, "usage": 0.95, "history": 0.95 }
    },
    "rankingPolicyVersion": "5.1",
    "limitations": ["Entrypoint detection uses naming/path heuristics"],
    "computedAt": "2024-12-16T10:30:00Z",
    "durationMs": 234
  },
  
  "truncation": {
    "truncated": true,
    "reason": "max-evidence",
    "returned": 10,
    "available": 23
  },
  
  "drilldowns": [
    {
      "label": "See all 23 callers",
      "query": "getCallGraph --symbolId=ckb:tastehub:sym:auth-refresh --direction=callers --maxNodes=50"
    }
  ]
}
```

### 17.2 justifySymbol (remove-candidate)

```json
{
  "ckbVersion": "0.5.1",
  "schemaVersion": 1,
  "tool": "justifySymbol",
  
  "resolved": {
    "symbolId": "ckb:tastehub:sym:old-helper",
    "resolvedFrom": "query",
    "confidence": 0.9,
    "ambiguous": false
  },
  
  "facts": {
    "symbol": {
      "stableId": "ckb:tastehub:sym:old-helper",
      "name": "_oldMigrationHelper",
      "qualifiedName": "utils._oldMigrationHelper",
      "kind": "function",
      "visibility": "private",
      "moduleName": "utils"
    },
    "origin": {
      "introducedIn": {
        "commit": "old123",
        "date": "2023-06-15",
        "author": "former@employee.com",
        "message": "Add helper for legacy migration"
      },
      "ageDays": 549
    },
    "usage": {
      "callerCount": 0,
      "externalCallerCount": 0,
      "testCallerCount": 0,
      "productionCallerCount": 0
    },
    "apiSurface": {
      "isExported": false,
      "isPublicApi": false,
      "isEntrypoint": false,
      "isInterfaceImpl": false,
      "isOverride": false,
      "isDeprecated": false
    },
    "riskFactors": {
      "dynamicUsagePossible": false,
      "usedInConfig": false,
      "hasDecorators": false,
      "isEventHandler": false,
      "isPublicPackageExport": false,
      "frameworkManagedRisk": false
    },
    "verdictInputs": {
      "hasProductionCallers": false,
      "hasOnlyTestCallers": false,
      "hasNoCallers": true,
      "hasKnownExternalReferences": false,
      "isRecentlyAdded": false,
      "isRecentlyUsed": false,
      "hasRiskFactors": false
    }
  },
  
  "summary": {
    "tldr": "Private unused function, safe to remove",
    "sections": {
      "verdict": {
        "recommendation": "remove-candidate",
        "confidence": 0.85,
        "reasoning": "Private symbol with no callers"
      },
      "origin": {
        "text": "Added 549 days ago by former@employee.com for legacy migration",
        "derivedFrom": ["origin.introducedIn", "origin.ageDays"],
        "confidence": 0.95
      },
      "justification": {
        "text": "No current justification—zero callers in production or tests",
        "derivedFrom": ["usage"],
        "confidence": 0.9
      },
      "nextSteps": {
        "text": "Safe to remove. Verify with: grep -r '_oldMigrationHelper'",
        "derivedFrom": ["riskFactors", "verdict"],
        "confidence": 0.8
      }
    }
  },
  
  "evidence": [
    {
      "type": "commit",
      "hash": "old123",
      "message": "Add helper for legacy migration",
      "author": "former@employee.com",
      "date": "2023-06-15",
      "linesChanged": 42,
      "category": "origin",
      "relevanceScore": 1.0,
      "ranking": {
        "signals": { "category": "origin" },
        "policyVersion": "5.1"
      }
    }
  ],
  
  "provenance": {
    "repoStateId": "abc123def456",
    "repoStateDirty": false,
    "backendCoverage": { "scip": "available", "lsp": "available", "git": "available" },
    "sources": { "scip": { "indexPath": ".scip/index.scip", "indexedCommit": "x9y8z7", "indexedAt": "2024-12-15" } },
    "completeness": { "scope": "workspace", "estimate": "high", "rationale": "SCIP available" },
    "confidence": { "overall": 0.85, "bySection": { "verdict": 0.85, "usage": 0.95 } },
    "rankingPolicyVersion": "5.1",
    "limitations": [],
    "computedAt": "2024-12-16T10:35:00Z",
    "durationMs": 156
  },
  
  "truncation": { "truncated": false, "returned": 1 },
  
  "drilldowns": []
}
```

-----

*End of CKB v5.1 Implementation-Ready Specification*
