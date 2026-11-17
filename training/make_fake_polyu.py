"""
make_fake_polyu.py - create FAKE fingerprint-like images so the Blind-Match training
notebook (training/blind-match_poly_1_N.ipynb) can be tested before the real
PolyU dataset arrives.

How it works
  * Reads every image path listed in polyu_meta.txt and polyu_meta_eval.txt.
  * Writes a synthetic "ridge pattern" image at each path under ../data/PolyU_Dataset/
  * Images of the same finger (same folder + same number before "_", e.g. 12_1 ... 12_6)
    share one base pattern, with a small random rotation, shift, blur and noise
    per image, so the model has something real to learn and accuracy should rise.

Run it from the training folder:
    python make_fake_polyu.py

Results on this data say NOTHING about the paper's accuracy. Its purpose is to
check that the code, GPU, paths and saving all work end to end.
Delete data/PolyU_Dataset before putting the real dataset there.
"""
import argparse
import os
import zlib

import numpy as np
from PIL import Image, ImageFilter

H, W = 300, 260  # roughly fingerprint-shaped; the notebook resizes to 224x224 anyway


def smooth_field(rng, grid=5, scale=1.0):
    """Random smooth 2D field of size HxW (bicubic upsampling of a small random grid)."""
    small = rng.normal(0, scale, (grid, grid)).astype(np.float32)
    img = Image.fromarray(small, mode="F").resize((W, H), Image.BICUBIC)
    return np.asarray(img)


def base_pattern(finger_key):
    """Parameters that define one finger's ridge pattern (same key -> same pattern)."""
    rng = np.random.default_rng(zlib.crc32(finger_key.encode()))
    return {
        "freq": rng.uniform(0.085, 0.12),            # ridges per pixel
        "angle": rng.uniform(0, np.pi),             # main ridge direction
        "warp1": smooth_field(rng, 4, 9.0),          # large-scale bending
        "warp2": smooth_field(rng, 7, 3.0),          # finer bending
        "cx": rng.uniform(0.35, 0.65) * W,           # a whorl/loop-like centre
        "cy": rng.uniform(0.35, 0.65) * H,
        "swirl": float(rng.choice([-1, 1])),  # whole number avoids a seam line
    }


def render(p, rng):
    """Render one noisy impression of a finger."""
    yy, xx = np.mgrid[0:H, 0:W].astype(np.float32)
    # per-impression placement: small rotation and shift (like pressing the finger again)
    rot = np.deg2rad(rng.uniform(-8, 8))
    dx, dy = rng.uniform(-12, 12, size=2)
    x = (xx - W / 2) * np.cos(rot) - (yy - H / 2) * np.sin(rot) + W / 2 + dx
    y = (xx - W / 2) * np.sin(rot) + (yy - H / 2) * np.cos(rot) + H / 2 + dy

    theta = np.arctan2(y - p["cy"], x - p["cx"])
    phase = 2 * np.pi * p["freq"] * (x * np.cos(p["angle"]) + y * np.sin(p["angle"]))
    xi = np.clip(x, 0, W - 1).astype(int)
    yi = np.clip(y, 0, H - 1).astype(int)
    phase += p["warp1"][yi, xi] + p["warp2"][yi, xi] + p["swirl"] * theta

    img = 0.5 + 0.5 * np.sin(phase)
    # elliptical finger mask with a soft edge
    r = ((xx - W / 2 - dx) / (W * 0.45)) ** 2 + ((yy - H / 2 - dy) / (H * 0.47)) ** 2
    mask = np.clip((1.0 - r) * 6, 0, 1)
    # uneven pressure / contrast + sensor noise
    pressure = 0.75 + 0.25 * smooth_field(rng, 3, 1.0).clip(-1, 1)
    img = img * pressure * mask                 # 0 outside the finger
    img = 1.0 - img * 0.85                       # dark ridges on light background
    img += rng.normal(0, 0.06, img.shape)
    img = (np.clip(img, 0, 1) * 255).astype(np.uint8)
    pil = Image.fromarray(img, mode="L").filter(ImageFilter.GaussianBlur(rng.uniform(0.4, 1.2)))
    return pil


def finger_key(rel_path):
    folder, name = os.path.split(rel_path.replace("\\", "/"))
    subject = os.path.splitext(name)[0].split("_")[0]
    return f"{folder}/{subject}"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", default="../data/PolyU_Dataset",
                    help="same folder as `root` in the notebook's FingerprintDataset")
    ap.add_argument("--metas", nargs="+", default=["polyu_meta.txt", "polyu_meta_eval.txt"])
    ap.add_argument("--overwrite", action="store_true", help="replace images that already exist")
    ap.add_argument("--seed", type=int, default=0)
    args = ap.parse_args()

    paths = []
    for m in args.metas:
        with open(m, encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if line:
                    paths.append(line.rsplit("|", 1)[0])
    paths = sorted(set(paths))
    print(f"{len(paths)} image paths found in {', '.join(args.metas)}")

    os.makedirs(args.root, exist_ok=True)
    with open(os.path.join(args.root, "FAKE_DATA_README.txt"), "w", encoding="utf-8") as f:
        f.write("These images are SYNTHETIC test data made by make_fake_polyu.py.\n"
                "Delete this whole folder before adding the real PolyU dataset.\n")

    rng = np.random.default_rng(args.seed)
    cache, made, skipped = {}, 0, 0
    for i, rel in enumerate(paths, 1):
        out = os.path.join(args.root, rel)
        if os.path.exists(out) and not args.overwrite:
            skipped += 1
            continue
        key = finger_key(rel)
        if key not in cache:
            cache[key] = base_pattern(key)
        os.makedirs(os.path.dirname(out), exist_ok=True)
        render(cache[key], rng).save(out, quality=92)
        made += 1
        if i % 500 == 0:
            print(f"  {i}/{len(paths)}")
    print(f"Done: {made} images written, {skipped} already existed (skipped), "
          f"{len(cache)} fake fingers, under {os.path.abspath(args.root)}")


if __name__ == "__main__":
    main()
