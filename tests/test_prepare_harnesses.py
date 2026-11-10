"""Verify a user-side native pack addition without network or executing the pack."""
import base64
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest.mock import patch

ROOT=Path(__file__).resolve().parents[1]
spec=importlib.util.spec_from_file_location('prepare_harnesses',ROOT/'scripts/prepare_harnesses.py')
module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)


class PersonalInstall(unittest.TestCase):
    def test_selected_client_is_verified_installed_and_manifested(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);bundle=root/'Reflect Go';bundle.mkdir()
            other=bundle/'harnesses/codex/macos-arm64/codex'
            other.parent.mkdir(parents=True);other.write_bytes(b'tampered existing binary')
            old_hash=hashlib.sha256(b'original verified binary').hexdigest()
            other_rel=other.relative_to(bundle).as_posix()
            (bundle/'manifest.json').write_text(json.dumps({other_rel:old_hash}))
            payload=b'fixture executable, never run'
            archive=io.BytesIO()
            with tarfile.open(fileobj=archive,mode='w:gz') as out:
                info=tarfile.TarInfo('package/claude');info.size=len(payload)
                out.addfile(info,io.BytesIO(payload))
            data=archive.getvalue();key='claude/macos-arm64'
            lock=root/'lock.json'
            lock.write_text(json.dumps({'versions':{'claude':'test'},'packs':{
                key:{'url':'https://example.test/claude.tgz','sha256':hashlib.sha256(data).hexdigest(),'license':'test'},
                'codex/macos-arm64':{'url':'https://example.test/must-not-download'},
            }}))
            with patch.object(module.urllib.request,'urlopen',return_value=io.BytesIO(data)) as fetch:
                module.install(bundle,['macos-arm64'],clients=['claude'],lock_path=lock,cache_dir=bundle/'state/downloads')
            fetch.assert_called_once()
            record=json.loads((bundle/'harnesses/installed.json').read_text())[key]
            exe=bundle/record['path']
            self.assertEqual(exe.read_bytes(),payload)
            self.assertEqual(record['sha256'],hashlib.sha256(payload).hexdigest())
            manifest=json.loads((bundle/'manifest.json').read_text())
            self.assertEqual(manifest[record['path']],record['sha256'])
            self.assertEqual(manifest[other_rel],old_hash,'Unrelated binary was silently rebaselined')
            self.assertEqual(manifest['harnesses/installed.json'],module.digest(bundle/'harnesses/installed.json'))
            with patch.object(module.urllib.request,'urlopen') as fetch:
                module.install(bundle,['macos-arm64'],clients=['claude'],lock_path=lock,cache_dir=bundle/'state/downloads')
            fetch.assert_not_called()

    def test_corrupt_archive_rejected_for_sha256_and_npm_integrity(self):
        for field,value in [('sha256','0'*64),('integrity','sha512-'+base64.b64encode(bytes(64)).decode())]:
            with self.subTest(field=field), tempfile.TemporaryDirectory() as tmp:
                root=Path(tmp);bundle=root/'Reflect Go';bundle.mkdir()
                lock=root/'lock.json'
                lock.write_text(json.dumps({'versions':{'claude':'test'},'packs':{
                    'claude/macos-arm64':{'url':'https://example.test/claude.tgz',field:value,'license':'test'},
                }}))
                with patch.object(module.urllib.request,'urlopen',return_value=io.BytesIO(b'corrupt archive')):
                    with self.assertRaisesRegex(ValueError,'checksum mismatch'):
                        module.install(bundle,['macos-arm64'],clients=['claude'],lock_path=lock,cache_dir=bundle/'state/downloads')
                self.assertFalse((bundle/'harnesses/claude').exists())
                self.assertFalse((bundle/'harnesses/installed.json').exists())

    def test_changed_installed_index_is_not_rebaselined(self):
        with tempfile.TemporaryDirectory() as tmp:
            root=Path(tmp);bundle=root/'Reflect Go';(bundle/'harnesses').mkdir(parents=True)
            (bundle/'harnesses/installed.json').write_text('{}')
            (bundle/'manifest.json').write_text(json.dumps({'harnesses/installed.json':'0'*64}))
            lock=root/'lock.json';lock.write_text('{"packs":{}}')
            with self.assertRaisesRegex(ValueError,'index failed its integrity check'):
                module.install(bundle,['macos-arm64'],lock_path=lock,cache_dir=bundle/'state/downloads')

    def test_missing_platform_does_not_print_ready(self):
        with tempfile.TemporaryDirectory() as tmp:
            bundle=Path(tmp);(bundle/'harnesses').mkdir();(bundle/'config').mkdir()
            (bundle/'harnesses/installed.json').write_text('{}')
            (bundle/'config/claude-cli.lock.json').write_text('{"packs":{}}')
            result=subprocess.run([sys.executable,str(ROOT/'scripts/add_claude_cli.py'),
                                   '--bundle',str(bundle),'--platform','linux-arm64'],capture_output=True,text=True)
            self.assertNotEqual(result.returncode,0)
            self.assertIn('No pinned claude download is available for linux-arm64',result.stderr)
            self.assertNotIn('is ready',result.stdout)
            self.assertFalse((bundle/'harnesses/claude').exists())


if __name__=='__main__':
    unittest.main()
