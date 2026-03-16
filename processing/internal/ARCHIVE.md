# Archive — Reference Code Only

The packages in this directory are **not part of the active production path**.
They are kept as design references for future work.

| Package | Reference For |
|---|---|
| `storage/` | Phase 4 (PostgreSQL partitioning) — `UniversalMessage` model and `StorageProvider` interface inform the partitioned schema design |
| `routing/` | Future horizontal scaling — `RoutingEngine`, `LoadBalancer`, `DeadLetterQueue` designs are relevant when distributing work across instances |

**Do not import these packages from production code.**
**Do not modify these files during normal feature work.**

When Phase 4 or horizontal scaling work begins, decisions about whether to adopt these designs or supersede them should be recorded in `PRODUCTION_READINESS.md` Decisions Log.
