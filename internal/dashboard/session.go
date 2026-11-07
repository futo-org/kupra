package dashboard

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// chunkSize keeps each cookie under browsers' 4096-byte limit, with room for its name and attributes.
const chunkSize = 3800

// maxChunks bounds how many cookies a sealed value may span.
const maxChunks = 10

// session is what the dashboard keeps of a signed-in user, sealed in their cookies.
type session struct {
	IDToken      string    `json:"id"`
	RefreshToken string    `json:"refresh,omitempty"`
	Expiry       time.Time `json:"expiry"`
}

// login is what the dashboard keeps between sending a user to the identity provider and their return.
type login struct {
	State    string `json:"state"`
	Nonce    string `json:"nonce"`
	Verifier string `json:"verifier"`
	Redirect string `json:"redirect"`
}

// sealer encrypts values into cookies, so the browser holds them without being able to read or change them.
type sealer struct {
	aead   cipher.AEAD
	secure bool
}

func newSealer(secret string, secure bool) (*sealer, error) {
	if len(secret) < 32 {
		return nil, errors.New("cookie secret must be at least 32 characters")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &sealer{aead: aead, secure: secure}, nil
}

// set seals v into cookies named name.0, name.1 and so on, and expires any chunks left over from a longer value.
func (s *sealer) set(w http.ResponseWriter, r *http.Request, name, path string, maxAge time.Duration, v any) error {
	plain, err := json.Marshal(v)
	if err != nil {
		return err
	}
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	// The cookie's name is authenticated too, so a sealed value can't be moved to another cookie.
	sealed := base64.RawURLEncoding.EncodeToString(s.aead.Seal(nonce, nonce, plain, []byte(name)))
	var chunks []string
	for len(sealed) > chunkSize {
		chunks = append(chunks, sealed[:chunkSize])
		sealed = sealed[chunkSize:]
	}
	chunks = append(chunks, sealed)
	if len(chunks) > maxChunks {
		return fmt.Errorf("sealed %s is %d cookies, more than %d", name, len(chunks), maxChunks)
	}
	for i, chunk := range chunks {
		http.SetCookie(w, s.cookie(name+"."+strconv.Itoa(i), path, chunk, int(maxAge.Seconds())))
	}
	for i := len(chunks); i < maxChunks; i++ {
		if _, err := r.Cookie(name + "." + strconv.Itoa(i)); err == nil {
			http.SetCookie(w, s.cookie(name+"."+strconv.Itoa(i), path, "", -1))
		}
	}
	return nil
}

// get opens the value sealed in cookies named name. It returns false if there is none or it doesn't open.
func (s *sealer) get(r *http.Request, name string, v any) bool {
	var b strings.Builder
	for i := range maxChunks {
		c, err := r.Cookie(name + "." + strconv.Itoa(i))
		if err != nil {
			break
		}
		b.WriteString(c.Value)
	}
	sealed, err := base64.RawURLEncoding.DecodeString(b.String())
	if err != nil || len(sealed) < s.aead.NonceSize() {
		return false
	}
	nonce, ciphertext := sealed[:s.aead.NonceSize()], sealed[s.aead.NonceSize():]
	plain, err := s.aead.Open(nil, nonce, ciphertext, []byte(name))
	if err != nil {
		return false
	}
	return json.Unmarshal(plain, v) == nil
}

// clear expires every chunk of name the request has.
func (s *sealer) clear(w http.ResponseWriter, r *http.Request, name, path string) {
	for i := range maxChunks {
		if _, err := r.Cookie(name + "." + strconv.Itoa(i)); err == nil {
			http.SetCookie(w, s.cookie(name+"."+strconv.Itoa(i), path, "", -1))
		}
	}
}

func (s *sealer) cookie(name, path, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	}
}
