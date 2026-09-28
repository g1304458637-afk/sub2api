"""The startup retry must survive curl transport failures under set -e."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('production-healthcheck.sh').resolve()


class HealthcheckTest(unittest.TestCase):
    def run_probe(self, recover):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            commands = {
                'docker': '''#!/bin/sh
n=0; test ! -f "$PROBE_STATE" || n=$(cat "$PROBE_STATE")
n=$((n+1)); echo "$n" > "$PROBE_STATE"
if [ "$n" -eq 1 ]; then echo 'true starting'; else echo 'true healthy'; fi
''',
                'curl': '''#!/bin/sh
if [ "$(cat "$PROBE_STATE")" -eq 1 ] || [ "$RECOVER" = no ]; then printf 000; exit 7; fi
case "$*" in
 *healthz*) case "$*" in *http_code*) printf 200;; *) printf '{"status":"ok","commit":"verified"}';; esac;;
 *connect-code*) printf 401;;
 *exchange*) printf 400;;
 *models*) printf 401;;
esac
''',
                'sleep': '#!/bin/sh\nexit 0\n',
            }
            for name, source in commands.items():
                path = root / name
                path.write_text(source)
                path.chmod(0o755)
            env = dict(os.environ, PATH=str(root) + os.pathsep + os.environ['PATH'], PROBE_STATE=str(root/'state'), RECOVER='yes' if recover else 'no')
            return subprocess.run(['bash', str(SCRIPT), '--expect-commit', 'verified', '--retries', '2', '--interval', '0'], env=env, text=True, capture_output=True)

    def test_connection_refusal_then_healthy_retries(self):
        result = self.run_probe(True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('第 2/2', result.stdout)
        self.assertIn('HEALTHCHECK PASS', result.stdout)

    def test_persistent_failure_exhausts_retries(self):
        result = self.run_probe(False)
        self.assertEqual(result.returncode, 1)
        self.assertIn('第 2/2', result.stdout)
        self.assertIn('HEALTHCHECK FAIL', result.stderr)


if __name__ == '__main__':
    unittest.main()
