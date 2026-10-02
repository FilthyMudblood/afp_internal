# Camera-ready / de-anonymization

Do this only after acceptance. The submitted PDF must stay anonymous.

## Author block

Replace the anonymous author with:

```latex
\author{%
  Muchen He \\
  Independent Researcher \\
  %% add a real contact email before camera-ready
}
```

If affiliation or email should differ, edit here first, not in the public whitepaper.

## Style flag

```latex
\usepackage[dblblindworkshop, final]{neurips_2026}
\workshoptitle{The Third Workshop on Agents in the Wild: Safety, Security, and Beyond}
```

## Optional name restoration

The anonymous draft calls the system **CPL** so reviewers are not one search away from the public repo.

After acceptance you may:

1. Add one sentence in the introduction: “We implement this layer as the Aegis Fabric Protocol (AFP).”
2. Restore GitHub / Zenodo links in a footnote or the appendix.
3. Keep the paper’s claims unchanged. Do not upgrade the Monte Carlo into a field-trial claim, and do not upgrade the LangGraph traces into computer-use / MCP / injection robustness, just because the name is back.

Suggested camera-ready citation of the archival whitepaper (third person is no longer required):

```bibtex
@techreport{he2026afp,
  author      = {He, Muchen},
  title       = {{Aegis Fabric Protocol (AFP) v2.0 --- Protocol Edition}},
  year        = {2026},
  institution = {Zenodo},
  doi         = {10.5281/zenodo.21330527}
}
```

## What not to change

- The ablation sentence: breaker is necessary and sufficient for this *mesh* adversary; 100% is the breaker invariant.
- The limitation paragraph: this is not an in-the-wild agent eval.
- The complementarity claim vs. AgentDojo / MCP security / AI control.
- Identity rotation remains an open-mesh limitation.
