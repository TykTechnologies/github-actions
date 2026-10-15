import importlib.util
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location("api_matrix", ROOT / ".github/actions/tests/test-controller/api_matrix.py")
matrix = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(matrix)


class APIMatrixTest(unittest.TestCase):
    def envfiles(self):
        return [{"db": "mongo7", "config": "sha256", "cache": "redis7", "apimarkers": "not local"},
                {"db": "postgres15", "config": "murmur128", "cache": "redis7", "apimarkers": "not local and not sql"}]

    def test_preserves_compatibility_combinations_and_adds_exactly_sixteen_mcp_rows(self):
        pumps = ["tykio/tyk-pump-docker-pub:v1.8", "$ECR/tyk-pump:master"]
        sinks = ["tykio/tyk-mdcb-docker:v2.4", "$ECR/tyk-sink:master"]
        for repository in ("tyk", "tyk-analytics", "tyk-pump", "tyk-sink"):
            with self.subTest(repository=repository):
                rows = matrix.api_matrix(self.envfiles(), pumps, sinks, repository)
                api = [row for row in rows if row["envfiles"]["suite"] == "api"]
                mcp = [row for row in rows if row["envfiles"]["suite"] == "mcp"]
                self.assertEqual(len(api), 4)
                self.assertEqual(len(mcp), 16)
                self.assertTrue(all("and not mcp" in row["envfiles"]["apimarkers"] for row in api))
                self.assertEqual({(r["envfiles"]["serializer"], r["envfiles"]["storage"], r["envfiles"]["shard_index"]) for r in mcp},
                                 {(serializer, storage, index) for serializer in ("msgpack", "protobuf")
                                  for storage in ("mongo", "postgres") for index in range(4)})
                if repository == "tyk-pump":
                    self.assertTrue(all(not row["pump"] for row in api))
                if repository == "tyk-sink":
                    self.assertTrue(all(not row["sink"] for row in api))

    def test_sql_markers_and_mdcb_topology_are_preserved(self):
        rows = matrix.api_matrix(self.envfiles(), ["pump"], ["sink"], "tyk-sink", "mdcb")
        mcp = [row["envfiles"] for row in rows if row["envfiles"]["suite"] == "mcp"]
        self.assertTrue(all(row["topology"] == "mdcb" for row in mcp))
        self.assertTrue(all(("not sql" in row["apimarkers"]) == (row["storage"] == "postgres") for row in mcp))
        self.assertEqual(rows, matrix.api_matrix(self.envfiles(), ["pump"], ["sink"], "tyk-sink", "mdcb"))

    def test_non_pr_matrix_retains_existing_api_scope(self):
        rows = matrix.api_matrix(self.envfiles(), ["pump"], ["sink"], "tyk", include_mcp=False)
        self.assertEqual(len(rows), 2)
        self.assertTrue(all(row["envfiles"]["suite"] == "api" for row in rows))

    def test_invalid_inputs_fail_instead_of_losing_coverage(self):
        for envfiles, pumps, sinks, repository, topology, count in [
            ([], ["p"], ["s"], "tyk", "direct", 4),
            ([{}], ["p"], ["s"], "tyk", "direct", 4),
            (self.envfiles(), [], ["s"], "tyk", "direct", 4),
            (self.envfiles(), ["p"], ["s"], "portal", "direct", 4),
            (self.envfiles(), ["p"], ["s"], "tyk", "unknown", 4),
            (self.envfiles(), ["p"], ["s"], "tyk", "direct", 0),
            (self.envfiles(), ["p"], ["s"], "tyk", "direct", 100),
        ]:
            with self.assertRaises(ValueError):
                matrix.api_matrix(envfiles, pumps, sinks, repository, topology, count)

    def test_reads_existing_controller_output_format(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "output"
            path.write_text('envfiles<<EOF\n[{"db":"mongo7"}]\nEOF\npump=["pump"]\nsink<<END\n["sink"]\nEND\n')
            self.assertEqual(matrix.read_outputs(path), {"envfiles": '[{"db":"mongo7"}]', "pump": '["pump"]', "sink": '["sink"]'})
            path.write_text('envfiles<<EOF\n[]\n')
            with self.assertRaises(ValueError):
                matrix.read_outputs(path)


if __name__ == "__main__":
    unittest.main()
