#!/usr/bin/env python3
"""Smoke-test the real all-in-one image. Uses isolated disposable data volumes."""
import json
import os
import re
import subprocess
import time
import unittest
import urllib.parse
import urllib.request
import uuid

IMAGE = os.environ.get("ALLINONE_TEST_IMAGE", "soulgem-relay-allinone:test")

def docker(*args):
    return subprocess.check_output(["docker", *args], text=True, stderr=subprocess.DEVNULL)

class BuiltinTurnTest(unittest.TestCase):
    def scenario(self, enabled=None, external=False):
        name = "turn-source-test-" + uuid.uuid4().hex[:10]
        args = ["run", "-d", "--name", name, "-p", "127.0.0.1::8080", "-e", "TURN_PUBLIC_IP=192.0.2.10"]
        if enabled is not None:
            args += ["-e", "BUILTIN_TURN_ENABLED=" + enabled]
        if external:
            args += ["-e", "RTC_TURN_URLS=turn:relay.invalid:49100?transport=udp", "-e", "RTC_TURN_SHARED_SECRET=" + "t"*48]
        docker(*args, IMAGE)
        try:
            port = json.loads(docker("inspect", name))[0]["NetworkSettings"]["Ports"]["8080/tcp"][0]["HostPort"]
            base = "http://127.0.0.1:" + port
            for _ in range(60):
                try:
                    with urllib.request.urlopen(base+"/readyz", timeout=1) as response:
                        if response.status == 200:
                            break
                except Exception:
                    time.sleep(0.5)
            else:
                self.fail("all-in-one did not become ready")
            docker("exec", name, "/usr/local/bin/gsm2sip-allinone-healthcheck")
            processes = docker("exec", name, "sh", "-c", 'if [ -s /run/gsm2sip/turn-supervisor.pid ]; then read -r pid < /run/gsm2sip/turn-supervisor.pid; kill -0 "$pid" && cat "/proc/$pid/comm"; fi')
            self.assertEqual("turnserver" in processes, enabled != "false" and not external)
            with urllib.request.urlopen(base+"/v1/capabilities") as response:
                self.assertTrue(json.load(response)["ice_servers_v1"])
            output = docker("exec", name, "mahogsm-admin", "join", "create", "--url", base)
            link = re.search(r"mahogsm://join[^\s]+", output).group()
            entry = urllib.parse.parse_qs(urllib.parse.urlsplit(link).query)["n"][0]
            code = entry.rsplit("#",1)[1]
            data = json.dumps({"pairing_code":code,"role":"client","device_name":"smoke","platform":"android"}).encode()
            request = urllib.request.Request(base+"/v1/pairings/claim", data, {"Content-Type":"application/json"})
            with urllib.request.urlopen(request) as response:
                token = json.load(response)["access_token"]
            request = urllib.request.Request(base+"/v1/rtc/ice-servers", headers={"Authorization":"Bearer "+token})
            with urllib.request.urlopen(request) as response:
                self.assertEqual(response.headers["Cache-Control"], "no-store")
                servers = json.load(response)["ice_servers"]
            if enabled == "false" and not external:
                self.assertEqual(servers, [])
                docker("restart", name)
                # The runtime PID marker must stay absent after a restart too.
                self.assertEqual(docker("exec", name, "sh", "-c", "test ! -e /run/gsm2sip/turn-supervisor.pid && echo absent").strip(), "absent")
            else:
                self.assertTrue(any("49100" in url if external else "3478" in url for server in servers for url in server["urls"]))
        finally:
            subprocess.run(["docker","rm","-f","-v",name], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    def test_default_starts_builtin_turn(self): self.scenario()
    def test_explicit_false_keeps_signaling_and_omits_turn(self): self.scenario("false")
    def test_external_turn_omits_builtin(self): self.scenario("true", True)

if __name__ == "__main__": unittest.main()
