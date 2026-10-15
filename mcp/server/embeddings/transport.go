package embeddings

import "net/http"

// directTransport is the default transport of the embedder, extractor,
// describer and vector store clients: it dials the configured host itself and ignores the
// HTTP_PROXY, HTTPS_PROXY and NO_PROXY environment variables. Document text,
// media and API keys therefore travel only to the host the operator named and
// the principal acknowledged, never through a proxy that an unrelated process
// environment names.
func directTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil
	return t
}

// DirectTransport is directTransport for the clients of ipfs mcp outside this
// package (the LLM client and the settings probe), so every client that carries
// operator-named credentials or content dials the configured host itself.
func DirectTransport() *http.Transport { return directTransport() }
