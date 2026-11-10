import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
import zipfile

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('package_starter', ROOT/'scripts/package_starter.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class StarterTests(unittest.TestCase):
    def fixture(self, root):
        models = root/'models'; models.mkdir()
        catalog = []
        for index, id in enumerate(module.MODEL_IDS):
            data = (b'GGUF model data '+bytes([index]))*7
            name = id+'.gguf'; (models/name).write_bytes(data)
            catalog.append(dict(id=id, filename=name, size=len(data), sha256=hashlib.sha256(data).hexdigest(),
                                name=id, license='apache-2.0', source='https://example.org/model'))
        return models, catalog

    def test_chunks_reconstruct_verified_models_and_reject_corruption(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); models, catalog = self.fixture(root)
            plan = module.plan(models, part_size=31, catalog=catalog)
            for m in plan['models']:
                reconstructed = bytearray()
                for p in m['parts']:
                    output = root/p['name']; module.stage_part(models, plan, p['name'], output)
                    self.assertLessEqual(output.stat().st_size, 31)
                    reconstructed.extend(output.read_bytes())
                    with self.assertRaises(ValueError):
                        module.stage_part(models, plan, p['name'], output)
                self.assertEqual(hashlib.sha256(reconstructed).hexdigest(), m['sha256'])
            (models/catalog[0]['filename']).write_bytes(b'x'*catalog[0]['size'])
            with self.assertRaises(ValueError):
                module.plan(models, part_size=31, catalog=catalog)
            damaged = root/'damaged'
            with self.assertRaises(ValueError):
                module.stage_part(models, plan, plan['models'][0]['parts'][0]['name'], damaged)
            self.assertFalse(damaged.exists())

    def test_generated_downloader_assembles_preloaded_folder_without_app_downloads(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); models, catalog = self.fixture(root)
            plan = module.plan(models, part_size=31, catalog=catalog)
            assets = root/'assets'; assets.mkdir()
            for m in plan['models']:
                for p in m['parts']:
                    module.stage_part(models, plan, p['name'], assets/p['name'])
            app = assets/'software.zip'
            with zipfile.ZipFile(app, 'w') as z:
                z.writestr('Reflect Go/Reflect Go Files/Models/', '')
                z.writestr('Reflect Go/Reflect Go Files/app.txt', 'verified application fixture')
            software = dict(name=app.name, sha256=module.digest(app))
            downloads = root/'downloads'; downloads.mkdir()
            for name in ('MODEL-NOTICES.txt','MODEL-LICENSE-APACHE-2.0.txt'):
                (downloads/name).write_text('model notices fixture')
            script = downloads/'download.sh'; script.write_text(module.posix_script(plan, software, 'linux'))
            # Replace only the network transport with local fixture copies. Run
            # the actual generated shell verification and assembly unchanged.
            tools = root/'tools'; tools.mkdir()
            curl = tools/'curl'; curl.write_text('#!/bin/sh\nwhile test "$#" -gt 0; do\n if test "$1" = --output; then shift; out=$1; fi\n url=$1; shift\ndone\ncp "$FIXTURE_ASSETS/${url##*/}" "$out"\n'); curl.chmod(0o755)
            env = {**os.environ, 'PATH': str(tools)+os.pathsep+os.environ['PATH'], 'FIXTURE_ASSETS': str(assets)}
            subprocess.run(['sh', str(script)], check=True, env=env, capture_output=True)
            folder = downloads/f'Reflect Go Starter Pack {module.VERSION}'/'Reflect Go'/'Reflect Go Files'
            for m in catalog:
                self.assertEqual((folder/'Models'/m['filename']).read_bytes(), (models/m['filename']).read_bytes())
            self.assertEqual((folder/'app.txt').read_text(), 'verified application fixture')
            self.assertEqual((folder/'licenses/models/MODEL-NOTICES.txt').read_text(),'model notices fixture')
            subprocess.run(['sh', str(script)], check=True, env=env, capture_output=True)
            target = folder/'Models'/catalog[0]['filename']; target.write_text('existing data')
            result = subprocess.run(['sh', str(script)], env=env, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(target.read_text(), 'existing data')
            # Insufficient space must stop before even the first HTTP transfer.
            empty=root/'no-space';empty.mkdir();blocked=empty/'download.sh';blocked.write_text(module.posix_script(plan,software,'linux'))
            df=tools/'df';df.write_text('#!/bin/sh\nprintf "Filesystem 1024-blocks Used Available Capacity Mounted\\nfixture 1 1 0 100%% /\\n"\n');df.chmod(0o755)
            result=subprocess.run(['sh',str(blocked)],env=env,capture_output=True)
            self.assertNotEqual(result.returncode,0)
            self.assertIn(b'free storage',result.stderr)
            self.assertFalse((empty/f'Reflect Go Starter Pack {module.VERSION}'/'software.zip').exists())


if __name__ == '__main__':
    unittest.main()
