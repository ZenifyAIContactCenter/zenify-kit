---
summary: Chặn tiếng Việt trong file mà agent đọc: skill, rule, và mã nguồn kit.
---
## Khi nào dùng

Trước khi mở PR sửa rule team hoặc skill. Bước lint của `/znf:ship` chạy lệnh này trên thư mục rule trong knowledge store.

## Kết quả

Lệnh in từng dòng vi phạm theo file và số dòng, hoặc "sạch." khi không có. Dòng nằm trong code fence, inline code, hoặc mang marker `<!-- znf:allow-lang -->` được bỏ qua. Trong mã nguồn kit, marker là `//znf:allow-lang` ở cuối dòng.

Không truyền tham số, lệnh quét asset skill đi kèm binary. Truyền đường dẫn để quét nơi khác.

## Cờ

| Cờ | Ý nghĩa |
|---|---|
| `--include-go` | Quét cả mã nguồn Go của kit. |

## Ví dụ

```bash
zenify rules lint ~/.zenify/knowledge/.config/rules
```
