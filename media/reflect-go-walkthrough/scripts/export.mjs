import {spawnSync} from 'node:child_process';
import {existsSync, mkdirSync} from 'node:fs';
import {dirname, join, resolve} from 'node:path';
import {fileURLToPath} from 'node:url';

const project = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const output = resolve(project, '../../docs/media');
const frames = join(project, 'out', 'final-scenes');
const remotion = join(project, 'node_modules', '@remotion', 'cli', 'remotion-cli.js');
const macChrome = '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome';
const browser = process.env.REFLECT_GO_BROWSER_EXECUTABLE || (existsSync(macChrome) ? macChrome : null);
const browserArgs = browser ? ['--browser-executable', browser] : [];
const ffmpeg = process.env.REFLECT_GO_FFMPEG || 'ffmpeg';

function run(command, args) {
  const result = spawnSync(command, args, {cwd: project, stdio: 'inherit'});
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`${command} failed (${result.signal || result.status})`);
}
mkdirSync(output, {recursive: true});
mkdirSync(frames, {recursive: true});
// Render the exact six settled composition frames, avoiding hundreds of
// redundant browser captures for the held screenshots in this social cut.
run(process.execPath, [remotion, 'render', 'ReflectGoWalkthrough', frames,
  '--frames=45,135,285,405,525,645', '--image-format=png', '--gl=swangle', '--concurrency=1', ...browserArgs]);
run(process.execPath, [remotion, 'still', 'ReflectGoPoster', join(output, 'reflect-go-walkthrough.jpg'),
  '--image-format=jpeg', '--gl=swangle', ...browserArgs]);

const scenes = [
  ['element-045.png', 3], ['element-135.png', 5], ['element-285.png', 4],
  ['element-405.png', 4], ['element-525.png', 4], ['element-645.png', 4],
];
const inputs = scenes.flatMap(([file, duration]) => ['-loop', '1', '-framerate', '30', '-t', String(duration), '-i', join(frames, file)]);
const filters = scenes.map(([,duration], i) =>
  `[${i}:v]fps=30,trim=duration=${duration},setpts=PTS-STARTPTS,format=yuv420p,fade=t=in:st=0:d=0.25[v${i}]`);
filters.push('[v0][v1][v2][v3][v4][v5]concat=n=6:v=1:a=0[out]');
run(ffmpeg, ['-hide_banner', '-loglevel', 'warning', '-y', ...inputs,
  '-filter_complex', filters.join(';'), '-map', '[out]', '-an', '-c:v', 'libx264',
  '-preset', 'medium', '-crf', '18', '-pix_fmt', 'yuv420p', '-movflags', '+faststart',
  '-r', '30', '-fps_mode', 'cfr', '-t', '24', join(output, 'reflect-go-walkthrough.mp4')]);
console.log('Exported 24-second 1080×1080 walkthrough and poster to docs/media.');
