"""Esporta i contratti pydantic in contracts/*.schema.json (fonte unica, versionata)."""
import json
import os

from contratti import CONTRATTI

dest = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "contracts")
os.makedirs(dest, exist_ok=True)
for nome, modello in CONTRATTI.items():
    schema = modello.model_json_schema()
    schema["$id"] = f"cockpit/{nome}"
    with open(os.path.join(dest, f"{nome}.schema.json"), "w", encoding="utf-8") as f:
        json.dump(schema, f, ensure_ascii=False, indent=2)
        f.write("\n")
print(f"{len(CONTRATTI)} schemi scritti in {os.path.abspath(dest)}")
