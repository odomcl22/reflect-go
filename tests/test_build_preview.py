import importlib.util
import plistlib
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('build_preview', ROOT/'scripts/build_preview.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class PreviewBuildTests(unittest.TestCase):
    def test_preview_identity_is_deterministic_and_distinct_from_preserved_phases(self):
        current = module.preview_bundle_identifier()
        self.assertEqual(current, 'local.reflectgo.workstation.preview.v0-4-7')
        self.assertEqual(current, module.preview_bundle_identifier('0.4.7'))
        self.assertNotEqual(current, 'local.reflectgo.workstation')
        self.assertNotEqual(current, module.preview_bundle_identifier('0.4.6'))
        with self.assertRaises(ValueError):
            module.preview_bundle_identifier('0.4.7; anything')

    def test_preview_metadata_preserves_unrelated_settings_and_source(self):
        source = {
            'CFBundleIdentifier': 'local.reflectgo.workstation',
            'CFBundleShortVersionString': '0.4.6',
            'CFBundleVersion': '46',
            'CFBundleExecutable': 'ReflectGo',
            'CFBundleName': 'Reflect Go',
            'LSMinimumSystemVersion': '13.3',
            'NSHighResolutionCapable': True,
        }
        before = plistlib.dumps(source)
        metadata = module.preview_info(source)
        self.assertEqual(plistlib.dumps(source), before)
        self.assertEqual(metadata['CFBundleIdentifier'], module.preview_bundle_identifier())
        self.assertEqual(metadata['CFBundleShortVersionString'], '0.4.7')
        self.assertEqual(metadata['CFBundleVersion'], '47')
        for key in ('CFBundleExecutable', 'CFBundleName', 'LSMinimumSystemVersion', 'NSHighResolutionCapable'):
            self.assertEqual(metadata[key], source[key])
        self.assertIn('portable software', metadata['NSDocumentsFolderUsageDescription'])

    def test_running_guard_distinguishes_same_named_previews_with_spaces(self):
        with tempfile.TemporaryDirectory() as tmp:
            current = Path(tmp)/'Reflect Go/Phase 4.7/Reflect Go.app/Contents/MacOS/ReflectGo'
            older = Path(tmp)/'Reflect Go/Phase 4.6/Reflect Go.app/Contents/MacOS/ReflectGo'
            processes = f'  100 {older}\n  200 {current}\n  201 {current}\n  300 /usr/bin/open\n malformed\n'
            self.assertEqual(module.matching_native_pids(processes, current), [200, 201])
            self.assertEqual(module.matching_native_pids(f'100 {older}\n', current), [])

    def test_refresh_rejects_running_native_app_before_mutating_package(self):
        with tempfile.TemporaryDirectory() as tmp:
            source, destination = Path(tmp)/'Phase 4.6', Path(tmp)/'Phase 4.7'
            source.mkdir(); destination.mkdir()
            (source/'manifest.json').write_text('{}')
            marker = destination/'unchanged.txt'; marker.write_bytes(b'existing preview')
            executable = destination/'Reflect Go.app/Contents/MacOS/ReflectGo'
            result = subprocess.CompletedProcess([], 0, f' 808 {executable}\n', '')
            with patch.object(module.sys, 'platform', 'darwin'), patch.object(module.subprocess, 'run', return_value=result) as run:
                with self.assertRaisesRegex(SystemExit, 'Close Reflect Go.*PID 808'):
                    module.assemble(source, destination, refresh=True)
                self.assertEqual(run.call_count, 1)
                self.assertEqual(run.call_args.args[0], ['/bin/ps', '-axww', '-o', 'pid=,comm='])
            self.assertEqual(marker.read_bytes(), b'existing preview')
            self.assertEqual(sorted(p.name for p in destination.iterdir()), ['unchanged.txt'])

    def test_guard_cannot_treat_failed_process_scan_as_stopped(self):
        with patch.object(module.sys, 'platform', 'darwin'), patch.object(module.subprocess, 'run', side_effect=subprocess.CalledProcessError(1, ['/bin/ps'])):
            with self.assertRaisesRegex(SystemExit, 'Could not check'):
                module.require_native_app_stopped(Path('/preview/Phase 4.7'))

    def test_refresh_cannot_target_a_preserved_phase_through_python_api(self):
        with patch.object(module.subprocess, 'run') as run:
            with self.assertRaisesRegex(SystemExit, 'restricted to Phase 4.7'):
                module.assemble(Path('/source'), Path('/preview/Phase 4.6'), refresh=True)
            run.assert_not_called()


if __name__ == '__main__':
    unittest.main()
