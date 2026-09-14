---
title: zenify observe report
---

## zenify observe report

Tóm tắt observe theo session (dispatch + tool-output)

### Synopsis

Tóm tắt state observe của kit theo từng session: subagent dispatch (từ
`zenify observe count`) và tool-output volume (từ `zenify observe meter`),
session mới hoạt động nhất trước. Đây là phương án single-binary trong-stack,
thay cho một web dashboard nặng.

Muốn web dashboard real-time đầy đủ (replay multi-agent, filter, token graph)
thì chạy song song simple10/agents-observe — nó là MIT và tự đăng ký hook Claude
Code riêng, nên tồn tại song song với hook znf chứ không thay thế.

```
zenify observe report [flags]
```

### Options

```
  -h, --help   help for report
      --json   output JSON instead of a table
```

### SEE ALSO

* [zenify observe](./zenify_observe)	 - Observability: đếm/nhắc fan-out subagent

