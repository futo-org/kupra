// Package names derives names for the objects kupra creates.
package names

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
)

// Hashed returns readable followed by a hash of key, shortened to fit a DNS label. The hash tells apart names whose readable parts are the same, or only differ after the shortened part. At 32 bits, collisions stay unlikely even among tens of thousands of names.
func Hashed(readable, key string) string {
	sum := sha256.Sum256([]byte(key))
	hash := hex.EncodeToString(sum[:])[:8]
	if max := validation.DNS1123LabelMaxLength - len(hash) - 1; len(readable) > max {
		readable = strings.TrimRight(readable[:max], "-.")
	}
	return strings.ReplaceAll(readable, ".", "-") + "-" + hash
}
