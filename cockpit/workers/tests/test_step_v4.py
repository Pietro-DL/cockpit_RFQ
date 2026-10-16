"""GP-3, sotto-blocco 5 (direttiva 21 §5.3): struttura STEP v4, DIAGNOSTICA della geometria e stato della struttura.

STEP COSTRUITI qui, con nomi ACME inventati: un assieme senza geometria propria, un pezzo con il solido raggiunto
attraverso una relazione fra rappresentazioni, un nodo «saldatura» con la sola terna di riferimento, e il
posizionamento d'assieme (relazione CON trasformazione) che non deve portare il solido del figlio nel padre.

Le regole difese: per nodo si riassume la geometria (solidi, superfici, curve, mappati, vuota) senza togliere nessun
nodo; le relazioni portano id e nome d'istanza delle NAUO; lo stato distingue assente, non estraibile, parziale e
valida, e «assente» non e' mai «pezzo unico».
"""
from __future__ import annotations

from contratti import StrutturaSTEP
from step_struttura import VERSIONE_STRUTTURA, leggi_struttura

TESTA = ("ISO-10303-21;\nHEADER;\nFILE_DESCRIPTION((''),'2;1');\nFILE_NAME('x','2026-01-01',(''),(''),'','','');\n"
         "FILE_SCHEMA(('AUTOMOTIVE_DESIGN { 1 0 10303 214 1 1 1 1 }'));\nENDSEC;\nDATA;\n")
CODA = "ENDSEC;\nEND-ISO-10303-21;\n"


def assieme_acme(con_nauo: bool = True) -> str:
    r = [TESTA,
         # tre prodotti con formation e definition
         "#10=PRODUCT('ACME-ASM','ACME-ASM','',(#1));\n#11=PRODUCT_DEFINITION_FORMATION('A','',#10);\n"
         "#12=PRODUCT_DEFINITION('design','',#11,#1);\n",
         "#20=PRODUCT('ACME-P1','ACME-P1','',(#1));\n#21=PRODUCT_DEFINITION_FORMATION('1','',#20);\n"
         "#22=PRODUCT_DEFINITION('design','',#21,#1);\n",
         "#30=PRODUCT('Saldatura d''angolo','Saldatura d''angolo','',(#1));\n"
         "#31=PRODUCT_DEFINITION_FORMATION('','',#30);\n#32=PRODUCT_DEFINITION('design','',#31,#1);\n",
         # le forme e le rappresentazioni
         "#40=PRODUCT_DEFINITION_SHAPE('','',#12);\n#41=SHAPE_REPRESENTATION('',(#90),#2);\n"
         "#42=SHAPE_DEFINITION_REPRESENTATION(#40,#41);\n",
         "#50=PRODUCT_DEFINITION_SHAPE('','',#22);\n#51=SHAPE_REPRESENTATION('',(#91),#2);\n"
         "#52=SHAPE_DEFINITION_REPRESENTATION(#50,#51);\n"
         "#53=ADVANCED_BREP_SHAPE_REPRESENTATION('',(#92,#93),#2);\n"
         "#54=SHAPE_REPRESENTATION_RELATIONSHIP('','',#51,#53);\n",
         "#60=PRODUCT_DEFINITION_SHAPE('','',#32);\n#61=SHAPE_REPRESENTATION('',(#94),#2);\n"
         "#62=SHAPE_DEFINITION_REPRESENTATION(#60,#61);\n",
         # il posizionamento del pezzo nell'assieme: CON trasformazione, non e' la geometria dell'assieme
         "#70=(REPRESENTATION_RELATIONSHIP('','',#51,#41)REPRESENTATION_RELATIONSHIP_WITH_TRANSFORMATION(#71)"
         "SHAPE_REPRESENTATION_RELATIONSHIP());\n",
         "#90=AXIS2_PLACEMENT_3D('',#3,#4,#5);\n#91=AXIS2_PLACEMENT_3D('',#3,#4,#5);\n"
         "#92=MANIFOLD_SOLID_BREP('ACME',#95);\n#93=AXIS2_PLACEMENT_3D('',#3,#4,#5);\n"
         "#94=AXIS2_PLACEMENT_3D('',#3,#4,#5);\n"]
    if con_nauo:
        r.append("#80=NEXT_ASSEMBLY_USAGE_OCCURRENCE('NAUO1','ACME-P1:1','',#12,#22,$);\n"
                 "#81=NEXT_ASSEMBLY_USAGE_OCCURRENCE('NAUO2','ACME-P1:2','',#12,#22,$);\n"
                 "#82=NEXT_ASSEMBLY_USAGE_OCCURRENCE('NAUO3','Saldatura d''angolo:1','',#12,#32,$);\n")
    r.append(CODA)
    return "".join(r)


def scrivi(tmp_path, testo: str, nome: str = "acme.stp") -> str:
    p = tmp_path / nome
    p.write_text(testo, encoding="latin-1")
    return str(p)


def nodo(s: dict, chiave: str) -> dict:
    return next(n for n in s["nodi"] if n["chiave"] == chiave)


def test_geometria_per_nodo_e_nomi_d_istanza(tmp_path):
    s = leggi_struttura(scrivi(tmp_path, assieme_acme()))
    StrutturaSTEP.model_validate(s)
    assert s["versione"] == VERSIONE_STRUTTURA == 4 and s["stato"] == "valida" and s["motivo_stato"] == ""
    asm, p1, sald = nodo(s, "#10"), nodo(s, "#20"), nodo(s, "#30")
    # il solido del pezzo si raggiunge attraverso la relazione SENZA trasformazione
    assert p1["geometria"] == {"solidi": 1, "superfici": 0, "curve": 0, "mappati": 0, "rappresentazioni": 2,
                               "vuota": False}
    # l'assieme non prende il solido del figlio dal posizionamento (relazione CON trasformazione)
    assert asm["geometria"]["vuota"] is True and asm["geometria"]["solidi"] == 0
    # la «saldatura» ha solo la terna: vuota, ma resta un nodo (nessuna rimozione, nessun giudizio)
    assert sald["geometria"]["vuota"] is True and len(s["nodi"]) == 3
    rel = {(r["padre"], r["figlio"]): r for r in s["relazioni"]}
    assert rel[("#10", "#20")]["qta"] == 2
    assert rel[("#10", "#20")]["istanze"] == [{"rif": "#80", "id": "NAUO1", "nome": "ACME-P1:1"},
                                              {"rif": "#81", "id": "NAUO2", "nome": "ACME-P1:2"}]
    assert rel[("#10", "#30")]["istanze"][0]["nome"] == "Saldatura d'angolo:1"


def test_struttura_assente_non_e_pezzo_unico(tmp_path):
    s = leggi_struttura(scrivi(tmp_path, assieme_acme(con_nauo=False)))
    assert s["stato"] == "assente" and len(s["nodi"]) == 3 and s["relazioni"] == []
    assert "non dice se il file e' un pezzo unico" in s["motivo_stato"]
    assert not any(k in s for k in ("mono_pezzo", "pezzo_unico"))
    vuoto = leggi_struttura(scrivi(tmp_path, TESTA + CODA, "vuoto.stp"))
    assert vuoto["stato"] == "assente" and vuoto["motivo_stato"] == "nessun PRODUCT nel file"


def test_non_estraibile_e_parziale(tmp_path):
    s = leggi_struttura(scrivi(tmp_path, "non sono uno STEP\n", "no.stp"))
    assert s["stato"] == "non_estraibile" and "Part 21" in s["motivo_stato"]
    StrutturaSTEP.model_validate(s)
    s = leggi_struttura(str(tmp_path / "manca.stp"))
    assert s["stato"] == "non_estraibile"
    s = leggi_struttura(scrivi(tmp_path, assieme_acme()), {"occorrenze_max": 1})
    assert s["stato"] == "parziale" and "occorrenze" in s["motivo_stato"]
    # un'occorrenza con un estremo che non porta a un nodo: parziale
    rotto = assieme_acme().replace("#12,#32,$)", "#12,#999,$)")
    s = leggi_struttura(scrivi(tmp_path, rotto, "rotto.stp"))
    assert s["stato"] == "parziale" and "non risolte" in s["motivo_stato"]


def test_superfici_curve_e_mappati(tmp_path):
    t = (TESTA + "#10=PRODUCT('ACME-S','ACME-S','',(#1));\n#11=PRODUCT_DEFINITION_FORMATION('','',#10);\n"
         "#12=PRODUCT_DEFINITION('design','',#11,#1);\n#40=PRODUCT_DEFINITION_SHAPE('','',#12);\n"
         "#41=MANIFOLD_SURFACE_SHAPE_REPRESENTATION('',(#90,#91,#92),#2);\n#42=SHAPE_DEFINITION_REPRESENTATION(#40,#41);\n"
         "#90=SHELL_BASED_SURFACE_MODEL('',(#5));\n#91=GEOMETRIC_CURVE_SET('',(#6));\n#92=MAPPED_ITEM('',#7,#8);\n" + CODA)
    g = leggi_struttura(scrivi(tmp_path, t))["nodi"][0]["geometria"]
    assert (g["superfici"], g["curve"], g["mappati"], g["solidi"], g["vuota"]) == (1, 1, 1, 0, False)
