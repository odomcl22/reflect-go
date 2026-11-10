# Model selector review, 2026-11-09

Scope: read-only analysis of installed LM Studio **0.4.26+4**, compared with
Reflect Go's model catalog. The installed app was not modified. No LM Studio
code or assets were copied into Reflect Go.

## What Reflect Go currently searches

`Sources/ReflectGo/model-catalog.json` is embedded in the executable and returned
by `/api/catalog`. It is a curated list of pinned Hugging Face GGUF files, with
sizes, licenses, and SHA-256 hashes. It is not a live Hugging Face search.
The original CSS sets 12 px filter text and 34 px dropdowns, matching the reported readability problem.

## Observed in the shipped LM Studio artifact

REA analyzed `Contents/Resources/app` and identified original selector modules
in the shipped renderer source map. The map SHA-256 is
`077932161b88b1820205452e5a08ee34d25513e58d198277f2d53f46f1eb6c2d`.

- `SearchInputBar.tsx` defines query entry and compatibility-type controls, with
  a debounced search request. Evidence:
  `ev_aa415cfb9549edfe4096c4ff956fff9daab3e8955e193ffce99b881b66fb1bc6`.
- `SearchOptionsBar.tsx` defines result counts and sort controls. Evidence:
  `ev_99b19c7bad4a4cd856a834f80c82305f99d1d41d73ca9af93302b209760c80c6`.
- `ModelDownloadOptions.tsx` defines downloadable variants with size,
  quantization, format, download state, tool/vision metadata, fit estimates, and
  a recommended option. Evidence:
  `ev_cefe709bc7e0a0e450b19afdf27b943546573347f6df27994511d556af65a97a`.
- `WillFitLabel.tsx`, read directly from the same source map, distinguishes full
  GPU offload, partial offload, CPU fit, and not fitting. This is more nuanced
  than a guarantee based only on the model file's size.
- The source-map search interfaces and merged results distinguish staff picks
  from Hugging Face results. This is static structure, not a verified network
  session or proof that every defined feature is enabled for every user.

The complete application analysis had partial semantic coverage due to heap,
timeout, and result-size limits. Parent evidence:
`ev_97ae458e901c6b9b699b60bc1240a14bdec9a1ec102824d630561b6c9bc9d675`.
Selected module originals were independently read from the shipped source map.
Native UI access timed out, so no live screenshot or pixel comparison is claimed.

## Reflect Go design decision

Adopt search-first discovery and explicit download information. Keep the default
screen simpler than an advanced model manager:

- Visible search, model developer, and memory-estimate controls.
- Additional use, download status, and size filters under **More filters**.
- Readable 14–15 px controls with 44–46 px input/button heights.
- Cards showing developer, download size, GGUF quantization, estimated memory,
  source/license, and one download action.
- Explicit **Reflect Go catalog** wording. Do not imply live search, MLX
  support, tested tool use, or guaranteed fit.

Live Hugging Face discovery, trusted publisher identification, variant grouping,
and architecture-aware pre-download memory estimates remain future work. They
need provider metadata, validation, and failure handling beyond a visual change.
