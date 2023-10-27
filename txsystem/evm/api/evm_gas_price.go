package api

import (
	"net/http"

	"github.com/alphabill-org/alphabill/common/util"
)

// GasPrice - returns static gas unit price. When gas price becomes dynamic
// a new approach is needed
func (a *API) GasPrice(w http.ResponseWriter, _ *http.Request) {
	util.WriteCBORResponse(w, &struct {
		_        struct{} `cbor:",toarray"`
		GasPrice string
	}{
		GasPrice: a.gasUnitPrice.String(),
	}, http.StatusOK, a.log)
}
