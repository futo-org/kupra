package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	kuprav1alpha1 "github.com/futo-org/kupra/api/v1alpha1"
)

// appNamespaces is the collection of namespaces that contain PreviewApps. The dashboard watches it with its own account; everything else, as the user.
const appNamespaces = "app-namespaces"

// maxCollections bounds how many collections one stream may watch.
const maxCollections = 50

// watchable are the resources the page may watch, by plural.
var watchable = map[string]bool{"previewapps": true, "previews": true, "previewsnapshots": true}

// collection is something a stream watches: a resource, in one namespace or all the user may list.
type collection struct {
	name      string
	resource  string
	namespace string
}

func parseCollection(s string) (collection, error) {
	if s == appNamespaces {
		return collection{name: s}, nil
	}
	resource, namespace, _ := strings.Cut(s, "/")
	if !watchable[resource] || strings.Contains(namespace, "/") {
		return collection{}, fmt.Errorf("can't watch %q", s)
	}
	return collection{name: s, resource: resource, namespace: namespace}, nil
}

// event is what the stream sends the page about a collection: a change to one of its objects, that it has been listed, or that it can't be watched.
type event struct {
	Collection string `json:"collection"`
	// Type is ADDED, MODIFIED or DELETED for a change, SYNCED once the collection has been listed, and ERROR when it can't be watched.
	Type   string         `json:"type"`
	Object runtime.Object `json:"object,omitempty"`
	Error  *metav1.Status `json:"error,omitempty"`
}

// watchHandler streams the collections the page asks for, as server-sent events. It watches them with client-go's informers, as the user, so the page holds one connection and never polls. The stream ends when the user's ID token expires, and the browser's EventSource reconnects with a refreshed one.
type watchHandler struct {
	auth      *authenticator
	anonymous *rest.Config
	own       dynamic.Interface
}

func (h *watchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	names := r.URL.Query()["c"]
	if len(names) == 0 || len(names) > maxCollections {
		http.Error(w, fmt.Sprintf("watch 1 to %d collections, with c", maxCollections), http.StatusBadRequest)
		return
	}
	collections := make([]collection, 0, len(names))
	for _, name := range names {
		c, err := parseCollection(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		collections = append(collections, c)
	}
	session, err := h.auth.session(w, r)
	if err != nil {
		authError(w, r, err)
		return
	}
	cfg := rest.CopyConfig(h.anonymous)
	cfg.BearerToken = session.IDToken
	user, err := dynamic.NewForConfig(cfg)
	if err != nil {
		httpError(w, r, http.StatusInternalServerError, err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	// The stream ends when the token expires, and the browser reconnects with a refreshed one.
	if until := time.Until(session.Expiry); until > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, until)
		defer stop()
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	events := make(chan event, 64)
	for _, c := range collections {
		var client dynamic.Interface = user
		if c.name == appNamespaces {
			client = h.own
		}
		go watchCollection(ctx, cancel, client, c, events)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-events:
			data, err := json.Marshal(e)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}
}

// watchCollection runs an informer on c until ctx ends, sending its changes as events. A collection the user may not watch is reported once and dropped. An expired token ends the whole stream, so the browser reconnects with a fresh one.
func watchCollection(ctx context.Context, endStream context.CancelFunc, client dynamic.Interface, c collection, events chan<- event) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	send := func(e event) {
		e.Collection = c.name
		select {
		case events <- e:
		case <-ctx.Done():
		}
	}
	var resource dynamic.ResourceInterface
	opts := func(o *metav1.ListOptions) {}
	if c.name == appNamespaces {
		resource = client.Resource(schema.GroupVersionResource{Version: "v1", Resource: "namespaces"})
		opts = func(o *metav1.ListOptions) { o.LabelSelector = kuprav1alpha1.LabelAppNamespace }
	} else {
		gvr := kuprav1alpha1.GroupVersion.WithResource(c.resource)
		if c.namespace != "" {
			resource = client.Resource(gvr).Namespace(c.namespace)
		} else {
			resource = client.Resource(gvr)
		}
	}
	// failed reports errors that retrying won't fix, and stops the informer.
	failed := func(err error) error {
		switch {
		case apierrors.IsUnauthorized(err):
			endStream()
		case apierrors.IsForbidden(err) || apierrors.IsNotFound(err) || apierrors.IsBadRequest(err):
			var status apierrors.APIStatus
			if errors.As(err, &status) {
				s := status.Status()
				send(event{Type: "ERROR", Error: &s})
			}
			cancel()
		}
		return err
	}
	lw := &cache.ListWatch{
		ListWithContextFunc: func(ctx context.Context, o metav1.ListOptions) (runtime.Object, error) {
			opts(&o)
			list, err := resource.List(ctx, o)
			if err != nil {
				return nil, failed(err)
			}
			return list, nil
		},
		WatchFuncWithContext: func(ctx context.Context, o metav1.ListOptions) (watch.Interface, error) {
			opts(&o)
			w, err := resource.Watch(ctx, o)
			if err != nil {
				return nil, failed(err)
			}
			return w, nil
		},
	}
	_, informer := cache.NewInformerWithOptions(cache.InformerOptions{
		ListerWatcher: lw,
		ObjectType:    &unstructured.Unstructured{},
		// Namespaces carry only their names: the page doesn't need the rest, and the dashboard's account reads them.
		Transform: func(obj any) (any, error) {
			if u, ok := obj.(*unstructured.Unstructured); ok && c.name == appNamespaces {
				return &unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": u.GetName()}}}, nil
			}
			return obj, nil
		},
		Handler: cache.ResourceEventHandlerFuncs{
			AddFunc:    func(obj any) { send(event{Type: string(watch.Added), Object: obj.(runtime.Object)}) },
			UpdateFunc: func(_, obj any) { send(event{Type: string(watch.Modified), Object: obj.(runtime.Object)}) },
			DeleteFunc: func(obj any) {
				if tombstone, ok := obj.(cache.DeletedFinalStateUnknown); ok {
					obj = tombstone.Obj
				}
				if o, ok := obj.(runtime.Object); ok {
					send(event{Type: string(watch.Deleted), Object: o})
				}
			},
		},
	})
	go informer.RunWithContext(ctx)
	if cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		send(event{Type: "SYNCED"})
	}
	<-ctx.Done()
}
