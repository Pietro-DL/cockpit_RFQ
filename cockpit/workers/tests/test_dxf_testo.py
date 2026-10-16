"""L2 — il testo di un DXF ASCII e i campi del cartiglio (K4).

Le fixture sono COSTRUITE qui, con codici ACME inventati: un cartiglio di prova sta in poche decine di coppie
(codice di gruppo, valore). La forma imita quella dei cartigli che hanno portato al K4: etichetta e valore sono
due MTEXT separati dentro un BLOCK richiamato da un INSERT, la piccola etichetta in alto nella cella e il valore
sotto, piu' grande; la revisione del documento e' centrata nella sua cella; sulla riga del riferimento 3D c'e' una
seconda «rev./Rev.», che e' la revisione del 3D e non si riporta; e nel cartiglio c'e' il nome di una persona, che
non deve uscire dal worker.

La regola che questi test difendono: il worker NON classifica (il valore e' testo grezzo) e riporta SOLO i campi che
riconosce, mai gli altri testi del disegno.
"""
from __future__ import annotations

import json

from contratti import TestoDXF as ModelloTestoDXF  # un nome che comincia per «Test» pytest lo raccoglierebbe
from dxf_testo import (DXF_LETTO, DXF_NON_LETTO, VERSIONE_TESTO_DXF, etichetta_di, leggi_testo_dxf,
                       leggi_testo_dxf_da_byte, pulisci_mtext, pulisci_text)


def mtext(x, y, h, testo, attacco=4):
    """Le coppie di un MTEXT (con i codici di formato di un esportatore vero in testa)."""
    return ["0", "MTEXT", "8", "0", "10", str(x), "20", str(y), "30", "0.0", "40", str(h), "71", str(attacco),
            "1", "\\fArial|b1|i0;\\W1.0000;" + testo]


def text(x, y, h, testo, orizz=0, vert=0):
    c = ["0", "TEXT", "8", "0", "10", str(x), "20", str(y), "30", "0.0", "40", str(h), "1", testo]
    if orizz or vert:
        c += ["72", str(orizz), "11", str(x), "21", str(y), "73", str(vert)]
    return c


def insert(nome, x, y, sx=1.0, sy=1.0, rot=0.0):
    return ["0", "INSERT", "8", "0", "2", nome, "10", str(x), "20", str(y), "30", "0.0", "41", str(sx), "42", str(sy),
            "50", str(rot)]


def blocco(nome, contenuto, base=(0.0, 0.0)):
    return ["0", "BLOCK", "8", "0", "2", nome, "70", "0", "10", str(base[0]), "20", str(base[1]), "30", "0.0",
            "3", nome] + contenuto + ["0", "ENDBLK"]


def dxf(blocchi=(), entita=(), acadver="AC1014", codepage="ANSI_1252", codifica="cp1252") -> bytes:
    coppie = ["0", "SECTION", "2", "HEADER", "9", "$ACADVER", "1", acadver, "9", "$DWGCODEPAGE", "3", codepage,
              "0", "ENDSEC", "0", "SECTION", "2", "BLOCKS"]
    for b in blocchi:
        coppie += b
    coppie += ["0", "ENDSEC", "0", "SECTION", "2", "ENTITIES"]
    for e in entita:
        coppie += e
    coppie += ["0", "ENDSEC", "0", "EOF"]
    return ("\r\n".join(coppie) + "\r\n").encode(codifica)


def cartiglio_acme(codice="ACME1001", documento="ACME2001", rev="00", rif3d="ACME3001", rev3d="01"):
    """Le due tabelle del cartiglio, nelle coordinate del blocco: la riga del codice, del documento e della sua
    revisione (centrata), e quella del riferimento 3D con la sua revisione e un nome di persona."""
    righe = (mtext(-145.725, 7.75, 1.5, "part no./Mat. Nr.") + mtext(-144.725, 1.75, 3.5, codice, 7)
             + mtext(-109.725, 7.75, 1.5, "doc. no./Dok. Nr.") + mtext(-108.725, 1.75, 3.5, documento, 7)
             + mtext(-73.225, 7.75, 1.5, "rev./Rev.") + mtext(-69.0, 1.75, 3.5, rev, 8))
    rif = (mtext(-160.225, 7.75, 1.5, "ref.3D doc./Ref.3D Dok.") + mtext(-159.975, 1.0, 2.0, rif3d, 7)
           + mtext(-127.225, 7.75, 1.5, "rev./Rev.") + mtext(-126.975, 1.0, 2.0, rev3d, 7)
           + mtext(-100.0, 7.75, 1.5, "approved by") + mtext(-99.75, 1.0, 2.0, "Mario Rossi", 7))
    return [blocco("CARTIGLIO_A", righe), blocco("CARTIGLIO_B", rif)]


def campi(fatto):
    return {(c["etichetta"], c["valore"]) for c in fatto["cartiglio"]}


def test_il_cartiglio_nei_blocchi_da_codice_documento_revisione_e_riferimento_3d():
    f = leggi_testo_dxf_da_byte(dxf(cartiglio_acme(), [insert("CARTIGLIO_A", 584, 10), insert("CARTIGLIO_B", 584, 28)]))
    assert f["versione"] == VERSIONE_TESTO_DXF == 1
    assert f["esito"] == DXF_LETTO and f["versione_dxf"] == "AC1014" and f["codifica"] == "cp1252"
    assert campi(f) == {("codice", "ACME1001"), ("numero_disegno", "ACME2001"), ("revisione", "00"),
                        ("riferimento_3d", "ACME3001")}
    # la revisione del 3D («01», sulla riga del riferimento) non e' la revisione del disegno
    assert [c["valore"] for c in f["cartiglio"] if c["etichetta"] == "revisione"] == ["00"]
    for c in f["cartiglio"]:
        assert c["fonte"] == "nativo" and c["zona"] == "pagina" and c["pagina"] == 1 and len(c["riquadro"]) == 4
    letta = {c["etichetta"]: c["letta"] for c in f["cartiglio"]}
    assert letta["codice"] == "part no./Mat. Nr." and letta["riferimento_3d"] == "ref.3D doc./Ref.3D Dok."
    # le posizioni sono nel disegno: il blocco e' inserito a (584, 10)
    cod = next(c for c in f["cartiglio"] if c["etichetta"] == "codice")
    assert cod["riquadro"][0] == round(584 - 144.725, 2)
    # dall'alto in basso: prima la riga del riferimento 3D (inserita piu' in alto)
    assert f["cartiglio"][0]["etichetta"] == "riferimento_3d"
    ModelloTestoDXF(**f)  # passa per il contratto


def test_il_fatto_non_porta_gli_altri_testi_del_disegno():
    """Il nome di una persona nel cartiglio («approved by») non esce dal worker: solo i campi riconosciuti."""
    testo = json.dumps(leggi_testo_dxf_da_byte(dxf(cartiglio_acme(), [insert("CARTIGLIO_A", 0, 0),
                                                                    insert("CARTIGLIO_B", 0, 30)])))
    assert "Mario" not in testo and "approved" not in testo


def test_la_revisione_conta_solo_sulla_riga_del_numero_del_disegno():
    soli = blocco("R", mtext(0, 7.75, 1.5, "rev./Rev.") + mtext(0.25, 1.75, 3.5, "07", 7)
                  + mtext(-40, 7.75, 1.5, "ref.3D doc./Ref.3D Dok.") + mtext(-39.75, 1.0, 2.0, "ACME3009", 7))
    f = leggi_testo_dxf_da_byte(dxf([soli], [insert("R", 100, 100)]))
    assert campi(f) == {("riferimento_3d", "ACME3009")}


def test_un_valore_fuori_dalla_finestra_non_e_il_valore():
    """Il valore e' sotto l'etichetta, vicino: un testo troppo in basso o troppo a destra non lo e'."""
    lontano = blocco("L", mtext(0, 10, 1.5, "part no./Mat. Nr.") + mtext(0, -10, 3.5, "ACME1099", 7)
                     + mtext(-40, 10, 1.5, "doc. no./Dok. Nr.") + mtext(-30, 5, 3.5, "ACME2099", 7))
    f = leggi_testo_dxf_da_byte(dxf([lontano], [insert("L", 0, 0)]))
    assert f["esito"] == DXF_LETTO and f["cartiglio"] == []


def test_insert_scalato_e_con_la_base_spostata():
    righe = blocco("S", mtext(10, 17.75, 1.5, "part no./Mat. Nr.") + mtext(11, 11.75, 3.5, "ACME1002", 7),
                   base=(10.0, 10.0))
    f = leggi_testo_dxf_da_byte(dxf([righe], [insert("S", 200, 50, 2.0, 2.0)]))
    assert campi(f) == {("codice", "ACME1002")}
    c = f["cartiglio"][0]
    # (11 - 10) * 2 + 200 = 202; altezza 3,5 * 2 = 7, centro (11,75 + 1,75 - 10) * 2 + 50 = 57
    assert c["riquadro"][0] == 202.0 and c["riquadro"][1] == 53.5 and c["riquadro"][3] == 60.5


def test_un_insert_ruotato_non_si_segue_e_il_fatto_lo_dice():
    """Ruotato di 90° o di 180°, «sotto» non e' piu' sotto: il blocco non si segue (la regola prenderebbe un altro
    testo per il valore), e il fatto lo dice nel motivo, con l'esito «letto». Un giro intero non e' una rotazione."""
    righe = blocco("G", mtext(0, 10, 1.5, "part no./Mat. Nr.") + mtext(1, 4, 3.5, "ACME1003", 7))
    for rot in (90.0, 180.0, -90.0):
        f = leggi_testo_dxf_da_byte(dxf([righe], [insert("G", 100, 100, rot=rot)]))
        assert f["esito"] == DXF_LETTO and f["testi"] == 0 and f["cartiglio"] == [], rot
        assert f["motivo"].startswith("1 INSERT ruotati non seguiti"), (rot, f["motivo"])
        ModelloTestoDXF(**f)
    f = leggi_testo_dxf_da_byte(dxf([righe], [insert("G", 100, 100, rot=360.0)]))
    assert campi(f) == {("codice", "ACME1003")} and f["motivo"] == ""
    # un INSERT ruotato di un blocco senza testi non toglie niente, e non si dice
    vuoto = blocco("V", [])
    f = leggi_testo_dxf_da_byte(dxf([righe, vuoto], [insert("G", 0, 0), insert("V", 0, 0, rot=90.0)]))
    assert campi(f) == {("codice", "ACME1003")} and f["motivo"] == ""


def test_un_insert_dentro_un_blocco_non_si_segue_e_si_conta():
    interno = blocco("INTERNO", mtext(0, 10, 1.5, "part no./Mat. Nr.") + mtext(1, 4, 3.5, "ACME1004", 7))
    esterno = blocco("ESTERNO", insert("INTERNO", 0, 0))
    f = leggi_testo_dxf_da_byte(dxf([interno, esterno], [insert("ESTERNO", 0, 0)]))
    assert f["annidati"] == 1 and f["cartiglio"] == []


def test_i_testi_nelle_entities_e_il_text_centrato():
    ent = [text(0, 10, 1.5, "PART NO."), text(1, 4, 3.5, "ACME1005"),
           text(40, 10, 1.5, "DOC. NO."), text(41, 4, 3.5, "ACME2005"),
           text(80, 10, 1.5, "REV."), text(83, 4, 3.5, "B", orizz=1)]
    f = leggi_testo_dxf_da_byte(dxf([], ent))
    assert campi(f) == {("codice", "ACME1005"), ("numero_disegno", "ACME2005"), ("revisione", "B")}


def test_la_codifica_cp1252_e_utf8():
    righe = blocco("C", mtext(0, 10, 1.5, "part no./Mat. Nr.") + mtext(1, 4, 3.5, "ACME-Ä1", 7))
    f = leggi_testo_dxf_da_byte(dxf([righe], [insert("C", 0, 0)]))
    assert campi(f) == {("codice", "ACME-Ä1")}
    f = leggi_testo_dxf_da_byte(dxf([righe], [insert("C", 0, 0)], acadver="AC1027", codepage="ANSI_1252",
                                    codifica="utf-8"))
    assert f["codifica"] == "utf-8" and campi(f) == {("codice", "ACME-Ä1")}


def test_l_elenco_particolari_sopra_il_cartiglio_non_e_il_codice():
    """L'intestazione dell'elenco particolari di un assieme («POS. PART NO. DESCRIPTION QTY») sta sopra il cartiglio:
    «PART NO.» li' e' il nome di una colonna, e la prima riga dell'elenco (un figlio, ACME5001) non e' il codice del
    disegno. Le righe dell'elenco sono piu' alte dell'intestazione, cosi' la cella sotto non le ferma da sola."""
    elenco = [text(440, 40, 1.5, "POS."), text(450, 40, 1.5, "PART NO."), text(480, 40, 1.5, "DESCRIPTION"),
              text(520, 40, 1.5, "QTY"),
              text(440, 36, 2.5, "1"), text(451, 36, 2.5, "ACME5001"), text(480, 36, 2.5, "VITE ACME"),
              text(520, 36, 2.5, "4")]
    f = leggi_testo_dxf_da_byte(dxf(cartiglio_acme(), [insert("CARTIGLIO_A", 584, 10), insert("CARTIGLIO_B", 584, 28)]
                                    + elenco))
    assert campi(f) == {("codice", "ACME1001"), ("numero_disegno", "ACME2001"), ("revisione", "00"),
                        ("riferimento_3d", "ACME3001")}
    # con il nome breve della posizione («N.») fuori da un'etichetta e' lo stesso
    breve = [text(440, 40, 1.5, "N."), text(450, 40, 1.5, "PART NO."), text(520, 40, 1.5, "Q.ty"),
             text(451, 36, 2.5, "ACME5001")]
    assert campi(leggi_testo_dxf_da_byte(dxf([], breve))) == set()
    # senza la colonna della quantita' non e' un'intestazione: «PART NO.» resta l'etichetta
    senza = [text(440, 40, 1.5, "POS."), text(450, 40, 1.5, "PART NO."), text(451, 36, 2.5, "ACME5001")]
    assert campi(leggi_testo_dxf_da_byte(dxf([], senza))) == {("codice", "ACME5001")}


def test_l_intestazione_della_tabella_come_nel_testo_dei_pdf():
    import dxf_testo
    import worker_analisi
    assert dxf_testo._COLONNA_POSIZIONE == worker_analisi._COLONNA_POSIZIONE
    assert dxf_testo._COLONNA_QUANTITA == worker_analisi._COLONNA_QUANTITA
    assert dxf_testo._COLONNA_POSIZIONE_BREVE == worker_analisi._COLONNA_POSIZIONE_BREVE

    def riga(*scritte):
        return [dxf_testo.Testo(10.0 * k, 0.0, 1.5, 5.0, s, 0) for k, s in enumerate(scritte)]
    assert dxf_testo.intestazione_tabella(riga("Item", "PART NO.", "Q.tà/Q.ty"))
    assert dxf_testo.intestazione_tabella(riga("RIF.", "PART NO.", "QTY"))
    # «Nr.» dentro un'etichetta del cartiglio non e' la colonna della posizione
    assert not dxf_testo.intestazione_tabella(riga("doc. no./Dok. Nr.", "QTY"))
    assert not dxf_testo.intestazione_tabella(riga("part no./Mat. Nr.", "doc. no./Dok. Nr.", "rev./Rev."))


def test_la_cella_vuota_non_prende_la_riga_sotto():
    """Il valore manca: sotto c'e' la cella dopo, con la sua etichetta (alta come quella del codice) e il nome di una
    persona. La ricerca si ferma all'etichetta della cella sotto e il campo non si riporta; il nome non esce."""
    vuota = blocco("V", mtext(0, 10, 1.5, "part no./Mat. Nr.") + mtext(0, 6, 1.5, "drawn/gez.")
                   + mtext(1, 3.5, 2.0, "Mario Rossi"))
    f = leggi_testo_dxf_da_byte(dxf([vuota], [insert("V", 0, 0)]))
    assert f["esito"] == DXF_LETTO and f["testi"] == 3 and f["cartiglio"] == []
    assert "Mario" not in json.dumps(f)
    # il testo piu' vicino alto come l'etichetta e' un'etichetta anche lui: sospetto, il campo non si riporta
    sospetto = blocco("S", mtext(0, 10, 1.5, "part no./Mat. Nr.") + mtext(1, 6, 1.5, "Mario Rossi"))
    f = leggi_testo_dxf_da_byte(dxf([sospetto], [insert("S", 0, 0)]))
    assert f["cartiglio"] == [] and "Mario" not in json.dumps(f)


def test_un_dxf_senza_testi_e_senza_blocchi_e_letto():
    """Un DXF ASCII valido, con le sezioni ma senza testi e senza BLOCK, e' letto con il cartiglio vuoto: non e' un
    file che «non sembra un DXF». Basta anche la sola coppia di fine."""
    f = leggi_testo_dxf_da_byte(dxf([], []))
    assert f["esito"] == DXF_LETTO and f["motivo"] == "" and f["testi"] == 0 and f["cartiglio"] == []
    assert f["versione_dxf"] == "AC1014"
    ModelloTestoDXF(**f)
    f = leggi_testo_dxf_da_byte(b"  0\r\nEOF\r\n")
    assert f["esito"] == DXF_LETTO and f["cartiglio"] == []
    f = leggi_testo_dxf_da_byte(b"999\nsolo un commento\n")
    assert f["esito"] == DXF_NON_LETTO


def test_la_lettura_non_tiene_piu_di_un_multiplo_di_testi_max():
    """testi_max limita anche la memoria: durante la lettura si tengono al piu' FATTORE_LETTI * testi_max elementi
    delle ENTITIES e altrettanti dei BLOCKS, e oltre il fatto e' troncato."""
    from dxf_testo import FATTORE_LETTI, _leggi_entita
    molti = [text(10 * k, 0, 1.0, f"ACME{k:04d}") for k in range(200)]
    quote = [blocco(f"Q{k}", text(0, 0, 1.0, f"{k}")) for k in range(200)]
    dati = dxf(quote, molti)
    letti = _leggi_entita(iter(dati.decode("cp1252").splitlines(keepends=True)), FATTORE_LETTI * 5)
    assert letti.pieno and len(letti.entita) == FATTORE_LETTI * 5
    assert len(letti.blocchi) + sum(len(b["testi"]) for b in letti.blocchi.values()) == FATTORE_LETTI * 5
    f = leggi_testo_dxf_da_byte(dati, limiti={"testi_max": 5})
    assert f["troncato"] and f["testi"] == 5
    # sotto il tetto niente e' troncato
    letti = _leggi_entita(iter(dati.decode("cp1252").splitlines(keepends=True)), 10_000)
    assert not letti.pieno and len(letti.entita) == 200 and len(letti.blocchi) == 200


def test_un_dxf_binario_o_che_non_e_un_dxf_e_un_esito():
    f = leggi_testo_dxf_da_byte(b"AutoCAD Binary DXF\r\n\x1a\x00" + bytes(range(256)))
    assert f["esito"] == DXF_NON_LETTO and "binario" in f["motivo"] and f["cartiglio"] == []
    ModelloTestoDXF(**f)
    f = leggi_testo_dxf_da_byte(b"non sono un disegno\n")
    assert f["esito"] == DXF_NON_LETTO and f["motivo"]
    f = leggi_testo_dxf_da_byte(b"")
    assert f["esito"] == DXF_NON_LETTO


def test_i_limiti_dichiarati():
    f = leggi_testo_dxf_da_byte(dxf(cartiglio_acme(), [insert("CARTIGLIO_A", 0, 0)]), limiti={"testi_max": 3})
    assert f["troncato"] and f["testi"] == 3 and f["limiti"]["testi_max"] == 3
    f = leggi_testo_dxf_da_byte(dxf(cartiglio_acme(), [insert("CARTIGLIO_A", 0, 0)]), limiti={"campi_max": 1})
    assert f["troncato"] and len(f["cartiglio"]) == 1
    piccolo = leggi_testo_dxf_da_byte(dxf(cartiglio_acme(), [insert("CARTIGLIO_A", 0, 0)]), limiti={"byte_max": 64})
    assert piccolo["troncato"]
    f = leggi_testo_dxf_da_byte(dxf(cartiglio_acme(), [insert("CARTIGLIO_A", 0, 0)]))
    assert not f["troncato"] and f["limiti"] == {"byte_max": 64 * 1024 * 1024, "testi_max": 5000, "testo_max": 512,
                                                  "campi_max": 24}


def test_le_etichette_riconosciute():
    assert etichetta_di("part no./Mat. Nr.") == "codice"
    assert etichetta_di("PART NO.") == "codice" and etichetta_di("Mat. Nr.") == "codice"
    assert etichetta_di("doc. no./Dok. Nr.") == "numero_disegno"
    assert etichetta_di("ref.3D doc./Ref.3D Dok.") == "riferimento_3d"
    assert etichetta_di("rev./Rev.") == "revisione"
    # non per pezzi di testo: il testo intero, e due voci diverse non fanno un'etichetta
    for t in ("part no. ACME1001", "Material", "part no./Dok. Nr.", "rev./Rev./Rev.", "approved by", ""):
        assert etichetta_di(t) is None, t


def test_i_codici_di_formato():
    assert pulisci_mtext("\\fArial|b1|i0;\\W1.0000;part no./Mat. Nr.") == "part no./Mat. Nr."
    assert pulisci_mtext("{\\H2.5x;\\C1;ACME}\\Priga due\\~fine") == "ACME riga due fine"
    assert pulisci_mtext("\\LACME\\l \\U+00C4 \\S1/2; \\\\ \\{x\\}") == "ACME Ä 1/2 \\ {x}"
    assert pulisci_text("%%c20 %%p0,1 45%%d %%uACME%%u") == "Ø20 ±0,1 45° ACME"


def test_l_analisi_di_un_dxf_porta_il_testo(tmp_path):
    from worker_analisi import analizza_file
    p = tmp_path / "ACME1001_D42_ACME2001_000_00.dxf"
    p.write_bytes(dxf(cartiglio_acme(), [insert("CARTIGLIO_A", 0, 0), insert("CARTIGLIO_B", 0, 30)]))
    r = analizza_file(str(p), p.name)
    # tipo, codice e rev in testa restano quelli del nome
    assert r["tipo_proposto"] == "sviluppo_dxf" and r["fonte"] == "nome_file"
    assert r["codice"] == "ACME1001_D42_ACME2001_000" and r["rev"] == "00"
    td = r["dettagli"]["testo_dxf"]
    assert td["esito"] == DXF_LETTO and ("riferimento_3d", "ACME3001") in campi(td)
    assert set(td) == set(ModelloTestoDXF.model_fields)


def test_un_dxf_illeggibile_non_fa_fallire_l_analisi(tmp_path):
    from worker_analisi import analizza_file
    p = tmp_path / "ACME1001.dxf"
    p.write_bytes(b"AutoCAD Binary DXF\r\n\x1a\x00\x00")
    r = analizza_file(str(p), p.name)
    assert r["tipo_proposto"] == "sviluppo_dxf" and r["dettagli"]["testo_dxf"]["esito"] == DXF_NON_LETTO


def test_leggi_dal_file(tmp_path):
    p = tmp_path / "x.dxf"
    p.write_bytes(dxf(cartiglio_acme(), [insert("CARTIGLIO_A", 0, 0)]))
    assert campi(leggi_testo_dxf(str(p))) >= {("codice", "ACME1001")}
    assert leggi_testo_dxf(str(tmp_path / "manca.dxf"))["esito"] == DXF_NON_LETTO


def test_nei_pdf_il_riferimento_3d_e_un_etichetta_e_doc_no_no():
    """Nel testo dei PDF (via OCR) «ref.3D doc./Ref.3D Dok.» e' il riferimento al 3D; «doc. no./Dok. Nr.» non e' il
    numero del disegno (la sottoversione del testo dei PDF non cambia)."""
    from worker_analisi import VERSIONE_TESTO_PDF, Parola, etichetta_a
    parole = [Parola(0, 0, 10, 2, w) for w in ("ref.3D", "doc./Ref.3D", "Dok.", "ACME3001")]
    assert etichetta_a(parole, 0) == ("riferimento_3d", 3)
    assert etichetta_a([Parola(0, 0, 10, 2, w) for w in ("doc.", "no./Dok.", "Nr.")], 0) is None
    # riscritta per GP-3: la sottoversione del testo e' la 3 (le tabelle), il cartiglio e' quello della 2
    assert VERSIONE_TESTO_PDF == 3
