# Blind-Match Windows patch (single-cluster test, no Linux needed)

Unzip into the cloned `blind-match` folder (overwrite files), then delete
`setup\go.work` and `server\cluster1\go.work`.

The authors' encryption/matching code (`server/cluster1/blind_match/he_module.go`,
`params.go`, `utils.go`, `HE_Cossim`) is NOT changed.

| File | Problem in the original | Fix |
|---|---|---|
| `setup/main.go` | Does not compile (`import modules`, undefined `o`, `FV_SIZE`, `blind_auth`, placeholder paths) | Rewritten: keys + encrypted DB for one cluster (2,048 IDs), 128-d and 64-d, saved in `blind_match_data/` |
| `setup/modules/generator.go` | All 29 Galois keys written to one placeholder path; DB packing used one variable for two sizes | Keys saved as `key/<type>/<i>` (the layout the server reads); packing matches `HE_Cossim` |
| `setup/go.mod`, `server/cluster1/go.mod` (+ delete the two go.work files) | `main.go` was outside every Go module, so it could not build | Normal Go modules with local `replace` lines |
| `server/cluster1/blind_match/go.mod` | Required onnx-go and image libraries the code never uses | Requires only Lattigo v5.0.2 |
| `server/cluster1/main.go` | Paths under `/root/src/...` | 3 path lines changed (marked CHANGED) |
| `client_go/` (new) | The authors' client needs `blind_match_client.so`, a Linux-only library | Go client doing the same steps as `client/main.py`: same CKKS parameters and Lattigo version, same keys, same HTTP request, same decryption and ID rule |

Like the original client, each query is a stored feature vector, so a correct run prints
the same ID twice with a score close to 1.0. Feature vectors are random unless you run
`go run . -features128 my_features.csv` in `setup` (one row of 2048x128 numbers).
