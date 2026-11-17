// FIXED version of the authors' setup/main.go (the original does not compile).
// Generates, for ONE cluster (2,048 IDs, the paper's per-cluster size):
//   <out>/key/{public_key,secret_key,relin_key,galois_key}/...   CKKS keys
//   <out>/ctxt/cluster1/feat128/<0..5>/<g>_<j>                   encrypted DB, 128-d
//   <out>/ctxt/cluster1/feat64/<0..4>/<g>_<j>                    encrypted DB, 64-d
//   <out>/data/features128.csv, features64.csv                   plain feature vectors (one row, like the client expects)
// If -features128 is given (one row of 2048*128 numbers), it is used instead of random vectors.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"

	"github.com/blind_match/modules"
)

const numStored = 2048

func unit(v []float64) []float64 {
	s := 0.0
	for _, x := range v {
		s += x * x
	}
	s = math.Sqrt(s)
	for i := range v {
		v[i] /= s
	}
	return v
}

func randomDB(r *rand.Rand, fv int) [][]float64 {
	db := make([][]float64, numStored)
	for i := range db {
		v := make([]float64, fv)
		for k := range v {
			v[k] = r.NormFloat64()
		}
		db[i] = unit(v)
	}
	return db
}

func readRow(path string, fv int) [][]float64 {
	f, err := os.Open(path)
	modules.ReturnErr(err)
	defer f.Close()
	rd := csv.NewReader(f)
	rd.FieldsPerRecord = -1
	rows, err := rd.ReadAll()
	modules.ReturnErr(err)
	row := rows[len(rows)-1] // the client also uses the last row
	if len(row) < numStored*fv {
		panic(fmt.Sprintf("%s has %d numbers, need %d (2048 x %d)", path, len(row), numStored*fv, fv))
	}
	db := make([][]float64, numStored)
	for i := range db {
		v := make([]float64, fv)
		for k := range v {
			v[k], err = strconv.ParseFloat(row[i*fv+k], 64)
			modules.ReturnErr(err)
		}
		db[i] = unit(v)
	}
	return db
}

func writeRow(path string, db [][]float64) {
	modules.ReturnErr(os.MkdirAll(filepath.Dir(path), 0755))
	f, err := os.Create(path)
	modules.ReturnErr(err)
	defer f.Close()
	var row []string
	for _, v := range db {
		for _, x := range v {
			row = append(row, strconv.FormatFloat(x, 'f', 8, 64))
		}
	}
	w := csv.NewWriter(f)
	modules.ReturnErr(w.Write(row))
	w.Flush()
}


func main() {
	out := flag.String("out", "../blind_match_data", "output folder")
	feat128 := flag.String("features128", "", "optional CSV (one row of 2048*128 numbers)")
	flag.Parse()

	params := modules.Blind_Auth_Param(13)
	_, o := modules.GenKeyPair(params, filepath.Join(*out, "key"))
	fmt.Println("keys saved")

	r := rand.New(rand.NewSource(1))
	var db128 [][]float64
	if *feat128 != "" {
		db128 = readRow(*feat128, 128)
	} else {
		db128 = randomDB(r, 128)
	}
	db64 := randomDB(r, 64)
	writeRow(filepath.Join(*out, "data", "features128.csv"), db128)
	writeRow(filepath.Join(*out, "data", "features64.csv"), db64)

	for _, cfg := range []struct {
		name string
		db   [][]float64
		fv   int
		logs int
	}{{"feat128", db128, 128, 5}, {"feat64", db64, 64, 4}} {
		for l := 0; l <= cfg.logs; l++ {
			cts := modules.GenCtxtDbs(params, o, cfg.db, cfg.fv, l)
			dir := filepath.Join(*out, "ctxt", "cluster1", cfg.name, strconv.Itoa(l))
			modules.ReturnErr(os.MkdirAll(dir, 0755))
			for g := range cts {
				for j := range cts[g] {
					b, err := cts[g][j].MarshalBinary()
					modules.ReturnErr(err)
					modules.ReturnErr(os.WriteFile(filepath.Join(dir, strconv.Itoa(g)+"_"+strconv.Itoa(j)), b, 0644))
				}
			}
		}
		fmt.Println("encrypted database saved:", cfg.name)
	}
	fmt.Println("save finished!")
}
