# Le funzioni dei due avviatori di rete, avvia-lan.sh (IPv4) e avvia-https.sh (IPv6). Non si lancia
# da solo: lo caricano loro con `source`, e chiamano avvia_in_rete con la famiglia e il nome della
# modalita'.
#
# Che cosa fanno, in ordine:
#   1. leggono le opzioni (-a, -p, -c, --config, --no-build, --mostra);
#   2. scelgono l'indirizzo di QUESTO PC su cui ascoltare, o controllano quello dato con -a;
#   3. rifiutano di partire se un cockpit gira gia': due server sullo stesso database si
#      contenderebbero la coda, e non si ferma a sorpresa quello di qualcun altro;
#   4. compilano cockpit (salvo --no-build);
#   5. avviano il server in primo piano con la rete sulla riga di comando: -ascolto, il certificato
#      della modalita', -url-pubblico, -reti. cockpit.toml non si tocca, e l'avvio di sempre
#      (scripts\avvia-dev.ps1, 127.0.0.1:8080) resta com'e'.
#
# Tutte e due le modalita' parlano HTTPS: il server fuori da questo PC non parte in chiaro, e dalla
# riga di comando non si eredita il consenti_lan_in_chiaro del file (config.Rete).
#
# Gira in Git Bash su Windows (il PC di sviluppo) e in bash su Linux (la VM, quando ci sara').

set -euo pipefail

RADICE="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

case "$(uname -s)" in
MINGW* | MSYS* | CYGWIN*) SISTEMA=windows ESEGUIBILE=cockpit.exe ;;
*) SISTEMA=linux ESEGUIBILE=cockpit ;;
esac

errore() {
	printf 'errore: %s\n' "$*" >&2
	exit 1
}

# chiedi_a_windows esegue un comando PowerShell e ne restituisce l'uscita senza i \r. La conversione
# dei percorsi di MSYS si spegne: una rete come 0.0.0.0/0 non e' un percorso.
chiedi_a_windows() {
	MSYS_NO_PATHCONV=1 powershell.exe -NoProfile -NonInteractive -Command "$1" 2>/dev/null | tr -d '\r'
}

# candidati stampa gli indirizzi di questo PC fra cui scegliere, «indirizzo/prefisso», il migliore
# per primo.
#
#   IPv4: quelli della scheda che porta il gateway predefinito, cioe' la LAN, e non le schede
#         virtuali (Hyper-V, WSL, VPN) che hanno un 172.x qualsiasi.
#   IPv6: niente link-local (fe80::, che i browser non sanno aprire), niente Teredo e 6to4; prima gli
#         indirizzi statici o da DHCPv6, poi gli ULA (fd00::/8), poi i globali da annuncio del router,
#         che possono cambiare.
candidati() {
	if [ "$SISTEMA" = windows ]; then
		if [ "$1" = 4 ]; then
			chiedi_a_windows '
				Get-NetRoute -DestinationPrefix "0.0.0.0/0" -ErrorAction SilentlyContinue | Sort-Object RouteMetric | ForEach-Object {
					Get-NetIPAddress -InterfaceIndex $_.InterfaceIndex -AddressFamily IPv4 -AddressState Preferred -ErrorAction SilentlyContinue
				} | ForEach-Object { "{0}/{1}" -f $_.IPAddress, $_.PrefixLength }'
		else
			chiedi_a_windows '
				Get-NetIPAddress -AddressFamily IPv6 -AddressState Preferred -ErrorAction SilentlyContinue |
					Where-Object { $_.IPAddress -notmatch "^(fe[89ab]|::1$|2001:0?:|2002:)" -and $_.IPAddress -notmatch "%" } |
					Sort-Object @{ e = { if ($_.PrefixOrigin -in "Manual", "Dhcp") { 0 } elseif ($_.IPAddress -match "^f[cd]") { 1 } else { 2 } } } |
					ForEach-Object { "{0}/{1}" -f $_.IPAddress, $_.PrefixLength }'
		fi
	else
		if [ "$1" = 4 ]; then
			local scheda
			scheda=$(ip -o -4 route show default 2>/dev/null | awk '{ for (i = 1; i < NF; i++) if ($i == "dev") { print $(i + 1); exit } }')
			[ -n "$scheda" ] && ip -o -4 addr show dev "$scheda" scope global | awk '{ print $4 }'
		else
			ip -o -6 addr show scope global 2>/dev/null | grep -v -e temporary -e deprecated -e tentative |
				awk '{ p = ($0 ~ / dynamic/) ? 2 : 0; if (p == 2 && $4 ~ /^f[cd]/) p = 1; print p, $4 }' |
				sort -n -s -k1,1 | awk '{ print $2 }'
		fi
	fi
}

# tutti stampa tutti gli indirizzi di questo PC della famiglia, «indirizzo/prefisso»: serve a trovare
# il prefisso di un indirizzo dato con -a.
tutti() {
	if [ "$SISTEMA" = windows ]; then
		chiedi_a_windows "Get-NetIPAddress -AddressFamily IPv$1 -ErrorAction SilentlyContinue | ForEach-Object { '{0}/{1}' -f \$_.IPAddress, \$_.PrefixLength }"
	else
		ip -o "-$1" addr show 2>/dev/null | awk '{ print $4 }'
	fi
}

# controlla_famiglia FAMIGLIA INDIRIZZO ferma l'avvio se l'indirizzo non e' della modalita', o e' un
# IPv6 che dagli altri PC non si puo' scrivere in un URL.
controlla_famiglia() {
	local ip=$2
	case "$1" in
	4) [[ "$ip" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]] || errore "$ip non e' un IPv4: per IPv6 c'e' avvia-https.sh" ;;
	6)
		[[ "$ip" == *:* ]] || errore "$ip non e' un IPv6: per IPv4 c'e' avvia-lan.sh"
		if [[ "$ip" == *%* || "${ip,,}" =~ ^fe[89ab] ]]; then
			errore "$ip e' un link-local: i browser non aprono un indirizzo con la zona (%), e dagli altri PC
non c'e' un URL da scrivere. Serve un ULA (fd00::/8) o un globale"
		fi
		if [[ "${ip,,}" == ::ffff:* ]]; then
			errore "$ip e' un IPv4 in forma IPv6: per IPv4 c'e' avvia-lan.sh"
		fi
		;;
	esac
}

# da_copiare stampa il comando in una forma che si incolla cosi' com'e' in bash: fra apici solo gli
# argomenti che ne hanno bisogno (percorsi Windows, le parentesi di un IPv6).
da_copiare() {
	local a uscita=""
	for a in "$@"; do
		if [[ "$a" =~ ^[A-Za-z0-9_./:,=+-]+$ ]]; then
			uscita+="$a "
		else
			uscita+="'$a' "
		fi
	done
	echo "${uscita% }"
}

gia_in_esecuzione() {
	local uscita
	if [ "$SISTEMA" = windows ]; then
		uscita=$(tasklist.exe //FI "IMAGENAME eq cockpit.exe" //NH 2>/dev/null || true)
		grep -qi '^cockpit\.exe' <<<"$uscita"
	else
		pgrep -x cockpit >/dev/null 2>&1
	fi
}

aiuto() {
	cat <<FINE
Uso: bash scripts/avvio-rete/$1 [opzioni]

  -a INDIRIZZO[/PREFISSO]  l'indirizzo di QUESTO PC su cui ascoltare (IPv$2). Assente = scelto
                           da solo. Senza /PREFISSO il prefisso si legge dalla scheda.
  -p PORTA                 porta di ascolto (8443)
  -c RETE[,RETE...]        da dove si accettano connessioni, al posto della rete della scheda:
                           reti o indirizzi singoli (10.0.0.15,10.0.0.16). Questo PC entra sempre.
  --config FILE            file di configurazione (cockpit.toml accanto a cmd/)
  --no-build               non ricompilare
  --mostra                 stampa il comando del server e non avvia niente
FINE
}

# avvia_in_rete FAMIGLIA MODALITA [opzioni...]: FAMIGLIA e' 4 o 6, MODALITA e' il nome della cartella
# del certificato (tls/<modalita>/) e del messaggio.
avvia_in_rete() {
	local famiglia=$1 modo=$2 script=$3
	shift 3
	local indirizzo="" porta=8443 consentite="" config="$RADICE/cockpit.toml" compila=1 mostra=0
	while [ $# -gt 0 ]; do
		case "$1" in
		-a | -p | -c | --config)
			[ $# -ge 2 ] || errore "$1 vuole un valore (--help per l'elenco)"
			case "$1" in
			-a) indirizzo=$2 ;;
			-p) porta=$2 ;;
			-c) consentite=$2 ;;
			--config) config=$2 ;;
			esac
			shift 2
			;;
		--no-build) compila=0 && shift ;;
		--mostra) mostra=1 && shift ;;
		-h | --help) aiuto "$script" "$famiglia" && exit 0 ;;
		*) errore "opzione sconosciuta: $1 (--help per l'elenco)" ;;
		esac
	done
	[[ "$porta" =~ ^[0-9]+$ ]] && [ "$porta" -ge 1 ] && [ "$porta" -le 65535 ] || errore "porta non valida: $porta"
	[ -f "$config" ] || errore "file di configurazione non trovato: $config"
	config="$(cd "$(dirname "$config")" && pwd)/$(basename "$config")"
	[ "$SISTEMA" = windows ] && config=$(cygpath -w "$config")

	# ---- l'indirizzo: «indirizzo/prefisso». La forma si guarda prima di cercarlo fra quelli del PC:
	# a chi scrive un link-local serve sapere perche' non va, non che non l'abbiamo trovato.
	[ -n "$indirizzo" ] && controlla_famiglia "$famiglia" "${indirizzo%/*}"
	local scheda
	if [ -z "$indirizzo" ]; then
		scheda=$(candidati "$famiglia" | head -n 1)
		if [ -z "$scheda" ]; then
			if [ "$famiglia" = 6 ]; then
				errore "questo PC non ha un IPv6 che gli altri PC possano aprire: c'e' al massimo il link-local (fe80::),
che i browser non sanno aprire. Serve l'IPv6 statico della VM (sulla VM: -a <indirizzo>) o un
ULA/globale assegnato dall'IT a questo PC. Per provare la modalita' su questo PC soltanto: -a ::1"
			fi
			errore "nessun IPv4 sulla scheda con il gateway predefinito: indicarlo con -a"
		fi
	elif [[ "$indirizzo" == */* ]]; then
		scheda=$indirizzo
	else
		local riga
		riga=$(tutti "$famiglia" | awk -v a="${indirizzo,,}" 'BEGIN { FS = "/" } tolower($1) == a { print; exit }')
		[ -n "$riga" ] || errore "$indirizzo non e' un indirizzo IPv$famiglia di questo PC. Un server ascolta solo sui propri
indirizzi: quello della VM si usa sulla VM. Indirizzi di questo PC:
$(tutti "$famiglia" | sed 's/^/  /')
Se l'indirizzo e' scritto in un'altra forma, darlo con il prefisso: -a INDIRIZZO/$([ "$famiglia" = 4 ] && echo 24 || echo 64)"
		scheda=$riga
	fi
	local ip=${scheda%/*} prefisso=${scheda#*/}
	[[ "$prefisso" =~ ^[0-9]+$ ]] || errore "prefisso non valido in $scheda"
	controlla_famiglia "$famiglia" "$ip"
	local locale=0
	[[ "$ip" == 127.* || "$ip" == ::1 ]] && locale=1

	local reti=${consentite:-$scheda} ascolto url
	if [ "$famiglia" = 6 ]; then
		ascolto="[$ip]:$porta" url="https://[$ip]:$porta"
	else
		ascolto="$ip:$porta" url="https://$ip:$porta"
	fi
	local comando=("./$ESEGUIBILE" -config "$config" -ascolto "$ascolto"
		-tls-cert "tls/$modo/cert.pem" -tls-key "tls/$modo/key.pem" -url-pubblico "$url" -reti "$reti")

	cd "$RADICE"
	echo "== Cockpit in rete, modalita' $modo (IPv$famiglia, HTTPS)"
	echo "   indirizzo      $url"
	echo "   reti ammesse   $reti, piu' questo PC"
	echo "   certificato    tls/$modo/ accanto al file di configurazione (se non c'e', lo genera il server)"
	echo "   configurazione $config (solo le voci di rete vengono dalla riga di comando)"
	if [ "$locale" = 1 ]; then
		echo "   ATTENZIONE: $ip e' questo PC soltanto. Serve a provare la modalita', non a collegare altri PC." >&2
	fi
	if [ -n "$indirizzo" ] || [ "$famiglia" = 4 ]; then
		:
	else
		echo "   scelto da solo fra: $(candidati "$famiglia" | tr '\n' ' ')"
	fi
	echo "   comando        $(da_copiare "${comando[@]}")"
	if [ "$mostra" = 1 ]; then
		return 0
	fi

	if gia_in_esecuzione; then
		errore "un cockpit e' gia' in esecuzione su questo PC: fermarlo prima (Ctrl+C nella sua finestra, o
powershell -ExecutionPolicy Bypass -File scripts\\ferma-dev.ps1). Due server sullo stesso database si
contendono la coda dei job"
	fi
	if [ "$compila" = 1 ]; then
		command -v go >/dev/null 2>&1 || errore "go non trovato: compilare altrove e rilanciare con --no-build"
		echo "== go build"
		go build -o "$ESEGUIBILE" ./cmd/cockpit
	fi
	[ -f "$ESEGUIBILE" ] || errore "$RADICE/$ESEGUIBILE non c'e': togliere --no-build"

	echo
	echo "Da un altro PC: aprire $url. Il certificato e' autofirmato: il browser avvisa finche' il PC non"
	echo "lo ha fra le autorita' radice (lo installa il pacchetto della postazione)."
	echo "I worker vogliono il pacchetto di QUESTO avvio (pagina Postazioni): indirizzo e impronta del"
	echo "certificato cambiano con la modalita'."
	if [ "$SISTEMA" = windows ] && [ "$locale" = 0 ]; then
		echo "Windows Firewall blocca le connessioni in ingresso finche' non c'e' una regola per la porta $porta"
		echo "(README, «Avvio in rete»)."
	fi
	echo "Ctrl+C per fermare."
	echo
	# In primo piano, al posto di questa shell: Ctrl+C arriva al server, che chiude pulito.
	MSYS_NO_PATHCONV=1 MSYS2_ARG_CONV_EXCL='*' exec "${comando[@]}"
}
