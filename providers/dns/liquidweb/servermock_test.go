package liquidweb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/liquidweb/liquidweb-go/network"
	"github.com/liquidweb/liquidweb-go/types"
)

func mockAPIServer(t *testing.T, initRecs ...network.DNSRecord) string {
	t.Helper()

	recs := make(map[int]network.DNSRecord)

	for _, rec := range initRecs {
		recs[int(rec.ID)] = rec
	}

	mux := http.NewServeMux()
	mux.Handle("/v1/Network/DNS/Record/delete", mockAPIDelete(recs))
	mux.Handle("/v1/Network/DNS/Record/create", mockAPICreate(recs))
	mux.Handle("/v1/Network/DNS/Zone/list", mockAPIListZones())
	mux.Handle("/bleed/Network/DNS/Record/delete", mockAPIDelete(recs))
	mux.Handle("/bleed/Network/DNS/Record/create", mockAPICreate(recs))
	mux.Handle("/bleed/Network/DNS/Zone/list", mockAPIListZones())

	server := httptest.NewServer(requireBasicAuth(requireJSON(mux)))
	t.Cleanup(server.Close)

	return server.URL
}

func requireBasicAuth(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if ok && username == "blars" && password == "tacoman" {
			next.ServeHTTP(w, r)
			return
		}

		http.Error(w, "invalid auth", http.StatusForbidden)
	}
}

func requireJSON(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		buf := &bytes.Buffer{}

		_, err := buf.ReadFrom(r.Body)
		if err != nil {
			http.Error(w, "malformed request - json required", http.StatusBadRequest)
			return
		}

		r.Body = io.NopCloser(buf)
		next.ServeHTTP(w, r)
	}
}

func mockAPICreate(recs map[int]network.DNSRecord) http.HandlerFunc {
	_, mockAPIServerZones := makeMockZones()
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid request", http.StatusInternalServerError)
			return
		}

		req := struct {
			Params network.DNSRecord `json:"params"`
		}{}

		if err = json.Unmarshal(body, &req); err != nil {
			http.Error(w, makeEncodingError(body), http.StatusBadRequest)
			return
		}
		req.Params.ID = types.FlexInt(rand.Intn(10000000))
		req.Params.ZoneID = types.FlexInt(mockAPIServerZones[req.Params.Name])

		if _, exists := recs[int(req.Params.ID)]; exists {
			http.Error(w, "dns record already exists", http.StatusTeapot)
			return
		}
		recs[int(req.Params.ID)] = req.Params

		resp, err := json.Marshal(req.Params)
		if err != nil {
			http.Error(w, "", http.StatusInternalServerError)
			return
		}
		http.Error(w, string(resp), http.StatusOK)
	}
}

func mockAPIDelete(recs map[int]network.DNSRecord) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid request", http.StatusInternalServerError)
			return
		}

		req := struct {
			Params struct {
				Name string `json:"name"`
				ID   int    `json:"id"`
			} `json:"params"`
		}{}

		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, makeEncodingError(body), http.StatusBadRequest)
			return
		}

		if req.Params.ID == 0 {
			http.Error(w, `{"error":"","error_class":"LW::Exception::Input::Multiple","errors":[{"error":"","error_class":"LW::Exception::Input::Required","field":"id","full_message":"The required field 'id' was missing a value.","position":null}],"field":["id"],"full_message":"The following input errors occurred:\nThe required field 'id' was missing a value.","type":null}`, http.StatusOK)
			return
		}

		if _, ok := recs[req.Params.ID]; !ok {
			http.Error(w, fmt.Sprintf(`{"error":"","error_class":"LW::Exception::RecordNotFound","field":"network_dns_rr","full_message":"Record 'network_dns_rr: %d' not found","input":"%d","public_message":null}`, req.Params.ID, req.Params.ID), http.StatusOK)
			return
		}
		delete(recs, req.Params.ID)
		http.Error(w, fmt.Sprintf("{\"deleted\":%d}", req.Params.ID), http.StatusOK)
	}
}

func mockAPIListZones() http.HandlerFunc {
	mockZones, mockAPIServerZones := makeMockZones()
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "invalid request", http.StatusInternalServerError)
			return
		}

		req := struct {
			Params struct {
				PageNum int `json:"page_num"`
			} `json:"params"`
		}{}

		if err = json.Unmarshal(body, &req); err != nil {
			http.Error(w, makeEncodingError(body), http.StatusBadRequest)
			return
		}

		switch {
		case req.Params.PageNum < 1:
			req.Params.PageNum = 1
		case req.Params.PageNum > len(mockZones):
			req.Params.PageNum = len(mockZones)
		}
		resp := mockZones[req.Params.PageNum]
		resp.ItemTotal = types.FlexInt(len(mockAPIServerZones))
		resp.PageNum = types.FlexInt(req.Params.PageNum)
		resp.PageSize = 5
		resp.PageTotal = types.FlexInt(len(mockZones))

		var respBody []byte
		if respBody, err = json.Marshal(resp); err == nil {
			http.Error(w, string(respBody), http.StatusOK)
			return
		}
		http.Error(w, "", http.StatusInternalServerError)
	}
}
