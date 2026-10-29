# Normalized profiles use alphabetical element ordering at both levels

A saved profile orders its top-level sections and each entry's permission properties alphabetically by element name, interleaving profile-level values (`custom`, `description`, `userLicense`) and sections the normalizer does not model (`loginIpRanges`, `loginFlows`) into the same sequence.

`sf project retrieve` writes profiles in exactly this order, so saving a freshly retrieved profile leaves no formatting diff and any remaining diff is a real permission change. The obvious alternative — preserving the incoming document order — makes saves rewrite unrelated lines, because retrieved order and hand-edited order differ; the other alternative, the schema (XSD) sequence, matches neither the retrieved order nor the source files.

## Excluded entries keep their source order

`profile save --exclude` is the one exception. An entry matched by an exclusion prefix is copied from the source rather than rendered, so its properties stay in the order the file has them; only its indentation is normalized. The flag exists to leave those entries alone, and reordering their properties would rewrite lines the operator asked the tool not to touch. Every other entry in the same profile is still sorted alphabetically.
