# Cognitive Vault — Vault & Folder Structure Specification (v1.1 Draft)

## Overview
Best practice design for solo builders, knowledge workers, and teams.

---

# 1. Mental Model

Cognitive Vault has exactly three levels:

| Level | Name   | Description |
|------|--------|-------------|
| 1    | Vault  | Context boundary (access, AI scope) |
| 2    | Folder | Thematic grouping |
| 3    | Entry  | Atomic unit of knowledge |

**Principle:** A vault is not a folder.

---

# 2. Core Design Rules

## Rule 1 — Max two folder levels
Deep nesting signals mis-scoped vaults.

## Rule 2 — One folder, one purpose
Folders must be describable as a single noun.

## Rule 3 — Folders for topic, tags for attribute
Never use folders for status, time, or priority.

## Rule 4 — Every entry has at least two tags
- type
- status

## Rule 5 — Archive in place
Use `status:archived`, never move files.

## Rule 6 — Vault names are permanent

---

# 3. Archetypes

- Personal Context
- Project
- Domain Knowledge
- People & Relationships
- Operations

Each vault should serve exactly one archetype.

---

# 4. Naming Conventions

- lowercase only
- kebab-case
- predictable formats

Example:
```
2026-02-19-database-choice.md
```

---

# 5. Tag Taxonomy

## Required
- type:
- status:

## Recommended
- project:
- person:
- priority:
- quarter:

---

# 6. Entry Frontmatter

## Minimal
```
---
title: "..."
date: YYYY-MM-DD
tags: [type:..., status:active]
---
```

---

# 🔁 v1.1 Improvements

## 1. Summary Field Requirement

The `summary` field is **required** for:

- decision
- spec
- procedure
- policy

### Why
The summary is used as the primary retrieval preview for AI systems. Without it, retrieval quality degrades.

---

## 2. Tooling Contract

The CLI must enforce structure.

### Entry Creation
`cv entry create` must:

- auto-generate frontmatter
- require type
- default status:active
- require summary (when needed)
- validate tags

### Linting
```
cv vault lint
```

Checks:
- missing tags
- missing summary
- inconsistent tags
- deep nesting
- duplicates

---

## 3. Density Thresholds

Replace vague rules with measurable signals:

### Folder
> >50 entries → split

### Vault
> >300 entries → consider new vault

---

# Design Philosophy

Cognitive Vault is:

> A structured, AI-queryable memory system

NOT:

> A flexible note-taking app

Structure is required for retrieval quality.
Friction must be removed through tooling, not by lowering standards.
