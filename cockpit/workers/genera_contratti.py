"""Esporta i contratti pydantic in contracts/*.schema.json (fonte unica, versionata)."""
import json
import os

from contratti import CONTRATTI
from protocollo import TEMPI

dest = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "contracts")
os.makedirs(dest, exist_ok=True)
for nome, modello in CONTRATTI.items():
    schema = modello.model_json_schema()
    schema["$id"] = f"cockpit/{nome}"
    with open(os.path.join(dest, f"{nome}.schema.json"), "w", encoding="utf-8") as f:
        json.dump(schema, f, ensure_ascii=False, indent=2)
        f.write("\n")

# I tempi del protocollo non sono uno schema: sono numeri che le due parti devono condividere (blocco
# 2 del 3R). Viaggiano nella stessa cartella e per la stessa ragione — che un test dell'altra parte
# possa accorgersi della divergenza — ma fuori da *.schema.json, che e' il confronto dei CAMPI.
with open(os.path.join(dest, "tempi_protocollo.json"), "w", encoding="utf-8") as f:
    json.dump(TEMPI, f, ensure_ascii=False, indent=2, sort_keys=True)
    f.write("\n")

print(f"{len(CONTRATTI)} schemi + i tempi del protocollo scritti in {os.path.abspath(dest)}")
