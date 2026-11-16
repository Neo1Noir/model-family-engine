import copy
import unittest

from family_assets import in_family, migrate_ecm_assets, norm_model, root_of


class FamilyAssetTests(unittest.TestCase):
    def setUp(self):
        self.family = {"version": "v1", "models": {
            "DEMO100": {"family": "DEMO100", "role": "main", "docs": [], "doc_kind": "own"},
            "DEMO100-A": {"family": "DEMO100", "role": "variant", "parent": "DEMO100", "docs": [], "doc_kind": "family"},
        }, "families": {"DEMO100": {"main": "DEMO100", "members": ["DEMO100", "DEMO100-A"]}}}
        self.index = {"models": copy.deepcopy(self.family["models"])}
        self.selection = {"schemas": {"DEMO100": {"doc": "old.pdf", "dims": [{"codes": {"A": {"page": "1"}}}]}}}
        self.doc = {"selected": True, "scope": "standard", "custom_order": "", "name": "demo.pdf", "models": ["DEMO100", "DEMO100-A"],
                    "doc_type": "manual", "version": "v2", "catalog_product": "DEMO100", "language": "zh", "sha256": "a" * 64, "source_ids": ["demo-source"]}
        self.manifest = {"version": "v2", "documents": [self.doc], "schema_decisions": {"DEMO100": {
            "status": "standard", "document_identity": ["standard", "", "demo.pdf"], "page_map": {"1": "3"}, "evidence": "synthetic review"}}}

    def run_migration(self):
        return migrate_ecm_assets(self.family, self.manifest, self.selection, self.index)

    def test_normalization_and_prefix_boundary(self):
        self.assertEqual(norm_model("DEMO100-A(V2)"), "DEMO100-A")
        self.assertEqual(root_of("DEMO100-A"), "DEMO100")
        self.assertTrue(in_family("DEMO100-A", "DEMO100"))
        self.assertFalse(in_family("DEMO1000", "DEMO100"))

    def test_reviewed_migration_preserves_identity_and_inputs(self):
        before = copy.deepcopy(self.family)
        result = self.run_migration()
        self.assertEqual(self.family, before)
        self.assertEqual(result["family_map.json"]["models"]["DEMO100-A"]["parent"], "DEMO100")
        self.assertEqual(result["selection_schema.json"]["schemas"]["DEMO100"]["dims"][0]["codes"]["A"]["page"], "3")
        self.assertEqual(result["models_index.json"]["version"], "v2")

    def test_custom_documents_never_enter_standard_scope(self):
        self.doc.update(scope="custom", custom_order="ORDER-DEMO")
        self.manifest["schema_decisions"]["DEMO100"].update(status="custom", document_identity=["custom", "ORDER-DEMO", "demo.pdf"])
        result = self.run_migration()
        self.assertEqual(result["docs_map.json"]["docs"]["DEMO100"], [])
        self.assertEqual(result["family_map.json"]["models"]["DEMO100"]["coverage_status"], "custom_only")
        self.assertIn("DEMO100", result["selection_schema.json"]["custom_schemas"]["ORDER-DEMO"])

    def test_duplicate_document_identity_rejected(self):
        self.manifest["documents"].append(copy.deepcopy(self.doc))
        with self.assertRaisesRegex(ValueError, "duplicate document"):
            self.run_migration()

    def test_missing_review_decision_rejected(self):
        self.manifest["schema_decisions"] = {}
        with self.assertRaisesRegex(ValueError, "explicit decision"):
            self.run_migration()

    def test_unrelated_schema_source_rejected(self):
        self.doc["models"] = ["DEMO100-A"]
        with self.assertRaisesRegex(ValueError, "does not apply"):
            self.run_migration()

    def test_new_family_requires_registered_main(self):
        self.manifest["new_models"] = {"OTHER-A": {"family": "OTHER", "role": "variant"}}
        with self.assertRaisesRegex(ValueError, "registered main"):
            self.run_migration()

    def test_unavailable_schema_retains_evidence(self):
        self.manifest["schema_decisions"]["DEMO100"] = {"status": "unavailable", "reason": "source withdrawn"}
        result = self.run_migration()["selection_schema.json"]
        self.assertEqual(result["schemas"], {})
        self.assertEqual(result["unavailable_schemas"]["DEMO100"]["reason"], "source withdrawn")


if __name__ == "__main__":
    unittest.main()
