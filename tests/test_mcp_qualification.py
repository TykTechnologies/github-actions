"""Execute the workflow's revision resolver against a deterministic GitHub API."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import yaml

ROOT = Path(__file__).resolve().parents[1]
WORKFLOW = yaml.safe_load((ROOT / '.github/workflows/mcp-qualification.yml').read_text())
STEPS = WORKFLOW['jobs']['qualify']['steps']
RESOLVER = next(step['run'] for step in STEPS if step['name'] == 'Resolve coordinated source revisions')


class RevisionResolutionTest(unittest.TestCase):
    def resolve(self, caller, refs, topology='mdcb'):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            gh = root / 'gh'
            gh.write_text('''#!/usr/bin/env python3
import json, os, sys
value = json.loads(os.environ['FIXTURE_REFS']).get(sys.argv[2])
if not value: raise SystemExit(1)
print(value)
''')
            gh.chmod(0o700)
            output = root / 'output'
            env = {**os.environ, 'PATH': str(root) + os.pathsep + os.environ['PATH'],
                   'FIXTURE_REFS': json.dumps(refs), 'CALLER_REPOSITORY': caller,
                   'CALLER_SHA': 'a' * 40, 'SOURCE_BRANCH': 'TT-18691-mdcb-mcp-qualification',
                   'BASE_BRANCH': 'master', 'TOPOLOGY': topology, 'GITHUB_OUTPUT': str(output)}
            result = subprocess.run(['bash', '-c', RESOLVER], env=env, text=True, capture_output=True)
            revisions = dict(line.split('=', 1) for line in output.read_text().splitlines()) if output.exists() else {}
            return result, revisions

    def refs(self):
        refs = {}
        for repository in ('tyk', 'tyk-analytics', 'tyk-pump', 'tyk-sink'):
            refs[f'repos/TykTechnologies/{repository}/git/ref/heads/master'] = 'b' * 40
        return refs

    def test_sink_uses_exact_caller_head_and_dashboard_matching_branch(self):
        refs = self.refs()
        refs['repos/TykTechnologies/tyk-analytics/git/ref/heads/TT-18691-mdcb-mcp-qualification'] = 'c' * 40
        refs['repos/TykTechnologies/tyk-sink/git/ref/heads/TT-18691-mdcb-mcp-qualification'] = 'd' * 40
        result, revisions = self.resolve('TykTechnologies/tyk-sink', refs)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(revisions, {'sink': 'a' * 40, 'dashboard': 'c' * 40,
                                     'gateway': 'b' * 40, 'pump': 'b' * 40})

    def test_direct_preserves_three_component_resolution(self):
        result, revisions = self.resolve('TykTechnologies/tyk-pump', self.refs(), 'direct')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(revisions['pump'], 'a' * 40)
        self.assertNotIn('sink', revisions)

    def test_unresolvable_component_fails_instead_of_qualifying(self):
        result, _ = self.resolve('TykTechnologies/tyk-sink', {})
        self.assertNotEqual(result.returncode, 0)

    def test_matrix_has_exactly_four_serializer_storage_profiles(self):
        matrix = WORKFLOW['jobs']['qualify']['strategy']['matrix']
        self.assertEqual(matrix, {'serializer': ['msgpack', 'protobuf'], 'storage': ['mongo', 'postgres']})
        run = next(step['run'] for step in STEPS if step['name'] == 'Build current sources and run complete MCP coverage serially')
        self.assertIn('--sink-root mcp/sink', run)
        self.assertIn('--topology "$TOPOLOGY"', run)
        self.assertNotIn('forward_analytics_to_pump=true', run)


if __name__ == '__main__':
    unittest.main()
