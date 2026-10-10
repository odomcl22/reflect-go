#!/usr/bin/env python3
"""Package the checked-in vector-rendered artwork as native Mac/Windows icons."""
from pathlib import Path
import json, shutil, struct, subprocess
ROOT=Path(__file__).resolve().parents[1]
def build():
    assets=ROOT/'assets/icons'
    iconset=ROOT/'.build/icons/ReflectGo.iconset';iconset.mkdir(parents=True,exist_ok=True)
    for size in (16,32,128,256,512):
        for scale in (1,2):
            shutil.copyfile(assets/f'icon-{size*scale}.png',iconset/f'icon_{size}x{size}{"@2x" if scale==2 else ""}.png')
    subprocess.run(['iconutil','-c','icns',str(iconset),'-o',str(assets/'ReflectGo.icns')],check=True)
    sizes=(16,32,48,64,128,256);images=[(assets/f'icon-{s}.png').read_bytes() for s in sizes]
    offset=6+16*len(sizes);entries=[]
    for size,data in zip(sizes,images):
        entries.append(struct.pack('<BBBBHHII',size%256,size%256,0,0,1,32,len(data),offset));offset+=len(data)
    (assets/'ReflectGo.ico').write_bytes(struct.pack('<HHH',0,1,len(sizes))+b''.join(entries)+b''.join(images))
    resource=ROOT/'.build/icons/winres.json'
    resource.write_text(json.dumps({'RT_GROUP_ICON':{'APP':{'0000':'../../assets/icons/ReflectGo.ico'}}}))
    tool=ROOT/'.build/tools/go-winres'
    if not tool.exists(): raise SystemExit('Install github.com/tc-hib/go-winres@v0.3.3 into .build/tools first.')
    for package,arches in [('ReflectGo','amd64,arm64'),('WindowsLauncher','386')]:
        subprocess.run([str(tool),'make','--in',str(resource),'--arch',arches,'--out',str(ROOT/'Sources'/package/'rsrc')],check=True)
if __name__=='__main__':build()
