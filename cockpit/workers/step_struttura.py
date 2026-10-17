"""Lettura della STRUTTURA di un file STEP (ISO 10303-21): nodi, relazioni, quantita'.

Che cosa fa e che cosa NON fa. Questo modulo legge il file e dice che cosa c'e' scritto: quali
`PRODUCT` esistono, quale contiene quale e quante volte. Non decide se un nome e' un codice, non
separa una revisione, non sa che cosa sia la famiglia di codici di un cliente. Quella e' una lettura che
dipende dal CLIENTE della richiesta, e il cliente il worker non lo conosce: la fa il server, con le
regole gia' scritte per oggetto, corpo e nomi file (addendum B8, A1.2).

Percio' ogni nodo porta i quattro attributi grezzi dell'entita' e l'evidenza di dove stanno:

    {"chiave": "#12", "id_grezzo": "77722757", "nome_grezzo": "77722757_B",
     "descrizione_grezza": "SUPPORTO COFANO", "rev_grezza": "B",
     "evidenza": {"entita": "PRODUCT", "riga": "#12", "formation": "#14", "definition": "#20"}}

Nodo e relazione sono separati, come in database (`componente` e `componente_relazione`): un
sottoassieme che compare sotto due padri e' UN nodo e DUE relazioni. Un modello che mettesse il padre
sulla riga del nodo non saprebbe rappresentarlo (A1.1).

Nessuna dipendenza nuova: Part 21 e' testo, e per leggerne quattro entita' basta un tokenizer di
poche decine di righe. Non si legge geometria, non si leggono trasformazioni, non si risolvono
riferimenti esterni (AP242 multi-file): quei nodi compaiono come `PRODUCT` senza figli.

Versione 3 (addendum B8, A4.4, D35): quello che il lettore ha scartato si conta anche in numeri, in
`scarti`, oltre che nelle frasi di `avvisi`. Sul server una MANCANZA diventa una proposta di rimozione
solo se la lettura e' completa, e decidere una cancellazione confrontando frasi sarebbe fragile: un
avviso riscritto meglio cambierebbe il giudizio. Le frasi restano, per la schermata.

Versione 3 con diagnostica 1 (LS-01, 17/10/2026). GP-3 aveva portato la forma a `versione: 4` per aggiungere la
geometria per nodo, i nomi d'istanza e lo stato; ma la 4 sul server non la accetta nessuno (la funzione SQL della
completezza, `struttura_motivo_parziale` della 0020, e la gerarchia del Motore A vogliono la 3), e ogni STEP letto
risultava «parziale». `versione` e' tornata la forma che il server accetta, la 3; i campi diagnostici restano
identici, con un numero tutto loro, `versione_diagnostica` (oggi la 1), che dice chi li puo' leggere. Una
diagnostica nuova alza `versione_diagnostica`, mai `versione`.
"""
from __future__ import annotations

import logging
import re
import time

log = logging.getLogger("worker-analisi")

# La versione della FORMA della struttura (StrutturaSTEP.versione; Go: worker.VersioneStruttura): quella che il server
# accetta per la completezza. La funzione SQL struttura_motivo_parziale della 0020 (riga 662) e la gerarchia del Motore A
# (valutazione.VersioneStrutturaStep) vogliono la 3, che porta gli scarti in numeri (addendum B8, A4.4, D35). LS-01: GP-3
# l'aveva alzata a 4 per la diagnostica, e il server ha preso per «parziale» ogni STEP letto; la diagnostica ha adesso il
# suo numero (VERSIONE_DIAGNOSTICA) e questa resta la 3. Cambiarla vuol dire cambiare insieme SQL, gerarchia e worker:
# le prove della catena (valutazione/versione_struttura_test.go, workerapi TestLS01...) diventano rosse subito.
VERSIONE_STRUTTURA = 3

# La versione del blocco DIAGNOSTICO (StrutturaSTEP.versione_diagnostica; Go: worker.VersioneDiagnostica): un numero
# esplicito, non la presenza di un campo (direttiva 21 §5-§6). La 1 e' la diagnostica di GP-3 (direttiva 21 §5.3), che
# fino a LS-01 si scriveva come «versione 4». DIAGNOSTICA, mai rimozioni: per nodo il riassunto della geometria raggiunta
# dalle sue rappresentazioni (solidi, superfici, curve, elementi mappati, rappresentazioni: `vuota` quando non c'e'
# niente oltre ai riferimenti di posizionamento); per relazione i nomi d'istanza delle NAUO (id e nome); per il file lo
# `stato` della struttura: `non_estraibile` (non e' Part 21, lettura fallita), `assente` (letto, nessuna occorrenza
# d'assieme: NON vuol dire «pezzo unico»), `parziale` (un tetto, occorrenze non risolte, prodotti senza definizione),
# `valida`. Un nodo senza solido resta un nodo: un contenitore CAD puo' rappresentare un sottoassieme. La completezza la
# decide solo la 0020: lo `stato` e' meno severo (non guarda le radici multiple ne' i testi troncati). Una diagnostica
# nuova alza questo numero, mai VERSIONE_STRUTTURA.
VERSIONE_DIAGNOSTICA = 1

# Limiti: un file enorme non deve ne' far fallire il job ne' tenere il worker mezz'ora. Ciò che si è
# letto vale, il resto si dichiara troncato (§5.4 del piano).
#
# Sono TRE tetti perche' un file puo' essere grande in tre modi diversi. `nodi_max` ferma l'assieme
# con troppi pezzi distinti; `occorrenze_max` ferma quello con pochi pezzi ripetuti un milione di
# volte (mille bulloni uguali sono mille NEXT_ASSEMBLY_USAGE_OCCURRENCE e un nodo solo, e senza
# questo tetto il primo a fermarsi sarebbe la memoria); `tempo_max_s` ferma il file che e' lento per
# una ragione che non avevamo previsto — ed e' l'unico che vale anche mentre si scorre la geometria,
# dove per minuti interi non si incontra nessuna delle due cose che si contano.
LIMITI_DEFAULT = {
    "nodi_max": 2000,
    "occorrenze_max": 50000,
    "tempo_max_s": 120.0,
}

# Ogni quante righe si guarda l'orologio nel giro veloce. `time.monotonic()` costa poco ma non zero, e
# su un file di geometria le righe sono centinaia di migliaia: una ogni 4096 e' una volta ogni pochi
# millisecondi di lettura, cioe' abbastanza spesso da non sforare il tetto in modo visibile.
RIGHE_FRA_CONTROLLI = 4096

# Quanto si conserva di una stringa del file: oltre, si tronca e si dice di averlo fatto. 200 e' la
# misura della colonna che la ricevera' (componente_proposta.nome_grezzo).
MAX_TESTO = 200

# Filtro veloce sulle righe: se una riga non contiene nessuno di questi pezzi di testo non puo'
# contenere niente che serva, e non viene nemmeno ricomposta. In uno STEP vero salta il 99,9% del file.
ENTITA_UTILI = ("PRODUCT", "NEXT_ASSEMBLY_USAGE_OCCURRENCE", "FILE_SCHEMA",
                # diagnostica 1: la geometria per nodo, solo i nomi delle entita' che la riassumono
                "SHAPE_REPRESENTATION", "SHAPE_DEFINITION_REPRESENTATION", "REPRESENTATION_RELATIONSHIP",
                "MANIFOLD_SOLID_BREP", "BREP_WITH_VOIDS", "FACETED_BREP", "SURFACE_MODEL", "GEOMETRIC_CURVE_SET",
                "GEOMETRIC_SET", "MAPPED_ITEM")

# Le entita' che servono davvero, con i sottotipi che gli esportatori usano al loro posto. Sono nomi
# ESATTI e non prefissi: `PRODUCT_DEFINITION_SHAPE` comincia per PRODUCT_DEFINITION ma il suo terzo
# attributo e' un'altra cosa, e presa per una definizione porterebbe il grafo altrove.
ENTITA = {
    "PRODUCT": "PRODUCT",
    "PRODUCT_DEFINITION": "PRODUCT_DEFINITION",
    "PRODUCT_DEFINITION_WITH_ASSOCIATED_DOCUMENTS": "PRODUCT_DEFINITION",
    "PRODUCT_DEFINITION_FORMATION": "PRODUCT_DEFINITION_FORMATION",
    "PRODUCT_DEFINITION_FORMATION_WITH_SPECIFIED_SOURCE": "PRODUCT_DEFINITION_FORMATION",
    "NEXT_ASSEMBLY_USAGE_OCCURRENCE": "NEXT_ASSEMBLY_USAGE_OCCURRENCE",
    "FILE_SCHEMA": "FILE_SCHEMA",
    # diagnostica 1: la catena prodotto -> forma -> rappresentazione -> elementi geometrici
    "PRODUCT_DEFINITION_SHAPE": "PRODUCT_DEFINITION_SHAPE",
    "SHAPE_DEFINITION_REPRESENTATION": "SHAPE_DEFINITION_REPRESENTATION",
    "SHAPE_REPRESENTATION": "SHAPE_REPRESENTATION",
    "ADVANCED_BREP_SHAPE_REPRESENTATION": "SHAPE_REPRESENTATION",
    "MANIFOLD_SURFACE_SHAPE_REPRESENTATION": "SHAPE_REPRESENTATION",
    "FACETED_BREP_SHAPE_REPRESENTATION": "SHAPE_REPRESENTATION",
    "GEOMETRICALLY_BOUNDED_SURFACE_SHAPE_REPRESENTATION": "SHAPE_REPRESENTATION",
    "GEOMETRICALLY_BOUNDED_WIREFRAME_SHAPE_REPRESENTATION": "SHAPE_REPRESENTATION",
    "EDGE_BASED_WIREFRAME_SHAPE_REPRESENTATION": "SHAPE_REPRESENTATION",
    "SHELL_BASED_WIREFRAME_SHAPE_REPRESENTATION": "SHAPE_REPRESENTATION",
    "TESSELLATED_SHAPE_REPRESENTATION": "SHAPE_REPRESENTATION",
    "SHAPE_REPRESENTATION_RELATIONSHIP": "REPRESENTATION_RELATIONSHIP",
    "REPRESENTATION_RELATIONSHIP": "REPRESENTATION_RELATIONSHIP",
    "MANIFOLD_SOLID_BREP": "GEOMETRIA",
    "BREP_WITH_VOIDS": "GEOMETRIA",
    "FACETED_BREP": "GEOMETRIA",
    "SHELL_BASED_SURFACE_MODEL": "GEOMETRIA",
    "FACE_BASED_SURFACE_MODEL": "GEOMETRIA",
    "GEOMETRIC_CURVE_SET": "GEOMETRIA",
    "GEOMETRIC_SET": "GEOMETRIA",
    "MAPPED_ITEM": "GEOMETRIA",
}

# Il genere di un elemento geometrico, per il riassunto per nodo (diagnostica 1).
GENERE_GEOMETRIA = {
    "MANIFOLD_SOLID_BREP": "solidi", "BREP_WITH_VOIDS": "solidi", "FACETED_BREP": "solidi",
    "SHELL_BASED_SURFACE_MODEL": "superfici", "FACE_BASED_SURFACE_MODEL": "superfici",
    "GEOMETRIC_CURVE_SET": "curve", "GEOMETRIC_SET": "curve",
    "MAPPED_ITEM": "mappati",
}
# Quante relazioni fra rappresentazioni (senza trasformazione) si seguono da quella di un prodotto.
SALTI_RAPPRESENTAZIONE = 4
ISTANZE_MAX = 20  # nomi d'istanza conservati per relazione, come le righe NAUO

# Gli stati della struttura (diagnostica 1).
STATO_VALIDA, STATO_PARZIALE, STATO_ASSENTE, STATO_NON_ESTRAIBILE = "valida", "parziale", "assente", "non_estraibile"

# Nomi convenzionali dei protocolli applicativi: FILE_SCHEMA porta il nome lungo, la schermata mostra
# quello corto. Se non e' fra questi si conserva il nome lungo, che e' comunque un fatto.
SCHEMI = {
    "AUTOMOTIVE_DESIGN": "AP214",
    "CONFIG_CONTROL_DESIGN": "AP203",
    "INTEGRATED_CNC_SCHEMA": "AP238",
    "STRUCTURAL_FRAME_SCHEMA": "AP230",
}

_DELIM = re.compile(r"'|/\*|\*/|;")
_ISTANZA = re.compile(r"^#(\d+)\s*=\s*(.+)$", re.S)
_ENTITA = re.compile(r"([A-Za-z_][A-Za-z0-9_]*)\s*\(")
_ESCAPE = re.compile(
    r"\\X2\\([0-9A-Fa-f]{4,}?)\\X0\\"      # UTF-16BE
    r"|\\X4\\([0-9A-Fa-f]{8,}?)\\X0\\"     # UTF-32BE
    r"|\\X\\([0-9A-Fa-f]{2})"              # un byte ISO 8859-1
    r"|\\S\\(.)"                           # il carattere seguente con il bit alto acceso
    r"|\\N\\|\\T\\"                        # a capo, tabulazione
)


def leggi_struttura(percorso: str, limiti: dict | None = None) -> dict:
    """Il grafo del file, o una struttura vuota con l'avviso che dice perche'.

    Non alza mai: un file illeggibile, troncato o che non e' Part 21 e' un ESITO, non un errore del
    job. Il tipo del file (`cad_3d`) lo si sa comunque dall'estensione.
    """
    lim = dict(LIMITI_DEFAULT)
    lim.update(limiti or {})
    s = {
        "versione": VERSIONE_STRUTTURA,
        # in ogni esito, anche in una lettura fallita: dice quale diagnostica porta questo oggetto (LS-01)
        "versione_diagnostica": VERSIONE_DIAGNOSTICA,
        "schema": "",
        "radici": [],
        "nodi": [],
        "relazioni": [],
        "avvisi": [],
        # Che cosa il lettore ha visto e lasciato fuori, in numeri. Tutti zero non vuol dire «letto
        # tutto»: vuol dire che niente di cio' che si e' letto e' stato buttato. Se la lettura si e'
        # fermata lo dicono i `limiti`, se e' fallita lo dicono gli `avvisi`.
        #   prodotti_senza_definizione  PRODUCT che nessuna PRODUCT_DEFINITION raggiunge: possono
        #                               essere nodi veri a cui manca un pezzo del file
        #   occorrenze_non_risolte      NAUO con un estremo che non porta a un nodo: un arco perso
        #   occorrenze_su_se_stesse     un pezzo dentro se stesso: un arco impossibile, ignorarlo non
        #                               nasconde niente
        #   testi_troncati              stringhe tagliate a MAX_TESTO: un nome tagliato puo' non
        #                               essere piu' riconosciuto come codice
        "scarti": {
            "prodotti_senza_definizione": 0,
            "occorrenze_non_risolte": 0,
            "occorrenze_su_se_stesse": 0,
            "testi_troncati": 0,
        },
        # I tetti tornano indietro insieme a ciò che si è letto, e non per simmetria: chi guarda un
        # albero troncato deve poter vedere CONTRO QUALE muro si è fermato, senza andare a leggere la
        # configurazione del worker che l'ha prodotto — che intanto puo' essere cambiata.
        "limiti": {
            "nodi_max": int(lim["nodi_max"]),
            "occorrenze_max": int(lim["occorrenze_max"]),
            "tempo_max_s": float(lim["tempo_max_s"]),
            "byte_letti": 0,
            "tempo_s": 0.0,
            "troncato": False,
            "motivo": "",
        },
    }
    s["stato"] = STATO_NON_ESTRAIBILE
    s["motivo_stato"] = ""
    inizio = time.monotonic()
    try:
        _leggi(percorso, lim, s)
        _stato(s)
        return s
    except Exception as e:                                  # noqa: BLE001 — l'analisi non deve morire qui
        log.warning("struttura STEP non letta per %s: %s", percorso, e)
        s["avvisi"].append(f"struttura non letta: {type(e).__name__}: {e}"[:MAX_TESTO])
        s["stato"], s["motivo_stato"] = STATO_NON_ESTRAIBILE, f"lettura fallita: {type(e).__name__}"[:MAX_TESTO]
        return s
    finally:
        s["limiti"]["tempo_s"] = round(time.monotonic() - inizio, 3)


def _stato(s: dict) -> None:
    """Lo stato della struttura (diagnostica 1), dai fatti gia' letti e mai dal nome del file. `assente` non vuol
    dire «pezzo unico»: dice solo che nel file non c'e' nessuna occorrenza d'assieme."""
    if s.get("_non_part21"):
        s["stato"], s["motivo_stato"] = STATO_NON_ESTRAIBILE, "non e' un file STEP Part 21"
    elif s["limiti"]["troncato"]:
        s["stato"], s["motivo_stato"] = STATO_PARZIALE, f"lettura fermata sul tetto «{s['limiti']['motivo']}»"
    elif not s["relazioni"]:
        s["stato"] = STATO_ASSENTE
        s["motivo_stato"] = (f"{len(s['nodi'])} PRODUCT e nessuna occorrenza d'assieme (NAUO): struttura assente, che "
                             "non dice se il file e' un pezzo unico") if s["nodi"] else "nessun PRODUCT nel file"
    elif s["scarti"]["occorrenze_non_risolte"] or s["scarti"]["prodotti_senza_definizione"]:
        s["stato"] = STATO_PARZIALE
        s["motivo_stato"] = (f"{s['scarti']['occorrenze_non_risolte']} occorrenze non risolte, "
                             f"{s['scarti']['prodotti_senza_definizione']} PRODUCT senza definizione")
    else:
        s["stato"], s["motivo_stato"] = STATO_VALIDA, ""
    s.pop("_non_part21", None)


def _leggi(percorso: str, lim: dict, s: dict) -> dict:
    # latin-1 non fallisce mai su nessun byte, ed e' la codifica dichiarata da Part 21 per il testo
    # fuori dalle sequenze \X\: le stringhe vere si decodificano dopo, dalle loro sequenze.
    with open(percorso, "r", encoding="latin-1", errors="replace", newline="") as f:
        prima = f.readline()
        s["limiti"]["byte_letti"] = len(prima)
        if "ISO-10303-21" not in prima.upper():
            s["avvisi"].append("non e' un file STEP Part 21")
            s["_non_part21"] = True
            return s

        prodotti: dict[str, tuple[str, str, str]] = {}      # #ref → (id, name, description)
        formazioni: dict[str, tuple[str, str]] = {}         # #ref → (rev, #ref del prodotto)
        definizioni: dict[str, str] = {}                    # #ref → #ref della formazione
        nauo: list[tuple[str, str, str]] = []               # (#ref, #ref pd padre, #ref pd figlio)
        istanze: dict[str, tuple[str, str]] = {}            # diagnostica 1: #ref NAUO → (id, nome) dell'istanza
        geo = _Geometria()                                  # diagnostica 1: la catena della geometria
        troncati = 0
        scaduto = time.monotonic() + float(lim["tempo_max_s"])

        for ref, testo in _istanze(f, s, scaduto):
            nome, argomenti = _prima_entita_utile(testo)
            if nome is None:
                continue
            if nome == "FILE_SCHEMA":
                if not s["schema"]:
                    grezzo, t = _testo(argomenti[0] if argomenti else "")
                    troncati += t
                    s["schema"] = _nome_schema(grezzo)
                continue
            if not ref:
                continue                                    # entita' d'intestazione: non e' un'istanza
            if nome == "PRODUCT" and len(argomenti) >= 3:
                valori = []
                for a in argomenti[:3]:
                    v, t = _testo(a)
                    troncati += t
                    valori.append(v)
                prodotti[ref] = (valori[0], valori[1], valori[2])
                if len(prodotti) > int(lim["nodi_max"]):
                    prodotti.pop(ref)
                    _tronca(s, "nodi", f"oltre {int(lim['nodi_max'])} PRODUCT: letti i primi")
                    break
            elif nome == "PRODUCT_DEFINITION_FORMATION" and len(argomenti) >= 3:
                rev, t = _testo(argomenti[0])
                troncati += t
                formazioni[ref] = (rev, _riferimento(argomenti[2]))
            elif nome == "PRODUCT_DEFINITION" and len(argomenti) >= 3:
                definizioni[ref] = _riferimento(argomenti[2])
            elif nome == "NEXT_ASSEMBLY_USAGE_OCCURRENCE" and len(argomenti) >= 5:
                nauo.append((ref, _riferimento(argomenti[3]), _riferimento(argomenti[4])))
                if len(nauo) > int(lim["occorrenze_max"]):
                    nauo.pop()
                    _tronca(s, "occorrenze",
                            f"oltre {int(lim['occorrenze_max'])} occorrenze: lette le prime")
                    break
                id_ist, t1 = _testo(argomenti[0])
                nome_ist, t2 = _testo(argomenti[1])
                troncati += t1 + t2
                istanze[ref] = (id_ist, nome_ist)
            elif nome in ("PRODUCT_DEFINITION_SHAPE", "SHAPE_DEFINITION_REPRESENTATION", "SHAPE_REPRESENTATION",
                          "REPRESENTATION_RELATIONSHIP", "GEOMETRIA"):
                geo.aggiungi(nome, ref, argomenti, testo)
            # Il tempo si guarda anche qui, e non solo dentro `_istanze`: un esportatore che scrive
            # tutto il file su una riga sola non fa mai girare il controllo per righe, e l'unico
            # momento in cui si torna a passare di qua e' a ogni istanza che quella riga produce.
            if time.monotonic() > scaduto:
                _tronca(s, "tempo",
                        f"lettura interrotta dopo {float(lim['tempo_max_s']):g} s: letto ciò che c'era")
                break

    s["scarti"]["testi_troncati"] = troncati
    if troncati:
        s["avvisi"].append(f"{troncati} testi troncati a {MAX_TESTO} caratteri")
    if not prodotti:
        # «Non ce n'erano» e «non ci siamo arrivati» sono due cose diverse, e in molti esportatori le
        # occorrenze stanno PRIMA dei prodotti: fermarsi sul loro tetto vuol dire uscire con zero
        # nodi da un file che ne aveva duecento. Dirlo «nessun PRODUCT nel file» sarebbe falso.
        s["avvisi"].append("lettura fermata prima di qualsiasi PRODUCT" if s["limiti"]["troncato"]
                           else "nessun PRODUCT nel file")
        return s
    _componi(s, prodotti, formazioni, definizioni, nauo, istanze, geo)
    return s


class _Geometria:
    """La catena della geometria (diagnostica 1): forma del prodotto -> rappresentazione -> elementi, con le
    relazioni fra rappresentazioni SENZA trasformazione (quelle con la trasformazione mettono un pezzo dentro un
    assieme: non sono la geometria del padre). Si leggono solo i nomi delle entita' e i riferimenti, mai la geometria
    vera."""

    def __init__(self):
        self.forme: dict[str, str] = {}                    # #PDS -> #PD
        self.sdr: list[tuple[str, str]] = []                # (#PDS, #SR)
        self.elementi: dict[str, list[str]] = {}            # #SR -> [#elementi]
        self.legami: dict[str, set[str]] = {}               # #SR <-> #SR (senza trasformazione)
        self.genere: dict[str, str] = {}                    # #elemento -> solidi | superfici | curve | mappati

    def aggiungi(self, nome: str, ref: str, argomenti: list[str], testo: str) -> None:
        if not ref:
            return
        if nome == "PRODUCT_DEFINITION_SHAPE" and len(argomenti) >= 3:
            self.forme[ref] = _riferimento(argomenti[2])
        elif nome == "SHAPE_DEFINITION_REPRESENTATION" and len(argomenti) >= 2:
            self.sdr.append((_riferimento(argomenti[0]), _riferimento(argomenti[1])))
        elif nome == "SHAPE_REPRESENTATION" and len(argomenti) >= 2:
            a = argomenti[1].strip()
            self.elementi[ref] = [x for x in (_riferimento(v) for v in _argomenti(a[1:-1])) if x] \
                if a.startswith("(") and a.endswith(")") else []
        elif nome == "REPRESENTATION_RELATIONSHIP" and len(argomenti) >= 4:
            if "WITH_TRANSFORMATION" in testo.upper():
                return                                      # un posizionamento d'assieme, non la geometria del padre
            a, b = _riferimento(argomenti[2]), _riferimento(argomenti[3])
            if a and b:
                self.legami.setdefault(a, set()).add(b)
                self.legami.setdefault(b, set()).add(a)
        elif nome == "GEOMETRIA":
            m = _ENTITA.search(testo)
            g = GENERE_GEOMETRIA.get(m.group(1).upper()) if m else None
            if g:
                self.genere[ref] = g

    def per_definizione(self) -> dict[str, list[str]]:
        """#PD -> le rappresentazioni direttamente legate alla sua forma."""
        out: dict[str, list[str]] = {}
        for pds, sr in self.sdr:
            pd = self.forme.get(pds)
            if pd and sr:
                out.setdefault(pd, []).append(sr)
        return out

    def riassunto(self, rappresentazioni: list[str]) -> dict:
        """Il riassunto della geometria raggiunta dalle rappresentazioni (e dalle loro legate senza trasformazione)."""
        visti, frontiera = set(rappresentazioni), list(rappresentazioni)
        for _ in range(SALTI_RAPPRESENTAZIONE):
            nuovi = [b for a in frontiera for b in self.legami.get(a, ()) if b not in visti]
            visti.update(nuovi)
            frontiera = nuovi
        conti = {"solidi": 0, "superfici": 0, "curve": 0, "mappati": 0}
        elementi = set()
        for r in visti:
            elementi.update(self.elementi.get(r, ()))
        for e in elementi:
            g = self.genere.get(e)
            if g:
                conti[g] += 1
        rapp = sum(1 for r in visti if r in self.elementi)
        return {**conti, "rappresentazioni": rapp, "vuota": not any(conti.values())}


def _componi(s: dict, prodotti: dict, formazioni: dict, definizioni: dict, nauo: list, istanze: dict | None = None,
             geo: _Geometria | None = None) -> None:
    """Da entita' sciolte a grafo: un nodo per PRODUCT raggiunto, una relazione per coppia. Diagnostica 1: la
    geometria per nodo (da tutte le sue definizioni) e i nomi d'istanza delle NAUO per relazione."""
    istanze = istanze or {}
    geo = geo or _Geometria()
    rapp_di = geo.per_definizione()
    # Un PRODUCT diventa un nodo solo se una PRODUCT_DEFINITION arriva fino a lui: e' la definizione
    # che lo mette nel modello. Un PRODUCT orfano e' un residuo dell'esportatore.
    per_prodotto: dict[str, dict] = {}
    for rif_def, rif_form in definizioni.items():
        form = formazioni.get(rif_form)
        if not form:
            continue
        rev, rif_prod = form
        if rif_prod not in prodotti:
            continue
        v = per_prodotto.setdefault(rif_prod, {"definizioni": [], "formazioni": [], "revisioni": []})
        v["definizioni"].append(rif_def)
        v["formazioni"].append(rif_form)
        if rev and rev not in v["revisioni"]:
            v["revisioni"].append(rev)

    orfani = len(prodotti) - len(per_prodotto)
    s["scarti"]["prodotti_senza_definizione"] = max(orfani, 0)
    if orfani > 0:
        s["avvisi"].append(f"{orfani} PRODUCT senza PRODUCT_DEFINITION: ignorati")

    for rif in sorted(per_prodotto, key=_ordine):
        id_grezzo, nome_grezzo, descrizione = prodotti[rif]
        v = per_prodotto[rif]
        evidenza = {"entita": "PRODUCT", "riga": rif}
        if v["formazioni"]:
            evidenza["formation"] = v["formazioni"][0]
        if v["definizioni"]:
            evidenza["definition"] = v["definizioni"][0]
        if len(v["revisioni"]) > 1:
            # due formazioni con id diverso sullo stesso prodotto: si conserva la prima e si dice che
            # ce n'erano altre. Sceglierne una in silenzio sarebbe inventare la revisione.
            evidenza["rev_alternative"] = v["revisioni"][1:]
        s["nodi"].append({
            "chiave": rif,
            "id_grezzo": id_grezzo,
            "nome_grezzo": nome_grezzo,
            "descrizione_grezza": descrizione,
            "rev_grezza": v["revisioni"][0] if v["revisioni"] else "",
            "evidenza": evidenza,
            "geometria": geo.riassunto([r for d in v["definizioni"] for r in rapp_di.get(d, ())]),
        })

    coppie: dict[tuple[str, str], dict] = {}
    non_risolte = 0
    anelli = 0
    for rif, rif_padre, rif_figlio in nauo:
        padre = _prodotto_di(rif_padre, definizioni, formazioni)
        figlio = _prodotto_di(rif_figlio, definizioni, formazioni)
        if padre not in per_prodotto or figlio not in per_prodotto:
            non_risolte += 1
            continue
        if padre == figlio:
            anelli += 1                                     # un pezzo dentro se stesso non esiste
            continue
        c = coppie.setdefault((padre, figlio), {"qta": 0, "righe": []})
        c["qta"] += 1
        if len(c["righe"]) < 20:
            c["righe"].append(rif)
    s["scarti"]["occorrenze_non_risolte"] = non_risolte
    s["scarti"]["occorrenze_su_se_stesse"] = anelli
    if non_risolte:
        s["avvisi"].append(f"{non_risolte} occorrenze con estremi non risolti: ignorate")
    if anelli:
        s["avvisi"].append(f"{anelli} occorrenze di un pezzo dentro se stesso: ignorate")

    for (padre, figlio) in sorted(coppie, key=lambda k: (_ordine(k[0]), _ordine(k[1]))):
        c = coppie[(padre, figlio)]
        evidenza = {"entita": "NEXT_ASSEMBLY_USAGE_OCCURRENCE", "righe": c["righe"]}
        if c["qta"] > len(c["righe"]):
            evidenza["altre"] = c["qta"] - len(c["righe"])
        ist = [{"rif": r, "id": istanze.get(r, ("", ""))[0], "nome": istanze.get(r, ("", ""))[1]}
               for r in c["righe"][:ISTANZE_MAX]]
        s["relazioni"].append({"padre": padre, "figlio": figlio, "qta": c["qta"], "evidenza": evidenza,
                               "istanze": ist})

    con_padre = {f for _, f in coppie}
    s["radici"] = [n["chiave"] for n in s["nodi"] if n["chiave"] not in con_padre]
    if not s["relazioni"]:
        s["avvisi"].append("nessuna occorrenza di assieme: solo parti")


def _prodotto_di(rif_definizione: str, definizioni: dict, formazioni: dict) -> str:
    form = formazioni.get(definizioni.get(rif_definizione, ""))
    return form[1] if form else ""


def _ordine(rif: str) -> int:
    try:
        return int(rif.lstrip("#"))
    except ValueError:
        return 0


def _tronca(s: dict, motivo: str, avviso: str) -> None:
    """Segna che il grafo e' PARZIALE, e contro quale tetto si e' fermato.

    Il primo motivo vince: se la lettura si e' fermata sui nodi, il tempo che scade un istante dopo
    non e' la ragione per cui manca il resto del file, ed e' la prima che chi guarda deve vedere.
    """
    if s["limiti"]["troncato"]:
        return
    s["limiti"]["troncato"] = True
    s["limiti"]["motivo"] = motivo
    s["avvisi"].append(avviso)


# ---------------------------------------------------------------- il tokenizer Part 21


def _istanze(f, s: dict, scaduto: float):
    """Genera `(#ref, testo)` per ogni istanza del file, riga per riga.

    Un'istanza e' `#id = ENTITA(attributi);` e puo' occupare piu' righe. Il `;` dentro una stringa non
    la chiude, l'apice raddoppiato `''` non la apre, e i commenti `/* */` possono stare ovunque: sono
    le tre cose per cui un `split(";")` non basta e serve questo giro.

    Il tempo si controlla QUI, e non solo da chi consuma: in uno STEP vero la geometria e' il 99% del
    file e non produce nessuna istanza utile, quindi un controllo fatto solo a ogni `yield` potrebbe
    non arrivare mai. Un file di soli triangoli deve fermarsi al suo tetto come tutti gli altri.
    """
    buf: list[str] = []
    in_stringa = False
    in_commento = False
    letti = s["limiti"]["byte_letti"]
    al_controllo = RIGHE_FRA_CONTROLLI
    for linea in f:
        letti += len(linea)
        s["limiti"]["byte_letti"] = letti      # anche se il chiamante si ferma a meta', il conto e' vero
        al_controllo -= 1
        if al_controllo <= 0:
            al_controllo = RIGHE_FRA_CONTROLLI
            if time.monotonic() > scaduto:
                _tronca(s, "tempo",
                        f"lettura interrotta dopo {s['limiti']['tempo_max_s']:g} s: letto ciò che c'era")
                return
        # Via veloce, ed e' quella che fa quasi tutto il lavoro: una riga di geometria non ha apici
        # ne' commenti, chiude ciò che apre e non nomina nessuna delle entita' che servono. Si salta
        # intera, senza ricomporre niente.
        if not buf and not in_stringa and not in_commento and "'" not in linea and "/*" not in linea and "*/" not in linea:
            pezzi = linea.split(";")
            resto = pezzi.pop().strip()
            if any(e in linea for e in ENTITA_UTILI):
                for p in pezzi:
                    if any(e in p for e in ENTITA_UTILI):
                        yield _spezza(p)
            if resto:
                buf.append(resto)
            continue

        i, n = 0, len(linea)
        while i < n:
            if in_commento:
                j = linea.find("*/", i)
                if j < 0:
                    i = n
                else:
                    i, in_commento = j + 2, False
                continue
            if in_stringa:
                j = linea.find("'", i)
                if j < 0:
                    buf.append(linea[i:])
                    i = n
                elif j + 1 < n and linea[j + 1] == "'":
                    buf.append(linea[i:j + 2])               # apice raddoppiato: la stringa continua
                    i = j + 2
                else:
                    buf.append(linea[i:j + 1])
                    i, in_stringa = j + 1, False
                continue
            m = _DELIM.search(linea, i)
            if m is None:
                buf.append(linea[i:])
                i = n
                continue
            buf.append(linea[i:m.start()])
            i = m.end()
            simbolo = m.group(0)
            if simbolo == "'":
                buf.append("'")
                in_stringa = True
            elif simbolo == "/*":
                in_commento = True
            elif simbolo == ";":
                testo = "".join(buf).strip()
                buf = []
                if testo and any(e in testo for e in ENTITA_UTILI):
                    yield _spezza(testo)
        if buf:
            buf.append(" ")                                  # le righe si ricompongono separate


def _spezza(testo: str) -> tuple[str, str]:
    m = _ISTANZA.match(testo.strip())
    if m:
        return "#" + m.group(1), m.group(2).strip()
    return "", testo.strip()


def _prima_entita_utile(testo: str) -> tuple[str | None, list[str]]:
    """Nome e argomenti della prima entita' utile dell'istanza.

    Un'istanza complessa e' `#7 = (PRODUCT_DEFINITION(...) ALTRO(...))`: gli esportatori la usano, e
    l'entita' che interessa puo' essere la seconda.
    """
    for m in _ENTITA.finditer(testo):
        nome = ENTITA.get(m.group(1).upper())
        if nome is None:
            continue
        chiuso = _fine_parentesi(testo, m.end() - 1)
        if chiuso < 0:
            continue
        return nome, _argomenti(testo[m.end():chiuso])
    return None, []


def _fine_parentesi(testo: str, apertura: int) -> int:
    """L'indice della parentesi che chiude quella in `apertura`, ignorando quelle dentro le stringhe."""
    livello = 0
    i, n = apertura, len(testo)
    while i < n:
        c = testo[i]
        if c == "'":
            i += 1
            while i < n:
                if testo[i] == "'":
                    if i + 1 < n and testo[i + 1] == "'":
                        i += 2
                        continue
                    break
                i += 1
        elif c == "(":
            livello += 1
        elif c == ")":
            livello -= 1
            if livello == 0:
                return i
        i += 1
    return -1


def _argomenti(grezzo: str) -> list[str]:
    """Gli argomenti di primo livello: le virgole dentro stringhe e parentesi non separano niente."""
    fuori: list[str] = []
    pezzo: list[str] = []
    livello = 0
    i, n = 0, len(grezzo)
    while i < n:
        c = grezzo[i]
        if c == "'":
            pezzo.append(c)
            i += 1
            while i < n:
                pezzo.append(grezzo[i])
                if grezzo[i] == "'":
                    if i + 1 < n and grezzo[i + 1] == "'":
                        pezzo.append(grezzo[i + 1])
                        i += 2
                        continue
                    i += 1
                    break
                i += 1
            continue
        if c in "([":
            livello += 1
        elif c in ")]":
            livello -= 1
        elif c == "," and livello == 0:
            fuori.append("".join(pezzo).strip())
            pezzo = []
            i += 1
            continue
        pezzo.append(c)
        i += 1
    fuori.append("".join(pezzo).strip())
    return fuori


def _riferimento(argomento: str) -> str:
    a = argomento.strip()
    return a if a.startswith("#") and a[1:].isdigit() else ""


def _testo(argomento: str) -> tuple[str, int]:
    """Il valore di un attributo stringa, decodificato. Il secondo valore dice se e' stato troncato.

    `$` (non impostato) e `*` (derivato) sono attributi VUOTI, non i caratteri che si vedono: un
    dollaro finito in `nome_grezzo` sarebbe un nome inventato dal parser.

    Gli spazi ai bordi si tolgono, e non e' una lettura: un esportatore vero scrive la revisione
    assente come `' '` e un altro come `''`, e sono la stessa cosa detta in due modi. Lasciare il
    primo passare significherebbe che `rev_grezza` a volte e' «vuota» e a volte «uno spazio», e che
    ogni lettore piu' avanti debba sapere che sono uguali. Dentro la stringa non si tocca niente.
    """
    a = argomento.strip()
    if a.startswith("(") and a.endswith(")"):
        interni = _argomenti(a[1:-1])                        # FILE_SCHEMA(('AUTOMOTIVE_DESIGN…'))
        return _testo(interni[0]) if interni else ("", 0)
    if not a.startswith("'") or not a.endswith("'") or len(a) < 2:
        return "", 0
    valore = _decodifica(a[1:-1].replace("''", "'")).strip()
    if len(valore) > MAX_TESTO:
        return valore[:MAX_TESTO], 1
    return valore, 0


def _decodifica(testo: str) -> str:
    """Le sequenze di controllo di Part 21 (\\X\\, \\X2\\, \\X4\\, \\S\\) tornano testo.

    Non si normalizza altro: gli spazi, le maiuscole e i trattini restano come li ha scritti chi ha
    esportato il file. Quello che sembra un codice lo dira' il server.
    """
    if "\\" not in testo:
        return testo

    def sostituisci(m: re.Match) -> str:
        if m.group(1) is not None:
            cifre = m.group(1)
            return "".join(chr(int(cifre[i:i + 4], 16)) for i in range(0, len(cifre) - 3, 4))
        if m.group(2) is not None:
            cifre = m.group(2)
            return "".join(chr(int(cifre[i:i + 8], 16)) for i in range(0, len(cifre) - 7, 8))
        if m.group(3) is not None:
            return chr(int(m.group(3), 16))
        if m.group(4) is not None:
            return chr(ord(m.group(4)) + 128)
        return " " if m.group(0) in ("\\N\\", "\\T\\") else m.group(0)

    try:
        return _ESCAPE.sub(sostituisci, testo)
    except ValueError:
        return testo


def _nome_schema(grezzo: str) -> str:
    nome = grezzo.strip().split("{")[0].strip().upper()
    return SCHEMI.get(nome, nome[:60])
