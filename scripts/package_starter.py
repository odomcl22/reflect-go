#!/usr/bin/env python3
"""Prepare the preloaded Starter Pack as shared GitHub-sized model parts.

The download scripts fetch a platform's app ZIP and the actual model parts,
verify every file, and assemble a ready-to-run folder. Model choice and download
inside Reflect Go are not required. No uploads occur here.
"""
import argparse
import hashlib
import json
import re
import shlex
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
VERSION = '0.4.9'
MODEL_IDS = ('gemma4-31b-qat-q4-0', 'muse-glimmer-30b-q4-k-m')
PART_SIZE = 1900 * 1024 * 1024  # under GitHub's 2 GiB per-asset limit
BASE = f'https://github.com/odomcl22/reflect-go/releases/download/v{VERSION}-preview/'


def digest(path):
    with path.open('rb') as f:
        return hashlib.file_digest(f, 'sha256').hexdigest()


def plan(models, part_size=PART_SIZE, catalog=None):
    if not 0 < part_size < 2**31:
        raise ValueError('Each part must be under 2 GiB.')
    catalog = catalog if catalog is not None else json.loads((ROOT/'Sources/ReflectGo/model-catalog.json').read_text())
    entries = {m['id']: m for m in catalog}
    result = {'version': VERSION, 'models': []}
    for id in MODEL_IDS:
        m = entries[id]
        if not re.fullmatch(r'[A-Za-z0-9_.-]+', m['filename']):
            raise ValueError('Unsafe model filename.')
        source = models/m['filename']
        if source.is_symlink() or not source.is_file() or source.stat().st_size != m['size']:
            raise ValueError('Model size or path mismatch: '+m['filename'])
        parts, whole, offset = [], hashlib.sha256(), 0
        with source.open('rb') as f:
            while offset < m['size']:
                size, checksum = min(part_size, m['size']-offset), hashlib.sha256()
                remaining = size
                while remaining:
                    block = f.read(min(8 << 20, remaining))
                    if not block:
                        raise ValueError('Model changed during preparation.')
                    whole.update(block); checksum.update(block); remaining -= len(block)
                parts.append({'name': f'StarterModels-{VERSION}-{id}.part{len(parts)+1:02}',
                              'offset': offset, 'size': size, 'sha256': checksum.hexdigest()})
                offset += size
        if whole.hexdigest() != m['sha256']:
            raise ValueError('Model checksum mismatch: '+m['filename'])
        result['models'].append({k: m[k] for k in ('id','name','filename','size','sha256','license','source')} | {'parts': parts})
    return result


def stage_part(models, specification, name, output):
    if output.exists():
        raise ValueError('Part output already exists.')
    model, part = next((m, p) for m in specification['models'] for p in m['parts'] if p['name'] == name)
    checksum = hashlib.sha256()
    try:
        with (models/model['filename']).open('rb') as source, output.open('xb') as target:
            source.seek(part['offset']); remaining = part['size']
            while remaining:
                block = source.read(min(8 << 20, remaining))
                if not block:
                    raise ValueError('Incomplete source model.')
                target.write(block); checksum.update(block); remaining -= len(block)
        if checksum.hexdigest() != part['sha256']:
            raise ValueError('Part checksum mismatch.')
    except BaseException:
        output.unlink(missing_ok=True)
        raise


def posix_script(specification, software, platform):
    q = shlex.quote
    required=sum(m['size'] for m in specification['models'])+max(m['size'] for m in specification['models'])+software.get('size',0)+software.get('unpacked_size',0)+(1<<30)
    lines = ['#!/bin/sh', 'set -euC', 'umask 077',
             'SOURCE_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)', 'cd -- "$SOURCE_DIR"',
             f'mkdir -p "Reflect Go Starter Pack {VERSION}"', f'cd "Reflect Go Starter Pack {VERSION}"',
             'mkdir .starter-work.lock 2>/dev/null || { echo "A Starter Pack download is already active. If its window was closed unexpectedly, remove the empty .starter-work.lock folder before retrying." >&2; exit 1; }',
             'trap \'rmdir .starter-work.lock 2>/dev/null || true\' 0', 'trap \'exit 1\' 1 2 15',
             'credited=0',
             'credit() { if test -f "$1" && ! test -L "$1"; then size=$(wc -c < "$1"); if test "$size" -gt "$2"; then size=$2; fi; credited=$((credited+size)); fi; }']
    candidates=[(software['name'],software.get('size',0))]
    for model in specification['models']:
        target='Reflect Go/Reflect Go Files/Models/'+model['filename']
        candidates += [(target,model['size']),(target+'.assembling',model['size'])]
        candidates += [(part['name'],part['size']) for part in model['parts']]
    for name,size in candidates:
        lines += [f'credit {q(name)} {size}']
        if not name.endswith('.assembling'):lines += [f'credit {q(name+".partial")} {size}']
    lines += ['available=$(df -Pk . | awk \'NR==2 {print $4}\')',
             'case "$available" in ""|*[!0-9]*) echo "Could not check available storage." >&2; exit 1;; esac',
             f'if test $((available*1024+credited)) -lt {required}; then echo "Not enough free storage for the Starter Pack. Allow about 60 GB during preparation, including any existing pack downloads." >&2; exit 1; fi',
             'checksum() { if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1"; else sha256sum "$1"; fi; }',
             'verify() { test -f "$1" && test "$(checksum "$1" | cut -d " " -f 1)" = "$2"; }',
             'fetch() {',
             '  if verify "$1" "$2"; then return; fi',
             '  if test -e "$1"; then echo "Checksum mismatch: $1. Inspect or remove that file before retrying." >&2; exit 1; fi',
             '  curl --proto "=https" --tlsv1.2 --fail --location --retry 3 --continue-at - --output "$1.partial" "$3"',
             '  verify "$1.partial" "$2" || { echo "Download verification failed: $1" >&2; exit 1; }',
             '  mv "$1.partial" "$1"', '}',
             'echo "Downloading Reflect Go Starter Pack: Gemma + Muse. Allow about 60 GB of free space."',
             f'fetch {q(software["name"])} {q(software["sha256"])} {q(BASE+software["name"])}',
             'if ! test -d "Reflect Go"; then',
             f'  unzip -q {q(software["name"])} -d app-extraction',
             '  mv "app-extraction/Reflect Go" "Reflect Go"',
             '  rmdir app-extraction', 'fi', 'mkdir -p "Reflect Go/Reflect Go Files/Models"',
             'mkdir -p "Reflect Go/Reflect Go Files/licenses/models"',
             'for notice in MODEL-NOTICES.txt MODEL-LICENSE-APACHE-2.0.txt; do',
             '  target="Reflect Go/Reflect Go Files/licenses/models/$notice"',
             '  if test -e "$target"; then cmp "$SOURCE_DIR/$notice" "$target" || { echo "Existing model notice differs." >&2; exit 1; }; else cp "$SOURCE_DIR/$notice" "$target"; fi',
             'done']
    for m in specification['models']:
        target = 'Reflect Go/Reflect Go Files/Models/'+m['filename']
        lines += [f'if ! verify {q(target)} {q(m["sha256"])}; then',
                  f'  if test -e {q(target)}; then echo "Existing model checksum mismatch; no files were overwritten." >&2; exit 1; fi']
        for p in m['parts']:
            lines += [f'  fetch {q(p["name"])} {q(p["sha256"])} {q(BASE+p["name"])}']
        parts = ' '.join(q(p['name']) for p in m['parts'])
        lines += [f'  cat {parts} > {q(target+".assembling")}',
                  f'  verify {q(target+".assembling")} {q(m["sha256"])} || {{ echo "Model assembly verification failed." >&2; exit 1; }}',
                  f'  mv {q(target+".assembling")} {q(target)}', f'  rm -- {parts}', 'fi']
    lines += ['echo "Ready. Open Reflect Go inside this Starter Pack. Models are already in Models/."']
    if platform == 'macos':
        lines += ['echo "Press Return to close."', 'read answer']
    return '\n'.join(lines)+'\n'


def powershell_script(specification, software):
    # JSON is generated from pinned catalog metadata; no commands from a remote
    # manifest are evaluated. PowerShell 5.1 is sufficient.
    required=sum(m['size'] for m in specification['models'])+max(m['size'] for m in specification['models'])+software.get('size',0)+software.get('unpacked_size',0)+(1<<30)
    data = json.dumps({'software': software, 'models': specification['models'], 'required_space': required}, separators=(',', ':'))
    return '''$ErrorActionPreference = 'Stop'
$lockHandle = $null
try {
  Set-Location -LiteralPath $PSScriptRoot
  $folder = 'Reflect Go Starter Pack VERSION'
  New-Item -ItemType Directory -Force -Path $folder | Out-Null
  Set-Location -LiteralPath $folder
  try { $lockHandle = [IO.File]::Open((Join-Path (Get-Location) '.starter-work.lock'), [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None) }
  catch { throw 'Another Starter Pack download is active in this folder. Finish or close it first.' }
  $spec = @'
DATA
'@ | ConvertFrom-Json
  $owned = @(@{name=$spec.software.name;size=$spec.software.size})
  foreach ($model in $spec.models) {
    $path = Join-Path 'Reflect Go/Reflect Go Files/Models' $model.filename
    $owned += @{name=$path;size=$model.size}; $owned += @{name=($path+'.assembling');size=$model.size}
    foreach ($part in $model.parts) { $owned += @{name=$part.name;size=$part.size} }
  }
  [long]$credited = 0
  foreach ($item in $owned) {
    foreach ($path in @($item.name, ($item.name+'.partial'))) {
      if (Test-Path -LiteralPath $path -PathType Leaf) { $credited += [Math]::Min((Get-Item -LiteralPath $path).Length, [long]$item.size) }
    }
  }
  $drive = [IO.DriveInfo]::new([IO.Path]::GetPathRoot((Get-Location).Path))
  if ($drive.DriveFormat -in @('FAT','FAT32')) { throw 'The large model files need exFAT, NTFS, or another filesystem supporting files over 4 GB.' }
  if ($drive.AvailableFreeSpace + $credited -lt $spec.required_space) { throw 'Not enough free storage. Allow about 60 GB during preparation, including existing pack downloads.' }
  function Verify($path, $sha) { return (Test-Path -LiteralPath $path -PathType Leaf) -and ((Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant() -eq $sha) }
  function Fetch($name, $sha) {
    if (Verify $name $sha) { return }
    if (Test-Path -LiteralPath $name) { throw "Checksum mismatch: $name. Inspect or remove that file before retrying." }
    Invoke-WebRequest -UseBasicParsing -Uri ('BASE' + $name) -OutFile ($name + '.partial')
    if (-not (Verify ($name + '.partial') $sha)) { throw "Download verification failed: $name" }
    Move-Item -LiteralPath ($name + '.partial') -Destination $name
  }
  Write-Host 'Downloading Reflect Go Starter Pack: Gemma + Muse. Allow about 60 GB free space on an exFAT or NTFS drive.'
  Fetch $spec.software.name $spec.software.sha256
  if (-not (Test-Path -LiteralPath 'Reflect Go')) {
    Expand-Archive -LiteralPath $spec.software.name -DestinationPath 'app-extraction'
    Move-Item -LiteralPath 'app-extraction/Reflect Go' -Destination 'Reflect Go'
    Remove-Item -LiteralPath 'app-extraction'
  }
  New-Item -ItemType Directory -Force -Path 'Reflect Go/Reflect Go Files/licenses/models' | Out-Null
  foreach ($notice in @('MODEL-NOTICES.txt','MODEL-LICENSE-APACHE-2.0.txt')) {
    $source = Join-Path $PSScriptRoot $notice
    $target = Join-Path 'Reflect Go/Reflect Go Files/licenses/models' $notice
    if (Test-Path -LiteralPath $target) {
      if (-not (Verify $target ((Get-FileHash -LiteralPath $source -Algorithm SHA256).Hash.ToLowerInvariant()))) { throw 'Existing model notice differs.' }
    } else { Copy-Item -LiteralPath $source -Destination $target }
  }
  foreach ($model in $spec.models) {
    $target = Join-Path 'Reflect Go/Reflect Go Files/Models' $model.filename
    if (Verify $target $model.sha256) { continue }
    if (Test-Path -LiteralPath $target) { throw 'Existing model checksum mismatch; no files were overwritten.' }
    foreach ($part in $model.parts) { Fetch $part.name $part.sha256 }
    $output = [IO.File]::Open((Join-Path (Get-Location) ($target + '.assembling')), [IO.FileMode]::CreateNew, [IO.FileAccess]::Write)
    try {
      foreach ($part in $model.parts) {
        $input = [IO.File]::OpenRead((Join-Path (Get-Location) $part.name))
        try { $input.CopyTo($output) } finally { $input.Dispose() }
      }
    } finally { $output.Dispose() }
    if (-not (Verify ($target + '.assembling') $model.sha256)) { throw 'Model assembly verification failed.' }
    Move-Item -LiteralPath ($target + '.assembling') -Destination $target
    foreach ($part in $model.parts) { Remove-Item -LiteralPath $part.name }
  }
  Write-Host 'Ready. Open Reflect Go.exe inside this Starter Pack. Both models are in Reflect Go Files/Models/.'
} catch { Write-Host $_ -ForegroundColor Red; exit 1 }
finally { if ($null -ne $lockHandle) { $lockHandle.Dispose() }; Read-Host 'Press Enter to close' | Out-Null }
'''.replace('VERSION', VERSION).replace('DATA', data).replace('BASE', BASE)


def write_downloaders(specification, software_dir, output):
    for platform in ('macos', 'windows', 'linux'):
        name = f'ReflectGo-{VERSION}-{platform}.zip'
        archive = software_dir/name
        software = {'name': name, 'sha256': digest(archive), 'size': archive.stat().st_size}
        with zipfile.ZipFile(archive) as z:software['unpacked_size']=sum(member.file_size for member in z.infolist())
        if platform == 'windows':
            (output/'Download Reflect Go Starter Pack.ps1').write_text(powershell_script(specification, software))
            (output/'Download Reflect Go Starter Pack.cmd').write_text('@echo off\r\npowershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0Download Reflect Go Starter Pack.ps1"\r\n')
        else:
            path = output/('Download Reflect Go Starter Pack.'+('command' if platform == 'macos' else 'sh'))
            path.write_text(posix_script(specification, software, platform)); path.chmod(0o755)


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--models', type=Path, required=True)
    p.add_argument('--output', type=Path, required=True)
    p.add_argument('--software-directory', type=Path)
    p.add_argument('--stage-part', help='Write only this named part from an existing plan.')
    a = p.parse_args(); a.output.mkdir(parents=True, exist_ok=True)
    manifest = a.output/'StarterPack-0.4.9.json'
    if a.stage_part:
        stage_part(a.models, json.loads(manifest.read_text()), a.stage_part, a.output/a.stage_part)
    else:
        if manifest.exists():
            raise SystemExit('Plan exists; preserve it or choose a new output directory.')
        spec = plan(a.models)
        manifest.write_text(json.dumps(spec, indent=2)+'\n')
        if a.software_directory:
            write_downloaders(spec, a.software_directory, a.output)
