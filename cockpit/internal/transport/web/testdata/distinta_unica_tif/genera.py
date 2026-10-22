# -*- coding: utf-8 -*-
"""Le TIF sintetiche del banco finto della Distinta unica (anteprime TIFF, richiesta del frontend del 22/10). Disegni
inventati, nessun dato vero. Si rigenerano con:

    python -I genera.py <cartella>

  ACM-1400_ASSIEME_G4.tif  CCITT G4, 1 bit, 1600×1131 (un foglio A in piccolo: cornice, linee sottili, testo)
  ACM-1400_A_LZW.tif       LZW, grigi a 8 bit, 800×566
  ACM-1400_DEFLATE.tif     Deflate (la pagina dice «formato TIFF non supportato (Deflate)»)
  ACM-1400_ROTTA.tif       l'intestazione di un TIFF e poi niente di leggibile
"""
import os
import sys

from PIL import Image, ImageDraw


def foglio(w, h, modo, nero):
    im = Image.new(modo, (w, h), 255 if modo != "1" else 1)
    d = ImageDraw.Draw(im)
    d.rectangle([10, 10, w - 11, h - 11], outline=nero, width=3)
    for i in range(0, w, 80):
        d.line([(i, 40), (i + 200, h - 60)], fill=nero, width=1)
    d.ellipse([w // 3, h // 3, w // 3 + h // 3, 2 * h // 3], outline=nero, width=2)
    d.rectangle([w - 360, h - 120, w - 20, h - 20], outline=nero, width=2)
    d.text((w - 350, h - 110), "ACM-1400 rev A", fill=nero)
    d.text((w - 350, h - 90), "DISEGNO DI PROVA", fill=nero)
    return im


def main(cartella):
    os.makedirs(cartella, exist_ok=True)
    foglio(1600, 1131, "1", 0).save(os.path.join(cartella, "ACM-1400_ASSIEME_G4.tif"), compression="group4")
    foglio(800, 566, "L", 40).save(os.path.join(cartella, "ACM-1400_A_LZW.tif"), compression="tiff_lzw")
    foglio(400, 283, "L", 0).save(os.path.join(cartella, "ACM-1400_DEFLATE.tif"), compression="tiff_adobe_deflate")
    with open(os.path.join(cartella, "ACM-1400_ROTTA.tif"), "wb") as f:
        f.write(b"II*\x00\x08\x00\x00\x00" + bytes(range(256)) * 4)


if __name__ == "__main__":
    main(sys.argv[1] if len(sys.argv) > 1 else os.path.dirname(os.path.abspath(__file__)))
