# Reflect Go — Reflect on the Go

Your portable inference engine, models and coding clients travel on this drive. The computer supplies RAM, compute, its browser/terminal and project development tools.

## Start with Phase 4.5

1. Open **Phase 4.5**.
2. **Windows:** double-click **START Reflect Go.exe**. **Mac:** open **Reflect Go.app**. **Linux:** run `sh "Reflect Go.sh"` in that folder.
3. Use the RAM table below to choose a model. For coding clients, use the coding trial settings below (at least **8,192 tokens**). Select **Automatic** compute, start the engine and wait until it is ready.
4. Choose a separate project folder, then open **Codex**, **Claude**, or **OpenCode**. These are portable CLI copies already on this personal drive. Use one at a time.

On Windows, opening the same executable again reopens the existing workstation. Closing its browser tab leaves it running; use **Quit workstation** when finished. On Mac, stop the engine and quit the app. Quit the client before ejecting the drive.

For a lighter inference test, choose the 1.5B model and 2,048 context, then **Try a quick response**. This setting is too small for the coding clients.

If the native Mac app's drive check keeps timing out, quit it and open **Phase 4.5 → Browser Workstation.command**. That fallback passed model discovery and session checks from this stick. Keep its terminal open while using the browser.

See [Phase 4.5 instructions](Phase%204.5/README.md) and [validation and limitations](Phase%204.5/VALIDATION.md). Mac Apple Silicon is tested. Intel Mac, Windows, Linux and conditional ChromeOS support still need physical device qualification. Windows ARM64 search needs an existing Microsoft Visual C++ runtime.

## Compact model selection for this USB

The personal USB library now contains three compact additions. They share the same **Models** folder; the original 1.5B and Qwen3 4B models remain available for the preserved phases. The new weights are downloaded directly to the USB, not duplicated on the development Mac or in release archives.

| Computer RAM | Starting model | File size | Starting chat context |
| --- | --- | --- | --- |
| 8 GB | Bonsai 8B Q1_0 | 1.16 GB | 2,048 tokens |
| 12 GB | Qwen3.5 4B IQ4_XS; Bonsai for a lighter option | 2.48 GB | 2,048 tokens |
| 16 GB | Qwen3.5 4B IQ4_XS | 2.48 GB | 4,096 tokens |
| 24 GB | Qwen3.5 9B IQ4_XS | 5.17 GB | 4,096 tokens |

The three additions total **8.80 GB**. Gemma, Qwen 27B and GPT-OSS are omitted to keep this drive practical for these computers. IQ4_XS is the compact 4-bit variant; Bonsai uses its publisher's native Q1_0 1-bit file. File sizes are decimal GB.

Use **Phase 4.5** for this library. All three models passed local API generation tests on the 16 GB M1 Mac: Bonsai and Qwen3.5 4B at 8,192 context, and Qwen3.5 9B at 1,024 context. The 9B test establishes basic runtime compatibility; its 24 GB profile still needs a physical device test. These are starting settings based on Reflect Go's conservative memory screen, not physical tests on all four computers. The screen budgets half of installed RAM and includes working memory and context; having space on the USB is separate from fitting in RAM. Use Automatic compute and lower the context or select Bonsai if a model is ineligible. On the 12 GB PC, the CPU path is available if its graphics hardware is insufficient.

Coding clients require at least **8,192 tokens**: start with Bonsai on the 12 GB PC, Qwen3.5 4B on 16 GB, and Qwen3.5 9B on 24 GB. On the 8 GB Mac, use Bonsai for chat at 2,048 context: the current screen estimates it at 4.09 GiB for 8,192 context, just over that computer's 4 GiB model budget. Qwen3.5 4B at 8,192 is close to the 16 GB screening limit. These are coding trial settings, not a guarantee of reliable tool use. The original Qwen3 4B remains the previously tested coding baseline.

Use the workstation and its Codex/Claude/OpenCode connections for Qwen3.5. The older standalone terminal chat command is not yet compatible with Qwen3.5 4B; the workstation's local API test passed. Finish downloads before loading a model. This stick took about five minutes to load the 9B model; the current Phase 4.5 workstation now allows ten minutes for startup. Simultaneous downloads can make loading slower.

Model sources: [Bonsai 8B](https://huggingface.co/prism-ml/Bonsai-8B-gguf), [Qwen3.5 4B](https://huggingface.co/unsloth/Qwen3.5-4B-GGUF), [Qwen3.5 9B](https://huggingface.co/unsloth/Qwen3.5-9B-GGUF). All are text GGUF files here; multimodal projectors are not included.

## Preserved phases

| Folder | Purpose |
| --- | --- |
| Phase 1 | Original local chat demonstration |
| Phase 2 | Local API and host-installed CLI connections |
| Phase 3 | Original Mac workstation |
| Phase 3.1 | Hardened Mac workstation |
| Phase 4 | Adaptive Windows, Mac and Linux runtime |
| Phase 4.5 | Portable CLI packs and simpler Windows startup |
| Models | One shared copy of each model for all phases |

Keep the phase folders beside **Models**. Each phase keeps its own settings and sessions in **state**; treat these as private. Do not share a working state folder.

Backup archives are stored only in the development project's top-level **releases** folder on the original Mac. There are no backup archives nested in this collection or on the stick. The backup excludes model weights, client state and proprietary Claude binaries; this personal runnable drive includes all three clients.
