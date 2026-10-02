package secrets

import (
	"fmt"
	"strings"
)

// heredocDelimiter ends the block list in DecryptCommand. Base64 blocks are
// hundreds of characters long, so no block can ever equal it.
const heredocDelimiter = "K8S_SECRET_BLOCKS"

// DecryptCommand returns a POSIX shell command that decrypts Encrypt's blocks with openssl
// into a new owner-only file at outputPath, then deletes the private key. The command
// never overwrites an existing file and removes its output if any block fails.
func DecryptCommand(blocks []string, privateKeyPath, outputPath string) string {
	key := shellQuote(privateKeyPath)
	out := shellQuote(outputPath)

	var b strings.Builder
	fmt.Fprintf(&b, `(
  set -C
  umask 077
  while IFS= read -r block; do
    printf '%%s' "$block" | openssl base64 -d -A |
      openssl pkeyutl -decrypt -inkey %[1]s -pkeyopt rsa_padding_mode:oaep ||
      { rm -f %[2]s; exit 1; }
  done > %[2]s
) <<'%[3]s' && rm -f %[1]s
`, key, out, heredocDelimiter)
	for _, block := range blocks {
		b.WriteString(block + "\n")
	}
	b.WriteString(heredocDelimiter + "\n")

	return b.String()
}

// shellQuote wraps s in single quotes so a POSIX shell treats it as one
// literal word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
