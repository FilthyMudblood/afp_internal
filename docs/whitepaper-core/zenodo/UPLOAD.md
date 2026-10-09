# Zenodo upload — AFP-Enterprise

Manuscript: [`../AFP-Core-Path-A-Whitepaper.md`](../AFP-Core-Path-A-Whitepaper.md)

This is a **new record**, not a new version of the mixed v2 (DOI 10.5281/zenodo.21330527). Related older mesh archive: 10.5281/zenodo.20674352.

## Files

| File | Purpose |
|------|---------|
| `../AFP-Core-Path-A-Whitepaper.md` | Main manuscript |
| `zenodo-metadata.json` | Copy into the Zenodo form |
| Optional PDF | Export the same Markdown if you want a PDF reader copy |

## Checklist

1. Manuscript has no `github.com`, no `FilthyMudblood`, no relative repo links used as citations.
2. License on the form: **CC-BY-NC-SA-4.0**.
3. Creator: `He, Muchen` · affiliation **Independent researcher** (empty is also fine; do not invent a lab).
4. Do **not** reuse DOI `10.5281/zenodo.21330527`.
5. After Zenodo reserves a DOI, paste it into the Citation / BibTeX block of the Markdown and, if needed, upload a corrected file as a new version of *this* record.
6. Language English, access Open, version `1.0.0`.
7. Communities / grants: add only if real.

## Steps

1. Sign in at zenodo.org → **New upload**.
2. Upload the Markdown (+ optional PDF).
3. Fill metadata from `zenodo-metadata.json`.
4. Preview → Publish.
5. Put the new DOI back into the manuscript Citation section.

## Preflight

```bash
rg -n 'github\.com|FilthyMudblood|aegis-fabric\.git' \
  docs/whitepaper-core/AFP-Core-Path-A-Whitepaper.md || echo OK
```
