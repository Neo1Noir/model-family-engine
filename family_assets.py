"""Family identities and reviewed document migration, extracted from the service assets."""

import copy
import re


VER_PAREN_RX = re.compile(r"[（(][^（()）]*[)）]\s*$")

MODEL_B_RX = re.compile(r"^([A-Za-z]+)-")

MODEL_A_RX = re.compile(r"^([A-Za-z]+\d+)")


def strip_ver_paren(s: str) -> str:
    prev = None
    while prev != s:
        prev = s
        s = VER_PAREN_RX.sub("", s).strip()
    return s

def norm_model(s: str) -> str:
    return strip_ver_paren((s or "").strip())

def root_of(model: str) -> str:
    m = norm_model(model)
    mb = MODEL_B_RX.match(m)
    if mb:
        return mb.group(1)
    ma = MODEL_A_RX.match(m)
    if ma:
        return ma.group(1)
    return m

def in_family(model: str, root: str) -> bool:
    return re.search(r"(?<![A-Za-z0-9])" + re.escape(root) + r"(?![A-Za-z0-9])", model) is not None

def norm_ws(s: str) -> str:
    return re.sub(r"\s+", " ", (s or "").strip())

def parse_kv(line: str) -> dict:
    out = {}
    for part in line.split("；"):
        if "：" in part:
            k, v = part.split("：", 1)
            out[k.strip()] = v.strip()
    return out

def migrate_ecm_assets(baseline, manifest, selection_baseline, index_baseline):
    """Apply reviewed document mappings without rebuilding confirmed identities."""
    version = manifest["version"]
    family = copy.deepcopy(baseline)
    family["version"] = version
    for model, info in manifest.get("new_models", {}).items():
        if model in family["models"]:
            raise ValueError("New model already exists: " + model)
        fam = info["family"]
        role = info["role"]
        if fam not in family["families"]:
            if fam != model or role != "main":
                raise ValueError("A new family requires an explicitly registered main model")
            family["families"][fam] = {"main": model, "members": []}
        family["families"][fam]["members"].append(model)
        family["models"][model] = copy.deepcopy(info)
    for info in family["models"].values():
        info["docs"] = []
        info["has_doc"] = False
        info["doc_kind"] = "none"
    custom_docs = {}
    by_identity = {}
    selected = [x for x in manifest["documents"] if x.get("selected") is True]
    for row in selected:
        scope, order = row["scope"], row.get("custom_order", "")
        identity = (scope, order, row["name"])
        if identity in by_identity or scope not in {"standard", "custom"}:
            raise ValueError("Invalid or duplicate document identity: " + repr(identity))
        if (scope == "custom") != bool(order) or not row.get("models"):
            raise ValueError("Missing document scope, order or applicability")
        doc = {"name": row["name"], "type": row["doc_type"], "version": row["version"],
               "scope": scope, "custom_order": order, "catalog_product": row["catalog_product"],
               "language": row["language"], "generation": row.get("generation", ""),
               "protocol": row.get("protocol", ""), "sha256": row["sha256"],
               "source_ids": row["source_ids"]}
        by_identity[identity] = doc
        for model in row["models"]:
            if scope == "standard":
                if model not in family["models"]:
                    raise ValueError("Unregistered standard model: " + model)
                family["models"][model]["docs"].append(copy.deepcopy(doc))
            else:
                custom_docs.setdefault(order, {}).setdefault(model, []).append(copy.deepcopy(doc))
    for model, info in family["models"].items():
        info["has_doc"] = bool(info["docs"])
        old_kind = baseline["models"].get(model, {}).get("doc_kind", "own")
        info["doc_kind"] = (old_kind if old_kind in {"own", "family"} else "own") if info["has_doc"] else "none"
        info["coverage_status"] = "standard" if info["has_doc"] else "not_covered"
        orders = sorted(order for order, models in custom_docs.items() if model in models)
        if orders:
            info["custom_orders"] = orders
            if not info["has_doc"]:
                info["coverage_status"] = "custom_only"
        else:
            info.pop("custom_orders", None)
    for name, info in baseline["models"].items():
        for key in ("family", "role", "parent", "aliases", "discontinued"):
            if family["models"][name].get(key) != info.get(key):
                raise ValueError("Confirmed model identity changed: " + name)
    docs = {"version": version, "docs": {m: v["docs"] for m, v in family["models"].items()},
            "custom_docs": custom_docs}
    index_models = copy.deepcopy(index_baseline["models"])
    for model, info in family["models"].items():
        index_models.setdefault(model, {}).update({k: info[k] for k in
                                                  ("family", "role", "parent", "aliases", "discontinued", "coverage_status") if k in info})
    for model, info in index_baseline["models"].items():
        for key in ("family", "role", "parent", "aliases", "discontinued"):
            if index_models[model].get(key) != info.get(key):
                raise ValueError("Confirmed model index identity changed: " + model)
    index = {"version": version, "family_map_version": version,
             "models": index_models, "families": family["families"],
             "custom_models": {order: sorted(models) for order, models in custom_docs.items()}}
    selection = copy.deepcopy(selection_baseline)
    selection.update(version=version, schemas={}, custom_schemas={}, unavailable_schemas={}, schema_aliases=manifest.get("schema_aliases", {}))
    decisions = manifest["schema_decisions"]
    if set(decisions) != set(selection_baseline["schemas"]):
        raise ValueError("Every baseline selection schema requires an explicit decision")
    for model, original in selection_baseline["schemas"].items():
        decision = decisions[model]
        if decision["status"] == "unavailable":
            selection["unavailable_schemas"][model] = {"schema": copy.deepcopy(original), "reason": decision["reason"]}
            continue
        identity = tuple(decision["document_identity"])
        if identity not in by_identity:
            raise ValueError("Selection source is not in the selected document set: " + model)
        doc = by_identity[identity]
        applicable_models = next(row["models"] for row in selected
                                 if (row["scope"], row.get("custom_order", ""), row["name"]) == identity)
        aliases = manifest.get("schema_aliases", {})
        if model not in applicable_models and not any(aliases.get(name) == model for name in applicable_models):
            raise ValueError("Selection source does not apply to the reviewed model: " + model)
        schema = copy.deepcopy(original)
        schema.update(decision.get("overrides", {}))
        schema["doc"] = doc["name"]
        page_map = decision.get("page_map", {})
        def update_pages(value):
            if isinstance(value, dict):
                for key, child in value.items():
                    if key == "page" and str(child) in page_map:
                        value[key] = page_map[str(child)]
                    else:
                        update_pages(child)
            elif isinstance(value, list):
                for child in value:
                    update_pages(child)
        update_pages(schema)
        schema.update(scope=doc["scope"], custom_order=doc["custom_order"], source_sha256=doc["sha256"],
                      source_review=decision["evidence"])
        if decision["status"] == "standard" and doc["scope"] == "standard":
            selection["schemas"][model] = schema
        elif decision["status"] == "custom" and doc["scope"] == "custom":
            selection["custom_schemas"].setdefault(doc["custom_order"], {})[model] = schema
        else:
            raise ValueError("Selection schema scope does not match its reviewed source")
    return {"family_map.json": family, "models_index.json": index, "docs_map.json": docs,
            "selection_schema.json": selection, "selection_schema_env.json": copy.deepcopy(selection)}
