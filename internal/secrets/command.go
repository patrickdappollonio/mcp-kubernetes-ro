package secrets

import (
	"fmt"
	"strings"
)

// heredocDelimiter ends the block list in DecryptCommand. Base64 blocks are
// hundreds of characters long, so no block can ever equal it.
const heredocDelimiter = "K8S_SECRET_BLOCKS"

// DecryptCommand returns a POSIX shell command that decrypts blocks produced
// by Encrypt using the openssl CLI and writes the value to outputPath. The
// command creates the file with owner-only permissions, refuses to overwrite
// an existing file, removes a partially written file if any block fails to
// decrypt, and deletes the private key once decryption succeeds.
func DecryptCommand(blocks []string, privateKeyPath, outputPath string) string {
	key := shellQuote(privateKeyPath)
	out := shellQuote(outputPath)

	var b strings.Builder
	fmt.Fprintf(&b, "(\n")
	fmt.Fprintf(&b, "  set -C\n")
	fmt.Fprintf(&b, "  umask 077\n")
	fmt.Fprintf(&b, "  while IFS= read -r block; do\n")
	fmt.Fprintf(&b, "    printf '%%s' \"$block\" | openssl base64 -d -A |\n")
	fmt.Fprintf(&b, "      openssl pkeyutl -decrypt -inkey %s -pkeyopt rsa_padding_mode:oaep ||\n", key)
	fmt.Fprintf(&b, "      { rm -f %s; exit 1; }\n", out)
	fmt.Fprintf(&b, "  done > %s\n", out)
	fmt.Fprintf(&b, ") <<'%s' && rm -f %s\n", heredocDelimiter, key)
	for _, block := range blocks {
		b.WriteString(block)
		b.WriteByte('\n')
	}
	b.WriteString(heredocDelimiter)
	b.WriteByte('\n')

	return b.String()
}

// shellQuote wraps s in single quotes so a POSIX shell treats it as one
// literal word.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
