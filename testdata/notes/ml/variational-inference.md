---
title: Variational Inference
domain: machine-learning
stage: deep
status: draft
tags: [ml, bayesian]
sources: [bishop2006prml]
---

# Variational Inference

## One-sentence intuition

Turn inference into optimization: find the closest tractable distribution.

Q: What does the ELBO lower-bound?
A: The marginal log likelihood $\log p(x)$. <!-- srs:0a1b2c3d -->

Q: Why is the KL divergence in VI reversed
compared to expectation propagation?
A: VI minimizes $KL(q \| p)$, which is mode-seeking;
EP minimizes $KL(p \| q)$, which is mass-covering.

The ELBO decomposes into {{c1::expected log likelihood}} minus {{c2::the KL from the prior}}.

```python
# Q: this should not become a card
# A: because it is inside a code fence
def elbo(q, x):
    pass
```

## Open questions

- How does amortization change the variational family?
- When does the mean-field assumption break down badly?

## Connections

See Bayesian neural networks.
