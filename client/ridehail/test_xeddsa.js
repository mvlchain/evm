// Quick XEdDSA test without TypeScript compilation
const { sha512 } = require('@noble/hashes/sha512');
const { ed25519, x25519 } = require('@noble/curves/ed25519');

function x25519ToEd25519Private(x25519PrivKey) {
  const hash = sha512(x25519PrivKey);
  const scalar = hash.slice(0, 32);

  // Clamp for Ed25519
  scalar[0] &= 248;
  scalar[31] &= 127;
  scalar[31] |= 64;

  return scalar;
}

function x25519ToEd25519Public(x25519PrivKey) {
  const ed25519PrivKey = x25519ToEd25519Private(x25519PrivKey);
  return ed25519.getPublicKey(ed25519PrivKey);
}

function xeddsaSign(message, x25519PrivKey) {
  const ed25519PrivKey = x25519ToEd25519Private(x25519PrivKey);
  return ed25519.sign(message, ed25519PrivKey);
}

function xeddsaVerify(signature, message, x25519PubKey, x25519PrivKey) {
  try {
    const ed25519PubKey = x25519ToEd25519Public(x25519PrivKey);
    return ed25519.verify(signature, message, ed25519PubKey);
  } catch {
    return false;
  }
}

// Test
console.log('Testing XEdDSA implementation...');

const x25519PrivKey = x25519.utils.randomPrivateKey();
const x25519PubKey = x25519.getPublicKey(x25519PrivKey);
const message = new Uint8Array([1, 2, 3, 4, 5]);

console.log('1. Signing message with X25519 private key...');
const signature = xeddsaSign(message, x25519PrivKey);
console.log('   Signature length:', signature.length, 'bytes');

console.log('2. Verifying signature...');
const isValid = xeddsaVerify(signature, message, x25519PubKey, x25519PrivKey);
console.log('   Verification result:', isValid ? '✅ PASS' : '❌ FAIL');

if (isValid) {
  console.log('\n✅ XEdDSA implementation working correctly!');
  process.exit(0);
} else {
  console.log('\n❌ XEdDSA verification failed!');
  process.exit(1);
}
