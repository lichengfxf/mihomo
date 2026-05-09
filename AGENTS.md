# Project Notes

- `svnt` outbound is implemented in `adapter/outbound/svnt.go` with local message framing in `adapter/outbound/svnt_msg.go`.
- `svnt` currently supports TCP only and `MSG_TYPE_TUNNEL` only; `encrypt` supports `0/1/2`, and `1` switches the data stream to the SVNT AES-CFB wrapper after handshake.
- `key-data` is required and is converted to `Auth` by mirroring `SVNT/src/svnt/crypto/crypto.go`.
- TUN docs and examples live under `docs/`; `docs/config_svnt.yaml` is the current minimal tun+svnt example.
- The TUN LAN route fix was added in `listener/sing_tun/server.go` for Linux `/32` route injection when `tun.auto-route` is enabled.
- Verify Go changes with `gofmt` and `go test ./adapter/... ./constant/...` when toolchain is available.
