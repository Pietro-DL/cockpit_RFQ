"""L2 — 3R blocco 1: un elemento anomalo non ferma la sincronizzazione di una cartella.

Il difetto, visto sulla Posta inviata vera:

    AttributeError: GetNext.ReceivedTime

`_lineari` leggeva `item.ReceivedTime` PRIMA che qualcuno guardasse `item.Class`, e proteggeva
quella lettura dal solo `com_error`. Un elemento non-mail — un rapporto di consegna, un
appuntamento, un elemento di Sync Issues — alza invece `AttributeError`, che usciva dalla scansione,
attraversava `leggi`, faceva cadere il self-test di Restrict (che chiama la stessa scansione) e
portava con sé il sync dell'INTERA cartella. Per un elemento che non sarebbe mai entrato comunque.

Che cosa questi test provano:

  * mail, elemento anomalo, mail → le due mail arrivano e l'elemento anomalo viene saltato, sia
    nella scansione lineare sia nella lettura completa;
  * l'errore riprodotto è quello vero, con la stessa forma del messaggio;
  * un `com_error` su una proprietà vale come l'`AttributeError`: si salta quell'elemento, non la
    cartella;
  * `Class` illeggibile NON fa scartare una mail: «non lo so» non è «non è una mail»;
  * l'elemento saltato non sparisce in silenzio: diventa uno scarto di lettura con cartella, Class
    ed EntryID, e chi non ha nemmeno un EntryID viene comunque contato;
  * il self-test di Restrict sopravvive all'elemento anomalo — è il punto in cui il difetto faceva
    più danno, perché fermava la cartella prima ancora di leggerla;
  * l'esito del self-test si ricorda fra un riavvio e l'altro, e la seconda volta non costa niente.

Che cosa NON provano: che Outlook si comporti come questa cartella finta. Quello è L5, e si fa sulla
Posta inviata che oggi produce l'errore, senza spostare né cancellare l'elemento che lo produce.
"""
from __future__ import annotations

import json
from datetime import datetime, timedelta, timezone

import pytest

outlook_com = pytest.importorskip("outlook_com", reason="serve pywin32 (solo su Windows)")

from finti_outlook import (CartellaFinta, ElementoNonMail, ElementoSenzaClasse, ElementoSordo,  # noqa: E402
                           ItemsFinti, MailFinta, com_error)

BASE = datetime(2026, 9, 15, 8, 0, tzinfo=timezone.utc)


def _outlook(usa_restrict=False, autoprova_ore=0, memoria=None, postazione="BANCO", cart=None):
    """L'adattatore senza COM. `cart` rende risolvibile `cartella()`, che qui non è in prova."""
    o = object.__new__(outlook_com.Outlook)
    o.usa_restrict = usa_restrict
    o.autoprova_ore = autoprova_ore
    o.restrict_ok = {}
    o.memoria = memoria
    o.postazione = postazione
    o.indirizzi_propri = set()
    o.ns = None                       # la Posta inviata non si risolve: il codice lo prevede
    if cart is not None:
        o.cartella = lambda nome, store_id="": cart
        o._store = lambda store_id: None
    return o


def _tre_elementi():
    """Mail, elemento anomalo, mail: la sequenza chiesta dal blocco 1."""
    return [
        MailFinta("E001", BASE, oggetto="richiesta di offerta"),
        ElementoNonMail("E002", ricevuto=BASE + timedelta(minutes=30)),
        MailFinta("E003", BASE + timedelta(hours=1), oggetto="risposta"),
    ]


# ---------------------------------------------------------------- l'errore riprodotto

def test_l_errore_riprodotto_e_quello_visto_sulla_posta_inviata():
    """Se questo cambia, i test qui sotto stanno provando un altro difetto."""
    with pytest.raises(AttributeError) as e:
        ElementoNonMail("E002").ReceivedTime
    assert "GetNext.ReceivedTime" in str(e.value)


# ---------------------------------------------------------------- la scansione lineare

def test_un_elemento_senza_received_time_non_ferma_la_scansione():
    cart = CartellaFinta(_tre_elementi())
    saltati = outlook_com.Saltati()
    letti = [e.EntryID for e in _outlook()._lineari(cart, BASE - timedelta(hours=1), None, saltati)]
    assert letti == ["E003", "E001"], letti        # dal piu’ recente al piu’ vecchio (blocco 3)
    assert saltati.totale == 1
    assert saltati.elementi == [], "un elemento non-mail non è da rileggere: in scarto non ci va"


def test_un_com_error_su_una_proprieta_salta_solo_quell_elemento():
    cart = CartellaFinta([
        MailFinta("E001", BASE),
        ElementoSordo("E002", BASE + timedelta(minutes=30)),
        MailFinta("E003", BASE + timedelta(hours=1)),
    ])
    saltati = outlook_com.Saltati()
    letti = [e.EntryID for e in _outlook()._lineari(cart, BASE - timedelta(hours=1), None, saltati)]
    assert letti == ["E003", "E001"], letti
    assert saltati.totale == 1


def test_una_classe_illeggibile_non_fa_scartare_una_mail():
    """«Class non letta» vuol dire «non lo so»: scartare per il dubbio perderebbe posta vera."""
    cart = CartellaFinta([ElementoSenzaClasse("E001", BASE), MailFinta("E002", BASE + timedelta(hours=1))])
    saltati = outlook_com.Saltati()
    letti = [e.EntryID for e in _outlook()._lineari(cart, BASE - timedelta(hours=1), None, saltati)]
    assert letti == ["E002", "E001"], letti
    assert saltati.totale == 0


def test_lo_scarto_porta_cartella_class_ed_entryid():
    """Lo scarto non è silenzioso: senza queste tre cose l'elemento non si ritrova in Outlook."""
    cart = CartellaFinta([MailFinta("E001", BASE), ElementoSordo("E002", BASE + timedelta(minutes=30))],
                         nome="Posta in arrivo")
    saltati = outlook_com.Saltati()
    list(_outlook()._lineari(cart, BASE - timedelta(hours=1), None, saltati))
    s = saltati.elementi[0]
    assert s.entry_id == "E002"
    assert s.cartella == "Posta in arrivo"
    assert "Class=43" in s.errore, s.errore
    assert "ReceivedTime" in s.errore


def test_una_mail_illeggibile_va_in_scarto_un_non_mail_no():
    """La distinzione che viene dal profilo vero.

    Nella Posta in arrivo di questa azienda ci sono rapporti di consegna (Class=46), inviti (53) e
    appuntamenti (26) salvati fra la posta: elementi che non entreranno MAI, per quante volte li si
    rilegga. Metterli in scarto riempirebbe /admin/scarti di righe che nessuno può chiudere. Una
    mail che oggi non si è riusciti a leggere è un'altra cosa: quella va in scarto, perché una
    rilettura mirata può farcela.
    """
    cart = CartellaFinta([
        MailFinta("E001", BASE),
        ElementoNonMail("E002", ricevuto=BASE + timedelta(minutes=10)),      # rapporto di consegna
        ElementoSordo("E003", BASE + timedelta(minutes=20)),                 # mail che non si legge
    ])
    saltati = outlook_com.Saltati()
    letti = [e.EntryID for e in _outlook()._lineari(cart, BASE - timedelta(hours=1), None, saltati)]
    assert letti == ["E001"]
    assert saltati.totale == 2, "tutti e due vanno contati"
    assert [s.entry_id for s in saltati.elementi] == ["E003"], "in scarto ci va solo ciò che ha senso rileggere"


def test_chi_non_ha_nemmeno_un_entryid_viene_contato_lo_stesso():
    """Il server rifiuta uno scarto senza EntryID (non saprebbe rileggerlo): resta il conteggio, e
    dire «zero saltati» sarebbe falso."""
    class MailSenzaNiente(ElementoSordo):
        """Una mail (Class=43) che non dà né la data né l'EntryID: sarebbe da rileggere, ma non si sa
        quale rileggere."""

        def __init__(self, ricevuto):
            self.Class = 43
            self._utc = ricevuto

        @property
        def EntryID(self):
            raise com_error("EntryID non disponibile")

    cart = CartellaFinta([MailFinta("E001", BASE), MailSenzaNiente(BASE + timedelta(minutes=1))])
    saltati = outlook_com.Saltati()
    letti = [e.EntryID for e in _outlook()._lineari(cart, BASE - timedelta(hours=1), None, saltati)]
    assert letti == ["E001"]
    assert saltati.totale == 1
    assert saltati.elementi == [], "senza EntryID non si può mandare al server, ma si deve contare"


# ---------------------------------------------------------------- la lettura completa

def test_la_lettura_completa_consegna_le_due_mail_e_registra_l_anomalo():
    """Il caso del blocco 1 dall'inizio alla fine: `leggi` non alza e consegna quello che c'è."""
    cart = CartellaFinta(_tre_elementi(), nome="Posta inviata")
    saltati = outlook_com.Saltati()
    o = _outlook(cart=cart)
    messaggi = list(o.leggi("Posta inviata", BASE - timedelta(hours=1), saltati=saltati))
    assert [m.entry_id for m in messaggi] == ["E003", "E001"]
    assert [m.oggetto for m in messaggi] == ["risposta", "richiesta di offerta"]
    assert saltati.totale == 1


def test_un_elemento_non_mail_che_restrict_restituisce_viene_saltato_alla_lettura():
    """Con Restrict la classe non la filtra nessuno: il controllo deve esserci anche lì.

    E deve esserci PRIMA della conversione, non dopo. Senza, l'elemento verrebbe scartato lo stesso
    — ma come «conversione non riuscita», cioè come un guasto invece che come un elemento che non è
    posta. È la riga che l'operatore legge in /admin/scarti per decidere se c'è qualcosa da capire.
    """
    cart = CartellaFinta(_tre_elementi())
    saltati = outlook_com.Saltati()
    o = _outlook(usa_restrict=True, cart=cart)
    o.restrict_ok[(cart.StoreID, cart.EntryID)] = True        # self-test già passato
    messaggi = list(o.leggi("Posta in arrivo", BASE - timedelta(hours=1), saltati=saltati))
    assert [m.entry_id for m in messaggi] == ["E003", "E001"]
    assert saltati.totale == 1
    assert saltati.elementi == [], (
        "l'elemento non-mail è stato messo in scarto: vuol dire che è arrivato fino alla conversione, "
        "ed è finito in /admin/scarti come un guasto invece che come un elemento che non è posta")


# ---------------------------------------------------------------- il self-test non deve cadere

def test_il_self_test_di_restrict_sopravvive_a_un_elemento_anomalo():
    """Il punto in cui il difetto faceva più danno.

    Il self-test confronta gli insiemi dei due modi, e per farlo chiama la scansione lineare: prima
    del 3R l'`AttributeError` cadeva qui, cioè PRIMA che la cartella venisse letta. Non si perdeva
    un elemento: si perdeva la cartella.
    """
    cart = CartellaFinta(_tre_elementi())
    o = _outlook(usa_restrict=True)
    assert o.autoprova_restrict(cart, BASE - timedelta(hours=1), None) is True
    assert o._restrict_affidabile(cart, BASE - timedelta(hours=1), None) is True


def test_l_elemento_non_mail_conta_come_in_piu_non_come_mancante():
    """Restrict lo restituisce, la lineare no: è la differenza innocua, non quella grave."""
    cart = CartellaFinta(_tre_elementi())
    m = _outlook().misura_finestra(cart, BASE - timedelta(hours=1))
    ok, mancanti, in_piu = outlook_com.confronta_insiemi(m["restrict"], m["lineare"])
    assert ok and not mancanti
    assert in_piu == ["E002"]


# ---------------------------------------------------------------- l'enumerazione che si rompe

def test_un_enumerazione_rotta_alza_sempre_anche_dopo_aver_consegnato():
    """Blocco 3. Prima la garanzia era «non consegna niente»: la scansione decrescente raccoglieva
    tutto in memoria, e un'interruzione buttava via il raccolto perche’ consegnarlo avrebbe portato
    il cursore oltre messaggi mai guardati.

    Adesso i lotti partono man mano — il lavoro fatto e’ lavoro fatto, e la deduplica lo assorbe — e
    la garanzia si e’ spostata: l'interruzione ALZA, sempre. Quello che non deve succedere non e’
    che dei messaggi arrivino, e’ che la finestra risulti conclusa avendone letta solo la cima.
    """
    elementi = [MailFinta("E%03d" % i, BASE + timedelta(hours=i)) for i in range(6)]
    cart = CartellaFinta(elementi, rompe_dopo=2)
    letti = []
    with pytest.raises(outlook_com.LetturaIncompleta):
        for m in _outlook()._lineari(cart, BASE - timedelta(hours=1), None):
            letti.append(m.EntryID)
    assert letti == ["E005", "E004"], "i due piu’ recenti erano gia’ usciti: quelli si tengono"


def test_un_enumerazione_rotta_alza_anche_nel_modo_con_restrict():
    """Il caso lasciato aperto dal revisore al blocco 1, chiuso qui.

    Nel modo con Restrict l'interruzione veniva registrata nel log e basta: si chiudeva
    l'enumerazione come se fosse finita. Era sicuro finche’ si leggeva in ordine crescente — cio’ che
    restava fuori era la parte NUOVA, che il cursore non aveva superato. In ordine decrescente cio’
    che resta fuori e’ la parte VECCHIA, e sopra passa il limite superiore della finestra: se
    qualcuno dichiarasse conclusa quella finestra, coprirebbe anche cio’ che nessuno ha guardato.
    """
    elementi = [MailFinta("E%03d" % i, BASE + timedelta(hours=i)) for i in range(6)]
    items = ItemsFinti(elementi, rompe_dopo=3)
    letti = []
    with pytest.raises(outlook_com.LetturaIncompleta):
        for e in outlook_com._scorri(items, dove="Posta in arrivo"):
            letti.append(e.EntryID)
    assert letti == ["E000", "E001", "E002"], "cio’ che era gia’ uscito resta uscito"


# ---------------------------------------------------------------- la memoria del self-test (1.D)

def test_il_self_test_non_si_rifa_se_l_esito_e_ancora_valido(tmp_path):
    """La prova costa una scansione lineare, cioè ciò che Restrict serve a evitare: rifarla a ogni
    riavvio su ogni cartella era un costo pagato per riscoprire una cosa che non era cambiata."""
    memoria = outlook_com.MemoriaRestrict(str(tmp_path / "restrict.json"))
    cart = CartellaFinta([MailFinta("E%03d" % i, BASE + timedelta(hours=i)) for i in range(5)])
    dal = BASE - timedelta(hours=1)

    primo = _outlook(usa_restrict=True, memoria=memoria)
    assert primo._restrict_affidabile(cart, dal, None) is True
    costo_prima_volta = cart.aperture
    assert costo_prima_volta >= 2, "la prova deve aver scorso la cartella in tutti e due i modi"

    # un altro processo: il dizionario in memoria è vuoto, il file no
    secondo = _outlook(usa_restrict=True, memoria=outlook_com.MemoriaRestrict(str(tmp_path / "restrict.json")))
    assert secondo._restrict_affidabile(cart, dal, None) is True
    assert cart.aperture == costo_prima_volta, "il self-test è stato rifatto senza motivo"


def test_anche_un_esito_negativo_si_ricorda(tmp_path):
    """«Su questa cartella Restrict perde roba» costa da scoprire quanto il contrario."""
    percorso = str(tmp_path / "restrict.json")
    cart = CartellaFinta([MailFinta("E%03d" % i, BASE + timedelta(hours=i)) for i in range(5)], perde={"E002"})
    dal = BASE - timedelta(hours=1)

    assert _outlook(usa_restrict=True, memoria=outlook_com.MemoriaRestrict(percorso))._restrict_affidabile(cart, dal, None) is False
    aperture = cart.aperture
    assert _outlook(usa_restrict=True, memoria=outlook_com.MemoriaRestrict(percorso))._restrict_affidabile(cart, dal, None) is False
    assert cart.aperture == aperture


def test_un_esito_scaduto_si_riprova(tmp_path):
    percorso = str(tmp_path / "restrict.json")
    outlook_com.MemoriaRestrict(percorso).scrivi("BANCO", "store-finto", "cartella-finta", True)

    # si invecchia l'esito di un mese: il file è la memoria, la data dentro è ciò che la fa scadere
    dati = json.loads(open(percorso, encoding="utf-8").read())
    voce = list(dati["cartelle"].values())[0]
    voce["quando"] = (datetime.now(timezone.utc) - timedelta(days=30)).isoformat(timespec="seconds")
    open(percorso, "w", encoding="utf-8").write(json.dumps(dati))

    memoria = outlook_com.MemoriaRestrict(percorso, valide_ore=168)
    assert memoria.leggi("BANCO", "store-finto", "cartella-finta") is None, "un esito di un mese fa non è una prova di oggi"


def test_un_esito_non_concludente_non_si_memorizza(tmp_path):
    """Due insiemi vuoti sono uguali e non dimostrano niente: ricordarlo sarebbe peggio che non provare."""
    percorso = str(tmp_path / "restrict.json")
    memoria = outlook_com.MemoriaRestrict(percorso)
    cart = CartellaFinta([])
    o = _outlook(usa_restrict=True, memoria=memoria)
    assert o._restrict_affidabile(cart, BASE, None) is False
    assert memoria.leggi("BANCO", cart.StoreID, cart.EntryID) is None
    assert not (tmp_path / "restrict.json").exists()


def test_la_memoria_distingue_postazione_casella_e_cartella(tmp_path):
    """Lo stesso indice può essere sano su un PC e rotto su un altro: l'esito parla di quella
    postazione, di quella casella e di quella cartella, non di «Restrict»."""
    m = outlook_com.MemoriaRestrict(str(tmp_path / "restrict.json"))
    m.scrivi("PC-A", "store-1", "cartella-1", True)
    assert m.leggi("PC-A", "store-1", "cartella-1") is True
    assert m.leggi("PC-B", "store-1", "cartella-1") is None
    assert m.leggi("PC-A", "store-2", "cartella-1") is None
    assert m.leggi("PC-A", "store-1", "cartella-2") is None


def test_un_file_di_memoria_rotto_non_ferma_niente(tmp_path):
    """È un file di comodo, non un dato: se non si legge si riparte dalla prova."""
    percorso = tmp_path / "restrict.json"
    percorso.write_text("{non è json", encoding="utf-8")
    m = outlook_com.MemoriaRestrict(str(percorso))
    assert m.leggi("BANCO", "store-finto", "cartella-finta") is None
    m.scrivi("BANCO", "store-finto", "cartella-finta", True)
    assert outlook_com.MemoriaRestrict(str(percorso)).leggi("BANCO", "store-finto", "cartella-finta") is True


def test_la_finestra_della_prova_ordinaria_e_di_24_ore():
    """Non 7 giorni: la prova ordinaria è un campione, e ciò che deve dimostrare — fuso e formato
    della data — si dimostra su un campione come su tutto. La prova larga resta `--restrict GIORNI`."""
    import inspect
    firma = inspect.signature(outlook_com.Outlook.__init__)
    assert firma.parameters["autoprova_ore"].default == 24.0
