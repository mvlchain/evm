# Core Matching Engine - 사용 가이드

## 🚀 아키텍처 개요

Hyperliquid 스타일의 빠른 UX를 위한 Core-레벨 매칭 엔진입니다.

### 이전 vs 현재

**이전 (EVM 레벨)**:
```
Rider Tx (2s) → Driver Tx (2s) → EVM 매칭 로직 실행
총 소요 시간: 4+ 초
```

**현재 (Core 레벨)**:
```
Rider Tx → Pending Pool (즉시)
Driver Tx → Driver Pool (즉시)
BeginBlocker → 자동 매칭 (블록 타임)
총 소요 시간: <1초 ⚡
```

## 🏗️ 컴포넌트

### 1. Core Module (x/ridehail)
- **PendingRequest Pool**: 매칭 대기 중인 라이드 요청
- **DriverCommit Pool**: 드라이버 커밋 저장소
- **ProcessMatching()**: BeginBlocker에서 자동 실행되는 매칭 엔진
- **Cosmos Events**: 실시간 이벤트 발생 (`ridehail_match`)

### 2. Thin Proxy (Precompile)
- **CreateRequest**: EVM → Core MsgServer 호출
- **AcceptCommit**: EVM → Core MsgServer 호출
- EVM 호환성 유지하면서 Core로 위임

### 3. Event Listener
- WebSocket을 통한 실시간 이벤트 감지
- Cosmos SDK 네이티브 이벤트 구독
- 드라이버가 매칭을 즉시 감지

## 📦 설치 및 빌드

```bash
# 1. 빌드
make build

# 2. 노드 시작 (터미널 1)
./local_node.sh

# 3. 의존성 설치 (이미 완료)
cd client/ridehail
npm install
```

## 🧪 테스트 방법

### 방법 1: 매칭 테스트 + 이벤트 리스너 (권장)

**터미널 1**: 노드 실행
```bash
./local_node.sh
```

**터미널 2**: 이벤트 리스너 시작
```bash
cd client/ridehail
npm run listen
```

**터미널 3**: 매칭 테스트 실행
```bash
cd client/ridehail
npm run test_matching
```

### 방법 2: 매칭 테스트만 실행

```bash
cd client/ridehail
npm run test_matching
```

## 📊 테스트 시나리오

`test_core_matching.ts`는 다음을 자동으로 실행합니다:

### Step 1: Rider 요청 생성
```typescript
// EVM에서 createRequest() 호출
// → Precompile이 Core Keeper.CreateRequest()로 프록시
// → PendingRequest Pool에 저장
// → ridehail_request_created 이벤트 발생
```

### Step 2: Driver 커밋 제출
```typescript
// EVM에서 acceptCommit() 호출
// → Precompile이 Core Keeper.SubmitDriverCommit()로 프록시
// → DriverCommit Pool에 저장
// → driver_commit_submitted 이벤트 발생
```

### Step 3: BeginBlocker 자동 매칭
```go
// 다음 블록 시작 시 자동 실행:
func (k Keeper) ProcessMatching(ctx sdk.Context) error {
    // 1. 모든 PendingRequest 조회
    // 2. 각 요청에 대한 DriverCommit 조회
    // 3. 최적 드라이버 선택 (가장 낮은 ETA)
    // 4. Session 생성
    // 5. ridehail_match 이벤트 발생
}
```

### Step 4: 결과 확인
- 블록 번호로 성능 측정
- 이벤트 로그 확인
- Session 생성 확인

## 📡 이벤트 리스너

`event_listener.ts`는 다음 이벤트를 실시간으로 감지합니다:

### 1. ridehail_request_created
```typescript
{
  request_id: "1",
  rider: "cosmos1...",
  cell_topic: "0x1234...",
  max_eta: "300",
  expires_at: "1234567890"
}
```

### 2. driver_commit_submitted
```typescript
{
  request_id: "1",
  driver: "cosmos1...",
  eta: "240"
}
```

### 3. ridehail_match (🎉 매칭 성공!)
```typescript
{
  request_id: "1",
  session_id: "1",
  rider: "cosmos1...",
  driver: "cosmos1..."
}
```

### 4. ridehail_request_expired
```typescript
{
  request_id: "1"
}
```

## 🔍 로그 확인

### 노드 로그에서 확인할 것:

**Precompile (Thin Proxy)**:
```
[RideHail] ========== CreateRequest (Thin Proxy) ==========
[RideHail] Calling core Keeper.CreateRequest...
[RideHail] ✅ Core request created! RequestId: 1

[RideHail] ========== AcceptCommit (Thin Proxy) ==========
[RideHail] Calling core Keeper.SubmitDriverCommit...
[RideHail] ✅ Driver commit submitted to core!
```

**Core Matching Engine**:
```
[ridehail] Ride request created, request_id=1, rider=cosmos1...
[ridehail] Driver commit submitted, request_id=1, driver=cosmos1...
[ridehail] Matched rider with driver, request_id=1, session_id=1, rider=cosmos1..., driver=cosmos1..., eta=240
```

**Cosmos Events**:
```
Event: ridehail_request_created
  - request_id: 1
  - rider: cosmos1...

Event: driver_commit_submitted
  - request_id: 1
  - driver: cosmos1...

Event: ridehail_match
  - request_id: 1
  - session_id: 1
  - rider: cosmos1...
  - driver: cosmos1...
```

## ⚡ 성능 분석

테스트 스크립트가 자동으로 다음을 출력합니다:

```
📈 Performance Analysis:
   Block of request creation: 100
   Block of driver commit:    100
   Block of matching:         101
   Total blocks elapsed:      1

⚡ Hyperliquid-style UX: Sub-second matching!
   (Only limited by block time, not transaction processing)
```

## 🎯 매칭 알고리즘

`SelectBestDriver()` 함수:

```go
func (k Keeper) SelectBestDriver(
    ctx sdk.Context,
    req *types.PendingRequest,
    commits []*types.DriverCommit
) *types.DriverCommit {
    var bestDriver *types.DriverCommit

    for _, commit := range commits {
        // 1. MaxDriverEta 체크
        if commit.Eta > req.MaxDriverEta {
            continue
        }

        // 2. 커밋 유효성 체크
        if len(commit.DriverCommit) != 32 {
            continue
        }

        // 3. 가장 낮은 ETA 선택
        if bestDriver == nil || commit.Eta < bestDriver.Eta {
            bestDriver = commit
        }
    }

    return bestDriver
}
```

## 🔐 다음 단계: 암호화 메시징

매칭 후:
1. Rider와 Driver가 `ridehail_match` 이벤트 감지
2. Double Ratchet를 통한 End-to-End 암호화 세션 시작
3. 실시간 위치 공유 및 메시징
4. Pickup/Dropoff 위치 reveal

## 📝 API Reference

### Precompile Methods

**createRequest()**
```solidity
function createRequest(
    bytes32 cellTopic,
    bytes32 regionTopic,
    bytes32 paramsHash,
    bytes32 pickupCommit,
    bytes32 dropoffCommit,
    uint32 maxDriverEta,
    uint64 ttl
) payable returns (uint256 requestId)
```

**acceptCommit()**
```solidity
function acceptCommit(
    uint256 requestId,
    bytes32 commitHash,
    uint64 eta
) payable returns ()
```

### Core Keeper Methods

**CreateRequest()**
```go
func (k Keeper) CreateRequest(
    ctx sdk.Context,
    rider string,
    cellTopic, regionTopic, paramsHash, pickupCommit, dropoffCommit []byte,
    maxDriverEta uint32,
    ttl uint32,
    deposit string
) (uint64, error)
```

**SubmitDriverCommit()**
```go
func (k Keeper) SubmitDriverCommit(
    ctx sdk.Context,
    driver string,
    requestId uint64,
    driverCommit []byte,
    eta uint32
) error
```

**ProcessMatching()**
```go
func (k Keeper) ProcessMatching(ctx sdk.Context) error
```

## 🐛 트러블슈팅

### WebSocket 연결 실패
```bash
# Tendermint RPC가 실행 중인지 확인
curl http://localhost:26657/status

# 노드가 실행 중인지 확인
ps aux | grep evmd
```

### 매칭이 안 됨
- BeginBlocker 로그 확인: `ProcessMatching` 호출되는지
- PendingRequest Pool에 요청이 있는지
- DriverCommit Pool에 커밋이 있는지
- ETA가 MaxDriverEta보다 작은지

### 이벤트가 안 들림
- WebSocket 연결 확인 (ws://localhost:26657/websocket)
- 노드 로그에서 이벤트 발생 확인
- Base64 디코딩 확인

## 💡 핵심 포인트

✅ **Thin Proxy Pattern**: Precompile은 단순히 Core로 위임만
✅ **Core-level Matching**: 비즈니스 로직은 모두 Core에서
✅ **BeginBlocker**: 매 블록마다 자동으로 매칭 처리
✅ **Event-driven**: Cosmos SDK 네이티브 이벤트로 실시간 감지
✅ **Sub-second UX**: Hyperliquid 스타일의 빠른 사용자 경험

🎉 **성공!** 이제 Core-레벨에서 매칭이 자동으로 처리됩니다!
