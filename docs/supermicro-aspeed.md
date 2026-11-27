# Supermicro boards with an ASPEED BMC (ATEN firmware)

Notes from a Supermicro A1SRi-2558F, whose BMC is an ASPEED AST2400 running ATEN firmware, as
opposed to the Winbond WPCM450 ("Hermon") in the [X8DTL-6F](supermicro-x8dtl-6f.md). Supermicro
used the same BMC and firmware family across many boards of that era (X9, X10, A1), so other
boards likely behave the same. The web CGI, the RFB handshake, input, power control and virtual
media are the same as on the X8DTL-6F; this page only lists what differs.

## System profile

- **Board**: A1SRi-2558F (Intel Atom C2558, Mini-ITX)
- **BMC hardware**: ASPEED AST2400 (the vendor viewer it serves is `iKVM__V1.69.25`)
- **BMC firmware**: `2.23` (`ipmitool mc info`; the firmware reports product ID `0x0813`,
  "X10SEAA-LN4F", as Supermicro ships one BMC image across boards of a generation)
- **Web server**: HTTP on 80, HTTPS on 443 with **TLS 1.2 only**, `ECDHE-RSA-AES256-GCM-SHA384`.
  CBC suites are refused, so a client that offers only TLS 1.0 era CBC ciphers fails the handshake.
- **Certificate**: self-signed, `CN=IPMI`, issued by Super Micro Computer (expired)
- **Default user**: `ADMIN`

## "KVM SSL": the TLS KVM port

The iKVM JNLP carries two extra arguments after the virtual media port: a `1` flag and a port
number (`3520` by default):

```text
<host> <token> <token> null 5900 623 0 0 1 3520
```

The flag means the firmware's "KVM SSL" option is on. Port 3520 then carries the same RFB 3.8
dialogue as port 5900 inside TLS (TLS 1.2, the same certificate as the web server). The plain
port 5900 stays open as well and accepts the same one-time ticket, so either port works.
`ipmi-connect` uses the TLS port when the flag is set.

## Video: the AST2100 encoding (`0x57`)

Where the WPCM450 sends Hermon tiles (encoding `0x59`), an ASPEED BMC sends its video engine's
bitstream under ATEN's encoding `0x57`, named "AST2100" after the first chip to use it. The
rectangle header is the usual ATEN one (x, y, w, h, encoding, a u32 frame mode, a u32 payload
length). One update carries one rectangle covering the whole screen; a frame may update only
the blocks that changed since the previous one, so the decoder keeps the previous picture.

The payload is a 4-byte header and then the bitstream:

| Offset | Size | Field |
| :---: | :---: | :--- |
| 0 | 1 | Luma quantisation table selector, `0x0` (lowest quality) to `0xB` |
| 1 | 1 | Chroma quantisation table selector, `0x0` to `0xB` |
| 2 | 2 | Chroma subsampling as a big-endian decimal: `422` (16x16 macroblocks) or `444` (8x8 blocks) |
| 4 | … | ASPEED bitstream, read as little-endian 32-bit words |

The bitstream is the same format the Dell C6220's Avocent firmware wraps in its `ib` packets
(see [dell-c6220.md](dell-c6220.md)), so the Go port of Aspeed's reference decoder in
`internal/aspeed/` decodes it unchanged: `422` is the decoder's 4:2:0 macroblock mode and
`444` its 4:4:4 block mode. The decoder's quantisation tables cover selectors `0` to `7`;
selectors `8` to `0xB`, which the BMC uses at its highest quality settings, decode with empty
tables for now.

The screen-off rectangle (`-640 x -480` as u16) and the 1x1 "another session has the video"
rectangle are sent as on the X8DTL-6F.

## Verified on hardware (A1SRi-2558F)

- Web login over TLS 1.2 (after adding GCM cipher suites), ticket fetch, clean logout
- KVM over the TLS port and over plain 5900, both with a one-time ticket
- Video at 1280x1024, selectors 4 and 7, subsampling 422
- Keyboard (USB HID) and the mouse info reply (3 bytes, as on the X8DTL-6F)
- Power status

Not tried on this board: power commands, virtual media, mouse movement.
