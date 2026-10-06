#!/usr/bin/env python3
"""Build native Phase 4 launchers and assemble verified portable runtime packs."""
import argparse
import concurrent.futures
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import struct
import subprocess
import tarfile
import tempfile
import zipfile

ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('legacy_build',ROOT/'scripts/build.py');legacy=importlib.util.module_from_spec(spec);spec.loader.exec_module(legacy)
CACHE=ROOT/'.build/downloads/phase4'

def download(info):
    CACHE.mkdir(parents=True,exist_ok=True)
    path=CACHE/info['filename']
    if not path.exists():
        partial=path.with_suffix(path.suffix+'.part')
        subprocess.run(['curl','-fLsS','--proto','=https','--retry','2','--connect-timeout','20','--max-time','900',info['url'],'-o',str(partial)],check=True)
        if legacy.digest(partial)!=info['sha256']:raise ValueError('Download checksum mismatch: '+path.name)
        partial.rename(path)
    if legacy.digest(path)!=info['sha256']:raise ValueError('Cached checksum mismatch: '+path.name)
    return path

def unpack(archive,dest):
    dest.mkdir(parents=True,exist_ok=True)
    if archive.suffix=='.zip':
        with zipfile.ZipFile(archive) as z:
            for member in z.infolist():
                path=Path(member.filename)
                if path.is_absolute() or '..' in path.parts or '\\' in member.filename or (member.external_attr>>16)&0o170000==0o120000:raise ValueError('Unsafe ZIP member')
                target=dest/path
                if member.is_dir():target.mkdir(parents=True,exist_ok=True)
                else:
                    target.parent.mkdir(parents=True,exist_ok=True)
                    with z.open(member)as src,target.open('wb')as out:shutil.copyfileobj(src,out)
        return dest
    legacy.extract_archive(archive,dest)
    return next(p for p in dest.iterdir() if p.is_dir())

def keep_runtime(name):
    if name.startswith(('LICENSE','COPYING','NOTICE')):return True
    if name in ('llama-server','llama-completion','llama-server.exe','llama-completion.exe'):return True
    if 'rpc' in name and name.endswith('.exe'):return False
    if '-impl' in name and 'server-impl' not in name and 'completion-impl' not in name:return False
    return '.so' in name or name.endswith(('.dll','.dylib','.metallib'))

def elf_audit(path):
    data=path.read_bytes()
    if data[:4]!=b'\x7fELF':return None
    if data[4:6]!=b'\x02\x01':raise ValueError('Expected 64-bit little-endian ELF')
    h=struct.unpack_from('<16sHHIQQQIHHHHHH',data)
    sections=[struct.unpack_from('<IIQQQQIIQQ',data,h[6]+i*h[11]) for i in range(h[12])]
    deps=[];paths=[]
    for section in sections:
        if section[1]!=6:continue
        st=sections[section[6]];strings=data[st[4]:st[4]+st[5]]
        for pos in range(section[4],section[4]+section[5],16):
            tag,value=struct.unpack_from('<qQ',data,pos)
            if tag in (1,15,29):
                value=strings[value:].split(b'\0')[0].decode()
                (deps if tag==1 else paths).append(value)
    versions=sorted(set(re.findall(rb'GLIBC_[0-9.]+',data)),key=lambda x:tuple(map(int,x[6:].split(b'.'))))
    cpp=sorted(set(re.findall(rb'GLIBCXX_[0-9.]+',data)),key=lambda x:tuple(map(int,x[8:].split(b'.'))))
    return {'file':path.name,'machine':h[2],'dependencies':deps,'rpaths':paths,'glibc':versions[-1].decode()[6:] if versions else None,'glibcxx':cpp[-1].decode()[8:] if cpp else None}

def pe_audit(path):
    data=path.read_bytes()
    if data[:2]!=b'MZ':return None
    pe=struct.unpack_from('<I',data,60)[0]
    if data[pe:pe+4]!=b'PE\0\0':raise ValueError('Invalid PE header')
    machine,count,_,_,_,optsize,_=struct.unpack_from('<HHIIIHH',data,pe+4)
    opt=pe+24
    if struct.unpack_from('<H',data,opt)[0]!=0x20b:raise ValueError('Expected PE32+')
    sections=[struct.unpack_from('<8sIIIIIIHHI',data,opt+optsize+40*i) for i in range(count)]
    def offset(rva):
        for _,virtual_size,virtual_addr,raw_size,raw_addr,*_ in sections:
            if virtual_addr<=rva<virtual_addr+max(virtual_size,raw_size):return raw_addr+rva-virtual_addr
        raise ValueError('PE address out of bounds')
    imports=[]
    # Ordinary and delay-load imports, both must be available on the target.
    for index,width,name_index in [(1,20,3),(13,32,1)]:
        rva,size=struct.unpack_from('<II',data,opt+112+8*index)
        if not rva:continue
        start=offset(rva)
        for pos in range(start,start+size,width):
            entry=struct.unpack_from('<'+'I'*(width//4),data,pos)
            if not any(entry):break
            begin=offset(entry[name_index]);imports.append(data[begin:].split(b'\0')[0].decode())
    return {'file':path.name,'machine':machine,'dependencies':sorted(set(imports))}

def unpack_deb(archive,dest):
    # An ar container with a pinned, official Ubuntu data archive. Validate every
    # extracted tar path and link using the same checks as runtime downloads.
    data=archive.read_bytes();assert data[:8]==b'!<arch>\n';offset=8;found=None
    while offset+60<=len(data):
        header=data[offset:offset+60];name=header[:16].decode().strip().rstrip('/');size=int(header[48:58]);offset+=60
        if name.startswith('data.tar'):found=(name,data[offset:offset+size])
        offset+=size+(size%2)
    if found is None:raise ValueError('Missing deb data archive')
    name,payload=found;temp=dest.parent/(archive.stem+'-'+name);temp.write_bytes(payload)
    # macOS libarchive supports zstd; repack to uncompressed tar for the shared
    # validated extractor, avoiding a Python decompression package dependency.
    if name.endswith('.zst'):
        names=subprocess.check_output(['/usr/bin/tar','-tf',str(temp)],text=True).splitlines()
        if any(Path(n).is_absolute() or '..' in Path(n).parts for n in names):raise ValueError('Unsafe deb path')
        # Ubuntu archives may have documentation links outside individual folders,
        # but all resolve within this temporary package root; inspect before use.
        dest.mkdir(parents=True,exist_ok=True)
        subprocess.run(['/usr/bin/tar','-xf',str(temp),'-C',str(dest)],check=True)
        for p in dest.rglob('*'):
            if p.is_symlink() and not p.resolve().is_relative_to(dest.resolve()):raise ValueError('Unsafe deb link')
    else:legacy.extract_archive(temp,dest)
    return dest

def assemble(output):
    if output.exists():raise ValueError('Destination exists; choose a fresh Phase 4 folder')
    if not (output.parent/'Models').is_dir():raise ValueError('Phase 4 needs its existing sibling Models library')
    lock=json.loads((ROOT/'config/runtime-targets.lock.json').read_text());artifacts=json.loads((ROOT/'config/artifacts.lock.json').read_text())
    toolchain=ROOT/'.build/toolchains/go';compiler=toolchain/'bin/go'
    archive=download(lock['build_toolchain'])
    if not compiler.exists():legacy.extract_archive(archive,toolchain.parent)
    version=subprocess.check_output([str(compiler),'version'],text=True)
    if lock['build_toolchain']['version'] not in version:raise ValueError('Unexpected Go compiler version')
    infos=list(lock['targets'].values())+lock.get('linux_libraries',[])+[lock['linux_license_package']]
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as ex:list(ex.map(download,infos))
    with tempfile.TemporaryDirectory(prefix='phase4-assembly-',dir=ROOT/'.build') as temporary:
        work=Path(temporary);bundle=work/'Phase 4';bundle.mkdir();packs={};audits=[]
        for id,info in lock['targets'].items():
            upstream=unpack(CACHE/info['filename'],work/id);runtime_dir=bundle/'runtime'/id;runtime_dir.mkdir(parents=True)
            if id.startswith('windows') and id.endswith('-cpu'):
                arch='x64' if '-x64-' in id else 'arm64'
                upstream=ROOT/'.build/phase4'/('windows-'+arch+'-static')/'bin'
                if not (upstream/'llama-server.exe').exists():raise ValueError('Run scripts/build_windows_runtime.py first')
                shutil.copyfile(upstream.parent/'BUILD.json',runtime_dir/'BUILD.json')
            for path in upstream.iterdir():
                if path.is_file() and keep_runtime(path.name):shutil.copyfile(path,runtime_dir/path.name);(runtime_dir/path.name).chmod(0o755)
            kind=id.rsplit('-',1)[1];platform=id.rsplit('-',1)[0]
            packs[id]={'path':'runtime/'+id,'platform':platform,'kind':kind,'minimum':'macOS 13.3+' if id.startswith('macos') else 'Windows 10/11, 64-bit' if id.startswith('windows') else 'glibc Linux; see dependency-audit.json'}
        gcc_docs=unpack_deb(CACHE/lock['linux_license_package']['filename'],work/'gcc-licenses')
        shutil.copytree(gcc_docs/'usr/share/doc/gcc-14-base',bundle/'licenses/gcc-runtime',symlinks=False)
        for info in lock.get('linux_libraries',[]):
            files=unpack_deb(CACHE/info['filename'],work/('deb-'+info['name']+'-'+info['arch']))
            names={'libgomp1':['libgomp.so.1'],'libstdc++6':['libstdc++.so.6'],'libgcc-s1':['libgcc_s.so.1'],'libssl3t64':['libssl.so.3','libcrypto.so.3']}
            selected=names[info['name']]
            arch='x64' if info['arch']=='amd64' else 'arm64'
            for id in packs:
                if id.startswith('linux-'+arch+'-'):
                    for name in selected:
                        source=next(p for p in files.rglob(name) if p.is_file());shutil.copyfile(source,bundle/'runtime'/id/name)
            notices=bundle/'licenses'/'ubuntu'/info['arch']/info['name'];notices.mkdir(parents=True)
            for p in files.rglob('*'):
                if p.is_file() and p.name in ('copyright','changelog.Debian.gz'):shutil.copyfile(p,notices/p.name)
            (notices/'SOURCE.json').write_text(json.dumps(info,indent=2)+'\n')
        for id,pack in packs.items():
            folder=bundle/pack['path'];local_audits=[]
            for p in sorted(folder.iterdir()):
                if id.startswith('linux'):
                    result=elf_audit(p)
                    if result:
                        expected=62 if '-x64-' in id else 183
                        if result['machine']!=expected:raise ValueError('Wrong ELF architecture: '+str(p))
                        system={'libc.so.6','libm.so.6','libdl.so.2','libpthread.so.0','librt.so.1','ld-linux-x86-64.so.2','ld-linux-aarch64.so.1','libvulkan.so.1'}
                        for dep in result['dependencies']:
                            if not (folder/dep).exists() and dep not in system:raise ValueError('Unbundled Linux dependency: '+dep)
                        local_audits.append(result)
                elif id.startswith('macos'):
                    arch='arm64' if '-arm64-' in id else 'x86_64'
                    file=subprocess.check_output(['/usr/bin/file','-b',str(p)],text=True)
                    if 'Mach-O' not in file:continue
                    if arch not in file:raise ValueError('Wrong Mac architecture: '+str(p))
                    dependencies=legacy.dependencies(p)
                    for dep in dependencies:
                        if dep.startswith(('/System/Library/','/usr/lib/')):continue
                        if dep.startswith(('@rpath/','@loader_path/')) and (folder/dep.split('/',1)[1]).exists():continue
                        raise ValueError('Unbundled Mac dependency: '+dep)
                    local_audits.append({'file':p.name,'dependencies':dependencies,'architecture':arch})
                elif id.startswith('windows'):
                    result=pe_audit(p)
                    if result:
                        expected=0x8664 if '-x64-' in id else 0xaa64
                        if result['machine']!=expected:raise ValueError('Wrong PE architecture: '+str(p))
                        local_audits.append(result)
            if id.startswith('windows'):
                available={p.name.lower() for p in folder.iterdir()}
                system={'msvcp140.dll','vcruntime140.dll','vcruntime140_1.dll'} if id.endswith('-vulkan') else set()
                system |= {'kernel32.dll','ntdll.dll','user32.dll','shell32.dll','ole32.dll','oleaut32.dll','advapi32.dll','ws2_32.dll','bcrypt.dll','crypt32.dll','secur32.dll','shlwapi.dll','psapi.dll','version.dll','msvcrt.dll','ucrtbase.dll','vulkan-1.dll'}
                for a in local_audits:
                    for dep in a['dependencies']:
                        if dep.lower() not in available|system and not dep.lower().startswith(('api-ms-win-','ext-ms-win-')):raise ValueError('Unbundled Windows dependency: '+dep)
            if id=='windows-x64-vulkan':pack['minimum']='Windows 10/11; existing Vulkan driver/loader and MSVC 2015–2022 runtime. Auto falls back to self-contained CPU when absent.'
            if id.startswith('linux'):
                floor=max((a['glibc'] for a in local_audits if a['glibc']),key=lambda s:tuple(map(int,s.split('.'))))
                pack['minimum']='glibc '+floor+'+ (Ubuntu 24.04 / Debian 13 qualification baseline); Vulkan needs a host driver/loader'
            audits.append({'target':id,'files':local_audits})
        (bundle/'runtime-targets.json').write_text(json.dumps(packs,indent=2)+'\n')
        (bundle/'dependency-audit.json').write_text(json.dumps(audits,indent=2)+'\n')
        env={**os.environ,'GOCACHE':str(ROOT/'.build/go-cache'),'GOTOOLCHAIN':'local','CGO_ENABLED':'0'}
        targets=[('darwin','arm64','macos-arm64'),('darwin','amd64','macos-x64'),('linux','arm64','linux-arm64'),('linux','amd64','linux-x64'),('windows','arm64','windows-arm64'),('windows','amd64','windows-x64')]
        def compile_target(target):
            osname,arch,id=target;dest=bundle/'bin'/id;dest.mkdir(parents=True);binary=dest/('reflectgo.exe' if osname=='windows' else 'reflectgo')
            subprocess.run([str(compiler),'build','-trimpath','-ldflags=-s -w','-o',str(binary),'.'],cwd=ROOT/'Sources/ReflectGo',env={**env,'GOOS':osname,'GOARCH':arch},check=True)
            print('Built',id,flush=True)
        with concurrent.futures.ThreadPoolExecutor(max_workers=3) as ex:list(ex.map(compile_target,targets))
        # The existing native Mac app remains a universal front end.
        app=bundle/'Reflect Go.app/Contents';(app/'MacOS').mkdir(parents=True)
        info={'CFBundleExecutable':'ReflectGo','CFBundleIdentifier':'local.reflectgo.workstation','CFBundleName':'Reflect Go','CFBundleDisplayName':'Reflect Go','CFBundlePackageType':'APPL','CFBundleShortVersionString':'0.4.0','CFBundleVersion':'40','LSMinimumSystemVersion':'13.3','NSHighResolutionCapable':True,'NSPrincipalClass':'NSApplication','NSRemovableVolumesUsageDescription':'Reflect Go reads models and saves settings on the portable drive you open it from.','NSAppTransportSecurity':{'NSAllowsLocalNetworking':True}}
        (app/'Info.plist').write_bytes(plistlib.dumps(info))
        for arch in ['arm64','x86_64']:
            subprocess.run(['/usr/bin/xcrun','swiftc','-parse-as-library','-O','-target',arch+'-apple-macosx13.3','-module-cache-path',str(ROOT/'.build/module-cache'),str(ROOT/'Sources/Workstation.swift'),'-o',str(work/('app-'+arch))],check=True)
            subprocess.run(['/usr/bin/xcrun','clang','-O2','-Wall','-Wextra','-Werror','-target',arch+'-apple-macosx13.3',str(ROOT/'Sources/EngineHost.c'),'-o',str(work/('host-'+arch))],check=True)
        subprocess.run(['/usr/bin/lipo','-create',str(work/'app-arm64'),str(work/'app-x86_64'),'-output',str(app/'MacOS/ReflectGo')],check=True)
        subprocess.run(['/usr/bin/lipo','-create',str(work/'host-arm64'),str(work/'host-x86_64'),'-output',str(bundle/'bin/engine-host')],check=True)
        subprocess.run(['/usr/bin/codesign','--force','--sign','-',str(app.parent)],check=True)
        for name in ['config','licenses']:
            if name=='licenses':shutil.copytree(ROOT/'dist/Reflect Go/Phase 3.1/licenses',bundle/name,dirs_exist_ok=True)
            else:(bundle/name).mkdir()
        for name in ['codex-local-instructions.txt','claude-local-instructions.txt']:shutil.copyfile(ROOT/'config'/name,bundle/'config'/name)
        windows_tools=next((ROOT/'.build/toolchains/windows').iterdir())
        (bundle/'licenses/llvm-mingw').mkdir(parents=True)
        shutil.copyfile(windows_tools/'LICENSE.TXT',bundle/'licenses/llvm-mingw/LLVM-LICENSE.TXT')
        shutil.copytree(windows_tools/'x86_64-w64-mingw32/share/mingw32',bundle/'licenses/llvm-mingw/mingw-runtime',symlinks=False)
        shutil.copytree(ROOT/'config/gcc-licenses',bundle/'licenses/gcc-runtime',dirs_exist_ok=True)
        shutil.copyfile(toolchain/'LICENSE',bundle/'licenses/Go-LICENSE');shutil.copyfile(toolchain/'PATENTS',bundle/'licenses/Go-PATENTS')
        settings=json.loads((ROOT/'config/portableai.json').read_text());settings.update(models_directory='../Models',provider_context=16384,provider_model=artifacts['agent_model']['name'],temperature=.2)
        (bundle/'config/portableai.json').write_text(json.dumps(settings,indent=2)+'\n')
        shutil.copyfile(ROOT/'config/runtime-targets.lock.json',bundle/'runtime-targets.lock.json')
        shutil.copyfile(ROOT/'docs/PHASE_4_README.md',bundle/'README.md');shutil.copyfile(ROOT/'docs/PHASE_4_VALIDATION.md',bundle/'VALIDATION.md')
        for source in (ROOT/'launchers/phase4').iterdir():shutil.copyfile(source,bundle/source.name);(bundle/source.name).chmod(0o755)
        # The native app calls this small system-shell selector; the actual launcher is native Go.
        (bundle/'bin/portableai').write_text('#!/bin/sh\nROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)\ncase "$(uname -m)" in arm64) ARCH=arm64;; x86_64) ARCH=x64;; *) echo "Unsupported Mac architecture" >&2; exit 1;; esac\nexec "$ROOT/bin/macos-$ARCH/reflectgo" "$@"\n');(bundle/'bin/portableai').chmod(0o755)
        legacy.write_manifest(bundle);manifest=json.loads((bundle/'manifest.json').read_text())
        for model in [artifacts['model'],artifacts['agent_model']]:
            path=output.parent/'Models'/model['name']
            if legacy.digest(path)!=model['sha256']:raise ValueError('Shared model integrity failed')
            manifest['models/'+model['name']]=model['sha256']
        (bundle/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n')
        shutil.move(str(bundle),output)
    print('Assembled',output,'— no model copies; other platforms still require physical qualification.')

if __name__=='__main__':
    parser=argparse.ArgumentParser(description=__doc__);parser.add_argument('--output',type=Path,default=ROOT/'dist/Reflect Go/Phase 4');args=parser.parse_args();assemble(args.output.resolve())
