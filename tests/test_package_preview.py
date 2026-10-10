import hashlib,importlib.util,json,tempfile,unittest,zipfile
from pathlib import Path
ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('package_preview',ROOT/'scripts/package_preview.py');module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
class PackageTests(unittest.TestCase):
 def test_private_state_and_proprietary_binary_excluded(self):
  with tempfile.TemporaryDirectory()as tmp:
   root=Path(tmp)/'Phase';root.mkdir();files={'bin/tool':b'public','harnesses/claude/test/claude':b'proprietary','state/provider-key':b'secret','models/private.gguf':b'weights','harnesses/installed.json':json.dumps({'claude/test':{},'codex/test':{}}).encode()}
   for name,data in files.items():
    path=root/name;path.parent.mkdir(parents=True,exist_ok=True);path.write_bytes(data)
   (root/'manifest.json').write_text(json.dumps({k:hashlib.sha256(v).hexdigest()for k,v in files.items()}))
   output=Path(tmp)/'software.zip';module.package(root,output)
   with zipfile.ZipFile(output)as z:
    names=z.namelist();self.assertFalse(any('/state/'in n or '/claude/'in n or n.endswith('.gguf')for n in names))
    installed=json.loads(z.read('Reflect Go/Phase 4.6/harnesses/installed.json'));self.assertNotIn('claude/test',installed)
    manifest=json.loads(z.read('Reflect Go/Phase 4.6/manifest.json'));self.assertNotIn('state/provider-key',manifest)
if __name__=='__main__':unittest.main()
