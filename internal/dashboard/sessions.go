package dashboard

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
)

const (
	sessionCookie = "kupra_session"
	// chunkSize keeps each cookie under browsers' 4096-byte limit, with room for its name and attributes. No library splits cookies, so this does, like Argo CD and oauth2-proxy.
	chunkSize = 3800
	// maxChunks bounds how many cookies a session may span.
	maxChunks = 10
)

// session is what the dashboard keeps of a signed-in user, sealed in their cookies.
type session struct {
	IDToken      string    `json:"id"`
	RefreshToken string    `json:"refresh,omitempty"`
	Expiry       time.Time `json:"expiry"`
}

// sessions keeps sessions in cookies that the browser holds but can't read or change.
type sessions struct {
	codec  *securecookie.SecureCookie
	secure bool
}

func newSessions(hashKey, blockKey []byte, secure bool) *sessions {
	codec := securecookie.New(hashKey, blockKey).SetSerializer(securecookie.JSONEncoder{}).MaxAge(int(sessionLifetime.Seconds()))
	// The sealed session is split into several cookies, so its own length isn't limited.
	codec.MaxLength(0)
	return &sessions{codec: codec, secure: secure}
}

// set seals s into cookies kupra_session.0, kupra_session.1 and so on, and expires any chunks left over from a longer session.
func (s *sessions) set(w http.ResponseWriter, r *http.Request, v session) error {
	sealed, err := s.codec.Encode(sessionCookie, v)
	if err != nil {
		return err
	}
	var chunks []string
	for len(sealed) > chunkSize {
		chunks = append(chunks, sealed[:chunkSize])
		sealed = sealed[chunkSize:]
	}
	chunks = append(chunks, sealed)
	if len(chunks) > maxChunks {
		return fmt.Errorf("session is %d cookies, more than %d", len(chunks), maxChunks)
	}
	for i, chunk := range chunks {
		http.SetCookie(w, s.cookie(i, chunk, int(sessionLifetime.Seconds())))
	}
	for i := len(chunks); i < maxChunks; i++ {
		if _, err := r.Cookie(chunkName(i)); err == nil {
			http.SetCookie(w, s.cookie(i, "", -1))
		}
	}
	return nil
}

// get opens the request's session. It returns false if there is none or it doesn't open.
func (s *sessions) get(r *http.Request) (session, bool) {
	var b strings.Builder
	for i := range maxChunks {
		c, err := r.Cookie(chunkName(i))
		if err != nil {
			break
		}
		b.WriteString(c.Value)
	}
	var v session
	return v, b.Len() > 0 && s.codec.Decode(sessionCookie, b.String(), &v) == nil
}

// clear expires every chunk of the request's session.
func (s *sessions) clear(w http.ResponseWriter, r *http.Request) {
	for i := range maxChunks {
		if _, err := r.Cookie(chunkName(i)); err == nil {
			http.SetCookie(w, s.cookie(i, "", -1))
		}
	}
}

func chunkName(i int) string {
	return sessionCookie + "." + strconv.Itoa(i)
}

func (s *sessions) cookie(i int, value string, maxAge int) *http.Cookie {
	return &http.Cookie{
		Name:     chunkName(i),
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
	}
}
