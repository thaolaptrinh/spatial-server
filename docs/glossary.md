# Glossary

> **Last Updated:** 2026-06-26

| Term | Definition |
|------|------------|
| **App Client** | The client application built with Unity, targeting Desktop (Windows/macOS/Linux), Mobile (iOS/Android), and WebGL platforms. Communicates with Spatial Server via WebSocket (WSS) using the binary packet protocol. |
| **AOI** | Area of Interest. The set of entities within a player's visual range. |
| **AOI Radius** | The distance from a player within which entities are considered visible (default 300 units). |
| **Backpressure** | Overload mitigation strategy where non-critical updates are dropped before the game loop blocks. |
| **Binary Packet Protocol** | Length-prefixed binary format for client↔Gateway communication. |
| **Business Backend** | External application that owns authentication, user management, and room/showroom metadata. Spatial Server's upstream caller. |
| **Degraded Mode** | Operational mode when a dependency (PostgreSQL/Redis) is unavailable. |
| **Degraded Write Queue** | Bounded buffer for queued writes during PostgreSQL outage. |
| **Entity** | A simulated object within a runtime (player avatar, NPC, interactive object). *Note: only player entities are currently implemented; the seeded NPC is a static demo placeholder.* Has position, attributes, and a type. |
| **Game Server** | Core service that simulates entities within its owned zones. |
| **Gateway** | Edge service that terminates WebSocket connections from clients. |
| **Gateway Cache** | Cached routing table in Gateway (TTL: 5s). |
| **Ghost Entity** | An entity whose owner has moved to a new zone but is still visible in the old zone for a brief period (smooth transition). |
| **Grid Size** | The dimensions of a single zone cell (default 100x100 units). |
| **gRPC Streaming** | Server-streaming RPC for zone state transfer. |
| **Heartbeat** | Periodic signal from Game Server to Room Service indicating liveness. |
| **HPA** | Horizontal Pod Autoscaler for Kubernetes (K3s). |
| **Leader Election** | Process of selecting a single active coordinator among replicas. |
| **mTLS** | Mutual TLS for internal service authentication. |
| **Orphan Zone** | Zone whose owning Game Server has failed. |
| **Player** | A human participant connected to a runtime. Each player has a Gateway session and a Game Server entity in a 1:1:1 relationship. |
| **Session** | The logical connection between a player and Spatial Server, spanning a Gateway WebSocket connection and a Game Server entity. Sessions survive brief disconnections via reconnection tokens. |
| **Ownership** | The binding of a zone to exactly one Game Server at any time. |
| **PDB** | PodDisruptionBudget for Kubernetes (K3s). |
| **Replay Protection** | Sequence number mechanism preventing packet replay. |
| **Room Service** | Coordinator service that manages zone ownership and load balancing. |
| **Runtime** | An instantiated realtime session (corresponding to a business room). Composed of one or more zones. |
| **Runtime Token** | Short-lived JWT issued by Business Backend, presented by clients to authenticate with Spatial Server. |
| **Service Discovery** | Mechanism for services to find each other. |
| **Session Token** | Short-lived token for reconnection after disconnect. |
| **Simulation Framework** | Dedicated load testing framework for benchmarking. |
| **Split-brain** | State where two servers believe they own the same zone. |
| **Tick** | One iteration of the Game Server simulation loop. Configurable rate (default 20Hz). |
| **WSS** | WebSocket Secure (WebSocket over TLS). |
| **Zone** | A grid cell within a runtime. The atomic unit of ownership. |
| **Zone State Persistence** | Periodic saving of zone state to PostgreSQL. |
| **Zone Transfer** | The process of moving zone ownership from one Game Server to another. |
