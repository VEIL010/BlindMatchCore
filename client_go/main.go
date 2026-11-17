// Windows-friendly replacement for client/main.py + blind_match_client.so (Linux-only).
// Does the same steps as the authors' client:
//   a/b. take the feature vector of a stored ID (INDEX) from DATA_PATH, like main.py does
//   c.   duplicate it to fill 8,192 slots and encrypt it with the public key (CKKS, same parameters)
//   d.   POST the ciphertext to the cluster server (same URL and JSON body as main.py)
//   e.   decrypt the server's answer with the secret key and read the matched ID
//        (same threshold rule as main.py: score between 0.99 and 1.1, same slot -> ID formula)
//
// Usage (from this folder):  go run .            (5 random queries)
//                            go run . -n 20      (20 queries)
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/tuneinsight/lattigo/v5/core/rlwe"
	"github.com/tuneinsight/lattigo/v5/he/hefloat"
	"github.com/tuneinsight/lattigo/v5/ring"
)

const (
	NumSlots      = 8192
	NumStoredCtxt = 2048 // IDs in one cluster
)

// Same CKKS parameters as the authors' server (server/cluster1/blind_match/params.go)
func blindAuthParam() hefloat.Parameters {
	p, err := hefloat.NewParametersFromLiteral(hefloat.ParametersLiteral{
		LogN:            13,
		LogQ:            []int{33, 30, 30, 30},
		LogP:            []int{35},
		LogDefaultScale: 30,
		RingType:        ring.ConjugateInvariant,
	})
	check(err)
	return p
}

func check(err error) {
	if err != nil {
		panic(err)
	}
}

func readFeatures(path string) []float64 {
	f, err := os.Open(path)
	check(err)
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	check(err)
	row := rows[len(rows)-1] // main.py also ends up with the last row
	out := make([]float64, len(row))
	for i, s := range row {
		out[i], err = strconv.ParseFloat(s, 64)
		check(err)
	}
	return out
}

func main() {
	keyPath := flag.String("key", "../blind_match_data/key/", "KEY_PATH (same folder the server uses)")
	dataPath := flag.String("data", "../blind_match_data/data/features128.csv", "DATA_PATH")
	server := flag.String("server", "127.0.0.1:18888", "cluster (or main) server address")
	fvSize := flag.Int("fv", 128, "R_FV_SIZE: feature vector size (128 or 64)")
	logCtxt := flag.Int("logctxt", 2, "LOG_NUM_INPUT_CTXT (main.py default 2)")
	n := flag.Int("n", 5, "number of queries")
	flag.Parse()

	params := blindAuthParam()
	pk, sk := &rlwe.PublicKey{}, &rlwe.SecretKey{}
	b, err := os.ReadFile(*keyPath + "/public_key/0")
	check(err)
	check(pk.UnmarshalBinary(b))
	b, err = os.ReadFile(*keyPath + "/secret_key/0")
	check(err)
	check(sk.UnmarshalBinary(b))
	ecd := hefloat.NewEncoder(params)
	enc := hefloat.NewEncryptor(params, pk)
	dec := hefloat.NewDecryptor(params, sk)

	feats := readFeatures(*dataPath)
	numCtxt := 1 << *logCtxt
	rVecSize := *fvSize / numCtxt
	url := fmt.Sprintf("http://%s/send-ctxt/face/%d/13-%d", *server, *fvSize, *logCtxt)
	fmt.Println("server:", url)
	fmt.Println("INDEX  PREDICTED  SCORE        (INDEX = ID we searched for, PREDICTED = ID found while encrypted)")

	correct := 0
	for q := 0; q < *n; q++ {
		INDEX := rand.Intn(NumStoredCtxt)
		query := feats[*fvSize*INDEX : *fvSize*(INDEX+1)]

		// duplicate to fill all slots (Figure 4, C_u) and encrypt
		vals := make([]float64, 0, NumSlots)
		for len(vals) < NumSlots {
			vals = append(vals, query...)
		}
		pt := hefloat.NewPlaintext(params, params.MaxLevel())
		check(ecd.Encode(vals, pt))
		ct, err := enc.EncryptNew(pt)
		check(err)
		ctBytes, err := ct.MarshalBinary()
		check(err)

		// send (same JSON body as main.py: LogSlots + Ctxt as base64 text)
		body, _ := json.Marshal(map[string]interface{}{"LogSlots": 13, "Ctxt": base64.StdEncoding.EncodeToString(ctBytes)})
		t := time.Now()
		resp, err := http.Post(url, "application/json", bytes.NewReader(body))
		check(err)
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		rtt := time.Since(t)
		var out struct {
			Data string `json:"data"`
		}
		check(json.Unmarshal(raw, &out))
		resB, err := base64.StdEncoding.DecodeString(out.Data)
		check(err)

		// decrypt and read the ID, same rule as main.py
		res := &rlwe.Ciphertext{}
		check(res.UnmarshalBinary(resB))
		dvals := make([]float64, NumSlots)
		check(ecd.Decode(dec.DecryptNew(res), dvals))
		found := false
		for i := 0; i < NumSlots; i++ {
			if dvals[i] > 0.99 && dvals[i] < 1.1 {
				PREDICTED := i/rVecSize + (i%rVecSize)*(NumSlots/rVecSize)
				fmt.Printf("%5d  %9d  %.10f   (round trip %v)\n", INDEX, PREDICTED, dvals[i], rtt.Round(time.Millisecond))
				if PREDICTED == INDEX {
					correct++
				}
				found = true
			}
		}
		if !found {
			fmt.Printf("%5d  no score in (0.99, 1.1) - no match\n", INDEX)
		}
	}
	fmt.Printf("\n%d / %d queries matched the correct ID\n", correct, *n)
}
