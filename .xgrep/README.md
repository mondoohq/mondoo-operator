# xgrep rules

Repo-specific static checks, run with [xgrep](https://www.npmjs.com/package/@mondoohq/xgrep)
(`npm i -g @mondoohq/xgrep`, Semgrep-compatible rule syntax). One rule per file, each with a
sibling `.go` fixture (`//go:build ignore`, never compiled) carrying `// ruleid:` / `// ok:` cases.

```bash
xgrep test .xgrep/rules                                     # fixtures, run after editing a rule
xgrep scan -f .xgrep/rules .                                # whole repo
xgrep scan -f .xgrep/rules --baseline-commit origin/main .  # changed lines only (CI shape)
xgrep explain .xgrep/rules/<rule>.yaml .                    # per clause: what matched, what never did
```

## Object names

Why: a MondooAuditConfig name, a node name on EKS/GKE, an external cluster name or a namespace
are user input. Concatenating them into an object name blows the limits: CronJob 52 (the Job
suffix needs 11), Job/Service/StatefulSet 63, label values 63, and this repo caps every generated
name at `k8s.ResourceNameMaxLength` (52). Each time this was fixed by hand (#1651, #1498, #1391) a
new call site reintroduced it.

The contract, in two halves that are each one rule:

| Rule | Checks |
|---|---|
| `object-name-not-length-checked` | Any `<group>.<Kind>{ObjectMeta: metav1.ObjectMeta{Name: ...}}` literal gets a name that passed through a `*<Kind>Name` helper or a truncating primitive (`k8s.CronJobName`, `nodeScopedName`). Taint rule: sources are every `.Name` / `.Namespace`, sinks are every object literal, sanitizers are the helpers. |
| `name-helper-without-length-check` | A function named `*<Kind>Name` returning a string truncates: its body uses `k8s.ResourceNameMaxLength`, delegates to `nodeScopedName` / `k8s.cronJobName` / `k8s.CronJobName*`, or hashes with sha256. |
| `name-limit-arithmetic-unclamped` | `ResourceNameMaxLength - len(...)` is never passed to a helper unclamped (#1651's panic). |

The first rule trusts a helper by its name; the second polices the body. Known gap: in
`--baseline-commit` mode a new call to a plain helper that already exists on main is invisible,
because the helper's own finding sits on an unchanged line. The full scan lists those helpers;
fix each once and the gap closes.

A finding that is deliberate (an old-style name that is looked up only to delete it) gets an inline
`// nosemgrep: <rule id>` comment with the reason, not a rule change.
