#!/usr/bin/env python3
"""Build baseline Windows CPU engines without MSVC redistributable dependencies."""
import importlib.util,json,os,subprocess
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('phase4',ROOT/'scripts/build_phase4.py');phase4=importlib.util.module_from_spec(spec);spec.loader.exec_module(phase4)

def build():
    lock=json.loads((ROOT/'config/runtime-targets.lock.json').read_text());artifacts=json.loads((ROOT/'config/artifacts.lock.json').read_text())
    archive=phase4.download(lock['windows_build_toolchain']);folder=ROOT/'.build/toolchains/windows'
    if not folder.exists():phase4.legacy.extract_archive(archive,folder)
    compiler=next(folder.glob('*/bin/x86_64-w64-mingw32-clang')).parent
    source_archive=ROOT/'.build/downloads/source.tar.gz'
    if not source_archive.exists():
        downloaded=phase4.download({**artifacts['source'],'filename':'llama-source-'+artifacts['source']['revision']+'.tar.gz'})
        source_archive=downloaded
    if phase4.legacy.digest(source_archive)!=artifacts['source']['sha256']:raise ValueError('llama source checksum mismatch')
    source_root=ROOT/'.build/phase4/llama-source'
    if not source_root.exists():phase4.legacy.extract_archive(source_archive,source_root)
    source=next(source_root.iterdir())
    for arch,triple in [('x64','x86_64'),('arm64','aarch64')]:
        out=ROOT/'.build/phase4'/('windows-'+arch+'-static')
        flags=['-DCMAKE_SYSTEM_NAME=Windows','-DCMAKE_SYSTEM_PROCESSOR='+triple,'-DCMAKE_C_COMPILER='+str(compiler/(triple+'-w64-mingw32-clang')),'-DCMAKE_CXX_COMPILER='+str(compiler/(triple+'-w64-mingw32-clang++')),'-DCMAKE_RC_COMPILER='+str(compiler/(triple+'-w64-mingw32-windres')),'-DCMAKE_C_FLAGS=-D_WIN32_WINNT=0x0A00','-DCMAKE_CXX_FLAGS=-D_WIN32_WINNT=0x0A00','-DCMAKE_BUILD_TYPE=Release','-DBUILD_SHARED_LIBS=OFF','-DGGML_STATIC=ON','-DGGML_NATIVE=OFF','-DGGML_OPENMP=OFF','-DLLAMA_OPENSSL=OFF','-DLLAMA_BUILD_TESTS=OFF','-DLLAMA_BUILD_EXAMPLES=OFF','-DLLAMA_BUILD_UI=OFF','-DGGML_RPC=OFF','-DCMAKE_EXE_LINKER_FLAGS=-static','-DGGML_SSE42=OFF','-DGGML_AVX=OFF','-DGGML_AVX2=OFF','-DGGML_FMA=OFF','-DGGML_F16C=OFF']
        subprocess.run(['cmake','-G','Ninja','-S',str(source),'-B',str(out),*flags],check=True)
        subprocess.run(['cmake','--build',str(out),'--target','llama-server','llama-completion','--parallel','4'],check=True)
        (out/'BUILD.json').write_text(json.dumps({'revision':artifacts['source']['revision'],'toolchain':lock['windows_build_toolchain'],'flags':flags},indent=2)+'\n')
        print('Built Windows',arch,'portable CPU runtime',flush=True)
if __name__=='__main__':build()
