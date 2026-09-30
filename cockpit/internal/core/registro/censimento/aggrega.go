package censimento

import (
	"sort"
	"strings"

	"github.com/google/uuid"

	"promatec/cockpit/internal/core/inbox/classificazione"
	"promatec/cockpit/internal/platform/db"
)

// Aggrega fa i conti dell'ingresso, cliente per cliente. Pura: le stesse righe, in qualunque ordine, danno lo
// stesso censimento (le schede seguono l'ordine dei clienti, quello dell'anagrafica). Per questo niente si
// sceglie per ordine d'arrivo: una riga che si conta una volta sola (una stringa per RFQ, un file, una lettura
// del cartiglio, un confronto con una decisione) si riconosce da tutto quello che la distingue, e ogni elenco
// si ordina su tutti i suoi campi. Una riga di un cliente che non c'e' nell'ingresso si ignora.
func Aggrega(in Ingresso) Censimento {
	out := Censimento{DominiNonCensiti: in.DominiNonCensiti, SolaLettura: in.SolaLettura}
	racc := map[uuid.UUID]*raccolta{}
	ordine := make([]*raccolta, 0, len(in.Clienti))
	for _, c := range in.Clienti {
		r := nuovaRaccolta(c)
		racc[c.ID] = r
		ordine = append(ordine, r)
	}
	// prima i codici decisi: le teste e le code di tutte le altre fonti si cercano intorno a loro
	for _, p := range in.Pezzi {
		if r := racc[p.Cliente]; r != nil {
			r.deciso(p.Thread, p.Codice)
		}
	}
	for _, d := range in.Decisi {
		if r := racc[d.Cliente]; r != nil {
			r.deciso(d.Thread, d.Codice)
		}
	}
	for _, r := range ordine {
		r.ordinaDecisi()
	}

	for _, m := range in.Messaggi {
		if r := racc[m.Cliente]; r != nil {
			r.messaggio(m)
		}
	}
	for _, f := range in.File {
		if r := racc[f.Cliente]; r != nil {
			r.file(f)
		}
	}
	for _, n := range in.Nodi {
		if r := racc[n.Cliente]; r != nil {
			r.nodo(n)
		}
	}
	for _, c := range in.Campi {
		if r := racc[c.Cliente]; r != nil {
			r.campo(c)
		}
	}
	for _, p := range in.Pezzi {
		if r := racc[p.Cliente]; r != nil {
			r.pezzo(p)
		}
	}
	for _, d := range in.Decisi {
		if r := racc[d.Cliente]; r != nil {
			r.forma(FonteDistinta, d.Thread, d.Codice)
		}
	}
	for _, r := range ordine {
		out.Schede = append(out.Schede, r.scheda())
	}
	return out
}

// ---------------------------------------------------------------- la raccolta di un cliente

// raccolta sono i contatori di un cliente mentre si leggono le righe.
type raccolta struct {
	s      Scheda
	motore *classificazione.Motore
	// decisi sono i codici decisi per RFQ, in maiuscolo, il piu' lungo prima
	decisi map[uuid.UUID][]string
	// visti evita di contare due volte la stessa stringa della stessa fonte nella stessa RFQ
	visti map[string]bool

	forme                map[string]*contaForma
	code, teste          map[string]*contaAggiunta
	oggetti, riferimenti map[string]*contaVoce
	mittenti             map[string]*contaVoce
	formeRif             map[string]*contaVoce
	letture              map[string]*contaVoce
	diversi, proposte    []Differenza
	nuovi, persi         esempi
	formeFirma, parole   map[string]*contaFirma
	scarti, nonViste     []RigaMinuteria
}

func nuovaRaccolta(c Cliente) *raccolta {
	return &raccolta{
		s:      Scheda{Cliente: Cliente{ID: c.ID, Nome: c.Nome, Ragione: c.Ragione, Attivo: c.Attivo}},
		motore: motore(c),
		decisi: map[uuid.UUID][]string{}, visti: map[string]bool{},
		forme: map[string]*contaForma{}, code: map[string]*contaAggiunta{}, teste: map[string]*contaAggiunta{},
		oggetti: map[string]*contaVoce{}, riferimenti: map[string]*contaVoce{}, mittenti: map[string]*contaVoce{},
		formeRif: map[string]*contaVoce{}, letture: map[string]*contaVoce{},
		formeFirma: map[string]*contaFirma{}, parole: map[string]*contaFirma{},
	}
}

// nuovo dice se la stringa di una fonte non e' ancora stata contata nella RFQ, e la segna. Le maiuscole non
// contano: «7120001_prt» e «7120001_PRT» sono la stessa stringa.
func (r *raccolta) nuovo(fonte string, thread uuid.UUID, s string) bool {
	return r.nuovoEsatto(fonte, thread, strings.ToUpper(strings.TrimSpace(s)))
}

// nuovoEsatto e' nuovo con la stringa com'e' scritta, maiuscole comprese: per i file, il cui nome finisce in
// una riga (un esempio, una differenza). Due nomi che differiscono solo per le maiuscole sono due allegati, e
// contarli come uno vorrebbe dire scegliere quale dei due scrivere secondo l'ordine delle righe.
func (r *raccolta) nuovoEsatto(chiave string, thread uuid.UUID, s string) bool {
	k := chiave + "\x00" + thread.String() + "\x00" + s
	if r.visti[k] {
		return false
	}
	r.visti[k] = true
	return true
}

// forma conta la forma di una stringa di una fonte, una volta per RFQ. Gli esempi prendono ogni variante
// della stringa (anche quella che non si conta perche' differisce solo per le maiuscole): sono i primi in
// ordine alfabetico, e non dipendono da quale variante arriva prima.
func (r *raccolta) forma(fonte string, thread uuid.UUID, s string) {
	s = strings.TrimSpace(s)
	f := classificazione.Forma(s)
	if f == "" {
		return
	}
	c := r.forme[f]
	if c == nil {
		c = &contaForma{per: map[string]int{}}
		r.forme[f] = c
	}
	c.esempi.metti(s)
	if !r.nuovo(fonte, thread, s) {
		return
	}
	c.per[fonte]++
	c.n++
}

func (r *raccolta) deciso(thread uuid.UUID, codice string) {
	c := strings.ToUpper(strings.TrimSpace(codice))
	if thread == uuid.Nil || c == "" {
		return
	}
	for _, x := range r.decisi[thread] {
		if x == c {
			return
		}
	}
	r.decisi[thread] = append(r.decisi[thread], c)
}

func (r *raccolta) ordinaDecisi() {
	for _, v := range r.decisi {
		sort.Slice(v, func(i, j int) bool {
			if len(v[i]) != len(v[j]) {
				return len(v[i]) > len(v[j])
			}
			return v[i] < v[j]
		})
	}
}

// intorno cerca nella stringa il codice deciso piu' lungo della stessa RFQ e conta quello che c'e' prima (la
// testa) e dopo (la coda). Una stringa identica a un codice deciso e' «uguale».
func (r *raccolta) intorno(fonte string, thread uuid.UUID, s string) {
	if thread == uuid.Nil {
		return
	}
	su := strings.ToUpper(strings.TrimSpace(s))
	for _, d := range r.decisi[thread] {
		if len(d) < minDeciso {
			continue
		}
		i := strings.Index(su, d)
		if i < 0 {
			continue
		}
		testa, coda := su[:i], su[i+len(d):]
		if testa == "" && coda == "" {
			r.s.Uguali++
			return
		}
		if coda != "" {
			conta(r.code, coda, d, thread, fonte, su)
		}
		if testa != "" {
			conta(r.teste, testa, d, thread, fonte, su)
		}
		return
	}
}

func conta(m map[string]*contaAggiunta, testo, deciso string, thread uuid.UUID, fonte, intera string) {
	c := m[testo]
	if c == nil {
		c = &contaAggiunta{pezzi: map[string]bool{}, rfq: map[uuid.UUID]bool{}, fonti: map[string]bool{}}
		m[testo] = c
	}
	c.n++
	c.pezzi[deciso] = true
	c.rfq[thread] = true
	c.fonti[fonte] = true
	c.esempi.metti(intera)
}

// ---------------------------------------------------------------- le fonti

func (r *raccolta) messaggio(m Messaggio) {
	r.s.NMessaggi++
	r.s.Mail.Messaggi++
	// il motore di oggi, sugli stessi testi della triage (IngressoTriage.Testi: oggetto, corpo tagliato, storia
	// citata, nomi degli allegati)
	in := classificazione.IngressoTriage{Oggetto: m.Oggetto, Corpo: m.Corpo, NomiAllegati: m.Allegati}
	e := r.motore.Estrai(in.Testi()...)
	prop := e.Proponibili()
	r.s.Mail.Proponibili += len(prop)
	r.s.Mail.Altri += len(e.Altri())
	if len(prop) > 0 {
		r.s.Mail.ConCodice++
	}
	if e.Riferimento != "" {
		r.s.Mail.ConRiferimento++
		voce(r.formeRif, classificazione.Forma(e.Riferimento), strings.ToUpper(e.Riferimento))
	}
	for _, c := range e.Codici {
		// i codici della storia citata sono di un altro messaggio, quelli degli allegati sono nomi di file
		// (fonte «nome»): qui solo cio' che il cliente ha scritto adesso
		if c.Dove == classificazione.DoveStoria || strings.HasPrefix(c.Dove, "allegato ") {
			continue
		}
		r.forma(FonteMail, m.Thread, c.Codice)
	}
	switch {
	case m.Interpretato && m.Estratto:
		r.confrontaSalvati(m, e.Codici)
	case m.Interpretato:
		r.s.Mail.SenzaEstrazione++
	}
	if f := FormaOggetto(m.Oggetto); f != "" {
		voce(r.oggetti, f, strings.TrimSpace(m.Oggetto))
	}
	utile, _ := classificazione.TagliaCatena(m.Corpo)
	for chiave, numeri := range Riferimenti(classificazione.OggettoPulito(m.Oggetto) + "\n" + utile) {
		v := voceDi(r.riferimenti, chiave)
		v.n++
		for _, n := range numeri {
			v.esempi.metti(n)
		}
	}
	if m.ViaDominio && m.Mittente != "" {
		voce(r.mittenti, m.Mittente, "")
	}
}

// confrontaSalvati mette accanto i codici che l'ingest ha salvato all'arrivo e quelli che il motore di oggi trova
// negli stessi testi, come insiemi (il codice e' la chiave di candidato_codice, e l'ingest lo salva tagliato a
// maxCodiceSalvato caratteri: qui si taglia allo stesso modo). Con il corpo tagliato si guardano solo l'oggetto e
// i nomi degli allegati, come fa la query dei salvati.
func (r *raccolta) confrontaSalvati(m Messaggio, trovati []classificazione.CodiceTrovato) {
	chiave := func(c string) string {
		k := []rune(strings.ToUpper(strings.TrimSpace(c)))
		if len(k) > maxCodiceSalvato {
			k = k[:maxCodiceSalvato]
		}
		return string(k)
	}
	oggi, allora := map[string]bool{}, map[string]bool{}
	for _, c := range trovati {
		if m.Troncato && c.Dove != "oggetto" && !strings.HasPrefix(c.Dove, "allegato ") {
			continue
		}
		if k := chiave(c.Codice); k != "" {
			oggi[k] = true
		}
	}
	for _, c := range m.Salvati {
		if k := chiave(c); k != "" {
			allora[k] = true
		}
	}
	r.s.Mail.Interpretati++
	r.s.Mail.Salvati += len(allora)
	for k := range oggi {
		if !allora[k] {
			r.s.Mail.Nuovi++
			r.nuovi.metti(k)
		}
	}
	for k := range allora {
		if !oggi[k] {
			r.s.Mail.Persi++
			r.persi.metti(k)
		}
	}
}

// file conta un file tecnico. Tre chiavi, perche' sono tre domande: la forma e la lettura del nome dipendono
// solo dal nome (una volta per RFQ e nome); il file e' il nome con il suo contenuto (lo stesso nome rimandato
// con una revisione nuova e' un altro file); il confronto con la decisione e con la proposta e' di quel
// contenuto con quella decisione o proposta. Cosi' il documento deciso non esce dal confronto perche' un file
// con lo stesso nome e un altro contenuto e' arrivato prima. Il nome conta com'e' scritto (nuovoEsatto).
func (r *raccolta) file(f File) {
	if !Tecnico(f.Nome) {
		return
	}
	nome := strings.TrimSpace(f.Nome)
	base := senzaEstensione(nome)
	if base == "" {
		return
	}
	if r.nuovoEsatto("file", f.Thread, nome+"\x00"+f.Contenuto) {
		r.s.NFile++
	}

	// la lettura del nome, come la fa l'ingest (solo dal nome, con il motore del cliente)
	v := classificazione.Valuta(classificazione.IngressoFile{Da: classificazione.DaIngest, NomeFile: nome,
		Direzione: "entrata", Motore: r.motore})
	if r.nuovoEsatto("lettura", f.Thread, nome) {
		r.forma(FonteNome, f.Thread, base)
		r.intorno(FonteNome, f.Thread, base)
		r.s.Nome.File++
		voce(r.letture, formaLettura(v.Codice.Valore, v.Rev.Valore), nome)
	}
	if f.Deciso && strings.TrimSpace(f.Codice) != "" &&
		r.nuovoEsatto("decisione", f.Thread, nome+"\x00"+f.Contenuto+"\x00"+strings.TrimSpace(f.Codice)+"\x00"+strings.TrimSpace(f.Rev)) {
		r.s.Nome.Confrontati++
		cod := strings.EqualFold(strings.TrimSpace(v.Codice.Valore), strings.TrimSpace(f.Codice))
		rev := strings.EqualFold(strings.TrimSpace(v.Rev.Valore), strings.TrimSpace(f.Rev))
		if cod {
			r.s.Nome.CodiceUguale++
		}
		if rev {
			r.s.Nome.RevUguale++
		}
		if !cod || !rev {
			r.diversi = append(r.diversi, Differenza{Nome: nome,
				Letto: lettura(v.Codice.Valore, v.Rev.Valore), Deciso: lettura(f.Codice, f.Rev)})
		}
	}
	if f.Proposto &&
		r.nuovoEsatto("proposta", f.Thread, nome+"\x00"+f.Contenuto+"\x00"+strings.TrimSpace(f.CodiceProposto)+"\x00"+strings.TrimSpace(f.RevProposta)) {
		r.s.Nome.ConProposta++
		if strings.EqualFold(strings.TrimSpace(v.Codice.Valore), strings.TrimSpace(f.CodiceProposto)) &&
			strings.EqualFold(strings.TrimSpace(v.Rev.Valore), strings.TrimSpace(f.RevProposta)) {
			r.s.Nome.PropostaUguale++
		} else {
			r.proposte = append(r.proposte, Differenza{Nome: nome,
				Letto: lettura(v.Codice.Valore, v.Rev.Valore), Deciso: lettura(f.CodiceProposto, f.RevProposta)})
		}
	}
}

func (r *raccolta) nodo(n Nodo) {
	s := strings.TrimSpace(n.ID)
	if s == "" {
		s = strings.TrimSpace(n.Nome)
	}
	if s == "" {
		return
	}
	if r.nuovo("nodo", n.Thread, s) {
		r.s.NNodi++
		r.intorno(FonteStep, n.Thread, s)
	}
	r.forma(FonteStep, n.Thread, s)
}

// campo conta una lettura del cartiglio una volta per RFQ, contenuto, etichetta, valore e fonte: la lettura
// dell'OCR e quella del testo nativo dello stesso valore sono due letture, e nessuna delle due prende il posto
// dell'altra per ordine d'arrivo.
func (r *raccolta) campo(c Campo) {
	v := strings.TrimSpace(c.Valore)
	fonte := "nativo"
	if c.OCR {
		fonte = "ocr"
	}
	if v == "" || !r.nuovo("campo", c.Thread, c.Contenuto+"\x00"+c.Etichetta+"\x00"+v+"\x00"+fonte) {
		return
	}
	if c.OCR {
		r.s.NCampiOCR++
		return
	}
	r.s.NCampi++
	r.intorno(FonteCartiglio, c.Thread, v)
	r.forma(FonteCartiglio, c.Thread, v)
}

func (r *raccolta) pezzo(p Pezzo) {
	r.s.NPezzi++
	r.forma(FonteDistinta, p.Thread, p.Codice)

	// le firme: la forma con le lettere e le parole dei nomi, per i commerciali decisi e per gli altri
	comm := p.Deciso == db.TipoComponenteCommerciale
	if comm {
		r.s.Firme.Commerciali++
	} else {
		r.s.Firme.Altri++
	}
	firma(r.formeFirma, classificazione.FormaCifre(p.Codice), comm, strings.ToUpper(strings.TrimSpace(p.Codice)))
	visti := map[string]bool{}
	for _, nome := range p.Nomi {
		for _, w := range Parole(nome) {
			if !visti[w] {
				visti[w] = true
				firma(r.parole, w, comm, "")
			}
		}
	}

	// la minuteria: proposto e deciso
	proposto := p.Proposto == db.TipoComponenteCommerciale
	if p.Proposto != "" {
		r.s.Minuteria.Proposte = true
	}
	riga := RigaMinuteria{Codice: strings.TrimSpace(p.Codice), Nome: primoNome(p.Nomi), Deciso: p.Deciso, Proposto: p.Proposto, Motivo: p.Motivo}
	switch {
	case proposto && comm:
		r.s.Minuteria.Confermate++
	case proposto:
		r.s.Minuteria.Scartate++
		r.scarti = append(r.scarti, riga)
	case comm:
		r.s.Minuteria.NonProposte++
		r.nonViste = append(r.nonViste, riga)
	default:
		r.s.Minuteria.Altri++
	}
}

// ---------------------------------------------------------------- la scheda

func (r *raccolta) scheda() Scheda {
	s := r.s

	forme := make([]FormaVista, 0, len(r.forme))
	for f, c := range r.forme {
		forme = append(forme, FormaVista{Forma: f, Per: c.per, Totale: c.n, Esempi: c.esempi.v})
	}
	sort.Slice(forme, func(i, j int) bool {
		if forme[i].Totale != forme[j].Totale {
			return forme[i].Totale > forme[j].Totale
		}
		return forme[i].Forma < forme[j].Forma
	})
	s.Forme, s.AltreForme = tagliaForme(forme, MaxForme)

	s.Code, s.AltreCode = aggiunte(r.code, MaxAggiunte)
	s.Teste, s.AltreTeste = aggiunte(r.teste, MaxAggiunte)
	s.Oggetti, s.AltriOggetti = voci(r.oggetti, MaxOggetti)
	s.Riferimenti, s.AltriRiferimenti = voci(r.riferimenti, MaxRiferimenti)
	s.Mittenti, s.AltriMittenti = voci(r.mittenti, MaxMittenti)
	s.Mail.FormeRiferimento, _ = voci(r.formeRif, MaxRiferimenti)
	s.Nome.Letture, s.Nome.AltreLetture = voci(r.letture, MaxLetture)

	ordinaDifferenze(r.diversi)
	ordinaDifferenze(r.proposte)
	s.Nome.Diversi, s.Nome.AltriDiversi = taglia(r.diversi, MaxDifferenze)
	s.Nome.ProposteDiverse, s.Nome.AltreProposteDiverse = taglia(r.proposte, MaxDifferenze)
	s.Mail.EsempiNuovi, s.Mail.EsempiPersi = r.nuovi.v, r.persi.v

	s.Firme.Forme, s.Firme.AltreForme = firme(r.formeFirma, MaxFirme, false)
	s.Firme.Parole, s.Firme.AltreParole = firme(r.parole, MaxParole, true)

	ordinaRighe(r.scarti)
	ordinaRighe(r.nonViste)
	s.Minuteria.Scarti, s.Minuteria.AltriScarti = taglia(r.scarti, MaxMinuteria)
	s.Minuteria.NonViste, s.Minuteria.AltriNonVisti = taglia(r.nonViste, MaxMinuteria)
	return s
}

func tagliaForme(v []FormaVista, max int) ([]FormaVista, Resto) {
	if len(v) <= max {
		return v, Resto{}
	}
	var resto Resto
	for _, x := range v[max:] {
		resto.Voci++
		resto.N += x.Totale
	}
	return v[:max], resto
}

func aggiunte(m map[string]*contaAggiunta, max int) ([]Aggiunta, Resto) {
	out := make([]Aggiunta, 0, len(m))
	for t, c := range m {
		a := Aggiunta{Testo: t, Forma: classificazione.Forma(t), N: c.n, Pezzi: len(c.pezzi), RFQ: len(c.rfq), Esempi: c.esempi.v}
		for _, f := range Fonti {
			if c.fonti[f] {
				a.Fonti = append(a.Fonti, f)
			}
		}
		a.Alias = a.Pezzi >= 2 && a.RFQ >= 2
		out = append(out, a)
	}
	// prima gli alias candidati, poi le piu' viste
	sort.Slice(out, func(i, j int) bool {
		if out[i].Alias != out[j].Alias {
			return out[i].Alias
		}
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Testo < out[j].Testo
	})
	if len(out) <= max {
		return out, Resto{}
	}
	var resto Resto
	for _, a := range out[max:] {
		resto.Voci++
		resto.N += a.N
	}
	return out[:max], resto
}

func firme(m map[string]*contaFirma, max int, soloCommerciali bool) ([]Firma, Resto) {
	out := make([]Firma, 0, len(m))
	for t, c := range m {
		if soloCommerciali && c.comm == 0 {
			continue // una parola che nessun commerciale porta non e' una firma
		}
		out = append(out, Firma{Testo: t, Commerciali: c.comm, Altri: c.altri, Esempi: c.esempi.v})
	}
	// prima quelle dei commerciali, e fra quelle le meno portate dagli altri pezzi
	sort.Slice(out, func(i, j int) bool {
		if out[i].Commerciali != out[j].Commerciali {
			return out[i].Commerciali > out[j].Commerciali
		}
		if out[i].Altri != out[j].Altri {
			return out[i].Altri < out[j].Altri
		}
		return out[i].Testo < out[j].Testo
	})
	if len(out) <= max {
		return out, Resto{}
	}
	var resto Resto
	for _, f := range out[max:] {
		resto.Voci++
		resto.N += f.Commerciali + f.Altri
	}
	return out[:max], resto
}

func ordinaDifferenze(v []Differenza) {
	sort.Slice(v, func(i, j int) bool {
		a, b := v[i], v[j]
		if a.Nome != b.Nome {
			return a.Nome < b.Nome
		}
		if a.Letto != b.Letto {
			return a.Letto < b.Letto
		}
		return a.Deciso < b.Deciso
	})
}

// ordinaRighe ordina su tutti i campi: due pezzi con lo stesso codice e lo stesso nome in due RFQ possono avere
// tipi o motivi diversi, e il loro ordine non deve dipendere da quello delle righe.
func ordinaRighe(v []RigaMinuteria) {
	sort.Slice(v, func(i, j int) bool {
		a, b := v[i], v[j]
		switch {
		case a.Codice != b.Codice:
			return a.Codice < b.Codice
		case a.Nome != b.Nome:
			return a.Nome < b.Nome
		case a.Deciso != b.Deciso:
			return a.Deciso < b.Deciso
		case a.Proposto != b.Proposto:
			return a.Proposto < b.Proposto
		}
		return a.Motivo < b.Motivo
	})
}

func taglia[T any](v []T, max int) ([]T, int) {
	if len(v) <= max {
		return v, 0
	}
	return v[:max], len(v) - max
}

// ---------------------------------------------------------------- i contatori

type contaForma struct {
	per    map[string]int
	n      int
	esempi esempi
}

type contaAggiunta struct {
	n      int
	pezzi  map[string]bool
	rfq    map[uuid.UUID]bool
	fonti  map[string]bool
	esempi esempi
}

type contaVoce struct {
	n      int
	esempi esempi
}

type contaFirma struct {
	comm, altri int
	esempi      esempi
}

func voceDi(m map[string]*contaVoce, testo string) *contaVoce {
	c := m[testo]
	if c == nil {
		c = &contaVoce{}
		m[testo] = c
	}
	return c
}

// voce conta una volta il testo, con un esempio se c'e'.
func voce(m map[string]*contaVoce, testo, esempio string) {
	c := voceDi(m, testo)
	c.n++
	if esempio != "" {
		c.esempi.metti(esempio)
	}
}

func voci(m map[string]*contaVoce, max int) ([]Voce, Resto) {
	out := make([]Voce, 0, len(m))
	for t, c := range m {
		out = append(out, Voce{Testo: t, N: c.n, Esempi: c.esempi.v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].N != out[j].N {
			return out[i].N > out[j].N
		}
		return out[i].Testo < out[j].Testo
	})
	if len(out) <= max {
		return out, Resto{}
	}
	var resto Resto
	for _, v := range out[max:] {
		resto.Voci++
		resto.N += v.N
	}
	return out[:max], resto
}

func firma(m map[string]*contaFirma, testo string, comm bool, esempio string) {
	if testo == "" {
		return
	}
	c := m[testo]
	if c == nil {
		c = &contaFirma{}
		m[testo] = c
	}
	if comm {
		c.comm++
		if esempio != "" {
			c.esempi.metti(esempio)
		}
	} else {
		c.altri++
	}
}

// esempi tiene i primi testi diversi in ordine alfabetico, al massimo numEsempi: lo stesso risultato qualunque
// sia l'ordine in cui arrivano le righe, e una memoria che non cresce con il database.
type esempi struct{ v []string }

func (e *esempi) metti(s string) {
	i := sort.SearchStrings(e.v, s)
	if i < len(e.v) && e.v[i] == s {
		return
	}
	if i >= numEsempi {
		return
	}
	e.v = append(e.v, "")
	copy(e.v[i+1:], e.v[i:])
	e.v[i] = s
	if len(e.v) > numEsempi {
		e.v = e.v[:numEsempi]
	}
}

// ---------------------------------------------------------------- le parole delle letture

// formaLettura e' la chiave di una lettura del nome: la forma del codice e quella della revisione.
func formaLettura(codice, rev string) string {
	c := classificazione.Forma(codice)
	if c == "" {
		c = "(nessun codice)"
	}
	if r := classificazione.Forma(rev); r != "" {
		return c + " · rev " + r
	}
	return c + " · senza rev"
}

// lettura e' un codice con la sua revisione, come si legge in una riga.
func lettura(codice, rev string) string {
	c := strings.TrimSpace(codice)
	if c == "" {
		c = "(nessun codice)"
	}
	if r := strings.TrimSpace(rev); r != "" {
		return c + " rev " + r
	}
	return c
}

func primoNome(nomi []string) string {
	for _, n := range nomi {
		if n = strings.TrimSpace(n); n != "" {
			return n
		}
	}
	return ""
}
