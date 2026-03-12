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
Docker 구성은 `--grpc-only` 기반이라 HTTP internal gossip fallback 경로를 검증한다.

```bash
cd typescript
npm run test:gossip:docker
```

기본 동작:
- compose 파일: `typescript/docker-compose.matchboard-gossip.yml`
- node-a matchboard: `http://127.0.0.1:28080`
- node-b matchboard: `http://127.0.0.1:28081`
- `MATCHBOARD_GOSSIP_PEERS` 없이 `--p2p.persistent_peers` 값에서 peer host를 유도해 gossip relay
- 테스트 완료 후 컨테이너 자동 정리(`docker compose down -v`)

속도 최적화(이미지 사전 빌드 후 재사용):

```bash
docker build -t evmd-gossip:ci -f typescript/Dockerfile.evmd-local .
cd typescript
MATCH_DOCKER_BUILD=0 EVMD_GOSSIP_IMAGE=evmd-gossip:ci npm run test:gossip:docker
```

디버깅용으로 컨테이너를 남기려면:

```bash
cd typescript
MATCH_DOCKER_KEEP=1 npm run test:gossip:docker
```

## Docker 기반 Full Flow E2E

`request -> response -> finalize -> proposer build/commit` 전체 플로우를
2개 노드에서 검증한다.

```bash
cd typescript
npm run test:flow:docker
```

주요 검증 항목:
- SSE subscription + intent gossip 수신
- inbox 전파(request/response/finalize)
- matcher candidates / proposer pending matches 가시성
- `require_certificate=true` 빌드 실패(인증서 없을 때 409)
- proposer commit 원자 롤백(유효 + 없는 match_id 동시 제출 시 409)
- 정상 commit 후 proposer pending 비움

`MATCH_DOCKER_BUILD=0`, `EVMD_GOSSIP_IMAGE=...`, `MATCH_DOCKER_KEEP=1` 환경변수는
`test:gossip:docker`와 동일하게 사용할 수 있다.


## 4노드 하드웨어 실측 벤치

4개 노드(`evmdhw0..3`)를 docker로 띄운 상태에서, target TPS별(기본 `50,100,500,1000`)로
`intent -> response -> finalize` 플로우를 부하 생성하고 `docker stats`를 샘플링해
CPU/RAM 추천치를 계산한다.

```bash
cd typescript
npm run bench:hardware:up
npm run bench:hardware
```

정리:

```bash
cd typescript
npm run bench:hardware:down
```

옵션 예시:

```bash
# 50/100/500/1000, 각 20초씩
npm run bench:hardware -- --targets=50,100,500,1000 --duration=20

# 컨테이너/엔드포인트 커스텀
MATCH_HW_CONTAINERS=evmdhw0,evmdhw1,evmdhw2,evmdhw3 \
MATCH_HW_BASE_URLS=http://127.0.0.1:28080,http://127.0.0.1:28081,http://127.0.0.1:28082,http://127.0.0.1:28083 \
npm run bench:hardware -- --duration=30
```

출력:
- 콘솔 Markdown 표 (target TPS / observed flow/s / CPU p95 / RAM p95 / 추천 vCPU/RAM)
- JSON 리포트 파일: `typescript/hardware-bench-<timestamp>.json`

참고:
- 기본 토큰/프린시펄은 `local_node.sh` 기본값(`token-alice`, `token-bob`, 기본 0x 주소)을 사용한다.
- 프린시펄/키를 바꿨다면 아래 환경변수도 함께 맞춰야 한다.
  - `MATCHBOARD_PRINCIPAL_ALICE`, `MATCHBOARD_PRINCIPAL_BOB`
  - `MATCHBOARD_ALICE_PRIVATE_KEY`, `MATCHBOARD_BOB_PRIVATE_KEY`

## 4노드 EVM 트랜잭션 하드웨어 벤치 (권장)

매치보드 HTTP가 아니라 **일반 EVM raw tx(`eth_sendRawTransaction`)**를 생성해
target TPS별(기본 `50,100,500,1000`) 성능을 측정한다.

```bash
cd typescript
npm run bench:evm:up
npm run bench:evm -- --targets=50,100,500,1000 --duration=30 --settle=10 --sender-count=128
```

정리:

```bash
cd typescript
npm run bench:evm:down
```

옵션/환경변수:
- `--targets`, `--duration`, `--settle`, `--workers`, `--sample-ms`, `--output`
- `--sender-count` (송신 지갑 개수, 기본 32)
- `MATCH_EVM_RPC_URLS` (기본: `http://127.0.0.1:28545,28555,28565,28575`)
- `MATCH_EVM_CONTAINERS` (기본: `evmevm0,evmevm1,evmevm2,evmevm3`)
- `MATCH_EVM_FUNDER_PRIVATE_KEY`, `MATCH_EVM_RECIPIENT`
- `MATCH_EVM_FUND_WEI` (sender 지갑당 초기 펀딩 금액)
- `MATCH_EVM_COMMIT_TIMEOUT` (기본 `500ms`, 블록 생성 속도 튜닝)
- `MATCH_EVM_EMPTY_BLOCK_INTERVAL` (기본 `0s`)
- `MATCH_EVM_SKIP_TIMEOUT_COMMIT` (기본 `true`)
- `MATCH_EVM_TIMEOUT_PROPOSE` / `MATCH_EVM_TIMEOUT_PREVOTE` / `MATCH_EVM_TIMEOUT_PRECOMMIT`

참고:
- 높은 TPS(예: 500~1000)에서는 single wallet nonce 직렬화 병목이 생기므로 `--sender-count`를 충분히 크게 두는 것을 권장.
- 예: `--sender-count=1000` (환경에 따라 초기 펀딩 시간이 길어질 수 있음)
- 스크립트는 기본적으로 **gas bump로 교체(replacement)** 하지 않고, sender별 이전 nonce가 체인에서 소진될 때까지 기다린 뒤 다음 tx를 전송한다.

출력:
- 콘솔 Markdown 표 (target TPS / sent TPS / included TPS / fail% / CPU p95 / RAM p95 / 추천 vCPU/RAM)
- JSON 리포트: `typescript/evm-hardware-bench-<timestamp>.json`

## 타입체크

```bash
cd typescript
npm run typecheck
```
