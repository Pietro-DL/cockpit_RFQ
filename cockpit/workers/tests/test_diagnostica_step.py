"""L2 — il comando diagnostico: i numeri che si guardano prima di fidarsi del lettore.

`diagnostica_step.py` non e' un attrezzo di servizio: e' la strada per cui un corpus vero — che nel
repository non c'e' e non ci entra — diventa un giudizio su `step_struttura`. Se i suoi numeri sono
sbagliati, il giudizio e' sbagliato, e non se ne accorge nessuno: nessuno conta a mano le relazioni
di un assieme da duecento pezzi.

La prova che conta e' la prima. Un ciclo `C dentro D dentro C` che non pende da nessuna radice non
veniva visitato da nessuno — il cammino partiva solo dalle radici — e la scheda diceva «0 archi che
tornano indietro» di un file che ne contiene uno. Il lettore lo consegnava, il comando lo perdeva.
"""
from __future__ import annotations

from diagnostica_step import albero, misura, scheda
from step_struttura import leggi_struttura
from test_step_struttura import scrivi_step

PRODOTTI = [
    ("A", "0.000.0000.0", "0.000.0000.0_B", "ASSIEME", "B"),
    ("B", "0.000.0001.0", "0.000.0001.0", "PIASTRA", ""),
    ("C", "0.000.0002.0", "0.000.0002.0", "STAFFA", ""),
    ("D", "0.000.0003.0", "0.000.0003.0", "RINFORZO", ""),
]


def leggi(tmp_path, nome, occorrenze, prodotti=PRODOTTI):
    percorso, rif = scrivi_step(tmp_path, nome, prodotti, occorrenze)
    return leggi_struttura(percorso), rif, percorso


def test_un_ciclo_che_nessuna_radice_raggiunge_viene_contato(tmp_path):
    # A → B e' l'albero vero; C e D si contengono a vicenda e non pendono da niente.
    s, rif, _ = leggi(tmp_path, "ciclo_staccato.step", [("A", "B"), ("C", "D"), ("D", "C")])
    assert s["radici"] == [rif["A"]], s["radici"]

    m = misura(s)
    assert m["nodi"] == 4
    assert m["relazioni"] == 3
    assert m["cicli"] == 1, "il ciclo staccato dall'albero non e' stato contato"
    assert m["scollegati"] == 2, "C e D non pendono da nessuna radice e la scheda non lo dice"
    assert m["profondita"] >= 2


def test_un_file_fatto_di_solo_ciclo_non_fa_girare_a_vuoto_il_comando(tmp_path):
    # Nessuna radice: nel file ci sono due soli pezzi, e ciascuno contiene l'altro. Il cammino deve
    # partire lo stesso — da dove non si sa — e soprattutto finire.
    s, _, _ = leggi(tmp_path, "solo_ciclo.step", [("C", "D"), ("D", "C")], PRODOTTI[2:])
    assert s["radici"] == []

    m = misura(s)
    assert m["cicli"] == 1
    assert m["profondita"] > 0, "senza radici non si e' misurato niente"
    assert m["scollegati"] == m["nodi"]


def test_i_numeri_di_un_assieme_normale(tmp_path):
    # B sta sotto A due volte (qta 2) e sotto C una: e' il pezzo condiviso, il caso A1.1.
    s, rif, percorso = leggi(tmp_path, "assieme.step",
                             [("A", "B"), ("A", "B"), ("A", "C"), ("C", "B"), ("A", "D")])
    m = misura(s)
    assert m["nodi"] == 4
    assert m["relazioni"] == 4
    assert m["occorrenze"] == 5
    assert m["radici"] == 1
    assert m["profondita"] == 3           # A → C → B
    assert m["multi_padre"] == 1          # B, sotto A e sotto C
    assert m["qta_oltre_1"] == 1          # A → B, due volte
    assert m["cicli"] == 0
    assert m["scollegati"] == 0
    assert m["orfani"] == 0 and m["non_risolte"] == 0 and m["anelli"] == 0

    testo = scheda(percorso, s, m)
    assert "4 nodi, 4 relazioni, 5 occorrenze, 1 radici" in testo
    assert "1 nodi con piu' di un padre" in testo
    assert "0 nodi che nessuna radice raggiunge" in testo
    assert "intero" in testo
    assert rif["A"] not in testo, "la scheda e' un riassunto, non un elenco di riferimenti #12"


def test_l_albero_stampa_i_campi_grezzi_e_non_ripete_un_sottoassieme(tmp_path):
    s, _, _ = leggi(tmp_path, "condiviso.step", [("A", "C"), ("A", "D"), ("C", "B"), ("D", "B")])
    testo = albero(s)
    assert "0.000.0000.0 | 0.000.0000.0_B | ASSIEME | B" not in testo, "l'albero stampa tre campi, non quattro"
    assert "0.000.0000.0 | 0.000.0000.0_B | B" in testo, testo
    # la revisione assente si vede come «-», non come un buco nella riga
    assert "0.000.0001.0 | 0.000.0001.0 | -" in testo, testo
    # B sta sotto C e sotto D: la seconda volta non si riapre
    assert testo.count("[gia' aperto sopra]") == 0, testo
    assert testo.count("0.000.0001.0 | 0.000.0001.0 | -") == 2, "il pezzo condiviso non compare sotto tutti e due i padri"


def test_la_scheda_dice_quando_la_lettura_si_e_fermata(tmp_path):
    s, _, percorso = leggi(tmp_path, "troncato.step", [("A", "B"), ("A", "C")])
    s["limiti"]["troncato"] = True
    s["limiti"]["motivo"] = "nodi"
    s["limiti"]["byte_letti"] = 10
    testo = scheda(percorso, s, misura(s))
    assert "TRONCATO (nodi)" in testo
    assert "% letto" in testo, "una lettura fermata a meta' non dice quanto ha letto"
