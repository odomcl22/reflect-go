# Reflect Go

**Your weights. Your way.** Carry your AI models and the software to run them on
an external drive. Open Reflect Go on a compatible computer, choose your app,
and work with your local model.

**[Download Reflect Go](https://github.com/odomcl22/reflect-go/releases/tag/v0.4.8-preview)** ·
[Watch the walkthrough](docs/media/reflect-go-walkthrough.mp4) ·
[App compatibility](docs/HARNESS_COMPATIBILITY.md)

Reflect Go includes a local inference engine, preconfigured portable coding
clients, a model library, and built-in chat. Your drive holds the model weights;
the computer provides the processing power and memory. Once your models and
software are downloaded, local inference works offline without a cloud AI
subscription or a separate engine installation.

[![Watch the Reflect Go interface walkthrough](docs/media/reflect-go-walkthrough.jpg)](docs/media/reflect-go-walkthrough.mp4)

*A 24-second tour of the Reflect Go interface.*

## Download

Choose the package for your computer. Both editions are ready to run after
extracting the ZIP—no build tools required.

- **Reflect Go** includes the application, inference runtimes, and portable Codex
  and OpenCode CLI clients. Add your own model or download one in the app.
- **Reflect Go Starter Pack** includes everything above plus Qwen2.5 1.5B Instruct
  Q4_K_M, a small model for trying built-in chat and checking your setup.

| Platform | Reflect Go | Reflect Go Starter Pack |
| --- | --- | --- |
| Mac · Apple Silicon + Intel | [Download ZIP](https://github.com/odomcl22/reflect-go/releases/download/v0.4.8-preview/ReflectGo-0.4.8-macos.zip) | [Download ZIP](https://github.com/odomcl22/reflect-go/releases/download/v0.4.8-preview/ReflectGo-StarterPack-0.4.8-macos.zip) |
| Windows · x64 + ARM64 | [Download ZIP](https://github.com/odomcl22/reflect-go/releases/download/v0.4.8-preview/ReflectGo-0.4.8-windows.zip) | [Download ZIP](https://github.com/odomcl22/reflect-go/releases/download/v0.4.8-preview/ReflectGo-StarterPack-0.4.8-windows.zip) |
| Linux · x64 + ARM64 | [Download ZIP](https://github.com/odomcl22/reflect-go/releases/download/v0.4.8-preview/ReflectGo-0.4.8-linux.zip) | [Download ZIP](https://github.com/odomcl22/reflect-go/releases/download/v0.4.8-preview/ReflectGo-StarterPack-0.4.8-linux.zip) |

**Current version: 0.4.8 preview.** Mac downloads are signed and Apple-notarized.
Windows executables are unsigned and may show a SmartScreen warning. Testing on
physical Windows and Linux computers is still in progress. See
[release notes and checksums](https://github.com/odomcl22/reflect-go/releases/tag/v0.4.8-preview)
and [testing status](docs/PREVIEW_0_4_8_VALIDATION.md).

**Desktop connection issue in the 0.4.8 downloads:** the local service can report
“Connection unavailable” when opening a desktop app. A fix is in the source;
updated download packages are awaiting desktop testing. For now, use built-in
chat or a portable CLI. [Follow the desktop test results](docs/PREVIEW_0_4_8_VALIDATION.md#follow-up-desktop-testing).

## Get started

1. **Extract the entire ZIP** to your external drive or a folder on your computer.
   Keep the launcher and its support folders together.
2. **Open Reflect Go:**
   - **Mac:** open `Reflect Go.app` (macOS 13.3 or later).
   - **Windows:** open `START Reflect Go.exe`.
   - **Linux:** run `sh "Reflect Go.sh"` from the extracted folder.
3. **Add a model** in **Model library**, or try the Starter Pack's included model
   in **Chat**.
4. **Open your coding client.** Reflect Go starts the local service automatically
   and supplies your saved model choices. Choosing a project folder or changing
   which models are available is optional.

For coding and desktop apps, choose a compatible model with enough context and
memory for the task. The Starter Pack's small model is intended for simple chat;
it does not meet the desktop connections' context requirements.
[Read the setup guide](docs/PREVIEW_0_4_8.md).

## Choose your app

Use **Codex CLI** or **OpenCode CLI** directly from the included portable clients.
Reflect Go configures their local connection and keeps their profiles on the drive.
A **Claude Code CLI** connector is also available; the Claude CLI software is a
separate download and is not included in these packages.

Already use a desktop app? Preview connections are available for **Claude
Desktop**, **Codex mode in ChatGPT Desktop**, and **OpenCode Desktop**. These
require the desktop app to be installed on your computer. Each app has its own
model and platform requirements, and full desktop coding workflows are still
being tested. [Check compatibility](docs/HARNESS_COMPATIBILITY.md) before connecting.

You can also use Reflect Go's built-in **Chat** or launch **OpenCode Web**.
Additional connections are under **More apps**.

## Bring your models

Download a model from **Model library** or import a compatible GGUF file. Reflect
Go uses llama.cpp and recommends models and settings based on your computer.
Model architecture, available memory, and context size determine what can run.

Choose which models to offer each supported client and set a default. Reflect Go
remembers those choices and loads models as they are requested. Offering several
models does not mean they all need to be loaded into memory at once.

Appearance and runtime controls are in **Settings**, including **Dark**, **Light**,
and **System** themes.

## Finish and take it with you

Finish your work and exit portable CLI sessions, then **Quit Reflect Go** before
ejecting the drive. For connected desktop apps, **Disconnect** or **Quit** restores
the settings Reflect Go changed and restarts the apps with their previous
configuration. You do not need to sign out of your accounts.

Clients can access the project folders you authorize. Installed desktop apps and
the operating system may keep their own files on the computer. If a connection
cannot be restored automatically, Reflect Go reports the issue;
[the setup guide](docs/PREVIEW_0_4_8.md#disconnect-without-signing-out) explains recovery.

## About the project

Reflect Go is in active development. Explore the [roadmap](docs/ROADMAP.md),
[compatibility details](docs/HARNESS_COMPATIBILITY.md), and
[validation report](docs/PREVIEW_0_4_8_VALIDATION.md), or
[report an issue](https://github.com/odomcl22/reflect-go/issues).

Developers can find the application code in [Sources](Sources), build and
packaging tools in [scripts](scripts), and design notes, earlier phase plans, and
release documentation in [docs](docs). Previous source versions are preserved in
the repository's commit history.
