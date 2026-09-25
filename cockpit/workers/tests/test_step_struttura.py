"""L2 — la struttura di un file STEP: nodi, relazioni, quantita'.

Le fixture sono COSTRUITE qui, non prese dal corpus riservato: un assieme di prova sta in venti righe
di Part 21 e il repository puo' contenerlo. Il corpus vero — che nel repository non c'e' e non ci
entra — si attraversa con `python workers/diagnostica_step.py`, ed e' li' che si vedono le cose che
una fixture non puo' inventare: quale esportatore lascia la revisione vuota, in che ordine scrive le
entita', quanto e' grande davvero un assieme. Due di quelle cose hanno cambiato il lettore, e i test
che ne sono nati stanno qui sotto.

La regola che questi test difendono, e che il piano chiama A1.2: il worker NON classifica. Non decide
se `77722757_B` e' un codice, non separa la revisione dal nome, non sa che cosa sia una famiglia. Se
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
        ("A", "77722757", "77722757_B", "SUPPORTO COFANO", "B"),
        ("B", "77720517", "77720517", "PIASTRA", "1"),
        ("C", "77720518", "77720518", "RINFORZO", ""),
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
    assert a["id_grezzo"] == "77722757"
    assert a["nome_grezzo"] == "77722757_B"
    assert a["descrizione_grezza"] == "SUPPORTO COFANO"
    assert a["rev_grezza"] == "B"
    assert a["evidenza"]["entita"] == "PRODUCT" and a["evidenza"]["riga"] == rif["A"]
    assert a["evidenza"]["formation"] and a["evidenza"]["definition"]


def test_la_struttura_non_porta_codici(tmp_path):
    """A1.2: il nodo porta i grezzi. Codice e revisione li decide il server con le regole del cliente."""
    percorso, _ = scrivi_step(tmp_path, "codici.step", [
        ("A", "77722757", "77722757_B", "SUPPORTO", "B"),
    ])
    s = leggi_struttura(percorso)
    nodo = s["nodi"][0]
    assert "codice" not in nodo and "rev" not in nodo, nodo
    assert set(nodo) == {"chiave", "id_grezzo", "nome_grezzo", "descrizione_grezza", "rev_grezza", "evidenza"}
    # il nome resta com'e' scritto: niente maiuscole forzate, niente suffissi tolti
    assert nodo["nome_grezzo"] == "77722757_B"


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
        ("A", "7772", "7772", "", "A"),
        ("B", "7773", "7773", "", "A"),
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
    percorso, rif = scrivi_step(tmp_path, "parte.step", [("A", "1234567A", "1234567A", "", "4")])
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
             + "#100=PRODUCT(\n  '77722757',\n  '77722757_B',\n  'SUPPORTO COFANO',\n  (#1)\n);\n"
             + "#101=PRODUCT_DEFINITION_FORMATION('B','',#100);\n"
             + "#102=PRODUCT_DEFINITION('design','',\n#101,#1);\n"
             + CHIUSURA)
    (tmp_path / "multiriga.step").write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(tmp_path / "multiriga.step"))
    assert [n["nome_grezzo"] for n in s["nodi"]] == ["77722757_B"]
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
             + "#100=PRODUCT('77722757','77722757','',(#1));\n"
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
    assert s["limiti"]["motivo"] == "nodi"
    assert len(s["nodi"]) == 3
    assert any("oltre 3 PRODUCT" in a for a in s["avvisi"]), s["avvisi"]
    assert s["limiti"]["byte_letti"] > 0


def test_oltre_il_limite_di_occorrenze_si_dichiara_troncato(tmp_path):
    """Mille bulloni uguali sono mille occorrenze e due soli nodi: il tetto sui nodi non li fermerebbe."""
    percorso, rif = scrivi_step(tmp_path, "bulloni.step", [
        ("A", "ASSIEME", "ASSIEME", "", "1"),
        ("B", "BULLONE", "BULLONE", "", "1"),
    ], [("A", "B")] * 6)
    s = leggi_struttura(percorso, {"occorrenze_max": 3})
    assert s["limiti"]["troncato"] is True
    assert s["limiti"]["motivo"] == "occorrenze"
    assert len(s["nodi"]) == 2, "i nodi c'erano tutti: il tetto e' sulle occorrenze"
    assert [(r["padre"], r["figlio"], r["qta"]) for r in s["relazioni"]] == [(rif["A"], rif["B"], 3)]
    assert any("oltre 3 occorrenze" in a for a in s["avvisi"]), s["avvisi"]


def test_fermarsi_prima_dei_prodotti_non_e_un_file_senza_prodotti(tmp_path):
    """In un file vero del corpus le occorrenze stanno PRIMA dei prodotti.

    Fermarsi sul loro tetto vuol dire uscire con zero nodi da un file che ne aveva duecento: se
    l'avviso dicesse «nessun PRODUCT nel file», direbbe una cosa falsa su un file intero.
    """
    testo = (INTESTAZIONE
             + "".join(f"#{500 + i}=NEXT_ASSEMBLY_USAGE_OCCURRENCE('{i}','','',#102,#112,$);\n"
                       for i in range(6))
             + "#100=PRODUCT('A','A','',(#1));\n"
             + "#101=PRODUCT_DEFINITION_FORMATION('1','',#100);\n"
             + "#102=PRODUCT_DEFINITION('design','',#101,#1);\n"
             + "#110=PRODUCT('B','B','',(#1));\n"
             + "#111=PRODUCT_DEFINITION_FORMATION('1','',#110);\n"
             + "#112=PRODUCT_DEFINITION('design','',#111,#1);\n"
             + CHIUSURA)
    (tmp_path / "prima.step").write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(tmp_path / "prima.step"), {"occorrenze_max": 3})
    assert s["limiti"]["motivo"] == "occorrenze"
    assert s["nodi"] == []
    assert any("fermata prima di qualsiasi PRODUCT" in a for a in s["avvisi"]), s["avvisi"]
    assert not any("nessun PRODUCT nel file" in a for a in s["avvisi"]), s["avvisi"]


def test_il_tempo_scade_anche_dove_non_c_e_niente_da_contare(tmp_path):
    """In uno STEP vero la geometria e' quasi tutto il file e non produce nessuna entita' utile.

    Un controllo del tempo fatto solo a ogni entita' trovata, li' dentro, non scatterebbe mai: il
    worker resterebbe sul file finche' non finisce, tetto o non tetto. I tre PRODUCT stanno in fondo
    apposta — la lettura non deve arrivarci, e i byte letti devono essere meno di tutto il file.
    """
    # meta' righe con apici e meta' senza, per passare da entrambe le vie del tokenizer: quella
    # completa e quella veloce, che salta la riga intera e prima di oggi non guardava l'orologio.
    geometria = "".join(
        f"#{1000 + i}=CARTESIAN_POINT('',(0.,0.,{i}.));\n" if i % 2 else
        f"#{1000 + i}=B_SPLINE_CURVE_WITH_KNOTS((#1,#2),.UNSPECIFIED.,.F.,.F.);\n"
        for i in range(6000))
    testo = (INTESTAZIONE + geometria
             + "#90000=PRODUCT('COD','COD','',(#1));\n"
             + "#90001=PRODUCT_DEFINITION_FORMATION('A','',#90000);\n"
             + "#90002=PRODUCT_DEFINITION('design','',#90001,#1);\n"
             + CHIUSURA)
    percorso = tmp_path / "geometria.step"
    percorso.write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(percorso), {"tempo_max_s": 0.0})
    assert s["limiti"]["troncato"] is True
    assert s["limiti"]["motivo"] == "tempo"
    assert s["limiti"]["byte_letti"] < percorso.stat().st_size
    assert s["nodi"] == []


def test_il_tempo_scade_anche_su_un_file_di_una_riga_sola(tmp_path):
    """Certi esportatori scrivono la sezione DATA tutta su una riga: il controllo per righe non
    scatterebbe mai, e l'unico momento in cui si torna a guardare l'orologio e' a ogni istanza."""
    corpo = "".join(f"#{100 + i}=PRODUCT('COD{i}','COD{i}','',(#1));" for i in range(50))
    testo = INTESTAZIONE + corpo + "\n" + CHIUSURA
    percorso = tmp_path / "una-riga.step"
    percorso.write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(percorso), {"tempo_max_s": 0.0})
    assert s["limiti"]["troncato"] is True
    assert s["limiti"]["motivo"] == "tempo"
    assert s["nodi"] == []


def test_i_tetti_tornano_indietro_anche_quando_non_servono(tmp_path):
    """Un albero completo lo dice: `troncato` falso e `motivo` vuoto. E i tetti ci sono lo stesso,
    perche' fra un anno nessuno potra' piu' sapere quali erano quel giorno."""
    percorso, _ = scrivi_step(tmp_path, "intero.step", [
        ("A", "ASSIEME", "ASSIEME", "", "1"),
        ("B", "PARTE", "PARTE", "", "1"),
    ], [("A", "B")])
    s = leggi_struttura(percorso)
    lim = s["limiti"]
    assert lim["troncato"] is False and lim["motivo"] == ""
    assert lim["nodi_max"] > 0 and lim["occorrenze_max"] > 0 and lim["tempo_max_s"] > 0
    assert lim["byte_letti"] == (tmp_path / "intero.step").stat().st_size
    assert isinstance(lim["tempo_s"], float) and lim["tempo_s"] >= 0.0


def test_una_revisione_fatta_di_spazi_e_una_revisione_assente(tmp_path):
    """Due CAD veri del corpus scrivono la revisione mancante in due modi: `' '` e `''`.

    Se il primo arrivasse com'e', `rev_grezza` sarebbe «vuota» in un file e «uno spazio» nell'altro,
    e ogni lettore piu' avanti — il confronto, la proposta, la schermata — dovrebbe sapere che le due
    cose sono la stessa. Gli spazi ai bordi si tolgono qui, una volta.
    """
    testo = (INTESTAZIONE
             + "#100=PRODUCT('0.000.0000.0',' 0.000.0000.0 ',' ',(#1));\n"
             + "#101=PRODUCT_DEFINITION_FORMATION_WITH_SPECIFIED_SOURCE(' ',' ',#100,.NOT_KNOWN.);\n"
             + "#102=PRODUCT_DEFINITION('design','',#101,#1);\n"
             + CHIUSURA)
    (tmp_path / "spazi.step").write_text(testo, encoding="latin-1")
    nodo = leggi_struttura(str(tmp_path / "spazi.step"))["nodi"][0]
    assert nodo["rev_grezza"] == ""
    assert nodo["descrizione_grezza"] == ""
    assert nodo["nome_grezzo"] == "0.000.0000.0", "dentro la stringa non si tocca niente, ai bordi si'"


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


# ---------------------------------------------------------------- v3: gli scarti in numeri (A4.4, D35)
#
# Sul server una MANCANZA diventa una proposta di rimozione solo se la lettura e' completa, e la
# completezza si decide dai numeri di `scarti`, non dalle frasi di `avvisi`. Questi test fissano che i
# numeri ci siano sempre, che dicano la stessa cosa delle frasi, e che non ne manchi nessuno.

SCARTI_ZERO = {"prodotti_senza_definizione": 0, "occorrenze_non_risolte": 0,
               "occorrenze_su_se_stesse": 0, "testi_troncati": 0}


def test_un_assieme_letto_per_intero_ha_gli_scarti_a_zero(tmp_path):
    percorso, _ = scrivi_step(tmp_path, "pulito.step", [
        ("A", "77722757", "77722757", "", ""),
        ("B", "77720517", "77720517", "", ""),
    ], [("A", "B"), ("A", "B")])
    s = leggi_struttura(percorso)
    assert s["versione"] == 3 == VERSIONE_STRUTTURA
    assert s["scarti"] == SCARTI_ZERO
    assert s["avvisi"] == []


def test_un_product_orfano_si_conta(tmp_path):
    testo = (INTESTAZIONE
             + "#100=PRODUCT('A','A','',(#1));\n#101=PRODUCT_DEFINITION_FORMATION('','',#100);\n"
             + "#102=PRODUCT_DEFINITION('design','',#101,#1);\n"
             + "#110=PRODUCT('ORFANO','ORFANO','',(#1));\n#120=PRODUCT('ALTRO','ALTRO','',(#1));\n"
             + CHIUSURA)
    (tmp_path / "orfani.step").write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(tmp_path / "orfani.step"))
    assert s["scarti"] == dict(SCARTI_ZERO, prodotti_senza_definizione=2)
    assert "2 PRODUCT senza PRODUCT_DEFINITION: ignorati" in s["avvisi"]


def test_un_occorrenza_con_un_estremo_che_non_porta_a_un_nodo_si_conta(tmp_path):
    testo = (INTESTAZIONE
             + "#100=PRODUCT('A','A','',(#1));\n#101=PRODUCT_DEFINITION_FORMATION('','',#100);\n"
             + "#102=PRODUCT_DEFINITION('design','',#101,#1);\n"
             + "#600=NEXT_ASSEMBLY_USAGE_OCCURRENCE('1','pos','',#102,#999,$);\n"
             + CHIUSURA)
    (tmp_path / "irrisolta.step").write_text(testo, encoding="latin-1")
    s = leggi_struttura(str(tmp_path / "irrisolta.step"))
    assert s["scarti"] == dict(SCARTI_ZERO, occorrenze_non_risolte=1)
    assert "1 occorrenze con estremi non risolti: ignorate" in s["avvisi"]


def test_un_pezzo_dentro_se_stesso_si_conta_a_parte(tmp_path):
    """Si conta, ma in un numero suo: e' un arco impossibile, e il server non lo prende per un buco."""
    percorso, _ = scrivi_step(tmp_path, "anello.step", [("A", "X", "X", "", "")], [("A", "A"), ("A", "A")])
    s = leggi_struttura(percorso)
    assert s["scarti"] == dict(SCARTI_ZERO, occorrenze_su_se_stesse=2)
    assert s["scarti"]["occorrenze_non_risolte"] == 0


def test_un_testo_troncato_si_conta(tmp_path):
    lungo = "B" * 250
    percorso, _ = scrivi_step(tmp_path, "lungo.step", [("A", lungo, lungo, "", "")])
    s = leggi_struttura(percorso)
    assert s["scarti"] == dict(SCARTI_ZERO, testi_troncati=2)   # id e nome
    assert "2 testi troncati a 200 caratteri" in s["avvisi"]


def test_gli_scarti_ci_sono_anche_quando_la_lettura_non_riesce(tmp_path):
    """La forma e' la stessa in ogni esito: chi legge non deve distinguere «zero» da «assente».
    Che la lettura sia fallita lo dicono gli avvisi, e il server li guarda PRIMA dei numeri."""
    finto = tmp_path / "finto.step"
    finto.write_bytes(b"%PDF-1.4\n")
    for s in (leggi_struttura(str(finto)), leggi_struttura(str(tmp_path / "non-c-e.step"))):
        assert s["versione"] == 3
        assert set(s["scarti"]) == set(SCARTI_ZERO)
        assert s["avvisi"], "una lettura fallita deve dirlo"


def test_gli_scarti_sono_quelli_del_contratto(tmp_path):
    from contratti import ScartiSTEP, StrutturaSTEP
    percorso, _ = scrivi_step(tmp_path, "p.step", [("A", "P", "P", "", "")])
    s = leggi_struttura(percorso)
    assert set(s["scarti"]) == set(ScartiSTEP.model_fields)
    assert StrutturaSTEP.model_validate(s).versione == 3
