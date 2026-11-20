from pathlib import Path
import subprocess
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[1]


def action(name):
    return yaml.safe_load((ROOT / f".github/actions/tests/{name}/action.yaml").read_text())


class APIActionsTest(unittest.TestCase):
    def test_changed_actions_have_valid_shell_syntax(self):
        for name in ("test-controller", "env-up", "api-tests", "reporting", "resolve-mcp-images", "mcp-evidence",
                     "choose-test-branch", "checkout-tyk-pro"):
            for step in action(name)["runs"]["steps"]:
                if "run" in step:
                    with self.subTest(action=name, step=step.get("name")):
                        result = subprocess.run(["bash", "-n"], input=step["run"], text=True, capture_output=True)
                        self.assertEqual(result.returncode, 0, result.stderr)

    def test_regular_environment_still_bootstraps_the_existing_ha_stack(self):
        regular, *mcp = action("env-up")["runs"]["steps"]
        self.assertEqual(regular["if"], "inputs.suite != 'mcp'")
        self.assertIn("./dash-bootstrap.sh", regular["run"])
        self.assertIn("pro-ha.yml", regular["run"])
        setup = next(step for step in mcp if step.get("id") == "mcp_env_up")
        self.assertNotIn("./dash-bootstrap.sh", setup["run"])
        self.assertNotIn("/scripts/run_mcp_v2_stack.py", setup["run"])
        self.assertNotIn("go build", setup["run"])
        self.assertIn("mcp.yml", setup["run"])
        self.assertIn("docker logs --follow", setup["run"])
        self.assertLess(setup["run"].index("phase dashboard"), setup["run"].index("phase bootstrap"))

    def test_mcp_runs_through_api_action_and_reporting_retains_only_sanitized_evidence(self):
        execute = action("api-tests")["runs"]["steps"][-1]
        self.assertEqual(execute["if"], "inputs.suite == 'mcp'")
        self.assertIn("mcp_api_stack.py execute", execute["run"])
        self.assertIn('--shard-count "$SHARD_COUNT"', execute["run"])
        steps = action("reporting")["runs"]["steps"]
        evidence = next(step for step in steps if "sanitized MCP" in step.get("name", ""))
        self.assertIn("-shard-${{ inputs.shard_index }}", evidence["with"]["name"])
        self.assertIn("test-inventory.json", evidence["with"]["path"])
        self.assertNotIn("runtime.log", evidence["with"]["path"])
        cleanup = steps[-1]
        self.assertIn("always()", cleanup["if"])
        self.assertIn("trap", cleanup["run"])
        self.assertIn("down --volumes", cleanup["run"])
        for step in steps:
            if "Docker logs for all" in step.get("name", "") or step.get("name") == "Upload compose logs":
                self.assertIn("inputs.suite != 'mcp'", step["if"])


if __name__ == "__main__":
    unittest.main()
