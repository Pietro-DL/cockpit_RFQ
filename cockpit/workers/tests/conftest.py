"""I test dei worker stanno qui; il codice che provano sta nella cartella sopra.

Un file di test importa `cockpit_client`, `contratti`, `outlook_com`, `worker_analisi` e
`server_finto` per nome, come li importa il worker vero: sono moduli di una cartella, non un
pacchetto installato. Finché i test stavano accanto a loro bastava il comportamento predefinito di
pytest, che mette la cartella del file di test in `sys.path`. Da quando stanno in `tests/` quella
cartella è questa, e i moduli sono uno sopra: senza questa riga l'import fallirebbe — e fallirebbe
con un `ModuleNotFoundError` che sembra un problema di ambiente e non lo è.

`insert(0, ...)` e non `append`: se sul PC esistesse un pacchetto con uno di questi nomi, a vincere
deve essere il codice del repository, che è quello che i test devono provare.
"""
from __future__ import annotations

import os
import sys

WORKERS = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
if WORKERS not in sys.path:
    sys.path.insert(0, WORKERS)
