"""Le TABELLE di una pagina, dalle parole con le coordinate (GP-3, direttiva 21 §5.1 e §5.2; PM-35R).

Un fatto generale, non legato al nome del file: qualunque PDF con testo nativo (un modulo di richiesta, una distinta,
l'elenco particolari di un disegno d'assieme) puo' avere una tabella, e la tabella si riconosce dalla sua
INTESTAZIONE, non da chi l'ha mandata. Il worker estrae le righe; che una riga sia un prodotto richiesto, un
componente o una menzione lo decide il server (estrazione e intento restano separati, PM-35R).

La regola, geometrica:

  - le parole si raggruppano per riga vista (stesso centro verticale entro mezza altezza);
  - una riga e' un'INTESTAZIONE quando le sue celle (parole vicine, a meno di un'altezza l'una dall'altra) non hanno
    cifre, ci sono almeno due ruoli diversi di colonna (colonne.ruolo_colonna) e c'e' un ruolo «forte»: la posizione o
    la quantita', oppure codice e descrizione insieme. Due intestazioni attaccate con un ruolo ciascuna («quantity
    un») sono due celle;
  - una parola delle righe sotto va nella colonna la cui intestazione le si sovrappone di piu'; se cade nello spazio
    fra due intestazioni, va nella colonna a sinistra (il seguito di una descrizione lunga sotto un'intestazione
    corta). I confini a meta' fra due intestazioni servono solo a riconoscere le parole «a cavallo»;
  - le righe di dati sono quelle sotto l'intestazione, una dopo l'altra, finche' (a) arriva un'altra intestazione,
    (b) c'e' un salto verticale piu' grande di 2,5 volte il passo piu' piccolo visto fra due righe (e di 4 altezze
    per la prima riga), (c) una riga ha almeno due parole a cavallo di un confine di colonna (un paragrafo, non una
    tabella), (d) finisce la pagina;
  - una riga senza la cella della posizione (o, senza colonna della posizione, senza quella del codice) e' la
    CONTINUAZIONE della riga sopra (una descrizione su due righe): si riporta com'e', segnata, senza fonderla.

Il valore di una cella e' il testo grezzo delle sue parole, nell'ordine da sinistra a destra, con uno spazio fra
l'una e l'altra; il riquadro e' quello delle sue parole. Le colonne «personali» (colonne.colonna_personale) hanno il
valore vuoto e `omessa: true`.

I riquadri sono nelle coordinate di chi chiama (per i PDF: punti sulla pagina vista, y verso il basso).
"""
from __future__ import annotations

from colonne import (CODICE, DESCRIZIONE, POSIZIONE, QUANTITA, colonna_personale, ha_cifre, ruolo_colonna,
                     ruolo_di_parola)

LIMITI_TABELLE = {
    "tabelle_max": 20,      # tabelle nel fatto
    "righe_max": 400,       # righe di dati, in tutte le tabelle insieme
    "colonne_max": 30,      # colonne di una tabella
    "cella_max": 200,       # caratteri del valore di una cella
}


def _cx(p) -> float:
    return (p.x0 + p.x1) / 2


def _cy(p) -> float:
    return (p.y0 + p.y1) / 2


def _h(p) -> float:
    return max(p.y1 - p.y0, 0.1)


def _filetto(testo: str) -> bool:
    """Una «parola» fatta solo di trattini, sottolineature, uguali o puntini: una linea della tabella disegnata con
    il testo, non un valore."""
    t = testo.strip()
    return len(t) >= 3 and all(c in "_-=.–—·" for c in t)


def righe_viste(parole: list) -> list[list]:
    """Le parole per riga vista, ognuna da sinistra a destra (come worker_analisi.righe_viste). Le linee fatte di
    caratteri (_filetto) non contano."""
    righe: list[list] = []
    for p in sorted((q for q in parole if not _filetto(q.testo)), key=lambda q: (_cy(q), q.x0)):
        if righe:
            u = righe[-1]
            cu = sum(_cy(q) for q in u) / len(u)
            if abs(_cy(p) - cu) <= max(_h(q) for q in u) / 2:
                u.append(p)
                continue
        righe.append([p])
    return [sorted(r, key=lambda q: q.x0) for r in righe]


def _celle_intestazione(riga: list) -> list[list]:
    """Le celle di una riga candidata intestazione: parole a meno di mezza altezza l'una dall'altra stanno insieme
    (lo spazio fra due parole della stessa intestazione), salvo due parole che hanno ciascuna un ruolo suo, diverso
    («quantity un»)."""
    celle: list[list] = []
    for p in riga:
        if celle:
            c = celle[-1]
            vicino = p.x0 - c[-1].x1 <= 0.5 * max(_h(p), _h(c[-1]))
            if vicino:
                r_prima = ruolo_di_parola(" ".join(w.testo for w in c))
                r_dopo = ruolo_di_parola(p.testo)
                if not (r_prima and r_dopo and r_prima != r_dopo):
                    c.append(p)
                    continue
        celle.append([p])
    return celle


def intestazione(riga: list) -> list[dict] | None:
    """Le colonne, se la riga e' un'intestazione (docstring del modulo); None altrimenti."""
    celle = _celle_intestazione(riga)
    if len(celle) < 2:
        return None
    testi = [" ".join(w.testo for w in c) for c in celle]
    if any(ha_cifre(t) for t in testi):
        return None
    ruoli = [ruolo_colonna(t) for t in testi]
    distinti = {r for r in ruoli if r}
    if len(distinti) < 2:
        return None
    if not (POSIZIONE in distinti or QUANTITA in distinti or {CODICE, DESCRIZIONE} <= distinti):
        return None
    return [{"indice": i, "intestazione": t, "ruolo": r, "personale": colonna_personale(t),
             "x0": min(w.x0 for w in c), "x1": max(w.x1 for w in c),
             "riquadro": _riquadro(c)} for i, (c, t, r) in enumerate(zip(celle, testi, ruoli))]


def _riquadro(parole: list) -> list[float]:
    return [round(min(p.x0 for p in parole), 1), round(min(p.y0 for p in parole), 1),
            round(max(p.x1 for p in parole), 1), round(max(p.y1 for p in parole), 1)]


def _confini(colonne: list[dict]) -> list[float]:
    """I confini interni fra colonne consecutive: a meta' fra la fine di una cella e l'inizio della successiva."""
    return [(a["x1"] + b["x0"]) / 2 for a, b in zip(colonne, colonne[1:])]


def _colonna_di(p, colonne: list[dict], prima=None) -> int:
    """La colonna di una parola: quella la cui intestazione si sovrappone di piu' alla parola (un valore allineato a
    sinistra comincia sotto l'inizio dell'intestazione, uno allineato a destra finisce sotto la sua fine). Una parola
    che cade nello spazio fra due intestazioni e' il seguito della parola prima sulla stessa riga, se le sta attaccata
    (a meno di un'altezza: la stessa frase, una descrizione lunga sotto un'intestazione corta, «Name»); altrimenti va
    all'intestazione piu' vicina (a parita', quella a sinistra). `prima` e' (parola, colonna) della parola prima."""
    migliore, sovrapposta = -1, 0.0
    for c in colonne:
        s = min(p.x1, c["x1"]) - max(p.x0, c["x0"])
        if s > sovrapposta:
            migliore, sovrapposta = c["indice"], s
    if migliore >= 0:
        return migliore
    if prima is not None and p.x0 - prima[0].x1 <= max(_h(p), _h(prima[0])):
        return prima[1]
    sinistra = [c for c in colonne if c["x1"] <= p.x0]
    destra = [c for c in colonne if c["x0"] >= p.x1]
    if not sinistra:
        return destra[0]["indice"] if destra else colonne[0]["indice"]
    if not destra:
        return sinistra[-1]["indice"]
    s, d = sinistra[-1], destra[0]
    return s["indice"] if p.x0 - s["x1"] <= d["x0"] - p.x1 else d["indice"]


def _a_cavallo(riga: list, confini: list[float]) -> int:
    """Quante parole della riga attraversano un confine di colonna (con un margine di un quarto d'altezza)."""
    n = 0
    for p in riga:
        tol = 0.25 * _h(p)
        if any(p.x0 < c - tol and p.x1 > c + tol for c in confini):
            n += 1
    return n


def _seguito_intestazione(riga: list, colonne: list[dict]) -> bool:
    """La riga subito sotto l'intestazione ne e' la seconda riga (le gemelle in un'altra lingua: «Pos. Q.ty'
    Thickness Weight» sotto «Pos. Q.ta Spessore Peso»): nessuna cifra, almeno due parole con lo stesso ruolo della
    colonna in cui cadono, e nessuna con un ruolo diverso da quello della sua colonna (l'intestazione di un'altra
    tabella subito sotto, «REV MODIFICA MOD. N°», non e' una seconda riga)."""
    if len(riga) < 2 or any(ha_cifre(p.testo) for p in riga):
        return False
    concordi = 0
    for p in riga:
        r = ruolo_colonna(p.testo)
        if not r:
            continue
        rc = colonne[_colonna_di(p, colonne)]["ruolo"]
        if rc and rc != r:
            return False
        concordi += rc == r
    return concordi >= 2


def _riga_dati(r: list, colonne: list[dict], chiave: int | None, lim: dict) -> tuple[dict, bool]:
    """Una riga di dati: le parole nelle colonne, il valore di ogni cella, la continuazione. Il secondo valore dice se
    una cella e' stata troncata."""
    per_colonna: dict[int, list] = {}
    prima = None
    for p in r:
        k = _colonna_di(p, colonne, prima)
        per_colonna.setdefault(k, []).append(p)
        prima = (p, k)
    celle, tronca = [], False
    for k in sorted(per_colonna):
        col = colonne[k]
        ps = per_colonna[k]
        valore = " ".join(p.testo for p in ps)
        if len(valore) > lim["cella_max"] and not col["personale"]:
            tronca = True
        celle.append({"colonna": k, "intestazione": col["intestazione"], "ruolo": col["ruolo"],
                      "valore": "" if col["personale"] else valore[:lim["cella_max"]],
                      "riquadro": _riquadro(ps), "omessa": col["personale"],
                      "sorgente": sorted({int(s) for p in ps for s in (getattr(p, "sorgente", None) or ())})})
    # senza la cella chiave, o con una cella chiave senza cifre (l'intestazione di un'altra tabella, una nota), la riga
    # non apre una voce: e' una continuazione
    continuazione = chiave is not None and (chiave not in per_colonna
                                            or not any(ha_cifre(p.testo) for p in per_colonna[chiave]))
    return {"indice": 0, "riquadro": _riquadro(r), "continuazione": continuazione, "celle": celle}, tronca


def _leggi_righe(righe: list[list], da: int, verso: int, colonne: list[dict], cy0: float, h_testa: float,
                 chiave: int | None, lim: dict, disponibili: int) -> tuple[list[dict], bool, int]:
    """Le righe di dati da righe[da] nel verso dato (+1 sotto, -1 sopra), con le regole di fine del modulo. Restituisce
    le righe, se c'e' stato un troncamento e l'indice dove ci si e' fermati."""
    confini = _confini(colonne)
    ultima_cy = cy0
    passo_min: float | None = None
    dati: list[dict] = []
    tronca = False
    j = da
    while 0 <= j < len(righe):
        r = righe[j]
        if intestazione(r) is not None:
            break
        cy = sum(_cy(p) for p in r) / len(r)
        passo = abs(cy - ultima_cy)
        soglia = 5 * h_testa if passo_min is None else max(2.5 * passo_min, 2 * h_testa)
        if passo > soglia or _a_cavallo(r, confini) >= 2:
            break
        if len(dati) >= disponibili:
            tronca = True
            break
        riga, t = _riga_dati(r, colonne, chiave, lim)
        tronca = tronca or t
        riga["indice"] = len(dati) + 1
        dati.append(riga)
        passo_min = passo if passo_min is None else min(passo_min, passo)
        ultima_cy = cy
        j += verso
    return dati, tronca, j


def trova_tabelle(parole: list, pagina: int, limiti: dict | None = None, gia: int = 0) -> tuple[list[dict], bool]:
    """Le tabelle della pagina. `gia` sono le righe gia' riportate (dalle pagine prima): il tetto righe_max vale per
    tutto il documento. Restituisce le tabelle e se ci si e' fermati a un tetto.

    Le righe si cercano prima SOTTO l'intestazione (un modulo, una distinta); se sotto non ce n'e' nessuna che non sia
    una continuazione, SOPRA (l'elenco particolari di un disegno, con l'intestazione in basso, appoggiata al
    cartiglio): `verso` lo dice, e le righe sono numerate dall'intestazione in fuori."""
    lim = dict(LIMITI_TABELLE)
    lim.update(limiti or {})
    righe = righe_viste(parole)
    tabelle: list[dict] = []
    troncato = False
    righe_tot = gia
    i = 0
    while i < len(righe):
        colonne = intestazione(righe[i])
        if colonne is None:
            i += 1
            continue
        if len(tabelle) >= lim["tabelle_max"]:
            return tabelle, True
        if len(colonne) > lim["colonne_max"]:
            colonne, troncato = colonne[:lim["colonne_max"]], True
        testa = list(righe[i])
        j = i + 1
        if j < len(righe) and _seguito_intestazione(righe[j], colonne):
            for p in righe[j]:
                c = colonne[_colonna_di(p, colonne)]
                c["intestazione"] += " / " + p.testo
                c["personale"] = c["personale"] or colonna_personale(p.testo)
            testa += righe[j]
            j += 1
        h_testa = max(_h(p) for p in testa)
        pos = next((c["indice"] for c in colonne if c["ruolo"] == POSIZIONE), None)
        cod = next((c["indice"] for c in colonne if c["ruolo"] == CODICE), None)
        chiave = pos if pos is not None else cod
        disponibili = max(lim["righe_max"] - righe_tot, 0)
        cy_sotto = sum(_cy(p) for p in righe[j - 1]) / len(righe[j - 1])
        dati, tronca, fine = _leggi_righe(righe, j, +1, colonne, cy_sotto, h_testa, chiave, lim, disponibili)
        verso = "sotto"
        if not any(not d["continuazione"] for d in dati):
            cy_sopra = sum(_cy(p) for p in righe[i]) / len(righe[i])
            sopra, t2, _ = _leggi_righe(righe, i - 1, -1, colonne, cy_sopra, h_testa, chiave, lim, disponibili)
            if any(not d["continuazione"] for d in sopra):
                dati, tronca, verso, fine = sopra, t2, "sopra", j
        troncato = troncato or tronca
        if any(not d["continuazione"] for d in dati):
            righe_tot += len(dati)
            tabelle.append({"pagina": pagina, "indice": len(tabelle) + 1, "verso": verso,
                            "riquadro_intestazione": _riquadro(testa),
                            "colonne": [{"indice": c["indice"], "intestazione": c["intestazione"], "ruolo": c["ruolo"],
                                         "omessa": c["personale"], "riquadro": c["riquadro"]} for c in colonne],
                            "righe": dati})
        i = max(fine, i + 1)
        if righe_tot >= lim["righe_max"]:
            if tronca:
                troncato = True
            break
    return tabelle, troncato