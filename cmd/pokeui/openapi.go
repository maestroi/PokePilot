package main

import (
	_ "embed"
	"encoding/json"
	"net/http"
)

//go:embed openapi.json
var operatorOpenAPI []byte

func serveOperatorOpenAPI(res http.ResponseWriter, _ *http.Request) {
	res.Header().Set("Content-Type", "application/json")
	res.Header().Set("Cache-Control", "no-store")
	if !json.Valid(operatorOpenAPI) {
		http.Error(res, "openapi spec is not valid JSON", http.StatusInternalServerError)
		return
	}
	_, _ = res.Write(operatorOpenAPI)
}
