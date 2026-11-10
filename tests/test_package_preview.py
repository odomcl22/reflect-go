import hashlib,importlib.util,json,tempfile,unittest,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('package_preview',ROOT/'scripts/package_preview.py');module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
class PackageTests(unittest.TestCase):
 def test_top_level_is_only_launcher_and_one_support_folder(self):
  with tempfile.TemporaryDirectory() as tmp:
   root=Path(tmp)/'Phase';root.mkdir()
   files={'bin/macos-arm64/reflectgo':b'mac','bin/windows-x64/reflectgo.exe':b'win','bin/linux-x64/reflectgo':b'linux','START Reflect Go.exe':b'launcher','Reflect Go.app/Contents/MacOS/ReflectGo':b'app','Reflect Go.sh':b'#!/bin/sh\n','config/portableai.json':b'{"models_directory":"../Models"}'}
   for name,data in files.items():
    p=root/name;p.parent.mkdir(parents=True,exist_ok=True);p.write_bytes(data)
   (root/'manifest.json').write_text(json.dumps({k:hashlib.sha256(v).hexdigest() for k,v in files.items()}))
   for platform,launcher in [('macos','Reflect Go.app'),('windows','Reflect Go.exe'),('linux','Reflect Go.desktop')]:
    archive=Path(tmp)/(platform+'.zip');module.package(root,archive,platform=platform)
    with zipfile.ZipFile(archive) as z:
     children={n.split('/')[1] for n in z.namelist() if n.startswith('Reflect Go/')}
     self.assertEqual(children,{launcher,'Reflect Go Files'})
     manifest=json.loads(z.read('Reflect Go/Reflect Go Files/manifest.json'))
     for rel,sha in manifest.items():self.assertEqual(hashlib.sha256(z.read('Reflect Go/Reflect Go Files/'+rel)).hexdigest(),sha)
     self.assertIn('Reflect Go/Reflect Go Files/Models/',z.namelist())
 def test_private_state_and_proprietary_binary_excluded(self):
  with tempfile.TemporaryDirectory()as tmp:
   root=Path(tmp)/'Phase';root.mkdir();files={'bin/tool':b'public','harnesses/claude/test/claude':b'proprietary','state/provider-key':b'secret','models/private.gguf':b'weights','harnesses/installed.json':json.dumps({'claude/test':{},'codex/test':{}}).encode()}
   for name,data in files.items():
    path=root/name;path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(data)
   (root/'manifest.json').write_text(json.dumps({k:hashlib.sha256(v).hexdigest()for k,v in files.items()}))
   output=Path(tmp)/'software.zip';module.package(root,output)
   with zipfile.ZipFile(output)as z:
    names=z.namelist();self.assertFalse(any('/state/'in n or '/claude/'in n or n.endswith('.gguf')for n in names))
    installed=json.loads(z.read('Reflect Go/Reflect Go Files/harnesses/installed.json'));self.assertNotIn('claude/test',installed)
    self.assertIn('Reflect Go/Reflect Go Files/tools/add_claude_cli.py',names)
    lock=json.loads(z.read('Reflect Go/Reflect Go Files/config/claude-cli.lock.json'))
    self.assertTrue(lock['packs']);self.assertTrue(all(k.startswith('claude/') for k in lock['packs']))
    manifest=json.loads(z.read('Reflect Go/Reflect Go Files/manifest.json'));self.assertNotIn('state/provider-key',manifest)
 def test_platform_archive_has_flat_launcher_matching_metadata_and_model_location(self):
  with tempfile.TemporaryDirectory() as tmp:
   root=Path(tmp)/'Phase';root.mkdir()
   files={'runtime/windows-x64-cpu/BUILD.json':json.dumps({'flags':['-DCMAKE_C_COMPILER=/Users/private-builder/tools/clang']}).encode(),'bin/windows-x64/reflectgo.exe':b'win','bin/macos-arm64/reflectgo':b'mac','START Reflect Go.exe':b'launcher','Reflect Go.app/Contents/MacOS/ReflectGo':b'app',
    'harnesses/codex/windows-x64/codex.exe':b'codex','harnesses/codex/macos-arm64/codex':b'other',
    'config/portableai.json':json.dumps({'models_directory':'../Models','provider_model':'private.gguf'}).encode(),
    'harnesses/installed.json':json.dumps({'codex/windows-x64':{'path':'harnesses/codex/windows-x64/codex.exe'},'codex/macos-arm64':{'path':'harnesses/codex/macos-arm64/codex'}}).encode()}
   for name,data in files.items():
    path=root/name;path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(data)
   (root/'manifest.json').write_text(json.dumps({k:hashlib.sha256(v).hexdigest() for k,v in files.items()}))
   output=Path(tmp)/'windows.zip';module.package(root,output,platform='windows')
   with zipfile.ZipFile(output) as z:
    self.assertIn('Reflect Go/Reflect Go.exe',z.namelist())
    metadata=z.read('Reflect Go/Reflect Go Files/runtime/windows-x64-cpu/BUILD.json');self.assertNotIn(b'private-builder',metadata);self.assertIn(b'<BUILD_HOME>',metadata)
    self.assertFalse(any('macos' in n or '.app/' in n for n in z.namelist()))
    config=json.loads(z.read('Reflect Go/Reflect Go Files/config/portableai.json'));self.assertEqual(config['models_directory'],'Models');self.assertNotIn('provider_model',config)
    installed=json.loads(z.read('Reflect Go/Reflect Go Files/harnesses/installed.json'));self.assertEqual(list(installed),['codex/windows-x64'])
    lock=json.loads(z.read('Reflect Go/Reflect Go Files/config/claude-cli.lock.json'))
    self.assertEqual(set(lock['packs']),{'claude/'+p for p in ['macos-arm64','macos-x64','windows-x64','windows-arm64','linux-x64','linux-arm64']})
    for rel,digest in json.loads(z.read('Reflect Go/Reflect Go Files/manifest.json')).items():self.assertEqual(hashlib.sha256(z.read('Reflect Go/Reflect Go Files/'+rel)).hexdigest(),digest)
if __name__=='__main__':unittest.main()
