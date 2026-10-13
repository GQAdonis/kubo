package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core/corehttp"
	"github.com/ipfs/kubo/test/cli/harness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebUI(t *testing.T) {
	t.Parallel()

	t.Run("NoFetch=true shows not available error", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init()

		node.UpdateConfig(func(cfg *config.Config) {
			cfg.Gateway.NoFetch = true
		})

		node.StartDaemon("--offline")

		apiClient := node.APIClient()
		resp := apiClient.Get("/webui/")

		// Should return 503 Service Unavailable when WebUI is not in local store
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		// Check response contains helpful information
		body := resp.Body
		assert.Contains(t, body, "IPFS WebUI Not Available")
		assert.Contains(t, body, "Gateway.NoFetch=true")
		assert.Contains(t, body, "ipfs pin add")
		assert.Contains(t, body, "ipfs dag import")
		assert.Contains(t, body, "https://github.com/ipfs/ipfs-webui/releases")
	})

	t.Run("DeserializedResponses=false shows incompatible error", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init()

		node.UpdateConfig(func(cfg *config.Config) {
			cfg.Gateway.DeserializedResponses = config.False
		})

		node.StartDaemon()

		apiClient := node.APIClient()
		resp := apiClient.Get("/webui/")

		// Should return 503 Service Unavailable
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		// Check response contains incompatibility message
		body := resp.Body
		assert.Contains(t, body, "IPFS WebUI Incompatible")
		assert.Contains(t, body, "Gateway.DeserializedResponses=false")
		assert.Contains(t, body, "WebUI requires deserializing IPFS responses")
		assert.Contains(t, body, "Gateway.DeserializedResponses=true")
	})

	t.Run("Both NoFetch=true and DeserializedResponses=false shows incompatible error", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init()

		node.UpdateConfig(func(cfg *config.Config) {
			cfg.Gateway.NoFetch = true
			cfg.Gateway.DeserializedResponses = config.False
		})

		node.StartDaemon("--offline")

		apiClient := node.APIClient()
		resp := apiClient.Get("/webui/")

		// Should return 503 Service Unavailable
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		// DeserializedResponses=false takes priority
		body := resp.Body
		assert.Contains(t, body, "IPFS WebUI Incompatible")
		assert.Contains(t, body, "Gateway.DeserializedResponses=false")
		// Should NOT mention NoFetch since DeserializedResponses check comes first
		assert.NotContains(t, body, "NoFetch")
	})

	t.Run("redirects /webui/ to the current WebUIPath", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init().StartDaemon("--offline")

		resp := node.APIClient().DisableRedirects().Get("/webui/")

		assert.Equal(t, http.StatusFound, resp.StatusCode)
		assert.Equal(t, corehttp.WebUIPath, resp.Headers.Get("Location"))
	})

	t.Run("earlier builds stay routed on the RPC listener", func(t *testing.T) {
		t.Parallel()
		node := harness.NewT(t).NewNode().Init()
		node.UpdateConfig(func(cfg *config.Config) {
			cfg.Gateway.NoFetch = true
		})
		node.StartDaemon("--offline")
		client := node.APIClient()

		// The upstream v4.13.0 build, replaced as WebUIPath by the fork build.
		// It is not in this node's blockstore, so the gateway answers 404 with
		// its own error page. A route that was dropped would instead get the
		// mux's plain "404 page not found".
		const upstream = "/ipfs/bafybeiciqeyipumpmhxzlxnbqdbbv6u5uij4hy4wax64dmj7kvrhusiq6y"
		assert.Contains(t, corehttp.WebUIPaths, upstream)
		const muxNotFound = "404 page not found"
		listed := client.Get(upstream + "/")
		assert.NotContains(t, listed.Body, muxNotFound)

		// A CID outside WebUIPaths is not served on the RPC listener.
		const unlisted = "/ipfs/bafybeigdyrzt5sfp7udm7hu76uh7y26nf3efuylqabf3oclgtqy55fbzdi/"
		other := client.Get(unlisted)
		assert.Equal(t, http.StatusNotFound, other.StatusCode)
		assert.Contains(t, other.Body, muxNotFound)
	})

	t.Run("NoFetch=true serves the build once it is local", func(t *testing.T) {
		t.Parallel()
		// The real build is about 30 MB, too large for the repo, so the test
		// takes it from the directory named by WEBUI_BUILD_DIR (the output of
		// `npm run build` in GQAdonis/ipfs-webui at the commit named in
		// core/corehttp/webui.go) and skips without it. Its CID is checked
		// against WebUIPath, so a stale build fails instead of passing.
		dir := os.Getenv("WEBUI_BUILD_DIR")
		if dir == "" {
			t.Skip("set WEBUI_BUILD_DIR to the ipfs-webui fork build output to run this test")
		}
		if _, err := os.Stat(filepath.Join(dir, "index.html")); err != nil {
			t.Skipf("WEBUI_BUILD_DIR has no index.html: %v", err)
		}

		node := harness.NewT(t).NewNode().Init()
		node.UpdateConfig(func(cfg *config.Config) {
			cfg.Gateway.NoFetch = true
		})
		node.StartDaemon("--offline")
		client := node.APIClient().DisableRedirects()

		// Without the build, the 503 path applies (pinned scenario below).
		resp := client.Get("/webui/")
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		assert.Contains(t, resp.Body, "IPFS WebUI Not Available")
		assert.Contains(t, resp.Body, corehttp.WebUIPath)

		// Same recipe as upstream's release workflow.
		root := node.IPFS("add", "--cid-version", "1", "-Q", "-r", dir).Stdout.Trimmed()
		require.Equal(t, corehttp.WebUIPath, "/ipfs/"+root, "WEBUI_BUILD_DIR is not the build WebUIPath names")

		resp = client.Get("/webui/")
		assert.Equal(t, http.StatusFound, resp.StatusCode)
		assert.Equal(t, corehttp.WebUIPath, resp.Headers.Get("Location"))

		resp = client.Get(corehttp.WebUIPath + "/")
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Contains(t, resp.Body, "<html")
	})
}
