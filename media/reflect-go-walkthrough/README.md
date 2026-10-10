# Reflect Go interface walkthrough

A 24-second, silent, square video for sharing the Reflect Go 4.7 preview. This is an interface walkthrough made from real screenshots of the application, not an end-to-end client session or performance benchmark.

The three screenshots in `public/` were captured from the current UI with a clean demonstration profile. They show the Apps screen, model selection, and optional project field. No fabricated assistant output appears in the video.

## Edit and export

```sh
npm ci
npm run dev
npm run lint
npm run export
```

Install FFmpeg and make `ffmpeg` available on your PATH for the export command. The script renders six settled scene frames plus the poster in Remotion, then holds and fades those frames in FFmpeg for the social cut. This avoids repeating hundreds of browser captures of unchanged screenshots.

The final output is 1080 × 1080, 30 fps, 24 seconds. It contains captions but no audio, making it readable without sound in a social feed. The separate poster is also 1080 × 1080.

## Scope of the demo

The video shows opening Reflect Go, choosing which models to offer to apps, optionally choosing a project, and the available client launch controls. It does not claim universal computer compatibility, public redistribution of every client, zero initial downloads, isolation from host files, or network distribution.

Shared model defaults and the portable Workspace make the model and project steps optional. Local model files still have to be acquired, and the computer must meet the runtime and memory requirements. Reflect Go remains an early preview.

Export uses software GL and one render worker to avoid host GPU capture artifacts. On macOS it uses installed Google Chrome when available. Set `REFLECT_GO_BROWSER_EXECUTABLE` to choose another compatible browser executable, or `REFLECT_GO_FFMPEG` to choose an FFmpeg executable. On other systems, Remotion uses its default browser. The Studio timeline remains editable; the sharing export uses static scene holds with gentle fades.
