package modules

// FIXED version of the authors' setup/modules/generator.go
//  - GenKeyPair: writes keys to <keyDir>/public_key/0, secret_key/0, relin_key/0, galois_key/0..28
//    (the layout the cluster server and blind_match_client.so read). The original wrote every
//    key to placeholder paths, so the Galois keys overwrote each other.
//  - GenCtxtDbs: packs the database in the layout HE_Cossim and the client decoder expect.
//    The original used one variable for two different sizes, so the packing was wrong.

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/tuneinsight/lattigo/v5/core/rlwe"
	"github.com/tuneinsight/lattigo/v5/he/hefloat"
)

type KeySet struct {
	Pk  *rlwe.PublicKey
	Sk  *rlwe.SecretKey
	Rlk *rlwe.RelinearizationKey
	Gks []*rlwe.GaloisKey
}

type ObjectSet struct {
	Ecd  *hefloat.Encoder
	Enc  *rlwe.Encryptor
	Dec  *rlwe.Decryptor
	Eval *hefloat.Evaluator
}

func writeKey(dir, kind string, idx int, b []byte) {
	ReturnErr(os.MkdirAll(filepath.Join(dir, kind), 0755))
	ReturnErr(os.WriteFile(filepath.Join(dir, kind, strconv.Itoa(idx)), b, 0644))
}

func GenKeyPair(params hefloat.Parameters, keyDir string) (*KeySet, *ObjectSet) {
	// same Galois elements as the authors' code (3 + 9 + 17 = 29 keys)
	galEls := []uint64{params.GaloisElement(-32), params.GaloisElement(-64), params.GaloisElement(-128)}
	for i := 0; i < 9; i++ {
		galEls = append(galEls, params.GaloisElement(PowerOfTwo(i)))
	}
	for i := 0; i < 17; i++ {
		galEls = append(galEls, params.GaloisElement((-1)*i))
	}

	kgen := rlwe.NewKeyGenerator(params)
	sk := kgen.GenSecretKeyNew()
	pk := kgen.GenPublicKeyNew(sk)
	rlk := kgen.GenRelinearizationKeyNew(sk)
	gks := kgen.GenGaloisKeysNew(galEls, sk)

	b, err := sk.MarshalBinary()
	ReturnErr(err)
	writeKey(keyDir, "secret_key", 0, b)
	b, err = pk.MarshalBinary()
	ReturnErr(err)
	writeKey(keyDir, "public_key", 0, b)
	b, err = rlk.MarshalBinary()
	ReturnErr(err)
	writeKey(keyDir, "relin_key", 0, b)
	for i := range gks {
		b, err = gks[i].MarshalBinary()
		ReturnErr(err)
		writeKey(keyDir, "galois_key", i, b)
	}

	ks := &KeySet{pk, sk, rlk, gks}
	o := &ObjectSet{
		Ecd: hefloat.NewEncoder(params),
		Enc: hefloat.NewEncryptor(params, pk),
		Dec: hefloat.NewDecryptor(params, sk),
	}
	return ks, o
}

// GenCtxtDbs encrypts NUM_STORED vectors of size fvSize with 2^logNumInput input ciphertexts.
// Returns [groups][numInput] ciphertexts; ciphertext (g, j) holds block j (width fvSize/numInput)
// of vectors g*perCt .. g*perCt+perCt-1 at slots t*width + r.
func GenCtxtDbs(params hefloat.Parameters, o *ObjectSet, db [][]float64, fvSize int, logNumInput int) [][]*rlwe.Ciphertext {
	slots := params.MaxSlots()
	numInput := PowerOfTwo(logNumInput)
	width := fvSize / numInput
	perCt := slots / width
	groups := len(db) / perCt
	out := make([][]*rlwe.Ciphertext, groups)
	for g := 0; g < groups; g++ {
		out[g] = make([]*rlwe.Ciphertext, numInput)
		for j := 0; j < numInput; j++ {
			vals := make([]float64, slots)
			for t := 0; t < perCt; t++ {
				copy(vals[t*width:(t+1)*width], db[g*perCt+t][j*width:(j+1)*width])
			}
			pt := hefloat.NewPlaintext(params, params.MaxLevel())
			ReturnErr(o.Ecd.Encode(vals, pt))
			ct, err := o.Enc.EncryptNew(pt)
			ReturnErr(err)
			out[g][j] = ct
		}
	}
	return out
}
