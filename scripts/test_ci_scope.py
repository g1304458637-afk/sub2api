import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("ci_scope", Path(__file__).with_name("ci-scope.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ScopeTest(unittest.TestCase):
    def test_independent_application_scopes(self):
        self.assertEqual(module.classify(["frontend/src/App.vue"]), {"frontend", "build"})
        self.assertEqual(module.classify(["backend/go.sum"]), {"backend", "build"})
        self.assertEqual(module.classify(["frontend/a.ts", "backend/a.go"]), {"frontend", "backend", "build"})

    def test_runtime_dependencies_are_not_documentation(self):
        self.assertEqual(module.classify(["docs/legal/privacy.md"]), {"frontend", "build"})
        self.assertEqual(module.classify(["deploy/Dockerfile"]), {"build", "operations"})
        self.assertEqual(module.classify(["backend/migrations/243.sql"]), {"backend", "build"})

    def test_operations_and_docs(self):
        self.assertEqual(module.classify(["scripts/production-deploy.sh", ".github/workflows/ci.yml"]), {"operations"})
        self.assertEqual(module.classify(["README.md", "docs/guide.md"]), set())

    def test_unknown_shared_input_runs_all(self):
        self.assertEqual(module.classify(["Makefile"]), {"frontend", "backend", "build", "operations"})


if __name__ == "__main__":
    unittest.main()
