# Model Family Engine

[![CI](https://github.com/Neo1Noir/model-family-engine/actions/workflows/ci.yml/badge.svg)](https://github.com/Neo1Noir/model-family-engine/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Compile reviewed product-selection rules into a deterministic Go lookup index. Preserve family identities and document relationships with a companion Python asset converter.

**No model calls, database, or third-party Go dependencies are needed at matching time.** This is the deterministic half of the [PDF Model Rules](https://github.com/Neo1Noir/pdf-model-rules) workflow.

## Technical focus

- **Explicit constraints:** required/optional dimensions, fixed/permuted multi-choice order, separators, repetition limits, dependencies, forbidden combinations, and literal suffixes.
- **Evidence-bound review:** source hashes, physical pages, excerpts, reviewer metadata, and interpretation fingerprints. Changing source-rule facts invalidates the compiled interpretation until reviewed again.
- **Deterministic conflicts:** collisions across identities remain ambiguous; the engine never chooses a longer prefix or a model-generated guess.
- **Bounded enumeration:** 250,000 variants per rule and 1,500,000 index entries; compilation fails without publishing a partial index.
- **Family preservation:** migrate reviewed document coverage while preserving family, role, parent, aliases, and discontinued status.

## Quick start

Go 1.23+ is required. The example is entirely fictional.

```sh
git clone https://github.com/Neo1Noir/model-family-engine.git
cd model-family-engine
go test ./...
go run ./cmd/model-family approve --input examples/candidate.json --reviewer "Demo reviewer" --basis "Checked the synthetic selection table" --out out/reviewed.json
go run ./cmd/model-family compile --input out/reviewed.json --version demo-v1 --out out/index.json
go run ./cmd/model-family match --input out/index.json --model METER100-B-X
go run ./cmd/model-family match --input out/index.json --model METER100-B
```

The first input resolves to `METER100`; the second is `not_found`, because input B requires option X. `METER1000-A` is also rejected: similar prefixes do not imply a relationship. Output files are created exclusively and existing files are not overwritten.

Build a single executable with `go build -o bin/model-family ./cmd/model-family`.

## Use as a Go library

```go
import engine "github.com/Neo1Noir/model-family-engine"

// reviewed is an engine.Rules value from a reviewed JSON artifact.
index, err := engine.Compile(&reviewed, "catalog-v1")
if err != nil {
    return err
}
result := index.Resolve("METER100-B-X")
```

Resolution statuses are `resolved`, `not_found`, `needs_clarification`, and `invalid_request`. The response carries the mapping version and matching method. A registered base identity can resolve as `registered_identity`; that does not mean an unspecified complete configuration has been validated.

## Rule format

See [examples/candidate.json](examples/candidate.json). Each canonical model has explicit family facts. Selection rules contain a source hash, physical pages, excerpts, dimensions, and a `deterministic` interpretation. Patterns reference dimensions using ① through ⑨. All dimensions must appear exactly once in each pattern.

`approve` records a deliberate reviewer decision and computes the source fingerprint. It is a local authoring operation, not authentication of a human identity or proof that the evidence is correct. Never run it automatically on unreviewed model output. Standard-scope rules are supported; custom-order rules are explicitly rejected by this compiler.

After approval, `compile` validates every rule. Any invalid/stale rule rejects the whole compilation. Matching normalizes case, whitespace, and full-width punctuation; it does not strip vendor names, infer brand prefixes, or fall back to another model.

## Family asset conversion

`family_assets.py` uses Python's standard library. Its `migrate_ecm_assets(baseline, manifest, selection_baseline, index_baseline)` function returns five asset dictionaries: family identities, model index, document map, selection rules, and environment-ready selection rules.

The converter separates `standard` and `custom` document scopes, preserves confirmed model identities, requires explicit decisions for existing schemas, and rejects duplicate document identities or unrelated selection sources. It does not read PDFs or infer shared-manual coverage from a prefix. The synthetic fixtures in `test_family_assets.py` are executable input examples.

```sh
python -m unittest -q
```

## Tests and scope

Tests cover option dependencies, missing review/evidence, stale fingerprints, conflicting identities, multi-choice order/repetition, blocked models, family migration, and document-scope isolation. All identifiers and documents are fictional. This project does not include business databases, manufacturer rule catalogs, deployment services, or historical production data.

## License

[MIT](LICENSE). Copyright (c) 2026 Neo1Noir.
