package apiclient

import (
	"encoding/json"
	"net/http"
)

func decodeJSON(r *http.Request, target interface{}) error {
	return json.NewDecoder(r.Body).Decode(target)
}
