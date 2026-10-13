//go:build !noagent

package server

import (
	"net/http"

	"github.com/ipfs/kubo/mcp/ui/catalog"
)

// aguiCatalogCORS is the strict CORS answer of the catalog route: a browser
// fetches it with GET, so the policy names GET where /agui's names POST.
var aguiCatalogCORS = CORSPolicy{
	Methods: []string{http.MethodGet, http.MethodOptions},
	Headers: []string{"authorization", "content-type"},
}

// aguiCatalogHandler serves the A2UI catalog discovery document
// (mcp-agui-a2ui-registry 1.1, decision D15). The bytes are the generated
// mcp/ui/catalog/ipfs-mcp.catalog.json, embedded by the catalog package and
// served unchanged: no copy of the catalog exists anywhere else in the tree.
// The route's protection (Host, Origin, bearer, principal kind ui, body
// limit) already ran in the mount chain, the same one /agui passes through.
type aguiCatalogHandler struct{}

// ServeHTTP implements http.Handler.
func (aguiCatalogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != RouteAGUICatalog {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeAGUIError(w, http.StatusMethodNotAllowed, CodeInvalidArgument, "GET the catalog", false)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(catalog.Bytes())
}
