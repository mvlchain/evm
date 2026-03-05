# TypeScript Match E2E

이 폴더는 실행 중인 노드/매치보드 서버를 대상으로 매칭 플로우를 검증하는 TypeScript E2E 스크립트를 포함합니다.

## 준비

1. 체인 노드 실행 (CometBFT RPC)
2. `matchboard` HTTP 서버 실행
3. 이 폴더에서 의존성 설치

`matchboard` 서버 실행 예시:

```bash
cd /Users/triggy/evm
go run ./server/matchboard/cmd/matchboardd
```

```bash
cd typescript
npm install
```

## 실행

기본값:
- `NODE_RPC_URL=http://127.0.0.1:26657`
- `MATCHBOARD_URL=http://127.0.0.1:8080`
- `MATCHBOARD_TOKEN_ALICE=token-alice`
- `MATCHBOARD_TOKEN_BOB=token-bob`
- `MATCHBOARD_PRINCIPAL_ALICE=alice`
- `MATCHBOARD_PRINCIPAL_BOB=bob`
- `MATCHBOARD_ALICE_PRIVATE_KEY=0x88cbead91aee890d27bf06e003ade3d4e952427e88f88d31d61d3ef5e5d54305`
- `MATCHBOARD_BOB_PRIVATE_KEY=0x741de4f8988ea941d3ff0287911ca4074e62b7d45c991a51186455366f10b544`

테스트는 `ethers.js`로 `intent_sign_hash`, `response_sign_hash`, `finalize_sign_hash`를 각각 secp256k1 서명한다.
기본적으로 상세 로그를 출력하며, 로그를 끄려면 `MATCH_E2E_VERBOSE=0`을 설정한다.

EVM 주소 기반 검증까지 켜려면 principal도 주소로 맞춰 실행한다:

```bash
export MATCHBOARD_PRINCIPAL_ALICE=0xC6Fe5D33615a1C52c08018c47E8Bc53646A0E101
export MATCHBOARD_PRINCIPAL_BOB=0x963EBDf2e1f8DB8707D05FC75bfeFFBa1B5BaC17
```

```bash
cd typescript
npm run test:match
```

## 타입체크

```bash
cd typescript
npm run typecheck
```
