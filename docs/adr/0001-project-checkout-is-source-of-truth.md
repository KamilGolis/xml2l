# The project checkout is the source of truth, the org is opt-in evidence

Normalization is driven by the metadata files in the project: an entry whose target has no file on disk is a ghost reference and is dropped, and missing entries are backfilled from the master schema built out of the project's own profiles. The Salesforce org is consulted only behind `--use-org-schema`, which filters entries against what the org actually contains.

We chose this over making the org authoritative because normalizing against the org would rewrite profiles to reflect permissions the repository does not track, and because it would make the tool unusable without org credentials. The consequence to remember: a `save` diff against a fresh retrieve can legitimately contain removed entries — the org keeps permissions that a profile no longer mentions, so dropping an entry does not revoke it.
