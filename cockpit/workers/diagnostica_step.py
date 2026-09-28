"""Passa un corpus di file STEP e stampa che cosa ne ha letto `step_struttura`.

A che cosa serve. Le fixture dei test sono scritte a mano: dimostrano che il lettore fa ciò che
intende fare, non che il MONDO sia come lo abbiamo immaginato. Un file uscito da NX, da Catia o da
SolidWorks puo' mettere le revisioni dove non ce le aspettiamo, ripetere un sottoassieme cento volte,
lasciare PRODUCT orfani o riferirsi a pezzi che stanno in un altro file. Prima di costruire tabelle,
proposte e schermate sopra questa lettura, si guarda che cosa esce dai file veri.

Come si usa, dalla cartella `cockpit`:

    python workers/diagnostica_step.py                      # tutti gli STEP di docs/step_files
    python workers/diagnostica_step.py docs/step_files/X.stp --albero
    python workers/diagnostica_step.py docs --tempo-max-s 300 --nodi-max 20000

Che cosa NON fa: non scrive niente, non tocca il database, non manda niente a nessuno. Legge i file
che gli si indicano e stampa. I CAD restano dove sono — `docs/` e' fuori dal repository — e qui
dentro non ce n'e' nemmeno uno: c'e' il comando che li attraversa.
"""
from __future__ import annotations

import argparse
import os
import sys
from collections import defaultdict

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from step_struttura import LIMITI_DEFAULT, leggi_struttura  # noqa: E402

ESTENSIONI = (".stp", ".step", ".p21")

# Il corpus sta in `docs/step_files`, che e' fuori dal repository: qui c'e' solo dove cercarlo, e
# vale sia lanciando il comando dalla cartella `cockpit` sia dalla radice del progetto.
_RADICE = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
CARTELLA_DEFAULT = os.path.join(_RADICE, "docs", "step_files")

# Gli scarti si leggono dai numeri della struttura v3 (`scarti`), non dalle frasi degli avvisi: sono
# un ESITO della lettura, non un limite. Un file con trecento PRODUCT orfani e' un file da guardare,
# ma non e' un file letto male, e non va a finire nei `limiti`.


def misura(s: dict) -> dict:
    """I numeri che servono al giudizio, ricavati dal grafo che il worker consegnerebbe al server."""
    figli: dict[str, list[str]] = defaultdict(list)
    padri: dict[str, int] = defaultdict(int)
    for r in s["relazioni"]:
        figli[r["padre"]].append(r["figlio"])
        padri[r["figlio"]] += 1
    chiavi = [n["chiave"] for n in s["nodi"]]
    profondita, cicli, scollegati = _profondita(s["radici"], figli, chiavi)
    return {
        "nodi": len(s["nodi"]),
        "relazioni": len(s["relazioni"]),
        "occorrenze": sum(r["qta"] for r in s["relazioni"]),
        "radici": len(s["radici"]),
        "profondita": profondita,
        "multi_padre": sum(1 for n in chiavi if padri[n] > 1),
        "qta_oltre_1": sum(1 for r in s["relazioni"] if r["qta"] > 1),
        "cicli": cicli,
        "scollegati": scollegati,
        "orfani": s["scarti"]["prodotti_senza_definizione"],
        "non_risolte": s["scarti"]["occorrenze_non_risolte"],
        "anelli": s["scarti"]["occorrenze_su_se_stesse"],
        "troncati": s["scarti"]["testi_troncati"],
    }


def _profondita(radici, figli, tutti) -> tuple[int, int, int]:
    """Livelli dell'albero piu' profondo, quanti archi tornano indietro, quanti nodi restano fuori.

    In pila e non in ricorsione: un grafo profondo non deve far morire il comando dove il worker
    invece regge. Un nodo gia' in cammino e' un ciclo — `A dentro B dentro A` — e si conta senza
    seguirlo, altrimenti il conto non finisce.

    Si parte DALLE RADICI, e poi da tutto il resto. La seconda meta' non e' una precauzione: senza,
    un ciclo staccato dall'albero — `C dentro D dentro C`, con nessuno dei due appeso a una radice —
    non lo visitava nessuno, e il comando stampava «0 archi che tornano indietro» a proposito di un
    file che ne contiene uno. Chi legge la diagnostica per decidere se fidarsi del lettore deve
    vedere il file INTERO, non solo la parte che pende dalle radici; e quanti nodi stiano in quella
    parte staccata e' esso stesso un fatto da stampare.
    """
    profondita: dict[str, int] = {}
    in_cammino: set[str] = set()
    cicli = 0
    partenze = list(radici) + list(tutti)
    quante_radici = len(list(radici))
    dalle_radici = 0
    for i, partenza in enumerate(partenze):
        if i == quante_radici:
            dalle_radici = len(profondita)
        if partenza in profondita:
            continue
        pila = [(partenza, False)]
        while pila:
            nodo, risalita = pila.pop()
            if risalita:
                in_cammino.discard(nodo)
                profondita[nodo] = 1 + max((profondita.get(f, 0) for f in figli.get(nodo, ())), default=0)
                continue
            if nodo in profondita:
                continue
            if nodo in in_cammino:
                cicli += 1
                continue
            in_cammino.add(nodo)
            pila.append((nodo, True))
            for f in figli.get(nodo, ()):
                if f not in profondita:
                    pila.append((f, False))
    if quante_radici >= len(partenze):
        dalle_radici = len(profondita)
    return max(profondita.values(), default=0), cicli, len(profondita) - dalle_radici


def scheda(percorso: str, s: dict, m: dict) -> str:
    lim = s["limiti"]
    byte_file = os.path.getsize(percorso)
    stato = f"TRONCATO ({lim['motivo']})" if lim["troncato"] else "intero"
    # «quanto si e' letto» si dice solo quando cambia qualcosa: fermarsi a un chilobyte dalla fine e
    # leggere tutto sono la stessa cosa per chi deve giudicare il grafo.
    quota = lim["byte_letti"] / byte_file if byte_file else 1.0
    letto = "" if quota > 0.99 else f"  ({quota * 100:.0f}% letto)"
    righe = [
        f"{os.path.basename(percorso)}",
        f"  file           {_mb(byte_file)}   schema {s['schema'] or '?'}   "
        f"{stato}{letto}   {lim['tempo_s']:.2f} s",
        f"  grafo          {m['nodi']} nodi, {m['relazioni']} relazioni, "
        f"{m['occorrenze']} occorrenze, {m['radici']} radici, profondita' {m['profondita']}",
        f"  condivisioni   {m['multi_padre']} nodi con piu' di un padre, "
        f"{m['qta_oltre_1']} relazioni con qta > 1",
        f"  scartati       {m['orfani']} PRODUCT orfani, {m['non_risolte']} occorrenze irrisolte, "
        f"{m['anelli']} anelli, {m['troncati']} testi troncati, {m['cicli']} archi che tornano indietro, "
        f"{m['scollegati']} nodi che nessuna radice raggiunge",
    ]
    for a in s["avvisi"]:
        righe.append(f"  avviso         {a}")
    return "\n".join(righe)


def albero(s: dict, profondita_max: int = 6, figli_max: int = 40) -> str:
    """L'albero come lo vedrebbe l'ingegnere: id, nome e revisione GREZZI, cosi' come sono nel file.

    Un nodo condiviso da due padri compare due volte — e' la stessa cosa che fa il CAD — ma la
    seconda si ferma li': ripetere un sottoassieme intero sotto ogni padre riempie lo schermo e non
    aggiunge un fatto.
    """
    per_chiave = {n["chiave"]: n for n in s["nodi"]}
    figli: dict[str, list[dict]] = defaultdict(list)
    for r in s["relazioni"]:
        figli[r["padre"]].append(r)
    righe: list[str] = []
    gia_aperti: set[str] = set()

    def scrivi(chiave: str, prefisso: str, qta: int, livello: int) -> None:
        n = per_chiave.get(chiave)
        etichetta = chiave if n is None else " | ".join(
            (n["id_grezzo"] or "-", n["nome_grezzo"] or "-", n["rev_grezza"] or "-"))
        moltiplicatore = f"{qta}x " if qta > 1 else ""
        ripetuto = chiave in gia_aperti and figli.get(chiave)
        righe.append(f"{prefisso}{moltiplicatore}{etichetta}" + ("   [gia' aperto sopra]" if ripetuto else ""))
        if ripetuto or livello >= profondita_max:
            if not ripetuto and figli.get(chiave):
                righe.append(f"{prefisso}   ...")
            return
        gia_aperti.add(chiave)
        sotto = figli.get(chiave, [])
        for r in sotto[:figli_max]:
            scrivi(r["figlio"], prefisso + "   ", r["qta"], livello + 1)
        if len(sotto) > figli_max:
            righe.append(f"{prefisso}   ... altri {len(sotto) - figli_max} figli")

    for radice in s["radici"]:
        scrivi(radice, "  ", 1, 0)
    return "\n".join(righe)


def _mb(byte: int) -> str:
    return f"{byte / 1048576:.1f} MB" if byte >= 1048576 else f"{byte / 1024:.0f} KB"


def _file_da(percorsi: list[str]) -> list[str]:
    trovati: list[str] = []
    for p in percorsi:
        if os.path.isdir(p):
            for radice, _, nomi in os.walk(p):
                trovati += [os.path.join(radice, n) for n in sorted(nomi)
                            if n.lower().endswith(ESTENSIONI)]
        elif os.path.isfile(p):
            trovati.append(p)
        else:
            print(f"non trovato: {p}", file=sys.stderr)
    return trovati


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description="Che cosa legge step_struttura da un corpus di file STEP.")
    p.add_argument("percorsi", nargs="*", default=[CARTELLA_DEFAULT],
                   help=f"file o cartelle da attraversare (default: {CARTELLA_DEFAULT})")
    p.add_argument("--albero", action="store_true", help="stampa anche l'albero di ogni file")
    p.add_argument("--profondita-albero", type=int, default=6)
    p.add_argument("--nodi-max", type=int, default=LIMITI_DEFAULT["nodi_max"])
    p.add_argument("--occorrenze-max", type=int, default=LIMITI_DEFAULT["occorrenze_max"])
    p.add_argument("--tempo-max-s", type=float, default=LIMITI_DEFAULT["tempo_max_s"])
    a = p.parse_args(argv)

    limiti = {"nodi_max": a.nodi_max, "occorrenze_max": a.occorrenze_max, "tempo_max_s": a.tempo_max_s}
    file = _file_da(a.percorsi or [CARTELLA_DEFAULT])
    if not file:
        print("nessun file STEP da leggere", file=sys.stderr)
        return 1

    print(f"{len(file)} file, tetti: {limiti['nodi_max']} nodi, {limiti['occorrenze_max']} occorrenze, "
          f"{limiti['tempo_max_s']:.0f} s\n")
    riepilogo = []
    for percorso in file:
        s = leggi_struttura(percorso, limiti)
        m = misura(s)
        print(scheda(percorso, s, m))
        if a.albero and s["nodi"]:
            print("  albero (id_grezzo | nome_grezzo | rev_grezza):")
            print(albero(s, a.profondita_albero))
        print()
        riepilogo.append((os.path.basename(percorso), s, m))

    print(f"{'file':<34} {'schema':<7} {'nodi':>6} {'rel':>6} {'occ':>7} {'rad':>4} "
          f"{'prof':>4} {'cond':>5} {'orf':>5} {'irr':>5} {'s':>6}  stato")
    for nome, s, m in riepilogo:
        stato = s["limiti"]["motivo"] if s["limiti"]["troncato"] else "intero"
        print(f"{nome[:34]:<34} {(s['schema'] or '?')[:7]:<7} {m['nodi']:>6} {m['relazioni']:>6} "
              f"{m['occorrenze']:>7} {m['radici']:>4} {m['profondita']:>4} {m['multi_padre']:>5} "
              f"{m['orfani']:>5} {m['non_risolte']:>5} {s['limiti']['tempo_s']:>6.2f}  {stato}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
