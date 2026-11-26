# Mssg

A decentralized, end-to-end encrypted messenger for Android and iOS (and desktops), written
in Go with [Giode](https://github.com/xmhrnx/giode). No servers, no accounts, no phone numbers.

- **Meet once with a code.** One person shares a short [magic wormhole](https://magic-wormhole.readthedocs.io)
  code (`7-guitarist-revenge`); the other types it. The PAKE-encrypted wormhole carries each
  device's public key and addresses, and is never used again.
- **Then talk directly.** Messages, files, voice and video calls go peer to peer over
  [libp2p](https://libp2p.io), found through the public IPFS DHT, through NATs with hole
  punching and circuit relays, and over mDNS on the same Wi-Fi.
- **Everything is end-to-end encrypted.** Streams use Noise with each device's Ed25519 key;
  calls use WebRTC's DTLS-SRTP, whose fingerprints travel over that authenticated channel.
  Relays and STUN servers only ever see ciphertext. Each contact screen shows a safety number
  to compare in person.
- **Text, emoji, GIFs, photos, videos, voice messages, any file**, voice and video calls, read
  receipts, typing indicators, delivery ticks, light and dark themes.

## Screens

Onboarding · chat list · invite code · conversation with photos, animated GIFs, voice notes,
files and videos · full-screen viewer · incoming and active calls · contact with safety
number · settings.

## How it works

```
┌────────────┐  wormhole code (once)   ┌────────────┐
│  Alice     │ ◄─────────────────────► │  Bob       │   card: peer ID, name, addresses, one-time token
│  device    │                         │  device    │
│            │  libp2p (QUIC / TCP)    │            │   Noise-encrypted streams:
│  Ed25519   │ ◄═════════════════════► │  Ed25519   │   /mssg/invite  handshake with the token
│  identity  │  direct, hole-punched,  │  identity  │   /mssg/msg     messages, receipts, profiles, call signaling
│            │  relayed or LAN (mDNS)  │            │   /mssg/file    attachments, resumable, SHA-256 checked
│            │                         │            │
│            │  WebRTC (DTLS-SRTP)     │            │   voice and video
└────────────┘ ◄═════════════════════► └────────────┘
```

1. **Identity.** On first run the app makes an Ed25519 key; its libp2p peer ID is your
   permanent address. The key and everything else live in a bbolt database whose values are
   sealed with XChaCha20-Poly1305 under a key kept in the platform's secure storage (Android
   Keystore, iOS Keychain).
2. **Invites.** *Share my code* opens a wormhole with your card and a one-time token. When your
   friend redeems it, their device adds you, dials you over libp2p and presents the token; your
   device checks it, adds them, and both are confirmed. Codes expire after one use; tokens
   after 24 hours.
3. **Finding each other.** Every device announces a presence record for its peer ID in the
   IPFS DHT and remembers contacts' last addresses and its own listen port. A presence loop
   keeps contacts connected (with backoff), so online dots, typing and incoming calls work.
4. **Messages.** Each message is one frame on a fresh stream, answered with an acknowledgment
   (delivered); read receipts follow. Messages to someone offline wait in an encrypted outbox
   and go out when you're both online.
5. **Files.** The sender keeps the file and offers it with its size, type, SHA-256 and a small
   thumbnail; the receiver pulls the bytes, resuming where it stopped, and checks the hash.
   Media up to a size you choose downloads automatically.
6. **Calls.** WebRTC through Giode's `rtc` package (VP8 video, G.711 audio, the platform's echo
   cancellation), with offers, answers and trickled ICE candidates sent over the libp2p
   channel. Phones show the system call screen (CallKit, Android ConnectionService).

## Project layout

```
main.go                 window and wiring
giode.toml              app metadata for the Giode CLI
assets/                 Inter font, Lucide icons, app icon
internal/
  store/                encrypted database: profile, contacts, messages, outbox, invites
  wire/                 frames on the wire
  p2p/                  libp2p node: transports, DHT, relays, hole punching, mDNS
  invite/               wormhole contact cards
  mediakit/             MIME kinds, thumbnails, GIF frames, WAV and waveforms
  messenger/            the engine: handshake, delivery, receipts, files, presence
ui/                     Giode interface, one component per file
```

## Building

You need Go 1.26 and the [Giode CLI](https://github.com/xmhrnx/giode-cli)
(`go install github.com/xmhrnx/giode-cli/cmd/giode@latest`); `giode doctor` lists what else
each platform needs.

```sh
giode run                 # on this computer
giode build android       # dist/android/Mssg.apk (Android SDK, NDK, JDK)
giode run android         # build, install and launch on a connected phone
giode build ios           # on a Mac with Xcode; set ios signing in giode.toml
giode build -release aab  # signed Play Store bundle
```

Tests: `go test ./...`. The messenger tests run two real libp2p nodes on localhost through the
handshake, messages, receipts, a 3 MB file transfer and the offline outbox.
`MSSG_NET_TESTS=1 go test ./internal/invite` uses the wormhole server; set
`MSSG_WORMHOLE_URL` to use your own mailbox server (`wormhole-mailbox-server`) instead of the
public one.

## Automation

Run with `GIODE_MCP=1` and Giode's MCP server exposes the UI to agents and tests (snapshot,
click, type, screenshot). The app adds a `send_file` tool for sending a local file to a contact
without the system picker.

## Limits worth knowing

- **Both devices must be online at the same time** for a message to move; there's no server to
  hold it. Turn on *Stay online in the background* (an Android foreground service) to receive
  with the app closed. iOS suspends apps in the background, so messages and calls arrive when
  the app is opened; waking an iPhone for a call needs a VoIP push service, which would be a
  server.
- **Calls** use public STUN servers (Google, Cloudflare) to learn your public address; on
  networks that block direct connections add your own TURN server in Settings. Audio is G.711
  (telephone quality); video is a pure-Go VP8 at about 1–2 Mbit/s.
- **Large files over relays.** Public circuit relays carry little data; transfers wait for a
  hole-punched direct connection first and resume if a relay cuts them off.
- **GIFs** are sent from your files; there's no GIF search, since that would mean a central
  service.
- The wormhole step uses the public mailbox server `relay.magic-wormhole.io` only to swap the
  two cards, PAKE-encrypted; it can't read them.
