# TypeScript Match E2E

이 폴더는 실행 중인 노드/매치보드 서버를 대상으로 매칭 플로우를 검증하는 TypeScript E2E 스크립트를 포함합니다.

## 준비

1. 체인 노드 실행 (CometBFT RPC + in-process `matchboard`)
2. 이 폴더에서 의존성 설치

권장 실행:

```bash
cd /Users/triggy/evm
./local_node.sh -y
```

참고: `evmd start`에 `--matchboard.enable=true`가 전달되면 `matchboard`는 노드 프로세스 안에서 같이 기동된다.

```bash
cd typescript
npm install
```

## 실행

기본값:
- `NODE_RPC_URL=http://127.0.0.1:26657`
- `MATCHBOARD_URL=http://127.0.0.1:8080`
- `MATCH_CHAIN_ID=9001`
- `MATCHBOARD_TOKEN_ALICE=token-alice`
- `MATCHBOARD_TOKEN_BOB=token-bob`
- `MATCHBOARD_PRINCIPAL_ALICE=0xC6Fe5D33615a1C52c08018c47E8Bc53646A0E101`
- `MATCHBOARD_PRINCIPAL_BOB=0x963EBDf2e1f8DB8707D05FC75bfeFFBa1B5BaC17`
- `MATCHBOARD_ALICE_PRIVATE_KEY=0x88cbead91aee890d27bf06e003ade3d4e952427e88f88d31d61d3ef5e5d54305`
- `MATCHBOARD_BOB_PRIVATE_KEY=0x741de4f8988ea941d3ff0287911ca4074e62b7d45c991a51186455366f10b544`
- `MATCH_EXPECT_ONCHAIN_REPLAY=1` (기본값)

테스트는 deterministic protobuf sign-doc 해시(`intent/response/finalize/certificate`)를 계산한 뒤 `ethers.js` secp256k1 서명으로 오프체인 게시 + 온체인 제출(`submitMatchCertificate`)까지 수행한다.
기본적으로 상세 로그를 출력하며, 로그를 끄려면 `MATCH_E2E_VERBOSE=0`을 설정한다.

```bash
cd typescript
npm run test:match
```

## Docker 기반 Gossip E2E

`evmd` 2개 컨테이너를 Docker Compose로 올린 뒤(in-process matchboard 포함),
`intent/response/finalize` gossip 전파를 검증한다.

```bash
cd typescript
npm run test:gossip:docker
```

기본 동작:
- compose 파일: `typescript/docker-compose.matchboard-gossip.yml`
- node-a matchboard: `http://127.0.0.1:28080`
- node-b matchboard: `http://127.0.0.1:28081`
- 테스트 완료 후 컨테이너 자동 정리(`docker compose down -v`)

디버깅용으로 컨테이너를 남기려면:

```bash
cd typescript
MATCH_DOCKER_KEEP=1 npm run test:gossip:docker
```

## 타입체크

```bash
cd typescript
npm run typecheck
```
