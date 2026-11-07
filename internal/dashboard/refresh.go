package dashboard

import (
	"crypto/sha256"
	"sync"
	"time"
)

// refreshResultLifetime is how long a refresh's result is kept for requests that still carry the old refresh token, such as those the browser sent before it got the new cookies.
const refreshResultLifetime = time.Minute

// refreshes runs one refresh per refresh token, and hands its result to every request that carries that token.
type refreshes struct {
	mu    sync.Mutex
	calls map[[sha256.Size]byte]*refreshCall
}

type refreshCall struct {
	done    chan struct{}
	session *session
	err     error
	ended   time.Time
}

func (r *refreshes) do(refreshToken string, refresh func() (*session, error)) (*session, error) {
	key := sha256.Sum256([]byte(refreshToken))
	r.mu.Lock()
	if r.calls == nil {
		r.calls = map[[sha256.Size]byte]*refreshCall{}
	}
	for k, c := range r.calls {
		if !c.ended.IsZero() && time.Since(c.ended) > refreshResultLifetime {
			delete(r.calls, k)
		}
	}
	if c, ok := r.calls[key]; ok {
		r.mu.Unlock()
		<-c.done
		return c.session, c.err
	}
	c := &refreshCall{done: make(chan struct{})}
	r.calls[key] = c
	r.mu.Unlock()

	c.session, c.err = refresh()
	r.mu.Lock()
	c.ended = time.Now()
	if c.err != nil {
		// A failed refresh isn't kept, so the next request tries again.
		delete(r.calls, key)
	}
	r.mu.Unlock()
	close(c.done)
	return c.session, c.err
}
