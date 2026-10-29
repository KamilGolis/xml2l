# `viewAllFields` is preserved but never backfilled

`objectPermissions/viewAllFields` is decoded, exported, and written back for entries that already carry it, but it is never added to entries that lack it — the only modelled permission boolean with that behaviour.

Salesforce only emits the element on orgs and releases that support it: two projects at `sourceApiVersion` 67.0 disagree, one retrieved with the element and one without. Injecting `<viewAllFields>false</viewAllFields>` everywhere would therefore break deploys to orgs that do not know the element, while backfilling `false` buys nothing that a missing element does not already mean. Encoding presence as a nil pointer keeps the distinction available to the graph, the exporters, and the normalizer.
