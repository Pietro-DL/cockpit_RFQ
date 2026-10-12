/* ================= 16 · le etichette del server =================
   Copia della tabella di Pietro (internal/transport/web/motore_a_etichette.go, 62 domini). Ogni valore di enum della
   zona A passa da etichetta(dominio, valore). Un valore che la tabella non conosce esce fra parentesi quadre: si vede,
   non sparisce (T-A1d-29). Le etichette le scrive il backend: il frontend ne PROPONE il testo (ETICHETTE_PROPOSTE, in
   fondo), non aggiunge mappature sue alle tabelle vere. */
const ETI_TIPO_DOC = {
	"cad_3d":            "3D",
	"disegno_2d":        "2D",
	"sviluppo_dxf":      "DXF",
	"capitolato":        "capitolato",
	"distinta_cliente":  "distinta del cliente",
	"commerciale":       "documento commerciale",
	"offerta_fornitore": "offerta del fornitore",
	"offerta_promatec":  "offerta Promatec",
	"ordine_cliente":    "ordine del cliente",
	"corrispondenza":    "corrispondenza",
	"rumore":            "rumore",
	"altro":             "altro",
	"da_determinare":    "da determinare",
};
const ETICHETTE = {

	"stato_prodotto": {
		"non_pronto":         "non pronto",
		"pronto_fattibilita": "pronto per la fattibilità",
		"da_riesaminare":     "da riesaminare",
	},

	"motivo_prodotto": {
		"target_non_confermato":                            "prodotto della RFQ non confermato",
		"fonte_strutturale_step_mancante_o_non_confermata": "STEP strutturale mancante o non confermato",
		"fonte_superata":                                   "STEP strutturale superato",
		"nomenclatura_non_verificata":                      "codici non verificati",
		"gerarchia_non_verificata":                         "albero non verificato",
		"smistamento_non_verificato":                       "smistamento dei file non verificato",
		"documentazione_incompleta":                        "documenti incompleti",
		"documentazione_non_calcolabile":                   "documenti non calcolabili",
		"conflitto":                                        "conflitto da risolvere",
		"revisione_tecnica_aperta":                         "revisione tecnica aperta",
	},

	"stato_fonte": {
		"assente":               "nessuna fonte",
		"in_attesa_di_conferma": "in attesa di conferma",
		"confermata":            "confermata",
	},

	"motivo_fonte": {
		"estrazione_riuscita":          "STEP letto",
		"dati_insufficienti":           "dati insufficienti",
		"analisi_in_corso":             "analisi in corso",
		"non_analizzata":               "STEP non ancora analizzato",
		"riferimento_superato":         "lo STEP confermato è superato",
		"da_scegliere":                 "più STEP: si sceglie quale",
		"proposta_3d_aperta":           "STEP proposto, non confermato",
		"indicata_sul_portale":         "indicato sul portale",
		"solo_altro_3d":                "solo un altro file 3D",
		"nessuna_fonte":                "nessuno STEP",
		"documento_candidato":          "STEP candidato",
		"estrazione_fallita":           "lettura dello STEP non riuscita",
		"solo_nodo_interno":            "solo come parte di un altro STEP",
		"prodotto_senza_componente":    "prodotto senza componente",
		"non_determinabile":            "non determinabile",
		"step_presente_non_analizzato": "STEP presente, non analizzato",
	},

	"stato_verifica_asse": {
		"da_verificare":    "da verificare",
		"verificata":       "verificata",
		"conflitto":        "in conflitto",
		"non_verificabile": "non verificabile",
	},

	"stato_smistamento": {
		"da_verificare": "da verificare",
		"verificato":    "verificato",
		"conflitto":     "in conflitto",
		"in_revisione":  "in revisione",
	},

	"motivo_smistamento": {
		"file_da_smistare":                "file da smistare",
		"nessun_candidato":                "nessun componente candidato",
		"associazione_ambigua":            "associazione ambigua",
		"associazione_discordante":        "associazione discordante",
		"collocazione_non_determinabile":  "posizione non determinabile",
		"fuori_richiesta_non_confermato":  "fuori richiesta, non confermato",
		"associazione_non_confermata":     "associazione non confermata",
		"associazione_in_conflitto":       "associazione in conflitto",
		"nuovo_file_su_componente_deciso": "file nuovo su un componente già deciso",
		"revisione_tecnica_aperta":        "revisione tecnica aperta",
		"identita_documento_in_conflitto": "identità del documento in conflitto",
	},

	"stato_documenti": {
		"non_calcolabile": "non calcolabile",
		"incompleta":      "incompleta",
		"completa":        "completa",
	},

	"esito_fabbisogno": {
		"presente":      "presente",
		"da_verificare": "da verificare",
		"manca":         "manca",
	},

	"motivo_fabbisogno": {
		"contenuto_non_analizzato":    "contenuto non analizzato",
		"contenuto_non_aperto":        "contenuto non aperto",
		"contenuto_non_letto":         "contenuto non letto",
		"contenuto_non_verificabile":  "contenuto non verificabile",
		"formato_non_configurato":     "formato non configurato",
		"associazione_non_confermata": "associazione non confermata",
		"nessun_documento":            "nessun documento",
		"derogato_non_sostituisce_2d": "derogato: la deroga non sostituisce il 2D",
		"sul_portale":                 "sul portale",
	},

	"disponibilita": {
		"disponibile": "disponibile",
		"mancante":    "non ancora scaricato",
		"pendente":    "in preparazione",
		"parziale":    "letto in parte",
		"senza_testo": "senza testo (scansione)",
		"illeggibile": "illeggibile",
		"errore":      "errore",
	},

	"stato_struttura": {
		"nessuna":                "nessuna struttura",
		"struttura_candidata":    "struttura candidata",
		"bom_di_lavoro_proposta": "BOM di lavoro",
	},

	"tipo_componente": {
		"finito":       "prodotto finito",
		"sottoassieme": "sottoassieme",
		"sciolto":      "particolare",
		"commerciale":  "commerciale",
	},

	"origine_tipo": {
		"confermato":            "confermato",
		"proposto_dalla_riga":   "proposto dalla riga",
		"proposto_dalla_regola": "proposto dalla regola",
	},

	"categoria": {
		"fabbricato":      "fabbricato",
		"commerciale":     "commerciale",
		"minuteria":       "minuteria",
		"non_determinata": "non determinata",
	},

	"origine_categoria": {
		"confermato":        "confermata",
		"derivato_dal_tipo": "dal tipo",
		"proposto":          "proposta",
	},

	"origine_dato": {
		"grezzo":     "letto dal file",
		"proposto":   "proposto",
		"manuale":    "manuale",
		"confermato": "confermato",
		"scenario":   "scenario",
	},

	"stato_nas": {
		"in_coda": "copia sul NAS in coda",
		"scritto": "copiato sul NAS",
		"errore":  "copia sul NAS in errore",
	},

	"badge": {
		"uguale":                    "uguale",
		"nuovo_ancoraggio":          "nuovo ancoraggio",
		"diverso":                   "diverso",
		"regressione_su_confermata": "regressione su proposta confermata",
		"fuori_richiesta":           "fuori richiesta",
	},

	"motivo_non_congelabile": {
		"thread_non_valutato":                      "thread non valutato",
		"nessun_target":                            "nessun prodotto",
		"bom_versione_con_prodotti_non_verificati": "BOM con prodotti non verificati",
		"prodotti_non_pronti":                      "prodotti non pronti",
	},

	"motivo_thread": {
		"senza_grammatica_a":       "nessuna grammatica A per questo cliente",
		"grammatica_scartata":      "grammatica A non valida per questo cliente",
		"ragione_sociale_discorde": "ragione sociale discorde",
		"errore_valutazione":       "errore nel calcolo",
	},

	"tipo_conflitto": {
		"codice":             "codice",
		"associazione":       "associazione",
		"arco":               "legame",
		"nuovo_file":         "file nuovo",
		"identita_documento": "identità del documento",
	},

	"perimetro": {
		"dentro":               "nel perimetro",
		"inline":               "immagine nel testo",
		"elemento_outlook":     "elemento di Outlook",
		"contenitore_estratto": "archivio estratto",
		"collegamento":         "collegamento",
		"messaggio_in_uscita":  "messaggio in uscita",
		"altra_controparte":    "altra controparte",
	},

	"asse_conflitto": {
		"nomenclatura": "codici",
		"gerarchia":    "albero",
		"smistamento":  "smistamento",
	},

	"associazione": {
		"non_valutata":     "non valutata",
		"nessun_candidato": "nessun candidato",
		"candidato_unico":  "candidato unico",
		"ambiguo":          "ambigua",
		"discordante":      "discordante",
	},

	"autorita": {
		"confermata": "confermata",
		"proposta":   "proposta",
		"scenario":   "scenario",
	},

	"cartiglio": {
		"letto":                 "cartiglio letto",
		"non_leggibile":         "cartiglio non leggibile",
		"formato_senza_lettura": "formato senza lettura del cartiglio",
	},

	"collocazione": {
		"radice":            "radice",
		"figlio":            "figlio",
		"fuori_richiesta":   "fuori richiesta",
		"non_determinabile": "non determinabile",
	},

	"compatibilita": {
		"uguale":               "uguale",
		"equivalente":          "equivalente",
		"compatibile_parziale": "compatibile in parte",
		"discordante":          "discordante",
		"non_determinabile":    "non determinabile",
	},

	"discordanza": {
		"documento_componente": "revisione del documento contro quella del componente",
		"proposta_registrata":  "revisione proposta contro quella registrata",
		"proposta_confermata":  "revisione proposta contro quella confermata",
	},

	"esito_riconciliazione": {
		"concorda":               "concorda",
		"completamento_proposto": "completamento proposto",
		"correzione_proposta":    "correzione proposta",
		"discordante":            "discordante",
		"non_verificabile":       "non verificabile",
	},

	"esito_vista": {
		"presente_analizzato":     "STEP presente, letto",
		"presente_parziale":       "STEP presente, letto in parte",
		"presente_non_analizzato": "STEP presente, non analizzato",
		"riferimento_superato":    "riferimento superato",
		"da_scegliere":            "da scegliere",
		"da_confermare":           "da confermare",
		"sul_portale":             "sul portale",
		"solo_altro_3d":           "solo un altro 3D",
		"mancante":                "mancante",
		"radice_senza_qualifica":  "radice senza qualifica di prodotto",
		"ok":                      "presente",
		"ok_in_coda":              "presente, copia sul NAS in coda",
		"ok_errore_nas":           "presente, copia sul NAS in errore",
		"derogato":                "derogato",
		"manca":                   "manca",
	},

	"estrazione": {
		"riuscita":       "riuscita",
		"fallita":        "non riuscita",
		"in_corso":       "in corso",
		"non_analizzata": "non analizzata",
	},

	"fonte_revisione": {
		"cartiglio":   "cartiglio",
		"step_entita": "nodo dello STEP",
		"nome_file":   "nome del file",
		"documento":   "documento registrato",
		"assente":     "nessuna fonte",
		"decisione":   "decisione sul componente",
		"componente":  "revisione registrata del componente",
		"confermata":  "revisione confermata",
	},

	"motivo_abbinamento": {
		"abbinamento_per_base":                 "abbinato per base",
		"componenti_stessa_base":               "più componenti con la stessa base",
		"abbinamento_accanto_alla_riga_decisa": "abbinato accanto alla riga decisa",
		"nodo_scartato":                        "nodo scartato",
		"decisione_non_compatibile":            "decisione non compatibile",
		"decisione_non_verificabile":           "decisione non verificabile",
	},

	"motivo_asse": {
		"conflitto":                          "conflitto",
		"target_senza_componente":            "prodotto senza componente",
		"senza_figli_nessun_gesto":           "senza figli, nessun gesto",
		"nessun_gesto":                       "nessun gesto",
		"nessuna_struttura":                  "nessuna struttura",
		"da_decidere":                        "decisioni aperte",
		"nomenclatura_radice_non_verificata": "codice della radice non verificato",
		"fonte_non_confermata":               "fonte non confermata",
		"bom_di_lavoro_assente":              "BOM di lavoro assente",
	},

	"motivo_classificazione": {
		"finito_chiede_sempre_il_2d": "un prodotto finito chiede sempre il 2D",
		"conferma_non_ammessa":       "conferma non ammessa",
		"tipo_non_determinato":       "tipo non determinato",
	},



	"motivo_conflitto": Object.assign({},ETI_TIPO_DOC,{
		"quantita":           "quantità diversa",
		"rimozione":          "legame tolto dallo STEP",
		"componente_diverso": "componente diverso",
		"codice_confermato":  "diverso dal codice confermato",
		"codice":             "codice diverso",
		"revisione":          "revisione diversa",
	}),

	"motivo_documentale": {
		"cartiglio_non_letto":         "cartiglio non letto",
		"nodo_senza_identita":         "nodo senza identità",
		"cartiglio_altro_namespace":   "cartiglio di un altro spazio di codici",
		"cartiglio_non_completo":      "codice del cartiglio non completo",
		"base_non_confrontabile":      "base non confrontabile",
		"revisione_cartiglio_assente": "revisione assente nel cartiglio",
		"revisione_cartiglio_ambigua": "revisione del cartiglio ambigua",
		"letture_discordanti":         "letture discordanti",
		"identita_discordanti":        "identità discordanti",
	},

	"motivo_documenti": {
		"non_determinabile":       "non determinabile",
		"target_senza_componente": "prodotto senza componente",
		"voce_mancante":           "manca un documento",
		"voce_non_presente":       "un documento non è presente",
		"perimetro_aperto":        "perimetro aperto",
	},

	"motivo_file": {
		"contenitore":             "contenitore",
		"natura_non_file":         "non è un file",
		"documento_non_leggibile": "documento non leggibile",
		"thread_non_valutato":     "thread non valutato",
	},

	"motivo_grafo": {
		"completezza del grafo non dichiarata dal documento":          "completezza del grafo non dichiarata dal documento",
		"completezza non determinabile: nessun motivo dal caricatore": "completezza non determinabile: nessun motivo dal caricatore",
	},

	"motivo_lettura": {
		"nessuna_lettura":     "nessuna lettura",
		"letture_discordanti": "letture discordanti",
		"codice_vuoto":        "codice vuoto",
		"senza_grammatica":    "senza grammatica",
		"basi_diverse":        "basi diverse",
	},

	"motivo_non_congelato": {
		"gesto_non_registrato": "il congelamento non è ancora un gesto",
	},

	"motivo_perimetro": {
		"target_senza_componente": "prodotto senza componente",
		"fonte_non_confermata":    "fonte non confermata",
		"bom_di_lavoro_assente":   "BOM di lavoro assente",
		"nodi_da_decidere":        "nodi da decidere",
		"archi_da_decidere":       "legami da decidere",
		"rimozioni_aperte":        "rimozioni aperte",
		"non_determinabile":       "non determinabile",
	},

	"motivo_previsto": {
		"nodo_proposto":               "nodo proposto",
		"nodo_deciso_fuori_perimetro": "nodo deciso fuori dal perimetro",
		"rimozione_aperta":            "rimozione aperta",
	},

	"motivo_proposto": {
		"non_composto":                   "codice non composto",
		"nessuna_lettura":                "nessuna lettura",
		"letture_discordanti":            "letture discordanti",
		"forma_cartiglio_assente":        "forma del cartiglio assente",
		"forme_cartiglio_multiple":       "più forme del cartiglio",
		"parte_non_determinata":          "parte non determinata",
		"qualificatore_non_trasferibile": "qualificatore non trasferibile",
		"revisione_non_determinata":      "revisione non determinata",
		"revisione_ambigua":              "revisione ambigua",
		"lettura_non_completa":           "lettura non completa",
	},

	"nota_regola": {
		"schema_senza_2d":                 "lo schema non chiede il 2D",
		"regola_cliente_senza_2d":         "il cliente non chiede il 2D",
		"regola_cliente_2d_non_bloccante": "per il cliente il 2D non è bloccante",
	},

	"origine_candidato": {
		"documento_del_prodotto": "documento del prodotto",
		"motore_a":               "proposto dal motore A",
		"proposta_aperta":        "proposta aperta",
	},

	"regola_primario": {
		"corrente":      "corrente",
		"compatibilita": "compatibilità",
		"formato":       "formato",
		"recente":       "più recente",
		"sha256":        "ordine del contenuto (sha256)",
	},

	"relazione_disegni": {
		"rappresentazione_alternativa": "rappresentazione alternativa",
		"revisione_diversa":            "revisione diversa",
		"elaborato_aggiuntivo":         "elaborato aggiuntivo",
		"non_determinata":              "non determinata",
	},

	"rev_provenienza": {
		"non_registrata":               "non registrata",
		"coincide_con_formazione_step": "coincide con la formazione dello STEP",
		"decisione_tracciata":          "decisione tracciata",
	},

	"ruolo": {
		"prodotto":   "prodotto",
		"componente": "componente",
	},

	"stato_identita_target": {
		"da_confermare": "da confermare",
		"confermata":    "confermata",
	},

	"stato_revisione": {
		"assente":    "nessuna revisione",
		"proposta":   "revisione proposta",
		"registrata": "revisione registrata",
		"confermata": "revisione confermata",
		"candidata":  "revisione candidata",
	},

	"stato_riga": {
		"aperta":     "aperta",
		"confermata": "confermata",
		"scartata":   "scartata",
		"duplicato":  "duplicato",
	},

	"stato_versione": {
		"bozza":     "bozza",
		"congelata": "congelata",
	},

	"tipo_documento": ETI_TIPO_DOC,

	"validita_2d": {
		"valido":                  "valido",
		"da_verificare":           "da verificare",
		"formato_non_configurato": "formato non configurato",
	},

	"stato_thread": {
		"APERTA": "aperta",
		"CHIUSA": "chiusa",
	},

	"semaforo": {
		"bianco": "senza tempi",
		"rosso":  "oltre i tempi",
		"giallo": "in scadenza",
		"verde":  "nei tempi",
	},


	"fase": {
		"RICEVUTA":        "ricevuta",
		"ATTESA_DISEGNI":  "in attesa dei disegni",
		"FATTIBILITA":     "fattibilità",
		"SCHEDA_COSTO":    "scheda costo",
		"OFFERTE_FORN":    "offerte dei fornitori",
		"OFFERTA_INVIATA": "offerta inviata",
		"ACCETTATA":       "accettata",
		"DISTINTA_ERP":    "distinta nel gestionale",
		"ORDINE":          "ordine",
		"PRODUZIONE":      "in produzione",
		"PERSA":           "persa",
		"RESPINTA":        "respinta",
		"SCADUTA":         "scaduta",
	},
};

/* Testi proposti a Pietro (RV-06): l'etichetta corta di una parola sulla casella dell'albero (M6). Finché Pietro non
   li mette nella tabella vera, il mockup li tiene qui, separati. */
const ETICHETTE_PROPOSTE = {
	"tipo_componente_breve": { "finito": "prodotto", "sottoassieme": "assieme", "sciolto": "partic.", "commerciale": "comm." },
};

function etichetta(dominio, valore){
  if(valore===null||valore===undefined||valore==="") return "[]";
  if(typeof valore!=="string") return "["+String(valore)+"]";
  const tab = ETICHETTE[dominio] || ETICHETTE_PROPOSTE[dominio];
  if(tab && Object.prototype.hasOwnProperty.call(tab, valore)) return tab[valore];
  return "["+valore+"]";
}

/* Il vocabolario degli stati (dal template vero: ma_tono, ma_tono_d, ma_st). Un valore di enum diventa SOLO una classe
   di tono; le parole le dà etichetta(). È presentazione: nessuno stato si calcola qui. */
const TONO = {
  ok:["confermata","confermato","verificata","verificato","completa","presente","scritto","disponibile","valido","pronto_fattibilita","uguale","equivalente","concorda","candidato_unico"],
  prop:["in_attesa_di_conferma","proposta","proposto","struttura_candidata","da_confermare","manuale","nuovo_ancoraggio","candidata","completamento_proposto","correzione_proposta","proposto_dalla_riga","proposto_dalla_regola","compatibile_parziale"],
  ver:["da_verificare","in_revisione","da_riesaminare","in_coda","pendente","parziale","non_pronto","bom_di_lavoro_proposta","mancante","diverso","ambiguo","manca","incompleta"],
  ko:["assente","conflitto","errore","illeggibile","regressione_su_confermata","discordante"],
};
function tono(v){ for(const [k,l] of Object.entries(TONO)) if(l.includes(v)) return k; return "neu"; }
function tonoD(dom, v){ if(dom==="stato_revisione" && v==="assente") return "neu"; return tono(v); }
const escE = s=>String(s??"").replace(/[<>&"]/g,c=>({"<":"&lt;",">":"&gt;","&":"&amp;",'"':"&quot;"}[c]));
/* ma_st: un valore vuoto è «—» con il title, mai «tutto bene» */
function st(dom, val, extra){
  if(val===null||val===undefined||val==="") return `<span class="ma-st ma-st--na" title="${dom}: campo vuoto">—</span>`;
  return `<span class="ma-st ma-st--${tonoD(dom,val)}${extra?" "+extra:""}" title="${escE(dom)} = ${escE(val)}">${escE(etichetta(dom,val))}</span>`;
}
