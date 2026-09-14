---
title: zenify up
---

## zenify up

Onboard workspace: wizard tương tác trong terminal, còn không thì in kế hoạch dry-run (--apply để chạy headless)

```
zenify up [flags]
```

### Options

```
      --apply              apply changes without the interactive wizard (required for non-interactive/CI runs)
      --dry-run            preview the plan without making changes (default true)
  -h, --help               help for up
      --json               emit the plan as a JSON envelope
      --manifest string    path to repos.yaml (default: manifest/repos.yaml under cwd when present, else the copy embedded in the binary)
      --non-interactive    never prompt — forces the headless dry-run/apply path instead of the interactive wizard
      --overlay string     path to personal overlay (default <workspace>/.zenify-overlay.yaml)
      --workspace string   workspace root (default: the workspace found from cwd or ~/.zenify/workspace; the wizard asks when there is none)
```

### SEE ALSO

* [zenify](./zenify)	 - zenify — bộ công cụ workspace dùng chung của team

