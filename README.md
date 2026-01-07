# High Performance AI Gateway with Semantic Caching

**Tech Stack:** Golang (`net/http`), Qdrant (Vector DB), Postgres, Cybertron (Transformers in Go)

This project is a high-performance middleware built to drastically cut LLM costs for companies by intercepting and caching repeating queries. It sits between the user and providers (like OpenAI/Gemini), routing queries to the most cost-effective model based on complexity.

The philosophy was simple: **optimise for cost without sacrificing a millisecond of latency**.

I avoided heavy frameworks (like Gin or Fiber) and stuck to Go's native `net/http` to ensure a lightweight, low-memory footprint.

---

## 🏗️ Architecture
<img width="2379" height="912" alt="image" src="https://github.com/user-attachments/assets/c0aa5c48-816c-48d3-96c1-fd1866c0d49b" />

## 🚀 Key Features:


### 1. Semantic Caching (The Cost Cutter)
Instead of caching exact string matches, the system uses **semantic caching**.

- **Vector Database:** Qdrant is used for its speed and high RAM efficiency compared to alternatives.
- **Freshness:** Query payloads are inserted with a TTL (Time-To-Live). A background Goroutine runs at specified intervals to sweep and clear old cache entries, ensuring data freshness without manual intervention.
- **Logic:** Non-dynamic / time-insensitive queries are intercepted. If a similar question exists in the vector store, the cached answer is served instantly (<200ms), completely bypassing the expensive LLM call.

---

### 2. Resilient Embedding Layer (The "Race" Logic)
This is where most of the performance tuning happens.

* **Local Embeddings:** Uses **Cybertron**, a pure Go transformer library, to generate embeddings locally in under ~90ms (no Python dependencies).
* **Bounded Concurrency (Worker Pool):** instead of spawning a Goroutine per request (which risks CPU thrashing or OOM errors during high load), embedding tasks are queued into a buffered channel and processed by a fixed-size worker pool. This ensures predictable RAM usage and thread safety by limiting parallel inference tasks.
* **Strict Context Timeout Strategy:**
    * **The 200ms Rule:** A separate context timer tracks embedding generation. If it takes >200ms, the wait is aborted and the request is immediately forwarded to the LLM. The user never waits on a slow cache check.
    * **Background Caching:** Even if the cache is skipped, the worker continues processing the embedding in the background. If it completes, it's cached silently—ensuring the *next* user gets the fast cache hit.

---

### 3. Smart LLM Routing
Not every query needs GPT-4.

- Incoming queries are classified as **Simple** or **Complex**.
- **Simple Queries:** Routed to cheaper and faster models (maximum cost savings).
- **Complex Queries:** Routed to more capable (and more expensive) reasoning models.

---

### 4. Non-Blocking Storage Layer (Worker Pools)
Every request and its metadata is logged to Postgres for analytics, with **zero impact on API latency**.

- **Worker Pool Architecture:** Database writes are handled by a pool of background workers.
- The main API handler *fires-and-forgets* log data to a channel and returns the response immediately.
- This allows the system to handle traffic spikes without choking on DB locks or write latency.

---

### 5. Rate Limiting (Cost & Abuse Protection)
Protects against runaway costs and ensures fair resource allocation.

- **Per-User Limits:** Configurable request limits per API key/user to prevent individual users from exhausting the system.
- **Global Throttling:** System-wide rate limits protect against traffic spikes and ensure predictable infrastructure costs.
- **Graceful Degradation:** Rate-limited requests receive clear HTTP 429 responses, allowing clients to implement retry logic.

This is critical for production deployments—without rate limiting, a single bad actor or misconfigured client could generate thousands of expensive LLM calls in seconds.

---

## 📊 Endpoints

- **POST `/chat`**  
  Main entry point. Handles semantic search, routing, and response generation.

- **GET `/stats`**  
  Returns real-time analytics on gateway performance:
  - **Cost Saved:** Calculated based on token usage avoided via cache.
  - **Cache Hit %:** Effectiveness of the semantic caching layer.

## 🧠 Engineering Decisions & Trade-offs

Building a system is about managing trade-offs. Here is why I made specific architectural choices:

### 1. `net/http` vs. Frameworks (Gin/Fiber)
* **Decision:** I stuck to Go's standard library (`net/http`) rather than using a framework like Gin or Fiber.
* **The Trade-off:** While frameworks offer convenient routing and middleware macros, they introduce external dependencies, reflection overhead, and "magic" that obscures control flow.
* **The Win:** By using the standard lib, I kept the binary size small, the memory footprint minimal, and the latency predictable. For a high-throughput Gateway, avoiding the overhead of a router based on reflection was a priority.

### 2. Worker Pools vs. Unbounded Goroutines
* **Decision:** I implemented a **Worker Pool pattern** for the Embedding Layer instead of spawning a new Goroutine for every incoming request.

* **The Why (Backpressure):** Matrix multiplication (embedding generation) is CPU-intensive. If 1,000 requests hit the gateway simultaneously, spawning 1,000 Goroutines would cause CPU thrashing (excessive context switching) and likely trigger an OOM (Out of Memory) kill.
* **The Win:** The Worker Pool applies **backpressure**. It limits concurrent heavy-lifting to a fixed number of workers (preventing resource exhaustion) while queuing excess requests. This ensures the system degrades gracefully under load rather than crashing.

### 3. Latency vs. Consistency (The "Race" Strategy)
* **Decision:** I allow the system to "fail open." If the embedding generation takes longer than 200ms, we skip the cache and go straight to the LLM.
* **The Trade-off:** We might pay for an LLM token even if we *technically* had the answer in the cache (it was just too slow to retrieve).
* **The Win:** **User Experience (UX) is king.** A user should never wait 500ms for a "cache miss" before the actual LLM generation even starts. We optimize for the P99 latency of the user, treating cost-saving as a secondary (but high) priority.
---

## 🛠️ How to Run

### Prerequisites
* Go 1.22+
* Docker & Docker Compose

### Quick Start
1. **Start the Infrastructure** (Postgres & Qdrant):
   ```bash
   docker-compose up -d
   ```
2. **Run the AI Gateway**:
   ```bash
    go run .
   ```
