# XEdDSA Implementation Guide

This document explains the two XEdDSA implementations in this project.

## Two Implementations

### 1. Original Implementation (`x3dh.ts`) - XEdDSA-inspired

**File**: `src/x3dh.ts`
**Test**: `npm run tester`

**Approach**:
- Uses SHA-512 hash to convert X25519 → Ed25519
- Simple and self-contained
- ✅ Works correctly for our use case
- ❌ Not compatible with Signal Protocol

**Key Conversion**:
```typescript
function x25519ToEd25519Private(x25519PrivKey: Uint8Array): Uint8Array {
  const hash = sha512(x25519PrivKey);
  const scalar = hash.slice(0, 32);
  // Ed25519 clamping
  scalar[0] &= 248;
  scalar[31] &= 127;
  scalar[31] |= 64;
  return scalar;
}
```

**Use When**:
- Building your own protocol
- No need for Signal compatibility
- Want simple, minimal dependencies

---

### 2. Proper Implementation (`x3dh_proper.ts`) - Signal Compatible

**File**: `src/x3dh_proper.ts`
**Test**: `npm run tester_proper`

**Approach**:
- Uses `@privacyresearch/curve25519-typescript` for X25519
- Uses `@privacyresearch/ed25519-ts` for Ed25519
- Follows Signal XEdDSA specification
- ✅ Signal Protocol compatible
- ✅ Proper curve conversion

**Libraries**:
```json
{
  "@privacyresearch/curve25519-typescript": "^1.0.3",
  "@privacyresearch/ed25519-ts": "^0.0.3"
}
```

**Key Features**:
1. **X25519 Key Pair Generation**: Uses `curve25519.generateKeyPair()`
2. **Ed25519 Conversion**: Proper Montgomery ↔ Edwards curve math
3. **XEdDSA Signing**: Uses Ed25519 private key derived from X25519
4. **XEdDSA Verification**: Uses Ed25519 public key

**Use When**:
- Need Signal Protocol compatibility
- Want to interoperate with Signal apps
- Building production messaging system

---

## Installation

```bash
npm install
```

This will install both implementations' dependencies.

---

## Testing

### Test Original Implementation
```bash
npm run tester
```

**Expected Output**:
```
sharedInitiator <hex>
sharedResponder <hex>
Bob 수신 3: message 3
Bob 수신 2: message 2
Bob 수신 1: message 1
Alice 수신: reply from bob
```

### Test Proper Implementation
```bash
npm run tester_proper
```

**Expected Output**:
```
=== Testing Proper XEdDSA Implementation ===

Step 1: Generating identity keys with proper XEdDSA...
✓ Alice and Bob identity keys generated
...
✅ All tests passed with proper XEdDSA implementation!
```

### Test Group Messaging (Sender Keys)
```bash
npm run tester_group
```

---

## Comparison

| Feature | Original (`x3dh.ts`) | Proper (`x3dh_proper.ts`) |
|---------|---------------------|--------------------------|
| Dependencies | `@noble/*` only | + Privacy Research libs |
| Signal Compatible | ❌ No | ✅ Yes |
| XEdDSA Spec | Inspired | Compliant |
| Curve Conversion | SHA-512 hash | Mathematical |
| Production Ready | ✅ Yes (custom) | ✅ Yes (standard) |
| Bundle Size | Smaller | Larger |

---

## Architecture

### Common Components (Both Implementations)

**Double Ratchet** (`double_ratchet.ts`):
- Forward/Backward secrecy
- Out-of-order message delivery
- ChaCha20-Poly1305 AEAD encryption

**Sender Keys** (`tester_group.ts`):
- Efficient 1:N group messaging
- Forward secrecy via chain key ratcheting
- Ed25519 message authentication

### Key Types

```typescript
// X3DH Bundle (both implementations)
type X3DHBundle = {
  identityPub: Uint8Array;           // X25519 public key (DH)
  identityEd25519Pub: Uint8Array;    // Ed25519 public key (signatures)
  signedPreKeyPub: Uint8Array;
  signature: Uint8Array;
  oneTimePreKeyPub?: Uint8Array;
};
```

---

## Migration Guide

### From Original to Proper

If you want to switch from the original to the proper implementation:

1. **Install Dependencies**:
   ```bash
   npm install @privacyresearch/curve25519-typescript @privacyresearch/ed25519-ts
   ```

2. **Update Imports**:
   ```typescript
   // Before
   import { generateIdentityKey } from "./x3dh";

   // After
   import { generateIdentityKey } from "./x3dh_proper";
   ```

3. **Update Key Generation**:
   ```typescript
   // Before (returns object with ed25519PublicKey as Uint8Array)
   const alice = generateIdentityKey();

   // After (returns object with ed25519PublicKey as PublicKey)
   const alice = generateIdentityKey();
   const ed25519Bytes = alice.ed25519PublicKey.bytes;
   ```

4. **Test**:
   ```bash
   npm run tester_proper
   ```

---

## Protocol Flow

### 1:1 Messaging (X3DH + Double Ratchet)

```
Bob                                           Alice
│                                              │
├─ generateIdentityKey()                      │
├─ generateSignedPreKey()                     │
├─ generateOneTimePreKey()                    │
│                                              │
├─ Publish Bundle ──────────────────────────► │
│   • identityPub (X25519)                    ├─ verifySignedPreKey()
│   • identityEd25519Pub (Ed25519)            ├─ deriveSharedSecretInitiator()
│   • signedPreKeyPub                         │
│   • signature (XEdDSA)                      │
│   • oneTimePreKeyPub                        │
│                                              │
├─ deriveSharedSecretResponder()              │
│                                              │
├─ initializeResponder()                      ├─ initializeInitiator()
│                                              │
│◄──────────── Encrypted Messages ───────────►│
│   (Double Ratchet with Forward Secrecy)     │
```

### Group Messaging (Sender Keys)

```
Alice                   Bob         Charlie       Dave
│                        │             │            │
├─ generateSenderKey()  │             │            │
│                        │             │            │
├─ Distribute via 1:1 ──┼────────────┼───────────►│
│   (encrypted)          │             │            │
│                        │             │            │
├─ encryptGroupMessage() │             │            │
│   (1x encryption)      │             │            │
│                        │             │            │
├─ Broadcast ───────────►├────────────►├───────────►│
│   (same ciphertext)    │             │            │
│                        │             │            │
│                   decryptGroupMessage()      │
│                   decryptGroupMessage()      │
│                                        decryptGroupMessage()
```

---

## Security Considerations

### Both Implementations

✅ **Forward Secrecy**: Double Ratchet key rotation
✅ **Backward Secrecy**: Out-of-order delivery support
✅ **Authentication**: Ed25519 signatures
✅ **Encryption**: ChaCha20-Poly1305 AEAD
✅ **Key Exchange**: X25519 ECDH

### Original Implementation

⚠️ **Not Signal Compatible**: Custom XEdDSA variant
⚠️ **Limited Interoperability**: Only works with same implementation

### Proper Implementation

✅ **Signal Compatible**: Standard XEdDSA
✅ **Interoperable**: Works with Signal Protocol apps
✅ **Audited Libraries**: Uses Privacy Research Group libraries

---

## References

- [Signal XEdDSA Specification](https://signal.org/docs/specifications/xeddsa/)
- [Signal X3DH Specification](https://signal.org/docs/specifications/x3dh/)
- [Signal Double Ratchet](https://signal.org/docs/specifications/doubleratchet/)
- [@privacyresearch/curve25519-typescript](https://github.com/privacyresearchgroup/curve25519-typescript)
- [@privacyresearch/ed25519-ts](https://github.com/privacyresearchgroup/ed25519-ts)

---

## License

See project LICENSE file.
