#!/usr/bin/env python3
"""Exercise real process ownership, signals, timeout and crash cleanup."""
import os
from pathlib import Path
import signal
import subprocess
import sys
import time
import unittest

HOST = str(Path(sys.argv.pop(1)).resolve())

class EngineHostTests(unittest.TestCase):
    def start(self, code, timeout=None):
        args = [HOST] + (["--timeout", str(timeout)] if timeout else [])
        child = subprocess.Popen(args + [sys.executable, "-u", "-c", code],
                                 stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.addCleanup(self.cleanup, child)
        return child

    def cleanup(self, child):
        if child.stdin and not child.stdin.closed: child.stdin.close()
        if child.poll() is None:
            child.terminate()
            child.wait(timeout=12)
        for stream in (child.stdout, child.stderr):
            if stream: stream.close()

    def ready(self, child):
        line = child.stdout.readline()
        self.assertTrue(line.startswith("READY "), line)
        return int(line.split()[1])

    def gone(self, pid):
        with self.assertRaises(ProcessLookupError): os.kill(pid, 0)

    def test_natural_exit_preserves_output_and_code(self):
        child = self.start("print('inventory-output'); raise SystemExit(7)")
        self.assertEqual(child.wait(timeout=4), 7)
        self.assertEqual(child.stdout.read().strip(), "inventory-output")

    def test_eof_stops_only_owned_child(self):
        foreign = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(90)"])
        self.addCleanup(lambda: (foreign.terminate(), foreign.wait()))
        child = self.start("import os,time; print('READY', os.getpid()); time.sleep(90)")
        pid = self.ready(child)
        child.stdin.close()
        child.wait(timeout=4)
        self.gone(pid)
        self.assertIsNone(foreign.poll())

    def test_sigterm_stops_child(self):
        child = self.start("import os,time; print('READY', os.getpid()); time.sleep(90)")
        pid = self.ready(child)
        child.terminate()
        child.wait(timeout=4)
        self.gone(pid)

    def test_stubborn_child_has_bounded_shutdown(self):
        child = self.start("import os,time,signal; signal.signal(signal.SIGTERM, signal.SIG_IGN); print('READY', os.getpid()); time.sleep(90)")
        pid = self.ready(child)
        began = time.monotonic()
        child.stdin.close()
        self.assertEqual(child.wait(timeout=12), 128 + signal.SIGKILL)
        self.assertLess(time.monotonic() - began, 11)
        self.gone(pid)

    def test_utility_timeout(self):
        child = self.start("import os,time; print('READY', os.getpid()); time.sleep(90)", timeout=.3)
        pid = self.ready(child)
        self.assertEqual(child.wait(timeout=4), 124)
        self.gone(pid)

    def test_immediate_cancel_during_launch(self):
        for _ in range(12):
            child = self.start("import time; time.sleep(90)")
            child.stdin.close()
            child.wait(timeout=4)

    def test_force_killed_owner_closes_pipe(self):
        code = """
import subprocess,sys,time
p = subprocess.Popen([sys.argv[1], sys.executable, '-u', '-c', "import os,time; print('READY', os.getpid()); time.sleep(90)"], stdin=subprocess.PIPE)
time.sleep(90)
"""
        owner = subprocess.Popen([sys.executable, "-u", "-c", code, HOST], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.addCleanup(self.cleanup_owner, owner)
        line = owner.stdout.readline()
        self.assertTrue(line.startswith('READY '), line)
        pid = int(line.split()[1])
        owner.kill()
        owner.wait(timeout=4)
        # Supervisor retains these streams until both it and its child exit.
        owner.communicate(timeout=12)
        self.gone(pid)

    def cleanup_owner(self, owner):
        if owner.poll() is None: owner.kill(); owner.wait()
        owner.stdout.close(); owner.stderr.close()

if __name__ == '__main__': unittest.main(verbosity=2)
