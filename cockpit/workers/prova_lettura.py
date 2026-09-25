"""L5 — legge una cartella Outlook VERA e non scrive niente (3R, blocco 1).

    python prova_lettura.py --cartella "Posta inviata" --giorni 7
    python prova_lettura.py --cartella "Posta inviata" --giorni 7 --vecchio-modo
    python prova_lettura.py --casella commerciale@azienda.example --cartella "Posta in arrivo"

A che cosa serve. Il sync della Posta inviata di questa postazione cadeva su

    AttributeError: GetNext.ReceivedTime

per un elemento che non è una mail (un rapporto di consegna, un appuntamento, un elemento di Sync
Issues): `ReceivedTime` veniva letto prima che qualcuno guardasse `Class`, e la protezione copriva
solo `com_error`. Un elemento portava con sé la cartella intera. Questo comando legge quella stessa
cartella con il codice di oggi e dice quanti messaggi ha letto e quanti elementi ha saltato.

`--vecchio-modo` rifà la scansione come era PRIMA della correzione. Serve a rendere la prova
significativa: se il vecchio modo cade e il nuovo no, sulla stessa cartella e nello stesso momento,
allora l'elemento anomalo c'è davvero e la correzione è ciò che fa la differenza. Se invece nessuno
dei due cade, la prova non ha dimostrato niente — quel giorno in quella cartella non c'era niente di
anomalo — e va detto così, non spacciato per una prova superata.

Che cosa NON fa: non parla con il server, non apre il database, non segna niente come letto, non
sposta e non crea bozze. Apre Outlook in lettura e chiude.
"""
from __future__ import annotations

import argparse
import logging
import sys
import time
from datetime import datetime, timedelta, timezone

import pywintypes

from outlook_com import ERRORI_ELEMENTO, Outlook, Saltati, _utc, classe_di, entryid_di

log = logging.getLogger("prova")


def vecchio_modo(cart, dal: datetime, al: datetime | None = None) -> tuple[int, str]:
    """La scansione com'era prima della correzione: ReceivedTime letto per primo, solo com_error.

    Restituisce (elementi raccolti, errore) — l'errore è la riga che fermava la cartella.
    """
    items = cart.Items
    items.Sort("[ReceivedTime]", True)
    raccolti = 0
    item = items.GetFirst()
    while item is not None:
        try:
            rt = _utc(item.ReceivedTime)
            if al is not None and rt > al:
                item = items.GetNext()
                continue
            if rt < dal:
                break
            raccolti += 1
        except pywintypes.com_error as e:
            log.warning("elemento saltato: %s", e)
        except Exception as e:                      # noqa: BLE001 - è il punto: usciva e fermava tutto
            return raccolti, "%s: %s" % (type(e).__name__, e)
        item = items.GetNext()
    return raccolti, ""


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--cartella", default="Posta inviata")
    ap.add_argument("--casella", default="", help="indirizzo della casella; senza, lo store predefinito del profilo")
    ap.add_argument("--giorni", type=float, default=7)
    ap.add_argument("--vecchio-modo", action="store_true", help="rifà la scansione come prima della correzione")
    ap.add_argument("--senza-restrict", action="store_true")
    ap.add_argument("--debug", action="store_true")
    a = ap.parse_args()
    logging.basicConfig(level=logging.DEBUG if a.debug else logging.INFO,
                        format="%(asctime)s %(levelname)-7s %(name)s %(message)s")

    ol = Outlook(consenti_invio=False, usa_restrict=not a.senza_restrict)
    store_id = ""
    if a.casella:
        store_id, come = ol._store_di(a.casella.lower())
        if not store_id:
            print("casella %s non presente nel profilo Outlook di questo PC" % a.casella)
            return 2
        print("casella %s risolta (%s)" % (a.casella, come))

    dal = datetime.now(timezone.utc) - timedelta(days=a.giorni)
    cart = ol.cartella(a.cartella, store_id)
    print("cartella: %s — finestra: ultimi %g giorni (dal %s UTC)" % (cart.Name, a.giorni, dal.isoformat()))

    if a.vecchio_modo:
        t0 = time.monotonic()
        raccolti, errore = vecchio_modo(cart, dal)
        print("\nVECCHIO MODO: %d elementi raccolti in %.1f s" % (raccolti, time.monotonic() - t0))
        if errore:
            print("VECCHIO MODO: la scansione si e FERMATA su -> %s" % errore)
            print("             (e il difetto: l'eccezione usciva dalla scansione e portava con se la cartella)")
        else:
            print("VECCHIO MODO: nessuna eccezione. In questa cartella, adesso, non c'e l'elemento anomalo:")
            print("             la prova sul modo nuovo non dimostra la correzione, dimostra solo che legge.")

    t0 = time.monotonic()
    saltati = Saltati()
    letti = 0
    ultimo = None
    try:
        for m in ol.leggi(a.cartella, dal, store_id=store_id, saltati=saltati):
            letti += 1
            ultimo = m.ricevuto_il or m.data_evento
    except Exception as e:                          # noqa: BLE001 - qui l'esito della prova e proprio questo
        print("\nMODO NUOVO: la lettura si e FERMATA su -> %s: %s" % (type(e).__name__, e))
        print("ESITO: FALLITO")
        return 1
    durata = time.monotonic() - t0

    print("\nMODO NUOVO: %d messaggi letti, %d elementi saltati, %.1f s" % (letti, saltati.totale, durata))
    for s in saltati.elementi[:10]:
        print("   in scarto: %s | %s | %s" % (s.entry_id[:24] + "...", s.cartella, s.errore))
    if saltati.totale > len(saltati.elementi):
        print("   (%d non messi in scarto: elementi non-mail, oppure senza EntryID. Il motivo di ognuno"
              % (saltati.totale - len(saltati.elementi)))
        print("    sta nelle righe WARNING qui sopra, con cartella, Class ed EntryID)")
    print("ultimo ricevuto: %s" % (ultimo.isoformat() if ultimo else "-"))
    print("ESITO: PASSATO — la cartella e stata letta per intero")
    return 0


if __name__ == "__main__":
    sys.exit(main())
