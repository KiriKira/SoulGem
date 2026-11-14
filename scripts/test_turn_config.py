#!/usr/bin/env python3
"""Exercise the real shell entrypoint without opening a listener or logging secrets."""
import contextlib
import io
import os
from pathlib import Path
import runpy
import subprocess
import tempfile
import unittest
from unittest.mock import patch

ENTRY = Path(__file__).resolve().parents[1] / "deploy/turn/entrypoint.sh"

class TurnConfigurationTest(unittest.TestCase):
    def render(self, **overrides):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            # Isolate the private generated config as well as the fake executable.
            script = root / "entrypoint.sh"
            config = root / "turn.conf"
            script.write_text(ENTRY.read_text().replace("/tmp/gsm2sip-turn.conf", str(config)))
            binary = root / "turnserver"
            binary.write_text("#!/bin/sh\nexit 0\n")
            binary.chmod(0o700)
            env = dict(os.environ, PATH=str(root)+":"+os.environ["PATH"],
                       RTC_TURN_SHARED_SECRET="a"*64, TURN_PUBLIC_IP="203.0.113.1",
                       TURN_PRIVATE_IP="192.0.2.1", TURN_REALM="turn.example.com",
                       TURN_LISTEN_PORT="3478", TURN_RELAY_MIN_PORT="49160",
                       TURN_RELAY_MAX_PORT="49259", TURN_USER_QUOTA="",
                       TURN_TOTAL_QUOTA="")
            env.update(overrides)
            result = subprocess.run(["/bin/sh", str(script)], env=env, capture_output=True)
            return result.returncode, config.read_text() if config.exists() else ""

    def test_defaults_and_custom_ports(self):
        for overrides, expected in [({}, (3478,49160,49259)),
                ({"TURN_LISTEN_PORT":"13478", "TURN_RELAY_MIN_PORT":"52000",
                  "TURN_RELAY_MAX_PORT":"52099"}, (13478,52000,52099)),
                ({"TURN_LISTEN_PORT":"65535", "TURN_RELAY_MIN_PORT":"1",
                  "TURN_RELAY_MAX_PORT":"1"}, (65535,1,1))]:
            code, config = self.render(**overrides)
            self.assertEqual(code, 0)
            for name, value in zip(("listening-port","min-port","max-port"),expected):
                self.assertIn(f"{name}={value}\n", config)
            self.assertIn("no-tls\n", config)

    def test_invalid_config_never_written(self):
        cases = [{name:value} for name in ("TURN_LISTEN_PORT","TURN_RELAY_MIN_PORT","TURN_RELAY_MAX_PORT")
                 for value in ("0","-1","65536","99999999999999999999999","03478","1.5","3478\nno-auth")]
        cases += [{"TURN_RELAY_MIN_PORT":"52099","TURN_RELAY_MAX_PORT":"52000"},
                  {"TURN_LISTEN_PORT":"49160"}, {"TURN_LISTEN_PORT":"49259"}]
        for case in cases:
            with self.subTest(case=case):
                code, config = self.render(**case)
                self.assertEqual(code, 2)
                self.assertEqual(config, "")

    def test_quota_defaults_overrides_and_small_ranges(self):
        for overrides, expected in [({}, (16, 80)),
                ({"TURN_USER_QUOTA": "12", "TURN_TOTAL_QUOTA": "72"}, (12, 72)),
                ({"TURN_TOTAL_QUOTA": "8"}, (8, 8)),
                ({"TURN_RELAY_MAX_PORT": "49163"}, (4, 4)),
                ({"TURN_RELAY_MIN_PORT": "1", "TURN_RELAY_MAX_PORT": "1"}, (1, 1))]:
            with self.subTest(overrides=overrides):
                code, config = self.render(**overrides)
                self.assertEqual(code, 0)
                self.assertIn(f"user-quota={expected[0]}\n", config)
                self.assertIn(f"total-quota={expected[1]}\n", config)

    def test_invalid_quota_never_written(self):
        cases = [{name: value} for name in ("TURN_USER_QUOTA", "TURN_TOTAL_QUOTA")
                 for value in ("0", "-1", "01", "+16", "65536", "9" * 200,
                               "1.5", "16\n", "16\nno-auth")]
        cases += [{"TURN_TOTAL_QUOTA": "101"}, {"TURN_USER_QUOTA": "81"},
                  {"TURN_USER_QUOTA": "9", "TURN_TOTAL_QUOTA": "8"}]
        for case in cases:
            with self.subTest(case=case):
                code, config = self.render(**case)
                self.assertEqual(code, 2)
                self.assertEqual(config, "")


class AllInOneTurnConfigurationTest(TurnConfigurationTest):
    """Use the real Python renderer, intercepting all writes and network lookup."""
    def render(self, **overrides):
        env = dict(RTC_TURN_SHARED_SECRET="a" * 64, TURN_PUBLIC_IP="203.0.113.1",
                   TURN_PRIVATE_IP="192.0.2.1", TURN_REALM="turn.example.com")
        env.update(overrides)
        output = {}
        script = ENTRY.parents[1] / "allinone/turn-config.py"
        def record_write(path, text, *args, **kwargs):
            output[path.name] = text
            return len(text)
        with patch.dict(os.environ, env, clear=True), \
                patch("pathlib.Path.write_text", record_write), \
                patch("socket.gethostbyname", side_effect=lambda host: host), \
                patch("subprocess.run", side_effect=AssertionError("unexpected network discovery")), \
                patch("os.umask"), contextlib.redirect_stderr(io.StringIO()):
            try:
                runpy.run_path(str(script))
            except SystemExit as error:
                code = error.code if isinstance(error.code, int) else 1
                return code, output.get("turn.conf", "")
        return 0, output["turn.conf"]


if __name__ == "__main__":
    unittest.main()
