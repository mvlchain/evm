# Matchboard Proposer Observability

`matchboard.proposer-abci.enable=true`가 활성화된 노드에서 다음 카운터를 사용해 운영 상태를 추적한다.

- `matchboard_proposal_reject_total{reason=...}`
- `matchboard_finalize_block_rollback_total{reason=...}`

## PromQL Quick Checks

```promql
sum by (reason) (increase(matchboard_proposal_reject_total[5m]))
```

```promql
sum by (reason) (increase(matchboard_finalize_block_rollback_total[5m]))
```

```promql
sum(increase(matchboard_proposal_reject_total{reason="batch_hash_mismatch"}[15m]))
```

## Grafana Panel Snippet (Timeseries)

```json
{
  "type": "timeseries",
  "title": "Matchboard Proposal Rejects (5m)",
  "targets": [
    {
      "expr": "sum by (reason) (increase(matchboard_proposal_reject_total[5m]))",
      "legendFormat": "{{reason}}",
      "refId": "A"
    }
  ]
}
```

```json
{
  "type": "timeseries",
  "title": "Matchboard Finalize Rollbacks (5m)",
  "targets": [
    {
      "expr": "sum by (reason) (increase(matchboard_finalize_block_rollback_total[5m]))",
      "legendFormat": "{{reason}}",
      "refId": "A"
    }
  ]
}
```

## Alert Suggestion

- `batch_hash_mismatch`가 5분 내 1회 이상 발생하면 경고:

```promql
sum(increase(matchboard_proposal_reject_total{reason="batch_hash_mismatch"}[5m])) > 0
```
