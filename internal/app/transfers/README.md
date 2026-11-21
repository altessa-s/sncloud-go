# Shared byte transfers

`transfers` is the byte path for web, device, and digest clients. Wiring
constructs one `Transfers` with the existing `port.Transport`, clock, and retry
policy. It does not create another HTTP client or retain a session token.

An uploading client:

1. Calls `Measure(file, name)` before apply to obtain size and MD5.
2. Captures `session.Pin()` and uses `Authorized` for apply, passing
   that pin and decoding into `UploadSlot`.
3. Builds `NewSigner(channel, session, slot.FileServer, pin)` and
   `FormServer{Host: configuredHost, Token: func() (string, error) { return session.AccessToken(pin) }}` for forms. Obtain the
   token with `AccessToken(pin)`, never with an unpinned lookup.
4. Calls `Send(ctx, original, slot, signer, formServer, progress)`.
5. Uses the returned stored name in its own finish request, again pinned to
   the same pin. Digests can omit finish when their protocol does.

`UploadSlot` accepts both `url` and the digest handwriting spelling `s3Url`.
`FileServer` preserves a number or string when sent back to the signer.
`Send` handles single PUT, S3 multipart, whole forms, and chunked forms; OSS
returns a typed rejection. A form's origin is replaced with the configured
origin before its account token is attached. S3 never receives that token.

For downloads, pass `Fetch` a signed URL, a nonnegative byte ceiling, an
optional MD5 (empty means absent), an optional inclusive `ByteRange`, and an
optional progress callback. Use `DownloadLimit` when the size is unknown.
`ServedFrom` repairs `/api/oss/` URLs behind private-cloud proxies. `Fetch`
returns a temporary filename owned by its caller; remove it after use. It
removes failed downloads itself. A range is checked against the 206 status,
Content-Range, and actual size, instead of the full-file MD5.

Callbacks run synchronously with transfers and may run again on a retry.
They must not block indefinitely. Form progress excludes MIME framing;
multipart progress includes preceding parts. S3 parts stream replayable sections of the source file without temporary copies.
Keep the source unchanged until upload completes. Private forms hash payload bytes while writing their replayable MIME file,
and remove that file before return. S3 abort runs with `context.WithoutCancel` and a
five-second deadline, preserving the initiating failure.
