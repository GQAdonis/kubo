package embeddings

import "net/http"

// directTransport is the default transport of the embedder, extractor and
// describer clients: it dials the configured host itself and ignores the
// HTTP_PROXY, HTTPS_PROXY and NO_PROXY environment variables. Document text,
// media and API keys therefore travel only to the host the operator named and
// the principal acknowledged, never through a proxy that an unrelated process
// environment names.
func directTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return t
}
