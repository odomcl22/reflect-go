# Reflect Go 0.4.9 preview

Bring your models and preferred coding client on an external drive. Reflect Go
provides the interface, llama.cpp runtime, built-in chat, and local connections.

## Two editions

- **Reflect Go:** application, runtimes, and portable Codex and OpenCode clients.
  Add your own compatible single-file GGUF or choose a model in **Models → Add
  models**. **Get Claude Code** downloads the native client directly from
  Anthropic into your portable folder; it does not require Python or Node.
- **Reflect Go Starter Pack:** the same software with **Gemma 4 31B Instruct
  QAT Q4_0** and **Muse Glimmer 30B Q4_K_M** model files. No model selection or
  in-app model download is needed after the Starter Pack download completes.

The Starter Pack transfers about **34.4 GB of model weights**, plus the
application. GitHub limits individual assets to under 2 GiB, so the pack is
distributed in verified parts. Its download helper fetches the application and
all parts, verifies their SHA-256 checksums, and assembles the final folder.
Parts are deleted after each complete model verifies. Allow about **60 GB free
space** during preparation; the finished folder takes roughly 35 GB. Use exFAT,
APFS, NTFS, or another filesystem supporting files larger than 4 GB, not FAT32.

On Mac run the `.command` helper, on Windows run the `.cmd` helper beside its
`.ps1` file, and on Linux run `sh` with the `.sh` helper. Internet is needed for
this initial download. The final folder can then move to your external drive.
The helpers use the operating system's download/archive tools; Linux needs
`curl`, `unzip`, and `sha256sum` or `shasum` available.

## Open and work

1. Open **Reflect Go.app** on Mac, **Reflect Go.exe** on Windows, or
   **Reflect Go.desktop** (or `sh "Reflect Go Files/Reflect Go.sh"`) on Linux. Keep the launcher beside **Reflect Go Files**, which holds the runtime, clients, models, and settings.
2. Open your client. Reflect Go starts the local service and supplies your saved
   model choices. Selecting a project folder or default model is optional.
3. Finish your tasks, close portable clients, quit Reflect Go, and eject the drive.

The large Starter Pack models target **48 GiB or more unified memory** on a Mac.
They load one at a time. Windows/Linux suitability also depends on CPU/GPU
memory and the selected backend. Hardware estimates are screening guidance,
not a guarantee. For a smaller computer, choose Reflect Go and download a small
model such as IBM Granite 4.0 1B for a basic chat test. **Start local chat** adapts an automatic model selection to a smaller context when needed. Granite uses CPU automatically on Mac in this preview; its Metal path produced invalid output in testing.

## Model and client choices

Use **Models → Add models** for individual verified catalog downloads, or
**Import an existing GGUF** for your own compatible model. The downloaded folder has just the launcher and **Reflect Go Files**; there is no source tree to navigate. Downloads can pause
and resume, and existing model files are never overwritten. Model source and
license links are shown in the catalog. Gemma and Muse downloads in this pack
contain text-model weights; image encoders and speculative drafters are separate.

Gemma and Muse have metadata-based memory profiles for CLI and desktop contexts. These profiles pass the 48 GiB Mac screening ceiling; they are estimates, not a physical-device test. Inference quality and desktop tool use on larger hosts remain unqualified.

Installed Claude, Codex/ChatGPT, and OpenCode desktop apps use preview adapters.
They are not included as portable desktop applications. A connection is not a
guarantee that every model supports the app's context, tools, and coding tasks.
Disconnecting restores settings changed by Reflect Go; it does not require
signing out. See [compatibility](HARNESS_COMPATIBILITY.md) and
[validation](PREVIEW_0_4_9_VALIDATION.md).

Hermes is not included in 0.4.9. Its CLI and desktop portability remain separate
research. The experimental custom Reflect Go coding harness is excluded.

## Privacy and local execution

Reflect Go has no developer analytics or advertising. Local inference runs on
the connected computer using the weights on your drive. Optional downloads
contact their publishers; client integrations and enabled network tools have
their own behavior and terms. Clients can read and change folders you authorize,
and the OS or installed desktop apps may retain files on the host computer.

Earlier releases remain available in GitHub's release history.
