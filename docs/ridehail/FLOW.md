# RideHail flow (sequence)

```text
Rider                          Driver                          Chain (RideHail precompile @ 0x...080a)
-----                          ------                          -----
createRequest(cell, commits)  ------------------------------>  store request + emit RideRequested
                               subscribe RideRequested(cell)
acceptCommit(requestId, hash) ------------------------------>  store commit + emit DriverAcceptCommitted
... wait commit window ...
acceptReveal(requestId, data) ------------------------------>  verify + match + emit Matched
                               fetch prekey bundle (KeyRegistry)
X3DH derive shared secret
postEncryptedMessage(details) ------------------------------>  store ciphertext + emit EncryptedMessage
                               decrypt details (Double Ratchet)
postEncryptedMessage(update)  ------------------------------>  store ciphertext
cancel/start/end (encrypted)  ------------------------------>  store ciphertext + state updates
```

Notes:
- Events are for discovery only; all authoritative state is in storage.
- X3DH + Double Ratchet run client-side; chain validates envelope format only.
