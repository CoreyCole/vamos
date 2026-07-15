# Monorepo PR-feedback workspace pattern

When preparing to address monorepo stack feedback, keep the canonical morning-sync checkout on `develop`.

Pattern used successfully:

1. Verify canonical checkout:
   - path: `/Users/swarm/cn/chestnut-flake/monorepo-swarm`
   - branch: `develop`
   - clean status
2. Create a copied sibling workspace, e.g. `/Users/swarm/cn/chestnut-flake/monorepo-<slug>-pr-feedback`.
3. In the copy, run `gt get --no-interactive <stack-top-branch>` to recover the submitted stack tip.
4. Do PR-feedback edits, `gt modify --into <comment-pr-branch>` when targeting a downstack PR, then `gt submit --no-verify`.
5. Before finishing, verify `monorepo-swarm` is still clean on `develop`.

Why: `monorepo-swarm` is used by morning sync automation; leaving it on a feature/review branch breaks that workflow.