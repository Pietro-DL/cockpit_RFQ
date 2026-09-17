"""L2 — 3R blocco 1: che cosa fa il worker con gli elementi che non è riuscito a leggere.

La correzione dentro `outlook_com` fa sì che un elemento anomalo venga saltato invece di far cadere
la cartella (test_outlook_confine.py). Qui si prova l'altra metà, che è quella che si vede da fuori:

  * un elemento saltato non sparisce — viaggia nel lotto come scarto di lettura, e da lì il server lo
    rimette in coda per una rilettura mirata. Prima il campo `saltati` del contratto esisteva, il
    server lo sapeva gestire, e il worker non lo riempiva mai: un elemento non letto era un elemento
    che non era mai esistito;
  * arriva anche quando non c'è nessun messaggio da consegnare, che è proprio il caso in cui
    qualcuno lo sta cercando;
  * non viene consegnato due volte quando i lotti sono più di uno;
  * una cartella che non si riesce a leggere per intero resta un problema DI QUELLA cartella: le
    altre cartelle dello stesso job vengono lette lo stesso, e il job non fallisce.

Il server qui è quello finto: si prova che cosa il worker MANDA, non che cosa il server ne fa (quello
è L4, lotto_db_test.go).
"""
from __future__ import annotations

from datetime import datetime, timezone

import pytest

from server_finto import ServerFinto

worker_outlook = pytest.importorskip("worker_outlook", reason="serve pywin32 (solo su Windows)")
outlook_com = pytest.importorskip("outlook_com", reason="serve pywin32 (solo su Windows)")

from contratti import ElementoSaltato, MessaggioIn                      # noqa: E402

CASELLA = "11111111-1111-1111-1111-111111111111"
STORE = "STORE-FRANCESCO-LOCALE"
CASELLE_SERVITE = [
    {"casella_id": CASELLA, "indirizzo": "francesco@azienda.example", "nome": "Francesco", "condivisa": False},
]


def _messaggio(n: int, cartella="Inbox"):
    return MessaggioIn(
        message_id=f"<m{n}@prova>", entry_id=f"E{n}", store_id=STORE, cartella=cartella,
        direzione="entrata", data_evento=datetime(2026, 9, 15, 10, n, tzinfo=timezone.utc),
        ricevuto_il=datetime(2026, 9, 15, 10, n, tzinfo=timezone.utc), oggetto=f"prova {n}",
    )


def _job(cartelle=("Inbox",), lotto=50) -> dict:
    return {
        "job_id": 30, "tipo": "sync_outlook", "tentativi": 1, "lease_s": 120,
        "lease_token": "tok-30", "durata_max_s": 1800,
        "payload": {
            "casella_id": CASELLA,
            "cartelle": [{"cartella": c, "ultimo_received": None} for c in cartelle],
            "dal": "2026-09-14T00:00:00Z", "sovrapposizione_s": 600, "lotto": lotto,
        },
    }


class OutlookConSaltati:
    """L'adattatore COM finto: consegna i messaggi che gli si danno e registra gli elementi saltati
    nello stesso `Saltati` che il worker gli passa, come fa `leggi` vera."""

    def __init__(self, per_cartella: dict, rotte: tuple = ()):
        self.per_cartella = per_cartella          # cartella -> (messaggi, quanti saltati)
        self.rotte = set(rotte)                   # cartelle che non si riescono a leggere
        self.lette: list[str] = []

    def risolvi_caselle(self, caselle):
        return {str(c["casella_id"]): STORE for c in caselle}, []

    def leggi(self, cartella, dal, al=None, store_id="", saltati=None):
        self.lette.append(cartella)
        if cartella in self.rotte:
            raise outlook_com.LetturaIncompleta("%s: enumerazione interrotta" % cartella)
        messaggi, n_saltati = self.per_cartella.get(cartella, ([], 0))
        for i in range(n_saltati):
            saltati.aggiungi(ElementoSaltato(entry_id="X%d-%s" % (i, cartella), cartella=cartella,
                                             errore="lettura: elemento non-mail (Class=46)"))
        for m in messaggi:
            yield m


def _esegui(s, tmp_path, monkeypatch, finto, job):
    s.caselle_worker = CASELLE_SERVITE
    s.metti_job(job)
    w = worker_outlook.Worker(s.config(staging=str(tmp_path)))
    monkeypatch.setattr(w, "ol", lambda: finto)
    w.esegui_per_sempre(una_volta=True)
    return w


def test_gli_elementi_saltati_viaggiano_nel_lotto(tmp_path, monkeypatch):
    with ServerFinto() as s:
        finto = OutlookConSaltati({"Inbox": ([_messaggio(1), _messaggio(2)], 1)})
        _esegui(s, tmp_path, monkeypatch, finto, _job())

        assert len(s.lotti) == 1, s.lotti
        saltati = s.lotti[0].get("saltati") or []
        assert len(saltati) == 1, "l'elemento saltato non è arrivato al server: nessuno saprà che esiste"
        assert saltati[0]["entry_id"] == "X0-Inbox"
        assert saltati[0]["cartella"] == "Inbox"
        assert "Class=46" in saltati[0]["errore"]


def test_gli_elementi_saltati_arrivano_anche_senza_nessun_messaggio(tmp_path, monkeypatch):
    """È il caso in cui contano di più: la cartella non ha portato niente, e la ragione è questa."""
    with ServerFinto() as s:
        finto = OutlookConSaltati({"Inbox": ([], 2)})
        _esegui(s, tmp_path, monkeypatch, finto, _job())

        assert len(s.lotti) == 1, "un lotto senza messaggi ma con scarti non è stato mandato"
        assert s.lotti[0]["messaggi"] == []
        assert [x["entry_id"] for x in s.lotti[0]["saltati"]] == ["X0-Inbox", "X1-Inbox"]


def test_un_elemento_saltato_non_viene_consegnato_due_volte(tmp_path, monkeypatch):
    """Con più lotti gli scarti partono col primo: rimandarli col secondo li scriverebbe due volte."""
    with ServerFinto() as s:
        finto = OutlookConSaltati({"Inbox": ([_messaggio(i) for i in range(1, 6)], 1)})
        _esegui(s, tmp_path, monkeypatch, finto, _job(lotto=2))

        assert len(s.lotti) == 3, [len(l["messaggi"]) for l in s.lotti]
        quanti = [len(l.get("saltati") or []) for l in s.lotti]
        assert sum(quanti) == 1, f"lo stesso scarto è partito {sum(quanti)} volte: {quanti}"


def test_il_conto_dei_saltati_arriva_nel_risultato_del_job(tmp_path, monkeypatch):
    """Il conto sta dove lo cerca chi guarda il job, non solo nel log del worker."""
    with ServerFinto() as s:
        finto = OutlookConSaltati({"Inbox": ([_messaggio(1)], 3)})
        _esegui(s, tmp_path, monkeypatch, finto, _job())

        cartelle = s.risultati[30]["dati"]["cartelle"]
        assert cartelle[0]["saltati"] == 3, cartelle[0]
        assert cartelle[0]["n_messaggi"] == 1


def test_una_cartella_illeggibile_non_porta_con_se_le_altre(tmp_path, monkeypatch):
    """Il difetto era questo: l'eccezione usciva dal ciclo delle cartelle e faceva fallire il job.

    Le cartelle dopo quella rotta non venivano nemmeno provate — e su una casella con Posta in
    arrivo e Posta inviata bastava un rapporto di consegna nella seconda per non sincronizzare più
    né l'una né l'altra.
    """
    with ServerFinto() as s:
        finto = OutlookConSaltati({"Posta inviata": ([_messaggio(9, "Posta inviata")], 0)},
                                  rotte=("Inbox",))
        _esegui(s, tmp_path, monkeypatch, finto, _job(cartelle=("Inbox", "Posta inviata")))

        assert finto.lette == ["Inbox", "Posta inviata"], "la seconda cartella non è stata nemmeno letta"
        assert s.risultati[30]["esito"] == "ok", s.risultati[30]
        cartelle = {c["cartella"]: c for c in s.risultati[30]["dati"]["cartelle"]}
        assert "LetturaIncompleta" in cartelle["Inbox"]["errore"]
        assert cartelle["Inbox"]["n_messaggi"] == 0
        assert cartelle["Posta inviata"]["n_messaggi"] == 1


def test_la_cartella_rotta_non_fa_avanzare_il_cursore(tmp_path, monkeypatch):
    """Una lettura interrotta a metà non è una finestra letta: il cursore resta dov'era e la
    finestra si rilegge. Avanzarlo vorrebbe dire dichiarare letto un pezzo che nessuno ha guardato."""
    with ServerFinto() as s:
        finto = OutlookConSaltati({}, rotte=("Inbox",))
        _esegui(s, tmp_path, monkeypatch, finto, _job())

        assert s.lotti == [], "una cartella non letta ha comunque mandato un lotto"
        cartelle = {c["cartella"]: c for c in s.risultati[30]["dati"]["cartelle"]}
        assert cartelle["Inbox"]["ultimo_received"] is None
