# Controllo dei dati privati prima del push, e scansione prima di ogni commit (giro 5; R2 del 03/10, R45, R46).
#
#   $env:COCKPIT_DATASET_A = "<percorso del manifest del dataset privato>"
#   git fetch origin
#   powershell -NoProfile -ExecutionPolicy Bypass -File cockpit\scripts\controlla-privati.ps1 -Ramo <ramo>
#   powershell -NoProfile -ExecutionPolicy Bypass -File cockpit\scripts\controlla-privati.ps1 -PrimaDelCommit -Messaggio <file>
#
# PERCHE' ESISTE
#
# Il repository e' pubblico, e i dati privati del progetto (attesi, dump, export, grammatiche dei clienti, elenchi)
# vivono fuori da Git. Questo script verifica, ramo per ramo e prima di ogni push, che niente di privato stia per
# uscire: si spinge solo con l'uscita 0 su tutti e due i rami della coppia.
#
# CHE COSA CONTROLLA (sui commit non ancora su nessun remoto, `git rev-list <ramo> --not --remotes`, e sull'albero
# in punta del ramo; la storia gia' pubblicata resta fuori)
#
#   1. percorsi vietati: espressioni generiche, scritte qui, che girano sempre, anche senza elenchi;
#   2. impronte: nessun oggetto Git uguale a un file del manifest o delle radici private (grezzo, o con i fine riga
#      convertiti), quindi anche le copie rinominate;
#   3. nomi dei file: nessun nome di file privato fra i nomi tracciati o aggiunti;
#   4. token: nessun token privato nelle righe aggiunte, nei nomi dei file aggiunti, nei messaggi dei commit e
#      nell'albero in punta. Il confine di parola e' «ne' lettera ne' cifra ASCII» (anche «_» separa);
#   5. eccezioni note: valgono solo per l'albero in punta, e solo se percorso e sha256 della riga coincidono.
#
# Con -PrimaDelCommit fa i controlli 1, 3 e 4 sul diff in staging e sul messaggio preparato (-Messaggio).
#
# DOVE STANNO I DATI
#
# Qui dentro non c'e' nessun token, percorso, nome o UUID reale. Il manifest si trova con COCKPIT_DATASET_A; le sue
# voci con ruolo «controllo» indicano gli elenchi, ciascuno con sha256 e byte, che si controllano prima di usarli:
#   controllo.token     gruppi «[<gruppo> <modo>]», modo fisso (maiuscole esatte, anche dentro una parola),
#                       parola (senza maiuscole, a parola intera), sigla (maiuscole esatte, a parola intera);
#                       una voce per riga; «#» apre un commento, e un token non comincia mai con «#»;
#   controllo.nomi      nomi di file privati, uno per riga, confrontati senza maiuscole con il nome senza cartella;
#                       si aggiungono da se' i nomi dei file delle voci del manifest;
#   controllo.eccezioni percorso<TAB>sha256 della riga<TAB>gruppo<TAB>motivo; per un nome di file la «riga» e' il
#                       percorso stesso; il gruppo dei nomi privati e' «nomi»;
#   controllo.radici    cartelle private da confrontare per impronta, una per riga, assolute o relative al manifest
#                       (facoltativo).
# I token restano in memoria: non passano da una riga di comando ne' da un file temporaneo. A video non compare mai
# un token: solo il gruppo, una maschera («AC…E(4)») e dove si trova.
#
# La ricerca vera la fa un piccolo cercatore C# compilato al volo (Add-Type, .NET Framework di Windows): con
# migliaia di token e le righe lunghissime dei file minimizzati, un ciclo in PowerShell o `git grep -F -f` (che
# prova un token alla volta) impiegano minuti; il cercatore legge i blob con `git cat-file --batch` e guarda, in
# ogni posizione, solo i token che cominciano con quei due caratteri.
#
# USCITE (le stesse del banco e del riepilogo delle prove)
#
#   0  i controlli sono eseguiti e non trovano niente fuori dalle eccezioni
#   1  almeno un controllo trova qualcosa: non si spinge (o non si committa)
#   2  uso o configurazione: non e' un repository, il ramo o i remoti non esistono, parametri o elenchi malformati
#   3  NON ESEGUITO: manifest o elenchi non raggiungibili; il controllo 1 gira comunque e il suo esito si stampa
#
# Una scoperta prevale su un NON ESEGUITO. Lo script non scrive niente nel repository e non fa il fetch: lo fa la
# procedura, e qui si avvisa se l'ultimo ha piu' di un giorno.
param(
    [string]$Ramo,
    [switch]$PrimaDelCommit,
    [string]$Messaggio,
    [int]$MassimoDettagli = 200
)

$ErrorActionPreference = "Stop"
try { [Console]::OutputEncoding = New-Object System.Text.UTF8Encoding $false } catch { }

$script:utf8 = New-Object System.Text.UTF8Encoding $false

$codiceCercatore = @'
using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Security.Cryptography;
using System.Text;
using System.Threading;

namespace ControlloPrivati
{
    public sealed class Occorrenza
    {
        public string Percorso;
        public int Riga;
        public string Testo;
        public int Token;
        public int Pos;
    }

    public sealed class Cercatore
    {
        private readonly string[] token;
        private readonly int[] modo; // 0 fisso, 1 sigla, 2 parola
        private readonly Dictionary<int, List<int>> esatti = new Dictionary<int, List<int>>();
        private readonly Dictionary<int, List<int>> liberi = new Dictionary<int, List<int>>();

        public Cercatore(string[] token, int[] modo)
        {
            this.token = token;
            this.modo = modo;
            for (int i = 0; i < token.Length; i++)
            {
                string t = token[i];
                if (t.Length < 2) continue;
                Dictionary<int, List<int>> d = modo[i] == 2 ? liberi : esatti;
                int k = modo[i] == 2 ? Chiave(char.ToLowerInvariant(t[0]), char.ToLowerInvariant(t[1])) : Chiave(t[0], t[1]);
                List<int> l;
                if (!d.TryGetValue(k, out l)) { l = new List<int>(); d[k] = l; }
                l.Add(i);
            }
        }

        private static int Chiave(char a, char b) { return (a << 16) | b; }

        private static bool Alnum(string s, int i)
        {
            if (i < 0 || i >= s.Length) return false;
            char c = s[i];
            return (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z');
        }

        // Le occorrenze in s, come coppie {indice del token, posizione}.
        public List<int[]> Trova(string s)
        {
            List<int[]> trovate = new List<int[]>();
            if (s == null) return trovate;
            for (int i = 0; i + 1 < s.Length; i++)
            {
                List<int> l;
                if (esatti.TryGetValue(Chiave(s[i], s[i + 1]), out l))
                {
                    foreach (int j in l)
                    {
                        string t = token[j];
                        if (i + t.Length > s.Length || string.CompareOrdinal(s, i, t, 0, t.Length) != 0) continue;
                        if (modo[j] == 1 && (Alnum(s, i - 1) || Alnum(s, i + t.Length))) continue;
                        trovate.Add(new int[] { j, i });
                    }
                }
                if (liberi.TryGetValue(Chiave(char.ToLowerInvariant(s[i]), char.ToLowerInvariant(s[i + 1])), out l))
                {
                    foreach (int j in l)
                    {
                        string t = token[j];
                        if (i + t.Length > s.Length || string.Compare(s, i, t, 0, t.Length, StringComparison.OrdinalIgnoreCase) != 0) continue;
                        if (Alnum(s, i - 1) || Alnum(s, i + t.Length)) continue;
                        trovate.Add(new int[] { j, i });
                    }
                }
            }
            return trovate;
        }

        // Tutti i blob di testo dell'albero del ramo (come `git grep -I`: un NUL nei primi 8000 byte vuol dire binario).
        public List<Occorrenza> ScansionaAlbero(string git, string cartella, string ramo)
        {
            List<Occorrenza> trovate = new List<Occorrenza>();
            byte[] elenco = Esegui(git, cartella, "ls-tree -r -z \"" + ramo + "\"");
            List<string> ids = new List<string>();
            List<string> percorsi = new List<string>();
            int inizio = 0;
            for (int i = 0; i < elenco.Length; i++)
            {
                if (elenco[i] != 0) continue;
                string rec = Encoding.UTF8.GetString(elenco, inizio, i - inizio);
                inizio = i + 1;
                int tab = rec.IndexOf('\t');
                if (tab < 0) continue;
                string[] testa = rec.Substring(0, tab).Split(' ');
                if (testa.Length < 3 || testa[1] != "blob") continue;
                ids.Add(testa[2]);
                percorsi.Add(rec.Substring(tab + 1));
            }
            if (ids.Count == 0) return trovate;
            ProcessStartInfo psi = Avvio(git, cartella, "cat-file --batch");
            psi.RedirectStandardInput = true;
            using (Process p = Process.Start(psi))
            {
                p.ErrorDataReceived += delegate { };
                p.BeginErrorReadLine();
                Stream ingresso = p.StandardInput.BaseStream;
                Thread scrittore = new Thread(delegate ()
                {
                    // Una riga vuota in testa: .NET Framework puo' aprire lo stdin del processo con il BOM della
                    // codifica d'ingresso della console; la riga vuota lo assorbe, e la sua risposta si scarta.
                    byte[] richieste = Encoding.ASCII.GetBytes("\n" + string.Join("\n", ids.ToArray()) + "\n");
                    ingresso.Write(richieste, 0, richieste.Length);
                    ingresso.Close();
                });
                scrittore.Start();
                Stream uscita = new BufferedStream(p.StandardOutput.BaseStream, 1 << 16);
                if (!LeggiRiga(uscita).EndsWith(" missing")) throw new InvalidOperationException("git cat-file: risposta inattesa alla riga vuota");
                for (int n = 0; n < ids.Count; n++)
                {
                    string testa = LeggiRiga(uscita);
                    string[] parti = testa.Split(' ');
                    if (parti.Length < 3) throw new InvalidOperationException("git cat-file: risposta inattesa per " + ids[n] + ": " + testa);
                    byte[] dati = LeggiEsatti(uscita, int.Parse(parti[2]));
                    LeggiEsatti(uscita, 1);
                    if (!Binario(dati)) ScansionaTesto(Encoding.UTF8.GetString(dati), percorsi[n], trovate);
                }
                scrittore.Join();
                p.WaitForExit();
                if (p.ExitCode != 0) throw new InvalidOperationException("git cat-file non riuscito");
            }
            return trovate;
        }

        private void ScansionaTesto(string s, string percorso, List<Occorrenza> trovate)
        {
            List<int[]> occ = Trova(s);
            if (occ.Count == 0) return;
            List<int> inizi = new List<int>();
            inizi.Add(0);
            for (int i = 0; i < s.Length; i++) if (s[i] == '\n') inizi.Add(i + 1);
            foreach (int[] o in occ)
            {
                int r = inizi.BinarySearch(o[1]);
                if (r < 0) r = ~r - 1;
                int a = inizi[r];
                int b = r + 1 < inizi.Count ? inizi[r + 1] - 1 : s.Length;
                Occorrenza x = new Occorrenza();
                x.Percorso = percorso;
                x.Riga = r + 1;
                x.Testo = s.Substring(a, b - a).TrimEnd('\r');
                x.Token = o[0];
                x.Pos = o[1] - a;
                trovate.Add(x);
            }
        }

        // Gli identificativi Git di un contenuto, come `git hash-object` senza filtri: grezzo e, per un testo, con
        // i fine riga LF e CRLF.
        public static string[] IdBlob(byte[] b)
        {
            List<string> ids = new List<string>();
            ids.Add(Id(b));
            if (!Binario(b))
            {
                byte[] lf = SoloLF(b);
                foreach (byte[] v in new byte[][] { lf, ConCRLF(lf) })
                {
                    string id = Id(v);
                    if (!ids.Contains(id)) ids.Add(id);
                }
            }
            return ids.ToArray();
        }

        private static string Id(byte[] b)
        {
            using (SHA1 h = SHA1.Create())
            {
                byte[] testa = Encoding.ASCII.GetBytes("blob " + b.Length + "\0");
                h.TransformBlock(testa, 0, testa.Length, null, 0);
                h.TransformFinalBlock(b, 0, b.Length);
                StringBuilder sb = new StringBuilder(40);
                foreach (byte x in h.Hash) sb.Append(x.ToString("x2"));
                return sb.ToString();
            }
        }

        private static byte[] SoloLF(byte[] b)
        {
            MemoryStream m = new MemoryStream(b.Length);
            for (int i = 0; i < b.Length; i++)
            {
                if (b[i] == 13 && i + 1 < b.Length && b[i + 1] == 10) continue;
                m.WriteByte(b[i]);
            }
            return m.ToArray();
        }

        private static byte[] ConCRLF(byte[] lf)
        {
            MemoryStream m = new MemoryStream(lf.Length + lf.Length / 16 + 1);
            foreach (byte x in lf)
            {
                if (x == 10) m.WriteByte(13);
                m.WriteByte(x);
            }
            return m.ToArray();
        }

        private static bool Binario(byte[] b)
        {
            int n = Math.Min(b.Length, 8000);
            for (int i = 0; i < n; i++) if (b[i] == 0) return true;
            return false;
        }

        private static ProcessStartInfo Avvio(string git, string cartella, string argomenti)
        {
            ProcessStartInfo psi = new ProcessStartInfo(git, argomenti);
            psi.WorkingDirectory = cartella;
            psi.UseShellExecute = false;
            psi.CreateNoWindow = true;
            psi.RedirectStandardOutput = true;
            psi.RedirectStandardError = true;
            return psi;
        }

        private static byte[] Esegui(string git, string cartella, string argomenti)
        {
            using (Process p = Process.Start(Avvio(git, cartella, argomenti)))
            {
                p.ErrorDataReceived += delegate { };
                p.BeginErrorReadLine();
                MemoryStream m = new MemoryStream();
                p.StandardOutput.BaseStream.CopyTo(m);
                p.WaitForExit();
                if (p.ExitCode != 0) throw new InvalidOperationException("git ls-tree non riuscito");
                return m.ToArray();
            }
        }

        private static string LeggiRiga(Stream o)
        {
            MemoryStream m = new MemoryStream();
            int c;
            while ((c = o.ReadByte()) >= 0 && c != '\n') m.WriteByte((byte)c);
            if (c < 0) throw new EndOfStreamException("git cat-file: uscita interrotta");
            return Encoding.ASCII.GetString(m.ToArray());
        }

        private static byte[] LeggiEsatti(Stream o, int n)
        {
            byte[] b = new byte[n];
            int letti = 0;
            while (letti < n)
            {
                int r = o.Read(b, letti, n - letti);
                if (r <= 0) throw new EndOfStreamException("git cat-file: uscita interrotta");
                letti += r;
            }
            return b;
        }
    }
}
'@

function Riga([string]$testo) { [Console]::Out.WriteLine($testo) }

# --- Git per i comandi brevi, con l'uscita letta come UTF-8 (PowerShell 5.1 userebbe la codepage della console) ---

function Quota([string]$a) {
    if ($a -eq '') { return '""' }
    if ($a -notmatch '[\s"]') { return $a }
    $sb = New-Object System.Text.StringBuilder
    [void]$sb.Append('"')
    $barre = 0
    foreach ($c in $a.ToCharArray()) {
        if ($c -eq [char]92) { $barre++; continue }
        if ($c -eq [char]34) { [void]$sb.Append([string]::new([char]92, 2 * $barre + 1)).Append('"'); $barre = 0; continue }
        if ($barre -gt 0) { [void]$sb.Append([string]::new([char]92, $barre)); $barre = 0 }
        [void]$sb.Append($c)
    }
    if ($barre -gt 0) { [void]$sb.Append([string]::new([char]92, 2 * $barre)) }
    [void]$sb.Append('"')
    return $sb.ToString()
}

function Git([string[]]$Argomenti) {
    $psi = New-Object System.Diagnostics.ProcessStartInfo
    $psi.FileName = $script:git
    $tutti = @('-c', 'core.quotePath=false', '-c', 'color.ui=never') + $Argomenti
    $psi.Arguments = (($tutti | ForEach-Object { Quota $_ }) -join ' ')
    $psi.UseShellExecute = $false
    $psi.CreateNoWindow = $true
    $psi.RedirectStandardOutput = $true
    $psi.RedirectStandardError = $true
    $psi.StandardOutputEncoding = $script:utf8
    $psi.StandardErrorEncoding = $script:utf8
    $psi.WorkingDirectory = (Get-Location).Path
    $p = [System.Diagnostics.Process]::Start($psi)
    $errore = $p.StandardError.ReadToEndAsync()
    $uscita = $p.StandardOutput.ReadToEnd()
    $p.WaitForExit()
    return [pscustomobject]@{ Codice = $p.ExitCode; Testo = $uscita; Errore = $errore.Result }
}

function GitOk([string[]]$Argomenti, [string]$Cosa) {
    $r = Git $Argomenti
    if ($r.Codice -ne 0) { throw (New-Object System.InvalidOperationException ("git non riuscito: " + $Cosa)) }
    return $r.Testo
}

function Righe([string]$testo) {
    if ([string]::IsNullOrEmpty($testo)) { return @() }
    return @($testo -split "`n" | ForEach-Object { $_.TrimEnd("`r") } | Where-Object { $_ -ne '' })
}

function Sha256Testo([string]$t) {
    $h = [System.Security.Cryptography.SHA256]::Create()
    return -join ($h.ComputeHash($script:utf8.GetBytes($t)) | ForEach-Object { $_.ToString('x2') })
}

function Sha256File([string]$p) { return (Get-FileHash -Algorithm SHA256 -LiteralPath $p).Hash.ToLowerInvariant() }

# --- Token: elenco, ricerca, maschera ---

function Maschera([string]$t) {
    if ($t.Length -le 3) { return $t.Substring(0, 1) + [char]0x2026 + '(' + $t.Length + ')' }
    return $t.Substring(0, 2) + [char]0x2026 + $t.Substring($t.Length - 1) + '(' + $t.Length + ')'
}

function LeggiToken([string]$p) {
    $voci = New-Object System.Collections.Generic.List[object]
    $gruppo = $null; $modo = $null; $n = 0
    foreach ($r in [System.IO.File]::ReadAllLines($p, $script:utf8)) {
        $n++
        $t = $r.Trim()
        if ($t -eq '' -or $t.StartsWith('#')) { continue }
        if ($t -match '^\[(\S+)\s+(fisso|parola|sigla)\]$') { $gruppo = $Matches[1]; $modo = $Matches[2]; continue }
        if ($t.StartsWith('[')) { throw (New-Object System.FormatException ("elenco dei token, riga " + $n + ": gruppo malformato")) }
        if ($null -eq $gruppo) { throw (New-Object System.FormatException ("elenco dei token, riga " + $n + ": voce fuori da un gruppo")) }
        if ($t.Length -lt 2) { throw (New-Object System.FormatException ("elenco dei token, riga " + $n + ": voce troppo corta")) }
        $voci.Add([pscustomobject]@{ Gruppo = $gruppo; Modo = $modo; Token = $t })
    }
    return , $voci
}

function Trova([string]$s) {
    $r = New-Object System.Collections.Generic.List[object]
    if ($null -ne $script:cercatore -and -not [string]::IsNullOrEmpty($s)) {
        foreach ($c in $script:cercatore.Trova($s)) { $r.Add([pscustomobject]@{ Voce = $script:voci[$c[0]]; Pos = $c[1] }) }
    }
    return , $r
}

# Un percorso o un testo da stampare: i token e i nomi privati diventano maschere.
function MascheraTesto([string]$s) {
    if ($null -eq $script:cercatore) { return $s }
    foreach ($o in @((Trova $s) | Sort-Object -Property Pos -Descending)) {
        $n = $o.Voce.Token.Length
        if ($o.Pos + $n -le $s.Length) { $s = $s.Substring(0, $o.Pos) + (Maschera $s.Substring($o.Pos, $n)) + $s.Substring($o.Pos + $n) }
    }
    $nome = ($s -split '/')[-1]
    if ($script:nomi.Contains($nome)) { $s = $s.Substring(0, $s.Length - $nome.Length) + (Maschera $nome) }
    return $s
}

# --- Diff: righe aggiunte con file e numero di riga, da un diff con -U0 ---

function RigheAggiunte([string]$diff, [string]$dove) {
    $out = New-Object System.Collections.Generic.List[object]
    $file = $null; $n = 0
    foreach ($l in ($diff -split "`n")) {
        if ($l.StartsWith('+++ ')) {
            $f = $l.Substring(4).TrimEnd("`r")
            if ($f -eq '/dev/null') { $file = $null } else { $file = $f.Trim('"'); if ($file.StartsWith('b/')) { $file = $file.Substring(2) } }
            continue
        }
        if ($l -match '^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@') { $n = [int]$Matches[1]; continue }
        if ($l.StartsWith('+') -and $null -ne $file) {
            $out.Add([pscustomobject]@{ Dove = $dove; File = $file; Riga = $n; Testo = $l.Substring(1).TrimEnd("`r") })
            $n++
        }
    }
    return , $out
}

# --- Il controllo ---

$script:trovate = 0
$script:dettagli = @{}
$script:avvisi = New-Object System.Collections.Generic.List[string]
$script:cercatore = $null
$script:voci = $null
$script:nomi = New-Object 'System.Collections.Generic.HashSet[string]' ([System.StringComparer]::OrdinalIgnoreCase)

function Trovato([string]$controllo, [string]$testo) {
    $script:trovate++
    if (-not $script:dettagli.ContainsKey($controllo)) { $script:dettagli[$controllo] = 0 }
    $script:dettagli[$controllo]++
    if ($script:dettagli[$controllo] -le $MassimoDettagli) { Riga ("[" + $controllo + "] " + $testo) }
    elseif ($script:dettagli[$controllo] -eq $MassimoDettagli + 1) { Riga ("[" + $controllo + "] ... (altre occorrenze non mostrate)") }
}

function Esegui {
    $g = Get-Command git -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($null -eq $g) { return @(2, "git non e' nel PATH") }
    $script:git = $g.Source

    if ($PrimaDelCommit) {
        if ($Ramo) { return @(2, "-Ramo e -PrimaDelCommit insieme") }
        if ($Messaggio -and -not (Test-Path -LiteralPath $Messaggio -PathType Leaf)) { return @(2, "il file del messaggio non esiste") }
    } else {
        if (-not $Ramo) { return @(2, "manca -Ramo <ramo> (oppure -PrimaDelCommit)") }
        if ($Messaggio) { return @(2, "-Messaggio vale solo con -PrimaDelCommit") }
    }
    if ((Git @('rev-parse', '--show-toplevel')).Codice -ne 0) { return @(2, "la cartella corrente non e' un repository Git") }

    $commit = @()
    if ($PrimaDelCommit) {
        Riga "Controllo dei dati privati: diff in staging e messaggio (controlli 1, 3 e 4)"
    } else {
        if ((Git @('rev-parse', '--verify', '--quiet', ('refs/heads/' + $Ramo + '^{commit}'))).Codice -ne 0) { return @(2, "il ramo non esiste: " + $Ramo) }
        if ((Righe (GitOk @('for-each-ref', '--format=%(refname)', 'refs/remotes') 'riferimenti remoti')).Count -eq 0) {
            return @(2, "nessun riferimento remoto: serve un git fetch")
        }
        Riga ("Controllo dei dati privati: ramo " + $Ramo)
        # FETCH_HEAD e' del worktree, non della cartella comune: lo dice --git-path.
        $fetch = (GitOk @('rev-parse', '--git-path', 'FETCH_HEAD') 'FETCH_HEAD').Trim()
        if (-not (Test-Path -LiteralPath $fetch)) { $script:avvisi.Add("nessun git fetch registrato in questo worktree: i riferimenti remoti potrebbero essere vecchi") }
        elseif (((Get-Date) - (Get-Item -LiteralPath $fetch).LastWriteTime).TotalHours -gt 24) { $script:avvisi.Add("l'ultimo git fetch ha piu' di un giorno") }
        $commit = @(Righe (GitOk @('rev-list', $Ramo, '--not', '--remotes') 'commit da pubblicare'))
        Riga ("Commit da pubblicare: " + $commit.Count)
    }

    # Che cosa si guarda: nomi (dell'albero e aggiunti), righe aggiunte, messaggi.
    $nomiPunta = @()
    $nomiNuovi = New-Object System.Collections.Generic.List[object]
    $aggiunte = New-Object System.Collections.Generic.List[object]
    $messaggi = New-Object System.Collections.Generic.List[object]
    if ($PrimaDelCommit) {
        foreach ($f in ((GitOk @('diff', '--cached', '--name-only', '--diff-filter=ACR', '-z') 'nomi in staging') -split [char]0)) {
            if ($f) { $nomiNuovi.Add([pscustomobject]@{ Dove = 'staging'; File = $f }) }
        }
        $d = GitOk @('diff', '--cached', '-U0', '--no-color', '--no-ext-diff', '--src-prefix=a/', '--dst-prefix=b/') 'diff in staging'
        foreach ($x in (RigheAggiunte $d 'staging')) { $aggiunte.Add($x) }
        if ($Messaggio) {
            $n = 0
            foreach ($l in [System.IO.File]::ReadAllLines($Messaggio, $script:utf8)) { $n++; $messaggi.Add([pscustomobject]@{ Dove = 'messaggio'; Riga = $n; Testo = $l }) }
        }
    } else {
        $nomiPunta = @((GitOk @('ls-tree', '-r', '--name-only', '-z', $Ramo) 'albero in punta') -split [char]0 | Where-Object { $_ })
        foreach ($c in $commit) {
            $breve = $c.Substring(0, 7)
            $genitori = @((GitOk @('rev-list', '--parents', '-n', '1', $c) 'genitori').Trim() -split ' ')
            if ($genitori.Count -le 2) {
                $nn = GitOk @('diff-tree', '-r', '--root', '--no-commit-id', '--name-only', '--diff-filter=ACR', '-z', $c) 'nomi del commit'
                $d = GitOk @('diff-tree', '-r', '--root', '--no-commit-id', '-p', '-U0', '--no-color', '--no-ext-diff', '--src-prefix=a/', '--dst-prefix=b/', $c) 'diff del commit'
            } else {
                # un merge: le righe nuove rispetto al primo genitore
                $nn = GitOk @('diff', '--name-only', '--diff-filter=ACR', '-z', ($c + '^1'), $c) 'nomi del commit'
                $d = GitOk @('diff', '-U0', '--no-color', '--no-ext-diff', '--src-prefix=a/', '--dst-prefix=b/', ($c + '^1'), $c) 'diff del commit'
            }
            foreach ($f in ($nn -split [char]0)) { if ($f) { $nomiNuovi.Add([pscustomobject]@{ Dove = ('commit ' + $breve); File = $f }) } }
            foreach ($x in (RigheAggiunte $d ('commit ' + $breve))) { $aggiunte.Add($x) }
            $n = 0
            foreach ($l in ((GitOk @('log', '-1', '--format=%B', $c) 'messaggio') -split "`n")) {
                $n++
                $messaggi.Add([pscustomobject]@{ Dove = ('messaggio del commit ' + $breve); Riga = $n; Testo = $l.TrimEnd("`r") })
            }
        }
    }
    # In PowerShell 5.1 «array + List» fallisce («Tipi di argomento non corrispondenti»): si aggiunge una voce per volta.
    $daGuardare = New-Object System.Collections.Generic.List[object]
    foreach ($f in $nomiPunta) { $daGuardare.Add([pscustomobject]@{ Dove = 'albero in punta'; File = $f }) }
    foreach ($x in $nomiNuovi) { $daGuardare.Add($x) }

    # 1. Percorsi vietati: espressioni ancorate ai nomi dei file privati, non alle parole (il pacchetto
    #    internal/platform/dataset e il file attesi.go del runner sono codice, non dati). Si stampano dopo, con le
    #    maschere, se gli elenchi ci sono.
    $vietati = @('(^|/)docs/', '(^|/)specs/', '^(cockpit/)?dataset/', '(^|/)exports_db/', '(^|/)esiti_qa/', '\.dump$',
        '(^|/)attesi-[^/]*\.yaml$', '(^|/)manifest-a\.json$', '\.eml$', '\.msg$', '(^|/)cockpit\.toml$', '(^|/)pgpass[^/]*$')
    $primo = New-Object System.Collections.Generic.List[object]
    foreach ($x in $daGuardare) {
        foreach ($e in $vietati) { if ($x.File -match $e) { $primo.Add(@($e, $x)); break } }
    }
    $esito1 = if ($primo.Count -eq 0) { "pulito" } else { "" + $primo.Count + " percorsi vietati" }

    # Il dataset privato: manifest ed elenchi, ciascuno con sha256 e byte del manifest.
    $mancante = $null
    $manifest = $null
    $cartella = $null
    $elenchi = @{}
    $richiesti = if ($PrimaDelCommit) { @('controllo.token', 'controllo.nomi') } else { @('controllo.token', 'controllo.nomi', 'controllo.eccezioni') }
    $percorsoManifest = $env:COCKPIT_DATASET_A
    if (-not $percorsoManifest) { $mancante = "COCKPIT_DATASET_A non impostata" }
    elseif (-not (Test-Path -LiteralPath $percorsoManifest -PathType Leaf)) { $mancante = "il manifest indicato da COCKPIT_DATASET_A non esiste" }
    else {
        try { $manifest = [System.IO.File]::ReadAllText($percorsoManifest, $script:utf8) | ConvertFrom-Json } catch { $mancante = "manifest illeggibile" }
        if ($null -eq $mancante) {
            $cartella = Split-Path -Parent ([System.IO.Path]::GetFullPath($percorsoManifest))
            Riga ("Manifest: sha256 " + (Sha256File $percorsoManifest))
            foreach ($nome in @('controllo.token', 'controllo.nomi', 'controllo.eccezioni', 'controllo.radici')) {
                $v = @($manifest.voci | Where-Object { $_.nome -eq $nome })
                if ($v.Count -eq 0) {
                    if ($richiesti -contains $nome) { $mancante = "il manifest non ha la voce " + $nome; break }
                    continue
                }
                $p = [System.IO.Path]::GetFullPath((Join-Path $cartella $v[0].percorso))
                if (-not (Test-Path -LiteralPath $p -PathType Leaf)) { $mancante = "elenco non raggiungibile: " + $nome; break }
                $sha = Sha256File $p
                if ($sha -ne ([string]$v[0].sha256).ToLowerInvariant() -or (Get-Item -LiteralPath $p).Length -ne [int64]$v[0].byte) {
                    $mancante = "sha256 o byte diversi dal manifest: " + $nome; break
                }
                $elenchi[$nome] = $p
                Riga ("Elenco " + $nome + ": sha256 " + $sha)
            }
        }
    }

    $eccezioni = New-Object System.Collections.Generic.List[object]
    if ($null -eq $mancante) {
        try {
            $script:voci = LeggiToken $elenchi['controllo.token']
            foreach ($r in [System.IO.File]::ReadAllLines($elenchi['controllo.nomi'], $script:utf8)) {
                $t = $r.Trim()
                if ($t -and -not $t.StartsWith('#')) { [void]$script:nomi.Add($t) }
            }
            foreach ($v in $manifest.voci) { [void]$script:nomi.Add((Split-Path -Leaf ([string]$v.percorso))) }
            if (-not $PrimaDelCommit) {
                $n = 0
                foreach ($r in [System.IO.File]::ReadAllLines($elenchi['controllo.eccezioni'], $script:utf8)) {
                    $n++
                    if ($r.Trim() -eq '' -or $r.TrimStart().StartsWith('#')) { continue }
                    $c = $r.Split("`t")
                    if ($c.Count -lt 3 -or $c[1] -notmatch '^[0-9a-f]{64}$') { throw (New-Object System.FormatException ("elenco delle eccezioni, riga " + $n + ": servono percorso, sha256 e gruppo separati da TAB")) }
                    $eccezioni.Add([pscustomobject]@{ Chiave = ($c[0] + "`t" + $c[1] + "`t" + $c[2]); Percorso = $c[0]; Sha = $c[1]; Gruppo = $c[2]; Usata = $false })
                }
            }
        } catch [System.FormatException] {
            return @(2, $_.Exception.Message)
        }
        if (-not ('ControlloPrivati.Cercatore' -as [type])) { Add-Type -TypeDefinition $codiceCercatore -Language CSharp }
        [string[]]$tok = @($script:voci | ForEach-Object { $_.Token })
        [int[]]$modi = @($script:voci | ForEach-Object { switch ($_.Modo) { 'fisso' { 0 } 'sigla' { 1 } default { 2 } } })
        $script:cercatore = [ControlloPrivati.Cercatore]::new($tok, $modi)
    }

    foreach ($p in $primo) { Trovato '1' ("percorso vietato (" + $p[0] + "): " + (MascheraTesto $p[1].File) + " — " + $p[1].Dove) }

    $note = 0
    $inPunta = New-Object 'System.Collections.Generic.HashSet[string]' ([System.StringComparer]::Ordinal)
    foreach ($f in $nomiPunta) { [void]$inPunta.Add($f) }
    if ($null -eq $mancante) {
        $mappaEcc = @{}
        foreach ($e in $eccezioni) { $mappaEcc[$e.Chiave] = $e }

        # 3. Nomi dei file privati.
        foreach ($x in $daGuardare) {
            $nome = ($x.File -split '/')[-1]
            if (-not $script:nomi.Contains($nome)) { continue }
            $chiave = $x.File + "`t" + (Sha256Testo $x.File) + "`tnomi"
            if ($x.Dove -eq 'albero in punta' -and $mappaEcc.ContainsKey($chiave)) { $mappaEcc[$chiave].Usata = $true; $note++; continue }
            Trovato '3' ("nome di file privato: " + (MascheraTesto $x.File) + " — " + $x.Dove)
        }

        # 4. Token: nell'albero in punta (contenuti e nomi), nelle righe aggiunte, nei nomi aggiunti, nei messaggi.
        $visti = New-Object 'System.Collections.Generic.HashSet[string]'
        $script:noteToken = 0
        $segnala = {
            param($occorrenze, $dove, $chiaveEcc)
            foreach ($o in $occorrenze) {
                if (-not $visti.Add($dove + "`t" + $o.Voce.Gruppo + "`t" + $o.Voce.Token)) { continue }
                if ($null -ne $chiaveEcc) {
                    $k = $chiaveEcc + "`t" + $o.Voce.Gruppo
                    if ($mappaEcc.ContainsKey($k)) { $mappaEcc[$k].Usata = $true; $script:noteToken++; continue }
                }
                Trovato '4' ($o.Voce.Gruppo + " " + (Maschera $o.Voce.Token) + " — " + $dove)
            }
        }
        if (-not $PrimaDelCommit) {
            foreach ($o in $script:cercatore.ScansionaAlbero($script:git, (Get-Location).Path, $Ramo)) {
                $occ = @([pscustomobject]@{ Voce = $script:voci[$o.Token]; Pos = $o.Pos })
                & $segnala $occ ("albero in punta, " + (MascheraTesto $o.Percorso) + ":" + $o.Riga) ($o.Percorso + "`t" + (Sha256Testo $o.Testo))
            }
            foreach ($f in $nomiPunta) {
                $occ = Trova $f
                if ($occ.Count -gt 0) { & $segnala $occ ("albero in punta, nome del file " + (MascheraTesto $f)) ($f + "`t" + (Sha256Testo $f)) }
            }
        }
        foreach ($x in $aggiunte) {
            $occ = Trova $x.Testo
            if ($occ.Count -gt 0) { & $segnala $occ ($x.Dove + ", " + (MascheraTesto $x.File) + ":" + $x.Riga) $null }
        }
        foreach ($x in $nomiNuovi) {
            $occ = Trova $x.File
            if ($occ.Count -gt 0) { & $segnala $occ ($x.Dove + ", nome del file " + (MascheraTesto $x.File)) $null }
        }
        foreach ($x in $messaggi) {
            $occ = Trova $x.Testo
            if ($occ.Count -gt 0) { & $segnala $occ ($x.Dove + ", riga " + $x.Riga) $null }
        }
        $note += $script:noteToken
    }

    # Righe aggiunte con un rimando a docs/: un riferimento funzionale e' ammesso, il nome di un documento privato no.
    foreach ($x in $aggiunte) {
        if ($x.Testo -match 'docs[/\\]') { $script:avvisi.Add("riga aggiunta con un rimando a docs/ da leggere: " + $x.Dove + ", " + (MascheraTesto $x.File) + ":" + $x.Riga) }
    }

    if ($null -eq $mancante -and -not $PrimaDelCommit) {
        # 2. Impronte: i file del manifest, il manifest stesso e quelli sotto le radici, contro gli oggetti dei commit
        #    da pubblicare (errore) e i blob dell'albero in punta gia' pubblici (avviso: copia di un file pubblico).
        $file = New-Object System.Collections.Generic.List[string]
        $file.Add([System.IO.Path]::GetFullPath($percorsoManifest))
        foreach ($v in $manifest.voci) {
            $p = [System.IO.Path]::GetFullPath((Join-Path $cartella $v.percorso))
            if (-not (Test-Path -LiteralPath $p -PathType Leaf)) { $mancante = "file del manifest non raggiungibile: " + $v.nome; break }
            $file.Add($p)
        }
        if ($null -eq $mancante -and $elenchi.ContainsKey('controllo.radici')) {
            foreach ($r in [System.IO.File]::ReadAllLines($elenchi['controllo.radici'], $script:utf8)) {
                $t = $r.Trim()
                if ($t -eq '' -or $t.StartsWith('#')) { continue }
                $rad = if ([System.IO.Path]::IsPathRooted($t)) { $t } else { [System.IO.Path]::GetFullPath((Join-Path $cartella $t)) }
                if (-not (Test-Path -LiteralPath $rad -PathType Container)) { $mancante = "radice privata non raggiungibile"; break }
                foreach ($f in (Get-ChildItem -LiteralPath $rad -Recurse -File -Force)) { $file.Add($f.FullName) }
            }
        }
        if ($null -eq $mancante) {
            $nuovi = @{}
            foreach ($l in (Righe (GitOk @('rev-list', '--objects', $Ramo, '--not', '--remotes') 'oggetti da pubblicare'))) {
                $nuovi[$l.Substring(0, 40)] = if ($l.Length -gt 41) { $l.Substring(41) } else { '' }
            }
            $punta = @{}
            foreach ($rec in ((GitOk @('ls-tree', '-r', '-z', $Ramo) 'blob in punta') -split [char]0)) {
                if ($rec -match '^\d+ blob ([0-9a-f]{40})\t(.*)$') { $punta[$Matches[1]] = $Matches[2] }
            }
            $vuoti = 0
            $fatti = New-Object 'System.Collections.Generic.HashSet[string]' ([System.StringComparer]::OrdinalIgnoreCase)
            foreach ($p in $file) {
                if (-not $fatti.Add($p)) { continue }
                $b = [System.IO.File]::ReadAllBytes($p)
                if ($b.Length -eq 0) { $vuoti++; continue }
                foreach ($id in [ControlloPrivati.Cercatore]::IdBlob($b)) {
                    if ($nuovi.ContainsKey($id)) {
                        Trovato '2' ("un oggetto da pubblicare e' uguale a un file privato (" + (MascheraTesto (Split-Path -Leaf $p)) + "): " + (MascheraTesto $nuovi[$id]))
                    } elseif ($punta.ContainsKey($id)) {
                        $script:avvisi.Add("un file privato (" + (MascheraTesto (Split-Path -Leaf $p)) + ") e' uguale a un file gia' pubblico: " + (MascheraTesto $punta[$id]))
                    }
                }
            }
            if ($vuoti -gt 0) { $script:avvisi.Add("" + $vuoti + " file privati vuoti non confrontati") }
            Riga ("Impronte confrontate: " + $fatti.Count + " file privati")
        }

        # 5. Eccezioni che non trovano piu' la loro riga. Quelle di un file che in questo ramo non c'e' (per esempio
        #    una prova, che il ramo di prodotto non ha) non sono scadute: riguardano l'altro ramo della coppia.
        foreach ($e in $eccezioni) {
            if (-not $e.Usata -and $inPunta.Contains($e.Percorso)) {
                $script:avvisi.Add("eccezione scaduta, toglierla: " + (MascheraTesto $e.Percorso) + ", riga " + $e.Sha.Substring(0, 8) + ", gruppo " + $e.Gruppo)
            }
        }
    }

    foreach ($a in $script:avvisi) { Riga ("avviso: " + $a) }
    $cosa = if ($PrimaDelCommit) { "commit" } else { "push" }
    if ($script:trovate -gt 0) {
        if ($null -ne $mancante) { Riga ("NON ESEGUITO in parte: " + $mancante) }
        return @(1, ("TROVATO: " + $script:trovate + " occorrenze; " + $cosa + " da non fare"))
    }
    if ($null -ne $mancante) { return @(3, ("NON ESEGUITO: " + $mancante + "; controllo 1: " + $esito1)) }
    if ($PrimaDelCommit) { return @(0, "PULITO: controlli 1, 3 e 4 eseguiti sul diff in staging e sul messaggio") }
    return @(0, ("PULITO: 5 controlli eseguiti; occorrenze note: " + $note + " (eccezioni)"))
}

$codice = 2
$ultima = "ERRORE D'USO: errore inatteso"
try {
    $esito = Esegui
    $codice = [int]$esito[0]
    $ultima = if ($codice -eq 2) { "ERRORE D'USO: " + $esito[1] } else { [string]$esito[1] }
} catch {
    $codice = 2
    $ultima = "ERRORE D'USO: " + $_.Exception.Message + " (riga " + $_.InvocationInfo.ScriptLineNumber + " dello script)"
}
Riga $ultima
exit $codice
