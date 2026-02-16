# High Performance AI Gateway with Semantic Caching

**Tech Stack:** Golang (`net/http`), Qdrant (Vector DB), Postgres, gRPC, Python (Embedding Service)

This project is a high-performance middleware built to drastically cut LLM costs for companies by intercepting and repeating queries. It sits between the user and providers (like OpenAI/Gemini), routing queries to the most cost-effective model based on complexity.

The philosophy was simple: **optimise for cost without sacrificing a millisecond of latency**.

I avoided heavy frameworks (like Gin or Fiber) and stuck to Go's native `net/http` to ensure a lightweight, low-memory footprint.

---

## 🏗️ Architecture
<img width="2379" height="912" alt="image" src="https://github.com/user-attachments/assets/c0aa5c48-816c-48d3-96c1-fd1866c0d49b" />

---

## 🔥 Performance Engineering: The "Flame Graph" Refactor using Pprof

During the initial development, I identified a critical bottleneck. The embedding model (originally running directly in Go via Cybertron) was throttling the CPU.

### 1. The Bottleneck (CPU-Bound Monolith)
Profiling the application with `pprof` revealed that matrix multiplication operations (`DotProdAVX32`) were consuming nearly 100% of the CPU time. This "stop-the-world" math computation blocked the Go runtime, preventing it from handling concurrent HTTP requests efficiently.

<img width="1916" height="766" alt="Screenshot 2026-01-18 145340" src="https://github.com/user-attachments/assets/b7852922-8d63-4f1c-ab0a-917ec4710ed2" />

*(Figure 1: Before Optimization. The wide orange bars show the CPU stuck performing vector math, leaving no room for I/O handling.)*


### 2. The Solution (Decoupling via gRPC)
To fix this, I refactored the architecture by extracting the embedding layer into a **dedicated Python microservice** running the BGE-M3 model, communicating with the Go Gateway via **gRPC**.

### 3. The Result (I/O-Bound Distributed System)
The new profile shows a drastic transformation. The Go Gateway is now **I/O-bound**, spending its time efficiently scheduling goroutines and waiting on network calls (`grpc.Invoke`). The heavy compute is isolated in the Python service, allowing the Gateway to maintain high throughput and concurrency without locking the main thread.

<img width="1919" height="734" alt="Screenshot 2026-02-16 165602" src="https://github.com/user-attachments/assets/483de937-b9ca-4e71-8001-409f34a823ed" />

*(Figure 2: After Optimization. The load is distributed. The CPU is no longer choked by a single function, and the runtime efficiently manages network wait times.)*

---

## 🚀 Key Features:

### 1. Semantic Caching (The Cost Cutter)
Instead of caching exact string matches, the system uses **semantic caching**.

- **Vector Database:** Qdrant is used for its speed and high RAM efficiency.
- **Freshness:** Query payloads are inserted with a TTL (Time-To-Live). A background Goroutine runs at specified intervals to sweep and clear old cache entries.
- **Logic:** Non-dynamic queries are intercepted. If a similar question exists in the vector store, the cached answer is served instantly (<200ms), completely bypassing the expensive LLM call.

### 2. Decoupled Embedding Layer (gRPC Microservice)
This is where the system ensures scalability.

* **Microservice Architecture:** The embedding generation is offloaded to a lightweight Python service via **gRPC**. This allows the Go server to remain responsive even under heavy load.
* **Model Maturity:** Using Python allows access to state-of-the-art embedding models (like BGE-M3) and optimized libraries (ONNX/PyTorch) that are more mature than their Go counterparts.
* **Strict Context Timeouts:** The Gateway enforces a strict **200ms** timeout on the gRPC call. If the embedding service is too slow, the request "fails open" and proceeds directly to the LLM to preserve user experience.

### 3. Smart LLM Routing
Not every query needs GPT-4.

- Incoming queries are classified as **Simple** or **Complex**.
- **Simple Queries:** Routed to cheaper and faster models.
- **Complex Queries:** Routed to reasoning models.

### 4. Non-Blocking Storage Layer (Worker Pools)
Every request and its metadata is logged to Postgres for analytics with **zero impact on API latency**.

- **Worker Pool Architecture:** Database writes are handled by a pool of background workers.
- The main API handler *fires-and-forgets* log data to a channel and returns the response immediately.

### 5. Rate Limiting (Cost & Abuse Protection)
Protects against runaway costs and ensures fair resource allocation.

- **Per-User Limits:** Configurable request limits per API key.
- **Global Throttling:** System-wide rate limits protect against traffic spikes.
- **Graceful Degradation:** Rate-limited requests receive clear HTTP 429 responses.

---

## 📊 Endpoints

- **POST `/chat`** Main entry point. Handles semantic search, routing, and response generation.

- **GET `/stats`** Returns real-time analytics on gateway performance (Cost Saved, Cache Hit %).

---

## 🧠 Engineering Decisions & Trade-offs

### 1. `net/http` vs. Frameworks
* **Decision:** Stuck to Go's standard library (`net/http`) over Gin/Fiber.
* **The Win:** Minimal memory footprint and no reflection overhead, critical for a high-throughput middleware.

### 2. Monolith vs. Microservices (The Pivot)
* **Decision:** Moved from pure Go embeddings to a Python gRPC service.
* **The Why:** Go is excellent for I/O and concurrency, but Python (with C++ bindings) is superior for Tensor operations.
* **The Win:** By letting each language do what it does best, I achieved **Heterogeneous Scaling**. I can now scale the lightweight Gateway separately from the compute-heavy Embedding Service.

### 3. Latency vs. Consistency (The "Race" Strategy)
* **Decision:** "Fail open" on cache misses or timeouts.
* **The Win:** **User Experience is king.** We optimize for P99 latency, ensuring no user waits excessively for a cache check.

---

## ⚡ Performance & Observability

The AI Gateway is fully instrumented with **OpenTelemetry (OTel)** to provide deep visibility into the request lifecycle. Traces are exported to **Jaeger**.

### The "Cache Win" (Benchmarks)

| Metric | Direct LLM Call (Gemini Flash) | Cached Response (Qdrant) | **Improvement** |
| :--- | :--- | :--- | :--- |
| **Latency** | ~4.98s | ~88ms | **56x Faster** |
| **Cost** | $$ (Input + Output Tokens) | $0.00 | **100% Savings** |

---

## 🛠️ How to Run

### Prerequisites
* Go 1.22+
* Docker & Docker Compose
* Make (optional, for convenience)

### Quick Start
1. **Start the Infrastructure** (Postgres, Qdrant, & Python Service):
   ```bash
   docker compose up -d
   ```
2. **Run the AI Gateway**:
   ```bash
   make run
   ```
   
