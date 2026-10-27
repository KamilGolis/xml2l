# xml2l

xml2l normalizes Salesforce profiles against the metadata that exists in a project checkout, so that profile diffs show permission changes instead of formatting churn.

## Language

**Profile**:
The Salesforce metadata file that grants permissions to the users assigned to it.
_Avoid_: permission file, security file

**Section**:
A direct child element of `<Profile>`: either a group of permission entries of one kind (`classAccesses`, `fieldPermissions`) or a profile-level value (`custom`, `description`, `userLicense`).
_Avoid_: block, group, category

**Entry**:
One permission element inside a section, identified by its name child (`<field>`, `<apexClass>`, `<object>`).
_Avoid_: item, record, permission row

**Permission property**:
A child element of an entry that carries a single flag (`enabled`, `readable`, `editable`, `visible`, `allowCreate`, `viewAllFields`).
_Avoid_: key, attribute, setting

**Ground truth** _(disk scan)_:
The set of metadata identifiers that exist as files in the project directory.
_Avoid_: repo index, file cache

**Ghost reference**:
An entry in a profile whose target has no file in the project, so normalization drops it.
_Avoid_: orphan, dangling reference, stale entry

**Master schema**:
The union of every entry in every profile of the project, used as the source of entries to backfill.
_Avoid_: global schema, merged profile

**Backfill**:
Adding a master-schema entry that a given profile does not contain, using default permission values.
_Avoid_: fill-in, sync, merge

**Source of truth**:
The project checkout. The Salesforce org is opt-in evidence, consulted only when the operator asks for it.
_Avoid_: canonical org, live org

**Retrieve-faithful order**:
Alphabetical order by element name at both section and permission-property level — the order Salesforce itself writes on retrieve.
_Avoid_: canonical order, sorted output

**Standalone value**:
A profile-level section whose element carries a value rather than entries (`custom`, `description`, `userLicense`).
_Avoid_: scalar, field
