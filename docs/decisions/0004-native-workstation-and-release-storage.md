# 0004 — Native workstation and one archive location

Problem: Phase 2 proves client integration but requires a provider terminal and separate connector terminals. Model, memory, performance, and error information need a discoverable home. Duplicate backup folders obscure which files users should carry.

Alternatives: a browser dashboard would need another local service and lifecycle; Electron would add a large runtime; a terminal menu would retain the usability barrier. A native AppKit/SwiftUI app is the smallest application package for the current Apple Silicon target, reusing system frameworks. Cross-platform frontends remain a later decision.

Decision: package `Reflect Go.app` directly in Phase 3 beside the native launcher. SwiftUI presents the interface in an AppKit window. All inference still runs through the existing checksum-aware launcher. A JSON inventory command exposes its authoritative model screening. The UI owns its child process, polls authenticated loopback metrics without proxies or redirects, and shuts down that child when quitting. Client launchers open in Terminal with quoted project paths and no credentials in generated scripts. No background service is installed.

The Phase 3 default port is 8766. Model files are shared with sibling phases. The phase root is derived from the app bundle's actual location. The app is ad-hoc signed for structural verification, not Developer ID signed or notarized. Gatekeeper and volume execution policy still apply.

Release policy: the project's top-level `releases/` directory is the only backup archive location. Archives include source, pinned artifact metadata, and assembled phase software; exclude models, state, credentials, caches, and AppleDouble metadata. `dist/Reflect Go` and the USB retain runnable Phase 1, Phase 2, Phase 3, and sibling Models. The unique USB migration snapshot is moved to the main archive location before removing duplicate release directories.

Tradeoffs: native UI code currently targets macOS; Codex and Claude keep their terminal interfaces; force-killing the app can leave a child engine alive, while ordinary stop/quit is managed. Memory fit remains a conservative estimate and resident memory is not a full GPU accounting metric. Existing small-model client limitations are unchanged.

Sources checked 2026-10-04:

- Apple, NSHostingView: https://developer.apple.com/documentation/swiftui/nshostingview
- Anthropic, supplied-key approval: https://code.claude.com/docs/en/authentication
- Anthropic, custom provider configuration: https://code.claude.com/docs/en/llm-gateway
- Pinned llama.cpp b11146 server README and metrics output retained locally during Phase 2.
