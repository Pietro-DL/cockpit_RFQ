"""L2 — la struttura di un file STEP: nodi, relazioni, quantita'.

Le fixture sono COSTRUITE qui, non prese dal corpus riservato: un assieme di prova sta in venti righe
di Part 21 e il repository puo' contenerlo. Il corpus vero serve alla prova manuale del B8.4 (tre
STEP dei clienti, grafo confrontato a mano con il CAD), non a questi test.

La regola che questi test difendono, e che il piano chiama A1.2: il worker NON classifica. Non decide
se `52922757_B` e' un codice, non separa la revisione dal nome, non sa che cosa sia una famiglia. Se
un giorno qualcuno rimettesse `sembra_codice` dentro la struttura, `test_la_struttura_non_porta_codici`
diventa rosso.
"""
from __future__ import annotations

from step_struttura import VERSIONE_STRUTTURA, leggi_struttura

INTESTAZIONE = """ISO-10303-21;
HEADER;
FILE_DESCRIPTION((''),'2;1');
FILE_NAME('prova.step','2026-09-22T00:00:00',(''),(''),'','','');
FILE_SCHEMA(('AUTOMOTIVE_DESIGN { 1 0 10303 214 3 1 1 }'));
ENDSEC;
DATA;
#1=APPLICATION_CONTEXT('prova');
"""

CHIUSURA = """ENDSEC;
END-ISO-10303-21;
"""


def scrivi_step(tmp_path, nome, prodotti, occorrenze=(), intestazione=INTESTAZIONE):
    """Scrive un file STEP minimo e restituisce (percorso, {etichetta: '#ref del PRODUCT'}).

    `prodotti`: (etichetta, id, name, description, rev). `occorrenze`: (etichetta padre, etichetta figlio),
    ripetuta tante volte quante sono le occorrenze.
    """
    righe = [intestazione]
    rif, definizione = {}, {}
    for i, (etichetta, ident, nome_p, descrizione, rev) in enumerate(prodotti):
        base = 100 + 10 * i
        rif[etichetta] = f"#{base}"
        definizione[etichetta] = f"#{base + 2}"
        righe.append(f"#{base}=PRODUCT('{ident}','{nome_p}','{descrizione}',(#1));\n")
        righe.append(f"#{base + 1}=PRODUCT_DEFINITION_FORMATION('{rev}','',#{base});\n")
        righe.append(f"#{base + 2}=PRODUCT_DEFINITION('design','',#{base + 1},#1);\n")
    for j, (padre, figlio) in enumerate(occorrenze):
        righe.append(f"#{500 + j}=NEXT_ASSEMBLY_USAGE_OCCURRENCE('{j + 1}','pos','',"
                     f"{definizione[padre]},{definizione[figlio]},$);\n")
    righe.append(CHIUSURA)
    percorso = tmp_path / nome
    percorso.write_text("".join(righe), encoding="latin-1")
    return str(percorso), rif


def test_un_assieme_con_due_parti(tmp_path):
    percorso, rif = scrivi_step(tmp_path, "assieme.step", [
        ("A", "52922757", "52922757_B", "SUPPORTO COFANO", "B"),
        ("B", "52920517", "52920517", "PIASTRA", "1"),
        ("C", "52920518", "52920518", "RINFORZO", ""),
    ], [("A", "B"), ("A", "C")])
    s = leggi_struttura(percorso)
    assert s["versione"] == VERSIONE_STRUTTURA
    assert s["schema"] == "AP214"
    assert s["avvisi"] == [], s["avvisi"]
    assert [n["chiave"] for n in s["nodi"]] == [rif["A"], rif["B"], rif["C"]]
    assert s["radici"] == [rif["A"]]
    assert [(r["padre"], r["figlio"], r["qta"]) for r in s["relazioni"]] == [
        (rif["A"], rif["B"], 1), (rif["A"], rif["C"], 1)]
    a = s["nodi"][0]
    assert a["id_grezzo"] == "52922757"
    assert a["nome_grezzo"] == "52922757_B"
    assert a["descrizione_grezza"] == "SUPPORTO COFANO"
    assert a["rev_grezza"] == "B"
    assert a["evidenza"]["entita"] == "PRODUCT" and a["evidenza"]["riga"] == rif["A"]
    assert a["evidenza"]["formation"] and a["evidenza"]["definition"]


def test_la_struttura_non_porta_codici(tmp_path):
    """A1.2: il nodo porta i grezzi. Codice e revisione li decide il server con le regole del cliente."""
    percorso, _ = scrivi_step(tmp_path, "codici.step", [
        ("A", "52922757", "52922757_B", "SUPPORTO", "B"),
    ])
    s = leggi_struttura(percorso)
    nodo = s["nodi"][0]
    assert "codice" not in nodo and "rev" not in nodo, nodo
    assert set(nodo) == {"chiave", "id_grezzo", "nome_grezzo", "descrizione_grezza", "rev_grezza", "evidenza"}
    # il nome resta com'e' scritto: niente maiuscole forzate, niente suffissi tolti
    assert nodo["nome_grezzo"] == "52922757_B"


def test_un_nome_che_non_e_un_codice_resta_un_nome(tmp_path):
    """«Part1» non e' un codice, ma e' un nodo: la proposta esistera' comunque, senza codice."""
    percorso, rif = scrivi_step(tmp_path, "part1.step", [
        ("A", "Assem2", "Assem2", "", ""),
        ("B", "Part1", "Part1", "", ""),
    ], [("A", "B")])
    s = leggi_struttura(percorso)
    assert [n["nome_grezzo"] for n in s["nodi"]] == ["Assem2", "Part1"]
    assert s["radici"] == [rif["A"]]


def test_due_occorrenze_sotto_lo_stesso_padre_sono_una_relazione_con_qta_2(tmp_path):
    percorso, rif = scrivi_step(tmp_path, "doppia.step", [
        ("A", "5292", "5292", "", "A"),
        ("B", "5293", "5293", "", "A"),
    ], [("A", "B"), ("A", "B")])
    s = leggi_struttura(percorso)
    assert len(s["nodi"]) == 2
    assert len(s["relazioni"]) == 1
    r = s["relazioni"][0]
    assert (r["padre"], r["figlio"], r["qta"]) == (rif["A"], rif["B"], 2)
    assert len(r["evidenza"]["righe"]) == 2, r["evidenza"]


def test_un_nodo_con_due_padri_ha_due_relazioni(tmp_path):
    """Il caso che ha fatto nascere `relazione_proposta`: X sotto PRODOTTO A e sotto PRODOTTO B."""
    percorso, rif = scrivi_step(tmp_path, "duepadri.step", [
        ("A", "PRODOTTO-A", "PRODOTTO-A", "", "1"),
        ("B", "PRODOTTO-B", "PRODOTTO-B", "", "1"),
        ("X", "SOTTO-X", "SOTTO-X", "", "1"),
    ], [("A", "X"), ("B", "X")])
    s = leggi_struttura(percorso)
    assert len(s["nodi"]) == 3, "X e' UN nodo, non due"
    assert len([n for n in s["nodi"] if n["chiave"] == rif["X"]]) == 1
    padri = sorted(r["padre"] for r in s["relazioni"] if r["figlio"] == rif["X"])
    assert padri == sorted([rif["A"], rif["B"]])
    assert sorted(s["radici"]) == sorted([rif["A"], rif["B"]])


def test_un_nodo_di_livello_3_sotto_due_sottoassiemi(tmp_path):
    """«un sottoassieme di livello 3 puo' essere legato a piu' sottoassiemi di livello 2»."""
    percorso, rif = scrivi_step(tmp_path, "livello3.step", [
        ("A", "FINITO", "FINITO", "", "1"),
        ("Y1", "SOTTO-1", "SOTTO-1", "", "1"),
        ("Y2", "SOTTO-2", "SOTTO-2", "", "1"),
        ("Z", "PEZZO-Z", "PEZZO-Z", "", "1"),
    ], [("A", "Y1"), ("A", "Y1"), ("A", "Y2"), ("Y1", "Z"), ("Y2", "Z")])
    s = leggi_struttura(percorso)
    assert len(s["nodi"]) == 4
    assert s["radici"] == [rif["A"]]
    relazioni = {(r["padre"], r["figlio"]): r["qta"] for r in s["relazioni"]}
    assert relazioni == {
        (rif["A"], rif["Y1"]): 2,
        (rif["A"], rif["Y2"]): 1,
        (rif["Y1"], rif["Z"]): 1,
        (rif["Y2"], rif["Z"]): 1,
    }


def test_una_parte_sola_e_una_radice_senza_relazioni(tmp_path):
    percorso, rif = scrivi_step(tmp_path, "parte.step", [("A", "6674611A", "6674611A", "", "4")])
    s = leggi_struttura(percorso)
    assert len(s["nodi"]) == 1 and s["relazioni"] == []
    assert s["radici"] == [rif["A"]]
    assert "nessuna occorrenza di assieme: solo parti" in s["avvisi"]


def test_un_file_che_non_e_step_non_e_un_errore(tmp_path):
    percorso = tmp_path / "finto.step"
    percorso.write_bytes(b"%PDF-1.4 non sono uno step\n" + bytes(range(256)))
    s = leggi_struttura(str(percorso))
    assert s["nodi"] == [] and s["relazioni"] == [] and s["radici"] == []
    assert s["avvisi"] == ["non e' un file STEP Part 21"]


def test_uno_step_senza_product(tmp_path):
    percorso = tmp_path / "vuoto.step"
    percorso.write_text(INTESTAZIONE + CHIUSURA, encoding="latin-1")
    s = leggi_struttura(str(percorso))
    assert s["nodi"] == []
    assert "nessun PRODUCT nel file" in s["avvisi"]
    assert s["schema"] == "AP214"


def test_un_product_senza_definizione_viene_ignorato_con_avviso(tmp_path):
    testo = INTESTAZIONE + "#100=PRODUCT('ORFANO','ORFANO','',(#1));\n" + CHIUSURA
    (tmp_path / "orfano.step").write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(tmp_path / "orfano.step"))
    assert s["nodi"] == []
    assert "1 PRODUCT senza PRODUCT_DEFINITION: ignorati" in s["avvisi"]


def test_un_istanza_su_piu_righe_si_legge_lo_stesso(tmp_path):
    """Gli esportatori vanno a capo dove vogliono: l'istanza finisce al `;`, non alla riga."""
    testo = (INTESTAZIONE
             + "#100=PRODUCT(\n  '52922757',\n  '52922757_B',\n  'SUPPORTO COFANO',\n  (#1)\n);\n"
             + "#101=PRODUCT_DEFINITION_FORMATION('B','',#100);\n"
             + "#102=PRODUCT_DEFINITION('design','',\n#101,#1);\n"
             + CHIUSURA)
    (tmp_path / "multiriga.step").write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(tmp_path / "multiriga.step"))
    assert [n["nome_grezzo"] for n in s["nodi"]] == ["52922757_B"]
    assert s["nodi"][0]["rev_grezza"] == "B"


def test_apici_raddoppiati_e_punto_e_virgola_dentro_una_stringa(tmp_path):
    """`''` e' un apice, e un `;` dentro una stringa non chiude niente: e' il motivo del tokenizer."""
    testo = (INTESTAZIONE
             + "#100=PRODUCT('D''AGOSTINO;1','STAFFA L''ALTRA','con ; dentro',(#1));\n"
             + "#101=PRODUCT_DEFINITION_FORMATION('A','',#100);\n"
             + "#102=PRODUCT_DEFINITION('design','',#101,#1);\n"
             + CHIUSURA)
    (tmp_path / "apici.step").write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(tmp_path / "apici.step"))
    n = s["nodi"][0]
    assert n["id_grezzo"] == "D'AGOSTINO;1"
    assert n["nome_grezzo"] == "STAFFA L'ALTRA"
    assert n["descrizione_grezza"] == "con ; dentro"


def test_le_sequenze_di_controllo_tornano_testo(tmp_path):
    """Part 21 scrive gli accenti come `\\X2\\00E0\\X0\\`: in database deve arrivare la lettera."""
    testo = (INTESTAZIONE
             + "#100=PRODUCT('P-1','STAFFA \\X2\\00C8\\X0\\ CURVA','',(#1));\n"
             + "#101=PRODUCT_DEFINITION_FORMATION('','',#100);\n"
             + "#102=PRODUCT_DEFINITION('design','',#101,#1);\n"
             + CHIUSURA)
    (tmp_path / "accenti.step").write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(tmp_path / "accenti.step"))
    assert s["nodi"][0]["nome_grezzo"] == "STAFFA È CURVA"
    assert s["nodi"][0]["rev_grezza"] == ""


def test_i_commenti_non_confondono_il_tokenizer(tmp_path):
    testo = (INTESTAZIONE
             + "/* questo commento contiene PRODUCT('finto','finto','',(#1)); */\n"
             + "#100=PRODUCT('VERO','VERO','',(#1));\n"
             + "#101=PRODUCT_DEFINITION_FORMATION('1','',#100);\n"
             + "#102=PRODUCT_DEFINITION('design','',#101,#1);\n"
             + CHIUSURA)
    (tmp_path / "commenti.step").write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(tmp_path / "commenti.step"))
    assert [n["nome_grezzo"] for n in s["nodi"]] == ["VERO"]


def test_i_sottotipi_degli_esportatori_valgono_come_gli_originali(tmp_path):
    """SolidWorks e altri scrivono `..._WITH_SPECIFIED_SOURCE`: e' la stessa entita' con un campo in piu'."""
    testo = (INTESTAZIONE
             + "#100=PRODUCT('52922757','52922757','',(#1));\n"
             + "#101=PRODUCT_DEFINITION_FORMATION_WITH_SPECIFIED_SOURCE('C','',#100,.NOT_KNOWN.);\n"
             + "#102=PRODUCT_DEFINITION_WITH_ASSOCIATED_DOCUMENTS('design','',#101,#1,(#1));\n"
             + CHIUSURA)
    (tmp_path / "sottotipi.step").write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(tmp_path / "sottotipi.step"))
    assert len(s["nodi"]) == 1 and s["nodi"][0]["rev_grezza"] == "C"


def test_product_definition_shape_non_e_una_definizione(tmp_path):
    """Comincia per PRODUCT_DEFINITION ma il suo terzo attributo e' un'altra cosa: presa per buona
    porterebbe il grafo dove non deve andare."""
    testo = (INTESTAZIONE
             + "#100=PRODUCT('P','P','',(#1));\n"
             + "#101=PRODUCT_DEFINITION_FORMATION('1','',#100);\n"
             + "#102=PRODUCT_DEFINITION('design','',#101,#1);\n"
             + "#103=PRODUCT_DEFINITION_SHAPE('','',#102);\n"
             + CHIUSURA)
    (tmp_path / "shape.step").write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(tmp_path / "shape.step"))
    assert len(s["nodi"]) == 1
    assert s["relazioni"] == []


def test_un_pezzo_dentro_se_stesso_viene_scartato(tmp_path):
    percorso, _ = scrivi_step(tmp_path, "anello.step", [("A", "X", "X", "", "1")], [("A", "A")])
    s = leggi_struttura(percorso)
    assert s["relazioni"] == []
    assert any("dentro se stesso" in a for a in s["avvisi"]), s["avvisi"]


def test_oltre_il_limite_di_nodi_si_dichiara_troncato(tmp_path):
    prodotti = [(f"P{i}", f"COD{i}", f"COD{i}", "", "1") for i in range(6)]
    percorso, _ = scrivi_step(tmp_path, "tanti.step", prodotti)
    s = leggi_struttura(percorso, {"nodi_max": 3})
    assert s["limiti"]["troncato"] is True
    assert len(s["nodi"]) == 3
    assert any("oltre 3 PRODUCT" in a for a in s["avvisi"]), s["avvisi"]
    assert s["limiti"]["byte_letti"] > 0


def test_una_stringa_lunghissima_si_tronca_e_lo_dice(tmp_path):
    lungo = "A" * 300
    percorso, _ = scrivi_step(tmp_path, "lungo.step", [("A", lungo, lungo, "", "1")])
    s = leggi_struttura(percorso)
    assert len(s["nodi"][0]["nome_grezzo"]) == 200
    assert any("troncati" in a for a in s["avvisi"]), s["avvisi"]


def test_due_revisioni_sullo_stesso_prodotto_si_conservano_tutte(tmp_path):
    """Sceglierne una in silenzio sarebbe inventare la revisione: la prima vale, l'altra si vede."""
    testo = (INTESTAZIONE
             + "#100=PRODUCT('P','P','',(#1));\n"
             + "#101=PRODUCT_DEFINITION_FORMATION('A','',#100);\n"
             + "#102=PRODUCT_DEFINITION('design','',#101,#1);\n"
             + "#103=PRODUCT_DEFINITION_FORMATION('B','',#100);\n"
             + "#104=PRODUCT_DEFINITION('design','',#103,#1);\n"
             + CHIUSURA)
    (tmp_path / "revisioni.step").write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(tmp_path / "revisioni.step"))
    assert len(s["nodi"]) == 1
    assert s["nodi"][0]["rev_grezza"] in ("A", "B")
    assert s["nodi"][0]["evidenza"]["rev_alternative"], s["nodi"][0]["evidenza"]


def test_un_file_che_non_esiste_non_alza(tmp_path):
    s = leggi_struttura(str(tmp_path / "non-c-e.step"))
    assert s["nodi"] == []
    assert any("struttura non letta" in a for a in s["avvisi"]), s["avvisi"]
