# Reflect Go 0.4.10 Mac preview

This Mac update fixes a first-launch problem found when testing a fresh 0.4.9
download on Apple Silicon. Metal can take more than a minute to prepare shaders
on its first launch from a new folder. Reflect Go now allows more time, and
choosing **CPU** skips Metal initialization entirely.

## Open the app

1. Extract the full Mac ZIP into a new folder. Keep **Reflect Go.app** beside
   **Reflect Go Files**.
2. Open **Reflect Go.app**. In **Models → Add models**, download a small model
   or import a compatible single-file GGUF. IBM Granite 4.0 1B is a basic chat
   option; Qwen3 4B Instruct is a coding candidate.
3. Use **Start local chat**, or open a coding client with a suitable model.
   Codex and OpenCode CLI are included. **Get Claude Code** downloads the
   native client directly from Anthropic.
4. Quit Reflect Go before ejecting the drive. Finish client tasks first and
   disconnect any installed desktop apps you connected.

On an 8 or 16 GB Mac, choose the app ZIP and a small model. The Mac Starter Pack
helper assembles the app with Gemma 4 31B and Muse Glimmer 30B, targeting compatible
computers with 48 GB or more memory. Allow about 60 GB of free storage while
preparing it. The helper reuses the original verified model parts; it requires
internet to assemble the pack. The weights do not need to be downloaded again
if you already have them: import or copy them into the new app's Models folder.

The first Metal launch can take a few minutes. If you prefer CPU, select it in
the service settings before starting local inference. See [validation](PREVIEW_0_4_10_VALIDATION.md)
and [client compatibility](HARNESS_COMPATIBILITY.md). Windows and Linux downloads
remain at 0.4.9 preview.

Complete the Claude Code download while connected before moving the prepared
folder to an offline Mac. Its client stays in the portable folder with Codex
and OpenCode.

Reflect Go's original code is MIT-licensed. See the included LICENSE file.
Third-party clients, runtimes, and models retain their own licenses.
