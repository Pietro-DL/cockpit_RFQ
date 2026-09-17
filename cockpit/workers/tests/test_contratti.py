"""L3 — contratti a due lati, metà Python (voce 5.5, test K1–K4).

L'altra metà è internal/api/contratti_test.go, che confronta i tipi Go con contracts/*.schema.json.
Quel confronto vale però solo se gli schemi su disco descrivono davvero i modelli pydantic di oggi:
se qualcuno modifica contratti.py e non rigenera, il Go continua a corrispondere a uno schema vecchio
e il test verde non dimostra più niente — anzi, copre il disallineamento invece di mostrarlo.

Qui si verifica quella premessa: rigenerare gli schemi non deve cambiare nessun file. È anche il modo
in cui il difetto si presenta a chi lavora, perché nessuno guarda i .schema.json a mano.
"""
from __future__ import annotations

import json
import os

from contratti import CONTRATTI

CARTELLA = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "contracts")


def _atteso(nome, modello):
    schema = modello.model_json_schema()
    schema["$id"] = f"cockpit/{nome}"
    return schema


def _su_disco(nome):
    with open(os.path.join(CARTELLA, f"{nome}.schema.json"), encoding="utf-8") as f:
        return json.load(f)


def test_gli_schemi_pubblicati_sono_quelli_dei_modelli_di_oggi():
    """Rigenerare non cambia niente: è la condizione perché il test Go significhi qualcosa."""
    diversi = []
    for nome, modello in CONTRATTI.items():
        if _su_disco(nome) != _atteso(nome, modello):
            diversi.append(nome)
    assert not diversi, (
        "questi schemi non corrispondono più ai modelli pydantic: "
        f"{', '.join(sorted(diversi))}. Rigenera con `python workers/genera_contratti.py` "
        "e rileggi il diff: se cambia un campo, cambia il contratto con il Go."
    )


def test_ogni_modello_ha_il_suo_file_e_viceversa():
    """Un modello senza file non viene confrontato con il Go; un file senza modello è un contratto
    che nessuno genera più e che resta indietro in silenzio."""
    su_disco = {
        f[: -len(".schema.json")]
        for f in os.listdir(CARTELLA)
        if f.endswith(".schema.json")
    }
    assert su_disco == set(CONTRATTI), (
        f"solo su disco: {sorted(su_disco - set(CONTRATTI))}; "
        f"solo in CONTRATTI: {sorted(set(CONTRATTI) - su_disco)}"
    )


def test_un_campo_sconosciuto_non_rompe_la_lettura():
    """K4, lato Python. Un server più nuovo del worker manda campi che il worker non conosce: deve
    ignorarli. È `extra="ignore"` in contratti.Base — una scelta, non un comportamento naturale."""
    job = CONTRATTI["job"].model_validate(
        {
            "job_id": 7,
            "tipo": "stage_allegato",
            "payload": {"allegato_id": "0e1d2c3b-4a59-4687-8765-0123456789ab"},
            "campo_del_futuro": {"annidato": [1, 2, 3]},
        }
    )
    assert job.job_id == 7
    assert job.tipo == "stage_allegato"
    assert not hasattr(job, "campo_del_futuro")


def test_gli_enum_del_contratto_sono_dichiarati():
    """K3, lato Python: i campi che il server valida contro un enum devono essere Literal, non str.
    Un `str` libero farebbe passare il valore sbagliato fino al database, dove diventa un errore SQL
    sull'elemento — cioè un messaggio scartato invece di un errore di programmazione visto subito."""
    attesi = {
        ("messaggio_in", "direzione"): {"entrata", "uscita"},
        ("claim_richiesta", "worker"): {"outlook", "analisi"},
        ("payload_crea_bozza", "tipo"): {
            "risposta", "rispondi_tutti", "inoltro", "nuovo", "sollecito",
        },
    }
    for (nome, campo), valori in attesi.items():
        prop = _su_disco(nome)["properties"][campo]
        assert "enum" in prop, f"{nome}.{campo} non dichiara un enum"
        assert set(prop["enum"]) == valori, f"{nome}.{campo}: {prop['enum']}"

    natura = _su_disco("messaggio_in")["$defs"]["AllegatoIn"]["properties"]["natura"]
    assert set(natura["enum"]) == {"file", "inline", "elemento_outlook", "collegamento"}
