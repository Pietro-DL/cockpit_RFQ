"""L1 — blocco 2 del 3R, lato worker: un worker dentro un job non deve sparire dalla testata.

Il difetto stava nell'incontro fra tre numeri che nessuno aveva mai messo uno accanto all'altro:

    il server chiamava offline chi taceva da        60 s
    il worker in attesa si faceva vivo ogni         20 s   (long-poll del claim)
    il worker DENTRO un job batteva ogni  lease_s / 4      → 150 s su un sync con lease da 10 minuti

Preso da solo nessuno dei tre era sbagliato. Insieme dicevano che un worker occupato e' spento.

Qui si prova la meta' Python della regola: dentro un job il claim non passa piu', quindi il battito e'
l'unica prova di vita, e non puo' essere piu' lento di quanto il server aspetta prima di dare il
worker per perso. L'altra meta' — che il server guardi l'ultimo contatto e non l'ultimo claim — sta in
internal/transport/web/presenza_db_test.go, contro PostgreSQL vero.
"""
import cockpit_client
import protocollo
from cockpit_client import Battito, cadenza_battito


def test_la_soglia_di_presenza_e_due_giri_di_claim():
    """Non e' un numero scelto: e' due attese di claim. Un giro perso si tollera, due sono un guasto."""
    assert protocollo.PRESENZA_ONLINE_ENTRO_S == 2 * protocollo.ATTESA_CLAIM_S


def test_il_battito_non_e_mai_piu_lento_di_quanto_il_server_aspetta():
    """Se il battito fosse piu' lento della soglia, ogni job lungo farebbe sparire il worker."""
    assert protocollo.BATTITO_MAX_S <= protocollo.PRESENZA_ONLINE_ENTRO_S
    # e con questo margine un battito perso non basta a far sparire nessuno
    assert 2 * protocollo.BATTITO_MAX_S <= protocollo.PRESENZA_ONLINE_ENTRO_S


def test_cadenza_battito_sta_sotto_il_tetto_anche_con_lease_lunghissimi():
    # sync_outlook ha il lease piu' lungo: e' il caso che si vedeva sul banco reale
    assert cadenza_battito(600) <= protocollo.BATTITO_MAX_S
    assert cadenza_battito(3600) <= protocollo.BATTITO_MAX_S
    # e resta legata al lease quando il lease e' corto: il battito serve prima di tutto a rinnovarlo
    assert cadenza_battito(40) == 10.0
    assert cadenza_battito(4) == 5.0        # mai sotto i 5 s: non si martella il server per niente


def test_il_tetto_lo_impone_battito_non_il_chiamante():
    """Il tetto sta in Battito, non nei chiamanti: un worker nuovo non deve ricordarselo.

    Il difetto era proprio un chiamante che sceglieva da se' — `ogni_s=max(5.0, lease_s / 4)` — senza
    sapere niente della soglia del server.
    """
    assert Battito(None, 7, "outlook@PC-A", "tok", ogni_s=150).ogni_s <= protocollo.BATTITO_MAX_S
    assert Battito(None, 7, "outlook@PC-A", "tok").ogni_s <= protocollo.BATTITO_MAX_S
    # piu' fitto resta piu' fitto: i test lo usano a 0,05 s e devono continuare a poterlo fare
    assert Battito(None, 7, "outlook@PC-A", "tok", ogni_s=0.05).ogni_s == 0.05


def test_il_claim_chiede_lattesa_del_protocollo(monkeypatch):
    """Il long-poll del claim e' il ritmo con cui un worker fermo si fa vivo: viene da protocollo.py."""
    visti = {}

    def finto(self, metodo, percorso, corpo=None, timeout=None, **kw):
        visti["corpo"] = corpo
        return None

    monkeypatch.setattr(cockpit_client.Cockpit, "chiama", finto)
    api = cockpit_client.Cockpit.__new__(cockpit_client.Cockpit)
    api.claim("outlook", "outlook@PC-A")
    assert visti["corpo"]["attesa_s"] == protocollo.ATTESA_CLAIM_S
