package archivio

// Il 7z si apre nel processo del server, quindi deve stare entro la memoria come lo zip (revisione del 14/10):
//
//   - il dizionario LZMA/LZMA2 lo dichiara l'header, e il decodificatore lo alloca per intero: un archivio di
//     poche centinaia di byte può chiedere 4 GB. I decodificatori LZMA e LZMA2 di sevenzip si sostituiscono qui
//     con due copie fedeli (stessa libreria, ulikunitz/xz) che prima rifiutano un dizionario oltre
//     maxDizionario7z. Vale anche per l'header codificato, che sevenzip decodifica già in OpenReader;
//   - Zstandard, Brotli e LZ4 non sono metodi del 7-Zip ufficiale (solo di versioni derivate) e hanno finestre
//     grandi: si rifiutano, e l'archivio va in «archivio non leggibile» con il motivo;
//   - sevenzip tiene un lettore aperto per ogni blocco compresso letto a metà: le voci si leggono raggruppate per
//     blocco e tutte fino in fondo, anche quelle che si scartano (vociSettezip), così resta vivo un solo
//     decodificatore alla volta.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/bodgit/sevenzip"
	"github.com/ulikunitz/xz/lzma"
)

// maxDizionario7z: il dizionario più grande che si accetta. Il 7-Zip ufficiale con la compressione massima
// usa 64 MB; 256 MB lascia margine senza permettere di esaurire la memoria del server.
const maxDizionario7z = 256 << 20

// ErrDizionario7z: l'archivio chiede un dizionario oltre maxDizionario7z.
var ErrDizionario7z = fmt.Errorf("7z: dizionario oltre %d MB, non si apre sul server", maxDizionario7z>>20)

// ErrMetodo7z: un metodo di compressione che sul server non si apre.
var ErrMetodo7z = errors.New("7z: metodo di compressione non ammesso sul server (solo quelli del 7-Zip ufficiale)")

func init() {
	sevenzip.RegisterDecompressor([]byte{0x03, 0x01, 0x01}, sevenzip.Decompressor(lettoreLZMA))
	sevenzip.RegisterDecompressor([]byte{0x21}, sevenzip.Decompressor(lettoreLZMA2))
	for _, m := range [][]byte{{0x04, 0xf7, 0x11, 0x01}, {0x04, 0xf7, 0x11, 0x02}, {0x04, 0xf7, 0x11, 0x04}} {
		sevenzip.RegisterDecompressor(m, sevenzip.Decompressor(func([]byte, uint64, []io.ReadCloser) (io.ReadCloser, error) {
			return nil, ErrMetodo7z
		}))
	}
}

// chiusuraLettore: il lettore decompresso con la chiusura del flusso sotto, come in sevenzip.
type chiusuraLettore struct {
	io.Reader
	c io.Closer
}

func (l *chiusuraLettore) Close() error { return l.c.Close() }

// lettoreLZMA: come internal/lzma di sevenzip (proprietà + dimensione davanti al flusso), con il controllo del
// dizionario: le proprietà sono 5 byte, il primo lc/lp/pb, gli altri quattro la dimensione del dizionario.
func lettoreLZMA(p []byte, s uint64, readers []io.ReadCloser) (io.ReadCloser, error) {
	if len(readers) != 1 {
		return nil, errors.New("lzma: serve un solo flusso")
	}
	if len(p) != 5 {
		return nil, errors.New("lzma: proprietà non valide")
	}
	if binary.LittleEndian.Uint32(p[1:]) > maxDizionario7z {
		return nil, ErrDizionario7z
	}
	h := bytes.NewBuffer(append([]byte(nil), p...))
	_ = binary.Write(h, binary.LittleEndian, s)
	lr, err := lzma.NewReader(io.MultiReader(h, readers[0]))
	if err != nil {
		return nil, fmt.Errorf("lzma: %w", err)
	}
	return &chiusuraLettore{Reader: lr, c: readers[0]}, nil
}

// lettoreLZMA2: come internal/lzma2 di sevenzip, con il controllo del dizionario (un byte di proprietà).
func lettoreLZMA2(p []byte, _ uint64, readers []io.ReadCloser) (io.ReadCloser, error) {
	if len(readers) != 1 {
		return nil, errors.New("lzma2: serve un solo flusso")
	}
	if len(p) != 1 || p[0] > 40 {
		return nil, errors.New("lzma2: proprietà non valide")
	}
	dict := (2 | (int64(p[0]) & 1)) << (p[0]/2 + 11)
	if dict > maxDizionario7z {
		return nil, ErrDizionario7z
	}
	config := lzma.Reader2Config{DictCap: int(dict)}
	if err := config.Verify(); err != nil {
		return nil, fmt.Errorf("lzma2: %w", err)
	}
	lr, err := config.NewReader2(readers[0])
	if err != nil {
		return nil, fmt.Errorf("lzma2: %w", err)
	}
	return &chiusuraLettore{Reader: lr, c: readers[0]}, nil
}

// vociSettezip: le voci del 7z raggruppate per blocco compresso (FileHeader.Stream), nell'ordine dell'archivio
// dentro il blocco. Una voce che il ciclo scarta si legge comunque fino in fondo (scarta), perché un blocco
// lasciato a metà terrebbe vivo il suo decodificatore fino alla chiusura dell'archivio.
func vociSettezip(r *sevenzip.ReadCloser) []voceArchivio {
	files := append([]*sevenzip.File(nil), r.File...)
	sort.SliceStable(files, func(i, j int) bool { return files[i].Stream < files[j].Stream })
	voci := make([]voceArchivio, 0, len(files))
	for _, f := range files {
		f := f
		voci = append(voci, voceArchivio{nome: f.Name, cartella: f.FileInfo().IsDir(), apri: f.Open,
			scarta: func(limite int64) error {
				rc, err := f.Open()
				if err != nil {
					return err
				}
				defer rc.Close()
				n, err := io.Copy(io.Discard, io.LimitReader(rc, limite+1))
				if err == nil && n > limite {
					err = ErrLimite
				}
				return err
			}})
	}
	return voci
}
