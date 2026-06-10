# pdfer XFA support: scope recommendations

**Status:** Design plan for pdfer's XFA surface. P1 is the committed roadmap;
P2 is a draft proposal still under consideration. Open for discussion — file
an issue or open a PR to suggest changes.

**Implementation status:** P1 is complete. #1 closed by commit `88b6989`
(orphan-script extraction), #2 by PR #16 (`fbe109a`, `FormSchema.Elements`),
#3 by the occur/bind metadata extraction (v2.6.0).

---

A review of pdfer's XFA support from the perspective of a downstream consumer
(`xfa-web`) building an interactive renderer on top of the library.

The headline: **the overall design is coherent and defensible. The scope
boundary is in the right place. Most of what follows is about finishing the
contract that the current code is already 80% of the way to, not redrawing
it.**

---

## TL;DR

| Change | Priority | Status | Why |
|---|---|---|---|
| Export all scripts, regardless of whether their owner node is emitted as a Question | **P1** | **Done** (`88b6989`) | The current emission filter drops the script-bearing nodes (`bind="none"` buttons, event-bearing draws, `<pageArea>` events) that renderers most need. This is the single largest fidelity gap. |
| Surface event-bearing and `bind="none"` nodes via a parallel `Elements` collection | **P1** | **Done** (PR #16, `fbe109a`) | Renderers need these nodes addressable in the schema; keeping them out of `Questions` preserves the "Question = thing a user answers" invariant. |
| Parse and expose `<occur>` and `<bind>` metadata on Questions and Sections | **P1** | **Done** (v2.6.0) | Without this, renderers can't tell which subforms are repeatable or what they bind to — i.e. can't implement dynamic XFA at all. |
| Ship a SOM path parser + schema resolver as `forms/xfa/som` | **P2** | Draft | Single correct implementation everyone needs; co-located with the schema it operates on. |
| Add data-DOM cursor API (`GetDataValue` / `SetDataValue` / `ListDataChildren` by SOM path) | **P2** | Draft | Current `UpdateXFAValues` is name-keyed and regex-based; renderers doing real binding need path-keyed access. |
| Capture `<validate><script>` child elements as regular FormScripts | **P2** | Draft | Currently only the `scriptTest` attribute form is captured; the child-element form is not. |

---

## The scope principle

Stated as plainly as possible, here's where the line should sit:

> pdfer's XFA support extracts structure and surfaces logic; it does not
> execute logic. Every `<script>` in the template is exposed verbatim with
> stable identifiers and owner pointers. Every `<occur>` and `<bind>`
> declaration is exposed as data. Every node that an authoring tool produced
> is reachable from the schema. The runtime model — instance counts, presence
> toggles, calculation order, script-driven UI changes — is the caller's
> responsibility.
>
> The schema is a **projection of the template DOM**, not a snapshot of a Form
> DOM. Callers building a renderer must implement merge themselves; callers
> only round-tripping data values can use the dataset APIs and never touch the
> template projection.

If you state it that way, "should pdfer drop bind=none help buttons?" answers
itself: no, because the renderer needs them addressable. And "should pdfer
execute click scripts?" also answers itself: no, because that's the runtime
layer.

The current code is about 80% of the way to that statement being true. The
remaining 20% is mostly removing emission filters that are doing the
renderer's job, plus the metadata additions below.

---

## What's working well

These are load-bearing pieces of the current design and should be preserved as
the contract evolves.

**Two-layer split.** Stream plumbing (`xfa.go`, `stream.go`, the translators)
is cleanly separated from schema translation (`xfa_form_translator.go`).
Callers who only want to patch dataset values don't have to pay for the
template walk. Keep this separation.

**Verbatim script bodies with stable SOM-keyed IDs.** This is the foundation
that makes a downstream runtime possible at all. As long as bodies stay
byte-identical to source and IDs stay stable across regenerations, consumers
can compile, cache, and attach behavior without pdfer caring. The existing
`TestFieldEventBodyPreservedVerbatim` invariant must remain inviolable as new
code lands.

**Flat-with-pointers schema shape.** `FormSchema.Questions` (flat) +
`FormSchema.Sections` (hierarchical tree referencing question IDs) +
`FormSchema.Scripts` (flat with owner pointers) is the right shape for a JS
frontend. It's much friendlier than a deep object graph and scales when scripts
have multiple owners.

**Dataset round-trip via incremental update.** `UpdateXFAValues` touches only
`<data>…</data>` and splices into the original PDF. Exporters want exactly
this — pdfer is not in the business of regenerating template packets and
shouldn't be.

---

## P1 — Required for a credible renderer

### 1. Export all scripts, decoupled from question emission

**Status: Done (commit `88b6989`).** `extractAllScripts` does a pre-pass over
the template tree; `populateScriptBackRefs` fills in `OwnerID` when the owner
is also a Question/Section and leaves orphans with empty `OwnerID`. Tests
assert pageArea, bind=none button, event-bearing draw, and per-option
exclGroup scripts all survive.

The original problem statement and rationale are preserved below for reference.

**Problem.** `attachFieldScripts` only runs inside `walkSubformChildren` for
nodes that survived `emitField` / `emitDraw`. Any script attached to a filtered
node is silently lost. Common XFA patterns silently disappeared from the
schema as a result:
- Help-text popup buttons (`bind="none"` buttons with a `click` script that
  calls `xfa.host.messageBox` or toggles `presence` on a sibling)
- Dynamic show/hide draws (event-bearing draws controlled by scripts)
- Per-option click handlers on radio buttons collapsed into an `<exclGroup>`
- Document-lifecycle events on `<pageArea>`

**Fix (shipped).** Script extraction was split from question emission: a
separate pre-pass over the template visits every `<field>`, `<draw>`,
`<subform>`, `<exclGroup>`, `<pageArea>`, and `<variables>` and pulls their
events into `FormSchema.Scripts`. Question emission then attaches IDs to the
questions that exist. Scripts on filtered nodes appear in the flat list with
`OwnerPath` set; their `OwnerID` is empty until §2 lands.

**Follow-on (§2).** Once `FormSchema.Elements` exists, `OwnerID` will be
populated to point at Element IDs for the orphan cases listed above —
giving every script a typed owner reference.

**Stability contract for orphan signaling.** The set of "orphan" scripts
(`OwnerID == ""`) is *expected to shrink* as `FormSchema.Elements` and
further typed collections land. The semantic of the empty-`OwnerID` signal
is stable — "the owner is not currently a typed entity in this schema" —
but the *enumeration* of orphan cases is not. Consumers that audit orphans
(e.g. "scripts I cannot attach to anything I render") will see their orphan
set get smaller over time, which is the intended direction; consumers that
hard-code today's four orphan cases as a permanent classification will
break. `FormScript`'s doc comment captures this; callers should treat
`OwnerID != ""` as "dereferenceable by ID in some typed collection" without
caring which one.

### 2. Surface event-bearing and `bind="none"` nodes via a parallel `Elements` collection

**Status: Done (PR #16, commit `fbe109a`).** `FormSchema.Elements` surfaces
`bind="none"` buttons, event-bearing draws, and pageAreas as typed
`FormElement` entries; `populateScriptBackRefs` fills `OwnerID` for
element-owned scripts. The original problem statement is preserved below.

**Problem.** `emitField` currently drops `bind="none"` button nodes as "UI
trigger (Help Text, Show Intro, etc.)", with a hardcoded exception for
`AddAttachment`. `emitDraw` drops any draw with `len(node.Events) > 0` as
"dynamic." Renderers need these nodes to exist so they can render the
buttons and attach the click handlers — and so the orphan scripts produced
by §1 have a typed owner to point at.

The current pattern of carving out exceptions case-by-case is not
generalizable. The general rule should be: *if it has visual presence or
events, it's part of the template — surface it.*

**Fix.** Add a parallel collection on `FormSchema` for non-question template
nodes:

```go
type FormSchema struct {
    // ... existing fields
    Elements []FormElement `json:"elements,omitempty"`
}

// FormElement represents a non-question template node with visual presence
// or events — e.g. a bind="none" button, an event-bearing <draw>, or a
// <pageArea>. Renderers that want full template fidelity iterate both
// Questions and Elements; renderers that only render input controls iterate
// Questions and ignore Elements.
type FormElement struct {
    ID         string                 `json:"id"`
    OwnerPath  string                 `json:"owner_path"`           // SOM path
    Role       string                 `json:"role"`                 // "button" | "draw" | "pageArea" | ...
    Label      string                 `json:"label,omitempty"`      // caption / text content
    Properties map[string]interface{} `json:"properties,omitempty"` // position, size, xfa_role, etc.
    Scripts    []string               `json:"scripts,omitempty"`    // FormScript IDs, parallel to Question.Scripts
}
```

`Question` stays semantically "thing a user answers" — renderers that iterate
questions to render input controls don't have to special-case button or draw
entries.

When this lands, `FormScript.OwnerID` will be populated for owners in either
collection. The existing `AddAttachment` special case in `emitField` can be
removed in favor of treating all `bind="none"` buttons uniformly via
`Elements`.

### 3. Parse and expose `<occur>` and `<bind>` metadata

**Status: Done (v2.6.0).** `types.Occur` and `types.Bind` are populated on
`Question`, `FormSection`, and `FormElement` (the last for symmetry — the
doc below predates the Elements collection). Occur values are normalized to
XFA spec defaults at parse time (min=1; max=min; initial=min; `max="-1"`
unbounded) and stay nil when the template has no `<occur>`. Bind `Match`
defaults to `"once"` when the attribute is absent; `Ref` is surfaced
verbatim. The metadata feeds no emission filter — the `bind="none"` leaf
drop stays keyed to the pre-existing internal string field. The original
problem statement is preserved below.

**Problem.** This is the missing piece for any renderer that wants to do
dynamic XFA. Currently:

- `<occur>` is not parsed at all. Every repeating subform appears in the
  schema exactly once. A "list of dependents" template that can grow to N
  rows looks identical to a one-row form.
- `<bind>` is read only to detect `match="none"`. The `ref` attribute and
  other `match` values (`once`, `global`, `dataRef`) are ignored.

**Fix.** Parse both elements and surface the data, without acting on it:

```go
// Occur reflects the template's declared cardinality. The actual number of
// instances in a filled form is determined by the data DOM and is the
// runtime's responsibility to materialize — the schema lists each repeating
// subform exactly once regardless of how many instances the dataset carries.
type Occur struct {
    Min     int
    Max     int // -1 for unbounded
    Initial int
}

type Bind struct {
    Match string // "once" | "global" | "none" | "dataRef" | ...
    Ref   string // SOM path when match="dataRef"
}
```

Add `Occur *Occur` and `Bind *Bind` to both `FormSection` and `Question`.
Renderers use this to decide which subforms get an "add another" button and
which data nodes a question reads/writes to.

pdfer doesn't have to merge, expand instances, or pre-bind. It just hands the
renderer what the template said.

---

## P2 — Substantially improves the contract

These items are draft proposals — the shapes below are starting points, not
committed designs. Worth thinking through before implementation.

### 4. SOM path parser and schema resolver (`forms/xfa/som`)

**Why it belongs here.** The test for "is this scope creep?" is whether the
thing has a single correct implementation that every consumer needs, or
whether it has policy choices different consumers will make differently. SOM
resolution is the first kind — the grammar is specified by Adobe, and walking
dots, resolving `..`, expanding `[n]` indices, distinguishing single-match
from multi-match all have right answers. If pdfer doesn't ship it, every
consumer writes the same code and gets subtle edge cases wrong.

It also pairs naturally with things pdfer already exposes — `OwnerPath` is a
SOM-style path, the dataset round-trip needs to address data nodes by path,
and the bind/occur metadata above presupposes that consumers can resolve
paths. The alternative is making pdfer's path format a de-facto public API
without owning it.

**Scope boundary.** Three things people call "the SOM resolver":

1. **Path parser** — string → structured representation. Belongs in pdfer.
2. **Schema resolver** — parsed path + schema (+ optional context) → matching
   `Question` / `FormSection` / `FormElement` / data node. Belongs in pdfer.
3. **Expression evaluator** — parsing FormCalc or JS source to extract SOM
   literals from script bodies. **Does not belong in pdfer.**

The trap is that #2 shades into #3 if you're not careful. Consumer hands you
a script body, asks "what node does this reference?" — and now you're parsing
JS. Don't. pdfer's resolver takes a path *string* that the caller has already
extracted. Pattern-matching `xfa.resolveNode("X")` calls out of script source
is the caller's job.

**Suggested API shape:**

```go
package som

func Parse(expr string) (Path, error)

func Resolve(schema *types.FormSchema, expr string, ctx *types.Question) ([]Resolved, error)
func ResolveOne(schema *types.FormSchema, expr string, ctx *types.Question) (Resolved, error)
```

Document explicitly: *operates on SOM expression strings, not on script
bodies.*

### 5. Data-DOM cursor API

**Problem.** `UpdateXFAValues` is keyed by flat field name and uses regex to
find/replace inside `<data>`. This is fine for simple flat datasets but
doesn't handle nested data nodes well, can't address repeating instances, and
forces every consumer into the same name-based model.

**Fix.** Add SOM-path-keyed accessors to complement the existing flat-map API:

```go
func GetDataValue(datasetsXML []byte, somPath string) (string, bool, error)
func SetDataValue(datasetsXML []byte, somPath, value string) ([]byte, error)
func ListDataChildren(datasetsXML []byte, somPath string) ([]string, error)
```

These build on the existing `<datasets>` parse. Anything above this level
(which question a given data node corresponds to) is the SOM resolver's job,
not the data API's.

**Open question.** Relationship to the existing name-keyed `UpdateXFAValues`:
either (a) migrate callers to the SOM-keyed API and remove the name-keyed
one, or (b) position the name-keyed API as a documented convenience layer
over the path-keyed core. Decide before shipping — two coexisting APIs with
no documented relationship is the worst outcome.

### 6. Capture `<validate><script>` child elements

**Problem.** `ValidationRules.CustomScript` captures the `scriptTest`
attribute string. But `<validate>` can also contain a `<script>` child with
its own `contentType`. The child-element form appears to be uncaptured.

**Fix.** Treat the child-element form the same as any other event — its body
appears in `FormSchema.Scripts` with `Event: "validate"`. The attribute form
can stay where it is or be normalized into the same shape.

---

## What I would *not* add

To keep the scope boundary sharp, these are things that have come up in
discussion and that pdfer should explicitly leave to consumers:

- **Script execution.** Not FormCalc, not JavaScript, not a sandboxed subset.
  This is the runtime layer. Consumers ship their own interpreter.
- **Merge algorithm.** Walking the template against the data DOM to produce a
  Form DOM with the right occurrence counts. This is the runtime layer.
- **Instance management.** `addInstance` / `removeInstance` / `setInstances`
  semantics on repeating subforms. Runtime layer.
- **Calculate dependency tracking.** Figuring out which scripts to re-run when
  a field changes. Runtime layer.
- **Layout engine.** Computing actual page positions from `x`/`y`/`w`/`h` plus
  layout mode plus content reflow. Renderers do this in CSS or canvas.
- **Script-body parsing.** Extracting SOM literals or method calls from
  FormCalc / JS source. The caller pattern-matches if they need to.

These all share a property: they require either (a) a language interpreter or
(b) a runtime mutable state model that doesn't belong in a parsing library.
The verbatim-script-body invariant gives consumers everything they need to
build these themselves.

---

## Suggested order of work

1. **§2 (`Elements` collection).** Surfaces non-question template nodes and
   lets `FormScript.OwnerID` resolve for the orphan cases §1 created.
2. **§3 (`<occur>` and `<bind>` metadata).** Pure data extraction, low risk,
   immediately unlocks dynamic-XFA renderers.
3. **§4 (SOM resolver).** Largest piece of new code, but bounded scope and
   no dependencies on the prior changes.
4. **§5 (data-DOM cursor API).** Builds on the SOM resolver.
5. **§6 (`<validate><script>` capture).** Small, isolated.

After §2 and §3, downstream renderers have the information they need to do
real dynamic XFA. §1 is already done.
