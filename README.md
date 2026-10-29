# xml2l — Salesforce Profile Normalizer & Analyzer

> The name `xml2l` is a shorthand for "XML Tool" — *2* reads as *to*/*too*/*tool*.

Spot differences between Salesforce profiles. Backfill missing entries. Understand what your profiles actually grant.

`xml2l` decodes `.profile-meta.xml` files from an SFDX project, builds a complete graph of every profile-to-permission relationship, and lets you:

- **Diff** profiles against each other — find out which Apex classes, objects, fields, tabs, or flows one profile has and another doesn't
- **Normalize** profiles — backfill every permission entry the Master Schema knows about, sorted and consistent, so a save diffs cleanly against a fresh retrieve
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

3. **Normalize** (optional) — For each profile, backfill every entry from the Master Schema that the profile doesn't already have. Sort every top-level element and every permission property by tag name — the order Salesforce itself writes on retrieve — so a save leaves no formatting diff. Permission properties this build does not model are preserved verbatim and reported on stderr. Write consistent XML back to disk.

The graph model is bipartite — one side is profiles, the other is permission targets (classes, fields, objects, tabs, flows, custom permissions, etc.). Edges carry the specific permission flags (`enabled`, `readable`, `editable`, `allowCreate`, `visible`, `hidden`, `visibility`, etc.).

## Supported profile sections

| Section | Permission flags | Default on backfill |
|---|---|---|
| `classAccesses` | enabled | `enabled: false` |
| `agentAccesses` | enabled | `enabled: false` |
| `fieldPermissions` | readable, editable | `readable: false`, `editable: false` |
| `fieldLevelSecurities` | hidden | preserved verbatim (not modeled or backfilled — `fieldPermissions` covers visibility) |
| `objectPermissions` | allowRead, allowCreate, allowEdit, allowDelete, modifyAll, viewAll, viewAllFields | the first six `: false`; `viewAllFields` is preserved but never backfilled (Salesforce omits it on orgs that do not support it) |
| `pageAccesses` | enabled | `enabled: false` |
| `layoutAssignments` | layout, recordType | not backfilled (Salesforce requires at most one layout per object without recordType) |
| `tabVisibilities` | visibility | `visibility: "Hidden"` |
| `recordTypeVisibilities` | visible, default, recordType | `visible: false`, `default: false` |
| `userPermissions` | enabled | `enabled: false` |
| `flowAccesses` | enabled | `enabled: false` |
| `applicationVisibilities` | visible, default | `visible: false`, `default: false` |
| `customPermissions` | enabled | `enabled: false` |
| `customMetadataTypeAccesses` | enabled | `enabled: false` |
| `customSettingAccesses` | enabled | `enabled: false` |
| `categoryGroupVisibilities` | visible | no boolean defaults (string-typed) |
| `loginFlows` | enabled | no boolean defaults (unstructured/profile-level) |
| `externalDataSourceAccesses` | enabled | `enabled: false` |
| `servicePresenceStatusAccesses` | enabled | `enabled: false` |
| `genComputingSummaryDefAccess` | enabled | `enabled: false` |

`--path` (`-p`) points at a Salesforce metadata directory such as
`force-app/main/default`. `--exclude` (`-e`, save only) takes comma-separated,
case-insensitive **prefixes**: a matching entry is copied from the source
instead of being normalized, so it keeps the property order of the file (only
its indentation is normalized). Every other entry in that profile is still
sorted.

`--entry-point-classes` (save only) rewrites `classAccesses` so a profile grants
Apex class access only to classes Salesforce invokes on behalf of the running
user:

| Entry point | Detected from |
|---|---|
| Visualforce controller or extension | `controller` / `extensions` on `.page` and `.component` files |
| Aura controller | `controller` on `.cmp` / `.app` / `.evt` |
| LWC controller | `import … from '@salesforce/apex/Class.method'` |
| REST endpoint | `@RestResource` on the class |
| SOAP endpoint | a `webservice` method |
| Flow invocable | `<apexClass>` in a flow's action call |

Every other `classAccesses` entry is removed and a required class missing from
the profile is added with `enabled: true`; other sections are untouched. A class
carrying `@AuraEnabled` or `@InvocableMethod` but referenced from nowhere is not
required, and neither are trigger handlers — triggers run in system context.
Excluded entries are never pruned. `--dry-run` prints the per-profile change and
writes nothing:

```
Admin: 44 enabled → 5, removed 39, added 0
```

Pruning is intentionally not retrieve-faithful for class access: the next
`sf project retrieve` restores the grants stored in the org (see
[ADR-0004](docs/adr/0004-entry-point-class-access.md)).

```bash
# Diff all profiles — see what each profile is missing relative to others
./xml2l profile diff --path ./my-sfdx-project/main/default

# Diff with detailed value comparison
./xml2l profile diff --path ./my-sfdx-project/main/default --details

# Diff and cross-reference against the filesystem
./xml2l profile diff --path ./my-sfdx-project/main/default --repo

# Normalize all profiles — backfill missing entries, sort, write
./xml2l profile save --path ./my-sfdx-project/main/default

# Normalize profiles with org schema cross-check
./xml2l profile save --path ./my-sfdx-project/main/default --use-org-schema

# Normalize profiles with org schema cross-check against specific org
./xml2l profile save --path ./my-sfdx-project/main/default --use-org-schema --org myOrg

# Normalize profiles with org schema cross-check against specific org and exclude specific metadata
./xml2l profile save --path ./my-sfdx-project/main/default --use-org-schema --org myOrg --exclude "chatter"

# Keep class access to Apex entry points only — report the change first
./xml2l profile save --path ./my-sfdx-project/main/default --entry-point-classes --dry-run
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
- **Layout assignments filtered and defaulted** — each profile keeps its own `layoutAssignments` entries (no backfilling from other profiles); entries are filtered to enforce Salesforce rules (at most one layout per object without a `recordType`, no duplicate `recordType` values); when a profile has no layout assignment for an object, a default is added from the first matching layout file on disk
- **Sorted sections** — entries within each section are sorted alphabetically
- **Preserved unmodeled sections** — every top-level element the tool doesn't model (`loginHours`, `loginIpRanges`, `loginFlows`, `profileActionOverrides`, `fieldLevelSecurities`, and anything else) is extracted from the source and re-inserted verbatim, in document order
- **Org schema filtering (optional)** — when `--use-org-schema` (`-s`) is passed, entries are cross-checked against the Salesforce org via `sf org list metadata`; any entry whose metadata name does not exist in the org is skipped (overrides repository and graph checks); use `--org` (`-o`) to target a specific org alias. A type whose query fails (transient error, unsupported type, missing permission) is **not** filtered — the tool only removes entries when the org explicitly reports that it has none

The goal is deterministic output: run it twice, get identical files.

## Graph exports - (in progress)

Diff reports can include a filesystem cross-reference (`--repo`), and the internal graph can be serialized to:

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
