# xml2l — Salesforce Profile Normalizer & Analyzer

> The name `xml2l` is a shorthand for "XML Tool" — *2* reads as *to*/*too*/*tool*.

Spot differences between Salesforce profiles. Backfill missing entries. Understand what your profiles actually grant.

`xml2l` decodes `.profile-meta.xml` files from an SFDX project, builds a complete graph of every profile-to-permission relationship, and lets you:

- **Diff** profiles against each other — find out which Apex classes, objects, fields, tabs, or flows one profile has and another doesn't
- **Normalize** profiles — backfill every permission entry the Master Schema knows about, sorted and consistent, so your files are deterministic
- **Cross-reference** profile permissions against the filesystem — flag metadata referenced in profiles but missing from disk, and vice versa
- **Export** the permission graph as JSON, Graphviz DOT, or Cytoscape.js — for visualization or pipeline consumption

## Installation

```bash
git clone https://github.com/KamilGolis/xml2l.git
cd xml2l
go build -o xml2l ./cmd/engine/
```

## How it works

The tool processes profiles through three phases:

1. **Scan** — Walk the SFDX project, build a ground-truth map of all `.cls` and `.field-meta.xml` files that exist on disk. Filters out ghost references.

2. **Schema** (concurrent) — Read every `.profile-meta.xml` file in parallel. Aggregate every permission entry into a single Master Schema: the union of everything every profile knows about. Build a bipartite graph of `ProfileNode ↔ MetadataNode` edges with permission properties.

3. **Normalize** (optional) — For each profile, backfill every entry from the Master Schema that the profile doesn't already have. Sort all sections alphabetically. Write consistent XML back to disk.

The graph model is bipartite — one side is profiles, the other is permission targets (classes, fields, objects, tabs, flows, custom permissions, etc.). Edges carry the specific permission flags (`enabled`, `readable`, `editable`, `allowCreate`, `visible`, `hidden`, `visibility`, etc.).

## Supported profile sections

| Section | Permission flags | Default on backfill |
|---|---|---|
| `classAccesses` | enabled | `enabled: false` |
| `agentAccesses` | enabled | `enabled: false` |
| `fieldPermissions` | readable, editable | `readable: false`, `editable: false` |
| `fieldLevelSecurities` | hidden | — (not backfilled as separate section — `fieldPermissions` covers visibility) |
| `objectPermissions` | allowRead, allowCreate, allowEdit, allowDelete, modifyAll, viewAll | all six `: false` |
| `layoutAssignments` | layout, recordType | no boolean defaults (string/unstructured) |
| `tabVisibilities` | visibility | `visibility: "Hidden"` |
| `recordTypeVisibilities` | visible, default, recordType | `visible: false`, `default: false` |
| `userPermissions` | enabled | `enabled: false` |
| `flowAccesses` | enabled | `enabled: false` |
| `appVisibilities` | visible, default | `visible: false`, `default: false` |
| `customPermissions` | enabled | `enabled: false` |
| `customMetadataTypeAccesses` | enabled | `enabled: false` |
| `customSettingAccesses` | enabled | `enabled: false` |
| `categoryGroupVisibilities` | visible | no boolean defaults (string-typed) |
| `loginFlows` | enabled | no boolean defaults (unstructured/profile-level) |
| `externalDataSourceAccesses` | enabled | `enabled: false` |
| `servicePresenceAccesses` | enabled | `enabled: false` |
| `enhancedSummary` | enabled | `enabled: false` |
```bash
# Diff all profiles — see what each profile is missing relative to others
./xml2l profile diff --path ./my-sfdx-project/main/default

# Diff with detailed value comparison
./xml2l profile diff --path ./my-sfdx-project/main/default --details

# Diff and cross-reference against the filesystem
./xml2l profile diff --path ./my-sfdx-project/main/default --repo

# Normalize all profiles — backfill missing entries, sort, write
./xml2l profile save --path ./my-sfdx-project/main/default

```

### Example output

```
[Admin]
  ApexClass (3):
    SomeClass
    AnotherClass
  objects (1):
    Account.Revenue__c

[Value Differences]
  FieldPermissions: editable on Account.Description
    SomeProfile → true
    OtherProfile → false
```

## What gets normalized

When you run `profile save`, every profile gets:

- **Backfilled entries** — if any profile references a piece of metadata, every profile gets an entry for it (with all booleans set to `false`)
- **Sorted sections** — entries within each section are sorted alphabetically
- **Preserved orphan sections** — `loginHours`, `loginIpRanges`, `profileActionOverrides` are extracted and re-inserted verbatim, even though the tool doesn't model them in the graph

The goal is deterministic output: run it twice, get identical files.

## Graph exports - (in progess)

Diff reports can optionally include a filesystem cross-reference (`--repo`), and the internal graph can be serialized to:

- **JSON** — Cytoscape.js-compatible format for web visualization
- **DOT** — Graphviz format for rendering with `dot` or `neato`
- **Cytoscape.js** — capped per-type summaries for large graphs

> **Note:** Graph export to JSON, DOT, and Cytoscape.js is currently in implementation and not available yet. The internal graph structures and export types are laid out, but the CLI flags and output plumbing are unfinished.

## Project structure

```
├── cmd/engine/main.go           CLI entry point (cobra commands)
├── internal/
│   ├── graph/                   Bipartite graph model + DOT/JSON/Cytoscape export
│   ├── normalizer/              Phase 3: backfill + sort + serialize to XML
│   ├── profile/                 Phase 1: decode a single .profile-meta.xml
│   ├── report/                  Diff computation + filesystem cross-reference
│   ├── scanner/                 Phase 1: walk filesystem for ground-truth
│   └── schema/                  Phase 2: concurrent decode + Master Schema
├── go.mod
└── go.sum
```
