# Phase 1 — Các Câu Hỏi Cần Confirm Trước Khi Implementation

> **Last Updated:** 2026-06-26

## Mục Đích

Liệt kê tất cả các quyết định cần sếp xác nhận trước khi bắt đầu Phase 1 (Foundation). Các câu hỏi dựa trên tài liệu kiến trúc hiện tại (`docs/`, `docs/adr/`, `docs/standards/`).

---

## 1. Go Module & Repository Setup

| # | Câu hỏi | Gợi ý / Lựa chọn | Doc tham chiếu |
|---|---------|-------------------|----------------|
| 1.1 | **Module path** là gì? | `github.com/org/spatial-server` hay tên khác? | `docs/standards/protobuf-convention.md` nói `spatialserver.v1` |
| 1.2 | **Go version** target? | Hiện tại Go 1.22+ (slog ổn định). Có thể dùng 1.23 không? | Go 1.21+ có `log/slog` |
| 1.3 | **Có cần** `tools.go` để pin tool versions (koanf, protoc, golangci-lint)? | Nên có để CI reproducible |
| 1.4 | **Có cần** `buf` (buf.build) để quản lý protobuf hay dùng `protoc` thuần? | `buf` có lint, breaking change detection, generation |

## 2. Directory Structure

Cấu trúc hiện tại trong `docs/architecture/repository-structure.md`:

```
spatial-server/
├── apps/gateway/
├── apps/room-service/
├── apps/game-server/
├── pkg/   (17 packages: auth, aoi, cluster, config, entity, game, gateway, logging, metrics, protocol, room, rpc, session, space, storage, zone)
├── internal/types/
├── internal/utils/
├── proto/ (common.proto, gateway.proto, room_service.proto, game_server.proto)
├── gen/
├── configs/
├── deploy/
├── infra/
├── scripts/
├── test/
├── docs/
├── benchmarks/
└── .github/
```

| # | Câu hỏi | Gợi ý / Lựa chọn |
|---|---------|-------------------|
| 2.1 | **Tổng thể cấu trúc** có OK không? Cần thêm/bớt thư mục nào? | — |
| 2.2 | `gen/` nên là **thư mục riêng** (như docs) hay nằm trong `proto/gen/`? | `proto/gen/` gọn hơn |
| 2.3 | `pkg/game/` và `pkg/gateway/` và `pkg/room/` có nên đặt **dưới `apps/`** (gần với binary) hay ở `pkg/`? | Hiện tại ở `pkg/` để các service khác có thể reuse. Nhưng nếu chỉ một binary dùng thì nên để gần |
| 2.4 | `pkg/cluster/` có thực sự cần ngay từ Phase 1? Hay chỉ tạo khi có Room Service? | Cluster discovery cần khi có >1 service |
| 2.5 | `pkg/session/` có cần Phase 1 không? | Có thể đợi Phase Gateway |
| 2.6 | `pkg/space/` khác `pkg/zone/` thế nào? Có nên gộp? | Space là runtime-level, Zone là grid cell. Có thể nhập vào `pkg/zone/` |

## 3. Go Module Design

| # | Câu hỏi | Gợi ý / Lựa chọn | Doc tham chiếu |
|---|---------|-------------------|----------------|
| 3.1 | **Module splitting**: 1 module (mono) hay multi-module? | Mono-module đơn giản hơn. Multi-module cho phép version hóa riêng từng package |
| 3.2 | Nếu multi-module, `gen/` (protobuf code) có module riêng? | `proto/gen/` riêng để các service import |
| 3.3 | **Package naming**: `pkg/entity` — file chính là `entity.go` hay `types.go`? | `entity.go` theo naming convention |
| 3.4 | **Interface location**: Interface ở consumer package hay domain package? | Coding standard nói consumer package. Ví dụ: interface `Storage` định nghĩa ở `pkg/storage/`, không phải `pkg/entity/` |

## 4. Kiểu Types & internal/types

| # | Câu hỏi | Gợi ý | Doc tham chiếu |
|---|---------|-------|----------------|
| 4.1 | **EntityID** là `string` (UUIDv7 dạng text) hay `[16]byte`? | String dễ debug, byte array nhanh hơn | `docs/adr/009-rpc-contract.md` dùng string trong proto |
| 4.2 | **ZoneID** format: `{runtime_id}:{grid_x}:{grid_y}` hay `string` UUIDv7 riêng? | ADR-009 dùng `ZoneID` message với 3 fields. Nhưng trong Go có nên dùng struct? |
| 4.3 | **Vector3** dùng `float32` hay `float64`? | Proto dùng `double` (= float64). Game thường dùng float32 để tiết kiệm memory/bandwidth |
| 4.4 | **Runtime status constants** có nên là `iota` enum trong `internal/types/`? | `Creating`, `Active`, `Draining`, `Destroyed` | `docs/glossary.md` |
| 4.5 | **Zone status constants**: `unowned`, `active`, `transferring`, `orphan` | Có nên dùng `string` constants hay `int` iota? |
| 4.6 | **Entity type** là string free-form, vậy type registry có trong Phase 1 không? | Nếu Phase 1 chỉ tạo kiểu thì có thể skip |

## 5. Protobuf Definitions

| # | Câu hỏi | Gợi ý | Doc tham chiếu |
|---|---------|-------|----------------|
| 5.1 | **Protobuf package** = `spatialserver.v1`? Versioning strategy? | `docs/adr/009-rpc-contract.md` |
| 5.2 | **Proto directory layout**: 1 file per service hay 1 file tất cả? | Hiện tại tách riêng từng file |
| 5.3 | **Có cần** `google.protobuf.Timestamp` hay dùng string ISO8601? | Timestamp là proto standard |
| 5.4 | **4 services đều cần ngay Phase 1?** | `SpatialServerAPI` cần khi integrate với Business Backend |
| 5.5 | **Tool chain**: `protoc` thuần hay dùng `buf`? | `buf` có `buf lint`, `buf breaking`, CLI completion |
| 5.6 | **Generated code output**: `gen/` ở root repo hay `proto/gen/`? | — |
| 5.7 | **gateway.proto** có thực sự cần không nếu Gateway không expose gRPC? | Có thể skip Phase 1 |
| 5.8 | **Error codes** trong proto: dùng `google.rpc.Status` hay custom enum? | ADR-009 định nghĩa custom error codes |

## 6. Entity Package (pkg/entity)

| # | Câu hỏi | Gợi ý | Doc tham chiếu |
|---|---------|-------|----------------|
| 6.1 | **Entity struct** bao gồm những field nào ở Phase 1? | `ID`, `Type`, `Position`, `Rotation`, `Attributes (map[string][]byte)`, `ZoneID`, `OwnerID` | ADR-023 |
| 6.2 | **Lifecycle interface**: chỉ `Spawn`/`Despawn` hay thêm `OnInit`/`OnDestroy`? | GoWorld có 10+ methods. Chúng ta nên tối giản |
| 6.3 | **Entity attributes**: `map[string]interface{}` hay `map[string][]byte`? | ADR-023 nói `map<string, bytes>` = `map[string][]byte` |
| 6.4 | **Entity factory pattern**: global registry (`RegisterType()`) hay interface-based? | ADR-023: type registry không cần recompile |
| 6.5 | **Có cần component system** (ECS) ngay từ Phase 1 không? | Phức tạp, có thể đợi Phase sau |
| 6.6 | **ID generation**: function riêng mỗi lần tạo entity? | `uuid.New()` v7 wrapper trong `pkg/entity/` hay `internal/types/`? |
| 6.7 | **Thread safety**: Entity có cần mutex không? (Nếu single-threaded game loop thì không cần) | GoWorld không cần. Nhưng kiến trúc của chúng ta support concurrent |

## 7. Zone Package (pkg/zone)

| # | Câu hỏi | Gợi ý | Doc tham chiếu |
|---|---------|-------|----------------|
| 7.1 | **Zone struct** Phase 1 gồm những gì? | `ZoneID`, `RuntimeID`, `GridX`, `GridY`, `Status`, `GameServerID` |
| 7.2 | **Zone status machine**: implement bằng state pattern hay switch? | `unowned -> active -> transferring -> orphan` |
| 7.3 | **Zone grid operations**: tính adjacent zones? Cần trong Phase 1 (cho AOI) hay đợi sau? | Cần cho AOI boundary queries |
| 7.4 | **Zone size**: configurable per-Runtime hay global default 100? | ADR-023 nói 100x100 global |

## 8. AOI Package (pkg/aoi)

| # | Câu hỏi | Gợi ý | Doc tham chiếu |
|---|---------|-------|----------------|
| 8.1 | **Grid cell size** = zone size (100x100) hay subdivide zone? | ADR-023: zone = AOI grid cell, 3x3 query = 300 units radius |
| 8.2 | **AOI data structure**: grid of sorted lists (`[]EntityID`) hay spatial hash? | O(n log n) per cell, sorted |
| 8.3 | **AOI operations tối thiểu Phase 1?** | `Enter`, `Leave`, `Move`, `Query` |
| 8.4 | **AOI interface**: consumer package nào? | `pkg/entity/Entity` cần AOI, nhưng interface nên ở consumer (`pkg/aoi/`) |
| 8.5 | **Entity ID vs Entity pointer** trong AOI index? | Pointer giúp direct access, string ID giúp serializable |
| 8.6 | **Ghost entity support** có trong Phase 1 không? | Hay để Phase Cross-Zone? |

## 9. Config System

| # | Câu hỏi | Gợi ý | Doc tham chiếu |
|---|---------|-------|----------------|
| 9.1 | **Koanf version**: dùng `koanf v2` mới nhất? | — |
| 9.2 | **Config structs**: shared hay mỗi service một struct? | Shared base + per-service extension |
| 9.3 | **Config files**: `configs/defaults.yml` + `configs/{service}.yml`? | Như doc |
| 9.4 | **Có cần** config validation schema? | Koanf có validator integration |
| 9.5 | **Environment variable prefix**: `SPATIAL_`? | Như standards |

## 10. Makefile & Tooling

| # | Câu hỏi | Gợi ý | Doc tham chiếu |
|---|---------|-------|----------------|
| 10.1 | **Makefile targets tối thiểu?** | `build`, `test`, `lint`, `proto`, `ci`, `dev-up`, `dev-down` | AGENTS.md |
| 10.2 | **Go lint tool**: `golangci-lint` version cụ thể? | `v1.60+` |
| 10.3 | **Có cần** `go generate` cho mock generation? | Gomock hay mockgen? | Pitaya dùng Gomock |
| 10.4 | **Có cần** pre-commit hooks? | Nếu có, dùng `lefthook` hay `pre-commit`? |
| 10.5 | **Có cần** Docker Compose ngay Phase 1? | Cần cho integration tests với PostgreSQL/Redis |

## 11. Error Handling

| # | Câu hỏi | Gợi ý | Doc tham chiếu |
|---|---------|-------|----------------|
| 11.1 | **Sentinel errors** định nghĩa ở đâu? | `internal/types/errors.go`? Hay mỗi package tự define? | Error handling standard |
| 11.2 | **Error wrapping format**: `"entity %s: %w"`? Bỏ `"failed to"`? | Đồng ý bỏ "failed to" |
| 11.3 | **gRPC error mapping**: cần helper function trong Phase 1? | `pkg/rpc/errors.go` |

## 12. Testing Strategy

| # | Câu hỏi | Gợi ý | Doc tham chiếu |
|---|---------|-------|----------------|
| 12.1 | **Test target coverage %** cho Phase 1? | 70%? 80%? |
| 12.2 | **Table-driven tests** cho tất cả? | OK theo convention |
| 12.3 | **Test naming**: `TestXxx_GivenY_WhenZ`? | OK theo convention |
| 12.4 | **Có cần** integration tests trong Phase 1? | Nếu Phase 1 không có DB thì chỉ unit test |
| 12.5 | **Race detection**: `-race` flag có bắt buộc mọi `go test`? | Nên có |

## 13. Dependency Management

| # | Câu hỏi | Gợi ý | Lý do |
|---|---------|-------|-------|
| 13.1 | **External deps cho Phase 1**: package nào được phép dùng? | `google.golang.org/protobuf`, `google.golang.org/grpc`, `github.com/google/uuid` (v7), `github.com/knadh/koanf` |
| 13.2 | **UUID library**: `github.com/google/uuid` có support UUIDv7 không? | Check version. Nếu không thì tự implement hoặc dùng `github.com/gofrs/uuid` |
| 13.3 | **Có cần** `testify/assert` hay dùng stdlib `testing`? | testify phổ biến hơn |

## 14. Phase 1 Scope — Final Check

| # | Câu hỏi | Phase 1 dự kiến |
|---|---------|-----------------|
| 14.1 | **Có làm** `apps/` binary nào trong Phase 1 không? | Không — Phase 1 chỉ là library |
| 14.2 | **Có cần** Cloud Infrastructure code (Terraform, Helm) trong Phase 1? | Không |
| 14.3 | **Có cần** Dockerfile / Docker Compose? | Có thể có `docker-compose.yml` cho PostgreSQL + Redis (cho test) |
| 14.4 | **Có cần** CI/CD workflow (GitHub Actions) trong Phase 1? | Tối thiểu: `lint`, `test`, `build` |
| 14.5 | **Có cần** scripts (`dev-up.sh`, `dev-down.sh`)? | Có thể skip nếu dùng Docker Compose direct |

---

## Tổng Kết

**Phase 1 scope (nếu đồng ý):**

```
internal/types/       → EntityID, Vector3, ZoneID, status constants, error sentinels
pkg/entity/           → Entity struct, lifecycle interface, factory
pkg/zone/             → Zone struct, status machine, grid operations
pkg/aoi/              → Grid-based AOI: Enter, Leave, Move, Query
pkg/config/           → Koanf setup, shared config structs
pkg/logging/          → Slog wrapper, standard fields
proto/                → 4 .proto files, generated code
gen/                  → Protobuf Go code
Makefile              → build, test, lint, proto, ci
.github/workflows/    → Lint + test + build
configs/              → defaults.yml
go.mod / go.sum
```

**Tổng số câu hỏi: ~50.** Có thể trả lời từng phần. Phần critical nhất là: **1.1 (module path)**, **5.x (protobuf toolchain)**, **14.x (scope boundaries)**.
