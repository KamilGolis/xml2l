# Profile save can prune Apex class access to entry points

`profile save --entry-point-classes` writes a profile's `classAccesses` from the
project's Apex entry points only: classes reached through Aura/LWC, Visualforce,
REST/SOAP, or Flow. A class carrying `@AuraEnabled` or `@InvocableMethod` without
a call site is not an entry point, and neither are trigger handlers, because
triggers run in system context. The rule is computed from the checkout, which is
the source of truth (ADR-0001), so every profile of a project is pruned the same way.

This mode deliberately stops being retrieve-faithful for class access (ADR-0002):
the org stores the grants, so the next retrieve restores them and the next save
prunes again. The alternative is the default path, which backfills every class in
the master schema with `enabled: false`; that keeps profiles large and hides which
classes a user can actually run. Pruning is opt-in, `--dry-run` reports the change
before it is written, and a project without `.cls` files is refused so a wrong
`--path` cannot empty every profile's class access.
