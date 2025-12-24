# Cosmos EVM Gasless Transaction 완전 가이드

> 특정 컨트랙트 주소로 가는 트랜잭션의 가스비를 스폰서가 대신 지불하는 시스템

**최종 업데이트**: 2024년 12월
**상태**: ✅ 프로덕션 준비 완료

---

## 📑 목차

1. [빠른 시작](#-빠른-시작-3분-설정)
2. [작동 원리](#-작동-원리)
3. [설정 가이드](#-설정-가이드)
4. [코드 통합](#-코드-통합)
5. [사용 방법](#-사용-방법)
6. [테스트 결과](#-테스트-결과)
7. [트러블슈팅](#-트러블슈팅)

---

## 🚀 빠른 시작 (3분 설정)

### 핵심 개념

Gasless 트랜잭션은 `to` 주소(트랜잭션 목적지)를 기준으로 작동합니다:

```
사용자 트랜잭션
    ↓
to: 0xAa0000... (허용된 주소)
    ↓
✅ 스폰서가 가스비 지불 → 사용자는 무료
```

### Step 1: 주소 준비

**테스트용 고정 주소 (추천):**
```
0xAa00000000000000000000000000000000000000
```

**또는 새 주소 생성:**
```python
python3 << 'EOF'
from eth_account import Account
import secrets
acc = Account.from_key(secrets.token_hex(32))
print(f"Address: {acc.address}")
EOF
```

### Step 2: 스폰서 계정 생성

```bash
# 키 생성
evmd keys add sponsor

# 주소 확인
SPONSOR=$(evmd keys show sponsor -a)
echo "Sponsor: $SPONSOR"
```

### Step 3: Genesis 설정

`~/.evmd/config/genesis.json` 파일에 추가:

```json
{
  "app_state": {
    "gasless": {
      "params": {
        "enabled": true,
        "allowed_contracts": [
          "0xAa00000000000000000000000000000000000000"
        ],
        "default_sponsor": "cosmos1sponsoraddress...",
        "max_gas_per_tx": "500000",
        "max_subsidy_per_block": "10000000000000000000"
      }
    }
  }
}
```

**파라미터 설명:**
- `enabled`: gasless 기능 활성화 (true/false)
- `allowed_contracts`: 무료 허용 주소 목록 (EVM 주소 배열)
- `default_sponsor`: 가스비 지불할 계정 (Cosmos 주소)
- `max_gas_per_tx`: 트랜잭션당 최대 가스 한도
- `max_subsidy_per_block`: 블록당 최대 지원 금액 (wei 단위, "0" = 무제한)

### Step 4: 노드 시작

```bash
# Genesis 검증
evmd validate-genesis

# 노드 시작
evmd start
```

### Step 5: Gasless 트랜잭션 전송

```javascript
// ethers.js 예시
const tx = {
  to: "0xAa00000000000000000000000000000000000000",
  value: ethers.parseEther("0.1"),
  gasLimit: 100000
};

// 잔액 없어도 전송 가능!
const receipt = await wallet.sendTransaction(tx);
```

---

## 🏗️ 작동 원리

### 전체 아키텍처

```
┌──────────────────────────────────────────┐
│         사용자 (Metamask, web3)           │
└──────────────┬───────────────────────────┘
               │ to: 0xAa00... (허용된 주소)
               ↓
┌──────────────────────────────────────────┐
│           evmd 노드                       │
│                                          │
│  JSON-RPC (8545) → Mempool               │
│         ↓                                │
│  Ante Handler Chain                      │
│    ┌────────────────────────┐            │
│    │ GaslessDecorator       │            │
│    │ 1. to 주소 확인          │            │
│    │ 2. 허용 여부 체크         │            │
│    │ 3. 가스 한도 검증         │            │
│    │ 4. 스폰서 계정 차감        │            │
│    │ 5. Context에 정보 저장    │            │
│    └────────────────────────┘            │
│         ↓                                │
│             x/vm (EVM 실행)               │
└──────────────────────────────────────────┘
               ↓
┌──────────────────────────────────────────┐
│      x/gasless Module (KV Store)         │
│  - params (설정)                         │
│  - subsidy/{blockHeight} (사용량 추적)    │
└──────────────────────────────────────────┘
```

### 실행 흐름

#### 1. 초기화 (evmd 시작)
```
evmd start
  → app.go: NewExampleApp()
    → GaslessKeeper 생성
      → ModuleManager에 등록
        → Genesis 로드 (InitGenesis)
          → Ante Handler 등록
```

#### 2. Genesis 로딩
```go
// module.go:InitGenesis()
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data json.RawMessage) {
    var genesisState types.GenesisState
    cdc.MustUnmarshalJSON(data, &genesisState)

    // Genesis params를 KV Store에 저장
    am.keeper.SetParams(ctx, genesisState.Params)
}
```

#### 3. 트랜잭션 처리 (런타임)
```
트랜잭션 수신
  ↓
GaslessDecorator.AnteHandle()
  ↓
keeper.IsGaslessAllowed(to) → ✅/❌
  ↓
keeper.ValidateGasLimit(gas)
  ↓
keeper.CheckBlockSubsidyLimit(fee)
  ↓
keeper.ChargeSponsor(sponsor, fee)
  ↓
트랜잭션 계속 처리
```

### Keeper 메서드

```go
// IsGaslessAllowed - to 주소가 허용되는지 확인
func (k Keeper) IsGaslessAllowed(ctx sdk.Context, ethTo string) (bool, sdk.AccAddress, error) {
    params := k.GetParams(ctx)  // KV Store에서 params 읽기

    // allowed_contracts 확인
    for _, addr := range params.AllowedContracts {
        if addr == ethTo {
            sponsor, _ := sdk.AccAddressFromBech32(params.DefaultSponsor)
            return true, sponsor, nil
        }
    }
    return false, nil, nil
}

// ChargeSponsor - 스폰서 계정에서 fee 차감
func (k Keeper) ChargeSponsor(ctx sdk.Context, sponsor sdk.AccAddress, fee sdk.Coins) error {
    return k.bankKeeper.SendCoinsFromAccountToModule(ctx, sponsor, types.ModuleName, fee)
}
```

---

## ⚙️ 설정 가이드

### 방법 1: Genesis 파일 (신규 체인) ✅ 추천

#### Step 1: 스폰서 주소 생성
```bash
evmd keys add sponsor
SPONSOR=$(evmd keys show sponsor -a)
```

#### Step 2: To 주소 결정
- **테스트용**: `0xAa00000000000000000000000000000000000000`
- **실제 컨트랙트**: 배포된 컨트랙트 주소 사용
- **새 생성**: Python/Node.js로 생성 (위 참조)

#### Step 3: Genesis 수정
```bash
jq '.app_state.gasless = {
  "params": {
    "enabled": true,
    "allowed_contracts": ["0xAa00000000000000000000000000000000000000"],
    "default_sponsor": "'$SPONSOR'",
    "max_gas_per_tx": "500000",
    "max_subsidy_per_block": "10000000000000000000"
  }
}' ~/.evmd/config/genesis.json > temp.json && mv temp.json ~/.evmd/config/genesis.json
```

#### Step 4: 스폰서 계정 잔액 추가
```bash
# Genesis에 스폰서 계정 추가 (2000 STAKE)
evmd add-genesis-account $SPONSOR 2000000000000000000000stake
```

#### Step 5: 검증 및 시작
```bash
evmd validate-genesis
evmd start
```

### 방법 2: 거버넌스 제안 (실행 중인 체인)

#### 제안서 작성
`update-gasless-params.json`:
```json
{
  "title": "Update Gasless Parameters",
  "description": "Add new allowed contracts",
  "changes": [{
    "subspace": "gasless",
    "key": "Params",
    "value": {
      "enabled": true,
      "allowed_contracts": [
        "0xAa00000000000000000000000000000000000000",
        "0xBb11111111111111111111111111111111111111"
      ],
      "default_sponsor": "cosmos1newsponsor...",
      "max_gas_per_tx": "1000000",
      "max_subsidy_per_block": "20000000000000000000"
    }
  }],
  "deposit": "10000000stake"
}
```

#### 제안 제출 및 투표
```bash
# 제안 제출
evmd tx gov submit-proposal param-change update-gasless-params.json \
  --from validator \
  --chain-id evmos_9000-1 \
  --gas auto

# 투표
evmd tx gov vote 1 yes --from validator
```

### 설정 확인

```bash
# Genesis에서 확인
cat ~/.evmd/config/genesis.json | jq '.app_state.gasless'

# 실행 중인 체인에서 확인
evmd query gasless params

# 스폰서 잔액 확인
evmd query bank balances $(evmd keys show sponsor -a)
```

---

## 💻 코드 통합

### 1. app.go 수정

#### Import 추가
```go
import (
    gaslesskeeper "github.com/cosmos/evm/x/gasless/keeper"
    gaslesstypes "github.com/cosmos/evm/x/gasless/types"
    "github.com/cosmos/evm/ante/gasless"
)
```

#### EVMD 구조체에 Keeper 추가
```go
type EVMD struct {
    *baseapp.BaseApp
    // ... 기존 필드들 ...
    GaslessKeeper gaslesskeeper.Keeper
}
```

#### StoreKey 추가
```go
keys := storetypes.NewKVStoreKeys(
    // ... 기존 store keys ...
    gaslesstypes.StoreKey,  // ← 추가
)
```

#### Keeper 초기화
```go
app.GaslessKeeper = gaslesskeeper.NewKeeper(
    appCodec,
    keys[gaslesstypes.StoreKey],
    app.BankKeeper,
    app.AccountKeeper,
    app.EVMKeeper,
)
```

#### ModuleManager 등록
```go
app.ModuleManager = module.NewManager(
    // ... 기존 모듈들 ...
    gasless.NewAppModule(app.GaslessKeeper),
)
```

#### Ante Handler 등록
```go
func (app *EVMD) setAnteHandler(txConfig client.TxConfig, maxGasWanted uint64) {
    options := evmante.HandlerOptions{
        // ... 기존 옵션들 ...
        GaslessKeeper: &app.GaslessKeeper,  // ← 추가
    }

    baseHandler := evmante.NewAnteHandler(options)
    app.SetAnteHandler(baseHandler)
}
```

### 2. ante/evm.go 수정

```go
// newMonoEVMAnteHandler에 gasless decorator 추가
func newMonoEVMAnteHandler(ctx sdk.Context, options HandlerOptions) sdk.AnteHandler {
    evmParams := options.EvmKeeper.GetParams(ctx)
    feemarketParams := options.FeeMarketKeeper.GetParams(ctx)

    decorators := []sdk.AnteDecorator{}

    // Gasless decorator를 맨 앞에 추가
    if options.GaslessKeeper != nil {
        decorators = append(decorators, gasless.NewGaslessDecorator(options.GaslessKeeper))
    }

    // 기존 EVM decorator
    decorators = append(decorators,
        evmante.NewEVMMonoDecorator(...),
        NewTxListenerDecorator(options.PendingTxListener),
    )

    return sdk.ChainAnteDecorators(decorators...)
}
```

### 3. ante/interfaces/evm.go 추가

```go
// GaslessKeeper 인터페이스 정의
type GaslessKeeper interface {
    IsGaslessAllowed(ctx sdk.Context, ethTo string) (bool, sdk.AccAddress, error)
    ChargeSponsor(ctx sdk.Context, sponsor sdk.AccAddress, fee sdk.Coins) error
    ValidateGasLimit(ctx sdk.Context, gas uint64) error
    CheckBlockSubsidyLimit(ctx sdk.Context, newFee sdk.Coins) error
}
```

---

## 📤 사용 방법

### JavaScript (ethers.js)

```javascript
const { ethers } = require('ethers');

const provider = new ethers.JsonRpcProvider('http://localhost:8545');
const wallet = new ethers.Wallet('0xYourPrivateKey', provider);

// Gasless 트랜잭션 전송
async function sendGaslessTx() {
  const tx = {
    to: "0xAa00000000000000000000000000000000000000",
    value: ethers.parseEther("0.1"),
    gasLimit: 100000
  };

  const receipt = await wallet.sendTransaction(tx);
  console.log('Hash:', receipt.hash);

  await receipt.wait();
  console.log('✅ Transaction confirmed!');
}
```

### JavaScript (web3.js)

```javascript
const Web3 = require('web3');
const web3 = new Web3('http://localhost:8545');

const account = web3.eth.accounts.privateKeyToAccount('0xYourPrivateKey');
web3.eth.accounts.wallet.add(account);

const tx = {
  from: account.address,
  to: '0xAa00000000000000000000000000000000000000',
  gas: 100000,
  value: '0'
};

const receipt = await web3.eth.sendTransaction(tx);
console.log('✅ Transaction:', receipt.transactionHash);
```

### Go (cast)

```bash
cd ~/Documents/cosmos-evm

# Cast 사용
cast send \
  --rpc-url http://localhost:8545 \
  --private-key YOUR_PRIVATE_KEY \
  --gas-limit 21000 \
  --value 0 \
  0xAa00000000000000000000000000000000000000
```

### Metamask

1. 네트워크 추가:
   - RPC URL: `http://localhost:8545`
   - Chain ID: `262144`
   - Currency: `STAKE`

2. 트랜잭션 전송:
   - To: `0xAa00000000000000000000000000000000000000`
   - Amount: 아무 값
   - ✅ 잔액 없어도 성공!

---

## ✅ 테스트 결과

### Ante Decorator 테스트 (4/4 통과)

```bash
$ go test ./ante/gasless/... -v

✅ TestGaslessDecorator_ChargesSponsorWhenAllowed
✅ TestGaslessDecorator_NoopWhenNotAllowed
✅ TestGaslessDecorator_ExceedsGasLimit
✅ TestGaslessDecorator_ExceedsBlockSubsidyLimit

PASS
ok  	github.com/cosmos/evm/ante/gasless	1.078s
```

### 테스트 커버리지

| 시나리오 | 결과 |
|---------|------|
| 허용된 주소로 gasless tx | ✅ 성공 |
| 스폰서 잔액 차감 | ✅ 성공 |
| GaslessInfo context 전파 | ✅ 성공 |
| 허용 안된 주소 | ✅ 일반 tx 처리 |
| 가스 한도 초과 | ✅ 거부 |
| 블록 지원 한도 초과 | ✅ 거부 |

### 실제 노드 테스트

```bash
# 노드 시작 로그
[1:38PM] INF GASLESS ANTE HANDLER CALLED!!!
[1:38PM] INF Gasless: checking address to=0xaa00...
[1:38PM] INF Gasless: APPROVED! sponsor=cosmos1sl8y... to=0xaa00...

# 트랜잭션 결과
effectiveGasPrice: 0  ← 가스비 0!
status: 1 (success)
```

---

## 🛠️ 트러블슈팅

### "invalid sponsor address" 에러

**원인**: Bech32 주소 형식 오류

**해결**:
```bash
# evmd keys로 생성한 주소 사용
evmd keys show sponsor -a
```

### "gasless tx exceeds max gas limit" 에러

**원인**: 트랜잭션 가스가 `max_gas_per_tx`를 초과

**해결**: Genesis 또는 거버넌스로 `max_gas_per_tx` 증가

### "gasless subsidy limit exceeded" 에러

**원인**: 블록당 지원 한도 초과

**해결**:
- `max_subsidy_per_block` 증가
- 다음 블록 대기
- "0"으로 설정하여 무제한 허용

### 스폰서 잔액 부족

**확인**:
```bash
evmd query bank balances $(evmd keys show sponsor -a)
```

**해결**:
```bash
# 스폰서에게 토큰 전송
evmd tx bank send source $SPONSOR 1000000000000000000stake \
  --from validator \
  --gas auto
```

### Genesis 검증 실패

```bash
# 구문 검증
jq empty ~/.evmd/config/genesis.json

# gasless 섹션 확인
jq '.app_state.gasless' ~/.evmd/config/genesis.json

# 스키마 검증
evmd validate-genesis
```

### Gasless가 작동하지 않음

**체크리스트**:
1. ✅ app.go에 Keeper 등록 확인
```bash
grep -n "GaslessKeeper" evmd/app.go
```

2. ✅ Ante Handler 등록 확인
```bash
grep -n "gasless.NewGaslessDecorator" ante/evm.go
```

3. ✅ Genesis 설정 확인
```bash
cat ~/.evmd/config/genesis.json | jq '.app_state.gasless'
```

4. ✅ 로그 확인
```bash
evmd start --log_level debug 2>&1 | grep gasless
```

---

## ⚠️ 프로덕션 주의사항

### 보안

1. **allowed_contracts 관리**
   - 신뢰할 수 있는 주소만 등록
   - 정기적인 감사 및 검토
   - 악의적 컨트랙트 방지

2. **스폰서 계정 보안**
   - 개인키 안전하게 백업
   - 멀티시그 고려
   - 정기적인 잔액 모니터링

3. **한도 설정**
   - `max_gas_per_tx`: 단일 tx 남용 방지
   - `max_subsidy_per_block`: 블록당 DoS 방지
   - 모니터링 알림 설정

### 모니터링

```bash
# Prometheus/Grafana 메트릭
- 스폰서 잔액
- 블록당 gasless tx 수
- 블록당 지원 금액
- 실패한 gasless tx 비율
```

### 운영 가이드

1. **스폰서 잔액 자동 충전**
   - 임계값 알림 설정
   - 자동 충전 스크립트

2. **로그 모니터링**
   - Gasless tx 패턴 분석
   - 이상 패턴 감지

3. **정기 감사**
   - allowed_contracts 검토
   - 사용 패턴 분석
   - 비용 최적화

---

## 📚 파일 구조

```
cosmos-evm/
├── ante/
│   ├── gasless/
│   │   ├── decorator.go          # Ante handler 구현
│   │   └── decorator_test.go     # 테스트
│   ├── evm.go                     # Ante chain 설정
│   ├── ante.go                    # Handler options
│   └── interfaces/
│       └── evm.go                 # GaslessKeeper 인터페이스
├── x/gasless/
│   ├── module.go                  # AppModule 구현
│   ├── keeper/
│   │   └── keeper.go              # Keeper 로직
│   └── types/
│       ├── params.go              # 파라미터 정의
│       └── genesis.go             # Genesis 처리
├── evmd/
│   ├── app.go                     # 앱 통합 (line 812)
│   └── cmd/gasless-test/
│       └── main.go                # 테스트 도구
└── example-genesis-gasless.json   # Genesis 예제
```

---

## 🎯 FAQ

### Q1: To 주소는 컨트랙트여야 하나요?
**A**: 아니요! 일반 EOA 주소도 가능합니다.

### Q2: 여러 개의 to 주소를 허용할 수 있나요?
**A**: 네! `allowed_contracts` 배열에 여러 주소 추가 가능합니다.

### Q3: 사용자의 잔액이 0이어도 되나요?
**A**: 네! 스폰서가 가스비를 대신 지불합니다.

### Q4: 허용되지 않은 주소로 보내면?
**A**: 일반 EVM 트랜잭션으로 처리됩니다 (사용자가 가스비 지불).

### Q5: 가스비는 얼마나 듭니까?
**A**: EIP-1559 dynamic fee로 계산됩니다. 현재 설정에서 약 1-1.1 gwei입니다.

### Q6: effectiveGasPrice가 0이 되는 원리는?
**A**: Gasless decorator가 ante chain에서 먼저 실행되어 스폰서가 가스비를 지불하므로, 사용자는 0 gas price로 트랜잭션을 실행할 수 있습니다.

---

## 📖 추가 리소스

### 코드 참조
- Ante Decorator: `ante/gasless/decorator.go:43-131`
- Keeper 구현: `x/gasless/keeper/keeper.go:76-163`
- 테스트: `ante/gasless/decorator_test.go`

### 관련 문서
- Cosmos SDK Ante Handler: https://docs.cosmos.network/
- EIP-1559: https://eips.ethereum.org/EIPS/eip-1559
- Go Ethereum: https://geth.ethereum.org/

---

**마지막 업데이트**: 2024년 12월 24일
**라이센스**: Apache 2.0
**상태**: ✅ 프로덕션 준비 완료
