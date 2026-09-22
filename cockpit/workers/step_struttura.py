"""Lettura della STRUTTURA di un file STEP (ISO 10303-21): nodi, relazioni, quantita'.

Che cosa fa e che cosa NON fa. Questo modulo legge il file e dice che cosa c'e' scritto: quali
`PRODUCT` esistono, quale contiene quale e quante volte. Non decide se un nome e' un codice, non
separa una revisione, non sa che cosa sia una famiglia Manitou o Krone. Quella e' una lettura che
dipende dal CLIENTE della richiesta, e il cliente il worker non lo conosce: la fa il server, con le
regole gia' scritte per oggetto, corpo e nomi file (addendum B8, A1.2).

Percio' ogni nodo porta i quattro attributi grezzi dell'entita' e l'evidenza di dove stanno:

    {"chiave": "#12", "id_grezzo": "52922757", "nome_grezzo": "52922757_B",
     "descrizione_grezza": "SUPPORTO COFANO", "rev_grezza": "B",
     "evidenza": {"entita": "PRODUCT", "riga": "#12", "formation": "#14", "definition": "#20"}}

Nodo e relazione sono separati, come in database (`componente` e `componente_relazione`): un
sottoassieme che compare sotto due padri e' UN nodo e DUE relazioni. Un modello che mettesse il padre
sulla riga del nodo non saprebbe rappresentarlo (A1.1).

Nessuna dipendenza nuova: Part 21 e' testo, e per leggerne quattro entita' basta un tokenizer di
poche decine di righe. Non si legge geometria, non si leggono trasformazioni, non si risolvono
riferimenti esterni (AP242 multi-file): quei nodi compaiono come `PRODUCT` senza figli.
"""
from __future__ import annotations

import logging
import re
import time

log = logging.getLogger("worker-analisi")

VERSIONE_STRUTTURA = 2

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
ENTITA_UTILI = ("PRODUCT", "NEXT_ASSEMBLY_USAGE_OCCURRENCE", "FILE_SCHEMA")

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
}

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
        "schema": "",
        "radici": [],
        "nodi": [],
        "relazioni": [],
        "avvisi": [],
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
    inizio = time.monotonic()
    try:
        return _leggi(percorso, lim, s)
    except Exception as e:                                  # noqa: BLE001 — l'analisi non deve morire qui
        log.warning("struttura STEP non letta per %s: %s", percorso, e)
        s["avvisi"].append(f"struttura non letta: {type(e).__name__}: {e}"[:MAX_TESTO])
        return s
    finally:
        s["limiti"]["tempo_s"] = round(time.monotonic() - inizio, 3)


def _leggi(percorso: str, lim: dict, s: dict) -> dict:
    # latin-1 non fallisce mai su nessun byte, ed e' la codifica dichiarata da Part 21 per il testo
    # fuori dalle sequenze \X\: le stringhe vere si decodificano dopo, dalle loro sequenze.
    with open(percorso, "r", encoding="latin-1", errors="replace", newline="") as f:
        prima = f.readline()
        s["limiti"]["byte_letti"] = len(prima)
        if "ISO-10303-21" not in prima.upper():
            s["avvisi"].append("non e' un file STEP Part 21")
            return s

        prodotti: dict[str, tuple[str, str, str]] = {}      # #ref → (id, name, description)
        formazioni: dict[str, tuple[str, str]] = {}         # #ref → (rev, #ref del prodotto)
        definizioni: dict[str, str] = {}                    # #ref → #ref della formazione
        nauo: list[tuple[str, str, str]] = []               # (#ref, #ref pd padre, #ref pd figlio)
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
            # Il tempo si guarda anche qui, e non solo dentro `_istanze`: un esportatore che scrive
            # tutto il file su una riga sola non fa mai girare il controllo per righe, e l'unico
            # momento in cui si torna a passare di qua e' a ogni istanza che quella riga produce.
            if time.monotonic() > scaduto:
                _tronca(s, "tempo",
                        f"lettura interrotta dopo {float(lim['tempo_max_s']):g} s: letto ciò che c'era")
                break

    if troncati:
        s["avvisi"].append(f"{troncati} testi troncati a {MAX_TESTO} caratteri")
    if not prodotti:
        # «Non ce n'erano» e «non ci siamo arrivati» sono due cose diverse, e in molti esportatori le
        # occorrenze stanno PRIMA dei prodotti: fermarsi sul loro tetto vuol dire uscire con zero
        # nodi da un file che ne aveva duecento. Dirlo «nessun PRODUCT nel file» sarebbe falso.
        s["avvisi"].append("lettura fermata prima di qualsiasi PRODUCT" if s["limiti"]["troncato"]
                           else "nessun PRODUCT nel file")
        return s
    _componi(s, prodotti, formazioni, definizioni, nauo)
    return s


def _componi(s: dict, prodotti: dict, formazioni: dict, definizioni: dict, nauo: list) -> None:
    """Da entita' sciolte a grafo: un nodo per PRODUCT raggiunto, una relazione per coppia."""
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
    if non_risolte:
        s["avvisi"].append(f"{non_risolte} occorrenze con estremi non risolti: ignorate")
    if anelli:
        s["avvisi"].append(f"{anelli} occorrenze di un pezzo dentro se stesso: ignorate")

    for (padre, figlio) in sorted(coppie, key=lambda k: (_ordine(k[0]), _ordine(k[1]))):
        c = coppie[(padre, figlio)]
        evidenza = {"entita": "NEXT_ASSEMBLY_USAGE_OCCURRENCE", "righe": c["righe"]}
        if c["qta"] > len(c["righe"]):
            evidenza["altre"] = c["qta"] - len(c["righe"])
        s["relazioni"].append({"padre": padre, "figlio": figlio, "qta": c["qta"], "evidenza": evidenza})

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
