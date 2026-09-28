package staging

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"strings"
)

// CartellaStaging era la sottocartella di staging di un messaggio: hash breve del Message-ID.
//
// Dal blocco 4A NON decide più dove va un file: un contenuto sta sotto _contenuti e si chiama come il
// proprio sha256 (vedi upload.go). Resta nel payload perché il worker la scrive nel log, ed è l'unico
// modo che ha chi legge quel log di risalire dal job al messaggio senza aprire il database.
func CartellaStaging(messageID string) string {
	h := sha1.Sum([]byte(messageID))
	return hex.EncodeToString(h[:])[:12]
}

// Staging risponde a una sola domanda: quel file c'è ancora sul disco?
//
// È un'interfaccia e non una chiamata a os.Stat dentro AccodaStage perché altrimenti la guardia della
// voce 1.11 sarebbe verificabile solo costruendo alberi di file veri, e un test che dipende da che
// cosa c'è davvero nello staging della macchina non prova quasi niente.
type Staging interface {
	Presente(percorso string) bool
}

// FileStaging è lo staging vero: il disco.
type FileStaging struct{}

func (FileStaging) Presente(percorso string) bool {
	if strings.TrimSpace(percorso) == "" {
		return false
	}
	st, err := os.Stat(percorso)
	return err == nil && !st.IsDir()
}
