# Reflect Go 0.4.10 Mac validation

This Mac update was prompted by testing the 0.4.9 public archive on an Apple M1
Mac with 16 GiB RAM. Windows and Linux still use the 0.4.9 downloads.

## First-start issue

The original 60-second runtime probe could expire during the first Metal
shader compilation in a fresh copy. A sampled runtime was waiting on Apple's
Metal compiler, and an uninterrupted cold device probe finished in about 56
seconds. The warmed copy completed its probe in under a second. Choosing CPU
in the original build still initialized the Metal registry before processing
the CPU-only command-line option.

The Mac update allows up to three minutes for a Metal probe and uses the
bundled runtime's CPU-only device enumeration for CPU probes and inference.
The isolated launcher does not inherit host Metal device overrides. On the
same runtime, the CPU-only device probe finished in 0.03 seconds and reported
no Metal device. See the pinned [llama.cpp Metal registry](https://github.com/ggml-org/llama.cpp/blob/7fe450e19305b828c199d602c23a8337aaa1f03b/ggml/src/ggml-metal/ggml-metal.cpp#L931).

## Model and download check

The native 0.4.9 model finder narrowed the nine-entry catalog to Qwen3 4B
Instruct. A fresh 2.33 GiB GGUF download was paused and restarted in the app.
The final file matched the pinned SHA-256. A fresh Mac archive extraction
passed Gatekeeper and package integrity checks, and native Claude Code setup
completed in that isolated copy. The corrected engine loaded the downloaded
model on Metal and returned `42` through the complete local chat stream.

## Signed Mac candidate

- Both CPU and Metal returned `42` through completed local chat streams using
  the downloaded model. The CPU provider was stopped before the Metal test.
- Native window startup, the model finder layout, and normal Quit were checked
  in the 0.4.10 development app. The production app passed Developer ID and
  Gatekeeper checks after its notarization ticket was stapled. Notary submission:
  `3b0eea3e-e048-4050-89cc-6c81284267e7`.
- Go race tests and vet, 25 Python packaging/download tests, and Node catalog
  filtering passed. The installed Codex app-server loaded the local catalog in
  an isolated profile. OpenCode loaded the local provider from a test global
  configuration, and profile restoration tests preserved unrelated edits and
  detected conflicting changes. These are separate from native desktop UI tests.

## Portable client smoke tests

Using Qwen3 4B, each client worked in a separate disposable folder. The check
required changing only a subtraction to addition, running `python3 smoke.py`,
and leaving the script's output receipt before the independent verification.
Both assertions remained unchanged. A separate execution then returned
`LOCAL TOOL OK` with exit code 0.

| Client | Result | Evidence |
| --- | --- | --- |
| Codex CLI 0.160.0 | Passed on the patched test build | Read the file, recovered from a stale shell-process ID call, made the exact edit, and executed the command. |
| Claude Code 2.1.289 | Passed on the signed candidate | Approved the exact shell command in manual mode; edit and command receipt verified. |
| OpenCode 1.18.34 | Passed on the signed candidate | Selected **Allow once**; output and command receipt verified. |

Non-interactive Claude and OpenCode denied unapproved writes. The successful
interactive runs kept those safeguards and approved only the test commands.
This is a small coding smoke test, not a general tool-use benchmark.

The Codex check was also repeated on the final signed candidate: the exact
edit, shell-command receipt, and independent rerun all passed. After the client
checks, normal service Quit removed the provider record and closed its listener.

## Scope

This is a Mac preview. Intel Mac execution, physical Windows/Linux testing,
and actual Gemma/Muse inference on a 48/128 GiB machine remain unverified.
Small-model success does not qualify every model, coding task, or native
desktop workflow. See [0.4.9 validation](https://github.com/odomcl22/reflect-go/blob/v0.4.9-preview/docs/PREVIEW_0_4_9_VALIDATION.md) for the
previous release and [compatibility](HARNESS_COMPATIBILITY.md) for desktop
client requirements.
